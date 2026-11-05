package ledger

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestWalletCache(t *testing.T) {
	created := WalletKey{"rider:1:wallet", "USD"}
	uncreated := WalletKey{"rider:1:bonus", "USD"}
	stored := map[WalletKey]StoredWallet{
		created:   {ID: tb.ToUint128(1), Created: true, Ledger: 1},
		uncreated: {ID: tb.ToUint128(2), Ledger: 1},
	}
	var storedCalls, idCalls atomic.Int32
	c := newWalletCache(&fakeStore{
		storedWallets: func(_ context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
			storedCalls.Add(1)
			out := map[WalletKey]StoredWallet{}
			for _, k := range keys {
				out[k] = stored[k]
			}
			return out, nil
		},
		walletsByTBIDs: func(_ context.Context, ids []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
			idCalls.Add(1)
			out := map[tb.Uint128]WalletKey{}
			for _, id := range ids {
				if id == tb.ToUint128(2) {
					out[id] = uncreated
				}
			}
			return out, nil
		},
	})
	ctx := context.Background()

	// every goroutine gets the stored wallets whether or not another one filled the cache first
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := c.StoredWallets(ctx, []WalletKey{created, uncreated})
			if err != nil || !maps.Equal(got, stored) {
				t.Errorf("StoredWallets = %v, %v; want %v", got, err, stored)
			}
		})
	}
	wg.Wait()

	storedCalls.Store(0)
	if _, err := c.StoredWallets(ctx, []WalletKey{created}); err != nil || storedCalls.Load() != 0 {
		t.Fatalf("created wallet: %d store calls, err %v; want 0", storedCalls.Load(), err)
	}
	if _, err := c.StoredWallets(ctx, []WalletKey{uncreated}); err != nil || storedCalls.Load() != 1 {
		t.Fatalf("uncreated wallet: %d store calls, err %v; want 1", storedCalls.Load(), err)
	}

	// id 1 is cached from StoredWallets, id 2 is read once, id 3 has no wallet
	ids := []tb.Uint128{tb.ToUint128(1), tb.ToUint128(2), tb.ToUint128(3)}
	want := map[tb.Uint128]WalletKey{tb.ToUint128(1): created, tb.ToUint128(2): uncreated}
	for range 2 {
		got, err := c.WalletsByTBIDs(ctx, ids)
		if err != nil || !maps.Equal(got, want) {
			t.Fatalf("WalletsByTBIDs = %v, %v; want %v", got, err, want)
		}
	}
	if idCalls.Load() != 2 {
		t.Fatalf("WalletsByTBIDs store calls = %d, want 2 (the missing id is read every time)", idCalls.Load())
	}
}
