package ledger

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

func TestBalances(t *testing.T) {
	usd, aed := tb.ID(), tb.ID()
	wallets := []sqlc.ListWalletsRow{
		{TbID: idToBytes(aed), Currency: "AED", Precision: 2},
		{TbID: idToBytes(usd), Currency: "USD", Precision: 2},
	}
	// only USD exists in TigerBeetle, with 30 debited and 100 credited
	inTB := []tb.Account{{ID: usd, DebitsPosted: tb.ToUint128(30), CreditsPosted: tb.ToUint128(100)}}

	tests := []struct {
		name       string
		typ        sqlc.AccountType
		missing    bool
		wallets    []sqlc.ListWalletsRow
		wantErr    error
		wantUSD    int64
		wantLookup bool
	}{
		{name: "unknown address", missing: true, wantErr: ErrNotFound},
		{name: "no wallets", typ: sqlc.AccountTypeLiability},
		{name: "credit-normal", typ: sqlc.AccountTypeLiability, wallets: wallets, wantUSD: 70, wantLookup: true},
		{name: "debit-normal", typ: sqlc.AccountTypeAsset, wallets: wallets, wantUSD: -70, wantLookup: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeQuerier{
				getAccount: func(_ context.Context, address string) (sqlc.Account, error) {
					if tt.missing {
						return sqlc.Account{}, pgx.ErrNoRows
					}
					return sqlc.Account{ID: 7, Address: address, Type: tt.typ}, nil
				},
				listWallets: func(context.Context, int64) ([]sqlc.ListWalletsRow, error) { return tt.wallets, nil },
			}
			looked := false
			tbc := &fakeTB{lookupAccounts: func([]tb.Uint128) ([]tb.Account, error) {
				looked = true
				return inTB, nil
			}}

			got, err := New(q, tbc).Balances(context.Background(), "rider:1:wallet")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if looked != tt.wantLookup {
				t.Fatalf("LookupAccounts called = %v, want %v", looked, tt.wantLookup)
			}
			if tt.wantErr != nil {
				return
			}
			if len(got) != len(tt.wallets) {
				t.Fatalf("got %d balances, want %d", len(got), len(tt.wallets))
			}
			for _, b := range got {
				want := int64(0)
				if b.Currency == "USD" {
					want = tt.wantUSD
				}
				if b.Amount.Int64() != want || b.Precision != 2 {
					t.Fatalf("%s balance = %s (precision %d), want %d", b.Currency, b.Amount, b.Precision, want)
				}
			}
		})
	}
}
