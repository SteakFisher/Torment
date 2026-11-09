package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestBalances(t *testing.T) {
	usd, aed := tb.ID(), tb.ID()
	wallets := []Wallet{{Currency: "AED", Precision: 2, ID: aed}, {Currency: "USD", Precision: 2, ID: usd}}
	// only USD exists in TigerBeetle, with 30 debited and 100 credited
	inTB := []tb.Account{{ID: usd, DebitsPosted: tb.ToUint128(30), CreditsPosted: tb.ToUint128(100)}}

	tests := []struct {
		name       string
		typ        AccountType
		missing    bool
		wallets    []Wallet
		currency   string
		wantN      int
		wantErr    error
		wantUSD    int64
		wantLookup bool
	}{
		{name: "unknown address", missing: true, wantErr: ErrAccountNotFound},
		{name: "no wallets", typ: TypeLiability},
		{name: "credit-normal", typ: TypeLiability, wallets: wallets, wantN: 2, wantUSD: 70, wantLookup: true},
		{name: "debit-normal", typ: TypeAsset, wallets: wallets, wantN: 2, wantUSD: -70, wantLookup: true},
		{name: "one currency", typ: TypeLiability, wallets: wallets, currency: "USD", wantN: 1, wantUSD: 70, wantLookup: true},
		{name: "no wallet in currency", typ: TypeLiability, wallets: wallets, currency: "EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{accountWallets: func(_ context.Context, address, currency string) (Account, error) {
				if tt.missing {
					return Account{}, ErrAccountNotFound
				}
				acct := Account{Address: address, Type: tt.typ}
				for _, w := range tt.wallets {
					if currency == "" || w.Currency == currency {
						acct.Wallets = append(acct.Wallets, w)
					}
				}
				return acct, nil
			}}
			looked := false
			tbc := &fakeTB{lookupAccounts: func([]tb.Uint128) ([]tb.Account, error) {
				looked = true
				return inTB, nil
			}}

			totals, accounts, err := New(store, tbc, tbc).Balances(context.Background(), "rider:1:wallet", tt.currency, time.Time{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if looked != tt.wantLookup {
				t.Fatalf("LookupAccounts called = %v, want %v", looked, tt.wantLookup)
			}
			if tt.wantErr != nil {
				return
			}
			var got []Balance
			if len(accounts) > 0 {
				got = accounts[0].Balances
			}
			// the exact account is listed even with no wallet in scope
			if len(accounts) != 1 || accounts[0].Address != "rider:1:wallet" || len(got) != tt.wantN || len(totals) != tt.wantN {
				t.Fatalf("got accounts %+v, totals %+v; want %d balances", accounts, totals, tt.wantN)
			}
			for i, b := range got {
				want := int64(0)
				if b.Currency == "USD" {
					want = tt.wantUSD
				}
				if b.Amount.Int64() != want || b.Precision != 2 {
					t.Fatalf("%s balance = %s (precision %d), want %d", b.Currency, b.Amount, b.Precision, want)
				}
				// one account, so each total is its balance
				if tot := totals[i]; tot.Type != tt.typ || tot.Currency != b.Currency || tot.Amount.Cmp(b.Amount) != 0 {
					t.Fatalf("total %d = %+v, want %s %s %s", i, tot, tt.typ, b.Currency, b.Amount)
				}
			}
		})
	}
}

func TestBalancesAt(t *testing.T) {
	usd, aed := tb.ID(), tb.ID()
	store := &fakeStore{accountWallets: func(_ context.Context, address, _ string) (Account, error) {
		return Account{Address: address, Type: TypeLiability, Wallets: []Wallet{{Currency: "AED", Precision: 2, ID: aed}, {Currency: "USD", Precision: 2, ID: usd}}}, nil
	}}
	at := time.Unix(100, 5)
	var filters []tb.AccountFilter
	tbc := &fakeTB{accountBalances: func(f tb.AccountFilter) ([]tb.AccountBalance, error) {
		filters = append(filters, f)
		if f.AccountID == usd { // AED has no transfer by at
			return []tb.AccountBalance{{DebitsPosted: tb.ToUint128(30), CreditsPosted: tb.ToUint128(100)}}, nil
		}
		return nil, nil
	}}

	_, accounts, err := New(store, tbc, tbc).Balances(context.Background(), "rider:1:wallet", "", at)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %+v, err = %v", accounts, err)
	}
	got := accounts[0].Balances
	if len(got) != 2 || got[0].Amount.Int64() != 0 || got[1].Amount.Int64() != 70 {
		t.Fatalf("balances = %+v; want AED 0, USD 70", got)
	}
	flags := tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32()
	if len(filters) != 2 || filters[0].AccountID == filters[1].AccountID {
		t.Fatalf("filters = %+v; want one per wallet", filters)
	}
	for i, f := range filters {
		if (f.AccountID != aed && f.AccountID != usd) || f.TimestampMax != 100_000_000_005 || f.TimestampMin != 0 || f.Limit != 1 || f.Flags != flags {
			t.Fatalf("filter %d = %+v", i, f)
		}
	}

	for _, bad := range []time.Time{time.Unix(0, 0), time.Unix(-5, 0), time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC)} {
		if _, _, err := New(store, tbc, tbc).Balances(context.Background(), "rider:1:wallet", "", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("at %v err = %v, want ErrInvalid", bad, err)
		}
	}
}
