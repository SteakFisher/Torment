package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
	"torment/internal/ledger"
)

// fakeQuerier implements sqlc.Querier with per-test funcs; calling a method whose func is nil panics.
type fakeQuerier struct {
	sqlc.Querier
	insertCurrency       func(ctx context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error)
	getCurrency          func(ctx context.Context, name string) (sqlc.Currency, error)
	insertAccount        func(ctx context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error)
	getAccount           func(ctx context.Context, address string) (sqlc.Account, error)
	listWallets          func(ctx context.Context, accountID int64) ([]sqlc.ListWalletsRow, error)
	getWallets           func(ctx context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error)
	listAccounts         func(ctx context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error)
	listWalletsByPattern func(ctx context.Context, arg sqlc.ListWalletsByPatternParams) ([]sqlc.ListWalletsByPatternRow, error)
}

func (f *fakeQuerier) InsertCurrency(ctx context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error) {
	return f.insertCurrency(ctx, arg)
}

func (f *fakeQuerier) GetCurrency(ctx context.Context, name string) (sqlc.Currency, error) {
	return f.getCurrency(ctx, name)
}

func (f *fakeQuerier) InsertAccount(ctx context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error) {
	return f.insertAccount(ctx, arg)
}

func (f *fakeQuerier) GetAccount(ctx context.Context, address string) (sqlc.Account, error) {
	return f.getAccount(ctx, address)
}

func (f *fakeQuerier) ListWallets(ctx context.Context, accountID int64) ([]sqlc.ListWalletsRow, error) {
	return f.listWallets(ctx, accountID)
}

func (f *fakeQuerier) GetWallets(ctx context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error) {
	return f.getWallets(ctx, arg)
}

func (f *fakeQuerier) ListAccounts(ctx context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error) {
	return f.listAccounts(ctx, arg)
}

func (f *fakeQuerier) ListWalletsByPattern(ctx context.Context, arg sqlc.ListWalletsByPatternParams) ([]sqlc.ListWalletsByPatternRow, error) {
	return f.listWalletsByPattern(ctx, arg)
}

func TestIDBytes(t *testing.T) {
	id := tb.ID()
	got, err := idFromBytes(idToBytes(id))
	if err != nil || got != id {
		t.Fatalf("round trip = %v, %v; want %v", got, err, id)
	}
	for _, n := range []int{0, 15, 17} {
		if _, err := idFromBytes(make([]byte, n)); err == nil {
			t.Errorf("idFromBytes(%d bytes) err = nil, want an error", n)
		}
	}
}

