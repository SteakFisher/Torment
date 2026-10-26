package ledger

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"torment/internal/db/sqlc"
)

func TestParseCurrencyCode(t *testing.T) {
	tests := []struct {
		code      string
		name      string
		precision int16
		wantErr   bool
	}{
		{code: "USD/2", name: "USD", precision: 2},
		{code: "JPY", name: "JPY", precision: 0},
		{code: "BTC/8", name: "BTC", precision: 8},
		{code: "USD_C/18", name: "USD_C", precision: 18},
		{code: "A1/0", name: "A1", precision: 0},
		{code: "", wantErr: true},
		{code: "usd/2", wantErr: true},
		{code: "1USD", wantErr: true},
		{code: "_USD", wantErr: true},
		{code: "USD/", wantErr: true},
		{code: "USD/19", wantErr: true},
		{code: "USD/-1", wantErr: true},
		{code: "USD/2/3", wantErr: true},
		{code: "US D", wantErr: true},
		{code: "USD/99999999999999999999", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			name, precision, err := ParseCurrencyCode(tt.code)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if name != tt.name || precision != tt.precision {
				t.Fatalf("got (%q, %d), want (%q, %d)", name, precision, tt.name, tt.precision)
			}
		})
	}
}

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

func TestRegisterCurrency(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		insertErr error
		wantErr   error
		wantCall  bool
	}{
		{name: "invalid code", code: "usd", wantErr: ErrInvalid},
		{name: "unique violation", code: "USD/2", insertErr: &pgconn.PgError{Code: "23505"}, wantErr: ErrConflict, wantCall: true},
		{name: "success", code: "USD/2", wantCall: true},
		{name: "success without precision", code: "JPY", wantCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			q := &fakeQuerier{insertCurrency: func(_ context.Context, arg sqlc.InsertCurrencyParams) (sqlc.Currency, error) {
				called = true
				return sqlc.Currency{Name: arg.Name, Precision: arg.Precision, Ledger: 7}, tt.insertErr
			}}
			got, err := New(q, nil).RegisterCurrency(context.Background(), tt.code)
			if called != tt.wantCall {
				t.Fatalf("InsertCurrency called = %v, want %v", called, tt.wantCall)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			name, precision, _ := ParseCurrencyCode(tt.code)
			if want := (Currency{Name: name, Precision: precision, Ledger: 7}); got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}
