package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

// paperlessKeyRe extracts the numeric document ID from a Paperless-ngx URL.
// e.g. "https://paperless.example.com/api/documents/42/download/" → "42"
var paperlessKeyRe = regexp.MustCompile(`/api/documents/(\d+)/`)

// LegacyPaperlessBackendID is the well-known UUID seeded by the
// 20260404000002_add_document_store.sql migration for the migrated Paperless backend.
var LegacyPaperlessBackendID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

// Service orchestrates document storage across multiple backends.
type Service struct {
	repo     Repository
	registry *Registry
}

func NewService(repo Repository, registry *Registry) *Service {
	return &Service{repo: repo, registry: registry}
}

// Download retrieves a document by ID, trying each known location until one succeeds.
// The caller must close the returned reader.
func (s *Service) Download(ctx context.Context, documentID uuid.UUID) (io.ReadCloser, *Document, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting document: %w", err)
	}

	locations, err := s.repo.ListLocations(ctx, documentID)
	if err != nil {
		return nil, nil, fmt.Errorf("listing locations: %w", err)
	}

	var errs []error

	for _, loc := range locations {
		cfg, err := s.repo.GetBackend(ctx, loc.BackendID)
		if err != nil {
			errs = append(errs, fmt.Errorf("backend %s: %w", loc.BackendID, err))
			continue
		}

		backend, err := s.registry.Create(cfg.Type, cfg.Config)
		if err != nil {
			errs = append(errs, fmt.Errorf("backend %s: creating backend: %w", loc.BackendID, err))
			continue
		}

		rc, err := backend.Download(ctx, loc.Key)
		if err != nil {
			errs = append(errs, fmt.Errorf("backend %s: download key %q: %w", loc.BackendID, loc.Key, err))
			continue
		}

		return rc, doc, nil
	}

	return nil, nil, fmt.Errorf("%w: %w", ErrNoAvailableLocation, errors.Join(errs...))
}

// AttachFromURL creates a document and location from a Paperless-ngx URL,
// linking it to the user's first enabled Paperless backend.
// Returns the created Document so the caller can link it to a transaction.
func (s *Service) AttachFromURL(ctx context.Context, rawURL string) (*Document, error) {
	matches := paperlessKeyRe.FindStringSubmatch(rawURL)
	if matches == nil {
		return nil, ErrURLNotSupported
	}

	key := matches[1]
	userID := auth.UserID(ctx)

	backends, err := s.repo.ListBackends(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}

	var backendCfg *BackendConfig

	for i := range backends {
		if backends[i].Type == "paperless" && backends[i].Enabled && backends[i].UserID == userID {
			backendCfg = &backends[i]
			break
		}
	}

	if backendCfg == nil {
		return nil, ErrNoBackends
	}

	doc := &Document{
		UserID:   userID,
		Filename: "invoice",
		MIMEType: "application/pdf",
	}

	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("creating document: %w", err)
	}

	loc := &Location{
		DocumentID: doc.ID,
		BackendID:  backendCfg.ID,
		Key:        key,
	}

	if err := s.repo.AddLocation(ctx, loc); err != nil {
		return nil, fmt.Errorf("adding location: %w", err)
	}

	return doc, nil
}

