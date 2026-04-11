package store

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
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

// scanTransaction reads a transaction row and returns a populated Transaction.
// Expected column order: id, amount, type, status, description, raw_description, date,
// document_id, doc_filename, doc_mime_type, created_at, updated_at, deleted_at
func scanTransaction(s scanner) (*transaction.Transaction, error) {
	var tx transaction.Transaction

	var typeStr, statusStr string
	var rawDesc sql.NullString
	var docID *uuid.UUID
	var docFilename, docMIMEType sql.NullString

	if err := s.Scan(
		&tx.ID, &tx.Amount, &typeStr, &statusStr, &tx.Description, &rawDesc, &tx.Date,
		&docID, &docFilename, &docMIMEType,
		&tx.CreatedAt, &tx.UpdatedAt, &tx.DeletedAt,
	); err != nil {
		return nil, err
	}

	tx.Type = transaction.Type(typeStr)
	tx.Status = transaction.Status(statusStr)
	tx.RawDescription = rawDesc.String
	tx.DocumentID = docID

	if docID != nil && docFilename.Valid {
		tx.Document = &transaction.Document{
			ID:       *docID,
			Filename: docFilename.String,
			MIMEType: docMIMEType.String,
		}
	}

	return &tx, nil
}

const selectTransactionColumns = `
	t.id, t.amount, t.type, t.status, t.description, t.raw_description, t.date,
	t.document_id, d.filename AS doc_filename, d.mime_type AS doc_mime_type,
	t.created_at, t.updated_at, t.deleted_at
`

const transactionJoin = `
	FROM transactions t
	LEFT JOIN documents d ON t.document_id = d.id
`

func (s *Store) CreateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	query := `
		INSERT INTO transactions (amount, type, status, description, raw_description, date, document_id, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(ctx, query,
		tx.Amount, tx.Type, tx.Status, tx.Description, tx.RawDescription,
		tx.Date, tx.DocumentID, auth.UserID(ctx),
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}

	return nil
}

func (s *Store) GetTransaction(ctx context.Context, id uuid.UUID) (*transaction.Transaction, error) {
	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.id = $1 AND t.user_id = $2 AND t.deleted_at IS NULL`

	tx, err := scanTransaction(s.db.QueryRowContext(ctx, query, id, auth.UserID(ctx)))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, transaction.ErrNotFound
		}

		return nil, fmt.Errorf("getting transaction: %w", err)
	}

	return tx, nil
}

func (s *Store) ListTransactions(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error) {
	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.deleted_at IS NULL AND t.user_id = $1`

	args := []any{auth.UserID(ctx)}
	argIdx := 2

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

	return txs, rows.Err()
}

func (s *Store) UpdateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	query := `
		UPDATE transactions
		SET amount = $1, type = $2, status = $3, description = $4, updated_at = NOW()
		WHERE id = $5 AND user_id = $6 AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query,
		tx.Amount, tx.Type, tx.Status, tx.Description,
		tx.ID, auth.UserID(ctx),
	)
	if err != nil {
		return fmt.Errorf("updating transaction: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return transaction.ErrNotFound
	}

	return nil
}

func (s *Store) UpdateStatus(ctx context.Context, id uuid.UUID, status transaction.Status) error {
	query := `
		UPDATE transactions
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, status, id, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("updating status: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return transaction.ErrNotFound
	}

	return nil
}

func (s *Store) DeleteTransaction(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE transactions
		SET deleted_at = NOW()
		WHERE id = $1 AND user_id = $2
	`

	result, err := s.db.ExecContext(ctx, query, id, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("deleting transaction: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return transaction.ErrNotFound
	}

	return nil
}

// AttachDocument links a document to a transaction and sets its status to complete.
// Returns ErrDocumentAlreadyAttached if the transaction already has a document,
// ensuring the check-then-attach is atomic at the DB level.
func (s *Store) AttachDocument(ctx context.Context, txID uuid.UUID, documentID uuid.UUID) error {
	query := `
		UPDATE transactions
		SET document_id = $1, status = 'complete', updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL AND document_id IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, documentID, txID, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("attaching document: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		// Distinguish "already has document" from "not found".
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM transactions WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`
		if err := s.db.QueryRowContext(ctx, checkQuery, txID, auth.UserID(ctx)).Scan(&exists); err != nil {
			return fmt.Errorf("checking transaction: %w", err)
		}
		if !exists {
			return transaction.ErrNotFound
		}
		return transaction.ErrDocumentAlreadyAttached
	}

	return nil
}

