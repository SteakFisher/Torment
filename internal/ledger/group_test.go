package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestGroupTransfers(t *testing.T) {
	world, rider, orphan := tb.ID(), tb.ID(), tb.ID()
	pay := tb.Transfer{ID: tb.ID(), DebitAccountID: world, CreditAccountID: rider, Amount: tb.ToUint128(100), Timestamp: 1_000_000_000}
	refund := tb.Transfer{ID: tb.ID(), DebitAccountID: rider, CreditAccountID: world, Amount: tb.ToUint128(30), Timestamp: 2_000_000_000}
	wallets := map[tb.Uint128]WalletKey{world: {"world", "USD"}, rider: {"rider:1:wallet", "USD"}}

	tests := []struct {
		name      string
		params    GroupTransfersParams
		found     []tb.Transfer
		wantErr   error
		wantOther bool // any error other than wantErr
		want      []TransferRecord
		wantMore  bool
		wantQuery bool
		// wantLimit and wantMax are the TigerBeetle filter's Limit and TimestampMax
		wantLimit uint32
		wantMax   uint64
	}{
		{name: "malformed", params: GroupTransfersParams{GroupID: "x"}, wantErr: ErrInvalid},
		{name: "zero", params: GroupTransfersParams{GroupID: "0"}, wantErr: ErrInvalid},
		{name: "limit too big", params: GroupTransfersParams{GroupID: "9", Limit: maxListLimit + 1}, wantErr: ErrInvalid},
		{name: "no transfers", params: GroupTransfersParams{GroupID: "9"}, want: []TransferRecord{}, wantLimit: defaultListLimit + 1},
		{
			name: "page with more", params: GroupTransfersParams{GroupID: "9", Cursor: time.Unix(3, 0), Limit: 1},
			found: []tb.Transfer{refund, pay}, wantQuery: true, wantMore: true, wantLimit: 2, wantMax: 3_000_000_000 - 1,
			want: []TransferRecord{
				{ID: refund.ID, Source: "rider:1:wallet", Destination: "world", Currency: "USD", Amount: refund.Amount.BigInt(), Timestamp: time.Unix(2, 0).UTC()},
			},
		},
		{
			name: "transfers", params: GroupTransfersParams{GroupID: "9"}, found: []tb.Transfer{pay, refund}, wantQuery: true, wantLimit: defaultListLimit + 1,
			want: []TransferRecord{
				{ID: pay.ID, Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: pay.Amount.BigInt(), Timestamp: time.Unix(1, 0).UTC()},
				{ID: refund.ID, Source: "rider:1:wallet", Destination: "world", Currency: "USD", Amount: refund.Amount.BigInt(), Timestamp: time.Unix(2, 0).UTC()},
			},
		},
		{name: "account with no wallet", params: GroupTransfersParams{GroupID: "9"}, found: []tb.Transfer{{ID: tb.ID(), DebitAccountID: orphan, CreditAccountID: rider}}, wantQuery: true, wantOther: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filter tb.QueryFilter
			tbc := &fakeTB{queryTransfers: func(f tb.QueryFilter) ([]tb.Transfer, error) {
				filter = f
				return tt.found, nil
			}}
			queried := false
			store := &fakeStore{walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
				queried = true
				return wallets, nil
			}}

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
			if filter.UserData128 != tb.ToUint128(9) || filter.Limit != tt.wantLimit || filter.TimestampMax != tt.wantMax || filter.Flags != (tb.QueryFilterFlags{Reversed: true}).ToUint32() {
				t.Fatalf("filter = %+v; want user data 128 = 9, limit %d, timestamp max %d, reversed", filter, tt.wantLimit, tt.wantMax)
			}
			if more != tt.wantMore {
				t.Fatalf("more = %v, want %v", more, tt.wantMore)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d transfers, want %d", len(got), len(tt.want))
			}
			for i, g := range got {
				w := tt.want[i]
				if g.ID != w.ID || g.Source != w.Source || g.Destination != w.Destination || g.Currency != w.Currency || g.Amount.Cmp(w.Amount) != 0 || !g.Timestamp.Equal(w.Timestamp) {
					t.Fatalf("transfer %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}
