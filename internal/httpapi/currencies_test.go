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
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "created", body: `{"code":"USD/2"}`, wantStatus: http.StatusCreated, wantBody: `{"name":"USD","precision":2,"ledger":1}`},
		{name: "malformed json", body: `{"code":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"code":"USD/2","x":1}`, wantStatus: http.StatusBadRequest},
		{name: "ledger is not accepted", body: `{"code":"USD/2","ledger":1}`, wantStatus: http.StatusBadRequest},
		{name: "conflict", body: `{"code":"USD/2"}`, err: ledger.ErrConflict, wantStatus: http.StatusConflict},
		{name: "invalid", body: `{"code":"usd"}`, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "unexpected", body: `{"code":"USD/2"}`, err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":"internal error"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{registerCurrency: func(context.Context, string) (ledger.Currency, error) {
				if tt.err != nil {
					return ledger.Currency{}, fmt.Errorf("wrapped: %w", tt.err)
				}
				return ledger.Currency{Name: "USD", Precision: 2, Ledger: 1}, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/currencies", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
