package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"

	"torment/internal/db"
)

// Asset is a registered currency. Each Name has exactly one Precision, the number of decimal
// places in one minor unit, and maps to one TigerBeetle Ledger number.
type Asset struct {
	Name      string
	Precision int16
	Ledger    int32
}

// assetCodeRE matches an asset code: an uppercase name starting with a letter (group 1),
// optionally followed by "/" and a decimal precision (group 2).
var assetCodeRE = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)(?:/([0-9]+))?$`)

// ParseAssetCode splits a code of the form NAME[/PRECISION] into its name and precision; precision
// is 0 when omitted. It returns ErrInvalid when the code is malformed or the precision exceeds 18.
func ParseAssetCode(code string) (name string, precision int16, err error) {
	m := assetCodeRE.FindStringSubmatch(code)
	if m == nil {
		return "", 0, fmt.Errorf("%w: asset code %q must look like USD/2", ErrInvalid, code)
	}
	if m[2] != "" {
		p, err := strconv.Atoi(m[2])
		if err != nil || p > 18 {
			return "", 0, fmt.Errorf("%w: asset precision in %q must be 0..18", ErrInvalid, code)
		}
		precision = int16(p)
	}
	return m[1], precision, nil
}

// RegisterAsset parses code with ParseAssetCode and stores it as a new asset on the given TigerBeetle ledger.
// It returns ErrInvalid for a malformed code or a non-positive ledger, and ErrConflict when the
// name or the ledger number is already registered.
func (s *Service) RegisterAsset(ctx context.Context, code string, ledger int32) (Asset, error) {
	name, precision, err := ParseAssetCode(code)
	if err != nil {
		return Asset{}, err
	}
	if ledger <= 0 {
		return Asset{}, fmt.Errorf("%w: ledger must be positive", ErrInvalid)
	}
	row, err := s.q.InsertAsset(ctx, db.InsertAssetParams{Name: name, Precision: precision, Ledger: ledger})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return Asset{}, fmt.Errorf("%w: asset %q or ledger %d already exists", ErrConflict, name, ledger)
		}
		return Asset{}, fmt.Errorf("error inserting asset: %w", err)
	}
	return Asset{Name: row.Name, Precision: row.Precision, Ledger: row.Ledger}, nil
}
