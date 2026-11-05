package ledger_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"torment/internal/ledger"
)

func TestIntegrationRegisterCurrency(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	usd, created, err := s.RegisterCurrency(ctx, "USD/2")
	if err != nil || !created {
		t.Fatalf("first register: created %v, err %v; want created", created, err)
	}
	eur, created, err := s.RegisterCurrency(ctx, "EUR/2")
	if err != nil || !created {
		t.Fatalf("second register: created %v, err %v; want created", created, err)
	}
	if usd.Ledger <= 0 || eur.Ledger == usd.Ledger {
		t.Fatalf("ledgers USD %d, EUR %d; want positive and distinct", usd.Ledger, eur.Ledger)
	}
	all, err := s.Currencies(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if want := []ledger.Currency{eur, usd}; !slices.Equal(all, want) {
		t.Fatalf("currencies = %v, want %v", all, want)
	}

	again, created, err := s.RegisterCurrency(ctx, "USD/2")
	if err != nil || created || again != usd {
		t.Fatalf("duplicate code = %+v, created %v, err %v; want %+v, not created", again, created, err, usd)
	}
	if _, _, err := s.RegisterCurrency(ctx, "USD/4"); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("same name different precision err = %v, want ErrConflict", err)
	}
}
