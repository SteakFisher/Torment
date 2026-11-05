package ledger

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
// assigned TigerBeetle ledger. It is idempotent: when the name is already registered with the same
// precision, it returns the stored currency with created false. It returns ErrInvalid for a malformed
// code and ErrConflict when the name is registered with a different precision.
func (s *Service) RegisterCurrency(ctx context.Context, code string) (c Currency, created bool, err error) {
	name, precision, err := ParseCurrencyCode(code)
	if err != nil {
		return Currency{}, false, err
	}
	c, created, err = s.store.InsertCurrency(ctx, name, precision)
	if err != nil {
		return Currency{}, false, err
	}
	if !created && c.Precision != precision {
		return Currency{}, false, fmt.Errorf("%w: currency %q already exists with precision %d", ErrConflict, name, c.Precision)
	}
	return c, created, nil
}

// Currencies returns every registered currency, ordered by name.
func (s *Service) Currencies(ctx context.Context) ([]Currency, error) {
	return s.store.ListCurrencies(ctx)
}

// lookupCurrencies returns each named currency, keyed by name. It returns
// ErrInvalid naming the first currency that is not registered.
func (s *Service) lookupCurrencies(ctx context.Context, names []string) (map[string]Currency, error) {
	if len(names) == 0 {
		return nil, nil
	}
	currencies, err := s.store.Currencies(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if _, ok := currencies[name]; !ok {
			return nil, fmt.Errorf("%w: currency %q is not registered", ErrInvalid, name)
		}
	}
	return currencies, nil
}
