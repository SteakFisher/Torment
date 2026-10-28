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

	plain, err := s.Transfer(ctx, []Posting{{Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100"}})
	if err != nil {
		t.Fatalf("error transferring: %v", err)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	_, err = s.Transfer(ctx, []Posting{{Source: "rider:1:wallet", Destination: "world", Currency: "USD", Amount: "150"}})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("overdraw err = %v, want ErrInsufficientFunds", err)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	group := tb.ID()
	grouped, err := s.Transfer(ctx, []Posting{{Source: "rider:1:wallet", Destination: "world", Currency: "USD", Amount: "40", GroupID: group.BigInt().String()}})
	if err != nil {
		t.Fatalf("error transferring with group: %v", err)
	}
	if grouped[0].GroupID != group {
		t.Fatalf("returned group id = %s, want %s", grouped[0].GroupID.BigInt(), group.BigInt())
	}

	found, err := testTB.LookupTransfers([]tb.Uint128{plain[0].ID, grouped[0].ID})
	if err != nil || len(found) != 2 {
		t.Fatalf("error looking up transfers: %v, found %d", err, len(found))
	}
	want := map[tb.Uint128]tb.Uint128{plain[0].ID: plain[0].ID, grouped[0].ID: group}
	for _, tr := range found {
		if tr.UserData128 != want[tr.ID] {
			t.Fatalf("transfer %s user data 128 = %s, want %s", tr.ID.BigInt(), tr.UserData128.BigInt(), want[tr.ID].BigInt())
		}
	}
}
