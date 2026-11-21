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

// TransferRecord is a committed transfer. Amount is in minor units of Currency. Timestamp is when
// TigerBeetle committed the transfer.
type TransferRecord struct {
	ID        tb.Uint128
	GroupID   tb.Uint128
	Debit     string
	Credit    string
	Currency  string
	Precision int16
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
		return nil, false, Errorf(ErrInvalid, "group_id %q must be a UUID other than the nil and max UUIDs", p.GroupID)
	}
	if p.Limit < 0 || p.Limit > maxListLimit {
		return nil, false, Errorf(ErrInvalid, "limit %d must be between 1 and %d", p.Limit, maxListLimit)
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
	transfers, err = s.names.transferRecords(ctx, found)
	return transfers, more, err
}

// AccountTransfersParams is the input to AccountTransfers. A zero From or Cursor leaves that side of the
// time range open, a zero Limit means defaultListLimit, and an empty Kind means every kind.
type AccountTransfersParams struct {
	Address  string
	Currency string
	Kind     string
	From     time.Time
	Cursor   time.Time
	Limit    int
}

// AccountTransfers returns the transfers of the account at p.Address, or of every account matching it when
// it is a pattern, of kind p.Kind unless it is empty, committed at or after p.From and before p.Cursor, newest first. A transfer between two
// matching accounts is returned once. more reports whether older transfers remain; passing the last
// Timestamp as Cursor gets the next page. It makes one TigerBeetle request per wallet in scope. It returns
// ErrAccountNotFound when no account has the exact address, ErrKindNotFound when p.Kind is not
// registered, ErrPatternTooBroad for a pattern matching more than maxPatternTransferWallets wallets, and
// ErrInvalid for a malformed pattern, a limit outside 0
// to maxListLimit or a time outside TigerBeetle's range.
func (s *Service) AccountTransfers(ctx context.Context, p AccountTransfersParams) (transfers []TransferRecord, more bool, err error) {
	if p.Limit < 0 || p.Limit > maxListLimit {
		return nil, false, Errorf(ErrInvalid, "limit %d must be between 1 and %d", p.Limit, maxListLimit)
	}
	if p.Limit == 0 {
		p.Limit = defaultListLimit
	}
	if err := validateAt(p.From); err != nil {
		return nil, false, err
	}
	if err := validateAt(p.Cursor); err != nil {
		return nil, false, err
	}
	code, err := s.filterCode(ctx, p.Kind)
	if err != nil {
		return nil, false, err
	}
	ids, err := s.transferWalletIDs(ctx, p.Address, p.Currency)
	if err != nil {
		return nil, false, err
	}

	filter := tb.AccountFilter{
		Code: code,
		// One extra transfer tells whether another page exists.
		Limit: uint32(p.Limit + 1),
		Flags: tb.AccountFilterFlags{Debits: true, Credits: true, Reversed: true}.ToUint32(),
	}
	if !p.From.IsZero() {
		filter.TimestampMin = uint64(p.From.UnixNano())
	}
	if !p.Cursor.IsZero() {
		filter.TimestampMax = uint64(p.Cursor.UnixNano()) - 1
		if filter.TimestampMin > filter.TimestampMax {
			return []TransferRecord{}, false, nil
		}
	}
	// The calls run at once so the client sends them together in one request.
	perWallet := make([][]tb.Transfer, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			f := filter
			f.AccountID = id
			perWallet[i], errs[i] = tbCall(ctx, func() ([]tb.Transfer, error) { return s.reads.GetAccountTransfers(f) })
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, false, fmt.Errorf("error getting tigerbeetle account transfers: %w", err)
	}

	var found []tb.Transfer
	seen := map[tb.Uint128]bool{}
	for _, got := range perWallet {
		for _, t := range got {
			if !seen[t.ID] {
				seen[t.ID] = true
				found = append(found, t)
			}
		}
	}
	// The newest p.Limit across all wallets are among each wallet's newest p.Limit.
	slices.SortFunc(found, func(a, b tb.Transfer) int { return cmp.Compare(b.Timestamp, a.Timestamp) })
	if more = len(found) > p.Limit; more {
		found = found[:p.Limit]
	}
	transfers, err = s.names.transferRecords(ctx, found)
	return transfers, more, err
}

// transferWalletIDs returns the TigerBeetle ids of the wallets in currency, or in every currency when it
// is empty, of the account at address or of every account matching it when it is a pattern. It returns
// the errors of scopeWallets.
func (s *Service) transferWalletIDs(ctx context.Context, address, currency string) ([]tb.Uint128, error) {
	wallets, err := s.scopeWallets(ctx, address, currency, maxPatternTransferWallets)
	if err != nil {
		return nil, err
	}
	ids := make([]tb.Uint128, len(wallets))
	for i, w := range wallets {
		ids[i] = w.ID
	}
	return ids, nil
}
