package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestPostTransfer(t *testing.T) {
	body := `{"postings":[{"source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100","group_id":"9"}]}`
	wantPostings := []ledger.Posting{{Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100", GroupID: "9"}}
	// 2^64 + 1 does not fit in a uint64
	bigID := tb.BigIntToUint128(new(big.Int).SetBit(big.NewInt(1), 64, 1))

	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "created",
			body:       body,
			wantStatus: http.StatusCreated,
			wantBody:   `{"transfers":[{"id":"18446744073709551617","group_id":"9"}]}`,
		},
		{name: "malformed json", body: `{"postings":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"postings":[],"x":1}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: body, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "not found", body: body, err: ledger.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "insufficient funds", body: body, err: ledger.ErrInsufficientFunds, wantStatus: http.StatusUnprocessableEntity},
		{name: "unexpected", body: body, err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPostings []ledger.Posting
			l := fakeLedger{transfer: func(_ context.Context, postings []ledger.Posting) ([]ledger.Transfer, error) {
				gotPostings = postings
				if tt.err != nil {
					return nil, fmt.Errorf("wrapped: %w", tt.err)
				}
				return []ledger.Transfer{{ID: bigID, GroupID: tb.ToUint128(9)}}, nil
			}}
			rec := httptest.NewRecorder()
			New(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.body == body && !reflect.DeepEqual(gotPostings, wantPostings) {
				t.Fatalf("postings = %+v, want %+v", gotPostings, wantPostings)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("body = %s, want %s", rec.Body, tt.wantBody)
			}
		})
	}
}
