package ledger

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestGroupTransfers(t *testing.T) {
	world, rider, orphan := tb.ID(), tb.ID(), tb.ID()
	pay := tb.Transfer{ID: tb.ID(), DebitAccountID: world, CreditAccountID: rider, Amount: tb.ToUint128(100), Code: 5, Timestamp: 1_000_000_000}
	refund := tb.Transfer{ID: tb.ID(), DebitAccountID: rider, CreditAccountID: world, Amount: tb.ToUint128(30), Code: 6, Timestamp: 2_000_000_000}
	wallets := map[tb.Uint128]AddressWallet{world: named("world", "USD"), rider: named("rider:1:wallet", "USD")}

	tests := []struct {
		name      string
		params    GroupTransfersParams
		found     []tb.Transfer
		wantErr   error
		wantOther bool // any error other than wantErr
		want      []TransferRecord
		wantMore  bool
		wantQuery bool
		// wantLimit, wantMax and wantCode are the TigerBeetle filter's Limit, TimestampMax and Code
		wantLimit uint32
		wantMax   uint64
		wantCode  uint16
	}{
		{name: "malformed", params: GroupTransfersParams{GroupID: "x"}, wantErr: ErrInvalid},
		{name: "zero", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000000"}, wantErr: ErrInvalid},
		{name: "limit too big", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009", Limit: maxListLimit + 1}, wantErr: ErrInvalid},
		{name: "no transfers", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009"}, want: []TransferRecord{}, wantLimit: defaultListLimit + 1},
		{
			name: "page with more", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009", Cursor: time.Unix(3, 0), Limit: 1},
			found: []tb.Transfer{refund, pay}, wantQuery: true, wantMore: true, wantLimit: 2, wantMax: 3_000_000_000 - 1,
			want: []TransferRecord{
				{ID: refund.ID, Debit: "rider:1:wallet", Credit: "world", Currency: "USD", Precision: 2, Amount: refund.Amount.BigInt(), Kind: "refund", Timestamp: time.Unix(2, 0).UTC()},
			},
		},
		{
			name: "one kind", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009", Kind: "deposit"},
			found: []tb.Transfer{pay}, wantQuery: true, wantLimit: defaultListLimit + 1, wantCode: 5,
			want: []TransferRecord{
				{ID: pay.ID, Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Precision: 2, Amount: pay.Amount.BigInt(), Kind: "deposit", Timestamp: time.Unix(1, 0).UTC()},
			},
		},
		{name: "unknown kind", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009", Kind: "payout"}, wantErr: ErrKindNotFound},
		{name: "code with no kind", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009"}, found: []tb.Transfer{{ID: tb.ID(), DebitAccountID: world, CreditAccountID: rider, Code: 9}}, wantQuery: true, wantOther: true},
		{
			name: "transfers", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009"}, found: []tb.Transfer{pay, refund}, wantQuery: true, wantLimit: defaultListLimit + 1,
			want: []TransferRecord{
				{ID: pay.ID, Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Precision: 2, Amount: pay.Amount.BigInt(), Kind: "deposit", Timestamp: time.Unix(1, 0).UTC()},
				{ID: refund.ID, Debit: "rider:1:wallet", Credit: "world", Currency: "USD", Precision: 2, Amount: refund.Amount.BigInt(), Kind: "refund", Timestamp: time.Unix(2, 0).UTC()},
			},
		},
		{name: "account with no wallet", params: GroupTransfersParams{GroupID: "00000000-0000-0000-0000-000000000009"}, found: []tb.Transfer{{ID: tb.ID(), DebitAccountID: orphan, CreditAccountID: rider, Code: 5}}, wantQuery: true, wantOther: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filter tb.QueryFilter
			tbc := &fakeTB{queryTransfers: func(f tb.QueryFilter) ([]tb.Transfer, error) {
				filter = f
				return tt.found, nil
			}}
			queried := false
			store := withKinds(&fakeStore{walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]AddressWallet, error) {
				queried = true
				return wallets, nil
			}})

			got, more, err := New(store, tbc, tbc).GroupTransfers(context.Background(), tt.params)
			if tt.wantOther {
				if err == nil {
					t.Fatal("err = nil, want an error")
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if queried != tt.wantQuery {
				t.Fatalf("WalletsByTBIDs called = %v, want %v", queried, tt.wantQuery)
			}
			if err != nil {
				return
			}
			if filter.UserData128 != tb.ToUint128(9) || filter.Limit != tt.wantLimit || filter.TimestampMax != tt.wantMax || filter.Code != tt.wantCode || filter.Flags != (tb.QueryFilterFlags{Reversed: true}).ToUint32() {
				t.Fatalf("filter = %+v; want user data 128 = 9, limit %d, timestamp max %d, code %d, reversed", filter, tt.wantLimit, tt.wantMax, tt.wantCode)
			}
			if more != tt.wantMore {
				t.Fatalf("more = %v, want %v", more, tt.wantMore)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d transfers, want %d", len(got), len(tt.want))
			}
			for i, g := range got {
				w := tt.want[i]
				if g.ID != w.ID || g.Debit != w.Debit || g.Credit != w.Credit || g.Currency != w.Currency || g.Amount.Cmp(w.Amount) != 0 || g.Kind != w.Kind || !g.Timestamp.Equal(w.Timestamp) {
					t.Fatalf("transfer %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestAccountTransfers(t *testing.T) {
	usd, aed, world := tb.ID(), tb.ID(), tb.ID()
	// transfer returns a transfer between the rider's wallet and world with the given timestamp
	transfer := func(wallet tb.Uint128, ts uint64) tb.Transfer {
		return tb.Transfer{ID: tb.ToUint128(ts), DebitAccountID: world, CreditAccountID: wallet, Amount: tb.ToUint128(1), Code: 5, Timestamp: ts}
	}
	// USD has transfers at 50, 30 and 10; AED at 40 and 20. Each wallet returns at most filter.Limit, newest first.
	inTB := map[tb.Uint128][]tb.Transfer{
		usd: {transfer(usd, 50), transfer(usd, 30), transfer(usd, 10)},
		aed: {transfer(aed, 40), transfer(aed, 20)},
	}
	store := withKinds(&fakeStore{
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
		walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]AddressWallet, error) {
			return map[tb.Uint128]AddressWallet{usd: named("rider:1:wallet", "USD"), aed: named("rider:1:wallet", "AED"), world: named("world", "USD")}, nil
		},
	})

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
		{name: "one kind", p: AccountTransfersParams{Address: "rider:1:wallet", Kind: "deposit"}, wantTimes: []int64{50, 40, 30, 20, 10}, wantFilter: tb.AccountFilter{Code: 5, Limit: defaultListLimit + 1}},
		{name: "unknown kind", p: AccountTransfersParams{Address: "rider:1:wallet", Kind: "payout"}, wantErr: ErrKindNotFound},
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
				if g.Debit != "world" || g.Credit != "rider:1:wallet" || g.Kind != "deposit" {
					t.Fatalf("transfer = %+v; want a deposit from world to rider:1:wallet", g)
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
	pay := tb.Transfer{ID: tb.ToUint128(1), DebitAccountID: world, CreditAccountID: wallet, Amount: tb.ToUint128(5), Code: 5, Timestamp: 10}
	move := tb.Transfer{ID: tb.ToUint128(2), DebitAccountID: wallet, CreditAccountID: bonus, Amount: tb.ToUint128(1), Code: 5, Timestamp: 30}
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
			store := withKinds(&fakeStore{
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
				walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]AddressWallet, error) {
					return map[tb.Uint128]AddressWallet{wallet: named("rider:1:wallet", "USD"), bonus: named("rider:1:bonus", "USD"), world: named("world", "USD")}, nil
				},
			})
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
