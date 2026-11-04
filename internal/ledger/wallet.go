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

// walletKey identifies the wallet in currency of the account at address.
type walletKey struct{ address, currency string }

// storedWallet is a wallet's TigerBeetle account id and whether that account is marked created, the
// ledger and precision of its currency, and the id, type and allowNegative of its account.
type storedWallet struct {
	id            tb.Uint128
	created       bool
	ledger        int32
	precision     int16
	accountID     int64
	typ           AccountType
	allowNegative bool
}

// ensureWallets returns the wallet for each of keys, which must be distinct. It inserts the wallets that
// do not exist with new ids, then creates and marks as created every TigerBeetle account not yet marked.
// It returns ErrNotFound naming the first key whose account or currency is not registered.
func (s *Service) ensureWallets(ctx context.Context, keys []walletKey) (map[walletKey]storedWallet, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	wallets, err := s.storedWallets(ctx, keys)
	if err != nil {
		return nil, err
	}
	var missing []newWallet
	for _, k := range keys {
		w, ok := wallets[k]
		if !ok {
			return nil, fmt.Errorf("%w: account %q or currency %q is not registered", ErrNotFound, k.address, k.currency)
		}
		if w.id == (tb.Uint128{}) {
			missing = append(missing, newWallet{accountID: w.accountID, currency: k.currency, id: tb.ID()})
		}
	}
	if len(missing) > 0 {
		if err := s.insertWallets(ctx, missing); err != nil {
			return nil, err
		}
		// reads the stored ids, which differ from the new ones where another request inserted the wallet first
		if wallets, err = s.storedWallets(ctx, keys); err != nil {
			return nil, err
		}
	}

	var tbAccounts []tb.Account
	var uncreated []tb.Uint128
	for _, k := range keys {
		w := wallets[k]
		if !w.created {
			tbAccounts = append(tbAccounts, newTBAccount(w.id, w.ledger, w.typ, w.allowNegative))
			uncreated = append(uncreated, w.id)
		}
	}
	if len(uncreated) > 0 {
		if err := s.ensureTBAccounts(tbAccounts); err != nil {
			return nil, err
		}
		if err := s.markWalletsCreated(ctx, uncreated); err != nil {
			return nil, err
		}
	}
	return wallets, nil
}
