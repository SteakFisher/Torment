package ledger

import (
	"context"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

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
	debitsLimited := tb.AccountFlags{DebitsMustNotExceedCredits: true}.ToUint16()

	p := RegisterAccountParams{Address: "rider:1:wallet", Type: "liability", Currencies: []string{"USD", "AED"}}
	first, _, err := s.RegisterAccount(ctx, p)
	if err != nil || len(first.Wallets) != 2 {
		t.Fatalf("first register: %+v, err %v", first, err)
	}
	for i, w := range first.Wallets {
		a := lookupTBAccount(t, w.ID)
		if a.Ledger != ledgers[w.Currency] || a.Code != tbCode(TypeLiability) || a.Flags != debitsLimited {
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

	world, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "world", Type: "asset", AllowNegative: true, Currencies: []string{"USD"}})
	if err != nil {
		t.Fatalf("world: %v", err)
	}
	if a := lookupTBAccount(t, world.Wallets[0].ID); a.Flags != 0 || a.Code != tbCode(TypeAsset) {
		t.Fatalf("world tigerbeetle account: code %d, flags %b; want code %d, no flags", a.Code, a.Flags, tbCode(TypeAsset))
	}

	// a wallet row whose TigerBeetle account was never created
	orphan := tb.ID()
	if _, err := testPool.Exec(ctx, "insert into wallets (tb_id, account_id, currency) select $1, id, 'AED' from accounts where address = 'world'", idToBytes(orphan)); err != nil {
		t.Fatalf("error inserting orphan wallet: %v", err)
	}
	repaired, _, err := s.RegisterAccount(ctx, RegisterAccountParams{Address: "world", Type: "asset", AllowNegative: true, Currencies: []string{"AED"}})
	if err != nil || repaired.Wallets[0].ID != orphan {
		t.Fatalf("repair: %+v, err %v; want wallet id %s", repaired, err, orphan.BigInt())
	}
	if a := lookupTBAccount(t, orphan); a.Ledger != ledgers["AED"] {
		t.Fatalf("repaired tigerbeetle account ledger = %d, want %d", a.Ledger, ledgers["AED"])
	}
}
