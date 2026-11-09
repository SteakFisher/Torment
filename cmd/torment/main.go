package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db"
	"torment/internal/httpapi"
	"torment/internal/ledger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("torment exited", "err", err)
		os.Exit(1)
	}
}

// run connects to Postgres and TigerBeetle, applies migrations and serves HTTP until the server fails or
// the process gets SIGINT or SIGTERM, then waits up to 10 seconds for requests in flight to finish.
// It reads DATABASE_URL, TB_ADDRESS and API_KEYS (required) and ADDR (default ":8080") from the environment.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	tbAddress, err := requireEnv("TB_ADDRESS")
	if err != nil {
		return err
	}
	apiKeys, err := parseAPIKeys(os.Getenv("API_KEYS"))
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

	// A client sends one request at a time, so writes get their own client and never wait behind reads.
	tbWrites, err := connectTB(tbAddress)
	if err != nil {
		return err
	}
	defer tbWrites.Close()
	tbReads, err := connectTB(tbAddress)
	if err != nil {
		return err
	}
	defer tbReads.Close()

	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(ledger.New(db.NewStore(pool), tbWrites, tbReads), apiKeys),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("listening", "addr", addr)
	select {
	case err := <-errc:
		return fmt.Errorf("error serving http: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down http server: %w", err)
	}
	return nil
}

// requireEnv returns the value of the environment variable key, or an error when it is unset or empty.
func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is not set", key)
	}
	return v, nil
}

// parseAPIKeys splits a comma-separated list of API keys, dropping spaces around each key and empty
// entries. It returns an error when no key remains.
func parseAPIKeys(s string) ([]string, error) {
	var keys []string
	for k := range strings.SplitSeq(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("API_KEYS is not set")
	}
	return keys, nil
}

// connectTB returns a TigerBeetle client for the cluster at address, once it answers a ping within 5
// seconds.
func connectTB(address string) (tb.Client, error) {
	c, err := tb.NewClient(tb.ToUint128(0), []string{address})
	if err != nil {
		return nil, fmt.Errorf("error creating tigerbeetle client: %w", err)
	}
	if err := pingTB(c, 5*time.Second); err != nil {
		c.Close()
		return nil, fmt.Errorf("error pinging tigerbeetle: %w", err)
	}
	return c, nil
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
