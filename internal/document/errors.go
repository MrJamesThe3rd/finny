package document

import "errors"

var (
	// ErrNoBackends is returned when a user has no enabled backends configured.
	ErrNoBackends = errors.New("no enabled backends configured")

	// ErrDocumentNotFound is returned when a document ID does not exist or does
	// not belong to the requesting user.
	ErrDocumentNotFound = errors.New("document not found")

	// ErrNoAvailableLocation is returned when all known locations for a document
	// fail to download (backend unreachable, key not found, etc.).
	ErrNoAvailableLocation = errors.New("no available location for document")

	// ErrURLNotSupported is returned when the given URL cannot be matched to a known backend.
	ErrURLNotSupported = errors.New("URL does not match any supported backend format")

	// ErrBackendNotFound is returned when a backend ID does not exist or does not
	// belong to the requesting user.
	ErrBackendNotFound = errors.New("backend not found")
)
