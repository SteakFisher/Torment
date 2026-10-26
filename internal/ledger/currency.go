package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"

	"torment/internal/db/sqlc"
)

// Currency maps a currency name to its Precision, the number of decimal places in one minor unit,
// and to one TigerBeetle Ledger number. Each Name has exactly one Precision.
type Currency struct {
	Name      string
	Precision int16
	Ledger    int32
}

// currencyCodeRE matches a currency code: an uppercase name starting with a letter (group 1),
// optionally followed by "/" and a decimal precision (group 2).
var currencyCodeRE = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)(?:/([0-9]+))?$`)

// ParseCurrencyCode splits a code of the form NAME[/PRECISION] into its name and precision; precision
// is 0 when omitted. It returns ErrInvalid when the code is malformed or the precision exceeds 18.
func ParseCurrencyCode(code string) (name string, precision int16, err error) {
	m := currencyCodeRE.FindStringSubmatch(code)
	if m == nil {
		return "", 0, fmt.Errorf("%w: currency code %q must look like USD/2", ErrInvalid, code)
	}
	if m[2] != "" {
		p, err := strconv.Atoi(m[2])
		if err != nil || p > 18 {
			return "", 0, fmt.Errorf("%w: currency precision in %q must be 0..18", ErrInvalid, code)
		}
		precision = int16(p)
	}
	return m[1], precision, nil
}

// RegisterCurrency parses code with ParseCurrencyCode and stores it as a new currency on a newly
// assigned TigerBeetle ledger. It returns ErrInvalid for a malformed code and ErrConflict when the
// name is already registered.
func (s *Service) RegisterCurrency(ctx context.Context, code string) (Currency, error) {
	name, precision, err := ParseCurrencyCode(code)
	if err != nil {
		return Currency{}, err
	}
	row, err := s.q.InsertCurrency(ctx, sqlc.InsertCurrencyParams{Name: name, Precision: precision})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return Currency{}, fmt.Errorf("%w: currency %q already exists", ErrConflict, name)
		}
		return Currency{}, fmt.Errorf("error inserting currency: %w", err)
	}
	return Currency{Name: row.Name, Precision: row.Precision, Ledger: row.Ledger}, nil
}
