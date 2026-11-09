package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"torment/internal/ledger"
)

// writeError responds with {"error": message} and a status chosen from err:
// decodeError and ledger.ErrInvalid → 400, ledger.ErrNotFound → 404, ledger.ErrConflict → 409,
// anything else → 500 with a generic message, logging the real error.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := http.StatusInternalServerError, "internal error"
	var decodeErr decodeError
	switch {
	case errors.As(err, &decodeErr), errors.Is(err, ledger.ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, ledger.ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, ledger.ErrConflict):
		status, msg = http.StatusConflict, err.Error()
	default:
		slog.Error("request failed", "request_id", requestID(r.Context()), "err", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
