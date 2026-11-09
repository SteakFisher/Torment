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

func TestKindCache(t *testing.T) {
	var calls atomic.Int32
	f := withKinds(&fakeStore{})
	kinds, byCodes := f.kinds, f.kindsByCodes
	f.kinds = func(ctx context.Context, names []string) (map[string]Kind, error) {
		calls.Add(1)
		return kinds(ctx, names)
	}
	f.kindsByCodes = func(ctx context.Context, codes []uint16) (map[uint16]Kind, error) {
		calls.Add(1)
		return byCodes(ctx, codes)
	}
	f.insertKind = func(_ context.Context, name string) (Kind, bool, error) { return Kind{Name: name, Code: 7}, true, nil }
	c := newKindCache(f)
	ctx := context.Background()

	deposit, refund := testKinds[0], testKinds[1]
	for range 2 {
		got, err := c.Kinds(ctx, []string{"deposit"})
		if err != nil || !maps.Equal(got, map[string]Kind{"deposit": deposit}) {
			t.Fatalf("Kinds = %v, %v", got, err)
		}
	}
	// deposit is cached by name and by code; refund is read once
	for range 2 {
		got, err := c.KindsByCodes(ctx, []uint16{deposit.Code, refund.Code})
		if err != nil || !maps.Equal(got, map[uint16]Kind{deposit.Code: deposit, refund.Code: refund}) {
			t.Fatalf("KindsByCodes = %v, %v", got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("store calls = %d, want 2", calls.Load())
	}

	// an unregistered name is read every time
	for range 2 {
		if got, err := c.Kinds(ctx, []string{"payout"}); err != nil || len(got) != 0 {
			t.Fatalf("Kinds(payout) = %v, %v; want none", got, err)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("store calls = %d, want 4", calls.Load())
	}

	// a registered kind is cached
	if _, _, err := c.InsertKind(ctx, "fee"); err != nil {
		t.Fatal(err)
	}
	if got, err := c.KindsByCodes(ctx, []uint16{7}); err != nil || got[7].Name != "fee" || calls.Load() != 4 {
		t.Fatalf("KindsByCodes(7) = %v, %v with %d store calls; want fee from the cache", got, err, calls.Load())
	}
}
