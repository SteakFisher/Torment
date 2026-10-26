package ledger

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// statuses returns a CreateAccounts func that answers with the given statuses.
func statuses(ss ...tb.CreateAccountStatus) func([]tb.Account) ([]tb.CreateAccountResult, error) {
	return func([]tb.Account) ([]tb.CreateAccountResult, error) {
		results := make([]tb.CreateAccountResult, len(ss))
		for i, s := range ss {
			results[i].Status = s
		}
		return results, nil
	}
}

func TestEnsureTBAccounts(t *testing.T) {
	accounts := []tb.Account{{ID: tb.ID()}, {ID: tb.ID()}}
	tests := []struct {
		name    string
		create  func([]tb.Account) ([]tb.CreateAccountResult, error)
		wantErr bool
	}{
		{name: "created and exists", create: statuses(tb.AccountCreated, tb.AccountExists)},
		{name: "exists with different flags", create: statuses(tb.AccountCreated, tb.AccountExistsWithDifferentFlags), wantErr: true},
		{name: "other failure", create: statuses(tb.AccountLedgerMustNotBeZero, tb.AccountCreated), wantErr: true},
		{name: "missing results", create: statuses(tb.AccountCreated), wantErr: true},
		{name: "client error", create: func([]tb.Account) ([]tb.CreateAccountResult, error) { return nil, tb.ErrClientClosed }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(nil, &fakeTB{createAccounts: tt.create}).ensureTBAccounts(accounts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterAccountWallets(t *testing.T) {
	storedID := tb.ID()
	ledgers := map[string]int32{"USD": 1, "AED": 2}
	tests := []struct {
		name          string
		currencies    []string
		existing      map[string]tb.Uint128 // wallets already stored, by currency
		wantErr       error
		wantCurrency  []string
		wantTBCall    bool
		wantStoredFor string // currency whose wallet must keep storedID
	}{
		{name: "no currencies"},
		{name: "unknown currency", currencies: []string{"USD", "EUR"}, wantErr: ErrInvalid},
		{name: "new wallets", currencies: []string{"AED", "USD"}, wantCurrency: []string{"AED", "USD"}, wantTBCall: true},
		{name: "duplicates", currencies: []string{"USD", "AED", "USD"}, wantCurrency: []string{"USD", "AED"}, wantTBCall: true},
		{name: "existing wallet", currencies: []string{"USD", "AED"}, existing: map[string]tb.Uint128{"USD": storedID}, wantCurrency: []string{"USD", "AED"}, wantTBCall: true, wantStoredFor: "USD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inserted []string
			q := &fakeQuerier{
				getCurrencies: func(_ context.Context, names []string) ([]sqlc.Currency, error) {
					var rows []sqlc.Currency
					for _, n := range names {
						if l, ok := ledgers[n]; ok {
							rows = append(rows, sqlc.Currency{Name: n, Precision: 2, Ledger: l})
						}
					}
					return rows, nil
				},
				insertAccount: func(_ context.Context, arg sqlc.InsertAccountParams) (sqlc.Account, error) {
					inserted = append(inserted, "account")
					return sqlc.Account{ID: 7, Address: arg.Address, Type: arg.Type}, nil
				},
				insertWallet: func(_ context.Context, arg sqlc.InsertWalletParams) (sqlc.Wallet, error) {
					inserted = append(inserted, arg.Currency)
					if _, ok := tt.existing[arg.Currency]; ok {
						return sqlc.Wallet{}, pgx.ErrNoRows
					}
					return sqlc.Wallet{TbID: arg.TbID, AccountID: arg.AccountID, Currency: arg.Currency}, nil
				},
				getWallet: func(_ context.Context, arg sqlc.GetWalletParams) (sqlc.Wallet, error) {
					return sqlc.Wallet{TbID: idToBytes(tt.existing[arg.Currency]), AccountID: arg.AccountID, Currency: arg.Currency}, nil
				},
			}
			var sent []tb.Account
			tbc := &fakeTB{createAccounts: func(accounts []tb.Account) ([]tb.CreateAccountResult, error) {
				sent = accounts
				results := make([]tb.CreateAccountResult, len(accounts))
				for i := range results {
					results[i].Status = tb.AccountCreated
				}
				return results, nil
			}}

			acct, _, err := New(q, tbc).RegisterAccount(context.Background(), RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: tt.currencies})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || len(inserted) != 0 || sent != nil {
					t.Fatalf("err = %v, inserts %v, sent %v; want %v and no writes", err, inserted, sent, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if (sent != nil) != tt.wantTBCall {
				t.Fatalf("CreateAccounts called = %v, want %v", sent != nil, tt.wantTBCall)
			}
			var gotCurrency []string
			for i, w := range acct.Wallets {
				gotCurrency = append(gotCurrency, w.Currency)
				a := sent[i]
				if a.ID != w.ID || w.Precision != 2 || a.Ledger != uint32(ledgers[w.Currency]) || a.Code != tbCode(TypeLiability) || a.Flags != tbFlags(TypeLiability, false) {
					t.Fatalf("sent %+v for wallet %+v", a, w)
				}
				if w.Currency == tt.wantStoredFor && w.ID != storedID {
					t.Fatalf("wallet %s id = %v, want stored id %v", w.Currency, w.ID, storedID)
				}
			}
			if !slices.Equal(gotCurrency, tt.wantCurrency) || len(sent) != len(tt.wantCurrency) {
				t.Fatalf("wallets %v, sent %d accounts; want %v", gotCurrency, len(sent), tt.wantCurrency)
			}
		})
	}
}
