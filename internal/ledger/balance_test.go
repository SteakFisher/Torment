package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

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

			got, err := New(q, tbc).Balances(context.Background(), "rider:1:wallet", time.Time{})
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

func TestBalancesAt(t *testing.T) {
	usd, aed := tb.ID(), tb.ID()
	q := &fakeQuerier{
		getAccount: func(_ context.Context, address string) (sqlc.Account, error) {
			return sqlc.Account{ID: 7, Address: address, Type: sqlc.AccountTypeLiability}, nil
		},
		listWallets: func(context.Context, int64) ([]sqlc.ListWalletsRow, error) {
			return []sqlc.ListWalletsRow{
				{TbID: idToBytes(aed), Currency: "AED", Precision: 2},
				{TbID: idToBytes(usd), Currency: "USD", Precision: 2},
			}, nil
		},
	}
	at := time.Unix(100, 5)
	var filters []tb.AccountFilter
	tbc := &fakeTB{accountBalances: func(f tb.AccountFilter) ([]tb.AccountBalance, error) {
		filters = append(filters, f)
		if f.AccountID == usd { // AED has no transfer by at
			return []tb.AccountBalance{{DebitsPosted: tb.ToUint128(30), CreditsPosted: tb.ToUint128(100)}}, nil
		}
		return nil, nil
	}}

	got, err := New(q, tbc).Balances(context.Background(), "rider:1:wallet", at)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0].Amount.Int64() != 0 || got[1].Amount.Int64() != 70 {
		t.Fatalf("balances = %+v; want AED 0, USD 70", got)
	}
	flags := tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32()
	for i, f := range filters {
		if f.AccountID != []tb.Uint128{aed, usd}[i] || f.TimestampMax != 100_000_000_005 || f.TimestampMin != 0 || f.Limit != 1 || f.Flags != flags {
			t.Fatalf("filter %d = %+v", i, f)
		}
	}

	for _, bad := range []time.Time{time.Unix(0, 0), time.Unix(-5, 0), time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC)} {
		if _, err := New(q, tbc).Balances(context.Background(), "rider:1:wallet", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("at %v err = %v, want ErrInvalid", bad, err)
		}
	}
}
