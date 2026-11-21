package ledger_test

import (
	"context"
	"fmt"
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

	group := ledger.FormatID(tb.ID())
	pay := ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: "100", GroupID: group, Kind: "test"}
	refund := ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: "rider:1:wallet", Credit: "world", Currency: "USD", Amount: "30", GroupID: group, Kind: "test"}
	other := ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: "5", Kind: "test"}
	got, err := s.Transfer(ctx, [][]ledger.Posting{{pay}, {refund}, {other}})
	if err != nil {
		t.Fatalf("error transferring: %v", err)
	}
	for _, chain := range got {
		if chain[0].Err != nil {
			t.Fatalf("transfer = %+v; want created", chain[0])
		}
	}

	transfers, more, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group})
	if err != nil || more {
		t.Fatalf("error getting group transfers: %v", err)
	}
	if len(transfers) != 2 {
		t.Fatalf("got %d transfers, want 2: %+v", len(transfers), transfers)
	}
	for i, want := range []ledger.Posting{refund, pay} {
		g := transfers[i]
		if ledger.FormatID(g.ID) != want.ID || g.Debit != want.Debit || g.Credit != want.Credit || g.Currency != "USD" || g.Precision != 2 || g.Amount.String() != want.Amount {
			t.Fatalf("transfer %d = %+v, want %+v", i, g, want)
		}
	}
	if !transfers[0].Timestamp.After(transfers[1].Timestamp) {
		t.Fatalf("timestamps %v, %v; want newest first", transfers[0].Timestamp, transfers[1].Timestamp)
	}

	// Paging one at a time returns the same two transfers.
	first, more, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group, Limit: 1})
	if err != nil || !more || len(first) != 1 || first[0].ID != transfers[0].ID {
		t.Fatalf("first page = %+v, more %v, err %v; want the refund and more", first, more, err)
	}
	second, more, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group, Cursor: first[0].Timestamp, Limit: 1})
	if err != nil || more || len(second) != 1 || second[0].ID != transfers[1].ID {
		t.Fatalf("second page = %+v, more %v, err %v; want the payment and no more", second, more, err)
	}

	unused, _, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: ledger.FormatID(tb.ID())})
	if err != nil || len(unused) != 0 {
		t.Fatalf("unused group = %+v, err %v; want none", unused, err)
	}
}

func TestIntegrationAccountTransfers(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	for _, code := range []string{"USD/2", "AED/2"} {
		if _, _, err := s.RegisterCurrency(ctx, code); err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
	}
	for _, p := range []ledger.RegisterAccountParams{
		{Address: "world", Type: "equity", AllowNegative: true},
		{Address: "rider:1:wallet", Type: "liability"},
		{Address: "rider:2:wallet", Type: "liability"},
	} {
		if _, _, err := s.RegisterAccount(ctx, p); err != nil {
			t.Fatalf("%s: %v", p.Address, err)
		}
	}
	// pay sends one posting and returns its id
	pay := func(debit, credit, currency, amount string) string {
		t.Helper()
		id := ledger.FormatID(tb.ID())
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: id, Debit: debit, Credit: credit, Currency: currency, Amount: amount, Kind: "test"}}})
		if err != nil || got[0][0].Err != nil {
			t.Fatalf("error transferring: %v, %+v", err, got)
		}
		return id
	}
	a := pay("world", "rider:1:wallet", "USD", "100")
	b := pay("world", "rider:1:wallet", "AED", "40")
	pay("world", "rider:2:wallet", "USD", "7") // not rider 1's
	c := pay("rider:1:wallet", "rider:2:wallet", "USD", "30")

	// ids lists one page of rider 1's transfers by id, newest first
	ids := func(p ledger.AccountTransfersParams) ([]string, []ledger.TransferRecord, bool) {
		t.Helper()
		p.Address = "rider:1:wallet"
		got, more, err := s.AccountTransfers(ctx, p)
		if err != nil {
			t.Fatalf("error getting transfers: %v", err)
		}
		var out []string
		for _, g := range got {
			out = append(out, ledger.FormatID(g.ID))
		}
		return out, got, more
	}

	all, records, more := ids(ledger.AccountTransfersParams{})
	if fmt.Sprint(all) != fmt.Sprint([]string{c, b, a}) || more {
		t.Fatalf("all = %v, more %v; want %v", all, more, []string{c, b, a})
	}
	if r := records[0]; r.Debit != "rider:1:wallet" || r.Credit != "rider:2:wallet" || r.Currency != "USD" || r.Amount.Int64() != 30 || ledger.FormatID(r.GroupID) != c {
		t.Fatalf("newest = %+v", r)
	}

	page1, records, more := ids(ledger.AccountTransfersParams{Limit: 2})
	if fmt.Sprint(page1) != fmt.Sprint([]string{c, b}) || !more {
		t.Fatalf("page 1 = %v, more %v", page1, more)
	}
	page2, _, more := ids(ledger.AccountTransfersParams{Limit: 2, Cursor: records[1].Timestamp})
	if fmt.Sprint(page2) != fmt.Sprint([]string{a}) || more {
		t.Fatalf("page 2 = %v, more %v", page2, more)
	}

	if usd, _, _ := ids(ledger.AccountTransfersParams{Currency: "USD"}); fmt.Sprint(usd) != fmt.Sprint([]string{c, a}) {
		t.Fatalf("USD = %v", usd)
	}
	_, all3, _ := ids(ledger.AccountTransfersParams{})
	if since, _, _ := ids(ledger.AccountTransfersParams{From: all3[1].Timestamp}); fmt.Sprint(since) != fmt.Sprint([]string{c, b}) {
		t.Fatalf("from b = %v", since)
	}
}
