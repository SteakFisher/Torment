package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestIntegrationRegisterCurrency(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	usd, err := s.RegisterCurrency(ctx, "USD/2")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	eur, err := s.RegisterCurrency(ctx, "EUR/2")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if usd.Ledger <= 0 || eur.Ledger == usd.Ledger {
		t.Fatalf("ledgers USD %d, EUR %d; want positive and distinct", usd.Ledger, eur.Ledger)
	}

	tests := []struct {
		name string
		code string
	}{
		{name: "duplicate code", code: "USD/2"},
		{name: "same name different precision", code: "USD/4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.RegisterCurrency(ctx, tt.code); !errors.Is(err, ErrConflict) {
				t.Fatalf("err = %v, want ErrConflict", err)
			}
		})
	}
}
