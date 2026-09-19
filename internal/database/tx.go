package database

import (
	"context"
	"database/sql"
	"fmt"
)

// InTx runs fn inside a transaction, committing if it returns nil and rolling
// back otherwise. Used wherever a state change and its audit row must land
// together — see knowledge-base.md §4.
func InTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// Rollback after a successful commit is a no-op.
	defer tx.Rollback() //nolint:errcheck

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
