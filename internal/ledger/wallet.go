package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// Wallet is an account's balance in one currency. Precision is the currency's number of decimal
// places; ID is the TigerBeetle account that holds the balance.
type Wallet struct {
	Currency  string
	Precision int16
	ID        tb.Uint128
}

// walletID returns the TigerBeetle id stored for the wallet of accountID in currency, first inserting
// the wallet with a new id when it does not exist.
func (s *Service) walletID(ctx context.Context, accountID int64, currency string) (tb.Uint128, error) {
	row, err := s.q.InsertWallet(ctx, sqlc.InsertWalletParams{TbID: idToBytes(tb.ID()), AccountID: accountID, Currency: currency})
	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		row, err = s.q.GetWallet(ctx, sqlc.GetWalletParams{AccountID: accountID, Currency: currency})
		if err != nil {
			return tb.Uint128{}, fmt.Errorf("error getting wallet: %w", err)
		}
	default:
		return tb.Uint128{}, fmt.Errorf("error inserting wallet: %w", err)
	}
	return idFromBytes(row.TbID)
}

// newTBAccount returns the TigerBeetle account for a wallet with the given id on ledger, owned by an
// account of type typ.
func newTBAccount(id tb.Uint128, ledger int32, typ AccountType, allowNegative bool) tb.Account {
	return tb.Account{ID: id, Ledger: uint32(ledger), Code: tbCode(typ), Flags: tbFlags(typ, allowNegative)}
}

// ensureTBAccounts creates accounts in TigerBeetle in one request. An account that already exists
// with the same fields counts as created. It returns an error when the client fails or any account
// is rejected; an account that exists with different fields is also logged as an error.
func (s *Service) ensureTBAccounts(accounts []tb.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	results, err := s.tb.CreateAccounts(accounts)
	if err != nil {
		return fmt.Errorf("error creating tigerbeetle accounts: %w", err)
	}
	if len(results) != len(accounts) {
		return fmt.Errorf("error creating tigerbeetle accounts: got %d results for %d accounts", len(results), len(accounts))
	}
	// results[i] is the outcome for accounts[i]
	for i, r := range results {
		id := accounts[i].ID.BigInt()
		switch r.Status {
		case tb.AccountCreated, tb.AccountExists:
		case tb.AccountExistsWithDifferentFlags,
			tb.AccountExistsWithDifferentUserData128,
			tb.AccountExistsWithDifferentUserData64,
			tb.AccountExistsWithDifferentUserData32,
			tb.AccountExistsWithDifferentLedger,
			tb.AccountExistsWithDifferentCode:
			slog.Error("tigerbeetle account exists with different fields", "id", id, "status", r.Status)
			return fmt.Errorf("error creating tigerbeetle account %s: %s", id, r.Status)
		default:
			return fmt.Errorf("error creating tigerbeetle account %s: %s", id, r.Status)
		}
	}
	return nil
}
