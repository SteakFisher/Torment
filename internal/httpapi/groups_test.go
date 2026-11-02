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

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestGetGroupTransfers(t *testing.T) {
	tests := []struct {
		name       string
		transfers  []ledger.GroupTransfer
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "transfers",
			transfers:  []ledger.GroupTransfer{{ID: tb.ToUint128(5), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: time.Unix(1, 500).UTC()}},
			wantStatus: http.StatusOK,
			wantBody:   `{"group_id":"9","transfers":[{"id":"5","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}]}`,
		},
		{name: "no transfers", transfers: []ledger.GroupTransfer{}, wantStatus: http.StatusOK, wantBody: `{"group_id":"9","transfers":[]}`},
		{name: "invalid", err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotGroup string
			l := fakeLedger{groupTransfers: func(_ context.Context, groupID string) ([]ledger.GroupTransfer, error) {
				gotGroup = groupID
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.transfers, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/groups/9/transfers", nil))
			if rec.Code != tt.wantStatus || gotGroup != "9" {
				t.Fatalf("status = %d, group %q; want %d (body %s)", rec.Code, gotGroup, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
