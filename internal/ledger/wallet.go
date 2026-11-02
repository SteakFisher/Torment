package ledger

import (
	"context"
	"fmt"
	"log/slog"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
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

// storedWallet is a wallet's TigerBeetle account id and the ledger and precision of its currency.
type storedWallet struct {
	id        tb.Uint128
	ledger    int32
	precision int16
}

// ensureWallets returns the wallet for each of keys, which must be distinct. It inserts the wallets that
// do not exist with new ids, then creates and marks as created every TigerBeetle account not yet marked.
// It returns ErrNotFound naming the first key whose account or currency is not registered.
func (s *Service) ensureWallets(ctx context.Context, keys []walletKey) (map[walletKey]storedWallet, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := s.getWallets(ctx, keys)
	if err != nil {
		return nil, err
	}
	var missing sqlc.InsertWalletsParams
	for _, k := range keys {
		r, ok := rows[k]
		if !ok {
			return nil, fmt.Errorf("%w: account %q or currency %q is not registered", ErrNotFound, k.address, k.currency)
		}
		if r.TbID == nil {
			missing.TbIds = append(missing.TbIds, idToBytes(tb.ID()))
			missing.AccountIds = append(missing.AccountIds, r.AccountID)
			missing.Currencies = append(missing.Currencies, k.currency)
		}
	}
	if len(missing.TbIds) > 0 {
		if err := s.q.InsertWallets(ctx, missing); err != nil {
			return nil, fmt.Errorf("error inserting wallets: %w", err)
		}
		// reads the stored ids, which differ from the new ones where another request inserted the wallet first
		if rows, err = s.getWallets(ctx, keys); err != nil {
			return nil, err
		}
	}

	wallets := make(map[walletKey]storedWallet, len(keys))
	var tbAccounts []tb.Account
	var uncreated [][]byte
	for _, k := range keys {
		r := rows[k]
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, err
		}
		wallets[k] = storedWallet{id: id, ledger: r.Ledger, precision: r.Precision}
		if !r.TbCreated.Bool {
			tbAccounts = append(tbAccounts, newTBAccount(id, r.Ledger, AccountType(r.Type), r.AllowNegative))
			uncreated = append(uncreated, r.TbID)
		}
	}
	if len(uncreated) > 0 {
		if err := s.ensureTBAccounts(tbAccounts); err != nil {
			return nil, err
		}
		if err := s.q.MarkWalletsTBCreated(ctx, uncreated); err != nil {
			return nil, fmt.Errorf("error marking wallets created: %w", err)
		}
	}
	return wallets, nil
}

// getWallets runs GetWallets for keys and returns the rows by key.
func (s *Service) getWallets(ctx context.Context, keys []walletKey) (map[walletKey]sqlc.GetWalletsRow, error) {
	var params sqlc.GetWalletsParams
	for _, k := range keys {
		params.Addresses = append(params.Addresses, k.address)
		params.Currencies = append(params.Currencies, k.currency)
	}
	rows, err := s.q.GetWallets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	byKey := make(map[walletKey]sqlc.GetWalletsRow, len(rows))
	for _, r := range rows {
		byKey[walletKey{r.Address, r.Currency}] = r
	}
	return byKey, nil
}

// newTBAccount returns the TigerBeetle account for a wallet with the given id on ledger, owned by an
// account of type typ.
func newTBAccount(id tb.Uint128, ledger int32, typ AccountType, allowNegative bool) tb.Account {
	return tb.Account{ID: id, Ledger: uint32(ledger), Code: tbCode(typ), Flags: tbFlags(typ, allowNegative)}
}

// ensureTBAccounts creates accounts in TigerBeetle in one request. An account that already exists
// with the same fields counts as created. It returns an error when the client fails or any account
// is rejected; an account that exists with different fields is also logged as an error.
func (s *Service) ensureTBAccounts(accounts []tb.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	results, err := s.tb.CreateAccounts(accounts)
	if err != nil {
		return fmt.Errorf("error creating tigerbeetle accounts: %w", err)
	}
	if len(results) != len(accounts) {
		return fmt.Errorf("error creating tigerbeetle accounts: got %d results for %d accounts", len(results), len(accounts))
	}
	// results[i] is the outcome for accounts[i]
	for i, r := range results {
		id := accounts[i].ID.BigInt()
		switch r.Status {
		case tb.AccountCreated, tb.AccountExists:
		case tb.AccountExistsWithDifferentFlags,
			tb.AccountExistsWithDifferentUserData128,
			tb.AccountExistsWithDifferentUserData64,
			tb.AccountExistsWithDifferentUserData32,
			tb.AccountExistsWithDifferentLedger,
			tb.AccountExistsWithDifferentCode:
			slog.Error("tigerbeetle account exists with different fields", "id", id, "status", r.Status)
			return fmt.Errorf("error creating tigerbeetle account %s: %s", id, r.Status)
		default:
			return fmt.Errorf("error creating tigerbeetle account %s: %s", id, r.Status)
		}
	}
	return nil
}
