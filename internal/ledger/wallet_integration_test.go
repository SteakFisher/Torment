package ledger_test

import (
	"context"
	"sync"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

// TigerBeetle account codes of TypeAsset and TypeLiability.
const assetCode, liabilityCode = 1, 3

// lookupTBAccount returns the TigerBeetle account with id, failing the test when it does not exist.
func lookupTBAccount(t *testing.T, id tb.Uint128) tb.Account {
	t.Helper()
	accounts, err := testTB.LookupAccounts([]tb.Uint128{id})
	if err != nil {
		t.Fatalf("error looking up tigerbeetle account: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("tigerbeetle account %s not found", id.BigInt())
	}
	return accounts[0]
}

func TestIntegrationRegisterAccountWallets(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	ledgers := map[string]uint32{}
	for _, code := range []string{"USD/2", "AED/2"} {
		c, err := s.RegisterCurrency(ctx, code)
		if err != nil {
			t.Fatalf("error registering currency %s: %v", code, err)
		}
		ledgers[c.Name] = uint32(c.Ledger)
	}
	debitsLimited := tb.AccountFlags{DebitsMustNotExceedCredits: true, History: true}.ToUint16()

	p := ledger.RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}}
	first, _, err := s.RegisterAccount(ctx, p)
	if err != nil || len(first.Wallets) != 2 {
		t.Fatalf("first register: %+v, err %v", first, err)
	}
	for i, w := range first.Wallets {
		a := lookupTBAccount(t, w.ID)
		if a.Ledger != ledgers[w.Currency] || a.Code != liabilityCode || a.Flags != debitsLimited {
			t.Fatalf("tigerbeetle account %d: ledger %d, code %d, flags %b", i, a.Ledger, a.Code, a.Flags)
		}
	}

	replay, created, err := s.RegisterAccount(ctx, p)
	if err != nil || created {
		t.Fatalf("replay: created %v, err %v", created, err)
	}
	for i := range first.Wallets {
		if replay.Wallets[i] != first.Wallets[i] {
			t.Fatalf("replay wallet %d = %+v, want %+v", i, replay.Wallets[i], first.Wallets[i])
		}
	}
	var n int
	if err := testPool.QueryRow(ctx, "select count(*) from wallets").Scan(&n); err != nil || n != 2 {
		t.Fatalf("wallet rows = %d, err %v; want 2", n, err)
	}

	world, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "asset", AllowNegative: true, Currencies: []string{"USD"}})
	if err != nil {
		t.Fatalf("world: %v", err)
	}
	if a := lookupTBAccount(t, world.Wallets[0].ID); a.Flags != (tb.AccountFlags{History: true}).ToUint16() || a.Code != assetCode {
		t.Fatalf("world tigerbeetle account: code %d, flags %b; want code %d, only history", a.Code, a.Flags, assetCode)
	}

	// a wallet row whose TigerBeetle account was never created
	orphan := tb.ID()
	orphanBytes := orphan.Bytes()
	if _, err := testPool.Exec(ctx, "insert into wallets (tb_id, account_id, currency) select $1, id, 'AED' from accounts where address = 'world'", orphanBytes[:]); err != nil {
		t.Fatalf("error inserting orphan wallet: %v", err)
	}
	repaired, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "asset", AllowNegative: true, Currencies: []string{"AED"}})
	if err != nil || repaired.Wallets[0].ID != orphan {
		t.Fatalf("repair: %+v, err %v; want wallet id %s", repaired, err, orphan.BigInt())
	}
	if a := lookupTBAccount(t, orphan); a.Ledger != ledgers["AED"] {
		t.Fatalf("repaired tigerbeetle account ledger = %d, want %d", a.Ledger, ledgers["AED"])
	}
	if !tbCreated(t, "world", "AED") {
		t.Fatal("repaired wallet tb_created = false, want true")
	}
}

// tbCreated returns tb_created of the wallet in currency of the account at address.
func tbCreated(t *testing.T, address, currency string) bool {
	t.Helper()
	var created bool
	err := testPool.QueryRow(context.Background(),
		"select w.tb_created from wallets w join accounts a on a.id = w.account_id where a.address = $1 and w.currency = $2",
		address, currency).Scan(&created)
	if err != nil {
		t.Fatalf("error reading tb_created of %s %s: %v", address, currency, err)
	}
	return created
}

func TestIntegrationTransferCreatesWallets(t *testing.T) {
	s := newIntegrationService(t)
	ctx := context.Background()
	if _, err := s.RegisterCurrency(ctx, "USD/2"); err != nil {
		t.Fatalf("error registering currency: %v", err)
	}
	if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: "world", Type: "equity", AllowNegative: true, Currencies: []string{"USD"}}); err != nil {
		t.Fatalf("world: %v", err)
	}
	for _, address := range []string{"rider:1:wallet", "rider:2:wallet", "rider:3:wallet"} {
		if _, _, err := s.RegisterAccount(ctx, ledger.RegisterAccountParams{Address: address, Type: "liability"}); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	// transfer posts amount USD from world to address with a new id and fails the test unless it is created
	transfer := func(address, amount string) {
		t.Helper()
		got, err := s.Transfer(ctx, [][]ledger.Posting{{{ID: tb.ID().BigInt().String(), Source: "world", Destination: address, Currency: "USD", Amount: amount}}})
		if err != nil || got[0][0].Err != nil || !got[0][0].Created {
			t.Errorf("transfer to %s = %+v, err %v; want created", address, got, err)
		}
	}
	checkUSD := func(address string, amount int64) {
		t.Helper()
		got, err := s.Balances(ctx, address, time.Time{})
		if err != nil || len(got) != 1 || got[0].Currency != "USD" || got[0].Amount.Int64() != amount {
			t.Fatalf("%s balances = %+v, err %v; want USD %d", address, got, err, amount)
		}
		if !tbCreated(t, address, "USD") {
			t.Fatalf("%s USD wallet tb_created = false, want true", address)
		}
	}

	transfer("rider:1:wallet", "100")
	checkUSD("rider:1:wallet", 100)

	// a wallet row whose TigerBeetle account was never created
	orphan := tb.ID()
	orphanBytes := orphan.Bytes()
	if _, err := testPool.Exec(ctx, "insert into wallets (tb_id, account_id, currency) select $1, id, 'USD' from accounts where address = 'rider:2:wallet'", orphanBytes[:]); err != nil {
		t.Fatalf("error inserting orphan wallet: %v", err)
	}
	transfer("rider:2:wallet", "50")
	checkUSD("rider:2:wallet", 50)
	if a := lookupTBAccount(t, orphan); a.CreditsPosted != tb.ToUint128(50) {
		t.Fatalf("orphan tigerbeetle account credits = %s, want 50", a.CreditsPosted.BigInt())
	}

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			transfer("rider:3:wallet", "10")
		}()
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	var rows int
	if err := testPool.QueryRow(ctx, "select count(*) from wallets w join accounts a on a.id = w.account_id where a.address = 'rider:3:wallet'").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rider:3:wallet wallet rows = %d, err %v; want 1", rows, err)
	}
	checkUSD("rider:3:wallet", 10*n)
}
