package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestTBCall(t *testing.T) {
	got, err := tbCall(context.Background(), func() (int, error) { return 7, nil })
	if got != 7 || err != nil {
		t.Fatalf("tbCall = %d, %v; want 7, nil", got, err)
	}

	// A call that never answers returns once ctx ends.
	block := make(chan struct{})
	defer close(block)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = tbCall(ctx, func() (int, error) { <-block; return 0, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
