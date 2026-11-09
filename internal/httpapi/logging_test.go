package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogRequests(t *testing.T) {
	tests := []struct {
		name, sent string
		keep       bool
	}{
		{name: "caller's id is kept", sent: "abc-123", keep: true},
		{name: "missing id is made"},
		{name: "too long id is replaced", sent: strings.Repeat("x", 65)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = requestID(r.Context())
				w.WriteHeader(http.StatusTeapot)
			}))
			req := httptest.NewRequest(http.MethodGet, "/currencies", nil)
			if tt.sent != "" {
				req.Header.Set("X-Request-Id", tt.sent)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-Id")
			if got == "" || got != seen {
				t.Fatalf("header id %q, handler saw %q; want the same non-empty id", got, seen)
			}
			if (got == tt.sent) != tt.keep {
				t.Fatalf("id = %q, sent %q, keep %v", got, tt.sent, tt.keep)
			}
			if rec.Code != http.StatusTeapot {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
			}
		})
	}
}

func TestReadyz(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want int
	}{{nil, http.StatusOK}, {errors.New("boom"), http.StatusServiceUnavailable}} {
		l := fakeLedger{ready: func(context.Context) error { return tt.err }}
		rec := httptest.NewRecorder()
		New(l, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != tt.want {
			t.Fatalf("ready err %v: status = %d, want %d", tt.err, rec.Code, tt.want)
		}
	}
}
