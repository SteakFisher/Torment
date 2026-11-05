package ledger

import (
	"context"
	"sync"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// walletCache is a Store that keeps the wallets it reads in memory and passes every other call to the
// Store it wraps. A wallet never changes once its TigerBeetle account is marked created, so StoredWallets
// keeps only created wallets; a wallet's key never changes once inserted, so WalletsByTBIDs keeps every
// key it finds.
type walletCache struct {
	Store
	mu    sync.RWMutex
	byKey map[WalletKey]StoredWallet
	byID  map[tb.Uint128]WalletKey
}

var _ Store = (*walletCache)(nil)

// ponytail: unbounded, holds every wallet read since start.
func newWalletCache(s Store) *walletCache {
	return &walletCache{Store: s, byKey: map[WalletKey]StoredWallet{}, byID: map[tb.Uint128]WalletKey{}}
}

func (c *walletCache) StoredWallets(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
	out := make(map[WalletKey]StoredWallet, len(keys))
	var misses []WalletKey
	c.mu.RLock()
	for _, k := range keys {
		if w, ok := c.byKey[k]; ok {
			out[k] = w
		} else {
			misses = append(misses, k)
		}
	}
	c.mu.RUnlock()
	if len(misses) == 0 {
		return out, nil
	}

	found, err := c.Store.StoredWallets(ctx, misses)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, w := range found {
		out[k] = w
		if w.Created {
			c.byKey[k] = w
			c.byID[w.ID] = k
		}
	}
	return out, nil
}

func (c *walletCache) WalletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
	out := make(map[tb.Uint128]WalletKey, len(ids))
	var misses []tb.Uint128
	c.mu.RLock()
	for _, id := range ids {
		if k, ok := c.byID[id]; ok {
			out[id] = k
		} else {
			misses = append(misses, id)
		}
	}
	c.mu.RUnlock()
	if len(misses) == 0 {
		return out, nil
	}

	found, err := c.Store.WalletsByTBIDs(ctx, misses)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, k := range found {
		out[id] = k
		c.byID[id] = k
	}
	return out, nil
}
