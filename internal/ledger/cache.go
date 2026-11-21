package ledger

import (
	"context"
	"sync"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// walletCache is a Store that keeps the wallets it reads in memory and passes every other call to the
// Store it wraps. A wallet never changes once its TigerBeetle account is marked created, so StoredWallets
// keeps only created wallets; a wallet's address, type, currency and precision never change once inserted,
// so WalletsByTBIDs keeps every wallet it finds.
type walletCache struct {
	Store
	mu    sync.RWMutex
	byKey map[WalletKey]StoredWallet
	byID  map[tb.Uint128]AddressWallet
}

var _ Store = (*walletCache)(nil)

// ponytail: unbounded, holds every wallet read since start.
func newWalletCache(s Store) *walletCache {
	return &walletCache{Store: s, byKey: map[WalletKey]StoredWallet{}, byID: map[tb.Uint128]AddressWallet{}}
}

func (c *walletCache) StoredWallets(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
	return readThrough(ctx, &c.mu, keys, c.byKey, c.Store.StoredWallets, func(k WalletKey, w StoredWallet) {
		if w.Created {
			c.byKey[k] = w
			c.byID[w.ID] = AddressWallet{Address: k.Address, Type: w.Type, Wallet: Wallet{Currency: k.Currency, Precision: w.Precision, ID: w.ID}}
		}
	})
}

func (c *walletCache) WalletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]AddressWallet, error) {
	return readThrough(ctx, &c.mu, ids, c.byID, c.Store.WalletsByTBIDs, func(id tb.Uint128, w AddressWallet) { c.byID[id] = w })
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
		c.mu.Lock()
		c.add(k)
		c.mu.Unlock()
	}
	return k, created, err
}

func (c *kindCache) Kinds(ctx context.Context, names []string) (map[string]Kind, error) {
	return readThrough(ctx, &c.mu, names, c.byName, c.Store.Kinds, func(_ string, k Kind) { c.add(k) })
}

func (c *kindCache) KindsByCodes(ctx context.Context, codes []uint16) (map[uint16]Kind, error) {
	return readThrough(ctx, &c.mu, codes, c.byCode, c.Store.KindsByCodes, func(_ uint16, k Kind) { c.add(k) })
}

// add caches k. The caller holds c.mu for writing.
func (c *kindCache) add(k Kind) {
	c.byName[k.Name] = k
	c.byCode[k.Code] = k
}

// readThrough returns the value for each of keys found in index, reading the others with load. It calls
// keep with mu held for writing for each value load finds. mu guards index. Keys with no value are left
// out.
func readThrough[K comparable, V any](ctx context.Context, mu *sync.RWMutex, keys []K, index map[K]V, load func(context.Context, []K) (map[K]V, error), keep func(K, V)) (map[K]V, error) {
	out := make(map[K]V, len(keys))
	var misses []K
	mu.RLock()
	for _, key := range keys {
		if v, ok := index[key]; ok {
			out[key] = v
		} else {
			misses = append(misses, key)
		}
	}
	mu.RUnlock()
	if len(misses) == 0 {
		return out, nil
	}
	found, err := load(ctx, misses)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	for key, v := range found {
		out[key] = v
		keep(key, v)
	}
	return out, nil
}
