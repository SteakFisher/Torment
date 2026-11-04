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
	"time"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

const (
	defaultListLimit = 100
	maxListLimit     = 1000
	// maxPatternWallets is the most wallets a balance query may cover.
	maxPatternWallets = 1000
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

// AccountBalances is one account's balances in a pattern balance query.
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

// PatternBalances returns the balances of the accounts whose address matches pattern (see patternRegex),
// in currency unless it is empty, with one total per type and currency. Accounts are ordered by address,
// their balances by currency, and totals by type code then currency. A zero at means now; otherwise
// balances are as of at, as in Balances. Accounts with no wallet in scope are left out. Totals and
// accounts come from the same read, so they agree. It returns ErrInvalid for a malformed pattern or at,
// or when more than maxPatternWallets wallets match.
func (s *Service) PatternBalances(ctx context.Context, pattern, currency string, at time.Time) ([]TypeTotal, []AccountBalances, error) {
	re, err := patternRegex(pattern)
	if err != nil {
		return nil, nil, err
	}
	if err := validateAt(at); err != nil {
		return nil, nil, err
	}
	// One extra row tells whether the pattern exceeds the cap.
	wallets, err := s.q.ListWalletsByPattern(ctx, sqlc.ListWalletsByPatternParams{Pattern: re, Currency: currency, Lim: maxPatternWallets + 1})
	if err != nil {
		return nil, nil, fmt.Errorf("error listing wallets: %w", err)
	}
	if len(wallets) > maxPatternWallets {
		return nil, nil, fmt.Errorf("%w: address pattern %q matches more than %d wallets; narrow it", ErrInvalid, pattern, maxPatternWallets)
	}
	if len(wallets) == 0 {
		return []TypeTotal{}, []AccountBalances{}, nil
	}

	ids := make([]tb.Uint128, len(wallets))
	for i, w := range wallets {
		if ids[i], err = idFromBytes(w.TbID); err != nil {
			return nil, nil, err
		}
	}
	byID, err := s.postedTotals(ids, at)
	if err != nil {
		return nil, nil, err
	}

	type totalKey struct {
		typ      AccountType
		currency string
	}
	sums := map[totalKey]*TypeTotal{}
	var accounts []AccountBalances
	for i, w := range wallets {
		typ := AccountType(w.Type)
		amount := netAmount(typ, byID[ids[i]])
		if len(accounts) == 0 || accounts[len(accounts)-1].Address != w.Address {
			accounts = append(accounts, AccountBalances{Address: w.Address, Type: typ})
		}
		last := &accounts[len(accounts)-1]
		last.Balances = append(last.Balances, Balance{Currency: w.Currency, Precision: w.Precision, Amount: amount})

		k := totalKey{typ, w.Currency}
		if sums[k] == nil {
			sums[k] = &TypeTotal{Type: typ, Currency: w.Currency, Precision: w.Precision, Amount: new(big.Int)}
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

// AccountTransfersParams is the input to AccountTransfers. A zero From or Before leaves that side of the
// time range open, and a zero Limit means defaultListLimit.
type AccountTransfersParams struct {
	Address  string
	Currency string
	From     time.Time
	Before   time.Time
	Limit    int
}

// AccountTransfers returns the transfers of the account at p.Address committed at or after p.From and
// before p.Before, newest first. more reports whether older transfers remain; passing the last Timestamp
// as Before gets the next page. It makes one TigerBeetle request per wallet in scope. It returns
// ErrNotFound when no account has the address, and ErrInvalid for a limit outside 0 to maxListLimit or a
// time outside TigerBeetle's range.
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
	if err := validateAt(p.Before); err != nil {
		return nil, false, err
	}
	acct, err := s.q.GetAccount(ctx, p.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("%w: account %q", ErrNotFound, p.Address)
	}
	if err != nil {
		return nil, false, fmt.Errorf("error getting account: %w", err)
	}
	wallets, err := s.q.ListWallets(ctx, acct.ID)
	if err != nil {
		return nil, false, fmt.Errorf("error listing wallets: %w", err)
	}

	filter := tb.AccountFilter{
		// One extra transfer tells whether another page exists.
		Limit: uint32(p.Limit + 1),
		Flags: tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32(),
	}
	if !p.From.IsZero() {
		filter.TimestampMin = uint64(p.From.UnixNano())
	}
	if !p.Before.IsZero() {
		filter.TimestampMax = uint64(p.Before.UnixNano()) - 1
		if filter.TimestampMin > filter.TimestampMax {
			return []TransferRecord{}, false, nil
		}
	}
	var found []tb.Transfer
	for _, w := range wallets {
		if p.Currency != "" && w.Currency != p.Currency {
			continue
		}
		if filter.AccountID, err = idFromBytes(w.TbID); err != nil {
			return nil, false, err
		}
		got, err := s.tb.GetAccountTransfers(filter)
		if err != nil {
			return nil, false, fmt.Errorf("error getting tigerbeetle account transfers: %w", err)
		}
		found = append(found, got...)
	}
	// The newest p.Limit across all wallets are among each wallet's newest p.Limit.
	slices.SortFunc(found, func(a, b tb.Transfer) int { return cmp.Compare(b.Timestamp, a.Timestamp) })
	if more = len(found) > p.Limit; more {
		found = found[:p.Limit]
	}
	transfers, err = s.transferRecords(ctx, found)
	return transfers, more, err
}
