package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"torment/internal/ledger"
)

func TestGetBalances(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantCurrency string
		wantAt       time.Time
		noCall       bool // the ledger is not called
		balances     []ledger.Balance
		err          error
		wantStatus   int
		wantBody     string
	}{
		{
			name: "balances",
			// -(2^64 + 1) does not fit in an int64
			balances:   []ledger.Balance{{Currency: "USD", Precision: 2, Amount: new(big.Int).Neg(new(big.Int).SetBit(big.NewInt(1), 64, 1))}},
			wantStatus: http.StatusOK,
			wantBody:   `{"address":"rider:1:wallet","balances":[{"currency":"USD","precision":2,"amount":"-18446744073709551617"}]}`,
		},
		{name: "no wallets", balances: []ledger.Balance{}, wantStatus: http.StatusOK, wantBody: `{"address":"rider:1:wallet","balances":[]}`},
		{name: "not found", err: ledger.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "currency", query: "?currency=USD", wantCurrency: "USD", balances: []ledger.Balance{}, wantStatus: http.StatusOK},
		{name: "at", query: "?at=2026-10-31T23:59:59.5%2B04:00", wantAt: time.Date(2026, 10, 31, 19, 59, 59, 5e8, time.UTC), balances: []ledger.Balance{}, wantStatus: http.StatusOK},
		{name: "bad at", query: "?at=yesterday", wantStatus: http.StatusBadRequest, noCall: true},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAddress, gotCurrency string
			var gotAt time.Time
			l := fakeLedger{balances: func(_ context.Context, address, currency string, at time.Time) ([]ledger.Balance, error) {
				gotAddress, gotCurrency, gotAt = address, currency, at
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.balances, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/rider:1:wallet/balances"+tt.query, nil))
			wantAddress := "rider:1:wallet"
			if tt.noCall {
				wantAddress = ""
			}
			if rec.Code != tt.wantStatus || gotAddress != wantAddress || gotCurrency != tt.wantCurrency || !gotAt.Equal(tt.wantAt) {
				t.Fatalf("status = %d, address %q, currency %q, at %v; want %d, %q, %q, %v (body %s)", rec.Code, gotAddress, gotCurrency, gotAt, tt.wantStatus, wantAddress, tt.wantCurrency, tt.wantAt, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetPatternBalances(t *testing.T) {
	totals := []ledger.TypeTotal{{Type: ledger.TypeLiability, Currency: "USD", Precision: 2, Amount: big.NewInt(150)}}
	accounts := []ledger.AccountBalances{
		{Address: "rider:1:bonus", Type: ledger.TypeLiability, Balances: []ledger.Balance{{Currency: "USD", Precision: 2, Amount: big.NewInt(50)}}},
		{Address: "rider:1:wallet", Type: ledger.TypeLiability, Balances: []ledger.Balance{{Currency: "USD", Precision: 2, Amount: big.NewInt(100)}}},
	}
	tests := []struct {
		name       string
		path       string
		err        error
		empty      bool
		wantArgs   string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "balances",
			path:       "/accounts/rider:1:*/balances?currency=USD&at=2026-10-31T00:00:00Z",
			wantArgs:   "rider:1:* USD 2026-10-31T00:00:00Z",
			wantStatus: http.StatusOK,
			wantBody: `{"address":"rider:1:*","totals":[{"type":"liability","currency":"USD","precision":2,"amount":"150"}],"accounts":[` +
				`{"address":"rider:1:bonus","type":"liability","balances":[{"currency":"USD","precision":2,"amount":"50"}]},` +
				`{"address":"rider:1:wallet","type":"liability","balances":[{"currency":"USD","precision":2,"amount":"100"}]}]}`,
		},
		{name: "none", path: "/accounts/nobody:*/balances", empty: true, wantArgs: "nobody:*  0001-01-01T00:00:00Z", wantStatus: http.StatusOK, wantBody: `{"address":"nobody:*","totals":[],"accounts":[]}`},
		{name: "bad at", path: "/accounts/*/balances?at=x", wantStatus: http.StatusBadRequest},
		{name: "too many wallets", path: "/accounts/*/balances", err: ledger.ErrInvalid, wantArgs: "*  0001-01-01T00:00:00Z", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs string
			l := fakeLedger{patternBalances: func(_ context.Context, pattern, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error) {
				gotArgs = fmt.Sprintf("%s %s %s", pattern, currency, at.Format(time.RFC3339))
				switch {
				case tt.err != nil:
					return nil, nil, fmt.Errorf("wrapped: %w", tt.err)
				case tt.empty:
					return []ledger.TypeTotal{}, []ledger.AccountBalances{}, nil
				}
				return totals, accounts, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus || gotArgs != tt.wantArgs {
				t.Fatalf("status = %d, args %q; want %d, %q (body %s)", rec.Code, gotArgs, tt.wantStatus, tt.wantArgs, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
