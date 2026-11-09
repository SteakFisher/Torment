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
		{name: "unknown currency", body: `{"address":"a","type":"asset","currencies":["EUR"]}`, err: ledger.ErrCurrencyNotFound, wantStatus: http.StatusNotFound},
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
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetAccounts(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	accounts := []ledger.Account{
		{Address: "rider:1:wallet", Type: ledger.TypeLiability, CreatedAt: created, Wallets: []ledger.Wallet{{Currency: "USD", Precision: 2}}},
		{Address: "rider:2:wallet", Type: ledger.TypeLiability, CreatedAt: created},
	}
	tests := []struct {
		name       string
		path       string
		more       bool
		err        error
		wantArgs   string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "pattern page with more",
			path:       "/accounts/rider:*:wallet?after=a&limit=2",
			more:       true,
			wantArgs:   "rider:*:wallet a 2",
			wantStatus: http.StatusOK,
			wantBody: `{"accounts":[{"address":"rider:1:wallet","type":"liability","allow_negative":false,"created_at":"2026-01-02T03:04:05Z","wallets":[{"currency":"USD","precision":2}]},` +
				`{"address":"rider:2:wallet","type":"liability","allow_negative":false,"created_at":"2026-01-02T03:04:05Z","wallets":[]}],"next":"rider:2:wallet"}`,
		},
		{name: "encoded star", path: "/accounts/rider:%2A:wallet", wantArgs: "rider:*:wallet  0", wantStatus: http.StatusOK},
		{name: "all, last page", path: "/accounts?address=rider:*", wantArgs: "  0", wantStatus: http.StatusOK},
		{name: "bad limit", path: "/accounts?limit=x", wantStatus: http.StatusBadRequest},
		{name: "invalid", path: "/accounts/a:*:.", err: ledger.ErrInvalid, wantArgs: "a:*:.  0", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs string
			l := fakeLedger{listAccounts: func(_ context.Context, pattern, after string, limit int) ([]ledger.Account, bool, error) {
				gotArgs = fmt.Sprintf("%s %s %d", pattern, after, limit)
				if tt.err != nil {
					return nil, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				return accounts, tt.more, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus || gotArgs != tt.wantArgs {
				t.Fatalf("status = %d, args %q; want %d, %q (body %s)", rec.Code, gotArgs, tt.wantStatus, tt.wantArgs, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
			if !tt.more && strings.Contains(rec.Body.String(), "next") {
				t.Fatalf("body = %s, want no next", rec.Body)
			}
		})
	}
}

func TestGetAccount(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "found",
			wantStatus: http.StatusOK,
			wantBody:   `{"address":"rider:1:wallet","type":"liability","allow_negative":false,"created_at":"2026-01-02T03:04:05Z","wallets":[{"currency":"USD","precision":2}]}`,
		},
		{name: "not found", err: ledger.ErrAccountNotFound, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAddress string
			l := fakeLedger{account: func(_ context.Context, address string) (ledger.Account, error) {
				gotAddress = address
				if tt.err != nil {
					return ledger.Account{}, fmt.Errorf("wrapped: %w", tt.err)
				}
				return ledger.Account{
					Address:   address,
					Type:      ledger.TypeLiability,
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
					Wallets:   []ledger.Wallet{{Currency: "USD", Precision: 2}},
				}, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/rider:1:wallet", nil))
			if rec.Code != tt.wantStatus || gotAddress != "rider:1:wallet" {
				t.Fatalf("status = %d, address %q; want %d (body %s)", rec.Code, gotAddress, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
