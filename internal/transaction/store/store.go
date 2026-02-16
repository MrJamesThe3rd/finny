package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanTransaction reads a transaction row from the scanner and returns a populated Transaction.
// Expected column order: id, amount, type, status, description, raw_description, date, invoice_id, invoice_url, created_at, updated_at, deleted_at
func scanTransaction(s scanner) (*transaction.Transaction, error) {
	var tx transaction.Transaction

	var typeStr, statusStr string

	var rawDesc sql.NullString

	var invID *uuid.UUID

	var invoiceURL sql.NullString

	if err := s.Scan(
		&tx.ID, &tx.Amount, &typeStr, &statusStr, &tx.Description, &rawDesc, &tx.Date,
		&invID, &invoiceURL,
		&tx.CreatedAt, &tx.UpdatedAt, &tx.DeletedAt,
	); err != nil {
		return nil, err
	}

	tx.Type = transaction.Type(typeStr)
	tx.Status = transaction.Status(statusStr)
	tx.RawDescription = rawDesc.String
	tx.InvoiceID = invID

	if invoiceURL.Valid && invID != nil {
		tx.Invoice = &transaction.Invoice{
			ID:  *invID,
			URL: invoiceURL.String,
		}
	}

	return &tx, nil
}

const selectTransactionColumns = `
	t.id, t.amount, t.type, t.status, t.description, t.raw_description, t.date,
	t.invoice_id, i.url as invoice_url, t.created_at, t.updated_at, t.deleted_at
`

func (s *Store) CreateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	query := `
		INSERT INTO transactions (amount, type, status, description, raw_description, date, invoice_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(ctx, query,
		tx.Amount,
		tx.Type,
		tx.Status,
		tx.Description,
		tx.RawDescription,
		tx.Date,
		tx.InvoiceID,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}

	return nil
}

func (s *Store) GetTransaction(ctx context.Context, id uuid.UUID) (*transaction.Transaction, error) {
	query := `SELECT ` + selectTransactionColumns + `
		FROM transactions t
		LEFT JOIN invoices i ON t.invoice_id = i.id
		WHERE t.id = $1 AND t.deleted_at IS NULL`

	tx, err := scanTransaction(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, transaction.ErrNotFound
		}

		return nil, fmt.Errorf("getting transaction: %w", err)
	}

	return tx, nil
}

func (s *Store) ListTransactions(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error) {
	query := `SELECT ` + selectTransactionColumns + `
		FROM transactions t
		LEFT JOIN invoices i ON t.invoice_id = i.id
		WHERE t.deleted_at IS NULL`

	var args []any

	argIdx := 1

	if filter.Status != nil {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)

		args = append(args, *filter.Status)
		argIdx++
	}

	if filter.StartDate != nil {
		query += fmt.Sprintf(" AND t.date >= $%d", argIdx)

		args = append(args, *filter.StartDate)
		argIdx++
	}

	if filter.EndDate != nil {
		query += fmt.Sprintf(" AND t.date <= $%d", argIdx)

		args = append(args, *filter.EndDate)
		argIdx++
	}

	query += " ORDER BY t.date ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}
	defer rows.Close()

	var txs []*transaction.Transaction

	for rows.Next() {
		tx, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}

		txs = append(txs, tx)
	}

	return txs, nil
}

func (s *Store) UpdateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	query := `
		UPDATE transactions
		SET amount = $1, type = $2, status = $3, description = $4, invoice_id = $5, updated_at = NOW()
		WHERE id = $6 AND deleted_at IS NULL
	`

	_, err := s.db.ExecContext(ctx, query,
		tx.Amount,
		tx.Type,
		tx.Status,
		tx.Description,
		tx.InvoiceID,
		tx.ID,
	)
	if err != nil {
		return fmt.Errorf("updating transaction: %w", err)
	}

	return nil
}

func (s *Store) UpdateStatus(ctx context.Context, id uuid.UUID, status transaction.Status) error {
	query := `
		UPDATE transactions
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`

	_, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating status: %w", err)
	}

	return nil
}

func (s *Store) DeleteTransaction(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE transactions
		SET deleted_at = NOW()
		WHERE id = $1
	`

	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting transaction: %w", err)
	}

	return nil
}

// UpdateInvoice finds or creates an invoice by URL and links it to the transaction.
// Both operations are wrapped in a database transaction for atomicity.
func (s *Store) UpdateInvoice(ctx context.Context, txID uuid.UUID, invoiceURL string) error {
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer dbTx.Rollback()

	invoiceQuery := `
		INSERT INTO invoices (url)
		VALUES ($1)
		ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		RETURNING id
	`

	var invoiceID uuid.UUID
	if err := dbTx.QueryRowContext(ctx, invoiceQuery, invoiceURL).Scan(&invoiceID); err != nil {
		return fmt.Errorf("upserting invoice: %w", err)
	}

	txQuery := `
		UPDATE transactions
		SET invoice_id = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`
	if _, err := dbTx.ExecContext(ctx, txQuery, invoiceID, txID); err != nil {
		return fmt.Errorf("linking invoice to transaction: %w", err)
	}

	if err := dbTx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
