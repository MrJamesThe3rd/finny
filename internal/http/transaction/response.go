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
	InvoiceID      *uuid.UUID         `json:"invoice_id,omitempty"`
	Invoice        *invoiceResponse   `json:"invoice,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      *time.Time         `json:"updated_at,omitempty"`
}

type invoiceResponse struct {
	ID  uuid.UUID `json:"id"`
	URL string    `json:"url"`
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
		InvoiceID:      tx.InvoiceID,
		CreatedAt:      tx.CreatedAt,
		UpdatedAt:      tx.UpdatedAt,
	}

	if tx.Invoice != nil {
		resp.Invoice = &invoiceResponse{
			ID:  tx.Invoice.ID,
			URL: tx.Invoice.URL,
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
