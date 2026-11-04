export DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment?sslmode=disable
export TB_ADDRESS ?= 3000
export TEST_DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment_test?sslmode=disable
export TEST_TB_ADDRESS ?= 3000
# Dev only: run and e2e share this key.
export API_KEYS ?= dev-key
export API_KEY ?= dev-key

.PHONY: up down run test e2e generate

up:
	docker compose up -d --wait

down:
	docker compose down

run: generate
	go run ./cmd/torment

test: generate
	go test ./...

# e2e needs the server running (make run).
e2e:
	./scripts/e2e.sh

generate:
	go tool sqlc generate
