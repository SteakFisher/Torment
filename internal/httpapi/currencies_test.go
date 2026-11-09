package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"torment/internal/ledger"
)

func TestPostCurrency(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		created    bool
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "created", body: `{"code":"USD/2"}`, created: true, wantStatus: http.StatusCreated, wantBody: `{"name":"USD","precision":2}`},
		{name: "replay", body: `{"code":"USD/2"}`, wantStatus: http.StatusOK, wantBody: `{"name":"USD","precision":2}`},
		{name: "malformed json", body: `{"code":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"code":"USD/2","x":1}`, wantStatus: http.StatusBadRequest},
		{name: "data after the body", body: `{"code":"USD/2"}{"code":"EUR/2"}`, wantStatus: http.StatusBadRequest},
		{name: "ledger is not accepted", body: `{"code":"USD/2","ledger":1}`, wantStatus: http.StatusBadRequest},
		{name: "conflict", body: `{"code":"USD/2"}`, err: ledger.ErrConflict, wantStatus: http.StatusConflict},
		{name: "invalid", body: `{"code":"usd"}`, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "unexpected", body: `{"code":"USD/2"}`, err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{registerCurrency: func(context.Context, string) (ledger.Currency, bool, error) {
				if tt.err != nil {
					return ledger.Currency{}, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				return ledger.Currency{Name: "USD", Precision: 2, Ledger: 1}, tt.created, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/currencies", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetCurrencies(t *testing.T) {
	tests := []struct {
		name       string
		currencies []ledger.Currency
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "none", wantStatus: http.StatusOK, wantBody: `{"currencies":[]}`},
		{
			name:       "some",
			currencies: []ledger.Currency{{Name: "EUR", Precision: 2, Ledger: 2}, {Name: "USD", Precision: 2, Ledger: 1}},
			wantStatus: http.StatusOK,
			wantBody:   `{"currencies":[{"name":"EUR","precision":2},{"name":"USD","precision":2}]}`,
		},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{currencies: func(context.Context) ([]ledger.Currency, error) { return tt.currencies, tt.err }}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/currencies", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Fatalf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}
