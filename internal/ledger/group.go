package ledger

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// TransferRecord is a committed transfer. Amount is in minor units of Currency. Timestamp is when
// TigerBeetle committed the transfer.
type TransferRecord struct {
	ID        tb.Uint128
	GroupID   tb.Uint128
	Debit     string
	Credit    string
	Currency  string
	Amount    *big.Int
	Kind      string
	Timestamp time.Time
}

// GroupTransfersParams is the input to GroupTransfers. A zero Cursor starts at the newest transfer, a
// zero Limit means defaultListLimit, and an empty Kind means every kind.
type GroupTransfersParams struct {
	GroupID string
	Kind    string
	Cursor  time.Time
	Limit   int
}

// GroupTransfers returns up to p.Limit transfers whose group id is p.GroupID, of kind p.Kind unless it
// is empty, committed before p.Cursor, newest first. more reports whether older transfers remain; passing the last Timestamp as Cursor gets
// the next page. An unused group id has no transfers. It returns ErrInvalid when the group id is not a
// UUID accepted by parseID, for a limit outside 0 to maxListLimit, or for a cursor outside TigerBeetle's
// range, and ErrKindNotFound when p.Kind is not registered.
func (s *Service) GroupTransfers(ctx context.Context, p GroupTransfersParams) (transfers []TransferRecord, more bool, err error) {
	group, ok := parseID(p.GroupID)
	if !ok {
		return nil, false, fmt.Errorf("%w: group_id %q must be a UUID other than the nil and max UUIDs", ErrInvalid, p.GroupID)
	}
	if p.Limit < 0 || p.Limit > maxListLimit {
		return nil, false, fmt.Errorf("%w: limit %d must be between 1 and %d", ErrInvalid, p.Limit, maxListLimit)
	}
	if p.Limit == 0 {
		p.Limit = defaultListLimit
	}
	if err := validateAt(p.Cursor); err != nil {
		return nil, false, err
	}
	code, err := s.filterCode(ctx, p.Kind)
	if err != nil {
		return nil, false, err
	}

	// One extra transfer tells whether another page exists.
	filter := tb.QueryFilter{UserData128: group, Code: code, Limit: uint32(p.Limit + 1), Flags: tb.QueryFilterFlags{Reversed: true}.ToUint32()}
	if !p.Cursor.IsZero() {
		if p.Cursor.UnixNano() <= 0 {
			return []TransferRecord{}, false, nil
		}
		filter.TimestampMax = uint64(p.Cursor.UnixNano()) - 1
	}
	found, err := tbCall(ctx, func() ([]tb.Transfer, error) { return s.reads.QueryTransfers(filter) })
	if err != nil {
		return nil, false, fmt.Errorf("error querying tigerbeetle transfers: %w", err)
	}
	if len(found) > p.Limit {
		found, more = found[:p.Limit], true
	}
	transfers, err = s.transferRecords(ctx, found)
	return transfers, more, err
}

// transferRecords returns found as TransferRecords in the same order, with TigerBeetle account ids
// replaced by wallet addresses and currencies and codes by kind names. It returns an error when an
// account has no wallet or a code has no kind.
func (s *Service) transferRecords(ctx context.Context, found []tb.Transfer) ([]TransferRecord, error) {
	if len(found) == 0 {
		return []TransferRecord{}, nil
	}
	var ids []tb.Uint128
	var codes []uint16
	for _, t := range found {
		ids = append(ids, t.DebitAccountID, t.CreditAccountID)
		if !slices.Contains(codes, t.Code) {
			codes = append(codes, t.Code)
		}
	}
	wallets, err := s.store.WalletsByTBIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	kinds, err := s.store.KindsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}

	out := make([]TransferRecord, len(found))
	for i, t := range found {
		debit, ok := wallets[t.DebitAccountID]
		credit, ok2 := wallets[t.CreditAccountID]
		if !ok || !ok2 {
			return nil, fmt.Errorf("transfer %s has a tigerbeetle account with no wallet", FormatID(t.ID))
		}
		kind, ok := kinds[t.Code]
		if !ok {
			return nil, fmt.Errorf("transfer %s has tigerbeetle code %d with no kind", FormatID(t.ID), t.Code)
		}
		out[i] = TransferRecord{
			ID:        t.ID,
			GroupID:   t.UserData128,
			Debit:     debit.Address,
			Credit:    credit.Address,
			Currency:  debit.Currency,
			Amount:    t.Amount.BigInt(),
			Kind:      kind.Name,
			Timestamp: time.Unix(0, int64(t.Timestamp)).UTC(),
		}
	}
	return out, nil
}
