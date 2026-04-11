package document

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// Repository is the storage interface for the document service.
type Repository interface {
	// Backend operations
	ListBackends(ctx context.Context) ([]BackendConfig, error)
	GetBackend(ctx context.Context, id uuid.UUID) (*BackendConfig, error)
	SetBackendConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) error
	CreateBackend(ctx context.Context, cfg *BackendConfig) error
	UpdateBackend(ctx context.Context, id uuid.UUID, name *string, config json.RawMessage, enabled *bool) error
	DeleteBackend(ctx context.Context, id uuid.UUID) error
	BackendHasDocuments(ctx context.Context, backendID uuid.UUID) (bool, error)

	// Document operations
	CreateDocument(ctx context.Context, doc *Document) error
	GetDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	DeleteDocument(ctx context.Context, id uuid.UUID) error

	// Location operations
	AddLocation(ctx context.Context, loc *Location) error
	ListLocations(ctx context.Context, documentID uuid.UUID) ([]Location, error)
}
