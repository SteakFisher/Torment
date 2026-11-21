package ledger

import (
	"context"
	"fmt"
	"slices"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Names turns TigerBeetle transfers into what the API shows: addresses, currencies and kind names in
// place of TigerBeetle ids and codes. It reads only Postgres, so the exporter needs no TigerBeetle client.
type Names struct {
	store Store
}

// NewNames returns Names that keeps wallets and kinds read from store in memory.
func NewNames(store Store) *Names {
	return &Names{store: cachedStore(store)}
}

// transferRecords returns found as TransferRecords in the same order, with TigerBeetle account ids
// replaced by wallet addresses and currencies and codes by kind names. It returns an error when an
// account has no wallet or a code has no kind.
func (n *Names) transferRecords(ctx context.Context, found []tb.Transfer) ([]TransferRecord, error) {
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
	wallets, err := n.store.WalletsByTBIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	kinds, err := n.store.KindsByCodes(ctx, codes)
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
			Precision: debit.Precision,
			Amount:    t.Amount.BigInt(),
			Kind:      kind.Name,
			Timestamp: time.Unix(0, int64(t.Timestamp)).UTC(),
		}
	}
	return out, nil
}
