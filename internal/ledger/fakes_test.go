package ledger

import (
	"context"
	"slices"
	"sync"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// fakeStore implements Store with per-test funcs; calling a method whose func is nil panics.
type fakeStore struct {
	Store
	insertCurrency     func(ctx context.Context, name string, precision int16) (Currency, bool, error)
	currencies         func(ctx context.Context, names []string) (map[string]Currency, error)
	listCurrencies     func(ctx context.Context) ([]Currency, error)
	insertAccount      func(ctx context.Context, address string, typ AccountType, allowNegative bool) (Account, bool, error)
	accountWallets     func(ctx context.Context, address, currency string) (Account, error)
	listAccounts       func(ctx context.Context, re, after string, limit int) ([]Account, bool, error)
	patternWallets     func(ctx context.Context, re, currency string, limit int) ([]AddressWallet, bool, error)
	walletsByTBIDs     func(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]WalletKey, error)
	storedWallets      func(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error)
	insertWallets      func(ctx context.Context, wallets []NewWallet) error
	markWalletsCreated func(ctx context.Context, ids []tb.Uint128) error
	ping               func(ctx context.Context) error
}

var _ Store = (*fakeStore)(nil)

func (f *fakeStore) InsertCurrency(ctx context.Context, name string, precision int16) (Currency, bool, error) {
	return f.insertCurrency(ctx, name, precision)
}

func (f *fakeStore) ListCurrencies(ctx context.Context) ([]Currency, error) {
	return f.listCurrencies(ctx)
}

func (f *fakeStore) Currencies(ctx context.Context, names []string) (map[string]Currency, error) {
	return f.currencies(ctx, names)
}

func (f *fakeStore) InsertAccount(ctx context.Context, address string, typ AccountType, allowNegative bool) (Account, bool, error) {
	return f.insertAccount(ctx, address, typ, allowNegative)
}

func (f *fakeStore) AccountWallets(ctx context.Context, address, currency string) (Account, error) {
	return f.accountWallets(ctx, address, currency)
}

func (f *fakeStore) ListAccounts(ctx context.Context, re, after string, limit int) ([]Account, bool, error) {
	return f.listAccounts(ctx, re, after, limit)
}

func (f *fakeStore) PatternWallets(ctx context.Context, re, currency string, limit int) ([]AddressWallet, bool, error) {
	return f.patternWallets(ctx, re, currency, limit)
}

func (f *fakeStore) WalletsByTBIDs(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]WalletKey, error) {
	return f.walletsByTBIDs(ctx, ids)
}

func (f *fakeStore) StoredWallets(ctx context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
	return f.storedWallets(ctx, keys)
}

func (f *fakeStore) InsertWallets(ctx context.Context, wallets []NewWallet) error {
	return f.insertWallets(ctx, wallets)
}

func (f *fakeStore) MarkWalletsCreated(ctx context.Context, ids []tb.Uint128) error {
	return f.markWalletsCreated(ctx, ids)
}

func (f *fakeStore) Ping(ctx context.Context) error {
	return f.ping(ctx)
}

// fakeAccount is an account in a walletStore.
type fakeAccount struct {
	id  int64
	typ AccountType
}

// fakeWallet is a wallet in a walletStore.
type fakeWallet struct {
	id      tb.Uint128
	created bool
}

// walletStore holds accounts, currencies and wallets in memory for the wallet methods of a fakeStore,
// and records the calls made to them. Every currency has precision 2.
type walletStore struct {
	accounts map[string]fakeAccount // by address
	ledgers  map[string]int32       // by currency name
	wallets  map[WalletKey]*fakeWallet
	gets     int
	inserted []WalletKey
	marked   []tb.Uint128
}

func newWalletStore() *walletStore {
	return &walletStore{accounts: map[string]fakeAccount{}, ledgers: map[string]int32{}, wallets: map[WalletKey]*fakeWallet{}}
}

// store returns a fakeStore whose wallet methods run against w.
func (w *walletStore) store() *fakeStore {
	return &fakeStore{
		storedWallets: func(_ context.Context, keys []WalletKey) (map[WalletKey]StoredWallet, error) {
			w.gets++
			out := make(map[WalletKey]StoredWallet)
			for _, k := range keys {
				a, ok := w.accounts[k.Address]
				ledger, ok2 := w.ledgers[k.Currency]
				if !ok || !ok2 {
					continue
				}
				sw := StoredWallet{Ledger: ledger, Precision: 2, AccountID: a.id, Type: a.typ}
				if fw, ok := w.wallets[k]; ok {
					sw.ID, sw.Created = fw.id, fw.created
				}
				out[k] = sw
			}
			return out, nil
		},
		insertWallets: func(_ context.Context, wallets []NewWallet) error {
			for _, nw := range wallets {
				for address, a := range w.accounts {
					k := WalletKey{address, nw.Currency}
					if a.id == nw.AccountID && w.wallets[k] == nil {
						w.inserted = append(w.inserted, k)
						w.wallets[k] = &fakeWallet{id: nw.ID}
					}
				}
			}
			return nil
		},
		markWalletsCreated: func(_ context.Context, ids []tb.Uint128) error {
			w.marked = append(w.marked, ids...)
			for _, fw := range w.wallets {
				if slices.Contains(ids, fw.id) {
					fw.created = true
				}
			}
			return nil
		},
	}
}

// fakeTB implements tbWriter and tbReader with per-test funcs, one call at a time; calling a method
// whose func is nil panics.
type fakeTB struct {
	mu               sync.Mutex
	createAccounts   func(accounts []tb.Account) ([]tb.CreateAccountResult, error)
	lookupAccounts   func(ids []tb.Uint128) ([]tb.Account, error)
	createTransfers  func(transfers []tb.Transfer) ([]tb.CreateTransferResult, error)
	lookupTransfers  func(ids []tb.Uint128) ([]tb.Transfer, error)
	queryTransfers   func(filter tb.QueryFilter) ([]tb.Transfer, error)
	accountBalances  func(filter tb.AccountFilter) ([]tb.AccountBalance, error)
	accountTransfers func(filter tb.AccountFilter) ([]tb.Transfer, error)
}

var (
	_ tbWriter = (*fakeTB)(nil)
	_ tbReader = (*fakeTB)(nil)
)

func (f *fakeTB) CreateAccounts(accounts []tb.Account) ([]tb.CreateAccountResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createAccounts(accounts)
}

func (f *fakeTB) LookupAccounts(ids []tb.Uint128) ([]tb.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lookupAccounts(ids)
}

func (f *fakeTB) CreateTransfers(transfers []tb.Transfer) ([]tb.CreateTransferResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createTransfers(transfers)
}

func (f *fakeTB) LookupTransfers(ids []tb.Uint128) ([]tb.Transfer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lookupTransfers(ids)
}

func (f *fakeTB) QueryTransfers(filter tb.QueryFilter) ([]tb.Transfer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queryTransfers(filter)
}

func (f *fakeTB) GetAccountBalances(filter tb.AccountFilter) ([]tb.AccountBalance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.accountBalances(filter)
}

func (f *fakeTB) GetAccountTransfers(filter tb.AccountFilter) ([]tb.Transfer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.accountTransfers(filter)
}