func TestInsertCurrency(t *testing.T) {
	stored := sqlc.Currency{Name: "USD", Precision: 4, Ledger: 3}
	tests := []struct {
		name        string
		insertErr   error
		getErr      error
		want        ledger.Currency
		wantCreated bool
		wantGet     bool
		wantErr     bool
	}{
		{name: "created", want: ledger.Currency{Name: "USD", Precision: 2, Ledger: 7}, wantCreated: true},
		{name: "exists", insertErr: pgx.ErrNoRows, want: ledger.Currency{Name: "USD", Precision: 4, Ledger: 3}, wantGet: true},
		{name: "get fails", insertErr: pgx.ErrNoRows, getErr: errors.New("boom"), wantGet: true, wantErr: true},
		{name: "insert fails", insertErr: errors.New("boom"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			s := &Store{q: &fakeQuerier{
				insertCurrency: func(_ context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error) {
					if tt.insertErr != nil {
						return sqlc.Currency{}, tt.insertErr
					}
					return sqlc.Currency{Name: arg.Name, Precision: arg.Precision, Ledger: 7}, nil
				},
				getCurrency: func(context.Context, string) (sqlc.Currency, error) {
					got = true
					return stored, tt.getErr
				},
			}}
			c, created, err := s.InsertCurrency(context.Background(), "USD", 2)
			if got != tt.wantGet {
				t.Fatalf("get called = %v, want %v", got, tt.wantGet)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (c != tt.want || created != tt.wantCreated) {
				t.Fatalf("got %+v, created %v; want %+v, %v", c, created, tt.want, tt.wantCreated)
			}
		})
	}
}

func TestInsertAccount(t *testing.T) {
	stored := sqlc.Account{Address: "rider:1:wallet", Type: sqlc.AccountTypeAsset, AllowNegative: true}
	tests := []struct {
		name        string
		insertErr   error
		getErr      error
		want        ledger.Account
		wantCreated bool
		wantGet     bool
		wantErr     bool
	}{
		{name: "created", want: ledger.Account{Address: "rider:1:wallet", Type: ledger.TypeLiability}, wantCreated: true},
		{name: "exists", insertErr: pgx.ErrNoRows, want: ledger.Account{Address: "rider:1:wallet", Type: ledger.TypeAsset, AllowNegative: true}, wantGet: true},
		{name: "get fails", insertErr: pgx.ErrNoRows, getErr: errors.New("boom"), wantGet: true, wantErr: true},
		{name: "insert fails", insertErr: errors.New("boom"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			s := &Store{q: &fakeQuerier{
				insertAccount: func(_ context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error) {
					if tt.insertErr != nil {
						return sqlc.Account{}, tt.insertErr
					}
					return sqlc.Account{Address: arg.Address, Type: arg.Type, AllowNegative: arg.AllowNegative}, nil
				},
				getAccount: func(context.Context, string) (sqlc.Account, error) {
					got = true
					return stored, tt.getErr
				},
			}}
			acct, created, err := s.InsertAccount(context.Background(), "rider:1:wallet", ledger.TypeLiability, false)
			if got != tt.wantGet {
				t.Fatalf("get called = %v, want %v", got, tt.wantGet)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (created != tt.wantCreated || acct.Address != tt.want.Address || acct.Type != tt.want.Type || acct.AllowNegative != tt.want.AllowNegative) {
				t.Fatalf("got %+v, created %v; want %+v, %v", acct, created, tt.want, tt.wantCreated)
			}
		})
	}
}

func TestAccountWallets(t *testing.T) {
	usd, aed := tb.ID(), tb.ID()
	s := &Store{q: &fakeQuerier{
		getAccount: func(_ context.Context, address string) (sqlc.Account, error) {
			if address != "rider:1:wallet" {
				return sqlc.Account{}, pgx.ErrNoRows
			}
			return sqlc.Account{ID: 7, Address: address, Type: sqlc.AccountTypeLiability}, nil
		},
		listWallets: func(_ context.Context, accountID int64) ([]sqlc.ListWalletsRow, error) {
			if accountID != 7 {
				t.Fatalf("listed wallets of account %d, want 7", accountID)
			}
			return []sqlc.ListWalletsRow{{TbID: idToBytes(aed), Currency: "AED", Precision: 2}, {TbID: idToBytes(usd), Currency: "USD", Precision: 3}}, nil
		},
	}}
	ctx := context.Background()
	if _, err := s.AccountWallets(ctx, "nobody", ""); !errors.Is(err, ledger.ErrAccountNotFound) {
		t.Fatalf("unknown address err = %v, want ErrAccountNotFound", err)
	}
	acct, err := s.AccountWallets(ctx, "rider:1:wallet", "")
	want := []ledger.Wallet{{Currency: "AED", Precision: 2, ID: aed}, {Currency: "USD", Precision: 3, ID: usd}}
	if err != nil || acct.Type != ledger.TypeLiability || len(acct.Wallets) != 2 || acct.Wallets[0] != want[0] || acct.Wallets[1] != want[1] {
		t.Fatalf("got %+v, err %v; want wallets %+v", acct, err, want)
	}
	acct, err = s.AccountWallets(ctx, "rider:1:wallet", "USD")
	if err != nil || len(acct.Wallets) != 1 || acct.Wallets[0] != want[1] {
		t.Fatalf("USD only: got %+v, err %v", acct, err)
	}
}

func TestListAccounts(t *testing.T) {
	rows := []sqlc.ListAccountsRow{
		{Address: "rider:1:bonus", Type: sqlc.AccountTypeLiability},
		{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability, Currencies: []string{"AED", "USD"}, Precisions: []int16{2, 3}},
		{Address: "rider:2:wallet", Type: sqlc.AccountTypeLiability},
	}
	tests := []struct {
		name     string
		limit    int
		wantMore bool
		wantN    int
	}{
		{name: "more", limit: 2, wantMore: true, wantN: 2},
		{name: "exact", limit: 3, wantN: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got sqlc.ListAccountsParams
			s := &Store{q: &fakeQuerier{listAccounts: func(_ context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error) {
				got = arg
				return rows[:min(len(rows), int(arg.Lim))], nil
			}}}
			accounts, more, err := s.ListAccounts(context.Background(), "^rider:", "rider:0", tt.limit)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != (sqlc.ListAccountsParams{Pattern: "^rider:", After: "rider:0", Lim: int32(tt.limit + 1)}) {
				t.Fatalf("query params = %+v", got)
			}
			if len(accounts) != tt.wantN || more != tt.wantMore {
				t.Fatalf("got %d accounts, more %v; want %d, %v", len(accounts), more, tt.wantN, tt.wantMore)
			}
			w := accounts[1].Wallets
			if len(w) != 2 || w[0] != (ledger.Wallet{Currency: "AED", Precision: 2}) || w[1] != (ledger.Wallet{Currency: "USD", Precision: 3}) {
				t.Fatalf("wallets = %+v", w)
			}
		})
	}
}

func TestPatternWallets(t *testing.T) {
	id := tb.ID()
	row := sqlc.ListWalletsByPatternRow{Address: "rider:1:card", Type: sqlc.AccountTypeAsset, Currency: "USD", Precision: 2, TbID: idToBytes(id)}
	var got sqlc.ListWalletsByPatternParams
	rows := []sqlc.ListWalletsByPatternRow{row}
	s := &Store{q: &fakeQuerier{listWalletsByPattern: func(_ context.Context, arg sqlc.ListWalletsByPatternParams) ([]sqlc.ListWalletsByPatternRow, error) {
		got = arg
		return rows, nil
	}}}
	ctx := context.Background()
	wallets, tooMany, err := s.PatternWallets(ctx, "^rider:", "USD", 1)
	if got != (sqlc.ListWalletsByPatternParams{Pattern: "^rider:", Currency: "USD", Lim: 2}) {
		t.Fatalf("query params = %+v", got)
	}
	want := ledger.AddressWallet{Address: "rider:1:card", Type: ledger.TypeAsset, Wallet: ledger.Wallet{Currency: "USD", Precision: 2, ID: id}}
	if err != nil || tooMany || len(wallets) != 1 || wallets[0] != want {
		t.Fatalf("got %+v, tooMany %v, err %v; want [%+v]", wallets, tooMany, err, want)
	}
	rows = []sqlc.ListWalletsByPatternRow{row, row}
	if wallets, tooMany, err := s.PatternWallets(ctx, "^rider:", "USD", 1); err != nil || !tooMany || wallets != nil {
		t.Fatalf("over the limit: got %+v, tooMany %v, err %v; want none and tooMany", wallets, tooMany, err)
	}
}

func TestStoredWallets(t *testing.T) {
	id := tb.ID()
	var got sqlc.GetWalletsParams
	s := &Store{q: &fakeQuerier{getWallets: func(_ context.Context, arg sqlc.GetWalletsParams) ([]sqlc.GetWalletsRow, error) {
		got = arg
		return []sqlc.GetWalletsRow{
			{Address: "world", Currency: "USD", AccountID: 1, Type: sqlc.AccountTypeAsset, AllowNegative: true, Ledger: 7, Precision: 2, TbID: idToBytes(id), TbCreated: true},
			{Address: "rider:1:wallet", Currency: "USD", AccountID: 2, Type: sqlc.AccountTypeLiability, Ledger: 7, Precision: 2},
		}, nil
	}}}
	world, rider := ledger.WalletKey{Address: "world", Currency: "USD"}, ledger.WalletKey{Address: "rider:1:wallet", Currency: "USD"}
	wallets, err := s.StoredWallets(context.Background(), []ledger.WalletKey{world, rider})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got.Addresses) != 2 || got.Addresses[1] != "rider:1:wallet" || got.Currencies[0] != "USD" {
		t.Fatalf("query params = %+v", got)
	}
	wantWorld := ledger.StoredWallet{ID: id, Created: true, Ledger: 7, Precision: 2, AccountID: 1, Type: ledger.TypeAsset, AllowNegative: true}
	wantRider := ledger.StoredWallet{Ledger: 7, Precision: 2, AccountID: 2, Type: ledger.TypeLiability}
	if len(wallets) != 2 || wallets[world] != wantWorld || wallets[rider] != wantRider {
		t.Fatalf("got %+v; want world %+v, rider %+v", wallets, wantWorld, wantRider)
	}
}
