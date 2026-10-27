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

	"torment/internal/ledger"
)

func TestGetBalances(t *testing.T) {
	tests := []struct {
		name       string
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
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAddress string
			l := fakeLedger{balances: func(_ context.Context, address string) ([]ledger.Balance, error) {
				gotAddress = address
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.balances, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/rider:1:wallet/balances", nil))
			if rec.Code != tt.wantStatus || gotAddress != "rider:1:wallet" {
				t.Fatalf("status = %d, address %q; want %d (body %s)", rec.Code, gotAddress, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
