package ledger

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db/sqlc"
)

var testWalletIDs = map[string]tb.Uint128{"world": tb.ID(), "rider:1:wallet": tb.ID(), "rider:2:wallet": tb.ID()}

// testStore returns a walletStore with an account for each address in testWalletIDs and a created ledger 7
// USD wallet with that id for each, except no account at missing.
func testStore(missing string) *walletStore {
	w := newWalletStore()
	w.ledgers["USD"] = 7
	i := int64(1)
	for address, id := range testWalletIDs {
		if address != missing {
			w.accounts[address] = sqlc.Account{ID: i, Address: address, Type: sqlc.AccountTypeLiability}
			w.wallets[walletKey{address, "USD"}] = &fakeWallet{tbID: idToBytes(id), created: true}
			i++
		}
	}
	return w
}

// testPosting returns a world → rider:1:wallet posting of 100 USD with the given id.
func testPosting(id int) Posting {
	return Posting{ID: strconv.Itoa(id), Source: "world", Destination: "rider:1:wallet", Currency: "USD", Amount: "100"}
}

// testChains returns chains of the given lengths whose postings have ids 1, 2, 3 and so on in order.
func testChains(lengths ...int) [][]Posting {
	chains := make([][]Posting, len(lengths))
	id := 1
	for i, n := range lengths {
		for range n {
			chains[i] = append(chains[i], testPosting(id))
			id++
		}
	}
	return chains
}

func TestTransferRejects(t *testing.T) {
	// edited returns one chain of one posting, changed by edit
	edited := func(edit func(p *Posting)) [][]Posting {
		p := testPosting(5)
		edit(&p)
		return [][]Posting{{p}}
	}
	tooMany := make([][]Posting, maxPostings+1)
	for i := range tooMany {
		tooMany[i] = []Posting{testPosting(i + 1)}
	}

	tests := []struct {
		name    string
		chains  [][]Posting
		missing string
		wantErr error
	}{
		{name: "no chains", wantErr: ErrInvalid},
		{name: "empty chain", chains: [][]Posting{{testPosting(1)}, {}}, wantErr: ErrInvalid},
		{name: "over the batch limit", chains: tooMany, wantErr: ErrInvalid},
		{name: "duplicate id across chains", chains: [][]Posting{{testPosting(1), testPosting(2)}, {testPosting(2)}}, wantErr: ErrInvalid},
		{name: "missing id", chains: edited(func(p *Posting) { p.ID = "" }), wantErr: ErrInvalid},
		{name: "zero id", chains: edited(func(p *Posting) { p.ID = "0" }), wantErr: ErrInvalid},
		{name: "max id", chains: edited(func(p *Posting) { p.ID = "340282366920938463463374607431768211455" }), wantErr: ErrInvalid},
		{name: "bad source", chains: edited(func(p *Posting) { p.Source = "a b" }), wantErr: ErrInvalid},
		{name: "bad destination", chains: edited(func(p *Posting) { p.Destination = "" }), wantErr: ErrInvalid},
		{name: "same source and destination", chains: edited(func(p *Posting) { p.Destination = "world" }), wantErr: ErrInvalid},
		{name: "zero amount", chains: edited(func(p *Posting) { p.Amount = "0" }), wantErr: ErrInvalid},
		{name: "negative amount", chains: edited(func(p *Posting) { p.Amount = "-1" }), wantErr: ErrInvalid},
		{name: "non-numeric amount", chains: edited(func(p *Posting) { p.Amount = "ten" }), wantErr: ErrInvalid},
		{name: "amount over 128 bits", chains: edited(func(p *Posting) { p.Amount = "340282366920938463463374607431768211456" }), wantErr: ErrInvalid},
		{name: "bad group id", chains: edited(func(p *Posting) { p.GroupID = "0" }), wantErr: ErrInvalid},
		{name: "unknown source account", chains: testChains(2, 1), missing: "world", wantErr: ErrNotFound},
		{name: "unknown destination account", chains: testChains(2, 1), missing: "rider:1:wallet", wantErr: ErrNotFound},
		{name: "unknown currency", chains: edited(func(p *Posting) { p.Currency = "AED" }), wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tbc := &fakeTB{createTransfers: func([]tb.Transfer) ([]tb.CreateTransferResult, error) {
				t.Fatal("CreateTransfers called")
				return nil, nil
			}}
			got, err := New(testStore(tt.missing).querier(), tbc).Transfer(context.Background(), tt.chains)
			if !errors.Is(err, tt.wantErr) || got != nil {
				t.Fatalf("got %+v, err %v; want %v", got, err, tt.wantErr)
			}
		})
	}
}

