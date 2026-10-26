package ledger

import (
	"context"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// fakeQuerier implements sqlc.Querier with per-test funcs; calling a method whose func is nil panics.
type fakeQuerier struct {
	sqlc.Querier
	insertCurrency func(ctx context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error)
	insertAccount  func(ctx context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error)
	getAccount     func(ctx context.Context, address string) (sqlc.Account, error)
	getCurrencies  func(ctx context.Context, names []string) ([]sqlc.Currency, error)
	insertWallet   func(ctx context.Context, arg sqlc.InsertWalletParams) (sqlc.Wallet, error)
	getWallet      func(ctx context.Context, arg sqlc.GetWalletParams) (sqlc.Wallet, error)
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

func (f *fakeQuerier) InsertWallet(ctx context.Context, arg sqlc.InsertWalletParams) (sqlc.Wallet, error) {
	return f.insertWallet(ctx, arg)
}

func (f *fakeQuerier) GetWallet(ctx context.Context, arg sqlc.GetWalletParams) (sqlc.Wallet, error) {
	return f.getWallet(ctx, arg)
}

// fakeTB implements tbClient with a per-test func; calling CreateAccounts when it is nil panics.
type fakeTB struct {
	createAccounts func(accounts []tb.Account) ([]tb.CreateAccountResult, error)
}

var _ tbClient = (*fakeTB)(nil)

func (f *fakeTB) CreateAccounts(accounts []tb.Account) ([]tb.CreateAccountResult, error) {
	return f.createAccounts(accounts)
}
