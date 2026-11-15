package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// Event is a committed transfer and the balance of its two accounts right after it. The balances are in
// minor units of the transfer's currency.
type Event struct {
	TransferRecord
	DebitBalance  *big.Int
	CreditBalance *big.Int
}

// cdcEvent is the fields Events reads from one message of TigerBeetle's CDC job.
type cdcEvent struct {
	Type     string `json:"type"`
	Transfer struct {
		ID          cdcUint `json:"id"`
		Amount      cdcUint `json:"amount"`
		UserData128 cdcUint `json:"user_data_128"`
		Code        uint16  `json:"code"`
		Timestamp   cdcUint `json:"timestamp"`
	} `json:"transfer"`
	DebitAccount  cdcAccount `json:"debit_account"`
	CreditAccount cdcAccount `json:"credit_account"`
}

// cdcAccount is an account in a CDC message, with its posted totals right after the transfer.
type cdcAccount struct {
	ID            cdcUint `json:"id"`
	DebitsPosted  cdcUint `json:"debits_posted"`
	CreditsPosted cdcUint `json:"credits_posted"`
}

// cdcUint is an unsigned integer of up to 128 bits in a CDC message. TigerBeetle's CDC job writes it as a JSON
// number when it is small and as a string of digits when it is large.
type cdcUint tb.Uint128

func (u *cdcUint) UnmarshalJSON(b []byte) error {
	s := string(bytes.Trim(b, `"`))
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 || n.BitLen() > 128 {
		return fmt.Errorf("%q is not an unsigned integer of at most 128 bits", s)
	}
	*u = cdcUint(tb.BigIntToUint128(n))
	return nil
}

// Events returns the event for each of raw, the JSON messages of TigerBeetle's CDC job, in the same
// order. It returns an error for a message that is malformed, is not a single-phase transfer, or has an
// account with no wallet or a code with no kind.
func (s *Service) Events(ctx context.Context, raw [][]byte) ([]Event, error) {
	parsed := make([]cdcEvent, len(raw))
	transfers := make([]tb.Transfer, len(raw))
	var ids []tb.Uint128
	for i, msg := range raw {
		e := &parsed[i]
		if err := json.Unmarshal(msg, e); err != nil {
			return nil, fmt.Errorf("error decoding cdc message %d: %w", i, err)
		}
		if e.Type != "single_phase" {
			return nil, fmt.Errorf("cdc message %d has type %q, want single_phase", i, e.Type)
		}
		ts := tb.Uint128(e.Transfer.Timestamp).BigInt()
		if !ts.IsUint64() {
			return nil, fmt.Errorf("cdc message %d has timestamp %s, which does not fit in 64 bits", i, ts.String())
		}
		transfers[i] = tb.Transfer{
			ID:              tb.Uint128(e.Transfer.ID),
			DebitAccountID:  tb.Uint128(e.DebitAccount.ID),
			CreditAccountID: tb.Uint128(e.CreditAccount.ID),
			Amount:          tb.Uint128(e.Transfer.Amount),
			UserData128:     tb.Uint128(e.Transfer.UserData128),
			Code:            e.Transfer.Code,
			Timestamp:       ts.Uint64(),
		}
		ids = append(ids, transfers[i].DebitAccountID, transfers[i].CreditAccountID)
	}

	records, err := s.transferRecords(ctx, transfers)
	if err != nil {
		return nil, err
	}
	wallets, err := s.store.WalletsByTBIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	events := make([]Event, len(raw))
	for i, e := range parsed {
		events[i] = Event{
			TransferRecord: records[i],
			DebitBalance:   e.DebitAccount.balance(wallets),
			CreditBalance:  e.CreditAccount.balance(wallets),
		}
	}
	return events, nil
}

// balance returns a's balance by the normal side of its account's type, read from wallets.
func (a cdcAccount) balance(wallets map[tb.Uint128]AddressWallet) *big.Int {
	return netAmount(wallets[tb.Uint128(a.ID)].Type, posted{tb.Uint128(a.DebitsPosted), tb.Uint128(a.CreditsPosted)})
}
