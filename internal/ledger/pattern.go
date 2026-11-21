package ledger

import (
	"context"
	"regexp"
	"strings"
)

// segmentRE matches one address segment.
var segmentRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsPattern reports whether address is an address pattern rather than an address. Addresses never
// contain "*", so any "*" makes it a pattern.
func IsPattern(address string) bool {
	return strings.Contains(address, "*")
}

// patternRegex returns a Postgres regex matching the addresses that pattern describes. pattern is an
// address in which a "*" segment matches any single segment; an empty pattern matches every address.
// It returns ErrInvalid for any other segment that is not a valid address segment.
func patternRegex(pattern string) (string, error) {
	if pattern == "" {
		return "", nil
	}
	if len(pattern) > maxAddressLen {
		return "", Errorf(ErrInvalid, "address pattern is longer than %d bytes", maxAddressLen)
	}
	segments := strings.Split(pattern, ":")
	for i, s := range segments {
		switch {
		case s == "*":
			segments[i] = "[^:]+"
		case !segmentRE.MatchString(s):
			return "", Errorf(ErrInvalid, "address pattern %q must be colon-separated segments of [a-zA-Z0-9_-] or *", pattern)
		}
	}
	return "^" + strings.Join(segments, ":") + "$", nil
}

// scopeWallets returns the wallets in currency, or in every currency when it is empty, of the account at
// address or of every account matching it when it is a pattern, ordered by address and currency. It
// returns ErrAccountNotFound when no account has the exact address, ErrCurrencyNotFound when currency is
// not registered, ErrPatternTooBroad for a pattern matching more than limit wallets, and ErrInvalid for
// a malformed address or pattern.
func (s *Service) scopeWallets(ctx context.Context, address, currency string, limit int) ([]AddressWallet, error) {
	var wallets []AddressWallet
	if !IsPattern(address) {
		if err := validateAddress(address); err != nil {
			return nil, err
		}
		acct, err := s.store.AccountWallets(ctx, address, currency)
		if err != nil {
			return nil, err
		}
		for _, w := range acct.Wallets {
			wallets = append(wallets, AddressWallet{Address: acct.Address, Type: acct.Type, Wallet: w})
		}
	} else {
		re, err := patternRegex(address)
		if err != nil {
			return nil, err
		}
		var tooMany bool
		wallets, tooMany, err = s.store.PatternWallets(ctx, re, currency, limit)
		if err != nil {
			return nil, err
		}
		if tooMany {
			return nil, Errorf(ErrPatternTooBroad, "address pattern %q matches more than %d wallets; narrow it", address, limit)
		}
	}
	// A registered currency is only looked up when nothing matched, so the usual read costs no query.
	if len(wallets) == 0 && currency != "" {
		if _, err := s.lookupCurrencies(ctx, []string{currency}); err != nil {
			return nil, err
		}
	}
	return wallets, nil
}
