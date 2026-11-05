package ledger_test

import (
	"context"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestIntegrationGroupTransfers(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	if _, _, err := s.RegisterCurrency(ctx, "USD/2"); err != nil {
		t.Fatalf("error registering currency: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true}); err != nil {
		t.Fatalf("world: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "rider:1:wallet", Type: "liability"}); err != nil {
		t.Fatalf("rider: %v", err)
	}

	group := tb.ID().BigInt().String()
	pay := ledger.Posting{ID: tb.ID().BigInt().String(), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100", GroupID: group}
	refund := ledger.Posting{ID: tb.ID().BigInt().String(), Source: "rider:1:wallet", Destination: "world", Currency: "USD", Amount: "30", GroupID: group}
	other := ledger.Posting{ID: tb.ID().BigInt().String(), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "5"}
	got, err := s.Transfer(ctx, [][]ledger.Posting{{pay}, {refund}, {other}})
	if err != nil {
		t.Fatalf("error transferring: %v", err)
	}
	for _, chain := range got {
		if chain[0].Err != nil {
			t.Fatalf("transfer = %+v; want created", chain[0])
		}
	}

	transfers, err := s.GroupTransfers(ctx, group)
	if err != nil {
		t.Fatalf("error getting group transfers: %v", err)
	}
	if len(transfers) != 2 {
		t.Fatalf("got %d transfers, want 2: %+v", len(transfers), transfers)
	}
	for i, want := range []ledger.Posting{refund, pay} {
		g := transfers[i]
		if g.ID.BigInt().String() != want.ID || g.Source != want.Source || g.Destination != want.Destination || g.Currency != "USD" || g.Amount.String() != want.Amount {
			t.Fatalf("transfer %d = %+v, want %+v", i, g, want)
		}
	}
	if !transfers[0].Timestamp.After(transfers[1].Timestamp) {
		t.Fatalf("timestamps %v, %v; want newest first", transfers[0].Timestamp, transfers[1].Timestamp)
	}

	unused, err := s.GroupTransfers(ctx, tb.ID().BigInt().String())
	if err != nil || len(unused) != 0 {
		t.Fatalf("unused group = %+v, err %v; want none", unused, err)
	}
}
