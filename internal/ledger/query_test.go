package ledger

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestPatternRegex(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		wantErr bool
	}{
		{pattern: "", want: ""},
		{pattern: "world", want: "^world$"},
		{pattern: "rider:*:wallet", want: "^rider:[^:]+:wallet$"},
		{pattern: "rider:123:*", want: "^rider:123:[^:]+$"},
		{pattern: "*", want: "^[^:]+$"},
		{pattern: "rider:**", wantErr: true},
		{pattern: "rider:.*", wantErr: true},
		{pattern: "rider::wallet", wantErr: true},
		{pattern: "rider:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := patternRegex(tt.pattern)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("patternRegex = %q, err %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestListAccounts(t *testing.T) {
	stored := []Account{
		{Address: "rider:1:bonus", Type: TypeLiability},
		{Address: "rider:1:wallet", Type: TypeLiability, Wallets: []Wallet{{Currency: "AED", Precision: 2}, {Currency: "USD", Precision: 3}}},
		{Address: "rider:2:wallet", Type: TypeLiability},
	}
	tests := []struct {
		name      string
		limit     int
		wantLimit int
		wantMore  bool
		wantN     int
		wantErr   error
	}{
		{name: "default limit", limit: 0, wantLimit: defaultListLimit, wantN: 3},
		{name: "more", limit: 2, wantLimit: 2, wantMore: true, wantN: 2},
		{name: "exact", limit: 3, wantLimit: 3, wantN: 3},
		{name: "negative", limit: -1, wantErr: ErrInvalid},
		{name: "too large", limit: maxListLimit + 1, wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRE, gotAfter string
			var gotLimit int
			store := &fakeStore{listAccounts: func(_ context.Context, re, after string, limit int) ([]Account, bool, error) {
				gotRE, gotAfter, gotLimit = re, after, limit
				return stored[:min(len(stored), limit)], len(stored) > limit, nil
			}}
			accounts, more, err := New(store, nil, nil).ListAccounts(context.Background(), "rider:*:*", "rider:0", tt.limit)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if gotRE != "^rider:[^:]+:[^:]+$" || gotAfter != "rider:0" || gotLimit != tt.wantLimit {
				t.Fatalf("store got re %q, after %q, limit %d", gotRE, gotAfter, gotLimit)
			}
			if len(accounts) != tt.wantN || more != tt.wantMore {
				t.Fatalf("got %d accounts, more %v; want %d, %v", len(accounts), more, tt.wantN, tt.wantMore)
			}
			w := accounts[1].Wallets
			if len(w) != 2 || w[0] != (Wallet{Currency: "AED", Precision: 2}) || w[1] != (Wallet{Currency: "USD", Precision: 3}) {
				t.Fatalf("wallets = %+v", w)
			}
		})
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

func TestAccountTransfers(t *testing.T) {
	usd, aed, world := tb.ID(), tb.ID(), tb.ID()
	// transfer returns a transfer between the rider's wallet and world with the given timestamp
	transfer := func(wallet tb.Uint128, ts uint64) tb.Transfer {
		return tb.Transfer{ID: tb.ToUint128(ts), DebitAccountID: world, CreditAccountID: wallet, Amount: tb.ToUint128(1), Timestamp: ts}
	}
	// USD has transfers at 50, 30 and 10; AED at 40 and 20. Each wallet returns at most filter.Limit, newest first.
	inTB := map[tb.Uint128][]tb.Transfer{
		usd: {transfer(usd, 50), transfer(usd, 30), transfer(usd, 10)},
		aed: {transfer(aed, 40), transfer(aed, 20)},
	}
	store := &fakeStore{
		accountWallets: func(_ context.Context, address, currency string) (Account, error) {
			if address != "rider:1:wallet" {
				return Account{}, ErrAccountNotFound
			}
			acct := Account{Address: address, Type: TypeLiability}
			for _, w := range []Wallet{{Currency: "AED", ID: aed}, {Currency: "USD", ID: usd}} {
				if currency == "" || w.Currency == currency {
					acct.Wallets = append(acct.Wallets, w)
				}
			}
			return acct, nil
		},
		walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
			return map[tb.Uint128]WalletKey{usd: {"rider:1:wallet", "USD"}, aed: {"rider:1:wallet", "AED"}, world: {"world", "USD"}}, nil
		},
	}

	tests := []struct {
		name       string
		p          AccountTransfersParams
		wantTimes  []int64
		wantMore   bool
		wantFilter tb.AccountFilter // AccountID is not compared
		wantErr    error
	}{
		{name: "merged newest first", p: AccountTransfersParams{Address: "rider:1:wallet"}, wantTimes: []int64{50, 40, 30, 20, 10}, wantFilter: tb.AccountFilter{Limit: defaultListLimit + 1}},
		{name: "page", p: AccountTransfersParams{Address: "rider:1:wallet", Limit: 2}, wantTimes: []int64{50, 40}, wantMore: true, wantFilter: tb.AccountFilter{Limit: 3}},
		{name: "one currency", p: AccountTransfersParams{Address: "rider:1:wallet", Currency: "AED"}, wantTimes: []int64{40, 20}, wantFilter: tb.AccountFilter{Limit: defaultListLimit + 1}},
		{
			name:       "range",
			p:          AccountTransfersParams{Address: "rider:1:wallet", From: time.Unix(0, 20), Cursor: time.Unix(0, 41), Limit: 5},
			wantTimes:  []int64{40, 30, 20},
			wantFilter: tb.AccountFilter{TimestampMin: 20, TimestampMax: 40, Limit: 6},
		},
		{name: "empty range", p: AccountTransfersParams{Address: "rider:1:wallet", From: time.Unix(0, 30), Cursor: time.Unix(0, 30)}, wantTimes: []int64{}},
		{name: "unknown account", p: AccountTransfersParams{Address: "nobody"}, wantErr: ErrAccountNotFound},
		{name: "bad limit", p: AccountTransfersParams{Address: "rider:1:wallet", Limit: maxListLimit + 1}, wantErr: ErrInvalid},
		{name: "bad time", p: AccountTransfersParams{Address: "rider:1:wallet", Cursor: time.Unix(-1, 0)}, wantErr: ErrInvalid},
	}
	flags := tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filters []tb.AccountFilter
			tbc := &fakeTB{accountTransfers: func(f tb.AccountFilter) ([]tb.Transfer, error) {
				filters = append(filters, f)
				var out []tb.Transfer
				for _, tr := range inTB[f.AccountID] {
					if tr.Timestamp >= f.TimestampMin && (f.TimestampMax == 0 || tr.Timestamp <= f.TimestampMax) && len(out) < int(f.Limit) {
						out = append(out, tr)
					}
				}
				return out, nil
			}}
			got, more, err := New(store, tbc, tbc).AccountTransfers(context.Background(), tt.p)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			var times []int64
			for _, g := range got {
				times = append(times, g.Timestamp.UnixNano())
				if g.Debit != "world" || g.Credit != "rider:1:wallet" {
					t.Fatalf("transfer = %+v; want world to rider:1:wallet", g)
				}
			}
			if fmt.Sprint(times) != fmt.Sprint(tt.wantTimes) || more != tt.wantMore {
				t.Fatalf("timestamps = %v, more %v; want %v, %v", times, more, tt.wantTimes, tt.wantMore)
			}
			for _, f := range filters {
				f.AccountID = tb.Uint128{}
				want := tt.wantFilter
				want.Flags = flags
				if f != want {
					t.Fatalf("filter = %+v, want %+v", f, want)
				}
			}
		})
	}
}

