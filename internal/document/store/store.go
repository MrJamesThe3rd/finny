package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

var _ document.Repository = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// auditBackend is the snapshot of a backend written to audit_log. It never
// includes config: that JSONB holds the backend's credentials, and audit_log
// is read far more widely than document_backends.
func auditBackend(cfg *document.BackendConfig) map[string]any {
	return map[string]any{
		"type":    cfg.Type,
		"name":    cfg.Name,
		"enabled": cfg.Enabled,
	}
}

// Backend operations

func (s *Store) ListBackends(ctx context.Context) ([]document.BackendConfig, error) {
	const query = `
		SELECT id, type, name, config, enabled, created_at
		FROM document_backends
		WHERE org_id = $1
		ORDER BY created_at ASC
	`

	return s.queryBackends(ctx, query)
}

func (s *Store) ListEnabledBackends(ctx context.Context) ([]document.BackendConfig, error) {
	const query = `
		SELECT id, type, name, config, enabled, created_at
		FROM document_backends
		WHERE org_id = $1 AND enabled
		ORDER BY created_at ASC
	`

	return s.queryBackends(ctx, query)
}

func (s *Store) queryBackends(ctx context.Context, query string) ([]document.BackendConfig, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing backends: %w", err)
	}
	defer rows.Close()

	var backends []document.BackendConfig

	for rows.Next() {
		var b document.BackendConfig

		var rawConfig []byte

		if err := rows.Scan(&b.ID, &b.Type, &b.Name, &rawConfig, &b.Enabled, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning backend: %w", err)
		}

		b.Config = json.RawMessage(rawConfig)
		backends = append(backends, b)
	}

	return backends, rows.Err()
}

func (s *Store) GetBackend(ctx context.Context, id uuid.UUID) (*document.BackendConfig, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT id, type, name, config, enabled, created_at
		FROM document_backends
		WHERE id = $1 AND org_id = $2
	`

	var b document.BackendConfig

	var rawConfig []byte

	err = s.db.QueryRowContext(ctx, query, id, orgID).Scan(
		&b.ID, &b.Type, &b.Name, &rawConfig, &b.Enabled, &b.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, document.ErrBackendNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting backend: %w", err)
	}

	b.Config = json.RawMessage(rawConfig)

	return &b, nil
}

func (s *Store) CreateBackend(ctx context.Context, cfg *document.BackendConfig) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		const query = `
			INSERT INTO document_backends (org_id, type, name, config, enabled)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at
		`

		if err := tx.QueryRowContext(ctx, query, orgID, cfg.Type, cfg.Name, []byte(cfg.Config), cfg.Enabled).
			Scan(&cfg.ID, &cfg.CreatedAt); err != nil {
			return fmt.Errorf("creating backend: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectBackend,
			SubjectID:   cfg.ID,
			Action:      audit.ActionCreate,
			NewValue:    auditBackend(cfg),
		})
	})
}

func (s *Store) UpdateBackend(ctx context.Context, id uuid.UUID, name *string, config json.RawMessage, enabled *bool) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		old, err := lockBackend(ctx, tx, id, orgID)
		if err != nil {
			return err
		}

		// config is replaced wholesale. Merging a partial patch is the service's
		// job: jsonb || only merges at the top level, happily concatenates a
		// non-object into an array, and cannot tell "set this key to null" from
		// "leave it alone" — all three of which are reachable from a PATCH body.
		const query = `
			UPDATE document_backends
			SET
				name    = COALESCE($1, name),
				config  = COALESCE($2, config),
				enabled = COALESCE($3, enabled)
			WHERE id = $4 AND org_id = $5
		`

		var nameArg any = name

		var configArg any = []byte(config)
		if config == nil {
			configArg = nil
		}

		var enabledArg any = enabled

		if _, err := tx.ExecContext(ctx, query, nameArg, configArg, enabledArg, id, orgID); err != nil {
			return fmt.Errorf("updating backend: %w", err)
		}

		next := *old
		if name != nil {
			next.Name = *name
		}

		if enabled != nil {
			next.Enabled = *enabled
		}

		newValue := auditBackend(&next)
		newValue["config_changed"] = config != nil

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectBackend,
			SubjectID:   id,
			Action:      audit.ActionUpdate,
			OldValue:    auditBackend(old),
			NewValue:    newValue,
		})
	})
}

func (s *Store) DeleteBackend(ctx context.Context, id uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		old, err := lockBackend(ctx, tx, id, orgID)
		if err != nil {
			return err
		}

		const query = `DELETE FROM document_backends WHERE id = $1 AND org_id = $2`

		if _, err := tx.ExecContext(ctx, query, id, orgID); err != nil {
			return fmt.Errorf("deleting backend: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectBackend,
			SubjectID:   id,
			Action:      audit.ActionDelete,
			OldValue:    auditBackend(old),
		})
	})
}

// lockBackend reads a backend FOR UPDATE, giving both the org check and the
// prior value for audit_log.old_value.
func lockBackend(ctx context.Context, tx *sql.Tx, id, orgID uuid.UUID) (*document.BackendConfig, error) {
	const query = `
		SELECT id, type, name, enabled
		FROM document_backends
		WHERE id = $1 AND org_id = $2
		FOR UPDATE
	`

	var b document.BackendConfig

	err := tx.QueryRowContext(ctx, query, id, orgID).Scan(&b.ID, &b.Type, &b.Name, &b.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, document.ErrBackendNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("locking backend: %w", err)
	}

	return &b, nil
}

func (s *Store) BackendHasDocuments(ctx context.Context, backendID uuid.UUID) (bool, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return false, err
	}

	const query = `
		SELECT EXISTS(
			SELECT 1 FROM document_locations l
			JOIN document_backends b ON b.id = l.backend_id
			WHERE l.backend_id = $1 AND b.org_id = $2
		)
	`

	var exists bool
	if err := s.db.QueryRowContext(ctx, query, backendID, orgID).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking backend documents: %w", err)
	}

	return exists, nil
}

// Document operations

func (s *Store) CreateDocument(ctx context.Context, doc *document.Document) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		const query = `
			INSERT INTO documents (org_id, filename, mime_type)
			VALUES ($1, $2, $3)
			RETURNING id, created_at
		`

		if err := tx.QueryRowContext(ctx, query, orgID, doc.Filename, doc.MIMEType).
			Scan(&doc.ID, &doc.CreatedAt); err != nil {
			return fmt.Errorf("creating document: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectDocument,
			SubjectID:   doc.ID,
			Action:      audit.ActionCreate,
			NewValue:    map[string]any{"filename": doc.Filename, "mime_type": doc.MIMEType},
		})
	})
}

func (s *Store) GetDocument(ctx context.Context, id uuid.UUID) (*document.Document, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT id, filename, mime_type, created_at
		FROM documents
		WHERE id = $1 AND org_id = $2
	`

	var doc document.Document

	err = s.db.QueryRowContext(ctx, query, id, orgID).
		Scan(&doc.ID, &doc.Filename, &doc.MIMEType, &doc.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, document.ErrDocumentNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting document: %w", err)
	}

	return &doc, nil
}

