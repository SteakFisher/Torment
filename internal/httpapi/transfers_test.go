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
		return `{"id":"` + id + `","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100"}`
	}
	body := `{"chains":[` +
		`[{"id":"00000000-0000-0001-0000-000000000001","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100","group_id":"00000000-0000-0000-0000-000000000009"}],` +
		`[` + posting("00000000-0000-0000-0000-000000000002") + `,` + posting("00000000-0000-0000-0000-000000000003") + `],` +
		`[` + posting("00000000-0000-0000-0000-000000000004") + `],` +
		` [` + posting("00000000-0000-0000-0000-000000000005") + `,` + posting("00000000-0000-0000-0000-000000000006") + `,` + posting("00000000-0000-0000-0000-000000000007") + `],` +
		`[` + posting("00000000-0000-0000-0000-000000000008") + `]]}`
	p := func(id string) ledger.Posting {
		return ledger.Posting{ID: id, Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: "100"}
	}
	first := p("00000000-0000-0001-0000-000000000001")
	first.GroupID = "00000000-0000-0000-0000-000000000009"
	wantChains := [][]ledger.Posting{{first}, {p("00000000-0000-0000-0000-000000000002"), p("00000000-0000-0000-0000-000000000003")}, {p("00000000-0000-0000-0000-000000000004")}, {p("00000000-0000-0000-0000-000000000005"), p("00000000-0000-0000-0000-000000000006"), p("00000000-0000-0000-0000-000000000007")}, {p("00000000-0000-0000-0000-000000000008")}}
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
	wantBody := `{"chains":[` +
		`[{"id":"00000000-0000-0001-0000-000000000001","group_id":"00000000-0000-0000-0000-000000000009","status":"created"}],` +
		`[{"id":"00000000-0000-0000-0000-000000000002","group_id":"00000000-0000-0000-0000-000000000002","status":"failed","error":"chain_failed","message":"chain failed: detail"},` +
		`{"id":"00000000-0000-0000-0000-000000000003","group_id":"00000000-0000-0000-0000-000000000003","status":"failed","error":"insufficient_funds","message":"insufficient funds"}],` +
		`[{"id":"00000000-0000-0000-0000-000000000004","group_id":"00000000-0000-0000-0000-000000000004","status":"exists"}],` +
		`[{"id":"00000000-0000-0000-0000-000000000005","group_id":"00000000-0000-0000-0000-000000000005","status":"failed","error":"conflict","message":"conflict"},` +
		`{"id":"00000000-0000-0000-0000-000000000006","group_id":"00000000-0000-0000-0000-000000000006","status":"failed","error":"id_already_failed","message":"id already failed"},` +
		`{"id":"00000000-0000-0000-0000-000000000007","group_id":"00000000-0000-0000-0000-000000000007","status":"failed","error":"internal","message":"internal error"}],` +
		`[{"id":"00000000-0000-0000-0000-000000000008","group_id":"00000000-0000-0000-0000-000000000008","status":"failed","error":"overflow","message":"overflow"}]]}`
	if strings.TrimSpace(rec.Body.String()) != wantBody {
		t.Fatalf("body = %s\nwant %s", rec.Body, wantBody)
	}
}

func TestPostTransferErrors(t *testing.T) {
	body := `{"chains":[[{"id":"00000000-0000-0000-0000-000000000001","debit":"world","credit":"rider:1:wallet","currency":"USD","amount":"100"}]]}`

	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "malformed json", body: `{"chains":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"chains":[],"x":1}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field in a posting", body: `{"chains":[[{"id":"00000000-0000-0000-0000-000000000001"},{"id":"00000000-0000-0000-0000-000000000002","x":1}]]}`, wantStatus: http.StatusBadRequest},
		{name: "posting outside a chain", body: `{"chains":[{"id":"00000000-0000-0000-0000-000000000001"}]}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: body, err: ledger.ErrInvalid, wantStatus: http.StatusBadRequest},
		{name: "account not found", body: body, err: ledger.ErrAccountNotFound, wantStatus: http.StatusNotFound},
		{name: "currency not found", body: body, err: ledger.ErrCurrencyNotFound, wantStatus: http.StatusNotFound},
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
