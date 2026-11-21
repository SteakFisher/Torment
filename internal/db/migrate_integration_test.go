package db

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrateConcurrently starts several instances' migrations at once on an empty database, as a
// deploy of several API instances does, and expects every one to succeed.
func TestMigrateConcurrently(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("torment_migrate_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "create database "+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "drop database "+name+" with (force)")

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = name

	const instances = 32
	errs := make(chan error, instances)
	var wg sync.WaitGroup
	for range instances {
		wg.Go(func() {
			pool, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				errs <- err
				return
			}
			defer pool.Close()
			errs <- Migrate(ctx, pool)
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
