package ledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

const (
	defaultListLimit = 100
	maxListLimit     = 1000
	// maxPatternWallets is the most wallets a balance query may cover.
	maxPatternWallets = 1000
	// maxPatternTransferWallets is the most wallets a transfer history query may cover.
	maxPatternTransferWallets = 100
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
// cursor, ordered by address, with their wallets in currency order. Wallet IDs are not set. limit 0 means
// defaultListLimit; more reports whether further accounts match. It returns ErrInvalid for a malformed
// pattern or a limit outside 0 to maxListLimit.
func (s *Service) ListAccounts(ctx context.Context, pattern, cursor string, limit int) (accounts []Account, more bool, err error) {
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
	return s.store.ListAccounts(ctx, re, cursor, limit)
}

// AccountBalances is one account's balances in a balance query.
type AccountBalances struct {
	Address  string
	Type     AccountType
	Balances []Balance
}

// TypeTotal is the sum of the balances of every matched account of one type in one currency.
type TypeTotal struct {
	Type      AccountType
	Currency  string
	Precision int16
	Amount    *big.Int
}

// Balances returns the balances of the account at address, or of every account matching it when it is a
// pattern (see patternRegex), in currency unless it is empty, with one total per type and currency.
// Accounts are ordered by address, their balances by currency, and totals by type code then currency. A
// zero at means now; otherwise each balance is as it was after the last transfer at or before at. A
// wallet with no TigerBeetle account, or no transfer by at, has a zero balance. The exact account is
// listed even with no wallet in scope; pattern accounts with no wallet in scope are left out. Totals and accounts come from the same read, so they agree. It returns
// ErrAccountNotFound when no account has the exact address, ErrPatternTooBroad for a pattern matching
// more than maxPatternWallets wallets, and ErrInvalid for a malformed pattern or an at outside 1970 to
// 2262.
func (s *Service) Balances(ctx context.Context, address, currency string, at time.Time) ([]TypeTotal, []AccountBalances, error) {
	if err := validateAt(at); err != nil {
		return nil, nil, err
	}
	wallets, err := s.scopeWallets(ctx, address, currency, maxPatternWallets)
	if err != nil {
		return nil, nil, err
	}
	if len(wallets) == 0 {
		if IsPattern(address) {
			return []TypeTotal{}, []AccountBalances{}, nil
		}
		acct, err := s.store.AccountWallets(ctx, address, currency)
		if err != nil {
			return nil, nil, err
		}
		return []TypeTotal{}, []AccountBalances{{Address: acct.Address, Type: acct.Type, Balances: []Balance{}}}, nil
	}

	ids := make([]tb.Uint128, len(wallets))
	for i, w := range wallets {
		ids[i] = w.ID
	}
	byID, err := s.postedTotals(ctx, ids, at)
	if err != nil {
		return nil, nil, err
	}

	type totalKey struct {
		typ      AccountType
		currency string
	}
	sums := map[totalKey]*TypeTotal{}
	var accounts []AccountBalances
	for _, w := range wallets {
		amount := netAmount(w.Type, byID[w.ID])
		if len(accounts) == 0 || accounts[len(accounts)-1].Address != w.Address {
			accounts = append(accounts, AccountBalances{Address: w.Address, Type: w.Type})
		}
		last := &accounts[len(accounts)-1]
		last.Balances = append(last.Balances, Balance{Currency: w.Currency, Precision: w.Precision, Amount: amount})

		k := totalKey{w.Type, w.Currency}
		if sums[k] == nil {
			sums[k] = &TypeTotal{Type: w.Type, Currency: w.Currency, Precision: w.Precision, Amount: new(big.Int)}
		}
		sums[k].Amount.Add(sums[k].Amount, amount)
	}
	totals := make([]TypeTotal, 0, len(sums))
	for _, t := range sums {
		totals = append(totals, *t)
	}
	slices.SortFunc(totals, func(a, b TypeTotal) int {
		return cmp.Or(cmp.Compare(tbCode(a.Type), tbCode(b.Type)), cmp.Compare(a.Currency, b.Currency))
	})
	return totals, accounts, nil
}

// AccountTransfersParams is the input to AccountTransfers. A zero From or Cursor leaves that side of the
// time range open, a zero Limit means defaultListLimit, and an empty Kind means every kind.
type AccountTransfersParams struct {
	Address  string
	Currency string
	Kind     string
	From     time.Time
	Cursor   time.Time
	Limit    int
}

// AccountTransfers returns the transfers of the account at p.Address, or of every account matching it when
// it is a pattern, of kind p.Kind unless it is empty, committed at or after p.From and before p.Cursor, newest first. A transfer between two
// matching accounts is returned once. more reports whether older transfers remain; passing the last
// Timestamp as Cursor gets the next page. It makes one TigerBeetle request per wallet in scope. It returns
// ErrAccountNotFound when no account has the exact address, ErrKindNotFound when p.Kind is not
// registered, ErrPatternTooBroad for a pattern matching more than maxPatternTransferWallets wallets, and
// ErrInvalid for a malformed pattern, a limit outside 0
// to maxListLimit or a time outside TigerBeetle's range.
func (s *Service) AccountTransfers(ctx context.Context, p AccountTransfersParams) (transfers []TransferRecord, more bool, err error) {
	if p.Limit < 0 || p.Limit > maxListLimit {
		return nil, false, fmt.Errorf("%w: limit %d must be between 1 and %d", ErrInvalid, p.Limit, maxListLimit)
	}
	if p.Limit == 0 {
		p.Limit = defaultListLimit
	}
	if err := validateAt(p.From); err != nil {
		return nil, false, err
	}
	if err := validateAt(p.Cursor); err != nil {
		return nil, false, err
	}
	code, err := s.filterCode(ctx, p.Kind)
	if err != nil {
		return nil, false, err
	}
	ids, err := s.transferWalletIDs(ctx, p.Address, p.Currency)
	if err != nil {
		return nil, false, err
	}

	filter := tb.AccountFilter{
		Code: code,
		// One extra transfer tells whether another page exists.
		Limit: uint32(p.Limit + 1),
		Flags: tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32(),
	}
	if !p.From.IsZero() {
		filter.TimestampMin = uint64(p.From.UnixNano())
	}
	if !p.Cursor.IsZero() {
		filter.TimestampMax = uint64(p.Cursor.UnixNano()) - 1
		if filter.TimestampMin > filter.TimestampMax {
			return []TransferRecord{}, false, nil
		}
	}
	// The calls run at once so the client sends them together in one request.
	perWallet := make([][]tb.Transfer, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			f := filter
			f.AccountID = id
			perWallet[i], errs[i] = tbCall(ctx, func() ([]tb.Transfer, error) { return s.reads.GetAccountTransfers(f) })
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, false, fmt.Errorf("error getting tigerbeetle account transfers: %w", err)
	}

	var found []tb.Transfer
	seen := map[tb.Uint128]bool{}
	for _, got := range perWallet {
		for _, t := range got {
			if !seen[t.ID] {
				seen[t.ID] = true
				found = append(found, t)
			}
		}
	}
	// The newest p.Limit across all wallets are among each wallet's newest p.Limit.
	slices.SortFunc(found, func(a, b tb.Transfer) int { return cmp.Compare(b.Timestamp, a.Timestamp) })
	if more = len(found) > p.Limit; more {
		found = found[:p.Limit]
	}
	transfers, err = s.transferRecords(ctx, found)
	return transfers, more, err
}

// transferWalletIDs returns the TigerBeetle ids of the wallets in currency, or in every currency when it
// is empty, of the account at address or of every account matching it when it is a pattern. It returns
// the errors of scopeWallets.
func (s *Service) transferWalletIDs(ctx context.Context, address, currency string) ([]tb.Uint128, error) {
	wallets, err := s.scopeWallets(ctx, address, currency, maxPatternTransferWallets)
	if err != nil {
		return nil, err
	}
	ids := make([]tb.Uint128, len(wallets))
	for i, w := range wallets {
		ids[i] = w.ID
	}
	return ids, nil
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
			return nil, fmt.Errorf("%w: address pattern %q matches more than %d wallets; narrow it", ErrPatternTooBroad, address, limit)
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
