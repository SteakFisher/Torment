package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestIntegrationTransfer(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	if _, _, err := s.RegisterCurrency(ctx, "USD/2"); err != nil {
		t.Fatalf("error registering currency: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("world: %v", err)
	}
	for _, address := range []string{"rider:1:wallet", "rider:2:wallet"} {
		if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: address, Type: "liability", Currencies: []string{"USD"}}); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	checkUSD := func(want map[string]int64) {
		t.Helper()
		for address, amount := range want {
			got, err := accountBalances(s, address, time.Time{})
			if err != nil || len(got) != 1 || got[0].Amount.Int64() != amount {
				t.Fatalf("%s balances = %+v, err %v; want USD %d", address, got, err, amount)
			}
		}
	}
	// transfer posts chains and fails the test on a request error
	transfer := func(chains ...[]ledger.Posting) [][]ledger.Transfer {
		t.Helper()
		got, err := s.Transfer(ctx, chains)
		if err != nil {
			t.Fatalf("error transferring: %v", err)
		}
		return got
	}
	// posting returns one posting with a new id
	posting := func(debit, credit, amount, group string) ledger.Posting {
		return ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: debit, Credit: credit, Currency: "USD", Amount: amount, GroupID: group, Kind: "test"}
	}

	plain := posting("world", "rider:1:wallet", "100", "")
	first := transfer([]ledger.Posting{plain})[0][0]
	if first.Err != nil || !first.Created {
		t.Fatalf("transfer = %+v; want created", first)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	if again := transfer([]ledger.Posting{plain})[0][0]; again != (ledger.Transfer{ID: first.ID, GroupID: first.GroupID}) {
		t.Fatalf("same transfer again = %+v; want %+v not created", again, first)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})

	changed := plain
	changed.Amount = "101"
	if got := transfer([]ledger.Posting{changed})[0][0]; !errors.Is(got.Err, ledger.ErrConflict) {
		t.Fatalf("same id with different amount err = %v, want ErrConflict", got.Err)
	}
	if _, _, err := s.RegisterKind(ctx, "other"); err != nil {
		t.Fatalf("error registering kind: %v", err)
	}
	otherKind := plain
	otherKind.Kind = "other"
	if got := transfer([]ledger.Posting{otherKind})[0][0]; !errors.Is(got.Err, ledger.ErrConflict) {
		t.Fatalf("same id with different kind err = %v, want ErrConflict", got.Err)
	}

	overdraw := posting("rider:1:wallet", "world", "150", "")
	if got := transfer([]ledger.Posting{overdraw})[0][0]; !errors.Is(got.Err, ledger.ErrInsufficientFunds) {
		t.Fatalf("overdraw err = %v, want ErrInsufficientFunds", got.Err)
	}
	checkUSD(map[string]int64{"world": -100, "rider:1:wallet": 100})
	if got := transfer([]ledger.Posting{posting("world", "rider:1:wallet", "100", "")})[0][0]; got.Err != nil {
		t.Fatalf("error topping up: %v", got.Err)
	}
	if got := transfer([]ledger.Posting{overdraw})[0][0]; !errors.Is(got.Err, ledger.ErrIDAlreadyFailed) {
		t.Fatalf("failed id after top-up err = %v, want ErrIDAlreadyFailed", got.Err)
	}
	checkUSD(map[string]int64{"world": -200, "rider:1:wallet": 200})

	group := tb.ID()
	grouped := transfer([]ledger.Posting{posting("rider:1:wallet", "world", "40", ledger.FormatID(group))})[0][0]
	if grouped.Err != nil || grouped.GroupID != group {
		t.Fatalf("transfer with group = %+v; want group id %s", grouped, group.BigInt())
	}
	found, err := testTB.LookupTransfers([]tb.Uint128{first.ID, grouped.ID})
	if err != nil || len(found) != 2 {
		t.Fatalf("error looking up transfers: %v, found %d", err, len(found))
	}
	want := map[tb.Uint128]tb.Uint128{first.ID: first.ID, grouped.ID: group}
	for _, tr := range found {
		if tr.UserData128 != want[tr.ID] {
			t.Fatalf("transfer %s user data 128 = %s, want %s", tr.ID.BigInt(), tr.UserData128.BigInt(), want[tr.ID].BigInt())
		}
	}
	checkUSD(map[string]int64{"world": -160, "rider:1:wallet": 160, "rider:2:wallet": 0})

	overspent := transfer([]ledger.Posting{
		posting("rider:1:wallet", "rider:2:wallet", "50", ""),
		posting("rider:2:wallet", "world", "20", ""),
		posting("rider:2:wallet", "world", "100", ""),
	})[0]
	if !errors.Is(overspent[0].Err, ledger.ErrChainFailed) || !errors.Is(overspent[1].Err, ledger.ErrChainFailed) || !errors.Is(overspent[2].Err, ledger.ErrInsufficientFunds) {
		t.Fatalf("overspending chain = %+v; want chain failed, chain failed, insufficient funds", overspent)
	}
	checkUSD(map[string]int64{"world": -160, "rider:1:wallet": 160, "rider:2:wallet": 0})

	independent := transfer(
		[]ledger.Posting{posting("rider:1:wallet", "rider:2:wallet", "60", "")},
		[]ledger.Posting{posting("rider:2:wallet", "world", "100", "")},
	)
	if !independent[0][0].Created || independent[0][0].Err != nil || !errors.Is(independent[1][0].Err, ledger.ErrInsufficientFunds) {
		t.Fatalf("independent chains = %+v; want created, insufficient funds", independent)
	}
	checkUSD(map[string]int64{"world": -160, "rider:1:wallet": 100, "rider:2:wallet": 60})

	chain := []ledger.Posting{posting("rider:1:wallet", "rider:2:wallet", "10", ""), posting("rider:2:wallet", "world", "5", "")}
	for _, tr := range transfer(chain)[0] {
		if tr.Err != nil || !tr.Created {
			t.Fatalf("chain posting = %+v; want created", tr)
		}
	}
	checkUSD(map[string]int64{"world": -155, "rider:1:wallet": 90, "rider:2:wallet": 65})
	for _, tr := range transfer(chain)[0] {
		if tr.Err != nil || tr.Created {
			t.Fatalf("same chain again posting = %+v; want existing", tr)
		}
	}
	checkUSD(map[string]int64{"world": -155, "rider:1:wallet": 90, "rider:2:wallet": 65})

	fresh := posting("rider:2:wallet", "world", "5", "")
	reused := transfer([]ledger.Posting{chain[0], fresh})[0]
	if !errors.Is(reused[0].Err, ledger.ErrConflict) || !errors.Is(reused[1].Err, ledger.ErrChainFailed) {
		t.Fatalf("chain reusing an id = %+v; want conflict, chain failed", reused)
	}
	if found, err := testTB.LookupTransfers([]tb.Uint128{reused[1].ID}); err != nil || len(found) != 0 {
		t.Fatalf("fresh transfer lookup = %+v, err %v; want none", found, err)
	}
	checkUSD(map[string]int64{"world": -155, "rider:1:wallet": 90, "rider:2:wallet": 65})
}
