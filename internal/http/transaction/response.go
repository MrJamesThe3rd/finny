package transaction

import (
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type transactionResponse struct {
	ID             uuid.UUID          `json:"id"`
	Amount         int64              `json:"amount"`
	Type           transaction.Type   `json:"type"`
	Status         transaction.Status `json:"status"`
	Description    string             `json:"description"`
	RawDescription string             `json:"raw_description,omitempty"`
	Date           time.Time          `json:"date"`
	DocumentID     *uuid.UUID         `json:"document_id,omitempty"`
	Document       *documentResponse  `json:"document,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      *time.Time         `json:"updated_at,omitempty"`
}

type documentResponse struct {
	ID       uuid.UUID `json:"id"`
	Filename string    `json:"filename"`
	MIMEType string    `json:"mime_type"`
}

func toResponse(tx *transaction.Transaction) transactionResponse {
	resp := transactionResponse{
		ID:             tx.ID,
		Amount:         tx.Amount,
		Type:           tx.Type,
		Status:         tx.Status,
		Description:    tx.Description,
		RawDescription: tx.RawDescription,
		Date:           tx.Date,
		DocumentID:     tx.DocumentID,
		CreatedAt:      tx.CreatedAt,
		UpdatedAt:      tx.UpdatedAt,
	}

	if tx.Document != nil {
		resp.Document = &documentResponse{
			ID:       tx.Document.ID,
			Filename: tx.Document.Filename,
			MIMEType: tx.Document.MIMEType,
		}
	}

	return resp
}

func toResponseList(txs []*transaction.Transaction) []transactionResponse {
	resp := make([]transactionResponse, len(txs))
	for i, tx := range txs {
		resp[i] = toResponse(tx)
	}

	return resp
}
