package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) FindMatch(ctx context.Context, rawDescription string) (string, error) {
	query := `
		SELECT preferred_description
		FROM description_mappings
		WHERE user_id = $1 AND $2 ILIKE '%' || raw_pattern || '%'
		ORDER BY LENGTH(raw_pattern) DESC, created_at DESC
		LIMIT 1
	`

	var preferred string

	err := s.db.QueryRowContext(ctx, query, auth.UserID(ctx), rawDescription).Scan(&preferred)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}

		return "", fmt.Errorf("finding match: %w", err)
	}

	return preferred, nil
}

func (s *Store) CreateMapping(ctx context.Context, rawPattern, preferredDescription string) error {
	query := `
		INSERT INTO description_mappings (raw_pattern, preferred_description, user_id, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, raw_pattern) DO UPDATE SET preferred_description = EXCLUDED.preferred_description
	`

	_, err := s.db.ExecContext(ctx, query, rawPattern, preferredDescription, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("creating mapping: %w", err)
	}

	return nil
}
