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
		transfers  []ledger.TransferRecord
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "transfers",
			transfers:  []ledger.TransferRecord{{ID: tb.ToUint128(5), GroupID: tb.ToUint128(9), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: time.Unix(1, 500).UTC()}},
			wantStatus: http.StatusOK,
			wantBody:   `{"group_id":"9","transfers":[{"id":"5","group_id":"9","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}]}`,
		},
		{name: "no transfers", transfers: []ledger.TransferRecord{}, wantStatus: http.StatusOK, wantBody: `{"group_id":"9","transfers":[]}`},
		{name: "invalid", err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotGroup string
			l := fakeLedger{groupTransfers: func(_ context.Context, groupID string) ([]ledger.TransferRecord, error) {
				gotGroup = groupID
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.transfers, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/groups/9/transfers", nil))
			if rec.Code != tt.wantStatus || gotGroup != "9" {
				t.Fatalf("status = %d, group %q; want %d (body %s)", rec.Code, gotGroup, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetAccountTransfers(t *testing.T) {
	ts := time.Unix(1, 500).UTC()
	transfers := []ledger.TransferRecord{{ID: tb.ToUint128(5), GroupID: tb.ToUint128(5), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: ts}}
	tests := []struct {
		name       string
		query      string
		more       bool
		err        error
		wantParams ledger.AccountTransfersParams
		wantStatus int
		wantBody   string
	}{
		{
			name:       "page with more",
			query:      "?currency=USD&from=1970-01-01T00:00:00Z&before=1970-01-01T00:00:02Z&limit=1",
			more:       true,
			wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet", Currency: "USD", From: time.Unix(0, 0), Before: time.Unix(2, 0), Limit: 1},
			wantStatus: http.StatusOK,
			wantBody: `{"address":"rider:1:wallet","transfers":[{"id":"5","group_id":"5","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}],` +
				`"next":"1970-01-01T00:00:01.0000005Z"}`,
		},
		{name: "last page", wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet"}, wantStatus: http.StatusOK},
		{name: "bad before", query: "?before=x", wantStatus: http.StatusBadRequest},
		{name: "bad limit", query: "?limit=x", wantStatus: http.StatusBadRequest},
		{name: "not found", err: ledger.ErrNotFound, wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet"}, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ledger.AccountTransfersParams
			l := fakeLedger{accountTransfers: func(_ context.Context, p ledger.AccountTransfersParams) ([]ledger.TransferRecord, bool, error) {
				got = p
				if tt.err != nil {
					return nil, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				return transfers, tt.more, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/rider:1:wallet/transfers"+tt.query, nil))
			if rec.Code != tt.wantStatus || got.Address != tt.wantParams.Address || got.Currency != tt.wantParams.Currency ||
				!got.From.Equal(tt.wantParams.From) || !got.Before.Equal(tt.wantParams.Before) || got.Limit != tt.wantParams.Limit {
				t.Fatalf("status = %d, params %+v; want %d, %+v (body %s)", rec.Code, got, tt.wantStatus, tt.wantParams, rec.Body)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
			if tt.name == "last page" && strings.Contains(rec.Body.String(), "next") {
				t.Fatalf("body = %s, want no next", rec.Body)
			}
		})
	}
}
