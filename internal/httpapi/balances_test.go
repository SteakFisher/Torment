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
	// -(2^64 + 1) does not fit in an int64
	huge := new(big.Int).Neg(new(big.Int).SetBit(big.NewInt(1), 64, 1))
	oneTotals := []ledger.TypeTotal{{Type: ledger.TypeLiability, Currency: "USD", Precision: 2, Amount: huge}}
	oneAccount := []ledger.AccountBalances{{Address: "rider:1:wallet", Type: ledger.TypeLiability, Balances: []ledger.Balance{{Currency: "USD", Precision: 2, Amount: huge}}}}
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
		one        bool // the ledger returns one account
		noWallet   bool // the ledger returns one account with no balances
		wantArgs   string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "account",
			path:       "/accounts/rider:1:wallet/balances",
			one:        true,
			wantArgs:   "rider:1:wallet  0001-01-01T00:00:00Z",
			wantStatus: http.StatusOK,
			wantBody: `{"address":"rider:1:wallet","totals":[{"type":"liability","currency":"USD","precision":2,"amount":"-18446744073709551617"}],"accounts":[` +
				`{"address":"rider:1:wallet","type":"liability","balances":[{"currency":"USD","precision":2,"amount":"-18446744073709551617"}]}]}`,
		},
		{name: "account in currency at", path: "/accounts/rider:1:wallet/balances?currency=USD&at=2026-10-31T23:59:59%2B04:00", one: true, wantArgs: "rider:1:wallet USD 2026-10-31T19:59:59Z", wantStatus: http.StatusOK},
		{
			name:       "account without wallet",
			path:       "/accounts/rider:1:wallet/balances",
			noWallet:   true,
			wantArgs:   "rider:1:wallet  0001-01-01T00:00:00Z",
			wantStatus: http.StatusOK,
			wantBody:   `{"address":"rider:1:wallet","totals":[],"accounts":[{"address":"rider:1:wallet","type":"liability","balances":[]}]}`,
		},
		{name: "account not found", path: "/accounts/nobody/balances", err: ledger.ErrAccountNotFound, wantArgs: "nobody  0001-01-01T00:00:00Z", wantStatus: http.StatusNotFound},
		{name: "unexpected", path: "/accounts/nobody/balances", err: errors.New("boom"), wantArgs: "nobody  0001-01-01T00:00:00Z", wantStatus: http.StatusInternalServerError},
		{
			name:       "pattern",
			path:       "/accounts/rider:1:*/balances?currency=USD&at=2026-10-31T00:00:00Z",
			wantArgs:   "rider:1:* USD 2026-10-31T00:00:00Z",
			wantStatus: http.StatusOK,
			wantBody: `{"address":"rider:1:*","totals":[{"type":"liability","currency":"USD","precision":2,"amount":"150"}],"accounts":[` +
				`{"address":"rider:1:bonus","type":"liability","balances":[{"currency":"USD","precision":2,"amount":"50"}]},` +
				`{"address":"rider:1:wallet","type":"liability","balances":[{"currency":"USD","precision":2,"amount":"100"}]}]}`,
		},
		{name: "none", path: "/accounts/nobody:*/balances", empty: true, wantArgs: "nobody:*  0001-01-01T00:00:00Z", wantStatus: http.StatusOK, wantBody: `{"address":"nobody:*","totals":[],"accounts":[]}`},
		{name: "bad at", path: "/accounts/*/balances?at=x", wantStatus: http.StatusBadRequest},
		{name: "too many wallets", path: "/accounts/*/balances", err: ledger.ErrPatternTooBroad, wantArgs: "*  0001-01-01T00:00:00Z", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs string
			l := fakeLedger{balances: func(_ context.Context, address, currency string, at time.Time) ([]ledger.TypeTotal, []ledger.AccountBalances, error) {
				gotArgs = fmt.Sprintf("%s %s %s", address, currency, at.UTC().Format(time.RFC3339))
				switch {
				case tt.err != nil:
					return nil, nil, fmt.Errorf("wrapped: %w", tt.err)
				case tt.empty:
					return []ledger.TypeTotal{}, []ledger.AccountBalances{}, nil
				case tt.noWallet:
					return []ledger.TypeTotal{}, []ledger.AccountBalances{{Address: "rider:1:wallet", Type: ledger.TypeLiability, Balances: []ledger.Balance{}}}, nil
				case tt.one:
					return oneTotals, oneAccount, nil
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
