package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// idToBytes returns id as 16 bytes in TigerBeetle's little-endian order, the form stored in Postgres.
func idToBytes(id tb.Uint128) []byte {
	b := id.Bytes()
	return b[:]
}

// idFromBytes is the inverse of idToBytes. It returns an error when b is not 16 bytes long.
func idFromBytes(b []byte) (tb.Uint128, error) {
	if len(b) != 16 {
		return tb.Uint128{}, fmt.Errorf("tigerbeetle id must be 16 bytes, got %d", len(b))
	}
	return tb.BytesToUint128([16]byte(b)), nil
}

// idsToBytes returns each of ids as idToBytes does.
func idsToBytes(ids []tb.Uint128) [][]byte {
	out := make([][]byte, len(ids))
	for i, id := range ids {
		out[i] = idToBytes(id)
	}
	return out
}

// insertCurrency stores a new currency and returns it with its assigned ledger. It returns ErrConflict
// when the name is already registered.
func (s *Service) insertCurrency(ctx context.Context, name string, precision int16) (Currency, error) {
	row, err := s.q.InsertCurrency(ctx, sqlc.InsertCurrencyParams{Name: name, Precision: precision})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return Currency{}, fmt.Errorf("%w: currency %q already exists", ErrConflict, name)
		}
		return Currency{}, fmt.Errorf("error inserting currency: %w", err)
	}
	return Currency{Name: row.Name, Precision: row.Precision, Ledger: row.Ledger}, nil
}

// currencies returns the registered currencies among names, keyed by name.
func (s *Service) currencies(ctx context.Context, names []string) (map[string]Currency, error) {
	rows, err := s.q.GetCurrencies(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("error getting currencies: %w", err)
	}
	currencies := make(map[string]Currency, len(rows))
	for _, r := range rows {
		currencies[r.Name] = Currency{Name: r.Name, Precision: r.Precision, Ledger: r.Ledger}
	}
	return currencies, nil
}

// insertAccount stores a new account and returns it with created true. When an account already has the
// address, it stores nothing and returns that account with created false. The account has no Wallets.
func (s *Service) insertAccount(ctx context.Context, address string, typ AccountType, allowNegative bool) (acct Account, created bool, err error) {
	row, err := s.q.InsertAccount(ctx, sqlc.InsertAccountParams{
		Address:       address,
		Type:          sqlc.AccountType(typ),
		AllowNegative: allowNegative,
	})
	switch {
	case err == nil:
		return toAccount(row), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		row, err = s.q.GetAccount(ctx, address)
		if err != nil {
			return Account{}, false, fmt.Errorf("error getting account: %w", err)
		}
		return toAccount(row), false, nil
	default:
		return Account{}, false, fmt.Errorf("error inserting account: %w", err)
	}
}

func toAccount(row sqlc.Account) Account {
	return Account{
		Address:       row.Address,
		Type:          AccountType(row.Type),
		AllowNegative: row.AllowNegative,
		CreatedAt:     row.CreatedAt.Time,
	}
}

// accountWallets returns the account at address with its wallets in currency order, only those in
// currency unless it is empty. It returns ErrNotFound when no account has the address.
func (s *Service) accountWallets(ctx context.Context, address, currency string) (Account, error) {
	row, err := s.q.GetAccount(ctx, address)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, fmt.Errorf("%w: account %q", ErrNotFound, address)
	}
	if err != nil {
		return Account{}, fmt.Errorf("error getting account: %w", err)
	}
	rows, err := s.q.ListWallets(ctx, row.ID)
	if err != nil {
		return Account{}, fmt.Errorf("error listing wallets: %w", err)
	}
	acct := toAccount(row)
	for _, r := range rows {
		if currency != "" && r.Currency != currency {
			continue
		}
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return Account{}, err
		}
		acct.Wallets = append(acct.Wallets, Wallet{Currency: r.Currency, Precision: r.Precision, ID: id})
	}
	return acct, nil
}

