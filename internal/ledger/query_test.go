package ledger

import (
	"context"
	"errors"
	"testing"

	"torment/internal/db/sqlc"
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

func TestListAccounts(t *testing.T) {
	rows := []sqlc.ListAccountsRow{
		{Address: "rider:1:bonus", Type: sqlc.AccountTypeLiability},
		{Address: "rider:1:wallet", Type: sqlc.AccountTypeLiability, Currencies: []string{"AED", "USD"}, Precisions: []int16{2, 3}},
		{Address: "rider:2:wallet", Type: sqlc.AccountTypeLiability},
	}
	tests := []struct {
		name     string
		limit    int
		wantLim  int32
		wantMore bool
		wantN    int
		wantErr  error
	}{
		{name: "default limit", limit: 0, wantLim: defaultListLimit + 1, wantN: 3},
		{name: "more", limit: 2, wantLim: 3, wantMore: true, wantN: 2},
		{name: "exact", limit: 3, wantLim: 4, wantN: 3},
		{name: "negative", limit: -1, wantErr: ErrInvalid},
		{name: "too large", limit: maxListLimit + 1, wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got sqlc.ListAccountsParams
			q := &fakeQuerier{listAccounts: func(_ context.Context, arg sqlc.ListAccountsParams) ([]sqlc.ListAccountsRow, error) {
				got = arg
				return rows[:min(len(rows), int(arg.Lim))], nil
			}}
			accounts, more, err := New(q, nil).ListAccounts(context.Background(), "rider:*:*", "rider:0", tt.limit)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != (sqlc.ListAccountsParams{Pattern: "^rider:[^:]+:[^:]+$", After: "rider:0", Lim: tt.wantLim}) {
				t.Fatalf("query params = %+v", got)
			}
			if len(accounts) != tt.wantN || more != tt.wantMore {
				t.Fatalf("got %d accounts, more %v; want %d, %v", len(accounts), more, tt.wantN, tt.wantMore)
			}
			w := accounts[1].Wallets
			if len(w) != 2 || w[0] != (Wallet{Currency: "AED", Precision: 2}) || w[1] != (Wallet{Currency: "USD", Precision: 3}) {
				t.Fatalf("wallets = %+v", w)
			}
		})
	}
}
