package document

import (
	"context"
	"io"
)

// Backend abstracts a single document storage service (Paperless, Google Drive, local FS, etc.).
// Each implementation is registered in the Registry and instantiated from JSONB config stored in the DB.
type Backend interface {
	// Type returns the backend type identifier (e.g. "paperless", "local").
	Type() string

	// Upload stores a document and returns the backend-specific key used to retrieve it later.
	Upload(ctx context.Context, filename string, content io.Reader) (key string, err error)

	// Download retrieves a document by its backend-specific key. Caller must close the reader.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes a document by its backend-specific key.
	Delete(ctx context.Context, key string) error
}
