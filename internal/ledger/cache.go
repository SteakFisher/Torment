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

// kindCache is a Store that keeps every kind it reads or inserts in memory and passes every other call to
// the Store it wraps. A kind never changes once registered.
type kindCache struct {
	Store
	mu     sync.RWMutex
	byName map[string]Kind
	byCode map[uint16]Kind
}

var _ Store = (*kindCache)(nil)

func newKindCache(s Store) *kindCache {
	return &kindCache{Store: s, byName: map[string]Kind{}, byCode: map[uint16]Kind{}}
}

func (c *kindCache) InsertKind(ctx context.Context, name string) (Kind, bool, error) {
	k, created, err := c.Store.InsertKind(ctx, name)
	if err == nil {
		c.add(k)
	}
	return k, created, err
}

func (c *kindCache) Kinds(ctx context.Context, names []string) (map[string]Kind, error) {
	return cachedKinds(ctx, c, names, c.byName, c.Store.Kinds)
}

func (c *kindCache) KindsByCodes(ctx context.Context, codes []uint16) (map[uint16]Kind, error) {
	return cachedKinds(ctx, c, codes, c.byCode, c.Store.KindsByCodes)
}

// add caches every kind in kinds.
func (c *kindCache) add(kinds ...Kind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range kinds {
		c.byName[k.Name] = k
		c.byCode[k.Code] = k
	}
}

// cachedKinds returns the kind for each of keys found in index, one of c's maps, reading the others with
// load and caching what it finds. Keys with no kind are left out.
func cachedKinds[K comparable](ctx context.Context, c *kindCache, keys []K, index map[K]Kind, load func(context.Context, []K) (map[K]Kind, error)) (map[K]Kind, error) {
	out := make(map[K]Kind, len(keys))
	var misses []K
	c.mu.RLock()
	for _, key := range keys {
		if k, ok := index[key]; ok {
			out[key] = k
		} else {
			misses = append(misses, key)
		}
	}
	c.mu.RUnlock()
	if len(misses) == 0 {
		return out, nil
	}
	found, err := load(ctx, misses)
	if err != nil {
		return nil, err
	}
	for key, k := range found {
		out[key] = k
		c.add(k)
	}
	return out, nil
}
