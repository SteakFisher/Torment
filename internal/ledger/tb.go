package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// tbWriter is the part of the TigerBeetle client that transfers and registration call. A client sends
// one request at a time, so writes get their own client and never queue behind reads.
type tbWriter interface {
	CreateAccounts(accounts []tb.Account) ([]tb.CreateAccountResult, error)
	CreateTransfers(transfers []tb.Transfer) ([]tb.CreateTransferResult, error)
	LookupTransfers(ids []tb.Uint128) ([]tb.Transfer, error)
}

// tbReader is the part of the TigerBeetle client that balance, history and group reads call.
type tbReader interface {
	LookupAccounts(ids []tb.Uint128) ([]tb.Account, error)
	QueryTransfers(filter tb.QueryFilter) ([]tb.Transfer, error)
	GetAccountBalances(filter tb.AccountFilter) ([]tb.AccountBalance, error)
	GetAccountTransfers(filter tb.AccountFilter) ([]tb.Transfer, error)
}

// Fail to compile when tb.Client no longer satisfies tbWriter or tbReader.
var (
	_ tbWriter = tb.Client(nil)
	_ tbReader = tb.Client(nil)
)

// tbCall runs f, a TigerBeetle client call, and returns its result, or ctx's error when ctx ends
// first. The client can't cancel a call, so f keeps running until TigerBeetle answers and its result
// is dropped. A batch larger than the cluster accepts returns ErrBatchTooLarge; nothing in it was
// applied.
func tbCall[T any](ctx context.Context, f func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := f()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		if errors.Is(r.err, tb.ErrTooMuchData) {
			return r.v, fmt.Errorf("%w: the request holds more than the tigerbeetle cluster accepts in one batch; split it", ErrBatchTooLarge)
		}
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// newTBAccount returns the TigerBeetle account for a wallet with the given id on ledger, owned by an
// account of type typ.
func newTBAccount(id tb.Uint128, ledger int32, typ AccountType, allowNegative bool) tb.Account {
	return tb.Account{ID: id, Ledger: uint32(ledger), Code: tbCode(typ), Flags: tbFlags(typ, allowNegative)}
}

// ensureTBAccounts creates accounts in TigerBeetle in one request. An account that already exists
// with the same fields counts as created. It returns an error when the client fails or any account
// is rejected; an account that exists with different fields is also logged as an error.
func (s *Service) ensureTBAccounts(ctx context.Context, accounts []tb.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	results, err := tbCall(ctx, func() ([]tb.CreateAccountResult, error) { return s.writes.CreateAccounts(accounts) })
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
