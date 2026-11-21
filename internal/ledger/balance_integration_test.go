package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestIntegrationBalances(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	for _, code := range []string{"USD/2", "AED/2"} {
		if _, _, err := s.RegisterCurrency(ctx, code); err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("world: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}}); err != nil {
		t.Fatalf("rider: %v", err)
	}

	// pay moves amount of USD from world to the rider and returns TigerBeetle's timestamp for it
	pay := func(amount string) time.Time {
		t.Helper()
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: ledger.FormatID(tb.ID()), Debit: "world", Credit: "rider:1:wallet", Currency: "USD", Amount: amount, Kind: "test"}}})
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
		got, err := accountBalances(s, address, time.Time{})
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
		got, err := accountBalances(s, "rider:1:wallet", at)
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

	if _, err := accountBalances(s, "nobody", time.Time{}); !errors.Is(err, ledger.ErrAccountNotFound) {
		t.Fatalf("unknown address err = %v, want ErrAccountNotFound", err)
	}
}

func TestIntegrationPatternBalances(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	for _, code := range []string{"USD/2", "AED/2"} {
		if _, _, err := s.RegisterCurrency(ctx, code); err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
	}
	for _, p := range []ledger.RegisterAccountParams{
		{Address: "world", Type: "equity", AllowNegative: true},
		{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"AED"}},
		{Address: "rider:1:bonus", Type: "liability"},
		{Address: "rider:1:card", Type: "asset", AllowNegative: true},
		{Address: "rider:2:wallet", Type: "liability"},
	} {
		if _, _, err := s.RegisterAccount(ctx, p); err != nil {
			t.Fatalf("%s: %v", p.Address, err)
		}
	}
	// pay moves amount of USD and returns TigerBeetle's timestamp for it
	pay := func(debit, credit, amount string) time.Time {
		t.Helper()
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: ledger.FormatID(tb.ID()), Debit: debit, Credit: credit, Currency: "USD", Amount: amount, Kind: "test"}}})
		if err != nil || got[0][0].Err != nil {
			t.Fatalf("error transferring: %v, %+v", err, got)
		}
		found, err := testTB.LookupTransfers([]tb.Uint128{got[0][0].ID})
		if err != nil || len(found) != 1 {
			t.Fatalf("error looking up transfer: %v", err)
		}
		return time.Unix(0, int64(found[0].Timestamp))
	}
	first := pay("world", "rider:1:wallet", "100")
	pay("world", "rider:1:bonus", "20")
	pay("rider:1:card", "rider:1:wallet", "5") // the card is an asset, so debiting it raises it to 5
	pay("world", "rider:2:wallet", "999")

	// summary prints totals and accounts as text
	summary := func(pattern, currency string, at time.Time) string {
		t.Helper()
		totals, accounts, err := s.Balances(ctx, pattern, currency, at)
		if err != nil {
			t.Fatalf("error getting balances for %q: %v", pattern, err)
		}
		out := ""
		for _, tt := range totals {
			out += fmt.Sprintf("%s %s %s; ", tt.Type, tt.Currency, tt.Amount)
		}
		for _, a := range accounts {
			out += a.Address
			for _, b := range a.Balances {
				out += fmt.Sprintf(" %s %s", b.Currency, b.Amount)
			}
			out += "; "
		}
		return out
	}

	if got, want := summary("rider:1:*", "", time.Time{}),
		"asset USD 5; liability AED 0; liability USD 125; rider:1:bonus USD 20; rider:1:card USD 5; rider:1:wallet AED 0 USD 105; "; got != want {
		t.Fatalf("rider:1:* =\n%s\nwant\n%s", got, want)
	}
	// wallets created after first are listed with zero, not hidden
	if got, want := summary("rider:1:*", "USD", first),
		"asset USD 0; liability USD 100; rider:1:bonus USD 0; rider:1:card USD 0; rider:1:wallet USD 100; "; got != want {
		t.Fatalf("rider:1:* USD at first =\n%s\nwant\n%s", got, want)
	}
	if got, want := summary("rider:1:wallet", "", time.Time{}), "liability AED 0; liability USD 105; rider:1:wallet AED 0 USD 105; "; got != want {
		t.Fatalf("rider:1:wallet = %q, want %q", got, want)
	}
	if got, want := summary("nobody:*", "", time.Time{}), ""; got != want {
		t.Fatalf("nobody:* = %q, want nothing", got)
	}
}