func TestTransferBatch(t *testing.T) {
	chains := [][]Posting{
		{testPosting(1), testPosting(2), {ID: "3", Source: "rider:1:wallet", Destination: "rider:2:wallet", Currency: "USD", Amount: "30", GroupID: "18446744073709551616"}},
		{testPosting(4)},
		{testPosting(5), testPosting(6)},
	}
	var sent []tb.Transfer
	// createAccounts is nil, so a CreateAccounts call panics
	tbc := &fakeTB{createTransfers: func(transfers []tb.Transfer) ([]tb.CreateTransferResult, error) {
		sent = transfers
		return make([]tb.CreateTransferResult, len(transfers)), nil
	}}
	store := testStore("")
	if _, err := New(store.querier(), tbc).Transfer(context.Background(), chains); err != nil {
		t.Fatalf("error transferring: %v", err)
	}
	if store.gets != 1 || store.inserted != nil || store.marked != nil {
		t.Fatalf("wallet lookups %d, inserted %v, marked %d; want 1 lookup and no writes", store.gets, store.inserted, len(store.marked))
	}

	linked := tb.TransferFlags{Linked: true}.ToUint16()
	// 2^64 does not fit in a uint64
	bigGroup := tb.BigIntToUint128(new(big.Int).Lsh(big.NewInt(1), 64))
	transfer := func(id uint64, flags uint16) tb.Transfer {
		return tb.Transfer{ID: tb.ToUint128(id), DebitAccountID: testWalletIDs["world"], CreditAccountID: testWalletIDs["rider:1:wallet"],
			Amount: tb.ToUint128(100), UserData128: tb.ToUint128(id), Ledger: 7, Code: 1, Flags: flags}
	}
	want := []tb.Transfer{
		transfer(1, linked),
		transfer(2, linked),
		{ID: tb.ToUint128(3), DebitAccountID: testWalletIDs["rider:1:wallet"], CreditAccountID: testWalletIDs["rider:2:wallet"],
			Amount: tb.ToUint128(30), UserData128: bigGroup, Ledger: 7, Code: 1},
		transfer(4, 0),
		transfer(5, linked),
		transfer(6, 0),
	}
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v\nwant %+v", sent, want)
	}
}

