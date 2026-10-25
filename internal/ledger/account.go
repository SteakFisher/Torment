package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"torment/internal/db"
)

// Account is a registered account, identified by its address. It holds no balances itself; those live
// in its per-currency wallets.
type Account struct {
	Address       string
	Type          AccountType
	AllowNegative bool
	CreatedAt     time.Time
}

// RegisterAccountParams is the input to RegisterAccount. Type is parsed with ParseAccountType.
type RegisterAccountParams struct {
	Address       string
	Type          string
	AllowNegative bool
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

// RegisterAccount stores a new account. It is idempotent: an identical re-registration returns
// the stored account with created false. It returns ErrInvalid for a malformed address or type, and ErrConflict when the account exists
// with different fields.
func (s *Service) RegisterAccount(ctx context.Context, p RegisterAccountParams) (acct Account, created bool, err error) {
	if err := validateAddress(p.Address); err != nil {
		return Account{}, false, err
	}
	typ, err := ParseAccountType(p.Type)
	if err != nil {
		return Account{}, false, err
	}

	row, err := s.q.InsertAccount(ctx, db.InsertAccountParams{
		Address:       p.Address,
		Type:          db.AccountType(typ),
		AllowNegative: p.AllowNegative,
	})
	switch {
	case err == nil:
		return toAccount(row), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// the account already exists; compare it below
	default:
		return Account{}, false, fmt.Errorf("error inserting account: %w", err)
	}

	row, err = s.q.GetAccount(ctx, p.Address)
	if err != nil {
		return Account{}, false, fmt.Errorf("error getting account: %w", err)
	}
	existing := toAccount(row)
	if existing.Type != typ || existing.AllowNegative != p.AllowNegative {
		return Account{}, false, fmt.Errorf("%w: account %q already exists with different fields", ErrConflict, p.Address)
	}
	return existing, false, nil
}

func toAccount(row db.Account) Account {
	return Account{
		Address:       row.Address,
		Type:          AccountType(row.Type),
		AllowNegative: row.AllowNegative,
		CreatedAt:     row.CreatedAt.Time,
	}
}
