package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/org"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

var _ transaction.Repository = (*Store)(nil)

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

// auditFields is the opaque snapshot of a transaction written to audit_log.
func auditFields(tx *transaction.Transaction) map[string]any {
	return map[string]any{
		"amount":      tx.Amount,
		"type":        tx.Type,
		"status":      tx.Status,
		"description": tx.Description,
		"date":        tx.Date,
		"document_id": tx.DocumentID,
	}
}

// lockTransaction reads the current row FOR UPDATE inside tx. Every mutation
// needs the prior value for audit_log.old_value, and without the row lock two
// concurrent writers both record a predecessor that was already gone — which
// corrupts precisely the record audit_log exists to make trustworthy.
func lockTransaction(ctx context.Context, tx *sql.Tx, id, orgID uuid.UUID) (*transaction.Transaction, error) {
	const query = `
		SELECT amount, type, status, description, date, document_id
		FROM transactions
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`

	var (
		out                transaction.Transaction
		typeStr, statusStr string
	)

	err := tx.QueryRowContext(ctx, query, id, orgID).
		Scan(&out.Amount, &typeStr, &statusStr, &out.Description, &out.Date, &out.DocumentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, transaction.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("locking transaction: %w", err)
	}

	out.ID = id
	out.Type = transaction.Type(typeStr)
	out.Status = transaction.Status(statusStr)

	return &out, nil
}

func (s *Store) CreateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		const query = `
			INSERT INTO transactions (amount, type, status, description, raw_description, date, document_id, org_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			RETURNING id, created_at, updated_at
		`

		if err := dbTx.QueryRowContext(ctx, query,
			tx.Amount, tx.Type, tx.Status, tx.Description, tx.RawDescription,
			tx.Date, tx.DocumentID, orgID,
		).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt); err != nil {
			return fmt.Errorf("creating transaction: %w", err)
		}

		// Creation is audited too: POST /transactions is born complete, which
		// is an assertion about invoice requirement made at birth.
		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   tx.ID,
			Action:      audit.ActionCreate,
			NewValue:    auditFields(tx),
		})
	})
}

func (s *Store) GetTransaction(ctx context.Context, id uuid.UUID) (*transaction.Transaction, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.id = $1 AND t.org_id = $2 AND t.deleted_at IS NULL`

	tx, err := scanTransaction(s.db.QueryRowContext(ctx, query, id, orgID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, transaction.ErrNotFound
		}

		return nil, fmt.Errorf("getting transaction: %w", err)
	}

	return tx, nil
}

func (s *Store) ListTransactions(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.deleted_at IS NULL AND t.org_id = $1`

	args := []any{orgID}
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
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		old, err := lockTransaction(ctx, dbTx, tx.ID, orgID)
		if err != nil {
			return err
		}

		const query = `
			UPDATE transactions
			SET amount = $1, type = $2, status = $3, description = $4, updated_at = NOW()
			WHERE id = $5 AND org_id = $6 AND deleted_at IS NULL
		`

		if _, err := dbTx.ExecContext(ctx, query,
			tx.Amount, tx.Type, tx.Status, tx.Description, tx.ID, orgID,
		); err != nil {
			return fmt.Errorf("updating transaction: %w", err)
		}

		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   tx.ID,
			Action:      audit.ActionUpdate,
			OldValue:    auditFields(old),
			NewValue:    auditFields(tx),
		})
	})
}

func (s *Store) UpdateStatus(ctx context.Context, id uuid.UUID, status transaction.Status) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		old, err := lockTransaction(ctx, dbTx, id, orgID)
		if err != nil {
			return err
		}

		const query = `
			UPDATE transactions
			SET status = $1, updated_at = NOW()
			WHERE id = $2 AND org_id = $3 AND deleted_at IS NULL
		`

		if _, err := dbTx.ExecContext(ctx, query, status, id, orgID); err != nil {
			return fmt.Errorf("updating status: %w", err)
		}

		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   id,
			Action:      audit.ActionStatusChange,
			OldValue:    map[string]any{"status": old.Status},
			NewValue:    map[string]any{"status": status},
		})
	})
}

func (s *Store) DeleteTransaction(ctx context.Context, id uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		old, err := lockTransaction(ctx, dbTx, id, orgID)
		if err != nil {
			return err
		}

		const query = `UPDATE transactions SET deleted_at = NOW() WHERE id = $1 AND org_id = $2`

		if _, err := dbTx.ExecContext(ctx, query, id, orgID); err != nil {
			return fmt.Errorf("deleting transaction: %w", err)
		}

		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   id,
			Action:      audit.ActionDelete,
			OldValue:    auditFields(old),
		})
	})
}