func TestTransferResults(t *testing.T) {
	const (
		created = "created"
		exists  = "exists"
		other   = "other"
	)
	// lookup modes
	const (
		lookupAll      = "all"
		lookupMissing  = "missing"
		lookupMismatch = "mismatch"
	)
	tests := []struct {
		name     string
		lengths  []int
		statuses []tb.CreateTransferStatus
		lookup   string
		// want is created, exists, other or the sentinel name of each posting's outcome
		want [][]any
	}{
		{
			name:     "all created",
			lengths:  []int{2, 1},
			statuses: []tb.CreateTransferStatus{tb.TransferCreated, tb.TransferCreated, tb.TransferCreated},
			want:     [][]any{{created, created}, {created}},
		},
		{
			name:     "one chain created one insufficient",
			lengths:  []int{1, 1},
			statuses: []tb.CreateTransferStatus{tb.TransferCreated, tb.TransferExceedsCredits},
			want:     [][]any{{created}, {ErrInsufficientFunds}},
		},
		{
			name:     "exceeds in the middle of a chain",
			lengths:  []int{3},
			statuses: []tb.CreateTransferStatus{tb.TransferLinkedEventFailed, tb.TransferExceedsDebits, tb.TransferLinkedEventFailed},
			want:     [][]any{{ErrChainFailed, ErrInsufficientFunds, ErrChainFailed}},
		},
		{
			name:     "exists with different amount",
			lengths:  []int{1},
			statuses: []tb.CreateTransferStatus{tb.TransferExistsWithDifferentAmount},
			want:     [][]any{{ErrConflict}},
		},
		{
			name:     "id already failed",
			lengths:  []int{2},
			statuses: []tb.CreateTransferStatus{tb.TransferLinkedEventFailed, tb.TransferIDAlreadyFailed},
			want:     [][]any{{ErrChainFailed, ErrIDAlreadyFailed}},
		},
		{
			name:     "unknown status",
			lengths:  []int{1, 1},
			statuses: []tb.CreateTransferStatus{tb.TransferOverflowsDebits, tb.TransferCreated},
			want:     [][]any{{other}, {created}},
		},
		{
			name:     "exists for a chain of one",
			lengths:  []int{1},
			statuses: []tb.CreateTransferStatus{tb.TransferExists},
			want:     [][]any{{exists}},
		},
		{
			name:     "exists for a longer chain that matches",
			lengths:  []int{2, 1, 2},
			statuses: []tb.CreateTransferStatus{tb.TransferExists, tb.TransferLinkedEventFailed, tb.TransferCreated, tb.TransferExists, tb.TransferLinkedEventFailed},
			lookup:   lookupAll,
			want:     [][]any{{exists, exists}, {created}, {exists, exists}},
		},
		{
			name:     "exists for a longer chain with a missing transfer",
			lengths:  []int{2},
			statuses: []tb.CreateTransferStatus{tb.TransferExists, tb.TransferLinkedEventFailed},
			lookup:   lookupMissing,
			want:     [][]any{{ErrConflict, ErrChainFailed}},
		},
		{
			name:     "exists for a longer chain with a different transfer",
			lengths:  []int{2},
			statuses: []tb.CreateTransferStatus{tb.TransferExists, tb.TransferLinkedEventFailed},
			lookup:   lookupMismatch,
			want:     [][]any{{ErrConflict, ErrChainFailed}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []tb.Transfer
			var lookups int
			tbc := &fakeTB{
				createTransfers: func(transfers []tb.Transfer) ([]tb.CreateTransferResult, error) {
					sent = transfers
					results := make([]tb.CreateTransferResult, len(tt.statuses))
					for i, s := range tt.statuses {
						results[i] = tb.CreateTransferResult{Timestamp: uint64(i), Status: s}
					}
					return results, nil
				},
				lookupTransfers: func(ids []tb.Uint128) ([]tb.Transfer, error) {
					lookups++
					var found []tb.Transfer
					for _, tr := range sent {
						if !slices.Contains(ids, tr.ID) {
							continue
						}
						switch {
						case tt.lookup == lookupMissing && tr.ID == ids[len(ids)-1]:
							continue
						case tt.lookup == lookupMismatch && tr.ID == ids[len(ids)-1]:
							tr.Amount = tb.ToUint128(99)
						}
						found = append(found, tr)
					}
					return found, nil
				},
			}
			got, err := New(testStore("").querier(), tbc).Transfer(context.Background(), testChains(tt.lengths...))
			if err != nil {
				t.Fatalf("error transferring: %v", err)
			}
			wantLookups := 0
			if tt.lookup != "" {
				wantLookups = 1
			}
			if lookups != wantLookups {
				t.Fatalf("LookupTransfers called %d times, want %d", lookups, wantLookups)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d chains, want %d", len(got), len(tt.want))
			}
			id := uint64(1)
			for i, chain := range tt.want {
				if len(got[i]) != len(chain) {
					t.Fatalf("chain %d has %d outcomes, want %d", i, len(got[i]), len(chain))
				}
				for j, want := range chain {
					tr := got[i][j]
					if tr.ID != tb.ToUint128(id) || tr.GroupID != tb.ToUint128(id) {
						t.Fatalf("chain %d posting %d id %s group %s, want %d", i, j, tr.ID.BigInt(), tr.GroupID.BigInt(), id)
					}
					id++
					var ok bool
					switch want {
					case created:
						ok = tr.Err == nil && tr.Created
					case exists:
						ok = tr.Err == nil && !tr.Created
					case other:
						ok = tr.Err != nil && !tr.Created && !errors.Is(tr.Err, ErrInsufficientFunds) && !errors.Is(tr.Err, ErrConflict) &&
							!errors.Is(tr.Err, ErrIDAlreadyFailed) && !errors.Is(tr.Err, ErrChainFailed)
					default:
						ok = errors.Is(tr.Err, want.(error)) && !tr.Created
					}
					if !ok {
						t.Fatalf("chain %d posting %d = created %v, err %v; want %v", i, j, tr.Created, tr.Err, want)
					}
				}
			}
		})
	}
}

