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
	world, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}})
	if err != nil {
		t.Fatalf("world: %v", err)
	}
	rider, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}})
	if err != nil {
		t.Fatalf("rider: %v", err)
	}

	// no transfer API yet, so move 100 from world to the rider directly
	results, err := testTB.CreateTransfers([]tb.Transfer{{
		ID:              tb.ID(),
		DebitAccountID:  world.Wallets[0].ID,
		CreditAccountID: rider.Wallets[0].ID,
		Amount:          tb.ToUint128(100),
		Ledger:          lookupTBAccount(t, world.Wallets[0].ID).Ledger,
		Code:            1,
	}})
	if err != nil || len(results) != 1 || results[0].Status != tb.TransferCreated {
		t.Fatalf("error creating transfer: %v %+v", err, results)
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
