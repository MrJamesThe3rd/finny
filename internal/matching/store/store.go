package store

import (
	"context"
	"database/sql"
	"fmt"
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
		WHERE $1 ILIKE '%' || raw_pattern || '%'
		ORDER BY LENGTH(raw_pattern) DESC, created_at DESC
		LIMIT 1
	`

	var preferred string

	err := s.db.QueryRowContext(ctx, query, rawDescription).Scan(&preferred)
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
		INSERT INTO description_mappings (raw_pattern, preferred_description, created_at)
		VALUES ($1, $2, NOW())
	`

	_, err := s.db.ExecContext(ctx, query, rawPattern, preferredDescription)
	if err != nil {
		return fmt.Errorf("creating mapping: %w", err)
	}

	return nil
}