func TestTransferLookupError(t *testing.T) {
	boom := errors.New("boom")
	tbc := &fakeTB{
		createTransfers: func([]tb.Transfer) ([]tb.CreateTransferResult, error) {
			return []tb.CreateTransferResult{{Status: tb.TransferExists}, {Status: tb.TransferLinkedEventFailed}, {Status: tb.TransferCreated}}, nil
		},
		lookupTransfers: func([]tb.Uint128) ([]tb.Transfer, error) { return nil, boom },
	}
	got, err := New(testStore("").querier(), tbc).Transfer(context.Background(), testChains(2, 1))
	if err != nil {
		t.Fatalf("error transferring: %v", err)
	}
	if !errors.Is(got[0][0].Err, boom) || !errors.Is(got[0][1].Err, boom) || got[1][0].Err != nil || !got[1][0].Created {
		t.Fatalf("got %+v; want lookup error on chain 0 and chain 1 created", got)
	}
}

func TestTransferEnsuresWallets(t *testing.T) {
	rider := walletKey{"rider:1:wallet", "USD"}
	stored := tb.ID()
	boom := errors.New("boom")
	tests := []struct {
		name         string
		wallet       *fakeWallet // rider's USD wallet; nil for none
		create       func([]tb.Account) ([]tb.CreateAccountResult, error)
		wantErr      bool
		wantInserted bool
	}{
		{name: "no wallet", create: statuses(tb.AccountCreated), wantInserted: true},
		{name: "wallet not created", wallet: &fakeWallet{tbID: idToBytes(stored)}, create: statuses(tb.AccountExists)},
		{name: "account rejected", create: statuses(tb.AccountLedgerMustNotBeZero), wantErr: true, wantInserted: true},
		{name: "client error", wallet: &fakeWallet{tbID: idToBytes(stored)}, create: func([]tb.Account) ([]tb.CreateAccountResult, error) { return nil, boom }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStore("")
			delete(store.wallets, rider)
			if tt.wallet != nil {
				store.wallets[rider] = tt.wallet
			}
			var accounts []tb.Account
			var transfers []tb.Transfer
			tbc := &fakeTB{
				createAccounts: func(a []tb.Account) ([]tb.CreateAccountResult, error) {
					accounts = a
					return tt.create(a)
				},
				createTransfers: func(tr []tb.Transfer) ([]tb.CreateTransferResult, error) {
					transfers = tr
					return make([]tb.CreateTransferResult, len(tr)), nil
				},
			}
			_, err := New(store.querier(), tbc).Transfer(context.Background(), testChains(1))

			wantInserted, wantGets := []walletKey(nil), 1
			if tt.wantInserted {
				wantInserted, wantGets = []walletKey{rider}, 2
			}
			if !slices.Equal(store.inserted, wantInserted) || store.gets != wantGets {
				t.Fatalf("inserted %v, lookups %d; want %v, %d", store.inserted, store.gets, wantInserted, wantGets)
			}
			id, _ := idFromBytes(store.wallets[rider].tbID)
			want := tb.Account{ID: id, Ledger: 7, Code: tbCode(TypeLiability), Flags: tbFlags(TypeLiability, false)}
			if len(accounts) != 1 || accounts[0] != want {
				t.Fatalf("CreateAccounts sent %+v, want [%+v]", accounts, want)
			}
			if tt.wantErr {
				if err == nil || store.marked != nil || transfers != nil {
					t.Fatalf("err %v, marked %d, transfers %+v; want an error, no mark and no transfers", err, len(store.marked), transfers)
				}
				return
			}
			if err != nil {
				t.Fatalf("error transferring: %v", err)
			}
			if len(store.marked) != 1 || !slices.Equal(store.marked[0], idToBytes(id)) || !store.wallets[rider].created {
				t.Fatalf("marked %v, want [%v]", store.marked, idToBytes(id))
			}
			if len(transfers) != 1 || transfers[0].CreditAccountID != id || transfers[0].DebitAccountID != testWalletIDs["world"] {
				t.Fatalf("transfers %+v; want world to wallet %s", transfers, id.BigInt())
			}
		})
	}
}