// DetachDocument clears the document link from a transaction and resets its status to pending_invoice.
func (s *Store) DetachDocument(ctx context.Context, txID uuid.UUID) error {
	query := `
		UPDATE transactions
		SET document_id = NULL, status = 'pending_invoice', updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, txID, auth.UserID(ctx))
	if err != nil {
		return fmt.Errorf("detaching document: %w", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		return transaction.ErrNotFound
	}

	return nil
}

func importLockKey(minDate, maxDate time.Time) int64 {
	h := fnv.New64a()
	h.Write([]byte(minDate.Format("2006-01-02")))
	h.Write([]byte{0})
	h.Write([]byte(maxDate.Format("2006-01-02")))

	return int64(h.Sum64())
}

type importTx struct {
	tx *sql.Tx
}

func (s *Store) BeginImport(ctx context.Context, minDate, maxDate time.Time) (transaction.ImportTx, error) {
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning import tx: %w", err)
	}

	lockKey := importLockKey(minDate, maxDate)
	if _, err := dbTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		dbTx.Rollback()
		return nil, fmt.Errorf("acquiring import lock: %w", err)
	}

	return &importTx{tx: dbTx}, nil
}

func (itx *importTx) Commit() error   { return itx.tx.Commit() }
func (itx *importTx) Rollback() error { return itx.tx.Rollback() }

func (itx *importTx) FindDuplicates(ctx context.Context, params []transaction.CreateParams) ([]*transaction.Transaction, error) {
	if len(params) == 0 {
		return nil, nil
	}

	type lookupKey struct {
		Date           string
		Amount         int64
		Type           transaction.Type
		RawDescription string
	}

	minDate := params[0].Date
	maxDate := params[0].Date
	keySet := make(map[lookupKey]struct{}, len(params))

	for _, p := range params {
		if p.Date.Before(minDate) {
			minDate = p.Date
		}

		if p.Date.After(maxDate) {
			maxDate = p.Date
		}

		keySet[lookupKey{
			Date:           p.Date.Format("2006-01-02"),
			Amount:         p.Amount,
			Type:           p.Type,
			RawDescription: p.RawDescription,
		}] = struct{}{}
	}

	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.deleted_at IS NULL AND t.user_id = $1 AND t.date >= $2 AND t.date <= $3
		ORDER BY t.date ASC`

	rows, err := itx.tx.QueryContext(ctx, query, auth.UserID(ctx), minDate, maxDate)
	if err != nil {
		return nil, fmt.Errorf("finding duplicates: %w", err)
	}
	defer rows.Close()

	var duplicates []*transaction.Transaction

	for rows.Next() {
		tx, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}

		k := lookupKey{
			Date:           tx.Date.Format("2006-01-02"),
			Amount:         tx.Amount,
			Type:           tx.Type,
			RawDescription: tx.RawDescription,
		}

		if _, found := keySet[k]; found {
			duplicates = append(duplicates, tx)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating duplicate rows: %w", err)
	}

	return duplicates, nil
}

func (itx *importTx) CreateTransactions(ctx context.Context, txs []*transaction.Transaction) error {
	query := `
		INSERT INTO transactions (amount, type, status, description, raw_description, date, document_id, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	userID := auth.UserID(ctx)

	for _, tx := range txs {
		err := itx.tx.QueryRowContext(ctx, query,
			tx.Amount, tx.Type, tx.Status, tx.Description, tx.RawDescription,
			tx.Date, tx.DocumentID, userID,
		).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
		if err != nil {
			return fmt.Errorf("creating transaction: %w", err)
		}
	}

	return nil
}
