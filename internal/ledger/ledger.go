package ledger

import (
	"context"
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Service runs ledger operations against a Store and TigerBeetle.
type Service struct {
	store  Store
	names  *Names
	writes tbWriter
	reads  tbReader
}

// New returns a Service that keeps wallets and kinds read from store in memory and sends TigerBeetle writes
// through writes and reads through reads, which should be separate clients.
func New(store Store, writes tbWriter, reads tbReader) *Service {
	cached := cachedStore(store)
	return &Service{store: cached, names: &Names{store: cached}, writes: writes, reads: reads}
}

// cachedStore returns store behind the wallet and kind caches.
func cachedStore(store Store) Store {
	return newKindCache(newWalletCache(store))
}

// Ready returns an error when Postgres or TigerBeetle can't be reached.
func (s *Service) Ready(ctx context.Context) error {
	if err := s.store.Ping(ctx); err != nil {
		return fmt.Errorf("error pinging postgres: %w", err)
	}
	// Any id works: looking it up is a round trip whether or not the account exists.
	if _, err := tbCall(ctx, func() ([]tb.Account, error) { return s.reads.LookupAccounts([]tb.Uint128{tb.ToUint128(1)}) }); err != nil {
		return fmt.Errorf("error reaching tigerbeetle: %w", err)
	}
	return nil
}

const (
	// defaultListLimit and maxListLimit are the default and largest page size of every list.
	defaultListLimit = 100
	maxListLimit     = 1000
	// maxPatternWallets is the most wallets a balance query may cover.
	maxPatternWallets = 1000
	// maxPatternTransferWallets is the most wallets a transfer history query may cover.
	maxPatternTransferWallets = 100
)

// listLimit returns limit, or defaultListLimit when it is 0. It returns ErrInvalid for a limit outside 0
// to maxListLimit.
func listLimit(limit int) (int, error) {
	if limit < 0 || limit > maxListLimit {
		return 0, Errorf(ErrInvalid, "limit %d must be between 1 and %d", limit, maxListLimit)
	}
	if limit == 0 {
		return defaultListLimit, nil
	}
	return limit, nil
}
