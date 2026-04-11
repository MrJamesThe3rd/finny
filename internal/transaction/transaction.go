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
