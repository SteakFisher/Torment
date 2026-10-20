export DATABASE_URL ?= postgres://torment:torment@localhost:5432/torment?sslmode=disable
export TB_ADDRESS ?= 3000

.PHONY: up down run test

up:
	docker compose up -d --wait

down:
	docker compose down

run:
	go run ./cmd/torment

test:
	go test ./...
