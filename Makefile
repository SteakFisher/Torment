export DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment?sslmode=disable
export TB_ADDRESS ?= 3000
export TEST_DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment_test?sslmode=disable
export TEST_TB_ADDRESS ?= 3000

.PHONY: up down run test generate

up:
	docker compose up -d --wait

down:
	docker compose down

run: generate
	go run ./cmd/torment

test: generate
	go test ./...

generate:
	go tool sqlc generate
