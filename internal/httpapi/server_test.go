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
	registerCurrency func(ctx context.Context, code string) (ledger.Currency, bool, error)
	currencies       func(ctx context.Context) ([]ledger.Currency, error)
	registerAccount  func(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error)
	account          func(ctx context.Context, address string) (ledger.Account, error)
	balances         func(ctx context.Context, address, currency string, at time.Time) ([]ledger.Balance, error)
	transfer         func(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	groupTransfers   func(ctx context.Context, groupID string) ([]ledger.TransferRecord, error)
	listAccounts     func(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error)
	patternBalances  func(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
	accountTransfers func(ctx context.Context, p ledger.AccountTransfersParams) ([]ledger.TransferRecord, bool, error)
	ready            func(ctx context.Context) error
}

var _ Ledger = fakeLedger{}

func (f fakeLedger) RegisterCurrency(ctx context.Context, code string) (ledger.Currency, bool, error) {
	return f.registerCurrency(ctx, code)
}

func (f fakeLedger) Currencies(ctx context.Context) ([]ledger.Currency, error) {
	return f.currencies(ctx)
}

func (f fakeLedger) RegisterAccount(ctx context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error) {
	return f.registerAccount(ctx, p)
}

func (f fakeLedger) Account(ctx context.Context, address string) (ledger.Account, error) {
	return f.account(ctx, address)
}

func (f fakeLedger) Balances(ctx context.Context, address, currency string, at time.Time) ([]ledger.Balance, error) {
	return f.balances(ctx, address, currency, at)
}

func (f fakeLedger) Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error) {
	return f.transfer(ctx, chains)
}

func (f fakeLedger) GroupTransfers(ctx context.Context, groupID string) ([]ledger.TransferRecord, error) {
	return f.groupTransfers(ctx, groupID)
}

func (f fakeLedger) ListAccounts(ctx context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error) {
	return f.listAccounts(ctx, pattern, after, limit)
}

func (f fakeLedger) PatternBalances(ctx context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error) {
	return f.patternBalances(ctx, pattern, currency, at)
}

func (f fakeLedger) AccountTransfers(ctx context.Context, p ledger.AccountTransfersParams) ([]ledger.TransferRecord, bool, error) {
	return f.accountTransfers(ctx, p)
}

func (f fakeLedger) Ready(ctx context.Context) error {
	return f.ready(ctx)
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWithTimeout(t *testing.T) {
	var deadline time.Time
	h := withTimeout(time.Minute, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, _ = r.Context().Deadline()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if left := time.Until(deadline); left <= 0 || left > time.Minute {
		t.Fatalf("deadline in %v, want within a minute", left)
	}
}
