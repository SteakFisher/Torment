package ledger_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestIntegrationKinds(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	deposit, created, err := s.RegisterKind(ctx, "deposit")
	if err != nil || !created {
		t.Fatalf("register deposit: created %v, err %v; want created", created, err)
	}
	again, created, err := s.RegisterKind(ctx, "deposit")
	if err != nil || created || again != deposit {
		t.Fatalf("register deposit again = %+v, created %v, err %v; want %+v, not created", again, created, err, deposit)
	}
	// The replay takes no code, so the next kind gets the next one.
	fee, _, err := s.RegisterKind(ctx, "fee")
	if err != nil || fee.Code != deposit.Code+1 {
		t.Fatalf("register fee = %+v, err %v; want code %d", fee, err, deposit.Code+1)
	}
	all, err := s.Kinds(ctx)
	if err != nil || !slices.Equal(all, []ledger.Kind{deposit, fee, {Name: "test", Code: deposit.Code - 1}}) {
		t.Fatalf("kinds = %v, err %v", all, err)
	}

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
	posting := func(amount, kind string) ledger.Posting {
		return ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: amount, GroupID: group, Kind: kind}
	}
	if _, err := s.Transfer(ctx, [][]ledger.Posting{{posting("1", "payout")}}); !errors.Is(err, ledger.ErrKindNotFound) {
		t.Fatalf("unregistered kind err = %v, want ErrKindNotFound", err)
	}
	got, err := s.Transfer(ctx, [][]ledger.Posting{{posting("100", "deposit"), posting("2", "fee")}, {posting("50", "deposit")}})
	if err != nil || got[0][0].Err != nil || got[0][1].Err != nil || got[1][0].Err != nil {
		t.Fatalf("transfer = %+v, err %v; want created", got, err)
	}

	// kinds returns the kind of each transfer, newest first
	kinds := func(transfers []ledger.TransferRecord) []string {
		var out []string
		for _, tr := range transfers {
			out = append(out, tr.Kind)
		}
		return out
	}
	transfers, _, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group})
	if err != nil || !slices.Equal(kinds(transfers), []string{"deposit", "fee", "deposit"}) {
		t.Fatalf("group transfers = %v, err %v; want deposit, fee, deposit", kinds(transfers), err)
	}
	// Paging with a filter keeps to the kind.
	first, more, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group, Kind: "deposit", Limit: 1})
	if err != nil || !more || !slices.Equal(kinds(first), []string{"deposit"}) || first[0].Amount.String() != "50" {
		t.Fatalf("first deposit page = %+v, more %v, err %v; want the 50 deposit and more", first, more, err)
	}
	second, more, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group, Kind: "deposit", Cursor: first[0].Timestamp, Limit: 1})
	if err != nil || more || !slices.Equal(kinds(second), []string{"deposit"}) || second[0].Amount.String() != "100" {
		t.Fatalf("second deposit page = %+v, more %v, err %v; want the 100 deposit and no more", second, more, err)
	}
	fees, _, err := s.AccountTransfers(ctx, ledger.AccountTransfersParams{Address: "rider:1:wallet", Kind: "fee"})
	if err != nil || !slices.Equal(kinds(fees), []string{"fee"}) {
		t.Fatalf("account fees = %v, err %v; want one fee", kinds(fees), err)
	}
	if _, _, err := s.AccountTransfers(ctx, ledger.AccountTransfersParams{Address: "rider:1:wallet", Kind: "payout"}); !errors.Is(err, ledger.ErrKindNotFound) {
		t.Fatalf("account filter by unregistered kind err = %v, want ErrKindNotFound", err)
	}
	if _, _, err := s.GroupTransfers(ctx, ledger.GroupTransfersParams{GroupID: group, Kind: "payout"}); !errors.Is(err, ledger.ErrKindNotFound) {
		t.Fatalf("group filter by unregistered kind err = %v, want ErrKindNotFound", err)
	}
}
