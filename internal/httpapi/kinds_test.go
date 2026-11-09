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

func TestPostKind(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		created    bool
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "created", body: `{"name":"deposit"}`, created: true, wantStatus: http.StatusCreated, wantBody: `{"name":"deposit"}`},
		{name: "replay", body: `{"name":"deposit"}`, wantStatus: http.StatusOK, wantBody: `{"name":"deposit"}`},
		{name: "malformed json", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "code is not accepted", body: `{"name":"deposit","code":2}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: `{"name":"Deposit"}`, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "unexpected", body: `{"name":"deposit"}`, err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{registerKind: func(_ context.Context, name string) (ledger.Kind, bool, error) {
				if tt.err != nil {
					return ledger.Kind{}, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				return ledger.Kind{Name: name, Code: 2}, tt.created, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/kinds", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetKinds(t *testing.T) {
	tests := []struct {
		name       string
		kinds      []ledger.Kind
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "none", wantStatus: http.StatusOK, wantBody: `{"kinds":[]}`},
		{name: "some", kinds: []ledger.Kind{{Name: "deposit", Code: 2}, {Name: "trade", Code: 1}}, wantStatus: http.StatusOK, wantBody: `{"kinds":[{"name":"deposit"},{"name":"trade"}]}`},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{kinds: func(context.Context) ([]ledger.Kind, error) { return tt.kinds, tt.err }}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kinds", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Fatalf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}
