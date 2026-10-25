package ledger

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"torment/internal/db"
)

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		address string
		wantErr bool
	}{
		{address: "world"},
		{address: "rider:42:wallet"},
		{address: "user:3f2a-9c:main"},
		{address: "A_b-1"},
		{address: strings.Repeat("a", 255)},
		{address: strings.Repeat("a", 256), wantErr: true},
		{address: "", wantErr: true},
		{address: "rider:", wantErr: true},
		{address: ":rider", wantErr: true},
		{address: "a::b", wantErr: true},
		{address: "a b", wantErr: true},
		{address: "a/b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			err := validateAddress(tt.address)
			if tt.wantErr != errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseAccountType(t *testing.T) {
	for _, s := range []string{"asset", "expense", "liability", "equity", "revenue"} {
		if got, err := ParseAccountType(s); err != nil || string(got) != s {
			t.Errorf("ParseAccountType(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "Asset", "debit", "assets"} {
		if _, err := ParseAccountType(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseAccountType(%q) err = %v, want ErrInvalid", s, err)
		}
	}
}

func TestRegisterAccount(t *testing.T) {
	stored := db.Account{Address: "rider:1:wallet", Type: db.AccountTypeLiability}
	valid := RegisterAccountParams{Address: "rider:1:wallet", Type: "liability"}
	with := func(f func(*RegisterAccountParams)) RegisterAccountParams {
		p := valid
		f(&p)
		return p
	}

	tests := []struct {
		name        string
		params      RegisterAccountParams
		insertErr   error
		wantCreated bool
		wantErr     error
		wantInsert  bool
		wantGet     bool
	}{
		{name: "invalid address", params: with(func(p *RegisterAccountParams) { p.Address = "a::b" }), wantErr: ErrInvalid},
		{name: "invalid type", params: with(func(p *RegisterAccountParams) { p.Type = "debit" }), wantErr: ErrInvalid},
		{name: "created", params: valid, wantCreated: true, wantInsert: true},
		{name: "replay", params: valid, insertErr: pgx.ErrNoRows, wantInsert: true, wantGet: true},
		{name: "different type", params: with(func(p *RegisterAccountParams) { p.Type = "asset" }), insertErr: pgx.ErrNoRows, wantErr: ErrConflict, wantInsert: true, wantGet: true},
		{name: "different allow_negative", params: with(func(p *RegisterAccountParams) { p.AllowNegative = true }), insertErr: pgx.ErrNoRows, wantErr: ErrConflict, wantInsert: true, wantGet: true},
		{name: "insert fails", params: valid, insertErr: errors.New("boom"), wantInsert: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inserted, got := false, false
			q := &fakeQuerier{
				insertAccount: func(_ context.Context, arg db.InsertAccountParams) (db.Account, error) {
					inserted = true
					if tt.insertErr != nil {
						return db.Account{}, tt.insertErr
					}
					return db.Account{Address: arg.Address, Type: arg.Type, AllowNegative: arg.AllowNegative}, nil
				},
				getAccount: func(context.Context, string) (db.Account, error) {
					got = true
					return stored, nil
				},
			}
			acct, created, err := New(q).RegisterAccount(context.Background(), tt.params)
			if inserted != tt.wantInsert || got != tt.wantGet {
				t.Fatalf("insert called = %v, get called = %v; want %v, %v", inserted, got, tt.wantInsert, tt.wantGet)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.insertErr != nil && !errors.Is(tt.insertErr, pgx.ErrNoRows) {
				if err == nil || errors.Is(err, ErrConflict) {
					t.Fatalf("err = %v, want a wrapped unexpected error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if created != tt.wantCreated || acct.Address != valid.Address || acct.Type != TypeLiability {
				t.Fatalf("got %+v, created %v", acct, created)
			}
		})
	}
}
