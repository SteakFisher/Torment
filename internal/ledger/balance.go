package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
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

// Balances returns the balance of each wallet of the account at address, ordered by currency. A zero at
// means now; otherwise each balance is as it was after the last transfer at or before at. A wallet with no
// TigerBeetle account, or no transfer by at, has a zero balance. It returns ErrNotFound when no account
// has the address, and ErrInvalid when at is not between 1970 and 2262.
func (s *Service) Balances(ctx context.Context, address string, at time.Time) ([]Balance, error) {
	if !at.IsZero() && (at.UnixNano() <= 0 || at.Year() >= 2262) {
		return nil, fmt.Errorf("%w: at %s must be between 1970 and 2262", ErrInvalid, at.Format(time.RFC3339))
	}
	acct, err := s.q.GetAccount(ctx, address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: account %q", ErrNotFound, address)
	}
	if err != nil {
		return nil, fmt.Errorf("error getting account: %w", err)
	}
	wallets, err := s.q.ListWallets(ctx, acct.ID)
	if err != nil {
		return nil, fmt.Errorf("error listing wallets: %w", err)
	}
	if len(wallets) == 0 {
		return []Balance{}, nil
	}

	ids := make([]tb.Uint128, len(wallets))
	for i, w := range wallets {
		if ids[i], err = idFromBytes(w.TbID); err != nil {
			return nil, err
		}
	}
	var totals map[tb.Uint128]posted
	if at.IsZero() {
		totals, err = s.postedNow(ids)
	} else {
		totals, err = s.postedAt(ids, at)
	}
	if err != nil {
		return nil, err
	}

	typ := AccountType(acct.Type)
	balances := make([]Balance, len(wallets))
	for i, w := range wallets {
		p := totals[ids[i]] // zero when missing
		amount := new(big.Int).Sub(p.credits.BigInt(), p.debits.BigInt())
		if debitNormal(typ) {
			amount.Neg(amount)
		}
		balances[i] = Balance{Currency: w.Currency, Precision: w.Precision, Amount: amount}
	}
	return balances, nil
}

// postedNow returns the current posted totals of the TigerBeetle accounts ids, in one request. Accounts
// missing from TigerBeetle are left out.
func (s *Service) postedNow(ids []tb.Uint128) (map[tb.Uint128]posted, error) {
	found, err := s.tb.LookupAccounts(ids)
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
// before at, with one request per account. Accounts with no such transfer are left out.
func (s *Service) postedAt(ids []tb.Uint128, at time.Time) (map[tb.Uint128]posted, error) {
	totals := make(map[tb.Uint128]posted, len(ids))
	for _, id := range ids {
		found, err := s.tb.GetAccountBalances(tb.AccountFilter{
			AccountID:    id,
			TimestampMax: uint64(at.UnixNano()),
			Limit:        1,
			Flags:        tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32(),
		})
		if err != nil {
			return nil, fmt.Errorf("error getting tigerbeetle account balances: %w", err)
		}
		if len(found) == 1 {
			totals[id] = posted{found[0].DebitsPosted, found[0].CreditsPosted}
		}
	}
	return totals, nil
}
