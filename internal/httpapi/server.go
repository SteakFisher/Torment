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
	RegisterCurrency(ctx context.Context, code string) (ledger.Currency, error)
	RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
	Balances(ctx context.Context, address string, at time.Time) ([]ledger.Balance, error)
	Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	GroupTransfers(ctx context.Context, groupID string) ([]ledger.GroupTransfer, error)
	ListAccounts(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error)
	PatternBalances(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
}

// Fails to compile when *ledger.Service no longer satisfies Ledger.
var _ Ledger = (*ledger.Service)(nil)

// New returns the HTTP handler with every route registered.
func New(l Ledger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /currencies", postCurrency(l))
	mux.HandleFunc("POST /accounts", postAccount(l))
	mux.HandleFunc("GET /accounts", getAccounts(l))
	mux.HandleFunc("GET /accounts/{address}/balances", getBalances(l))
	mux.HandleFunc("GET /balances", getPatternBalances(l))
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