func (s *Store) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		const lockQuery = `
			SELECT filename, mime_type FROM documents
			WHERE id = $1 AND org_id = $2
			FOR UPDATE
		`

		var filename, mimeType string

		err := tx.QueryRowContext(ctx, lockQuery, id, orgID).Scan(&filename, &mimeType)
		if errors.Is(err, sql.ErrNoRows) {
			return document.ErrDocumentNotFound
		}

		if err != nil {
			return fmt.Errorf("locking document: %w", err)
		}

		const query = `DELETE FROM documents WHERE id = $1 AND org_id = $2`

		if _, err := tx.ExecContext(ctx, query, id, orgID); err != nil {
			return fmt.Errorf("deleting document: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectDocument,
			SubjectID:   id,
			Action:      audit.ActionDelete,
			OldValue:    map[string]any{"filename": filename, "mime_type": mimeType},
		})
	})
}

func (s *Store) PurgeDocument(ctx context.Context, id uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	const query = `DELETE FROM documents WHERE id = $1 AND org_id = $2`

	if _, err := s.db.ExecContext(ctx, query, id, orgID); err != nil {
		return fmt.Errorf("purging document: %w", err)
	}

	return nil
}

// Location operations

func (s *Store) AddLocation(ctx context.Context, loc *document.Location) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	// document_locations carries no org_id of its own: a second copy of the
	// owner could drift from documents.org_id with nothing enforcing agreement.
	// The scope is reached through documents, so it cannot disagree.
	const query = `
		INSERT INTO document_locations (document_id, backend_id, key)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM documents d WHERE d.id = $1 AND d.org_id = $4)
		RETURNING id
	`

	err = s.db.QueryRowContext(ctx, query, loc.DocumentID, loc.BackendID, loc.Key, orgID).Scan(&loc.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return document.ErrDocumentNotFound
	}

	if err != nil {
		return fmt.Errorf("adding location: %w", err)
	}

	return nil
}

func (s *Store) ListLocations(ctx context.Context, documentID uuid.UUID) ([]document.Location, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
		SELECT l.id, l.document_id, l.backend_id, l.key
		FROM document_locations l
		WHERE l.document_id = $1
		  AND EXISTS (SELECT 1 FROM documents d WHERE d.id = l.document_id AND d.org_id = $2)
		ORDER BY l.created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, documentID, orgID)
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