// listAccounts returns up to limit accounts whose address matches the Postgres regex re and sorts after
// after, ordered by address, with their wallets in currency order. Wallet IDs are not set. more reports
// whether further accounts match.
func (s *Service) listAccounts(ctx context.Context, re, after string, limit int) (accounts []Account, more bool, err error) {
	// One extra row tells whether another page exists.
	rows, err := s.q.ListAccounts(ctx, sqlc.ListAccountsParams{Pattern: re, After: after, Lim: int32(limit + 1)})
	if err != nil {
		return nil, false, fmt.Errorf("error listing accounts: %w", err)
	}
	if more = len(rows) > limit; more {
		rows = rows[:limit]
	}
	accounts = make([]Account, len(rows))
	for i, r := range rows {
		accounts[i] = Account{Address: r.Address, Type: AccountType(r.Type), AllowNegative: r.AllowNegative, CreatedAt: r.CreatedAt.Time}
		for j, c := range r.Currencies {
			accounts[i].Wallets = append(accounts[i].Wallets, Wallet{Currency: c, Precision: r.Precisions[j]})
		}
	}
	return accounts, more, nil
}

// addressWallet is a wallet with the address and type of its account.
type addressWallet struct {
	address string
	typ     AccountType
	Wallet
}

// patternWallets returns up to limit wallets of the accounts whose address matches the Postgres regex
// re, only in currency unless it is empty, ordered by address and currency. When more than limit wallets
// match, it returns none and tooMany true.
func (s *Service) patternWallets(ctx context.Context, re, currency string, limit int) (wallets []addressWallet, tooMany bool, err error) {
	// One extra row tells whether the pattern exceeds the limit.
	rows, err := s.q.ListWalletsByPattern(ctx, sqlc.ListWalletsByPatternParams{Pattern: re, Currency: currency, Lim: int32(limit + 1)})
	if err != nil {
		return nil, false, fmt.Errorf("error listing wallets: %w", err)
	}
	if len(rows) > limit {
		return nil, true, nil
	}
	wallets = make([]addressWallet, len(rows))
	for i, r := range rows {
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, false, err
		}
		wallets[i] = addressWallet{address: r.Address, typ: AccountType(r.Type), Wallet: Wallet{Currency: r.Currency, Precision: r.Precision, ID: id}}
	}
	return wallets, false, nil
}

// walletsByTBIDs returns the key of each wallet whose TigerBeetle account is in ids, by that id. Ids with
// no wallet are left out.
func (s *Service) walletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]walletKey, error) {
	rows, err := s.q.GetWalletsByTBIDs(ctx, idsToBytes(ids))
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	wallets := make(map[tb.Uint128]walletKey, len(rows))
	for _, r := range rows {
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, err
		}
		wallets[id] = walletKey{r.Address, r.Currency}
	}
	return wallets, nil
}

// storedWallets returns the stored wallet for each of keys whose account and currency are registered, by
// key. A key whose account has no wallet in the currency gets a storedWallet with a zero id.
func (s *Service) storedWallets(ctx context.Context, keys []walletKey) (map[walletKey]storedWallet, error) {
	var params sqlc.GetWalletsParams
	for _, k := range keys {
		params.Addresses = append(params.Addresses, k.address)
		params.Currencies = append(params.Currencies, k.currency)
	}
	rows, err := s.q.GetWallets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	byKey := make(map[walletKey]storedWallet, len(rows))
	for _, r := range rows {
		w := storedWallet{
			created:       r.TbCreated.Bool,
			ledger:        r.Ledger,
			precision:     r.Precision,
			accountID:     r.AccountID,
			typ:           AccountType(r.Type),
			allowNegative: r.AllowNegative,
		}
		if r.TbID != nil {
			if w.id, err = idFromBytes(r.TbID); err != nil {
				return nil, err
			}
		}
		byKey[walletKey{r.Address, r.Currency}] = w
	}
	return byKey, nil
}

// newWallet is a wallet to insert: its account's id, its currency and its TigerBeetle account id.
type newWallet struct {
	accountID int64
	currency  string
	id        tb.Uint128
}

// insertWallets stores wallets, skipping any whose account already has a wallet in the currency.
func (s *Service) insertWallets(ctx context.Context, wallets []newWallet) error {
	var params sqlc.InsertWalletsParams
	for _, w := range wallets {
		params.TbIds = append(params.TbIds, idToBytes(w.id))
		params.AccountIds = append(params.AccountIds, w.accountID)
		params.Currencies = append(params.Currencies, w.currency)
	}
	if err := s.q.InsertWallets(ctx, params); err != nil {
		return fmt.Errorf("error inserting wallets: %w", err)
	}
	return nil
}

// markWalletsCreated records that the TigerBeetle accounts ids of their wallets exist.
func (s *Service) markWalletsCreated(ctx context.Context, ids []tb.Uint128) error {
	if err := s.q.MarkWalletsTBCreated(ctx, idsToBytes(ids)); err != nil {
		return fmt.Errorf("error marking wallets created: %w", err)
	}
	return nil
}
