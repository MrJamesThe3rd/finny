package document

import (
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/document"
)

type documentResponse struct {
	ID       uuid.UUID `json:"id"`
	Filename string    `json:"filename"`
	MIMEType string    `json:"mime_type"`
}

type backendResponse struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func toDocumentResponse(doc *document.Document) documentResponse {
	return documentResponse{
		ID:       doc.ID,
		Filename: doc.Filename,
		MIMEType: doc.MIMEType,
	}
}

func toBackendResponse(cfg document.BackendConfig) backendResponse {
	return backendResponse{
		ID:        cfg.ID,
		Type:      cfg.Type,
		Name:      cfg.Name,
		Enabled:   cfg.Enabled,
		CreatedAt: cfg.CreatedAt,
	}
}
