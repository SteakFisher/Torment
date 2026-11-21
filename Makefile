export DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment?sslmode=disable
export TB_ADDRESS ?= 3000
export TEST_DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment_test?sslmode=disable
export TEST_TB_ADDRESS ?= 3001
export TEST_CDC_SOURCE_URL ?= amqp://test:test@localhost:5672/tigerbeetle-test
export TEST_CDC_EVENTS_URL ?= amqp://test:test@localhost:5672/torment-test
export CDC_SOURCE_URL ?= amqp://torment:torment@localhost:5672/tigerbeetle
export CDC_EVENTS_URL ?= amqp://torment:torment@localhost:5672/torment
export RABBITMQ_API ?= http://consumer:consumer@localhost:15672
# Dev only: run and e2e share this key.
export API_KEYS ?= dev-key
export API_KEY ?= dev-key

.PHONY: up down run cdc test bench bench-compare e2e generate docs brokerage

up:
	docker compose up -d --wait

down:
	docker compose down

run:
	go run ./cmd/torment

# cdc exports events from TigerBeetle's CDC messages; run exactly one.
cdc:
	go run ./cmd/torment cdc

test:
	go test ./...

# bench runs the ledger benchmarks against the test database and cluster and saves them to bench/new.txt.
# Trust pg-queries/op and allocs/op; ns/op only compares before and after on the same laptop, since
# Docker on macOS fakes fsync.
bench:
	mkdir -p bench
	set -o pipefail; go test -run '^$$' -bench . -benchmem -count 6 ./internal/ledger | tee bench/new.txt

# bench-compare compares bench/old.txt with bench/new.txt; copy new.txt to old.txt to set a baseline.
bench-compare:
	go run golang.org/x/perf/cmd/benchstat@v0.0.0-20261117185222-4febdb082b2c bench/old.txt bench/new.txt

# e2e needs the server (make run) and the exporter (make cdc) running; RABBITMQ_API= skips the export checks.
e2e:
	./scripts/e2e.sh

# brokerage runs the tutorial's requests; it needs the server and, unless RABBITMQ_API= is set, the exporter.
brokerage:
	./scripts/brokerage.sh

# generate rewrites internal/db/sqlc from the queries; commit the result.
generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

# Serves the docs site with live reload at http://localhost:4321.
docs:
	cd docs && bun install && bun run dev
