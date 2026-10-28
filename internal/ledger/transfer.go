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
// of Destination. Amount and GroupID are base-10 strings; GroupID is empty when omitted.
type Posting struct {
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
// destination wallet, and returns the transfers in posting order. A posting without a group id gets its
// own transfer id as its group id. Exactly one posting is accepted. It returns ErrInvalid for malformed
// postings, ErrNotFound when a wallet does not exist, and ErrInsufficientFunds when a posting would take
// an account below zero when it does not allow it.
func (s *Service) Transfer(ctx context.Context, postings []Posting) ([]Transfer, error) {
	if len(postings) != 1 {
		return nil, fmt.Errorf("%w: exactly one posting is required, got %d", ErrInvalid, len(postings))
	}
	p := postings[0]
	if err := validateAddress(p.Source); err != nil {
		return nil, err
	}
	if err := validateAddress(p.Destination); err != nil {
		return nil, err
	}
	if p.Source == p.Destination {
		return nil, fmt.Errorf("%w: source and destination must differ, both are %q", ErrInvalid, p.Source)
	}
	amount, ok := parseUint128(p.Amount)
	if !ok {
		return nil, fmt.Errorf("%w: amount %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.Amount)
	}
	id := tb.ID()
	group := id
	if p.GroupID != "" {
		if group, ok = parseUint128(p.GroupID); !ok {
			return nil, fmt.Errorf("%w: group_id %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.GroupID)
		}
	}

	debit, ledger, err := s.walletByAddress(ctx, p.Source, p.Currency)
	if err != nil {
		return nil, err
	}
	credit, _, err := s.walletByAddress(ctx, p.Destination, p.Currency)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("error creating tigerbeetle transfers: %w", err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("error creating tigerbeetle transfers: got %d results for 1 transfer", len(results))
	}
	switch r := results[0]; r.Status {
	case tb.TransferCreated:
	case tb.TransferExceedsCredits, tb.TransferExceedsDebits:
		return nil, fmt.Errorf("%w: %s %s from %q to %q", ErrInsufficientFunds, p.Amount, p.Currency, p.Source, p.Destination)
	default:
		return nil, fmt.Errorf("error creating tigerbeetle transfer %s: %s", id.BigInt(), r.Status)
	}
	return []Transfer{{ID: id, GroupID: group}}, nil
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
