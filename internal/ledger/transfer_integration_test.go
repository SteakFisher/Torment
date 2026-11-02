package ledger

import (
	"context"
	"errors"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestIntegrationTransfer(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	if _, err := s.RegisterCurrency(ctx, "USD/2"); err != nil {
		t.Fatalf("error registering currency: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("world: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("rider: %v", err)
	}
	checkUSD := func(want map[string]int64) {
		t.Helper()
		for address, amount := range want {
			got, err := s.Balances(ctx, address)
			if err != nil || len(got) != 1 || got[0].Amount.Int64() != amount {
				t.Fatalf("%s balances = %+v, err %v; want USD %d", address, got, err, amount)
			}
		}
	}

	// posting returns one posting with a new id
	posting := func(source, destination, amount, group string) Posting {
		return Posting{ID: tb.ID().BigInt().String(), Source: source, Destination: destination, Currency: "USD", Amount: amount, GroupID: group}
	}

	plain := posting("world", "rider:1:wallet", "100", "")
	first, created, err := s.Transfer(ctx, []Posting{plain})
	if err != nil || !created {
		t.Fatalf("transfer: created %v, err %v; want created", created, err)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	again, created, err := s.Transfer(ctx, []Posting{plain})
	if err != nil || created || again[0] != first[0] {
		t.Fatalf("same transfer again = %+v, created %v, err %v; want %+v not created", again, created, err, first)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	changed := plain
	changed.Amount = "101"
	if _, _, err := s.Transfer(ctx, []Posting{changed}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same id with different amount err = %v, want ErrConflict", err)
	}

	overdraw := posting("rider:1:wallet", "world", "150", "")
	if _, _, err := s.Transfer(ctx, []Posting{overdraw}); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("overdraw err = %v, want ErrInsufficientFunds", err)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})
	if _, _, err := s.Transfer(ctx, []Posting{posting("world", "rider:1:wallet", "100", "")}); err != nil {
		t.Fatalf("error topping up: %v", err)
	}
	if _, _, err := s.Transfer(ctx, []Posting{overdraw}); !errors.Is(err, ErrConflict) {
		t.Fatalf("failed id after top-up err = %v, want ErrConflict", err)
	}
	checkUSD(map[string]int64{"world": -200, "rider:1:wallet": 200})

	group := tb.ID()
	grouped, _, err := s.Transfer(ctx, []Posting{posting("rider:1:wallet", "world", "40", group.BigInt().String())})
	if err != nil {
		t.Fatalf("error transferring with group: %v", err)
	}
	if grouped[0].GroupID != group {
		t.Fatalf("returned group id = %s, want %s", grouped[0].GroupID.BigInt(), group.BigInt())
	}

	found, err := testTB.LookupTransfers([]tb.Uint128{first[0].ID, grouped[0].ID})
	if err != nil || len(found) != 2 {
		t.Fatalf("error looking up transfers: %v, found %d", err, len(found))
	}
	want := map[tb.Uint128]tb.Uint128{first[0].ID: first[0].ID, grouped[0].ID: group}
	for _, tr := range found {
		if tr.UserData128 != want[tr.ID] {
			t.Fatalf("transfer %s user data 128 = %s, want %s", tr.ID.BigInt(), tr.UserData128.BigInt(), want[tr.ID].BigInt())
		}
	}
}
