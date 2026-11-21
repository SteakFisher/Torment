package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestPatternRegex(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		wantErr bool
	}{
		{pattern: "", want: ""},
		{pattern: "world", want: "^world$"},
		{pattern: "rider:*:wallet", want: "^rider:[^:]+:wallet$"},
		{pattern: "rider:123:*", want: "^rider:123:[^:]+$"},
		{pattern: "*", want: "^[^:]+$"},
		{pattern: "rider:**", wantErr: true},
		{pattern: "rider:.*", wantErr: true},
		{pattern: "rider::wallet", wantErr: true},
		{pattern: "rider:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := patternRegex(tt.pattern)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("patternRegex = %q, err %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestScopeWalletsChecks(t *testing.T) {
	store := &fakeStore{
		accountWallets: func(_ context.Context, address, _ string) (Account, error) {
			return Account{Address: address, Type: TypeLiability}, nil
		},
		patternWallets: func(context.Context, string, string, int) ([]AddressWallet, bool, error) { return nil, false, nil },
		currencies: func(context.Context, []string) (map[string]Currency, error) {
			return map[string]Currency{"USD": {Name: "USD"}}, nil
		},
	}
	s := New(store, &fakeTB{}, &fakeTB{})
	tests := []struct {
		address, currency string
		wantErr           error
	}{
		{"rider:1", "USD", nil},
		{"rider:*", "USD", nil},
		{"rider:1", "", nil},
		{"rider:1", "EUR", ErrCurrencyNotFound},
		{"rider:*", "EUR", ErrCurrencyNotFound},
		{"rider.1", "", ErrInvalid},
	}
	for _, tt := range tests {
		if _, err := s.scopeWallets(context.Background(), tt.address, tt.currency, 10); !errors.Is(err, tt.wantErr) {
			t.Errorf("scopeWallets(%q, %q) err = %v, want %v", tt.address, tt.currency, err, tt.wantErr)
		}
	}
	if _, err := s.Account(context.Background(), "rider 1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Account with a space err = %v, want ErrInvalid", err)
	}
}
