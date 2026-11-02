package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"torment/internal/db/sqlc"
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

	row, err := s.q.InsertAccount(ctx, sqlc.InsertAccountParams{
		Address:       p.Address,
		Type:          sqlc.AccountType(typ),
		AllowNegative: p.AllowNegative,
	})
	switch {
	case err == nil:
		created = true
	case errors.Is(err, pgx.ErrNoRows):
		row, err = s.q.GetAccount(ctx, p.Address)
		if err != nil {
			return Account{}, false, fmt.Errorf("error getting account: %w", err)
		}
		if AccountType(row.Type) != typ || row.AllowNegative != p.AllowNegative {
			return Account{}, false, fmt.Errorf("%w: account %q already exists with different fields", ErrConflict, p.Address)
		}
	default:
		return Account{}, false, fmt.Errorf("error inserting account: %w", err)
	}

	keys := make([]walletKey, len(names))
	for i, name := range names {
		keys[i] = walletKey{p.Address, name}
	}
	wallets, err := s.ensureWallets(ctx, keys)
	if err != nil {
		return Account{}, false, err
	}
	acct = toAccount(row)
	for _, k := range keys {
		w := wallets[k]
		acct.Wallets = append(acct.Wallets, Wallet{Currency: k.currency, Precision: w.precision, ID: w.id})
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

func toAccount(row sqlc.Account) Account {
	return Account{
		Address:       row.Address,
		Type:          AccountType(row.Type),
		AllowNegative: row.AllowNegative,
		CreatedAt:     row.CreatedAt.Time,
	}
}