// AttachDocument links a document to a transaction and sets its status to complete.
// Returns ErrDocumentAlreadyAttached if the transaction already has a document;
// the locked read makes the check-then-attach atomic.
func (s *Store) AttachDocument(ctx context.Context, txID uuid.UUID, documentID uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		old, err := lockTransaction(ctx, dbTx, txID, orgID)
		if err != nil {
			return err
		}

		if old.DocumentID != nil {
			return transaction.ErrDocumentAlreadyAttached
		}

		const query = `
			UPDATE transactions
			SET document_id = $1, status = 'complete', updated_at = NOW()
			WHERE id = $2 AND org_id = $3 AND deleted_at IS NULL
		`

		if _, err := dbTx.ExecContext(ctx, query, documentID, txID, orgID); err != nil {
			return fmt.Errorf("attaching document: %w", err)
		}

		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   txID,
			Action:      audit.ActionAttach,
			OldValue:    map[string]any{"status": old.Status, "document_id": nil},
			NewValue:    map[string]any{"status": transaction.StatusComplete, "document_id": documentID},
		})
	})
}

// DetachDocument clears the document link from a transaction and resets its status to pending_invoice.
func (s *Store) DetachDocument(ctx context.Context, txID uuid.UUID) error {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return err
	}

	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(dbTx *sql.Tx) error {
		old, err := lockTransaction(ctx, dbTx, txID, orgID)
		if err != nil {
			return err
		}

		const query = `
			UPDATE transactions
			SET document_id = NULL, status = 'pending_invoice', updated_at = NOW()
			WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		`

		if _, err := dbTx.ExecContext(ctx, query, txID, orgID); err != nil {
			return fmt.Errorf("detaching document: %w", err)
		}

		return audit.Record(ctx, dbTx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectTransaction,
			SubjectID:   txID,
			Action:      audit.ActionDetach,
			OldValue:    map[string]any{"status": old.Status, "document_id": old.DocumentID},
			NewValue:    map[string]any{"status": transaction.StatusPendingInvoice, "document_id": nil},
		})
	})
}

// importLockKey is org-scoped: hashing the date range alone made two
// organizations importing overlapping periods serialize on one lock.
func importLockKey(orgID uuid.UUID, minDate, maxDate time.Time) int64 {
	h := fnv.New64a()
	h.Write(orgID[:])
	h.Write([]byte{0})
	h.Write([]byte(minDate.Format(time.DateOnly)))
	h.Write([]byte{0})
	h.Write([]byte(maxDate.Format(time.DateOnly)))

	return int64(h.Sum64())
}

type importTx struct {
	tx      *sql.Tx
	orgID   uuid.UUID
	actorID uuid.UUID
}

func (s *Store) BeginImport(ctx context.Context, minDate, maxDate time.Time) (transaction.ImportTx, error) {
	orgID, err := org.OrgID(ctx)
	if err != nil {
		return nil, err
	}

	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning import tx: %w", err)
	}

	lockKey := importLockKey(orgID, minDate, maxDate)
	if _, err := dbTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		dbTx.Rollback() //nolint:errcheck
		return nil, fmt.Errorf("acquiring import lock: %w", err)
	}

	return &importTx{tx: dbTx, orgID: orgID, actorID: auth.UserID(ctx)}, nil
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
			Date:           p.Date.Format(time.DateOnly),
			Amount:         p.Amount,
			Type:           p.Type,
			RawDescription: p.RawDescription,
		}] = struct{}{}
	}

	query := `SELECT ` + selectTransactionColumns + transactionJoin +
		`WHERE t.deleted_at IS NULL AND t.org_id = $1 AND t.date >= $2 AND t.date <= $3
		ORDER BY t.date ASC`

	rows, err := itx.tx.QueryContext(ctx, query, itx.orgID, minDate, maxDate)
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
			Date:           tx.Date.Format(time.DateOnly),
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
	const query = `
		INSERT INTO transactions (amount, type, status, description, raw_description, date, document_id, org_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	for _, tx := range txs {
		err := itx.tx.QueryRowContext(ctx, query,
			tx.Amount, tx.Type, tx.Status, tx.Description, tx.RawDescription,
			tx.Date, tx.DocumentID, itx.orgID,
		).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
		if err != nil {
			return fmt.Errorf("creating transaction: %w", err)
		}
	}

	if len(txs) == 0 {
		return nil
	}

	// One audit row for the batch, not one per transaction.
	return audit.Record(ctx, itx.tx, audit.Entry{
		OrgID:       itx.orgID,
		ActorUserID: itx.actorID,
		SubjectType: audit.SubjectImportBatch,
		SubjectID:   uuid.New(),
		Action:      audit.ActionCreate,
		NewValue:    map[string]any{"count": len(txs)},
	})
}
