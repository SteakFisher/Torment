package ledger

import "errors"

// Service errors are wrapped with detail; match them with errors.Is.
var (
	// ErrInvalid means the input failed validation.
	ErrInvalid = errors.New("invalid")
	// ErrConflict means the input clashes with data that already exists.
	ErrConflict = errors.New("conflict")
)
