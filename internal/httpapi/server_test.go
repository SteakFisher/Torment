package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"torment/internal/ledger"
)

// fakeLedger implements Ledger with per-test funcs; calling a method whose func is nil panics.
type fakeLedger struct {
	registerCurrency func(ctx context.Context, code string) (ledger.Currency, error)
	registerAccount  func(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
	balances         func(ctx context.Context, address string, at time.Time) ([]ledger.Balance, error)
	transfer         func(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	groupTransfers   func(ctx context.Context, groupID string) ([]ledger.GroupTransfer, error)
	listAccounts     func(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error)
	patternBalances  func(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
}

var _ Ledger = fakeLedger{}

func (f fakeLedger) RegisterCurrency(ctx context.Context, code string) (ledger.Currency, error) {
	return f.registerCurrency(ctx, code)
}

func (f fakeLedger) RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error) {
	return f.registerAccount(ctx, p)
}

func (f fakeLedger) Balances(ctx context.Context, address string, at time.Time) ([]ledger.Balance, error) {
	return f.balances(ctx, address, at)
}

func (f fakeLedger) Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error) {
	return f.transfer(ctx, chains)
}

func (f fakeLedger) GroupTransfers(ctx context.Context, groupID string) ([]ledger.GroupTransfer, error) {
	return f.groupTransfers(ctx, groupID)
}

func (f fakeLedger) ListAccounts(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error) {
	return f.listAccounts(ctx, pattern, after, limit)
}

func (f fakeLedger) PatternBalances(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error) {
	return f.patternBalances(ctx, pattern, currency, at)
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
