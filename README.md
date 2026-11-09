# Torment

A ledger service on top of [TigerBeetle](https://tigerbeetle.com). Callers name accounts with readable addresses such as `rider:123:wallet` and accounting types such as `liability`. Torment maps them to TigerBeetle accounts, so a write path that needs only TigerBeetle keeps TigerBeetle's speed.

- **TigerBeetle** holds balances and transfers.
- **Postgres** holds currencies, kinds, accounts and the mapping from each wallet to its TigerBeetle account.

**Docs:** run `make docs` and open `localhost:4321`, or read the Markdown in [`docs/src/content/docs`](docs/src/content/docs). Start with the [introduction](docs/src/content/docs/getting-started/introduction.md), which also compares Torment with Formance, and the [quickstart](docs/src/content/docs/getting-started/quickstart.md).

## Running locally
You need Go 1.26 and Docker.

```sh
make up      # Postgres 17 and a single-replica TigerBeetle 0.17.9
make run     # migrates the database, then serves on :8080 with the API key dev-key
make test    # unit and integration tests; integration tests use the torment_test database
make e2e     # runs the curl flow in scripts/e2e.sh against the running server
make docs    # the docs site at localhost:4321 (needs Bun)
make down
```

Without the containers, run only the unit tests: `TEST_DATABASE_URL= go test ./...`.

### Configuration

| Variable | |
|---|---|
| `DATABASE_URL` | Postgres connection string. Required |
| `TB_ADDRESS` | TigerBeetle address, such as `3000` or `10.0.0.1:3000`. Required |
| `API_KEYS` | comma-separated API keys. Required. Several keys let you rotate one without downtime |
| `ADDR` | listen address. Default `:8080` |

The Makefile sets all of these for development. The `Dockerfile` builds a production image.

## Layout
```
cmd/torment        wiring: config, Postgres, TigerBeetle clients, HTTP server
internal/ledger    the domain: accounts, wallets, transfers, queries
internal/db        Postgres store, migrations, sqlc queries
internal/httpapi   routes, JSON and the mapping from errors to statuses
scripts            e2e flow, test database setup
docs               the docs site (Astro Starlight)
```
