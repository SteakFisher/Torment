package ledger

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db"
	"torment/internal/db/sqlc"
)

var (
	// testPool is the migrated test database; nil when TEST_DATABASE_URL is unset.
	testPool *pgxpool.Pool
	// testTB is the test TigerBeetle cluster; nil when TEST_TB_ADDRESS is unset.
	testTB tb.Client
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run migrates the test database when TEST_DATABASE_URL is set, connects to TigerBeetle when
// TEST_TB_ADDRESS is set, runs the tests and returns the exit code.
func run(m *testing.M) int {
	if addr := os.Getenv("TEST_TB_ADDRESS"); addr != "" {
		client, err := tb.NewClient(tb.ToUint128(0), []string{addr})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating tigerbeetle client:", err)
			return 1
		}
		defer client.Close()
		testTB = client
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return m.Run()
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening test pool:", err)
		return 1
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	testPool = pool
	return m.Run()
}

// newIntegrationService returns a Service on an emptied test database and the test TigerBeetle cluster,
// skipping unless TEST_DATABASE_URL and TEST_TB_ADDRESS are both set.
func newIntegrationService(t *testing.T) *Service {
	t.Helper()
	if testPool == nil || testTB == nil {
		t.Skip("TEST_DATABASE_URL or TEST_TB_ADDRESS is not set")
	}
	if _, err := testPool.Exec(context.Background(), "truncate wallets, accounts, currencies"); err != nil {
		t.Fatalf("error truncating tables: %v", err)
	}
	return New(sqlc.New(testPool), testTB)
}
