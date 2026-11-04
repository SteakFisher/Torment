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
