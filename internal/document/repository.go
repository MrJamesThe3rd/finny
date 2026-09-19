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
	// ListEnabledBackends returns only backends with enabled = true. It exists
	// as its own query because ListBackends feeds the settings UI, which must
	// show disabled backends, while the upload path must never write to one.
	ListEnabledBackends(ctx context.Context) ([]BackendConfig, error)
	GetBackend(ctx context.Context, id uuid.UUID) (*BackendConfig, error)
	CreateBackend(ctx context.Context, cfg *BackendConfig) error
	UpdateBackend(ctx context.Context, id uuid.UUID, name *string, config json.RawMessage, enabled *bool) error
	DeleteBackend(ctx context.Context, id uuid.UUID) error
	BackendHasDocuments(ctx context.Context, backendID uuid.UUID) (bool, error)

	// Document operations
	CreateDocument(ctx context.Context, doc *Document) error
	GetDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	// PurgeDocument deletes without writing an audit row. Only the upload
	// rollback may use it: cleaning up a partly-failed multi-backend upload
	// through the audited delete would write a delete row indistinguishable
	// from a user deliberately destroying a document someone relied on.
	PurgeDocument(ctx context.Context, id uuid.UUID) error

	// Location operations
	AddLocation(ctx context.Context, loc *Location) error
	ListLocations(ctx context.Context, documentID uuid.UUID) ([]Location, error)
}
