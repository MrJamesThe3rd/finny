package local

import "errors"

var (
	// ErrRootNotConfigured is returned when no storage root has been set, which
	// would leave base_path unconfined.
	ErrRootNotConfigured = errors.New("storage root is not configured")

	// ErrBasePathRequired is returned when a config omits base_path.
	ErrBasePathRequired = errors.New("base_path is required")

	// ErrBasePathNotRelative is returned when base_path is absolute; it is
	// always interpreted relative to the storage root.
	ErrBasePathNotRelative = errors.New("base_path must be relative to the storage root")

	// ErrBasePathEscapesRoot is returned when base_path resolves outside the
	// storage root.
	ErrBasePathEscapesRoot = errors.New("base_path escapes the storage root")

	// ErrFileNotFound is returned when a key has no file beneath base_path.
	ErrFileNotFound = errors.New("file not found")
)
