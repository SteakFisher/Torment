package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestIntegrationRegisterCurrency(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	if _, err := s.RegisterCurrency(ctx, "USD/2", 1); err != nil {
		t.Fatalf("first register: %v", err)
	}

	tests := []struct {
		name   string
		code   string
		ledger int32
	}{
		{name: "duplicate code", code: "USD/2", ledger: 2},
		{name: "same name different precision", code: "USD/4", ledger: 2},
		{name: "duplicate ledger", code: "EUR/2", ledger: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.RegisterCurrency(ctx, tt.code, tt.ledger); !errors.Is(err, ErrConflict) {
				t.Fatalf("err = %v, want ErrConflict", err)
			}
		})
	}
}
