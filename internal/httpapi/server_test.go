package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"torment/internal/ledger"
)

// fakeLedger implements Ledger with per-test funcs; calling a method whose func is nil panics.
type fakeLedger struct {
	registerCurrency func(ctx context.Context, code string) (ledger.Currency, error)
	registerAccount  func(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
}

var _ Ledger = fakeLedger{}

func (f fakeLedger) RegisterCurrency(ctx context.Context, code string) (ledger.Currency, error) {
	return f.registerCurrency(ctx, code)
}

func (f fakeLedger) RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error) {
	return f.registerAccount(ctx, p)
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
