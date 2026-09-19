package transaction

import (
	"time"

	"github.com/google/uuid"
)

// Type represents the type of transaction (income or expense).
type Type string

const (
	TypeIncome  Type = "income"
	TypeExpense Type = "expense"
)

// Status represents the lifecycle state of a transaction.
type Status string

const (
	StatusDraft          Status = "draft"
	StatusPendingInvoice Status = "pending_invoice"
	StatusComplete       Status = "complete"
	StatusNoInvoice      Status = "no_invoice"
)

// OwnerOnlyStatus reports whether moving a transaction to s is reserved to an
// organization's owner.
//
// no_invoice is the assertion that suppresses a movement from the
// missing-invoice report — the mechanism by which an undocumented expense
// disappears from view — and draft unpublishes a movement the accountant may
// already have worked from. Both are irreversible in practice; pending_invoice
// and complete are not.
//
// This function is deliberately pure and knows nothing about roles or
// organizations: the comparison happens at the HTTP boundary, which holds both
// sides. See knowledge-base.md §4.
func OwnerOnlyStatus(s Status) bool {
	return s == StatusNoInvoice || s == StatusDraft
}

// Transaction represents a financial transaction.
type Transaction struct {
	ID             uuid.UUID
	Amount         int64 // Amount in cents
	Type           Type
	Status         Status
	Description    string
	RawDescription string
	Date           time.Time
	DocumentID     *uuid.UUID
	Document       *Document // Loaded via JOIN; contains metadata only (no download URL)
	CreatedAt      time.Time
	UpdatedAt      *time.Time
	DeletedAt      *time.Time
}

// Document is the document metadata attached to a transaction.
// Content is retrieved via the document service.
type Document struct {
	ID       uuid.UUID
	Filename string
	MIMEType string
}
