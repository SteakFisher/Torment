package ledger

import (
	"errors"
	"fmt"
)

// Code is a kind of failure, named as the API reports it, such as "invalid_request". Service errors carry
// one; match it with errors.Is and read it with errors.As. A Code with a parent also matches the parent.
type Code struct {
	name   string
	parent *Code
}

func (c *Code) Error() string { return c.name }

// Unwrap returns c's parent, so errors.Is(ErrAccountNotFound, ErrNotFound) is true.
func (c *Code) Unwrap() error {
	if c.parent == nil {
		return nil
	}
	return c.parent
}

// Name returns the code as the API reports it.
func (c *Code) Name() string { return c.name }

// codeError is a failure of code whose text is the message alone, without the code.
type codeError struct {
	code *Code
	msg  string
}

func (e *codeError) Error() string { return e.msg }
func (e *codeError) Unwrap() error { return e.code }

// Errorf returns an error of code with a message formatted as by fmt.Sprintf. The error's text is the
// message alone.
func Errorf(code *Code, format string, args ...any) error {
	return &codeError{code: code, msg: fmt.Sprintf(format, args...)}
}

// CodeOf returns the code err carries, or nil when it carries none.
func CodeOf(err error) *Code {
	var c *Code
	if errors.As(err, &c) {
		return c
	}
	return nil
}

var (
	// ErrInvalid means the input failed validation.
	ErrInvalid = &Code{name: "invalid_request"}
	// ErrPatternTooBroad means an address pattern matches more wallets than allowed. It matches ErrInvalid.
	ErrPatternTooBroad = &Code{name: "pattern_too_broad", parent: ErrInvalid}
	// ErrBatchTooLarge means a request holds more than the TigerBeetle cluster accepts in one batch. It
	// matches ErrInvalid.
	ErrBatchTooLarge = &Code{name: "batch_too_large", parent: ErrInvalid}
	// ErrNotFound means the requested data does not exist.
	ErrNotFound = &Code{name: "not_found"}
	// ErrAccountNotFound means no account has the address. It matches ErrNotFound.
	ErrAccountNotFound = &Code{name: "account_not_found", parent: ErrNotFound}
	// ErrCurrencyNotFound means the currency is not registered. It matches ErrNotFound.
	ErrCurrencyNotFound = &Code{name: "currency_not_found", parent: ErrNotFound}
	// ErrKindNotFound means the transfer kind is not registered. It matches ErrNotFound.
	ErrKindNotFound = &Code{name: "kind_not_found", parent: ErrNotFound}
	// ErrConflict means the input clashes with data that already exists.
	ErrConflict = &Code{name: "conflict"}
	// ErrInsufficientFunds means a posting would take an account below zero when the account does not
	// allow it.
	ErrInsufficientFunds = &Code{name: "insufficient_funds"}
	// ErrIDAlreadyFailed means the id belongs to a transfer that failed; a new id is needed.
	ErrIDAlreadyFailed = &Code{name: "id_already_failed"}
	// ErrChainFailed means another posting in the same chain failed.
	ErrChainFailed = &Code{name: "chain_failed"}
	// ErrOverflow means a posting would push an account's total debits or credits past what 128 bits hold.
	ErrOverflow = &Code{name: "overflow"}
)
