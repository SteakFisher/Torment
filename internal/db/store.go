package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
	"torment/internal/ledger"
)

// Store implements ledger.Store with the sqlc queries.
type Store struct {
	q    sqlc.Querier
	pool *pgxpool.Pool
}

// Fails to compile when Store no longer satisfies ledger.Store.
var _ ledger.Store = (*Store)(nil)

// NewStore returns a Store that runs its queries on pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: sqlc.New(pool), pool: pool}
}

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

func (s *Store) InsertCurrency(ctx context.Context, name string, precision int16) (c ledger.Currency, created bool, err error) {
	row, err := s.q.InsertCurrency(ctx, sqlc.InsertCurrencyParams{Name: name, Precision: precision})
	switch {
	case err == nil:
		created = true
	case errors.Is(err, pgx.ErrNoRows):
		if row, err = s.q.GetCurrency(ctx, name); err != nil {
			return ledger.Currency{}, false, fmt.Errorf("error getting currency: %w", err)
		}
	default:
		return ledger.Currency{}, false, fmt.Errorf("error inserting currency: %w", err)
	}
	return ledger.Currency{Name: row.Name, Precision: row.Precision, Ledger: row.Ledger}, created, nil
}

func (s *Store) ListCurrencies(ctx context.Context) ([]ledger.Currency, error) {
	// ponytail: no paging, every currency in one response.
	rows, err := s.q.ListCurrencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing currencies: %w", err)
	}
	currencies := make([]ledger.Currency, len(rows))
	for i, r := range rows {
		currencies[i] = ledger.Currency{Name: r.Name, Precision: r.Precision, Ledger: r.Ledger}
	}
	return currencies, nil
}

func (s *Store) Currencies(ctx context.Context, names []string) (map[string]ledger.Currency, error) {
	rows, err := s.q.GetCurrencies(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("error getting currencies: %w", err)
	}
	currencies := make(map[string]ledger.Currency, len(rows))
	for _, r := range rows {
		currencies[r.Name] = ledger.Currency{Name: r.Name, Precision: r.Precision, Ledger: r.Ledger}
	}
	return currencies, nil
}

func (s *Store) InsertAccount(ctx context.Context, address string, typ ledger.AccountType, allowNegative bool) (acct ledger.Account, created bool, err error) {
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
			return ledger.Account{}, false, fmt.Errorf("error getting account: %w", err)
		}
		return toAccount(row), false, nil
	default:
		return ledger.Account{}, false, fmt.Errorf("error inserting account: %w", err)
	}
}

func toAccount(row sqlc.Account) ledger.Account {
	return ledger.Account{
		Address:       row.Address,
		Type:          ledger.AccountType(row.Type),
		AllowNegative: row.AllowNegative,
		CreatedAt:     row.CreatedAt.Time,
	}
}

func (s *Store) AccountWallets(ctx context.Context, address, currency string) (ledger.Account, error) {
	row, err := s.q.GetAccount(ctx, address)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Account{}, fmt.Errorf("%w: account %q", ledger.ErrNotFound, address)
	}
	if err != nil {
		return ledger.Account{}, fmt.Errorf("error getting account: %w", err)
	}
	rows, err := s.q.ListWallets(ctx, row.ID)
	if err != nil {
		return ledger.Account{}, fmt.Errorf("error listing wallets: %w", err)
	}
	acct := toAccount(row)
	for _, r := range rows {
		if currency != "" && r.Currency != currency {
			continue
		}
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return ledger.Account{}, err
		}
		acct.Wallets = append(acct.Wallets, ledger.Wallet{Currency: r.Currency, Precision: r.Precision, ID: id})
	}
	return acct, nil
}

