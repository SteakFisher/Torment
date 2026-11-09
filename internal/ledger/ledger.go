package ledger

import (
	"context"
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Service runs ledger operations against a Store and TigerBeetle.
type Service struct {
	store  Store
	writes tbWriter
	reads  tbReader
}

// New returns a Service that keeps wallets and kinds read from store in memory and sends TigerBeetle writes
// through writes and reads through reads, which should be separate clients.
func New(store Store, writes tbWriter, reads tbReader) *Service {
	return &Service{store: newKindCache(newWalletCache(store)), writes: writes, reads: reads}
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

// Store keeps currencies, kinds, accounts and wallets. Errors other than ErrNotFound and ErrConflict mean the
// store failed.
type Store interface {
	// InsertCurrency stores a new currency and returns it with its assigned ledger and created true. When a
	// currency already has the name, it stores nothing and returns that currency with created false.
	InsertCurrency(ctx context.Context, name string, precision int16) (c Currency, created bool, err error)

	// Currencies returns the registered currencies among names, keyed by name.
	Currencies(ctx context.Context, names []string) (map[string]Currency, error)
	// ListCurrencies returns every registered currency, ordered by name.
	ListCurrencies(ctx context.Context) ([]Currency, error)

	// InsertKind stores a new kind and returns it with its assigned code and created true. When a kind
	// already has the name, it stores nothing and returns that kind with created false.
	InsertKind(ctx context.Context, name string) (k Kind, created bool, err error)
	// Kinds returns the registered kinds among names, keyed by name.
	Kinds(ctx context.Context, names []string) (map[string]Kind, error)
	// KindsByCodes returns the registered kinds among codes, keyed by code.
	KindsByCodes(ctx context.Context, codes []uint16) (map[uint16]Kind, error)
	// ListKinds returns every registered kind, ordered by name.
	ListKinds(ctx context.Context) ([]Kind, error)

	// InsertAccount stores a new account and returns it with created true. When an account already has
	// the address, it stores nothing and returns that account with created false. The account has no
	// Wallets.
	InsertAccount(ctx context.Context, address string, typ AccountType, allowNegative bool) (acct Account, created bool, err error)

	// AccountWallets returns the account at address with its wallets in currency order, only those in
	// currency unless it is empty. It returns ErrAccountNotFound when no account has the address.
	AccountWallets(ctx context.Context, address, currency string) (Account, error)

	// ListAccounts returns up to limit accounts whose address matches the Postgres regex re and sorts
	// after after, ordered by address, with their wallets in currency order. Wallet IDs are not set. more
	// reports whether further accounts match.
	ListAccounts(ctx context.Context, re, after string, limit int) (accounts []Account, more bool, err error)

	// PatternWallets returns up to limit wallets of the accounts whose address matches the Postgres regex
	// re, only in currency unless it is empty, ordered by address and currency. When more than limit
	// wallets match, it returns none and tooMany true.
	PatternWallets(ctx context.Context, re, currency string, limit int) (wallets []AddressWallet, tooMany bool, err error)

	// WalletsByTBIDs returns the key of each wallet whose TigerBeetle account is in ids, by that id. Ids
	// with no wallet are left out.
	WalletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]WalletKey, error)

	// StoredWallets returns the stored wallet for each of keys whose account and currency are registered,
	// by key. A key whose account has no wallet in the currency gets a StoredWallet with a zero ID and
	// Created false.
	StoredWallets(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error)

	// InsertWallets stores wallets, skipping any whose account already has a wallet in the currency.
	InsertWallets(ctx context.Context, wallets []NewWallet) error

	// MarkWalletsCreated records that the TigerBeetle accounts ids of their wallets exist.
	MarkWalletsCreated(ctx context.Context, ids []tb.Uint128) error

	// Ping returns an error when the store can't be reached.
	Ping(ctx context.Context) error
}
