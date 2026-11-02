package ledger

import (
	"context"
	"fmt"
	"math/big"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// maxGroupTransfers is the most transfers TigerBeetle returns from one query.
const maxGroupTransfers = 8189

// GroupTransfer is a transfer in a group. Amount is in minor units of Currency. Timestamp is when
// TigerBeetle committed the transfer.
type GroupTransfer struct {
	ID          tb.Uint128
	Source      string
	Destination string
	Currency    string
	Amount      *big.Int
	Timestamp   time.Time
}

// GroupTransfers returns the transfers whose group id is groupID, oldest first. An unused group id has no
// transfers. It returns ErrInvalid when groupID is not a base-10 integer greater than zero that fits in
// 128 bits.
func (s *Service) GroupTransfers(ctx context.Context, groupID string) ([]GroupTransfer, error) {
	group, ok := parseUint128(groupID)
	if !ok {
		return nil, fmt.Errorf("%w: group_id %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, groupID)
	}
	// ponytail: returns only the first maxGroupTransfers transfers of a group.
	found, err := s.tb.QueryTransfers(tb.QueryFilter{UserData128: group, Limit: maxGroupTransfers})
	if err != nil {
		return nil, fmt.Errorf("error querying tigerbeetle transfers: %w", err)
	}
	if len(found) == 0 {
		return []GroupTransfer{}, nil
	}

	var ids [][]byte
	for _, t := range found {
		ids = append(ids, idToBytes(t.DebitAccountID), idToBytes(t.CreditAccountID))
	}
	rows, err := s.q.GetWalletsByTBIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("error getting wallets: %w", err)
	}
	type wallet struct{ address, currency string }
	wallets := make(map[tb.Uint128]wallet, len(rows))
	for _, r := range rows {
		id, err := idFromBytes(r.TbID)
		if err != nil {
			return nil, err
		}
		wallets[id] = wallet{r.Address, r.Currency}
	}

	out := make([]GroupTransfer, len(found))
	for i, t := range found {
		debit, ok := wallets[t.DebitAccountID]
		credit, ok2 := wallets[t.CreditAccountID]
		if !ok || !ok2 {
			return nil, fmt.Errorf("transfer %s has a tigerbeetle account with no wallet", t.ID.BigInt())
		}
		out[i] = GroupTransfer{
			ID:          t.ID,
			Source:      debit.address,
			Destination: credit.address,
			Currency:    debit.currency,
			Amount:      t.Amount.BigInt(),
			Timestamp:   time.Unix(0, int64(t.Timestamp)).UTC(),
		}
	}
	return out, nil
}
