package ledger

import (
	"context"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Service runs ledger operations against a Store and TigerBeetle.
type Service struct {
	store Store
	tb    tbClient
}

func New(store Store, tb tbClient) *Service {
	return &Service{store: store, tb: tb}
}

// Store keeps currencies, accounts and wallets. Errors other than ErrNotFound and ErrConflict mean the
// store failed.
type Store interface {
	// InsertCurrency stores a new currency and returns it with its assigned ledger. It returns ErrConflict
	// when the name is already registered.
	InsertCurrency(ctx context.Context, name string, precision int16) (Currency, error)

	// Currencies returns the registered currencies among names, keyed by name.
	Currencies(ctx context.Context, names []string) (map[string]Currency, error)

	// InsertAccount stores a new account and returns it with created true. When an account already has
	// the address, it stores nothing and returns that account with created false. The account has no
	// Wallets.
	InsertAccount(ctx context.Context, address string, typ AccountType, allowNegative bool) (acct Account, created bool, err error)

	// AccountWallets returns the account at address with its wallets in currency order, only those in
	// currency unless it is empty. It returns ErrNotFound when no account has the address.
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
}
