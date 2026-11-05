package ledger

import (
	"context"
	"errors"
	"slices"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
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
			err := New(nil, &fakeTB{createAccounts: tt.create}, nil).ensureTBAccounts(accounts)
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
		name         string
		currencies   []string
		existing     *fakeWallet // stored USD wallet with storedID; nil for none
		wantErr      error
		wantCurrency []string
		wantSent     []string // currencies sent to CreateAccounts
	}{
		{name: "no currencies"},
		{name: "unknown currency", currencies: []string{"USD", "EUR"}, wantErr: ErrInvalid},
		{name: "new wallets", currencies: []string{"AED", "USD"}, wantCurrency: []string{"AED", "USD"}, wantSent: []string{"AED", "USD"}},
		{name: "duplicates", currencies: []string{"USD", "AED", "USD"}, wantCurrency: []string{"USD", "AED"}, wantSent: []string{"USD", "AED"}},
		{name: "existing wallet", currencies: []string{"USD", "AED"}, existing: &fakeWallet{created: true}, wantCurrency: []string{"USD", "AED"}, wantSent: []string{"AED"}},
		{name: "existing wallet not created", currencies: []string{"USD", "AED"}, existing: &fakeWallet{}, wantCurrency: []string{"USD", "AED"}, wantSent: []string{"USD", "AED"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newWalletStore()
			store.ledgers = ledgers
			if tt.existing != nil {
				store.accounts["rider:1:wallet"] = fakeAccount{id: 7, typ: TypeLiability}
				tt.existing.id = storedID
				store.wallets[WalletKey{"rider:1:wallet", "USD"}] = tt.existing
			}
			insertedAccount := false
			fs := store.store()
			fs.currencies = func(_ context.Context, names []string) (map[string]Currency, error) {
				found := map[string]Currency{}
				for _, n := range names {
					if l, ok := ledgers[n]; ok {
						found[n] = Currency{Name: n, Precision: 2, Ledger: l}
					}
				}
				return found, nil
			}
			fs.insertAccount = func(_ context.Context, address string, typ AccountType, _ bool) (Account, bool, error) {
				insertedAccount = true
				store.accounts[address] = fakeAccount{id: 7, typ: typ}
				return Account{Address: address, Type: typ}, true, nil
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

			acct, _, err := New(fs, tbc, tbc).RegisterAccount(context.Background(), RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: tt.currencies})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || insertedAccount || store.inserted != nil || sent != nil {
					t.Fatalf("err = %v, account inserted %v, wallets inserted %v, sent %v; want %v and no writes", err, insertedAccount, store.inserted, sent, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			var gotCurrency []string
			ids := map[string]tb.Uint128{}
			for _, w := range acct.Wallets {
				gotCurrency = append(gotCurrency, w.Currency)
				ids[w.Currency] = w.ID
				if w.Precision != 2 {
					t.Fatalf("wallet %+v, want precision 2", w)
				}
			}
			if !slices.Equal(gotCurrency, tt.wantCurrency) {
				t.Fatalf("wallets %v, want %v", gotCurrency, tt.wantCurrency)
			}
			if tt.existing != nil && ids["USD"] != storedID {
				t.Fatalf("USD wallet id = %v, want stored id %v", ids["USD"], storedID)
			}
			if len(sent) != len(tt.wantSent) || len(store.marked) != len(tt.wantSent) {
				t.Fatalf("sent %d accounts, marked %d; want %v", len(sent), len(store.marked), tt.wantSent)
			}
			for i, c := range tt.wantSent {
				a := sent[i]
				if a.ID != ids[c] || a.Ledger != uint32(ledgers[c]) || a.Code != tbCode(TypeLiability) || a.Flags != tbFlags(TypeLiability, false) || store.marked[i] != ids[c] {
					t.Fatalf("sent %+v, marked %v for %s wallet %v", a, store.marked[i], c, ids[c])
				}
			}
		})
	}
}
