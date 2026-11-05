package ledger_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestIntegrationListAccounts(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	for _, code := range []string{"USD/2", "AED/2"} {
		if _, _, err := s.RegisterCurrency(ctx, code); err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
	}
	for _, p := range []ledger.RegisterAccountParams{
		{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}},
		{Address: "rider:1:bonus", Type: "liability"},
		{Address: "rider:2:wallet", Type: "liability"},
		{Address: "rider:2:wallet:old", Type: "liability"},
		{Address: "world", Type: "equity", AllowNegative: true},
	} {
		if _, _, err := s.RegisterAccount(ctx, p); err != nil {
			t.Fatalf("%s: %v", p.Address, err)
		}
	}
	// list returns the addresses of one page
	list := func(pattern, after string, limit int) ([]string, bool) {
		t.Helper()
		accounts, more, err := s.ListAccounts(ctx, pattern, after, limit)
		if err != nil {
			t.Fatalf("error listing %q: %v", pattern, err)
		}
		var out []string
		for _, a := range accounts {
			out = append(out, a.Address)
		}
		return out, more
	}

	if got, _ := list("rider:*:wallet", "", 0); len(got) != 2 || got[0] != "rider:1:wallet" || got[1] != "rider:2:wallet" {
		t.Fatalf("rider:*:wallet = %v", got)
	}
	if got, _ := list("rider:1:*", "", 0); len(got) != 2 || got[0] != "rider:1:bonus" || got[1] != "rider:1:wallet" {
		t.Fatalf("rider:1:* = %v", got)
	}
	if got, _ := list("", "", 0); len(got) != 5 {
		t.Fatalf("all accounts = %v", got)
	}

	page1, more := list("", "", 3)
	if len(page1) != 3 || !more {
		t.Fatalf("page 1 = %v, more %v", page1, more)
	}
	page2, more := list("", page1[2], 3)
	if len(page2) != 2 || more || page2[0] <= page1[2] {
		t.Fatalf("page 2 = %v, more %v", page2, more)
	}

	accounts, _, err := s.ListAccounts(ctx, "rider:1:wallet", "", 0)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("rider:1:wallet = %+v, err %v", accounts, err)
	}
	w := accounts[0].Wallets
	if len(w) != 2 || w[0].Currency != "AED" || w[1].Currency != "USD" || w[0].Precision != 2 {
		t.Fatalf("wallets = %+v; want AED, USD", w)
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
	pay := func(source, destination, amount string) time.Time {
		t.Helper()
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: tb.ID().BigInt().String(), Source: source, Destination: destination, Currency: "USD", Amount: amount}}})
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
		totals, accounts, err := s.PatternBalances(ctx, pattern, currency, at)
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
	if got, want := summary("nobody:*", "", time.Time{}), ""; got != want {
		t.Fatalf("nobody:* = %q, want nothing", got)
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
	pay := func(source, destination, currency, amount string) string {
		t.Helper()
		id := tb.ID().BigInt().String()
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: id, Source: source, Destination: destination, Currency: currency, Amount: amount}}})
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
			out = append(out, g.ID.BigInt().String())
		}
		return out, got, more
	}

	all, records, more := ids(ledger.AccountTransfersParams{})
	if fmt.Sprint(all) != fmt.Sprint([]string{c, b, a}) || more {
		t.Fatalf("all = %v, more %v; want %v", all, more, []string{c, b, a})
	}
	if r := records[0]; r.Source != "rider:1:wallet" || r.Destination != "rider:2:wallet" || r.Currency != "USD" || r.Amount.Int64() != 30 || r.GroupID.BigInt().String() != c {
		t.Fatalf("newest = %+v", r)
	}

	page1, records, more := ids(ledger.AccountTransfersParams{Limit: 2})
	if fmt.Sprint(page1) != fmt.Sprint([]string{c, b}) || !more {
		t.Fatalf("page 1 = %v, more %v", page1, more)
	}
	page2, _, more := ids(ledger.AccountTransfersParams{Limit: 2, Before: records[1].Timestamp})
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
