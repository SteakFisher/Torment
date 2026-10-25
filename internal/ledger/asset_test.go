package ledger

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"torment/internal/db"
)

func TestParseAssetCode(t *testing.T) {
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
			name, precision, err := ParseAssetCode(tt.code)
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

// fakeQuerier implements db.Querier with per-test funcs; calling a method whose func is nil panics.
type fakeQuerier struct {
	db.Querier
	insertAsset   func(ctx context.Context, arg db.InsertAssetParams) (db.Asset, error)
	insertAddress func(ctx context.Context, arg db.InsertAddressParams) (db.Address, error)
	getAddress    func(ctx context.Context, address string) (db.Address, error)
}

var _ db.Querier = (*fakeQuerier)(nil)

func (f *fakeQuerier) InsertAsset(ctx context.Context, arg db.InsertAssetParams) (db.Asset, error) {
	return f.insertAsset(ctx, arg)
}

func (f *fakeQuerier) InsertAddress(ctx context.Context, arg db.InsertAddressParams) (db.Address, error) {
	return f.insertAddress(ctx, arg)
}

func (f *fakeQuerier) GetAddress(ctx context.Context, address string) (db.Address, error) {
	return f.getAddress(ctx, address)
}

func TestRegisterAsset(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		ledger    int32
		insertErr error
		wantErr   error
		wantCall  bool
	}{
		{name: "invalid code", code: "usd", ledger: 1, wantErr: ErrInvalid},
		{name: "invalid ledger", code: "USD/2", ledger: 0, wantErr: ErrInvalid},
		{name: "unique violation", code: "USD/2", ledger: 1, insertErr: &pgconn.PgError{Code: "23505"}, wantErr: ErrConflict, wantCall: true},
		{name: "success", code: "USD/2", ledger: 1, wantCall: true},
		{name: "success without precision", code: "JPY", ledger: 3, wantCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			q := &fakeQuerier{insertAsset: func(_ context.Context, arg db.InsertAssetParams) (db.Asset, error) {
				called = true
				return db.Asset(arg), tt.insertErr
			}}
			got, err := New(q).RegisterAsset(context.Background(), tt.code, tt.ledger)
			if called != tt.wantCall {
				t.Fatalf("InsertAsset called = %v, want %v", called, tt.wantCall)
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
			name, precision, _ := ParseAssetCode(tt.code)
			if want := (Asset{Name: name, Precision: precision, Ledger: tt.ledger}); got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}
