package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"torment/internal/ledger"
)

func TestPostAccount(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		created    bool
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "created",
			body:       `{"address":"rider:1:wallet","type":"liability"}`,
			created:    true,
			wantStatus: http.StatusCreated,
			wantBody:   `{"address":"rider:1:wallet","type":"liability","allow_negative":false,"created_at":"2026-01-02T03:04:05Z","wallets":[]}`,
		},
		{
			name:       "with wallets",
			body:       `{"address":"rider:1:wallet","type":"liability","currencies":["USD","AED"]}`,
			created:    true,
			wantStatus: http.StatusCreated,
			wantBody:   `{"address":"rider:1:wallet","type":"liability","allow_negative":false,"created_at":"2026-01-02T03:04:05Z","wallets":[{"currency":"USD","precision":2},{"currency":"AED","precision":2}]}`,
		},
		{name: "replay", body: `{"address":"rider:1:wallet","type":"liability"}`, wantStatus: http.StatusOK},
		{name: "malformed json", body: `{"address":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"address":"a","type":"asset","x":1}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: `{"address":"a","type":"nope"}`, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "conflict", body: `{"address":"a","type":"asset"}`, err: ledger.ErrConflict, wantStatus: http.StatusConflict},
		{name: "unexpected", body: `{"address":"a","type":"asset"}`, err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{registerAccount: func(_ context.Context, p ledger.RegisterAccountParams) (ledger.Account, bool, error) {
				if tt.err != nil {
					return ledger.Account{}, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				var wallets []ledger.Wallet
				for _, c := range p.Currencies {
					wallets = append(wallets, ledger.Wallet{Currency: c, Precision: 2})
				}
				return ledger.Account{
					Address:       p.Address,
					Type:          ledger.AccountType(p.Type),
					AllowNegative: p.AllowNegative,
					CreatedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
					Wallets:       wallets,
				}, tt.created, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
