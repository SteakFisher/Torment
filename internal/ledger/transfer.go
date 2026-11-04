package ledger

import (
	"context"
	"fmt"
	"slices"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// maxPostings is the most transfers TigerBeetle accepts in one request.
const maxPostings = 8189

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

// Transfer is the outcome of one posting. Err nil and Created true means the transfer was created; Err
// nil and Created false means it already existed with the same fields; Err non-nil means it failed.
type Transfer struct {
	ID      tb.Uint128
	GroupID tb.Uint128
	Created bool
	Err     error
}

// Transfer posts each posting as a TigerBeetle transfer that debits the source wallet and credits the
// destination wallet, and returns an outcome for each posting in the shape of chains. Each chain is
// applied atomically and chains independently. It is idempotent per posting id. The returned error
// covers only failures before anything is written: ErrInvalid for malformed input, ErrNotFound when an
// account or currency is not registered, or another error. Each outcome's Err is ErrInsufficientFunds,
// ErrConflict, ErrIDAlreadyFailed, ErrChainFailed or another error.
func (s *Service) Transfer(ctx context.Context, chains [][]Posting) ([][]Transfer, error) {
	batch, err := validateChains(chains)
	if err != nil {
		return nil, err
	}
	if err := s.resolveWallets(ctx, chains, batch); err != nil {
		return nil, err
	}
	results, err := s.tb.CreateTransfers(batch)
	if err != nil {
		return nil, fmt.Errorf("error creating tigerbeetle transfers: %w", err)
	}
	if len(results) != len(batch) {
		return nil, fmt.Errorf("error creating tigerbeetle transfers: got %d results for %d transfers", len(results), len(batch))
	}
	return s.transferResults(chains, batch, results), nil
}

// validateChains returns the TigerBeetle transfers for chains in order, with every field set except the
// accounts and ledger. Every transfer but the last of each chain is linked to the next. It returns
// ErrInvalid naming the position of the first invalid posting.
func validateChains(chains [][]Posting) ([]tb.Transfer, error) {
	if len(chains) == 0 {
		return nil, fmt.Errorf("%w: at least one chain is required", ErrInvalid)
	}
	n := 0
	for i, chain := range chains {
		if len(chain) == 0 {
			return nil, fmt.Errorf("%w: chain %d is empty", ErrInvalid, i)
		}
		n += len(chain)
	}
	if n > maxPostings {
		return nil, fmt.Errorf("%w: %d postings exceed the limit of %d", ErrInvalid, n, maxPostings)
	}
	batch := make([]tb.Transfer, 0, n)
	seen := make(map[tb.Uint128]bool, n)
	for i, chain := range chains {
		for j, p := range chain {
			t, err := parsePosting(p)
			if err != nil {
				return nil, fmt.Errorf("chain %d posting %d: %w", i, j, err)
			}
			if seen[t.ID] {
				return nil, fmt.Errorf("%w: chain %d posting %d: id %s is used more than once", ErrInvalid, i, j, p.ID)
			}
			seen[t.ID] = true
			if j < len(chain)-1 {
				t.Flags = tb.TransferFlags{Linked: true}.ToUint16()
			}
			batch = append(batch, t)
		}
	}
	return batch, nil
}

// parsePosting returns the TigerBeetle transfer for p without accounts, ledger or flags. It returns
// ErrInvalid when a field is malformed.
func parsePosting(p Posting) (tb.Transfer, error) {
	id, ok := parseUint128(p.ID)
	if !ok || id == maxUint128 {
		return tb.Transfer{}, fmt.Errorf("%w: id %q must be a base-10 integer greater than zero and less than 2^128-1", ErrInvalid, p.ID)
	}
	if err := validateAddress(p.Source); err != nil {
		return tb.Transfer{}, err
	}
	if err := validateAddress(p.Destination); err != nil {
		return tb.Transfer{}, err
	}
	if p.Source == p.Destination {
		return tb.Transfer{}, fmt.Errorf("%w: source and destination must differ, both are %q", ErrInvalid, p.Source)
	}
	amount, ok := parseUint128(p.Amount)
	if !ok {
		return tb.Transfer{}, fmt.Errorf("%w: amount %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.Amount)
	}
	group := id
	if p.GroupID != "" {
		if group, ok = parseUint128(p.GroupID); !ok {
			return tb.Transfer{}, fmt.Errorf("%w: group_id %q must be a base-10 integer greater than zero that fits in 128 bits", ErrInvalid, p.GroupID)
		}
	}
	return tb.Transfer{ID: id, Amount: amount, UserData128: group, Code: 1}, nil
}

// resolveWallets sets the accounts and ledger of each transfer in batch from the wallets of the posting
// at the same position in chains, creating any wallet that does not exist. It returns ErrNotFound when an
// account or currency is not registered.
func (s *Service) resolveWallets(ctx context.Context, chains [][]Posting, batch []tb.Transfer) error {
	var keys []WalletKey
	seen := make(map[WalletKey]bool)
	for _, chain := range chains {
		for _, p := range chain {
			for _, k := range []WalletKey{{p.Source, p.Currency}, {p.Destination, p.Currency}} {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	wallets, err := s.ensureWallets(ctx, keys)
	if err != nil {
		return err
	}
	k := 0
	for _, chain := range chains {
		for _, p := range chain {
			debit := wallets[WalletKey{p.Source, p.Currency}]
			batch[k].DebitAccountID = debit.ID
			batch[k].CreditAccountID = wallets[WalletKey{p.Destination, p.Currency}].ID
			batch[k].Ledger = uint32(debit.Ledger)
			k++
		}
	}
	return nil
}

// existingChain is a chain of more than one posting whose first failure is TransferExists.
type existingChain struct {
	out    []Transfer
	sent   []tb.Transfer
	failed int
}

// transferResults returns the outcome of each posting in the shape of chains, where batch and results
// hold the transfer and status for each posting in order. The ids of chains whose first failure is
// TransferExists are looked up in one call to tell an identical chain from a different one.
func (s *Service) transferResults(chains [][]Posting, batch []tb.Transfer, results []tb.CreateTransferResult) [][]Transfer {
	out := make([][]Transfer, len(chains))
	var existing []existingChain
	start := 0
	for i, chain := range chains {
		sent, res := batch[start:start+len(chain)], results[start:start+len(chain)]
		start += len(chain)
		out[i] = make([]Transfer, len(chain))
		for j, t := range sent {
			out[i][j] = Transfer{ID: t.ID, GroupID: t.UserData128}
		}
		// failed is the posting that made the chain fail; every other posting reports TransferLinkedEventFailed
		failed := slices.IndexFunc(res, func(r tb.CreateTransferResult) bool {
			return r.Status != tb.TransferCreated && r.Status != tb.TransferLinkedEventFailed
		})
		if failed < 0 {
			failed = slices.IndexFunc(res, func(r tb.CreateTransferResult) bool { return r.Status != tb.TransferCreated })
		}
		switch {
		case failed < 0:
			for j := range out[i] {
				out[i][j].Created = true
			}
		case res[failed].Status != tb.TransferExists:
			failChain(out[i], failed, statusError(chain[failed], res[failed].Status))
		case len(chain) > 1:
			existing = append(existing, existingChain{out: out[i], sent: sent, failed: failed})
		}
	}
	if len(existing) > 0 {
		s.checkExistingChains(existing)
	}
	return out
}

// checkExistingChains looks up the ids of every chain in one call. A chain whose transfers were all
// found with the fields that were sent is left as existing; any other chain fails with ErrConflict on
// its failed posting.
func (s *Service) checkExistingChains(chains []existingChain) {
	var ids []tb.Uint128
	for _, c := range chains {
		for _, t := range c.sent {
			ids = append(ids, t.ID)
		}
	}
	found, err := s.tb.LookupTransfers(ids)
	if err != nil {
		err = fmt.Errorf("error looking up tigerbeetle transfers: %w", err)
		for _, c := range chains {
			for j := range c.out {
				c.out[j].Err = err
			}
		}
		return
	}
	byID := make(map[tb.Uint128]tb.Transfer, len(found))
	for _, t := range found {
		byID[t.ID] = t
	}
	for _, c := range chains {
		same := true
		for _, t := range c.sent {
			f, ok := byID[t.ID]
			if !ok || f.DebitAccountID != t.DebitAccountID || f.CreditAccountID != t.CreditAccountID || f.Amount != t.Amount ||
				f.Ledger != t.Ledger || f.Code != t.Code || f.UserData128 != t.UserData128 || f.Flags != t.Flags {
				same = false
				break
			}
		}
		if !same {
			failChain(c.out, c.failed, fmt.Errorf("%w: the chain differs from the one posted with these ids", ErrConflict))
		}
	}
}

// failChain sets err on chain[failed] and ErrChainFailed on every other posting of chain.
func failChain(chain []Transfer, failed int, err error) {
	for j := range chain {
		chain[j].Err = fmt.Errorf("%w: transfer %s in the same chain failed", ErrChainFailed, chain[failed].ID.BigInt())
	}
	chain[failed].Err = err
}

// statusError returns the error for p failing with status.
func statusError(p Posting, status tb.CreateTransferStatus) error {
	switch status {
	case tb.TransferExceedsCredits, tb.TransferExceedsDebits:
		return fmt.Errorf("%w: %s %s from %q to %q", ErrInsufficientFunds, p.Amount, p.Currency, p.Source, p.Destination)
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
		return fmt.Errorf("%w: transfer %s exists with different fields: %s", ErrConflict, p.ID, status)
	case tb.TransferIDAlreadyFailed:
		return fmt.Errorf("%w: id %s belongs to a transfer that failed; use a new id", ErrIDAlreadyFailed, p.ID)
	default:
		return fmt.Errorf("error creating tigerbeetle transfer %s: %s", p.ID, status)
	}
}
