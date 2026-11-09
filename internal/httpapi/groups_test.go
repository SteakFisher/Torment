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
		query      string
		transfers  []ledger.TransferRecord
		more       bool
		err        error
		wantParams ledger.GroupTransfersParams
		wantStatus int
		wantBody   string
	}{
		{
			name:       "page with more",
			query:      "?cursor=1970-01-01T00:00:02Z&limit=1",
			transfers:  []ledger.TransferRecord{{ID: tb.ToUint128(5), GroupID: tb.ToUint128(9), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: time.Unix(1, 500).UTC()}},
			more:       true,
			wantParams: ledger.GroupTransfersParams{GroupID: "00000000-0000-0000-0000-00000000000A", Cursor: time.Unix(2, 0), Limit: 1},
			wantStatus: http.StatusOK,
			wantBody: `{"group_id":"00000000-0000-0000-0000-00000000000a","transfers":[{"id":"00000000-0000-0000-0000-000000000005","group_id":"00000000-0000-0000-0000-000000000009","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}],` +
				`"next":"1970-01-01T00:00:01.0000005Z"}`,
		},
		{name: "malformed cursor", query: "?cursor=yesterday", wantStatus: http.StatusBadRequest},
		{name: "malformed limit", query: "?limit=x", wantStatus: http.StatusBadRequest},
		{
			name:       "transfers",
			transfers:  []ledger.TransferRecord{{ID: tb.ToUint128(5), GroupID: tb.ToUint128(9), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: time.Unix(1, 500).UTC()}},
			wantParams: ledger.GroupTransfersParams{GroupID: "00000000-0000-0000-0000-00000000000A"},
			wantStatus: http.StatusOK,
			wantBody:   `{"group_id":"00000000-0000-0000-0000-00000000000a","transfers":[{"id":"00000000-0000-0000-0000-000000000005","group_id":"00000000-0000-0000-0000-000000000009","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}]}`,
		},
		{name: "no transfers", transfers: []ledger.TransferRecord{}, wantParams: ledger.GroupTransfersParams{GroupID: "00000000-0000-0000-0000-00000000000A"}, wantStatus: http.StatusOK, wantBody: `{"group_id":"00000000-0000-0000-0000-00000000000a","transfers":[]}`},
		{name: "invalid", err: ledger.ErrInvalid, wantParams: ledger.GroupTransfersParams{GroupID: "00000000-0000-0000-0000-00000000000A"}, wantStatus: http.StatusBadRequest},
		{name: "unexpected", err: errors.New("boom"), wantParams: ledger.GroupTransfersParams{GroupID: "00000000-0000-0000-0000-00000000000A"}, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ledger.GroupTransfersParams
			l := fakeLedger{groupTransfers: func(_ context.Context, p ledger.GroupTransfersParams) ([]ledger.TransferRecord, bool, error) {
				got = p
				if tt.err != nil {
					return nil, false, fmt.Errorf("wrapped: %w", tt.err)
				}
				return tt.transfers, tt.more, nil
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/groups/00000000-0000-0000-0000-00000000000A/transfers"+tt.query, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got.GroupID != tt.wantParams.GroupID || !got.Cursor.Equal(tt.wantParams.Cursor) || got.Limit != tt.wantParams.Limit {
				t.Fatalf("params = %+v, want %+v", got, tt.wantParams)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestGetAccountTransfers(t *testing.T) {
	ts := time.Unix(1, 500).UTC()
	transfers := []ledger.TransferRecord{{ID: tb.ToUint128(5), GroupID: tb.ToUint128(5), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: big.NewInt(100), Timestamp: ts}}
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
			query:      "?currency=USD&from=1970-01-01T00:00:00Z&cursor=1970-01-01T00:00:02Z&limit=1",
			more:       true,
			wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet", Currency: "USD", From: time.Unix(0, 0), Cursor: time.Unix(2, 0), Limit: 1},
			wantStatus: http.StatusOK,
			wantBody: `{"address":"rider:1:wallet","transfers":[{"id":"00000000-0000-0000-0000-000000000005","group_id":"00000000-0000-0000-0000-000000000005","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100","timestamp":"1970-01-01T00:00:01.0000005Z"}],` +
				`"next":"1970-01-01T00:00:01.0000005Z"}`,
		},
		{name: "last page", wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet"}, wantStatus: http.StatusOK},
		{name: "bad cursor", query: "?cursor=x", wantStatus: http.StatusBadRequest},
		{name: "before is not a cursor", query: "?before=1970-01-01T00:00:02Z", wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet"}, wantStatus: http.StatusOK},
		{name: "bad limit", query: "?limit=x", wantStatus: http.StatusBadRequest},
		{name: "not found", err: ledger.ErrAccountNotFound, wantParams: ledger.AccountTransfersParams{Address: "rider:1:wallet"}, wantStatus: http.StatusNotFound},
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
				!got.From.Equal(tt.wantParams.From) || !got.Cursor.Equal(tt.wantParams.Cursor) || got.Limit != tt.wantParams.Limit {
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
