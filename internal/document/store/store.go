package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/document"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Backend operations

func (s *Store) ListBackends(ctx context.Context) ([]document.BackendConfig, error) {
	query := `
		SELECT id, user_id, type, name, config, enabled, created_at
		FROM document_backends
		WHERE user_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, auth.UserID(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}
	defer rows.Close()

	var backends []document.BackendConfig

	for rows.Next() {
		var b document.BackendConfig
		var rawConfig []byte

		if err := rows.Scan(&b.ID, &b.UserID, &b.Type, &b.Name, &rawConfig, &b.Enabled, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning backend: %w", err)
		}

		b.Config = json.RawMessage(rawConfig)
		backends = append(backends, b)
	}

	return backends, rows.Err()
}

func (s *Store) GetBackend(ctx context.Context, id uuid.UUID) (*document.BackendConfig, error) {
	query := `
		SELECT id, user_id, type, name, config, enabled, created_at
		FROM document_backends
		WHERE id = $1
	`

	var b document.BackendConfig
	var rawConfig []byte

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&b.ID, &b.UserID, &b.Type, &b.Name, &rawConfig, &b.Enabled, &b.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, document.ErrBackendNotFound
		}

		return nil, fmt.Errorf("getting backend: %w", err)
	}

	b.Config = json.RawMessage(rawConfig)

	return &b, nil
}

func (s *Store) SetBackendConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) error {
	query := `UPDATE document_backends SET config = $1 WHERE id = $2`

	_, err := s.db.ExecContext(ctx, query, []byte(config), id)
	if err != nil {
		return fmt.Errorf("setting backend config: %w", err)
	}

	return nil
}

func (s *Store) CreateBackend(ctx context.Context, cfg *document.BackendConfig) error {
	query := `
		INSERT INTO document_backends (user_id, type, name, config, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	userID := auth.UserID(ctx)
	err := s.db.QueryRowContext(ctx, query, userID, cfg.Type, cfg.Name, []byte(cfg.Config), cfg.Enabled).
		Scan(&cfg.ID, &cfg.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}

	cfg.UserID = userID
	return nil
}

func (s *Store) UpdateBackend(ctx context.Context, id uuid.UUID, name *string, config json.RawMessage, enabled *bool) error {
	query := `
		UPDATE document_backends
		SET
			name    = COALESCE($1, name),
			config  = COALESCE($2, config),
			enabled = COALESCE($3, enabled)
		WHERE id = $4 AND user_id = $5
	`

	var nameArg any = name
	var configArg any = []byte(config)
	if config == nil {
		configArg = nil
	}
	var enabledArg any = enabled

	result, err := s.db.ExecContext(ctx, query, nameArg, configArg, enabledArg, id, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("updating backend: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return document.ErrBackendNotFound
	}

	return nil
}

func (s *Store) DeleteBackend(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM document_backends WHERE id = $1 AND user_id = $2`

	result, err := s.db.ExecContext(ctx, query, id, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("deleting backend: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return document.ErrBackendNotFound
	}

	return nil
}

func (s *Store) BackendHasDocuments(ctx context.Context, backendID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM document_locations WHERE backend_id = $1)`

	var exists bool
	if err := s.db.QueryRowContext(ctx, query, backendID).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking backend documents: %w", err)
	}

	return exists, nil
}

// Document operations

func (s *Store) CreateDocument(ctx context.Context, doc *document.Document) error {
	query := `
		INSERT INTO documents (user_id, filename, mime_type)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`

	err := s.db.QueryRowContext(ctx, query, doc.UserID, doc.Filename, doc.MIMEType).
		Scan(&doc.ID, &doc.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating document: %w", err)
	}

	return nil
}

func (s *Store) GetDocument(ctx context.Context, id uuid.UUID) (*document.Document, error) {
	query := `
		SELECT id, user_id, filename, mime_type, created_at
		FROM documents
		WHERE id = $1 AND user_id = $2
	`

	var doc document.Document

	err := s.db.QueryRowContext(ctx, query, id, auth.UserID(ctx)).
		Scan(&doc.ID, &doc.UserID, &doc.Filename, &doc.MIMEType, &doc.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, document.ErrDocumentNotFound
		}

		return nil, fmt.Errorf("getting document: %w", err)
	}

	return &doc, nil
}

func (s *Store) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM documents WHERE id = $1 AND user_id = $2`

	_, err := s.db.ExecContext(ctx, query, id, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	return nil
}

// Location operations

func (s *Store) AddLocation(ctx context.Context, loc *document.Location) error {
	query := `
		INSERT INTO document_locations (document_id, backend_id, key)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	err := s.db.QueryRowContext(ctx, query, loc.DocumentID, loc.BackendID, loc.Key).
		Scan(&loc.ID)
	if err != nil {
		return fmt.Errorf("adding location: %w", err)
	}

	return nil
}

func (s *Store) ListLocations(ctx context.Context, documentID uuid.UUID) ([]document.Location, error) {
	query := `
		SELECT id, document_id, backend_id, key
		FROM document_locations
		WHERE document_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, documentID)
	if err != nil {
		return nil, fmt.Errorf("listing locations: %w", err)
	}
	defer rows.Close()

	var locs []document.Location

	for rows.Next() {
		var loc document.Location

		if err := rows.Scan(&loc.ID, &loc.DocumentID, &loc.BackendID, &loc.Key); err != nil {
			return nil, fmt.Errorf("scanning location: %w", err)
		}

		locs = append(locs, loc)
	}

	return locs, rows.Err()
}
