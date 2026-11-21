package httpapi

import (
	"log/slog"
	"net/http"

	"torment/internal/ledger"
)

// statuses maps the codes a request can fail with to their HTTP status. A code not listed is a bug and
// responds 500.
var statuses = map[*ledger.Code]int{
	ledger.ErrInvalid:          http.StatusBadRequest,
	ledger.ErrBatchTooLarge:    http.StatusBadRequest,
	ledger.ErrPatternTooBroad:  http.StatusBadRequest,
	ledger.ErrNotFound:         http.StatusNotFound,
	ledger.ErrAccountNotFound:  http.StatusNotFound,
	ledger.ErrCurrencyNotFound: http.StatusNotFound,
	ledger.ErrKindNotFound:     http.StatusNotFound,
	ledger.ErrConflict:         http.StatusConflict,
}

// writeError responds with the status of err's code from statuses and {"error": {"code", "message"}},
// where the message is err's text. An error with no listed code is logged and responds 500 internal
// with a generic message.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := ledger.CodeOf(err)
	status, ok := statuses[code]
	if !ok {
		slog.Error("request failed", "request_id", requestID(r.Context()), "err", err)
		writeErrorBody(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeErrorBody(w, status, code.Name(), err.Error())
}

// writeErrorBody responds with status and {"error": {"code": code, "message": msg}}.
func writeErrorBody(w http.ResponseWriter, status int, code, msg string) {
	type body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	writeJSON(w, status, map[string]body{"error": {code, msg}})
}