// Upload stores the file on all enabled backends and returns the created Document.
// Fails if no enabled backends are configured. Fails if any backend upload fails.
// The caller must not close content before this returns.
func (s *Service) Upload(ctx context.Context, filename, mimeType string, content io.Reader) (*Document, error) {
	userID := auth.UserID(ctx)

	backends, err := s.repo.ListBackends(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}

	var enabled []BackendConfig
	for _, b := range backends {
		if b.Enabled && b.UserID == userID {
			enabled = append(enabled, b)
		}
	}

	if len(enabled) == 0 {
		return nil, ErrNoBackends
	}

	// Buffer content once so it can be re-read by each backend.
	buf, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("reading upload content: %w", err)
	}

	doc := &Document{
		UserID:   userID,
		Filename: filename,
		MIMEType: mimeType,
	}

	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("creating document: %w", err)
	}

	// Track backends that successfully received the file so we can roll them
	// back if a later step fails.
	type uploadedLocation struct {
		backend Backend
		key     string
	}
	var uploaded []uploadedLocation

	rollback := func() {
		for _, u := range uploaded {
			if err := u.backend.Delete(ctx, u.key); err != nil {
				slog.Warn("rollback: failed to delete from backend", "key", u.key, "error", err)
			}
		}
		_ = s.repo.DeleteDocument(ctx, doc.ID)
	}

	for _, cfg := range enabled {
		backend, err := s.registry.Create(cfg.Type, cfg.Config)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("creating backend %s: %w", cfg.Name, err)
		}

		key, err := backend.Upload(ctx, filename, bytes.NewReader(buf))
		if err != nil {
			rollback()
			return nil, fmt.Errorf("uploading to backend %s: %w", cfg.Name, err)
		}

		uploaded = append(uploaded, uploadedLocation{backend: backend, key: key})

		loc := &Location{
			DocumentID: doc.ID,
			BackendID:  cfg.ID,
			Key:        key,
		}

		if err := s.repo.AddLocation(ctx, loc); err != nil {
			rollback()
			return nil, fmt.Errorf("recording location for backend %s: %w", cfg.Name, err)
		}
	}

	return doc, nil
}

// Delete removes a document from all backends and deletes its DB record.
// Backend deletions are best-effort; DB deletion always runs.
func (s *Service) Delete(ctx context.Context, documentID uuid.UUID) error {
	locations, err := s.repo.ListLocations(ctx, documentID)
	if err != nil {
		return fmt.Errorf("listing locations: %w", err)
	}

	for _, loc := range locations {
		cfg, err := s.repo.GetBackend(ctx, loc.BackendID)
		if err != nil {
			slog.Warn("failed to get backend for deletion", "backend_id", loc.BackendID, "error", err)
			continue
		}

		backend, err := s.registry.Create(cfg.Type, cfg.Config)
		if err != nil {
			slog.Warn("failed to create backend for deletion", "backend", cfg.Name, "error", err)
			continue
		}

		if err := backend.Delete(ctx, loc.Key); err != nil {
			slog.Warn("failed to delete from backend", "backend", cfg.Name, "key", loc.Key, "error", err)
		}
	}

	// CASCADE in the DB removes document_locations rows.
	return s.repo.DeleteDocument(ctx, documentID)
}

// ListBackends returns all backends configured for the requesting user.
func (s *Service) ListBackends(ctx context.Context) ([]BackendConfig, error) {
	return s.repo.ListBackends(ctx)
}

// CreateBackend creates a new backend configuration.
func (s *Service) CreateBackend(ctx context.Context, cfg *BackendConfig) error {
	return s.repo.CreateBackend(ctx, cfg)
}

// UpdateBackend updates mutable fields of a backend configuration.
func (s *Service) UpdateBackend(ctx context.Context, id uuid.UUID, name *string, config json.RawMessage, enabled *bool) error {
	return s.repo.UpdateBackend(ctx, id, name, config, enabled)
}

// DeleteBackend deletes a backend configuration.
func (s *Service) DeleteBackend(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteBackend(ctx, id)
}

// BackendHasDocuments reports whether any document location references the given backend.
func (s *Service) BackendHasDocuments(ctx context.Context, backendID uuid.UUID) (bool, error) {
	return s.repo.BackendHasDocuments(ctx, backendID)
}

// SeedLegacyBackend updates the legacy Paperless backend config from application config.
// Called at startup so the migrated backend works without manual DB edits.
func (s *Service) SeedLegacyBackend(ctx context.Context, baseURL, token string) error {
	legacyID := LegacyPaperlessBackendID

	cfg, err := json.Marshal(map[string]string{
		"base_url": baseURL,
		"token":    token,
	})
	if err != nil {
		return fmt.Errorf("marshalling backend config: %w", err)
	}

	return s.repo.SetBackendConfig(ctx, legacyID, json.RawMessage(cfg))
}
