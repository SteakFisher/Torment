package ledger

import "errors"

// Service errors are wrapped with detail; match them with errors.Is.
var (
	// ErrInvalid means the input failed validation.
	ErrInvalid = errors.New("invalid")
	// ErrNotFound means the requested data does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the input clashes with data that already exists.
	ErrConflict = errors.New("conflict")
	// ErrInsufficientFunds means a posting would take an account below zero when the account does not
	// allow it.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrIDAlreadyFailed means the id belongs to a transfer that failed; a new id is needed.
	ErrIDAlreadyFailed = errors.New("id already failed")
	// ErrChainFailed means another posting in the same chain failed.
	ErrChainFailed = errors.New("chain failed")
	// ErrOverflow means a posting would push an account's total debits or credits past what 128 bits hold.
	ErrOverflow = errors.New("overflow")
)
