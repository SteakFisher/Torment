package ledger

import (
	"context"
	"errors"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
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

	_, err = tbCall(context.Background(), func() (int, error) { return 0, tb.ErrTooMuchData })
	if !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("err = %v, want ErrBatchTooLarge", err)
	}
}

func TestReady(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name           string
		pingErr, tbErr error
		wantErr        error
	}{
		{name: "both reachable"},
		{name: "postgres down", pingErr: boom, wantErr: boom},
		{name: "tigerbeetle down", tbErr: boom, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{ping: func(context.Context) error { return tt.pingErr }}
			reads := &fakeTB{lookupAccounts: func([]tb.Uint128) ([]tb.Account, error) { return nil, tt.tbErr }}
			err := (&Service{store: store, reads: reads}).Ready(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Ready = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
