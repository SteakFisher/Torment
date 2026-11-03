package ledger

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// fakeQuerier implements sqlc.Querier with per-test funcs; calling a method whose func is nil panics.
type fakeQuerier struct {
	sqlc.Querier
	insertCurrency       func(ctx context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error)
	insertAccount        func(ctx context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error)
	getAccount           func(ctx context.Context, address string) (sqlc.Account, error)
	getCurrencies        func(ctx context.Context, names []string) ([]sqlc.Currency, error)
	listWallets          func(ctx context.Context, accountID int64) ([]sqlc.ListWalletsRow, error)
	getWallets           func(ctx context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error)
	insertWallets        func(ctx context.Context, arg sqlc.InsertWalletsParams) error
	markWalletsTBCreated func(ctx context.Context, tbIds [][]byte) error
	getWalletsByTBIDs    func(ctx context.Context, tbIds [][]byte) ([]sqlc.GetWalletsByTBIDsRow, error)
	listAccounts         func(ctx context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error)
}

var _ sqlc.Querier = (*fakeQuerier)(nil)

func (f *fakeQuerier) InsertCurrency(ctx context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error) {
	return f.insertCurrency(ctx, arg)
}

func (f *fakeQuerier) InsertAccount(ctx context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error) {
	return f.insertAccount(ctx, arg)
}

func (f *fakeQuerier) GetAccount(ctx context.Context, address string) (sqlc.Account, error) {
	return f.getAccount(ctx, address)
}

func (f *fakeQuerier) GetCurrencies(ctx context.Context, names []string) ([]sqlc.Currency, error) {
	return f.getCurrencies(ctx, names)
}

func (f *fakeQuerier) ListWallets(ctx context.Context, accountID int64) ([]sqlc.ListWalletsRow, error) {
	return f.listWallets(ctx, accountID)
}

func (f *fakeQuerier) GetWallets(ctx context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error) {
	return f.getWallets(ctx, arg)
}

func (f *fakeQuerier) InsertWallets(ctx context.Context, arg sqlc.InsertWalletsParams) error {
	return f.insertWallets(ctx, arg)
}

func (f *fakeQuerier) MarkWalletsTBCreated(ctx context.Context, tbIds [][]byte) error {
	return f.markWalletsTBCreated(ctx, tbIds)
}

func (f *fakeQuerier) GetWalletsByTBIDs(ctx context.Context, tbIds [][]byte) ([]sqlc.GetWalletsByTBIDsRow, error) {
	return f.getWalletsByTBIDs(ctx, tbIds)
}

func (f *fakeQuerier) ListAccounts(ctx context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error) {
	return f.listAccounts(ctx, arg)
}

// fakeWallet is a wallet row in a walletStore.
type fakeWallet struct {
	tbID    []byte
	created bool
}

// walletStore holds accounts, currencies and wallets in memory for the wallet queries of a fakeQuerier,
// and records the calls made to them. Every currency has precision 2.
type walletStore struct {
	accounts map[string]sqlc.Account // by address
	ledgers  map[string]int32        // by currency name
	wallets  map[walletKey]*fakeWallet
	gets     int
	inserted []walletKey
	marked   [][]byte
}

func newWalletStore() *walletStore {
	return &walletStore{accounts: map[string]sqlc.Account{}, ledgers: map[string]int32{}, wallets: map[walletKey]*fakeWallet{}}
}

// querier returns a fakeQuerier whose wallet queries run against w.
func (w *walletStore) querier() *fakeQuerier {
	return &fakeQuerier{
		getWallets: func(_ context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error) {
			w.gets++
			var rows []sqlc.GetWalletsRow
			for i, address := range arg.Addresses {
				a, ok := w.accounts[address]
				ledger, ok2 := w.ledgers[arg.Currencies[i]]
				if !ok || !ok2 {
					continue
				}
				r := sqlc.GetWalletsRow{Address: address, Currency: arg.Currencies[i], AccountID: a.ID, Type: a.Type, AllowNegative: a.AllowNegative, Ledger: ledger, Precision: 2}
				if fw, ok := w.wallets[walletKey{address, arg.Currencies[i]}]; ok {
					r.TbID, r.TbCreated = fw.tbID, pgtype.Bool{Bool: fw.created, Valid: true}
				}
				rows = append(rows, r)
			}
			return rows, nil
		},
		insertWallets: func(_ context.Context, arg sqlc.InsertWalletsParams) error {
			for i, id := range arg.TbIds {
				for address, a := range w.accounts {
					k := walletKey{address, arg.Currencies[i]}
					if a.ID == arg.AccountIds[i] && w.wallets[k] == nil {
						w.inserted = append(w.inserted, k)
						w.wallets[k] = &fakeWallet{tbID: id}
					}
				}
			}
			return nil
		},
		markWalletsTBCreated: func(_ context.Context, tbIds [][]byte) error {
			w.marked = append(w.marked, tbIds...)
			for _, fw := range w.wallets {
				if slices.ContainsFunc(tbIds, func(id []byte) bool { return slices.Equal(id, fw.tbID) }) {
					fw.created = true
				}
			}
			return nil
		},
	}
}

// fakeTB implements tbClient with per-test funcs; calling a method whose func is nil panics.
type fakeTB struct {
	createAccounts  func(accounts []tb.Account) ([]tb.CreateAccountResult, error)
	lookupAccounts  func(ids []tb.Uint128) ([]tb.Account, error)
	createTransfers func(transfers []tb.Transfer) ([]tb.CreateTransferResult, error)
	lookupTransfers func(ids []tb.Uint128) ([]tb.Transfer, error)
	queryTransfers  func(filter tb.QueryFilter) ([]tb.Transfer, error)
}

var _ tbClient = (*fakeTB)(nil)

func (f *fakeTB) CreateAccounts(accounts []tb.Account) ([]tb.CreateAccountResult, error) {
	return f.createAccounts(accounts)
}

func (f *fakeTB) LookupAccounts(ids []tb.Uint128) ([]tb.Account, error) {
	return f.lookupAccounts(ids)
}

func (f *fakeTB) CreateTransfers(transfers []tb.Transfer) ([]tb.CreateTransferResult, error) {
	return f.createTransfers(transfers)
}

func (f *fakeTB) LookupTransfers(ids []tb.Uint128) ([]tb.Transfer, error) {
	return f.lookupTransfers(ids)
}

func (f *fakeTB) QueryTransfers(filter tb.QueryFilter) ([]tb.Transfer, error) {
	return f.queryTransfers(filter)
}
