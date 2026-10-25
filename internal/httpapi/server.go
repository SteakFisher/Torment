package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"torment/internal/ledger"
)

// Ledger is the set of ledger operations the HTTP handlers call.
type Ledger interface {
	RegisterAsset(ctx context.Context, code string, ledger int32) (ledger.Asset, error)
}

// Fails to compile when *ledger.Service no longer satisfies Ledger.
var _ Ledger = (*ledger.Service)(nil)

// New returns the HTTP handler with every route registered.
func New(l Ledger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /assets", postAsset(l))
	return mux
}

// decodeError marks a request body that is not valid JSON for the target type.
type decodeError struct{ err error }

func (e decodeError) Error() string { return e.err.Error() }

// decodeJSON reads a body of at most 1 MiB into v, rejecting unknown fields. Failures are returned
// as decodeError.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return decodeError{fmt.Errorf("error decoding request body: %w", err)}
	}
	return nil
}

// writeJSON responds with status and v encoded as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
