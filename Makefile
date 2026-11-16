export DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment?sslmode=disable
export TB_ADDRESS ?= 3000
export TEST_DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment_test?sslmode=disable
export TEST_TB_ADDRESS ?= 3001
export CDC_SOURCE_URL ?= amqp://torment:torment@localhost:5672/tigerbeetle
export CDC_EVENTS_URL ?= amqp://torment:torment@localhost:5672/torment
export RABBITMQ_API ?= http://consumer:consumer@localhost:15672
# Dev only: run and e2e share this key.
export API_KEYS ?= dev-key
export API_KEY ?= dev-key

.PHONY: up down run cdc test e2e generate docs brokerage

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

# e2e needs the server (make run) and the exporter (make cdc) running; RABBITMQ_API= skips the export checks.
e2e:
	./scripts/e2e.sh

# brokerage runs the tutorial's requests; it also needs the server running.
brokerage:
	./scripts/brokerage.sh

# generate rewrites internal/db/sqlc from the queries; commit the result.
generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

# Serves the docs site with live reload at http://localhost:4321.
docs:
	cd docs && bun install && bun run dev
