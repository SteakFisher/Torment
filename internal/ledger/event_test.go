package ledger

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestEvents(t *testing.T) {
	// A message captured from TigerBeetle's CDC job: world (equity) pays 100 to a rider's wallet (liability).
	sample, err := os.ReadFile("testdata/cdc_single_phase.json")
	if err != nil {
		t.Fatal(err)
	}
	id := func(s string) tb.Uint128 {
		n, ok := parseUint128(s)
		if !ok {
			t.Fatalf("bad id %q", s)
		}
		return n
	}
	transfer, world, rider := id("2169084731697126250633813240208052679"), id("2169084730993670218564254952420777486"), id("2169084731686970812380528612393717684")
	wallets := map[tb.Uint128]AddressWallet{
		world: {Address: "world", Type: TypeEquity, Wallet: Wallet{Currency: "USD", Precision: 2, ID: world}},
		rider: {Address: "rider:1:wallet", Type: TypeLiability, Wallet: Wallet{Currency: "USD", Precision: 2, ID: rider}},
	}
	edit := func(old, new string) []byte {
		if !strings.Contains(string(sample), old) {
			t.Fatalf("sample has no %q", old)
		}
		return []byte(strings.Replace(string(sample), old, new, 1))
	}

	tests := []struct {
		name    string
		raw     []byte
		wallets map[tb.Uint128]AddressWallet
		wantErr bool
	}{
		{name: "sample", raw: sample, wallets: wallets},
		{name: "small numbers as strings", raw: edit(`"amount":100`, `"amount":"100"`), wallets: wallets},
		{name: "not json", raw: []byte("{"), wallets: wallets, wantErr: true},
		{name: "negative amount", raw: edit(`"amount":100`, `"amount":-100`), wallets: wallets, wantErr: true},
		{name: "two-phase", raw: edit(`"type":"single_phase"`, `"type":"two_phase_pending"`), wallets: wallets, wantErr: true},
		{name: "account with no wallet", raw: sample, wallets: map[tb.Uint128]AddressWallet{world: wallets[world]}, wantErr: true},
		{name: "code with no kind", raw: edit(`"code":2,"flags":0`, `"code":9,"flags":0`), wallets: wallets, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				walletsByTBIDs: func(context.Context, []tb.Uint128) (map[tb.Uint128]AddressWallet, error) { return tt.wallets, nil },
				kindsByCodes: func(_ context.Context, codes []uint16) (map[uint16]Kind, error) {
					if len(codes) == 1 && codes[0] == 2 {
						return map[uint16]Kind{2: {Name: "top_up", Code: 2}}, nil
					}
					return map[uint16]Kind{}, nil
				},
			}
			got, err := New(store, nil, nil).Events(context.Background(), [][]byte{tt.raw})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want an error: %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			e := got[0]
			if len(got) != 1 || e.ID != transfer || e.GroupID != transfer || e.Debit != "world" || e.Credit != "rider:1:wallet" ||
				e.Currency != "USD" || e.Precision != 2 || e.Amount.String() != "100" || e.Kind != "top_up" ||
				!e.Timestamp.Equal(time.Unix(0, 1794224837045092322)) {
				t.Fatalf("event = %+v", e)
			}
			// world is credit-normal, so paying out 100 leaves it at -100.
			if e.DebitBalance.String() != "-100" || e.CreditBalance.String() != "100" {
				t.Fatalf("balances = %s, %s; want -100, 100", e.DebitBalance, e.CreditBalance)
			}
		})
	}
}
