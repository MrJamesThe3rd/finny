package transaction

import "errors"

var (
	ErrNotFound                = errors.New("transaction not found")
	ErrDocumentAlreadyAttached = errors.New("transaction already has a document")
)
