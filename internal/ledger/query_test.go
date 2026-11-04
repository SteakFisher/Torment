package ledger

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
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
	rows := []sqlc.ListAccountsRow{
		{Address: "rider:1:bonus", Type: sqlc.AccountTypeLiability},
		{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability, Currencies: []string{"AED", "USD"}, Precisions: []int16{2, 3}},
		{Address: "rider:2:wallet", Type: sqlc.AccountTypeLiability},
	}
	tests := []struct {
		name     string
		limit    int
		wantLim  int32
		wantMore bool
		wantN    int
		wantErr  error
	}{
		{name: "default limit", limit: 0, wantLim: defaultListLimit + 1, wantN: 3},
		{name: "more", limit: 2, wantLim: 3, wantMore: true, wantN: 2},
		{name: "exact", limit: 3, wantLim: 4, wantN: 3},
		{name: "negative", limit: -1, wantErr: ErrInvalid},
		{name: "too large", limit: maxListLimit + 1, wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got sqlc.ListAccountsParams
			q := &fakeQuerier{listAccounts: func(_ context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error) {
				got = arg
				return rows[:min(len(rows), int(arg.Lim))], nil
			}}
			accounts, more, err := New(q, nil).ListAccounts(context.Background(), "rider:*:*", "rider:0", tt.limit)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != (sqlc.ListAccountsParams{Pattern: "^rider:[^:]+:[^:]+$", After: "rider:0", Lim: tt.wantLim}) {
				t.Fatalf("query params = %+v", got)
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
	wallets := []sqlc.ListWalletsByPatternRow{
		{Address: "rider:1:bonus", Type: sqlc.AccountTypeLiability, Currency: "USD", Precision: 2, TbID: idToBytes(ids[0])},
		{Address: "rider:1:card", Type: sqlc.AccountTypeAsset, Currency: "USD", Precision: 2, TbID: idToBytes(ids[1])},
		{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability, Currency: "AED", Precision: 2, TbID: idToBytes(ids[2])},
		{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability, Currency: "USD", Precision: 2, TbID: idToBytes(ids[3])},
	}
	// every account has 10 debited and 40 credited, except AED which has no TigerBeetle account
	inTB := []tb.Account{
		{ID: ids[0], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
		{ID: ids[1], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
		{ID: ids[3], DebitsPosted: tb.ToUint128(10), CreditsPosted: tb.ToUint128(40)},
	}
	var got sqlc.ListWalletsByPatternParams
	q := &fakeQuerier{listWalletsByPattern: func(_ context.Context, arg sqlc.ListWalletsByPatternParams) ([]sqlc.ListWalletsByPatternRow, error) {
		got = arg
		return wallets, nil
	}}
	tbc := &fakeTB{lookupAccounts: func([]tb.Uint128) ([]tb.Account, error) { return inTB, nil }}

	totals, accounts, err := New(q, tbc).PatternBalances(context.Background(), "rider:1:*", "", time.Time{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != (sqlc.ListWalletsByPatternParams{Pattern: "^rider:1:[^:]+$", Lim: maxPatternWallets + 1}) {
		t.Fatalf("query params = %+v", got)
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

	tooMany := make([]sqlc.ListWalletsByPatternRow, maxPatternWallets+1)
	q.listWalletsByPattern = func(context.Context, sqlc.ListWalletsByPatternParams) ([]sqlc.ListWalletsByPatternRow, error) {
		return tooMany, nil
	}
	if _, _, err := New(q, tbc).PatternBalances(context.Background(), "*", "", time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many wallets err = %v, want ErrInvalid", err)
	}
	for _, c := range []struct {
		pattern string
		at      time.Time
	}{{"rider:.*", time.Time{}}, {"rider", time.Unix(-1, 0)}} {
		if _, _, err := New(q, tbc).PatternBalances(context.Background(), c.pattern, "", c.at); !errors.Is(err, ErrInvalid) {
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
	q := &fakeQuerier{
		getAccount: func(_ context.Context, address string) (sqlc.Account, error) {
			if address != "rider:1:wallet" {
				return sqlc.Account{}, pgx.ErrNoRows
			}
			return sqlc.Account{ID: 7, Address: address, Type: sqlc.AccountTypeLiability}, nil
		},
		listWallets: func(context.Context, int64) ([]sqlc.ListWalletsRow, error) {
			return []sqlc.ListWalletsRow{{TbID: idToBytes(aed), Currency: "AED"}, {TbID: idToBytes(usd), Currency: "USD"}}, nil
		},
		getWalletsByTBIDs: func(context.Context, [][]byte) ([]sqlc.GetWalletsByTBIDsRow, error) {
			return []sqlc.GetWalletsByTBIDsRow{
				{TbID: idToBytes(usd), Address: "rider:1:wallet", Currency: "USD"},
				{TbID: idToBytes(aed), Address: "rider:1:wallet", Currency: "AED"},
				{TbID: idToBytes(world), Address: "world", Currency: "USD"},
			}, nil
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
			p:          AccountTransfersParams{Address: "rider:1:wallet", From: time.Unix(0, 20), Before: time.Unix(0, 41), Limit: 5},
			wantTimes:  []int64{40, 30, 20},
			wantFilter: tb.AccountFilter{TimestampMin: 20, TimestampMax: 40, Limit: 6},
		},
		{name: "empty range", p: AccountTransfersParams{Address: "rider:1:wallet", From: time.Unix(0, 30), Before: time.Unix(0, 30)}, wantTimes: []int64{}},
		{name: "unknown account", p: AccountTransfersParams{Address: "nobody"}, wantErr: ErrNotFound},
		{name: "bad limit", p: AccountTransfersParams{Address: "rider:1:wallet", Limit: maxListLimit + 1}, wantErr: ErrInvalid},
		{name: "bad time", p: AccountTransfersParams{Address: "rider:1:wallet", Before: time.Unix(-1, 0)}, wantErr: ErrInvalid},
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
			got, more, err := New(q, tbc).AccountTransfers(context.Background(), tt.p)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			var times []int64
			for _, g := range got {
				times = append(times, g.Timestamp.UnixNano())
				if g.Source != "world" || g.Destination != "rider:1:wallet" {
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
