package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

// Posting moves Amount, an integer in minor units of Currency, from the wallet of Source to the wallet
// of Destination. ID is the TigerBeetle transfer id. ID, Amount and GroupID are base-10 strings; an empty
// GroupID makes ID the group id.
type Posting struct {
	ID          string
	Source      string
	Destination string
	Currency    string
	Amount      string
	GroupID     string
}

// Transfer is a posted transfer and the group it belongs to.
type Transfer struct {
	ID      tb.Uint128
	GroupID tb.Uint128
}

// Transfer posts each posting as a TigerBeetle transfer that debits the source wallet and credits the
// destination wallet, and returns the transfers in posting order. Exactly one posting is accepted. It is
// idempotent: a posting whose id already exists with the same fields returns created false. It returns
// ErrInvalid for malformed postings, ErrNotFound when a wallet does not exist, ErrInsufficientFunds when
// a posting would take an account below zero when it does not allow it, and ErrConflict when the id
// exists with different fields or belongs to a transfer that failed.
func (s *Service) Transfer(ctx context.Context, postings []Posting) (transfers []Transfer, created bool, err error) {
	if len(postings) != 1 {
		return nil, false, fmt.Errorf("%w: exactly one posting is required, got %d", ErrInvalid, len(postings))
	}
	p := postings[0]
	id, ok := parseUint128(p.ID)
	if !ok || id == maxUint128 {
		return nil, false, fmt.Errorf("%w: id %q must be a base-10 integer greater than zero and less than 2^128-1", ErrInvalid, p.ID)
	}
	if err := validateAddress(p.Source); err != nil {
		return nil, false, err
	}
	if err := validateAddress(p.Destination); err != nil {
		return nil, false, err
	}
	if p.Source == p.Destination {
		return nil, false, fmt.Errorf("%w: source and destination must differ, both are %q", ErrInvalid, p.Source)
	}
	amount, ok := parseUint128(p.Amount)
	if !ok {
		return nil, false, fmt.Errorf("%w: amount %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.Amount)
	}
	group := id
	if p.GroupID != "" {
		if group, ok = parseUint128(p.GroupID); !ok {
			return nil, false, fmt.Errorf("%w: group_id %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.GroupID)
		}
	}

	debit, ledger, err := s.walletByAddress(ctx, p.Source, p.Currency)
	if err != nil {
		return nil, false, err
	}
	credit, _, err := s.walletByAddress(ctx, p.Destination, p.Currency)
	if err != nil {
		return nil, false, err
	}

	results, err := s.tb.CreateTransfers([]tb.Transfer{{
		ID:              id,
		DebitAccountID:  debit,
		CreditAccountID: credit,
		Amount:          amount,
		UserData128:     group,
		Ledger:          uint32(ledger),
		Code:            1,
	}})
	if err != nil {
		return nil, false, fmt.Errorf("error creating tigerbeetle transfers: %w", err)
	}
	if len(results) != 1 {
		return nil, false, fmt.Errorf("error creating tigerbeetle transfers: got %d results for 1 transfer", len(results))
	}
	switch r := results[0]; r.Status {
	case tb.TransferCreated:
		created = true
	case tb.TransferExists:
	case tb.TransferExceedsCredits, tb.TransferExceedsDebits:
		return nil, false, fmt.Errorf("%w: %s %s from %q to %q", ErrInsufficientFunds, p.Amount, p.Currency, p.Source, p.Destination)
	case tb.TransferExistsWithDifferentFlags,
		tb.TransferExistsWithDifferentPendingID,
		tb.TransferExistsWithDifferentTimeout,
		tb.TransferExistsWithDifferentDebitAccountID,
		tb.TransferExistsWithDifferentCreditAccountID,
		tb.TransferExistsWithDifferentAmount,
		tb.TransferExistsWithDifferentUserData128,
		tb.TransferExistsWithDifferentUserData64,
		tb.TransferExistsWithDifferentUserData32,
		tb.TransferExistsWithDifferentLedger,
		tb.TransferExistsWithDifferentCode:
		return nil, false, fmt.Errorf("%w: transfer %s exists with different fields: %s", ErrConflict, p.ID, r.Status)
	case tb.TransferIDAlreadyFailed:
		return nil, false, fmt.Errorf("%w: id %s already used by a failed transfer; use a new id", ErrConflict, p.ID)
	default:
		return nil, false, fmt.Errorf("error creating tigerbeetle transfer %s: %s", p.ID, r.Status)
	}
	return []Transfer{{ID: id, GroupID: group}}, created, nil
}

// walletByAddress returns the TigerBeetle id and ledger of the wallet in currency of the account at
// address. It returns ErrNotFound when there is no such wallet.
func (s *Service) walletByAddress(ctx context.Context, address, currency string) (tb.Uint128, int32, error) {
	row, err := s.q.GetWalletByAddress(ctx, sqlc.GetWalletByAddressParams{Address: address, Currency: currency})
	if errors.Is(err, pgx.ErrNoRows) {
		return tb.Uint128{}, 0, fmt.Errorf("%w: no %q wallet for account %q", ErrNotFound, currency, address)
	}
	if err != nil {
		return tb.Uint128{}, 0, fmt.Errorf("error getting wallet: %w", err)
	}
	id, err := idFromBytes(row.TbID)
	return id, row.Ledger, err
}
