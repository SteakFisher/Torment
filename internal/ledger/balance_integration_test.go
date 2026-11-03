package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestIntegrationBalances(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	for _, code := range []string{"USD/2", "AED/2"} {
		if _, err := s.RegisterCurrency(ctx, code); err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
	}
	if _, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("world: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}}); err != nil {
		t.Fatalf("rider: %v", err)
	}

	// pay moves amount of USD from world to the rider and returns TigerBeetle's timestamp for it
	pay := func(amount string) time.Time {
		t.Helper()
		got, err := s.Transfer(ctx, [][]Posting{{{ID: tb.ID().BigInt().String(), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: amount}}})
		if err != nil || got[0][0].Err != nil {
			t.Fatalf("error transferring: %v, %+v", err, got)
		}
		found, err := testTB.LookupTransfers([]tb.Uint128{got[0][0].ID})
		if err != nil || len(found) != 1 {
			t.Fatalf("error looking up transfer: %v, found %d", err, len(found))
		}
		return time.Unix(0, int64(found[0].Timestamp))
	}
	first := pay("100")

	want := map[string]map[string]int64{
		"world":          {"USD": -100},
		"rider:1:wallet": {"AED": 0, "USD": 100},
	}
	for address, amounts := range want {
		got, err := s.Balances(ctx, address, time.Time{})
		if err != nil || len(got) != len(amounts) {
			t.Fatalf("%s balances: %+v, err %v", address, got, err)
		}
		for _, b := range got {
			if b.Amount.Int64() != amounts[b.Currency] {
				t.Fatalf("%s %s balance = %s, want %d", address, b.Currency, b.Amount, amounts[b.Currency])
			}
		}
	}

	second := pay("50")
	// riderUSD returns the rider's USD balance at at
	riderUSD := func(at time.Time) int64 {
		t.Helper()
		got, err := s.Balances(ctx, "rider:1:wallet", at)
		if err != nil || len(got) != 2 || got[0].Amount.Sign() != 0 {
			t.Fatalf("balances at %v: %+v, err %v; want AED 0", at, got, err)
		}
		return got[1].Amount.Int64()
	}
	for _, c := range []struct {
		at   time.Time
		want int64
	}{
		{first.Add(-1), 0},
		{first, 100},
		{second.Add(-1), 100},
		{second, 150},
		{time.Time{}, 150},
	} {
		if got := riderUSD(c.at); got != c.want {
			t.Fatalf("rider USD at %v = %d, want %d", c.at, got, c.want)
		}
	}

	if _, err := s.Balances(ctx, "nobody", time.Time{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown address err = %v, want ErrNotFound", err)
	}
}
