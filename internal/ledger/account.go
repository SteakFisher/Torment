package ledger

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// Account is a registered account, identified by its address. It holds no balances itself; those live
// in its per-currency wallets.
type Account struct {
	Address       string
	Type          AccountType
	AllowNegative bool
	CreatedAt     time.Time
	Wallets       []Wallet
}

// RegisterAccountParams is the input to RegisterAccount. Type is parsed with ParseAccountType.
// Currencies names the registered currencies the account gets a wallet in.
type RegisterAccountParams struct {
	Address       string
	Type          string
	AllowNegative bool
	Currencies    []string
}

// addressRE matches an address: one or more segments of letters, digits, "_" or "-", joined by ":".
var addressRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+(:[a-zA-Z0-9_-]+)*$`)

const maxAddressLen = 255

// validateAddress returns ErrInvalid unless address matches addressRE and is at most maxAddressLen bytes.
func validateAddress(address string) error {
	if len(address) > maxAddressLen || !addressRE.MatchString(address) {
		return fmt.Errorf("%w: address %q must be colon-separated segments of [a-zA-Z0-9_-], at most %d bytes", ErrInvalid, address, maxAddressLen)
	}
	return nil
}

// RegisterAccount stores a new account and ensures it has a wallet, backed by a TigerBeetle account,
// in each of p.Currencies. It is idempotent: an identical re-registration returns the stored account
// with created false, adding any missing wallets and TigerBeetle accounts. The returned Wallets follow
// the order of p.Currencies without duplicates. It returns ErrInvalid for a malformed address or type or
// an unregistered currency, and ErrConflict when the account exists with different fields.
func (s *Service) RegisterAccount(ctx context.Context, p RegisterAccountParams) (acct Account, created bool, err error) {
	if err := validateAddress(p.Address); err != nil {
		return Account{}, false, err
	}
	typ, err := ParseAccountType(p.Type)
	if err != nil {
		return Account{}, false, err
	}
	names := dedupe(p.Currencies)
	if _, err := s.lookupCurrencies(ctx, names); err != nil {
		return Account{}, false, err
	}

	acct, created, err = s.store.InsertAccount(ctx, p.Address, typ, p.AllowNegative)
	if err != nil {
		return Account{}, false, err
	}
	if !created && (acct.Type != typ || acct.AllowNegative != p.AllowNegative) {
		return Account{}, false, fmt.Errorf("%w: account %q already exists with different fields", ErrConflict, p.Address)
	}

	keys := make([]WalletKey, len(names))
	for i, name := range names {
		keys[i] = WalletKey{p.Address, name}
	}
	wallets, err := s.ensureWallets(ctx, keys)
	if err != nil {
		return Account{}, false, err
	}
	for _, k := range keys {
		w := wallets[k]
		acct.Wallets = append(acct.Wallets, Wallet{Currency: k.Currency, Precision: w.Precision, ID: w.ID})
	}
	return acct, created, nil
}

// dedupe returns ss without repeated values, keeping the first occurrence of each.
func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
