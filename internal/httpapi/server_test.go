package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	transfer         func(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error)
	groupTransfers   func(ctx context.Context, p ledger.GroupTransfersParams) ([]ledger.TransferRecord, bool, error)
	listAccounts     func(ctx context.Context, pattern, cursor string, limit int) ([]ledger.Account, bool, error)
	balances         func(ctx context.Context, address, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error)
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

func (f fakeLedger) Transfer(ctx context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error) {
	return f.transfer(ctx, chains)
}

func (f fakeLedger) GroupTransfers(ctx context.Context, p ledger.GroupTransfersParams) ([]ledger.TransferRecord, bool, error) {
	return f.groupTransfers(ctx, p)
}

func (f fakeLedger) ListAccounts(ctx context.Context, pattern, cursor string, limit int) ([]ledger.Account, bool, error) {
	return f.listAccounts(ctx, pattern, cursor, limit)
}

func (f fakeLedger) Balances(ctx context.Context, address, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error) {
	return f.balances(ctx, address, currency, at)
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

func TestUnmatchedRoutes(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
		wantAllow    string
		wantBody     string
	}{
		{http.MethodGet, "/nope", http.StatusNotFound, "", `{"error":{"code":"route_not_found","message":"no route for GET /nope"}}`},
		{http.MethodDelete, "/currencies", http.StatusMethodNotAllowed, "GET, HEAD, POST", `{"error":{"code":"method_not_allowed","message":"method DELETE is not allowed on /currencies"}}`},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		routes(fakeLedger{}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.wantStatus || rec.Header().Get("Allow") != tt.wantAllow || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s %s: status %d, Allow %q, Content-Type %q; want %d, %q, application/json", tt.method, tt.path, rec.Code, rec.Header().Get("Allow"), rec.Header().Get("Content-Type"), tt.wantStatus, tt.wantAllow)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
			t.Errorf("%s %s: body = %s, want %s", tt.method, tt.path, got, tt.wantBody)
		}
	}
}
