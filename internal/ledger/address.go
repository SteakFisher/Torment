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

// Address is a registered address. It holds no balances itself; those live in its per-asset accounts.
type Address struct {
	Address       string
	Type          AccountType
	AllowNegative bool
	CreatedAt     time.Time
}

// RegisterAddressParams is the input to RegisterAddress. Type is parsed with ParseAccountType.
type RegisterAddressParams struct {
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

// RegisterAddress stores a new address. It is idempotent: an identical re-registration returns
// the stored address with created false. It returns ErrInvalid for a malformed address or type, and ErrConflict when the address exists
// with different fields.
func (s *Service) RegisterAddress(ctx context.Context, p RegisterAddressParams) (addr Address, created bool, err error) {
	if err := validateAddress(p.Address); err != nil {
		return Address{}, false, err
	}
	typ, err := ParseAccountType(p.Type)
	if err != nil {
		return Address{}, false, err
	}

	row, err := s.q.InsertAddress(ctx, db.InsertAddressParams{
		Address:       p.Address,
		Type:          db.AccountType(typ),
		AllowNegative: p.AllowNegative,
	})
	switch {
	case err == nil:
		return toAddress(row), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// the address already exists; compare it below
	default:
		return Address{}, false, fmt.Errorf("error inserting address: %w", err)
	}

	row, err = s.q.GetAddress(ctx, p.Address)
	if err != nil {
		return Address{}, false, fmt.Errorf("error getting address: %w", err)
	}
	existing := toAddress(row)
	if existing.Type != typ || existing.AllowNegative != p.AllowNegative {
		return Address{}, false, fmt.Errorf("%w: address %q already exists with different fields", ErrConflict, p.Address)
	}
	return existing, false, nil
}

func toAddress(row db.Address) Address {
	return Address{
		Address:       row.Address,
		Type:          AccountType(row.Type),
		AllowNegative: row.AllowNegative,
		CreatedAt:     row.CreatedAt.Time,
	}
}
