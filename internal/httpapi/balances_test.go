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
		name       string
		query      string
		wantAt     time.Time
		noCall     bool // the ledger is not called
		balances   []ledger.Balance
		err        error
		wantStatus int
		wantBody   string
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
		{name: "at", query: "?at=2026-10-31T23:59:59.5%2B04:00", wantAt: time.Date(2026, 10, 31, 19, 59, 59, 5e8, time.UTC), balances: []ledger.Balance{}, wantStatus: http.StatusOK},
		{name: "bad at", query: "?at=yesterday", wantStatus: http.StatusBadRequest, noCall: true},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAddress string
			var gotAt time.Time
			l := fakeLedger{balances: func(_ context.Context, address string, at time.Time) ([]ledger.Balance, error) {
				gotAddress, gotAt = address, at
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.balances, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/rider:1:wallet/balances"+tt.query, nil))
			wantAddress := "rider:1:wallet"
			if tt.noCall {
				wantAddress = ""
			}
			if rec.Code != tt.wantStatus || gotAddress != wantAddress || !gotAt.Equal(tt.wantAt) {
				t.Fatalf("status = %d, address %q, at %v; want %d, %q, %v (body %s)", rec.Code, gotAddress, gotAt, tt.wantStatus, wantAddress, tt.wantAt, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
