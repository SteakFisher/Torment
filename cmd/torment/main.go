package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db"
	"torment/internal/db/sqlc"
	"torment/internal/httpapi"
	"torment/internal/ledger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("torment exited", "err", err)
		os.Exit(1)
	}
}

// run connects to Postgres and TigerBeetle, applies migrations and serves HTTP until the server fails.
// It reads DATABASE_URL and TB_ADDRESS (required) and ADDR (default ":8080") from the environment.
func run() error {
	ctx := context.Background()

	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	tbAddress, err := requireEnv("TB_ADDRESS")
	if err != nil {
		return err
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("error opening postgres pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("error pinging postgres: %w", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	tbClient, err := tb.NewClient(tb.ToUint128(0), []string{tbAddress})
	if err != nil {
		return fmt.Errorf("error creating tigerbeetle client: %w", err)
	}
	defer tbClient.Close()
	if err := pingTB(tbClient, 5*time.Second); err != nil {
		return fmt.Errorf("error pinging tigerbeetle: %w", err)
	}

	slog.Info("listening", "addr", addr)
	svc := ledger.New(sqlc.New(pool))
	return http.ListenAndServe(addr, httpapi.New(svc))
}

// requireEnv returns the value of the environment variable key, or an error when it is unset or empty.
func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is not set", key)
	}
	return v, nil
}

// pingTB round-trips a no-op request, failing if no reply arrives within timeout.
func pingTB(c tb.Client, timeout time.Duration) error {
	errc := make(chan error, 1)
	go func() { errc <- c.Nop() }()
	select {
	case err := <-errc:
		return err
	case <-time.After(timeout):
		return errors.New("timed out")
	}
}
