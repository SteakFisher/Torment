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
	// posting returns the JSON for a world → rider:1:wallet posting with the given id
	posting := func(id string) string {
		return `{"id":"` + id + `","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100"}`
	}
	body := `{"postings":[` +
		`{"id":"18446744073709551617","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100","group_id":"9"},` +
		`[` + posting("2") + `,` + posting("3") + `],` +
		posting("4") + `,` +
		` [` + posting("5") + `,` + posting("6") + `,` + posting("7") + `],` +
		posting("8") + `]}`
	p := func(id string) ledger.Posting {
		return ledger.Posting{ID: id, Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100"}
	}
	first := p("18446744073709551617")
	first.GroupID = "9"
	wantChains := [][]ledger.Posting{{first}, {p("2"), p("3")}, {p("4")}, {p("5"), p("6"), p("7")}, {p("8")}}
	// 2^64 + 1 does not fit in a uint64
	bigID := tb.BigIntToUint128(new(big.Int).SetBit(big.NewInt(1), 64, 1))
	tr := func(id uint64, created bool, err error) ledger.Transfer {
		return ledger.Transfer{ID: tb.ToUint128(id), GroupID: tb.ToUint128(id), Created: created, Err: err}
	}

	var got [][]ledger.Posting
	l := fakeLedger{transfer: func(_ context.Context, chains [][]ledger.Posting) ([][]ledger.Transfer, error) {
		got = chains
		return [][]ledger.Transfer{
			{{ID: bigID, GroupID: tb.ToUint128(9), Created: true}},
			{tr(2, false, fmt.Errorf("%w: detail", ledger.ErrChainFailed)), tr(3, false, ledger.ErrInsufficientFunds)},
			{tr(4, false, nil)},
			{tr(5, false, ledger.ErrConflict), tr(6, false, ledger.ErrIDAlreadyFailed), tr(7, false, errors.New("boom"))},
			{tr(8, false, ledger.ErrOverflow)},
		}, nil
	}}
	rec := httptest.NewRecorder()
	routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	if !reflect.DeepEqual(got, wantChains) {
		t.Fatalf("chains = %+v, want %+v", got, wantChains)
	}
	wantBody := `{"transfers":[` +
		`{"id":"18446744073709551617","group_id":"9","status":"created"},` +
		`[{"id":"2","group_id":"2","status":"failed","error":"chain_failed","message":"chain failed: detail"},` +
		`{"id":"3","group_id":"3","status":"failed","error":"insufficient_funds","message":"insufficient funds"}],` +
		`{"id":"4","group_id":"4","status":"exists"},` +
		`[{"id":"5","group_id":"5","status":"failed","error":"conflict","message":"conflict"},` +
		`{"id":"6","group_id":"6","status":"failed","error":"id_already_failed","message":"id already failed"},` +
		`{"id":"7","group_id":"7","status":"failed","error":"internal","message":"internal error"}],` +
		`{"id":"8","group_id":"8","status":"failed","error":"overflow","message":"overflow"}]}`
	if strings.TrimSpace(rec.Body.String()) != wantBody {
		t.Fatalf("body = %s\nwant %s", rec.Body, wantBody)
	}
}

func TestPostTransferErrors(t *testing.T) {
	body := `{"postings":[{"id":"1","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"100"}]}`

	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "malformed json", body: `{"postings":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"postings":[],"x":1}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field in a posting", body: `{"postings":[{"id":"1","x":1}]}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field in a chained posting", body: `{"postings":[[{"id":"1"},{"id":"2","x":1}]]}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: body, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "not found", body: body, err: ledger.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "unexpected", body: body, err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := fakeLedger{transfer: func(context.Context, [][]ledger.Posting) ([][]ledger.Transfer, error) {
				if tt.err == nil {
					t.Fatal("Transfer called")
				}
				return nil, fmt.Errorf("wrapped: %w", tt.err)
			}}
			rec := httptest.NewRecorder()
			routes(l).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
		})
	}
}
