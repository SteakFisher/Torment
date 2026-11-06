# Torment

A ledger service on top of [TigerBeetle](https://tigerbeetle.com). Callers name accounts with readable addresses such as `rider:123:wallet` and accounting types such as `liability`. Torment maps them to TigerBeetle accounts, so a write path that needs only TigerBeetle keeps TigerBeetle's speed.

- **TigerBeetle** holds balances and transfers.
- **Postgres** holds currencies, accounts and the mapping from each wallet to its TigerBeetle account.

## Concepts
- **Currency.** What money is counted in. `USD/2` means US dollars with 2 decimal places. Torment never uses decimals: you send amounts in the smallest unit, so `"1050"` is $10.50.
- **Account.** A named pot of money, like `rider:123:wallet` or `platform:fees`. The name is up to you; the colons just keep related pots together. Every account has a type that says what it is to your business:

  | Type | Is | Example |
  |---|---|---|
  | `asset` | money you own | your bank balance |
  | `liability` | money you hold for someone else | a rider's wallet |
  | `equity` | money put into the business | `world`, where money comes from |
  | `revenue` | money you earned | fees |
  | `expense` | money you spent | card processing costs |

  New to debits and credits? TigerBeetle's [Financial Accounting](https://docs.tigerbeetle.com/coding/financial-accounting/) guide explains them and these five types in a few minutes.

  An account can't go below zero unless you register it with `allow_negative: true`. That's usually only for a source of money like `world`.
- **Wallet.** One account can hold several currencies, and each currency gets its own wallet. You never create wallets: the first time money in a new currency touches an account, Torment makes the wallet.
- **Posting.** One movement of money: take this amount from `source` and give it to `destination`. You give every posting your own id. If a request times out, send it again with the same id. Torment will never apply it twice.
- **Chain.** A set of postings that must all happen or none of them. "Pay the merchant $25 and charge a $1 fee" is a chain: if the rider can't afford both, neither happens.
- **Group.** A label (`group_id`) that ties related postings together so you can fetch them later. For example, a payment and its refund share a group.
- **Pattern.** An address with `*` in it, used to read many accounts at once. `rider:123:*` means all of rider 123's pots, and `rider:*:wallet` means every rider's wallet.

## Running locally
You need Go 1.26 and Docker.

```sh
make up      # Postgres 17 and a single-replica TigerBeetle 0.17.9
make run     # migrates the database, then serves on :8080 with the API key dev-key
make test    # unit and integration tests; integration tests use the torment_test database
make e2e     # runs the curl flow in scripts/e2e.sh against the running server
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

## Example

```sh
auth='Authorization: Bearer dev-key'
curl -H "$auth" -d '{"code":"USD/2"}' localhost:8080/currencies
curl -H "$auth" -d '{"address":"world","type":"equity","allow_negative":true}' localhost:8080/accounts
curl -H "$auth" -d '{"address":"rider:1:wallet","type":"liability"}' localhost:8080/accounts
curl -H "$auth" -d '{"postings":[{"id":"1","source":"world","destination":"rider:1:wallet","currency":"USD","amount":"10000"}]}' localhost:8080/transfers
curl -H "$auth" localhost:8080/accounts/rider:1:wallet/balances
```

## Docs
- [docs/api.md](docs/api.md): every route, with its request, response and status codes.
- [docs/decisions.md](docs/decisions.md): why it works the way it does, what happens when each step fails, and how to batch.

## Layout
```
cmd/torment        wiring: config, Postgres, TigerBeetle clients, HTTP server
internal/ledger    the domain: accounts, wallets, transfers, queries
internal/db        Postgres store, migrations, sqlc queries
internal/httpapi   routes, JSON and the mapping from errors to statuses
scripts            e2e flow, test database setup
```
