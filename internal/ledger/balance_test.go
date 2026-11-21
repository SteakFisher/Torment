package ledger

import (
	"context"
	"errors"
	"fmt"
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
		{name: "unregistered currency", typ: TypeLiability, wallets: wallets, currency: "GBP", wantErr: ErrCurrencyNotFound},
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
			}, currencies: func(_ context.Context, names []string) (map[string]Currency, error) {
				registered := map[string]Currency{}
				for _, n := range names {
					if n == "USD" || n == "EUR" {
						registered[n] = Currency{Name: n}
					}
				}
				return registered, nil
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

func TestPatternBalances(t *testing.T) {
	ids := []tb.Uint128{tb.ID(), tb.ID(), tb.ID(), tb.ID()}
	wallets := []AddressWallet{
		{Address: "rider:1:bonus", Type: TypeLiability, Wallet: Wallet{Currency: "USD", Precision: 2, ID: ids[0]}},
		{Address: "rider:1:card", Type: TypeAsset, Wallet: Wallet{Currency: "USD", Precision: 2, ID: ids[1]}},
		{Address: "rider:1:wallet", Type: TypeLiability, Wallet: Wallet{Currency: "AED", Precision: 2, ID: ids[2]}},
		{Address: "rider:1:wallet", Type: TypeLiability, Wallet: Wallet{Currency: "USD", Precision: 2, ID: ids[3]}},
	}
	// every account has 10 debited and 40 credited, except AED which has no TigerBeetle account
	inTB := []tb.Account{
		{ID: ids[0], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
		{ID: ids[1], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
		{ID: ids[3], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
	}
	var gotRE, gotCurrency string
	var gotLimit int
	store := &fakeStore{patternWallets: func(_ context.Context, re, currency string, limit int) ([]AddressWallet, bool, error) {
		gotRE, gotCurrency, gotLimit = re, currency, limit
		return wallets, false, nil
	}}
	tbc := &fakeTB{lookupAccounts: func([]tb.Uint128) ([]tb.Account, error) { return inTB, nil }}

	totals, accounts, err := New(store, tbc, tbc).Balances(context.Background(), "rider:1:*", "", time.Time{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotRE != "^rider:1:[^:]+$" || gotCurrency != "" || gotLimit != maxPatternWallets {
		t.Fatalf("store got re %q, currency %q, limit %d", gotRE, gotCurrency, gotLimit)
	}
	// asset sorts before liability by type code; the asset nets to -30, liabilities to 30 each
	wantTotals := []string{"asset USD -30", "liability AED 0", "liability USD 60"}
	if len(totals) != len(wantTotals) {
		t.Fatalf("totals = %+v", totals)
	}
	for i, tt := range totals {
		if s := fmt.Sprintf("%s %s %s", tt.Type, tt.Currency, tt.Amount); s != wantTotals[i] {
			t.Fatalf("total %d = %s, want %s", i, s, wantTotals[i])
		}
	}
	wantAccounts := []string{"rider:1:bonus [USD 30]", "rider:1:card [USD -30]", "rider:1:wallet [AED 0 USD 30]"}
	if len(accounts) != len(wantAccounts) {
		t.Fatalf("accounts = %+v", accounts)
	}
	for i, a := range accounts {
		var parts []string
		for _, b := range a.Balances {
			parts = append(parts, b.Currency, b.Amount.String())
		}
		if s := fmt.Sprintf("%s %v", a.Address, parts); s != wantAccounts[i] {
			t.Fatalf("account %d = %s, want %s", i, s, wantAccounts[i])
		}
	}

	store.patternWallets = func(context.Context, string, string, int) ([]AddressWallet, bool, error) {
		return nil, true, nil
	}
	if _, _, err := New(store, tbc, tbc).Balances(context.Background(), "*", "", time.Time{}); !errors.Is(err, ErrPatternTooBroad) {
		t.Fatalf("too many wallets err = %v, want ErrPatternTooBroad", err)
	}
	for _, c := range []struct {
		pattern string
		at      time.Time
	}{{"rider:.*", time.Time{}}, {"rider", time.Unix(-1, 0)}} {
		if _, _, err := New(store, tbc, tbc).Balances(context.Background(), c.pattern, "", c.at); !errors.Is(err, ErrInvalid) {
			t.Fatalf("pattern %q at %v err = %v, want ErrInvalid", c.pattern, c.at, err)
		}
	}
}