func (s *Store) ListAccounts(ctx context.Context, re, after string, limit int) (accounts []ledger.Account, more bool, err error) {
	// One extra row tells whether another page exists.
	rows, err := s.q.ListAccounts(ctx, sqlc.ListAccountsParams{Pattern: re, After: after, Lim: int32(limit + 1)})
	if err != nil {
		return nil, false, fmt.Errorf("error listing accounts: %w", err)
	}
	if more = len(rows) > limit; more {
		rows = rows[:limit]
	}
	accounts = make([]ledger.Account, len(rows))
	for i, r := range rows {
		accounts[i] = ledger.Account{Address: r.Address, Type: ledger.AccountType(r.Type), AllowNegative: r.AllowNegative, CreatedAt: r.CreatedAt.Time}
		for j, c := range r.Currencies {
			accounts[i].Wallets = append(accounts[i].Wallets, ledger.Wallet{Currency: c, Precision: r.Precisions[j]})
		}
	}
	return accounts, more, nil
}

func (s *Store) PatternWallets(ctx context.Context, re, currency string, limit int) (wallets []ledger.AddressWallet, tooMany bool, err error) {
	// One extra row tells whether the pattern exceeds the limit.
	rows, err := s.q.ListWalletsByPattern(ctx, sqlc.ListWalletsByPatternParams{Pattern: re, Currency: currency, Lim: int32(limit + 1)})
	if err != nil {
		return nil, false, fmt.Errorf("error listing wallets: %w", err)
	}
	if len(rows) > limit {
		return nil, true, nil
	}
	wallets = make([]ledger.AddressWallet, len(rows))
	for i, r := range rows {
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, false, err
		}
		wallets[i] = ledger.AddressWallet{Address: r.Address, Type: ledger.AccountType(r.Type), Wallet: ledger.Wallet{Currency: r.Currency, Precision: r.Precision, ID: id}}
	}
	return wallets, false, nil
}

func (s *Store) WalletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]ledger.WalletKey, error) {
	rows, err := s.q.GetWalletsByTBIDs(ctx, idsToBytes(ids))
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	wallets := make(map[tb.Uint128]ledger.WalletKey, len(rows))
	for _, r := range rows {
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, err
		}
		wallets[id] = ledger.WalletKey{Address: r.Address, Currency: r.Currency}
	}
	return wallets, nil
}

func (s *Store) StoredWallets(ctx context.Context, keys []ledger.WalletKey) (map[ledger.WalletKey]ledger.StoredWallet, error) {
	var params sqlc.GetWalletsParams
	for _, k := range keys {
		params.Addresses = append(params.Addresses, k.Address)
		params.Currencies = append(params.Currencies, k.Currency)
	}
	rows, err := s.q.GetWallets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	byKey := make(map[ledger.WalletKey]ledger.StoredWallet, len(rows))
	for _, r := range rows {
		w := ledger.StoredWallet{
			Created:       r.TbCreated,
			Ledger:        r.Ledger,
			Precision:     r.Precision,
			AccountID:     r.AccountID,
			Type:          ledger.AccountType(r.Type),
			AllowNegative: r.AllowNegative,
		}
		if r.TbID != nil {
			if w.ID, err = idFromBytes(r.TbID); err != nil {
				return nil, err
			}
		}
		byKey[ledger.WalletKey{Address: r.Address, Currency: r.Currency}] = w
	}
	return byKey, nil
}

func (s *Store) InsertWallets(ctx context.Context, wallets []ledger.NewWallet) error {
	var params sqlc.InsertWalletsParams
	for _, w := range wallets {
		params.TbIds = append(params.TbIds, idToBytes(w.ID))
		params.AccountIds = append(params.AccountIds, w.AccountID)
		params.Currencies = append(params.Currencies, w.Currency)
	}
	if err := s.q.InsertWallets(ctx, params); err != nil {
		return fmt.Errorf("error inserting wallets: %w", err)
	}
	return nil
}

func (s *Store) MarkWalletsCreated(ctx context.Context, ids []tb.Uint128) error {
	if err := s.q.MarkWalletsTBCreated(ctx, idsToBytes(ids)); err != nil {
		return fmt.Errorf("error marking wallets created: %w", err)
	}
	return nil
}

// Ping returns an error when Postgres can't be reached.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
