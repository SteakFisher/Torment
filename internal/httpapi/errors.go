package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"torment/internal/ledger"
)

// writeError responds with a status and code chosen from err, most specific first:
// ledger.ErrPatternTooBroad → 400 pattern_too_broad, decodeError and ledger.ErrInvalid → 400
// invalid_request, ledger.ErrCurrencyNotFound → 404 currency_not_found, ledger.ErrNotFound → 404
// account_not_found, ledger.ErrConflict → 409 conflict, anything else → 500 internal with a generic
// message, logging the real error.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := http.StatusInternalServerError, "internal", "internal error"
	var decodeErr decodeError
	switch {
	case errors.Is(err, ledger.ErrPatternTooBroad):
		status, code = http.StatusBadRequest, "pattern_too_broad"
	case errors.As(err, &decodeErr), errors.Is(err, ledger.ErrInvalid):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, ledger.ErrCurrencyNotFound):
		status, code = http.StatusNotFound, "currency_not_found"
	case errors.Is(err, ledger.ErrNotFound):
		status, code = http.StatusNotFound, "account_not_found"
	case errors.Is(err, ledger.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	default:
		slog.Error("request failed", "request_id", requestID(r.Context()), "err", err)
	}
	if status != http.StatusInternalServerError {
		msg = err.Error()
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
