package ledger

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

func TestTransfer(t *testing.T) {
	// posting returns one world → rider posting of 100 USD with id 5, changed by edit
	posting := func(edit func(p *Posting)) []Posting {
		p := Posting{ID: "5", Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100"}
		if edit != nil {
			edit(&p)
		}
		return []Posting{p}
	}
	tbIDs := map[string]tb.Uint128{"world": tb.ID(), "rider:1:wallet": tb.ID()}
	// 2^64 does not fit in a uint64
	bigGroup := tb.BigIntToUint128(new(big.Int).Lsh(big.NewInt(1), 64))

	tests := []struct {
		name        string
		postings    []Posting
		missing     string
		status      tb.CreateTransferStatus
		wantErr     error
		wantOther   bool
		wantCall    bool
		wantCreated bool
		wantGroup   *tb.Uint128
	}{
		{name: "no postings", wantErr: ErrInvalid},
		{name: "two postings", postings: append(posting(nil), posting(nil)...), wantErr: ErrInvalid},
		{name: "missing id", postings: posting(func(p *Posting) { p.ID = "" }), wantErr: ErrInvalid},
		{name: "zero id", postings: posting(func(p *Posting) { p.ID = "0" }), wantErr: ErrInvalid},
		{name: "max id", postings: posting(func(p *Posting) { p.ID = "340282366920938463463374607431768211455" }), wantErr: ErrInvalid},
		{name: "bad source", postings: posting(func(p *Posting) { p.Source = "a b" }), wantErr: ErrInvalid},
		{name: "bad destination", postings: posting(func(p *Posting) { p.Destination = "" }), wantErr: ErrInvalid},
		{name: "same source and destination", postings: posting(func(p *Posting) { p.Destination = "world" }), wantErr: ErrInvalid},
		{name: "zero amount", postings: posting(func(p *Posting) { p.Amount = "0" }), wantErr: ErrInvalid},
		{name: "negative amount", postings: posting(func(p *Posting) { p.Amount = "-1" }), wantErr: ErrInvalid},
		{name: "non-numeric amount", postings: posting(func(p *Posting) { p.Amount = "ten" }), wantErr: ErrInvalid},
		{name: "amount over 128 bits", postings: posting(func(p *Posting) { p.Amount = "340282366920938463463374607431768211456" }), wantErr: ErrInvalid},
		{name: "bad group id", postings: posting(func(p *Posting) { p.GroupID = "0" }), wantErr: ErrInvalid},
		{name: "missing source wallet", postings: posting(nil), missing: "world", wantErr: ErrNotFound},
		{name: "missing destination wallet", postings: posting(nil), missing: "rider:1:wallet", wantErr: ErrNotFound},
		{name: "exceeds credits", postings: posting(nil), status: tb.TransferExceedsCredits, wantErr: ErrInsufficientFunds, wantCall: true},
		{name: "exceeds debits", postings: posting(nil), status: tb.TransferExceedsDebits, wantErr: ErrInsufficientFunds, wantCall: true},
		{name: "exists with different amount", postings: posting(nil), status: tb.TransferExistsWithDifferentAmount, wantErr: ErrConflict, wantCall: true},
		{name: "id already failed", postings: posting(nil), status: tb.TransferIDAlreadyFailed, wantErr: ErrConflict, wantCall: true},
		{name: "other status", postings: posting(nil), status: tb.TransferOverflowsDebits, wantOther: true, wantCall: true},
		{name: "created", postings: posting(nil), status: tb.TransferCreated, wantCall: true, wantCreated: true},
		{name: "exists", postings: posting(nil), status: tb.TransferExists, wantCall: true},
		{
			name:        "created with group",
			postings:    posting(func(p *Posting) { p.GroupID = "18446744073709551616" }),
			status:      tb.TransferCreated,
			wantCall:    true,
			wantCreated: true,
			wantGroup:   &bigGroup,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeQuerier{getWalletByAddress: func(_ context.Context, arg sqlc.GetWalletByAddressParams) (sqlc.GetWalletByAddressRow, error) {
				if arg.Address == tt.missing || arg.Currency != "USD" {
					return sqlc.GetWalletByAddressRow{}, pgx.ErrNoRows
				}
				return sqlc.GetWalletByAddressRow{TbID: idToBytes(tbIDs[arg.Address]), Ledger: 7}, nil
			}}
			var sent []tb.Transfer
			tbc := &fakeTB{createTransfers: func(transfers []tb.Transfer) ([]tb.CreateTransferResult, error) {
				sent = transfers
				return []tb.CreateTransferResult{{Status: tt.status}}, nil
			}}

			got, created, err := New(q, tbc).Transfer(context.Background(), tt.postings)
			if (sent != nil) != tt.wantCall {
				t.Fatalf("CreateTransfers called = %v, want %v", sent != nil, tt.wantCall)
			}
			if tt.wantOther {
				if err == nil || errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInsufficientFunds) || errors.Is(err, ErrConflict) {
					t.Fatalf("err = %v, want an error that is none of the sentinels", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if len(sent) != 1 {
				t.Fatalf("sent %d transfers, want 1", len(sent))
			}
			if created != tt.wantCreated {
				t.Fatalf("created = %v, want %v", created, tt.wantCreated)
			}
			tr := sent[0]
			wantGroup := tb.ToUint128(5)
			if tt.wantGroup != nil {
				wantGroup = *tt.wantGroup
			}
			if tr.ID != tb.ToUint128(5) || tr.DebitAccountID != tbIDs["world"] || tr.CreditAccountID != tbIDs["rider:1:wallet"] ||
				tr.Ledger != 7 || tr.Code != 1 || tr.Amount != tb.ToUint128(100) || tr.UserData128 != wantGroup {
				t.Fatalf("sent transfer = %+v", tr)
			}
			if len(got) != 1 || got[0] != (Transfer{ID: tr.ID, GroupID: wantGroup}) {
				t.Fatalf("got %+v, want [{%v %v}]", got, tr.ID, wantGroup)
			}
		})
	}
}
