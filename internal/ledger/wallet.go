package ledger

import (
	"context"
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Wallet is an account's balance in one currency. Precision is the currency's number of decimal
// places; ID is the TigerBeetle account that holds the balance.
type Wallet struct {
	Currency  string
	Precision int16
	ID        tb.Uint128
}

// WalletKey identifies the wallet in Currency of the account at Address.
type WalletKey struct{ Address, Currency string }

// StoredWallet is a wallet's TigerBeetle account ID and whether that account is marked Created, the
// Ledger and Precision of its currency, and the AccountID, Type and AllowNegative of its account.
type StoredWallet struct {
	ID            tb.Uint128
	Created       bool
	Ledger        int32
	Precision     int16
	AccountID     int64
	Type          AccountType
	AllowNegative bool
}

// NewWallet is a wallet to insert: its account's id, its currency and its TigerBeetle account id.
type NewWallet struct {
	AccountID int64
	Currency  string
	ID        tb.Uint128
}

// AddressWallet is a wallet with the address and type of its account.
type AddressWallet struct {
	Address string
	Type    AccountType
	Wallet
}

// ensureWallets returns the wallet for each of keys, which must be distinct. It inserts the wallets that
// do not exist with new ids, then creates and marks as created every TigerBeetle account not yet marked.
// For the first key whose account or currency is not registered, it returns ErrCurrencyNotFound when
// the currency is missing and ErrAccountNotFound otherwise.
func (s *Service) ensureWallets(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	wallets, err := s.store.StoredWallets(ctx, keys)
	if err != nil {
		return nil, err
	}
	var missing []NewWallet
	for _, k := range keys {
		w, ok := wallets[k]
		if !ok {
			return nil, s.missingWallet(ctx, k)
		}
		if w.ID == (tb.Uint128{}) {
			missing = append(missing, NewWallet{AccountID: w.AccountID, Currency: k.Currency, ID: tb.ID()})
		}
	}
	if len(missing) > 0 {
		if err := s.store.InsertWallets(ctx, missing); err != nil {
			return nil, err
		}
		// reads the stored ids, which differ from the new ones where another request inserted the wallet first
		if wallets, err = s.store.StoredWallets(ctx, keys); err != nil {
			return nil, err
		}
	}

	var tbAccounts []tb.Account
	var uncreated []tb.Uint128
	for _, k := range keys {
		w := wallets[k]
		if !w.Created {
			tbAccounts = append(tbAccounts, newTBAccount(w.ID, w.Ledger, w.Type, w.AllowNegative))
			uncreated = append(uncreated, w.ID)
		}
	}
	if len(uncreated) > 0 {
		if err := s.ensureTBAccounts(ctx, tbAccounts); err != nil {
			return nil, err
		}
		if err := s.store.MarkWalletsCreated(ctx, uncreated); err != nil {
			return nil, err
		}
	}
	return wallets, nil
}

// missingWallet returns the error for a key StoredWallets left out: ErrCurrencyNotFound when k.Currency
// is not registered, and ErrAccountNotFound otherwise.
func (s *Service) missingWallet(ctx context.Context, k WalletKey) error {
	if _, err := s.lookupCurrencies(ctx, []string{k.Currency}); err != nil {
		return err
	}
	return fmt.Errorf("%w: account %q is not registered", ErrAccountNotFound, k.Address)
}
