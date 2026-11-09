package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"torment/internal/ledger"
)

func TestAPIKey(t *testing.T) {
	l := fakeLedger{listAccounts: func(context.Context, string, string, int) ([]ledger.Account, bool, error) {
		return nil, false, nil
	}}
	h := New(l, []string{"old-key", "new-key"})
	tests := []struct {
		name       string
		path       string
		auth       string
		wantStatus int
	}{
		{name: "first key", path: "/accounts", auth: "Bearer old-key", wantStatus: http.StatusOK},
		{name: "second key", path: "/accounts", auth: "Bearer new-key", wantStatus: http.StatusOK},
		{name: "no header", path: "/accounts", wantStatus: http.StatusUnauthorized},
		{name: "wrong key", path: "/accounts", auth: "Bearer other", wantStatus: http.StatusUnauthorized},
		{name: "prefix of a key", path: "/accounts", auth: "Bearer old", wantStatus: http.StatusUnauthorized},
		{name: "not bearer", path: "/accounts", auth: "old-key", wantStatus: http.StatusUnauthorized},
		{name: "empty bearer", path: "/accounts", auth: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "unknown route without key", path: "/nope", wantStatus: http.StatusUnauthorized},
		{name: "healthz without key", path: "/healthz", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if want := `{"error":{"code":"unauthorized","message":"unauthorized"}}`; rec.Code == http.StatusUnauthorized && strings.TrimSpace(rec.Body.String()) != want {
				t.Fatalf("body = %s, want %s", rec.Body, want)
			}
		})
	}
}
