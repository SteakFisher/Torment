package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"torment/internal/ledger"
)

// Ledger is the set of ledger operations the HTTP handlers call.
type Ledger interface {
	RegisterCurrency(ctx context.Context, code string) (ledger.Currency, bool, error)
	Currencies(ctx context.Context) ([]ledger.Currency, error)
	RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
	Account(ctx context.Context, address string) (ledger.Account, error)
	Balances(ctx context.Context, address, currency string, at time.Time) ([]ledger.Balance, error)
	Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	GroupTransfers(ctx context.Context, groupID string) ([]ledger.TransferRecord, error)
	ListAccounts(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error)
	PatternBalances(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
	AccountTransfers(ctx context.Context, p ledger.AccountTransfersParams) ([]ledger.TransferRecord, bool, error)
}

// Fails to compile when *ledger.Service no longer satisfies Ledger.
var _ Ledger = (*ledger.Service)(nil)

// New returns the HTTP handler. GET /healthz is open; every other route requires one of apiKeys.
func New(l Ledger, apiKeys []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.Handle("/", requireAPIKey(apiKeys, withTimeout(requestTimeout, routes(l))))
	return mux
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
	mux.HandleFunc("POST /accounts", postAccount(l))
	mux.HandleFunc("GET /accounts", getAccounts(l))
	mux.HandleFunc("GET /accounts/{address}", getAccounts(l))
	mux.HandleFunc("GET /accounts/{address}/balances", getBalances(l))
	mux.HandleFunc("GET /accounts/{address}/transfers", getAccountTransfers(l))
	mux.HandleFunc("POST /transfers", postTransfer(l))
	mux.HandleFunc("GET /groups/{group_id}/transfers", getGroupTransfers(l))
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