func TestAccountTransfersPattern(t *testing.T) {
	wallet, bonus, world := tb.ID(), tb.ID(), tb.ID()
	// The move from wallet to bonus at 30 is in both wallets' histories.
	pay := tb.Transfer{ID: tb.ToUint128(1), DebitAccountID: world, CreditAccountID: wallet, Amount: tb.ToUint128(5), Timestamp: 10}
	move := tb.Transfer{ID: tb.ToUint128(2), DebitAccountID: wallet, CreditAccountID: bonus, Amount: tb.ToUint128(1), Timestamp: 30}
	inTB := map[tb.Uint128][]tb.Transfer{wallet: {move, pay}, bonus: {move}}

	tests := []struct {
		name      string
		tooMany   bool
		wantIDs   []string
		wantLimit int
		wantErr   error
	}{
		{name: "merged once", wantIDs: []string{"2", "1"}, wantLimit: maxPatternTransferWallets},
		{name: "too many wallets", tooMany: true, wantErr: ErrPatternTooBroad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRe string
			var gotLimit int
			store := &fakeStore{
				patternWallets: func(_ context.Context, re, _ string, limit int) ([]AddressWallet, bool, error) {
					gotRe, gotLimit = re, limit
					if tt.tooMany {
						return nil, true, nil
					}
					return []AddressWallet{
						{Address: "rider:1:bonus", Type: TypeLiability, Wallet: Wallet{Currency: "USD", ID: bonus}},
						{Address: "rider:1:wallet", Type: TypeLiability, Wallet: Wallet{Currency: "USD", ID: wallet}},
					}, false, nil
				},
				walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
					return map[tb.Uint128]WalletKey{wallet: {"rider:1:wallet", "USD"}, bonus: {"rider:1:bonus", "USD"}, world: {"world", "USD"}}, nil
				},
			}
			tbc := &fakeTB{accountTransfers: func(f tb.AccountFilter) ([]tb.Transfer, error) { return inTB[f.AccountID], nil }}
			got, _, err := New(store, tbc, tbc).AccountTransfers(context.Background(), AccountTransfersParams{Address: "rider:1:*"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if gotRe != "^rider:1:[^:]+$" {
				t.Fatalf("regex = %q", gotRe)
			}
			if err != nil {
				return
			}
			var ids []string
			for _, g := range got {
				ids = append(ids, g.ID.BigInt().String())
			}
			if fmt.Sprint(ids) != fmt.Sprint(tt.wantIDs) || gotLimit != tt.wantLimit {
				t.Fatalf("ids = %v, wallet limit %d; want %v, %d", ids, gotLimit, tt.wantIDs, tt.wantLimit)
			}
		})
	}
}
