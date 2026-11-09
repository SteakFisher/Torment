package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"torment/internal/ledger"
)

// writeError responds with a status and code chosen from err, most specific first:
// ledger.ErrBatchTooLarge → 400 batch_too_large, ledger.ErrPatternTooBroad → 400 pattern_too_broad, decodeError and ledger.ErrInvalid → 400
// invalid_request, ledger.ErrCurrencyNotFound → 404 currency_not_found, ledger.ErrKindNotFound → 404
// kind_not_found, ledger.ErrNotFound → 404 account_not_found, ledger.ErrConflict → 409 conflict, anything else → 500 internal with a generic
// message, logging the real error. The message is err's text without the "<sentinel>: " it carries.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := http.StatusInternalServerError, "internal", "internal error"
	var sentinel error
	var decodeErr decodeError
	switch {
	case errors.As(err, &decodeErr):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, ledger.ErrBatchTooLarge):
		status, code, sentinel = http.StatusBadRequest, "batch_too_large", ledger.ErrInvalid
	case errors.Is(err, ledger.ErrPatternTooBroad):
		status, code, sentinel = http.StatusBadRequest, "pattern_too_broad", ledger.ErrInvalid
	case errors.Is(err, ledger.ErrInvalid):
		status, code, sentinel = http.StatusBadRequest, "invalid_request", ledger.ErrInvalid
	case errors.Is(err, ledger.ErrCurrencyNotFound):
		status, code, sentinel = http.StatusNotFound, "currency_not_found", ledger.ErrNotFound
	case errors.Is(err, ledger.ErrKindNotFound):
		status, code, sentinel = http.StatusNotFound, "kind_not_found", ledger.ErrNotFound
	case errors.Is(err, ledger.ErrNotFound):
		status, code, sentinel = http.StatusNotFound, "account_not_found", ledger.ErrNotFound
	case errors.Is(err, ledger.ErrConflict):
		status, code, sentinel = http.StatusConflict, "conflict", ledger.ErrConflict
	default:
		slog.Error("request failed", "request_id", requestID(r.Context()), "err", err)
	}
	if status != http.StatusInternalServerError {
		msg = err.Error()
	}
	if sentinel != nil {
		// The sentinel's text precedes any user input in the message, so the first match is the sentinel.
		msg = strings.Replace(msg, sentinel.Error()+": ", "", 1)
	}
	writeErrorBody(w, status, code, msg)
}

// writeErrorBody responds with status and {"error": {"code": code, "message": msg}}.
func writeErrorBody(w http.ResponseWriter, status int, code, msg string) {
	type body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	writeJSON(w, status, map[string]body{"error": {code, msg}})
}
