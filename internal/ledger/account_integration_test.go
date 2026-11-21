package ledger_test

import (
	"context"
	"errors"
	"testing"
	"torment/internal/ledger"
)

func TestIntegrationRegisterAccount(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	p := ledger.RegisterAccountParams{Address: "rider:1:wallet", Type: "liability"}

	first, created, err := s.RegisterAccount(ctx, p)
	if err != nil || !created {
		t.Fatalf("first register: created %v, err %v", created, err)
	}

	replay, created, err := s.RegisterAccount(ctx, p)
	if err != nil || created {
		t.Fatalf("replay: created %v, err %v", created, err)
	}
	if !replay.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replay created_at = %v, want %v", replay.CreatedAt, first.CreatedAt)
	}

	p.AllowNegative = true
	if _, _, err := s.RegisterAccount(ctx, p); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("different fields: err = %v, want ErrConflict", err)
	}

	world, created, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "asset", AllowNegative: true})
	if err != nil || !created || !world.AllowNegative {
		t.Fatalf("world: %+v, created %v, err %v", world, created, err)
	}
}

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
