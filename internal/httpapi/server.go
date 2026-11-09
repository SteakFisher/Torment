package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"torment/internal/ledger"
)

// Ledger is the set of ledger operations the HTTP handlers call.
type Ledger interface {
	RegisterCurrency(ctx context.Context, code string) (ledger.Currency, bool, error)
	Currencies(ctx context.Context) ([]ledger.Currency, error)
	RegisterKind(ctx context.Context, name string) (ledger.Kind, bool, error)
	Kinds(ctx context.Context) ([]ledger.Kind, error)
	RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
	Account(ctx context.Context, address string) (ledger.Account, error)
	Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	GroupTransfers(ctx context.Context, p ledger.GroupTransfersParams) ([]ledger.TransferRecord, bool, error)
	ListAccounts(ctx context.Context, pattern, cursor string, limit int) ([]ledger.Account, bool, error)
	Balances(ctx context.Context, address, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
	AccountTransfers(ctx context.Context, p ledger.AccountTransfersParams) ([]ledger.TransferRecord, bool, error)
	Ready(ctx context.Context) error
}

// Fails to compile when *ledger.Service no longer satisfies Ledger.
var _ Ledger = (*ledger.Service)(nil)

// New returns the HTTP handler. GET /healthz and GET /readyz are open; every other route requires one
// of apiKeys. Every request is logged by logRequests.
func New(l Ledger, apiKeys []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /readyz", withTimeout(requestTimeout, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := l.Ready(r.Context()); err != nil {
			slog.Error("not ready", "request_id", requestID(r.Context()), "err", err)
			writeErrorBody(w, http.StatusServiceUnavailable, "not_ready", "not ready")
			return
		}
		w.Write([]byte("ok\n"))
	})))
	mux.Handle("/", requireAPIKey(apiKeys, withTimeout(requestTimeout, routes(l))))
	return logRequests(mux)
}

// requestTimeout is how long a request may wait on Postgres and TigerBeetle. It is shorter than the
// server's 30s WriteTimeout, so the client gets an error response before the connection is closed.
const requestTimeout = 10 * time.Second

// withTimeout gives every request to h a context that ends after d.
func withTimeout(d time.Duration, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// routes returns the API routes without authentication.
func routes(l Ledger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /currencies", postCurrency(l))
	mux.HandleFunc("GET /currencies", getCurrencies(l))
	mux.HandleFunc("POST /kinds", postKind(l))
	mux.HandleFunc("GET /kinds", getKinds(l))
	mux.HandleFunc("POST /accounts", postAccount(l))
	mux.HandleFunc("GET /accounts", getAccounts(l))
	mux.HandleFunc("GET /accounts/{address}", getAccounts(l))
	mux.HandleFunc("GET /accounts/{address}/balances", getBalances(l))
	mux.HandleFunc("GET /accounts/{address}/transfers", getAccountTransfers(l))
	mux.HandleFunc("POST /transfers", postTransfer(l))
	mux.HandleFunc("GET /groups/{group_id}/transfers", getGroupTransfers(l))
	return jsonMuxErrors(mux)
}

// jsonMuxErrors serves requests with mux, answering a request that matches no route with 404
// route_not_found, and one whose path matches only under other methods with 405 method_not_allowed and
// the Allow header.
func jsonMuxErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// h is mux's plain-text 404 or 405 handler; run it for its status and headers only.
		rec := &statusRecorder{ResponseWriter: w}
		h.ServeHTTP(rec, r)
		if rec.status == http.StatusMethodNotAllowed {
			writeErrorBody(w, rec.status, "method_not_allowed", fmt.Sprintf("method %s is not allowed on %s", r.Method, r.URL.Path))
			return
		}
		writeErrorBody(w, http.StatusNotFound, "route_not_found", fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
	})
}

// statusRecorder records the status written to it and discards the body; headers go to the wrapped
// ResponseWriter.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int)        { s.status = code }
func (s *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }

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
