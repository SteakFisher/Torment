package ledger

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"torment/internal/db/sqlc"
)

const (
	defaultListLimit = 100
	maxListLimit     = 1000
)

// segmentRE matches one address segment.
var segmentRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// patternRegex returns a Postgres regex matching the addresses that pattern describes. pattern is an
// address in which a "*" segment matches any single segment; an empty pattern matches every address.
// It returns ErrInvalid for any other segment that is not a valid address segment.
func patternRegex(pattern string) (string, error) {
	if pattern == "" {
		return "", nil
	}
	if len(pattern) > maxAddressLen {
		return "", fmt.Errorf("%w: address pattern is longer than %d bytes", ErrInvalid, maxAddressLen)
	}
	segments := strings.Split(pattern, ":")
	for i, s := range segments {
		switch {
		case s == "*":
			segments[i] = "[^:]+"
		case !segmentRE.MatchString(s):
			return "", fmt.Errorf("%w: address pattern %q must be colon-separated segments of [a-zA-Z0-9_-] or *", ErrInvalid, pattern)
		}
	}
	return "^" + strings.Join(segments, ":") + "$", nil
}

// ListAccounts returns the accounts whose address matches pattern (see patternRegex) and sorts after
// after, ordered by address, with their wallets in currency order. Wallet IDs are not set. limit 0 means
// defaultListLimit; more reports whether further accounts match. It returns ErrInvalid for a malformed
// pattern or a limit outside 0 to maxListLimit.
func (s *Service) ListAccounts(ctx context.Context, pattern, after string, limit int) (accounts []Account, more bool, err error) {
	re, err := patternRegex(pattern)
	if err != nil {
		return nil, false, err
	}
	if limit < 0 || limit > maxListLimit {
		return nil, false, fmt.Errorf("%w: limit %d must be between 1 and %d", ErrInvalid, limit, maxListLimit)
	}
	if limit == 0 {
		limit = defaultListLimit
	}
	// One extra row tells whether another page exists.
	rows, err := s.q.ListAccounts(ctx, sqlc.ListAccountsParams{Pattern: re, After: after, Lim: int32(limit + 1)})
	if err != nil {
		return nil, false, fmt.Errorf("error listing accounts: %w", err)
	}
	if more = len(rows) > limit; more {
		rows = rows[:limit]
	}
	accounts = make([]Account, len(rows))
	for i, r := range rows {
		accounts[i] = Account{Address: r.Address, Type: AccountType(r.Type), AllowNegative: r.AllowNegative, CreatedAt: r.CreatedAt.Time}
		for j, c := range r.Currencies {
			accounts[i].Wallets = append(accounts[i].Wallets, Wallet{Currency: c, Precision: r.Precisions[j]})
		}
	}
	return accounts, more, nil
}
