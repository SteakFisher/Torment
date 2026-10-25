package ledger

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"torment/internal/db"
	"torment/internal/db/sqlc"
)

// testPool is the migrated test database; nil when TEST_DATABASE_URL is unset.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run migrates the test database when TEST_DATABASE_URL is set, runs the tests and returns the exit code.
func run(m *testing.M) int {
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

// newIntegrationService returns a Service on an emptied test database, skipping when TEST_DATABASE_URL is unset.
func newIntegrationService(t *testing.T) *Service {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if _, err := testPool.Exec(context.Background(), "truncate currencies, accounts"); err != nil {
		t.Fatalf("error truncating tables: %v", err)
	}
	return New(sqlc.New(testPool))
}
