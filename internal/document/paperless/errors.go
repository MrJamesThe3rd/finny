package paperless

import "errors"

var (
	// ErrBaseURLRequired is returned when a config omits base_url.
	ErrBaseURLRequired = errors.New("base_url is required")

	// ErrBaseURLInvalid is returned when base_url is not a well-formed http or
	// https URL with a host.
	ErrBaseURLInvalid = errors.New("base_url must be a valid http or https URL with a host")

	// ErrNonPublicAddress is returned when a connection would reach an address
	// that is not publicly routable. It deliberately carries no address: it is
	// wrapped into errors that reach logs and API responses.
	ErrNonPublicAddress = errors.New("refusing to connect to non-public address")

	// ErrAddressUnparseable is returned when a dial target cannot be parsed,
	// which is treated as a denial rather than allowed through.
	ErrAddressUnparseable = errors.New("refusing to connect to unparseable address")

	// ErrRequestFailed replaces the underlying transport error, which carries the
	// configured host in its text. See requestError.
	ErrRequestFailed = errors.New("request to paperless failed")

	// ErrInvalidDocumentKey is returned when a storage key is not a bare
	// Paperless document ID.
	ErrInvalidDocumentKey = errors.New("invalid paperless document key")

	// ErrNoDocumentID is returned when consumption succeeded but reported no
	// usable document ID.
	ErrNoDocumentID = errors.New("paperless task succeeded but returned no document ID")

	// ErrDeleteNotSupported is returned by Delete; deletions are managed in the
	// Paperless UI.
	ErrDeleteNotSupported = errors.New("paperless delete not supported")
)
