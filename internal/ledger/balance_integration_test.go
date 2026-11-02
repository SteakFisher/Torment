package ledger

import (
	"context"
	"errors"
	"testing"

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

	if _, _, err := s.Transfer(ctx, []Posting{{ID: tb.ID().BigInt().String(), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100"}}); err != nil {
		t.Fatalf("error transferring: %v", err)
	}

	want := map[string]map[string]int64{
		"world":          {"USD": -100},
		"rider:1:wallet": {"AED": 0, "USD": 100},
	}
	for address, amounts := range want {
		got, err := s.Balances(ctx, address)
		if err != nil || len(got) != len(amounts) {
			t.Fatalf("%s balances: %+v, err %v", address, got, err)
		}
		for _, b := range got {
			if b.Amount.Int64() != amounts[b.Currency] {
				t.Fatalf("%s %s balance = %s, want %d", address, b.Currency, b.Amount, amounts[b.Currency])
			}
		}
	}

	if _, err := s.Balances(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown address err = %v, want ErrNotFound", err)
	}
}
