package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/MrJamesThe3rd/finny/internal/matching"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

var _ matching.Repository = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) FindMatch(ctx context.Context, rawDescription string) (string, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return "", err
	}

	const query = `
		SELECT preferred_description
		FROM description_mappings
		WHERE org_id = $1 AND $2 ILIKE '%' || raw_pattern || '%'
		ORDER BY LENGTH(raw_pattern) DESC, created_at DESC
		LIMIT 1
	`

	var preferred string

	err = s.db.QueryRowContext(ctx, query, orgID, rawDescription).Scan(&preferred)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("finding match: %w", err)
	}

	return preferred, nil
}

func (s *Store) CreateMapping(ctx context.Context, rawPattern, preferredDescription string) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO description_mappings (raw_pattern, preferred_description, org_id, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (org_id, raw_pattern) DO UPDATE SET preferred_description = EXCLUDED.preferred_description
	`

	if _, err := s.db.ExecContext(ctx, query, rawPattern, preferredDescription, orgID); err != nil {
		return fmt.Errorf("creating mapping: %w", err)
	}

	return nil
}
