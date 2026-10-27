package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"

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

// Balances returns the balance of each wallet of the account at address, ordered by currency. A wallet
// with no TigerBeetle account has a zero balance. It returns ErrNotFound when no account has the address.
func (s *Service) Balances(ctx context.Context, address string) ([]Balance, error) {
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
	found, err := s.tb.LookupAccounts(ids)
	if err != nil {
		return nil, fmt.Errorf("error looking up tigerbeetle accounts: %w", err)
	}
	byID := make(map[tb.Uint128]tb.Account, len(found))
	for _, a := range found {
		byID[a.ID] = a
	}

	typ := AccountType(acct.Type)
	balances := make([]Balance, len(wallets))
	for i, w := range wallets {
		a := byID[ids[i]] // the zero Account when missing from TigerBeetle
		amount := new(big.Int).Sub(a.CreditsPosted.BigInt(), a.DebitsPosted.BigInt())
		if debitNormal(typ) {
			amount.Neg(amount)
		}
		balances[i] = Balance{Currency: w.Currency, Precision: w.Precision, Amount: amount}
	}
	return balances, nil
}
