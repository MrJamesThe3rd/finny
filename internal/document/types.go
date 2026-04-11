package document

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BackendConfig is a user-configured storage backend record from the DB.
type BackendConfig struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Type      string          // "paperless", "local", …
	Name      string          // user-friendly label
	Config    json.RawMessage // backend-specific JSON (base_url, token, …)
	Enabled   bool
	CreatedAt time.Time
}

// Document is a logical document (metadata only; content lives on backends).
type Document struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Filename  string
	MIMEType  string
	CreatedAt time.Time
}

// Location records where one copy of a document is stored on a specific backend.
type Location struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	BackendID  uuid.UUID
	Key        string // backend-specific identifier (e.g. "42" for Paperless)
}
