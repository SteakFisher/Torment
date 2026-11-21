package ledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Balance is an account's balance in one wallet, in minor units of the currency. Amount grows with
// debits for a debit-normal account and with credits otherwise; it is negative only when the account
// allows it.
type Balance struct {
	Currency  string
	Precision int16
	Amount    *big.Int
}

// posted is a TigerBeetle account's posted totals.
type posted struct{ debits, credits tb.Uint128 }

// netAmount returns the balance of an account of type typ with posted totals p: debits minus credits for
// a debit-normal type, credits minus debits otherwise.
func netAmount(typ AccountType, p posted) *big.Int {
	amount := new(big.Int).Sub(p.credits.BigInt(), p.debits.BigInt())
	if debitNormal(typ) {
		amount.Neg(amount)
	}
	return amount
}

// postedTotals returns the posted totals of the TigerBeetle accounts ids, now when at is zero and as of at
// otherwise. Accounts with nothing to report are left out.
func (s *Service) postedTotals(ctx context.Context, ids []tb.Uint128, at time.Time) (map[tb.Uint128]posted, error) {
	if at.IsZero() {
		return s.postedNow(ctx, ids)
	}
	return s.postedAt(ctx, ids, at)
}

// validateAt returns ErrInvalid unless at is zero or between 1970 and 2262, the range of TigerBeetle
// timestamps.
func validateAt(at time.Time) error {
	if !at.IsZero() && (at.UnixNano() <= 0 || at.Year() >= 2262) {
		return Errorf(ErrInvalid, "at %s must be between 1970 and 2262", at.Format(time.RFC3339))
	}
	return nil
}

// postedNow returns the current posted totals of the TigerBeetle accounts ids, in one request. Accounts
// missing from TigerBeetle are left out.
func (s *Service) postedNow(ctx context.Context, ids []tb.Uint128) (map[tb.Uint128]posted, error) {
	// ponytail: one request, so more than 8189 ids fails.
	found, err := tbCall(ctx, func() ([]tb.Account, error) { return s.reads.LookupAccounts(ids) })
	if err != nil {
		return nil, fmt.Errorf("error looking up tigerbeetle accounts: %w", err)
	}
	totals := make(map[tb.Uint128]posted, len(found))
	for _, a := range found {
		totals[a.ID] = posted{a.DebitsPosted, a.CreditsPosted}
	}
	return totals, nil
}

// postedAt returns the posted totals of the TigerBeetle accounts ids after their last transfer at or
// before at, with one query per account. The queries run at once so the client sends them together in
// one request. Accounts with no such transfer are left out.
func (s *Service) postedAt(ctx context.Context, ids []tb.Uint128, at time.Time) (map[tb.Uint128]posted, error) {
	perAccount := make([][]tb.AccountBalance, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			filter := tb.AccountFilter{
				AccountID:    id,
				TimestampMax: uint64(at.UnixNano()),
				Limit:        1,
				Flags:        tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32(),
			}
			perAccount[i], errs[i] = tbCall(ctx, func() ([]tb.AccountBalance, error) { return s.reads.GetAccountBalances(filter) })
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("error getting tigerbeetle account balances: %w", err)
	}

	totals := make(map[tb.Uint128]posted, len(ids))
	for i, id := range ids {
		if found := perAccount[i]; len(found) == 1 {
			totals[id] = posted{found[0].DebitsPosted, found[0].CreditsPosted}
		}
	}
	return totals, nil
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
