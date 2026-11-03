package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"torment/internal/db/sqlc"
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
	stored := sqlc.Account{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability}
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
				insertAccount: func(_ context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error) {
					inserted = true
					if tt.insertErr != nil {
						return sqlc.Account{}, tt.insertErr
					}
					return sqlc.Account{Address: arg.Address, Type: arg.Type, AllowNegative: arg.AllowNegative}, nil
				},
				getAccount: func(context.Context, string) (sqlc.Account, error) {
					got = true
					return stored, nil
				},
			}
			acct, created, err := New(q, nil).RegisterAccount(context.Background(), tt.params)
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

func TestTBCodeAndFlags(t *testing.T) {
	const (
		history        = 1 << 3
		debitsLimited  = 1<<1 | history // debits_must_not_exceed_credits
		creditsLimited = 1<<2 | history // credits_must_not_exceed_debits
	)
	tests := []struct {
		typ           AccountType
		allowNegative bool
		code          uint16
		flags         uint16
	}{
		{typ: TypeAsset, code: 1, flags: creditsLimited},
		{typ: TypeAsset, allowNegative: true, code: 1, flags: history},
		{typ: TypeExpense, code: 2, flags: creditsLimited},
		{typ: TypeExpense, allowNegative: true, code: 2, flags: history},
		{typ: TypeLiability, code: 3, flags: debitsLimited},
		{typ: TypeLiability, allowNegative: true, code: 3, flags: history},
		{typ: TypeEquity, code: 4, flags: debitsLimited},
		{typ: TypeEquity, allowNegative: true, code: 4, flags: history},
		{typ: TypeRevenue, code: 5, flags: debitsLimited},
		{typ: TypeRevenue, allowNegative: true, code: 5, flags: history},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/allowNegative=%v", tt.typ, tt.allowNegative), func(t *testing.T) {
			if got := tbCode(tt.typ); got != tt.code {
				t.Errorf("tbCode = %d, want %d", got, tt.code)
			}
			if got := tbFlags(tt.typ, tt.allowNegative); got != tt.flags {
				t.Errorf("tbFlags = %b, want %b", got, tt.flags)
			}
		})
	}
}
