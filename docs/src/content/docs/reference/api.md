---
title: API
---

JSON over HTTP. The server listens on `ADDR` (default `:8080`).

## Conventions
- **Auth:** every route except `GET /healthz` and `GET /readyz` needs `Authorization: Bearer <key>`, with a key from `API_KEYS`. A missing or wrong key returns 401.
- **Errors:** a failed request returns `{"error": {"code": "<code>", "message": "<message>"}}`. Match on `code`; `message` is for people and may change. See [Errors and retries](/reference/errors/).

  | Status | `code` | Meaning |
  |---|---|---|
  | 400 | `invalid_request` | malformed JSON, an unknown field or an invalid value |
  | 400 | `batch_too_large` | more postings than the TigerBeetle cluster accepts in one request |
  | 400 | `pattern_too_broad` | an address pattern matches more wallets than allowed |
  | 401 | `unauthorized` | missing or wrong API key |
  | 404 | `account_not_found` | the account does not exist |
  | 404 | `currency_not_found` | the currency is not registered |
  | 404 | `kind_not_found` | the kind is not registered |
  | 404 | `route_not_found` | no route has this path |
  | 405 | `method_not_allowed` | the path exists, but not with this method; `Allow` lists the methods it takes |
  | 409 | `conflict` | the input clashes with something already stored |
  | 500 | `internal` | internal error, logged on the server; safe to retry |
  | 503 | `not_ready` | `GET /readyz` only: Postgres or TigerBeetle did not answer |

- **Ids:** posting ids and group ids are UUIDs, such as `0190f3a2-7b1c-7def-8abc-0123456789ab`, in any case; responses use lowercase. The nil and max UUIDs are rejected. Examples write them as `00000000-0000-0000-0000-000000001001` so they're easy to read; real code should generate a UUIDv7 for each posting with any UUID library.
- **Amounts:** base-10 strings of digits, with no sign or leading zeros, because they are 128-bit. Amounts are integers in minor units: `"1050"` in `USD/2` is 10.50 USD.
- **Times:** RFC 3339, with optional fractional seconds and offset. Transfer timestamps are TigerBeetle's commit time in nanoseconds.
- **Bodies:** one JSON value, at most 1 MiB.
- **Addresses:** colon-separated segments of `[a-zA-Z0-9_-]`, at most 255 bytes, such as `rider:123:wallet`. An address containing `*` is a pattern: each `*` matches exactly one segment, so `rider:*:wallet` matches every rider's wallet and `rider:123:*` matches every pot of rider 123. An invalid address or pattern in a path returns 400 `invalid_request`.
- **Paging:** a list response has `next` when more results remain. Pass it back as `cursor` for the next page. `limit` defaults to 100, and goes from 1 to 1000.

## Health

### `GET /healthz`
Returns 200 with the body `ok` while the process is serving. It needs no API key.

### `GET /readyz`
Returns 200 with the body `ok` when Postgres and TigerBeetle both answer within 10 seconds, and 503 with code `not_ready` otherwise. It needs no API key.

### Request ids
Every response has an `X-Request-Id` header. Send your own (up to 64 bytes) to have it used; otherwise Torment makes one. The same id appears in the server's log lines for that request, so quote it when reporting a problem.

## Currencies

### `POST /currencies`
Registers a currency. `code` is `NAME/PRECISION`: an uppercase name, and the number of decimal places in one minor unit, from 0 to 18 with no leading zeros (0 if omitted).

```json
{"code": "USD/2"}
```

Returns `{"name": "USD", "precision": 2}`.

| Status | When |
|---|---|
| 201 | registered |
| 200 | already registered with the same precision |
| 400 | malformed code |
| 409 | already registered with a different precision |

### `GET /currencies`
Returns 200 with every currency, ordered by name.

```json
{"currencies": [{"name": "EUR", "precision": 2}, {"name": "USD", "precision": 2}]}
```

## Kinds

A kind says what a transfer is, such as `deposit`, `trade` or `commission`. Every posting needs one, and you can filter transfer history by it.

### `POST /kinds`
Registers a kind. `name` is lowercase letters, digits and underscores, starting with a letter, at most 63 bytes.

```json
{"name": "deposit"}
```

Returns `{"name": "deposit"}`.

| Status | When |
|---|---|
| 201 | registered |
| 200 | already registered |
| 400 | malformed name |

A kind can't be changed or removed once registered.

### `GET /kinds`
Returns 200 with every kind, ordered by name.

```json
{"kinds": [{"name": "deposit"}, {"name": "trade"}]}
```

## Accounts

An account is one pot of money, named by its address. It has one wallet per currency, which is created when the account is registered with `currencies` or by its first transfer in that currency.

| Type | Balance grows with |
|---|---|
| `asset`, `expense` | debits |
| `liability`, `equity`, `revenue` | credits |

An account's balance can't go below zero unless it was registered with `allow_negative: true`. This can't be changed later.

### `POST /accounts`
Registers an account. `currencies` is optional, and each currency must already be registered.

```json
{"address": "rider:123:wallet", "type": "liability", "allow_negative": false, "currencies": ["USD"]}
```

Returns the account, with `wallets` listing the requested currencies in the order they were sent:

```json
{
  "address": "rider:123:wallet",
  "type": "liability",
  "allow_negative": false,
  "created_at": "2026-11-06T10:00:00.123456Z",
  "wallets": [{"currency": "USD", "precision": 2}]
}
```

| Status | When |
|---|---|
| 201 | registered |
| 200 | already registered with the same fields. Any missing wallets in `currencies` are added |
| 400 | invalid address or type (`invalid_request`) |
| 404 | an unregistered currency (`currency_not_found`) |
| 409 | already registered with a different type or `allow_negative` |

### `GET /accounts/{address}`
Returns 200 with the matching accounts ordered by address, each in the same shape as above. For a pattern it takes `cursor` and `limit`. For an address, `accounts` holds just that account, or it returns 404 `account_not_found`.

```json
{"accounts": [ ... ], "next": "rider:123:wallet"}
```

### `GET /accounts`
Lists every account, the same way a pattern does.

## Balances

### `GET /accounts/{address}/balances`
Query parameters:
- `currency`: only this currency. An unregistered currency returns 404 `currency_not_found`.
- `at`: the balance after the last transfer at or before this time. Without it, the balance is the current one.

Returns 200 with a total for each type and currency, plus each matching account's balances, one per wallet, ordered by currency. Types are never added together. A wallet with no transfers reads as zero. A pattern leaves out accounts with no wallet in scope.

For an address, `accounts` holds just that account and the totals are its balances, or it returns 404 `account_not_found`. An account with no wallet in scope is still listed, with `"balances": []` and `"totals": []`. A pattern matching more than 1000 wallets returns 400 `pattern_too_broad`.

```json
{
  "address": "rider:123:*",
  "totals": [{"type": "liability", "currency": "USD", "precision": 2, "amount": "8000"}],
  "accounts": [
    {"address": "rider:123:bonus", "type": "liability", "balances": [{"currency": "USD", "precision": 2, "amount": "1000"}]},
    {"address": "rider:123:wallet", "type": "liability", "balances": [{"currency": "USD", "precision": 2, "amount": "7000"}]}
  ]
}
```

`amount` follows the account type's normal side, so it is negative only for accounts with `allow_negative`.

## Transfers

### `POST /transfers`
Moves money between accounts. Each posting debits the account in `debit` and credits the account in `credit` by `amount`. Whether that raises or lowers each balance depends on the account's type: see the table under [Accounts](#accounts). Both accounts must be registered, and missing wallets are created.

| Field | |
|---|---|
| `id` | required. Your UUID for the posting, unique within the request. Use a UUIDv7: its ids increase over time, and random ids slow TigerBeetle down |
| `debit`, `credit` | account addresses, which must differ |
| `currency` | a registered currency name |
| `amount` | greater than zero, in minor units |
| `kind` | required. A registered kind, such as `deposit` |
| `group_id` | optional. Links related postings, such as a payment and its refund. Defaults to `id` |

`chains` is a list of chains, and each chain is a list of postings. Either all of a chain is applied, or none of it is. Separate chains are independent of each other. A posting on its own is a chain of one.

```json
{"chains": [
  [
    {"id": "00000000-0000-0000-0000-000000001001", "debit": "world", "credit": "rider:123:wallet", "currency": "USD", "amount": "10000", "kind": "top_up"}
  ],
  [
    {"id": "00000000-0000-0000-0000-000000001002", "debit": "rider:123:wallet", "credit": "merchant:9", "currency": "USD", "amount": "2500", "kind": "payment"},
    {"id": "00000000-0000-0000-0000-000000001003", "debit": "rider:123:wallet", "credit": "platform:fees", "currency": "USD", "amount": "100", "kind": "fee"}
  ]
]}
```

Returns 200 with one result per posting, nested in chains the same way as the request:

```json
{"chains": [
  [
    {"id": "00000000-0000-0000-0000-000000001001", "group_id": "00000000-0000-0000-0000-000000001001", "status": "created"}
  ],
  [
    {"id": "00000000-0000-0000-0000-000000001002", "group_id": "00000000-0000-0000-0000-000000001002", "status": "failed", "error": "insufficient_funds", "message": "..."},
    {"id": "00000000-0000-0000-0000-000000001003", "group_id": "00000000-0000-0000-0000-000000001003", "status": "failed", "error": "chain_failed", "message": "..."}
  ]
]}
```

| `status` | `error` | Meaning | What to do |
|---|---|---|---|
| `created` | | applied now | nothing |
| `exists` | | already applied by an earlier request | nothing |
| `failed` | `insufficient_funds` | an account would go below zero, and it wasn't registered with `allow_negative`. Nothing in the chain was applied | top up, and resend this posting **with a new id** |
| `failed` | `chain_failed` | another posting in the chain failed | fix that one; this id can be reused |
| `failed` | `conflict` | the id was already used with different fields, such as another `kind` | use a new id |
| `failed` | `id_already_failed` | the id belongs to a posting that failed before | use a new id |
| `failed` | `overflow` | an account's total debits or credits would pass 2¹²⁸−1 minor units. Nothing in the chain was applied | use a smaller amount and a new id |
| `failed` | `internal` | unexpected error, logged on the server | resend with the same id |

The whole request fails, and nothing is applied, only for:

| Status | When |
|---|---|
| 400 | invalid input, a missing `kind`, a duplicate id in the request, or more than 8189 postings (`invalid_request`); more postings than the cluster accepts (`batch_too_large`) |
| 404 | a kind is not registered (`kind_not_found`), an account is not registered (`account_not_found`) or a currency is not registered (`currency_not_found`) |
| 500 | an error before or while sending to TigerBeetle. The outcome is unknown, so resend the same request with the same ids |

**Retries are safe.** Resending a posting with the same id and fields never applies it twice; it returns `exists`. Resend each chain as it was: whether a posting is linked to the next one in its chain is one of its fields, so an id sent alone and later inside a chain, or the other way round, can return `conflict`.

**Batch when you have volume.** One request can carry thousands of independent postings and chains, and that is much faster than one request per posting. Send whatever has queued since your last request returned, and never wait to fill a batch. See [Batching](/internals/decisions/#batching).

### `GET /groups/{group_id}/transfers`
Returns 200 with the group's transfers, newest first, a page at a time. An unknown group returns an empty list. An id that isn't a UUID returns 400.

Query parameters:
- `kind`: only transfers of this kind. An unregistered kind returns 404 `kind_not_found`.
- `limit`: page size, default 100, at most 1000.
- `cursor`: the `next` value from the previous page.

When more transfers remain, the response has `next`. Pass it back as `cursor` for the next page. With no `next`, you have them all.

```json
{
  "group_id": "00000000-0000-0000-0000-000000001001",
  "transfers": [
    {"id": "00000000-0000-0000-0000-000000001004", "group_id": "00000000-0000-0000-0000-000000001001", "debit": "rider:123:wallet", "credit": "world", "currency": "USD", "amount": "3000", "kind": "refund", "timestamp": "2026-11-06T10:05:00.123456789Z"},
    {"id": "00000000-0000-0000-0000-000000001001", "group_id": "00000000-0000-0000-0000-000000001001", "debit": "world", "credit": "rider:123:wallet", "currency": "USD", "amount": "10000", "kind": "top_up", "timestamp": "2026-11-06T10:00:00.123456789Z"}
  ]
}
```

### `GET /accounts/{address}/transfers`
An account's transfer history, newest first.

Query parameters:
- `currency`: only this currency. An unregistered currency returns 404 `currency_not_found`.
- `kind`: only transfers of this kind.
- `from`: the earliest time, inclusive.
- `cursor`: the latest time, exclusive. Pass `next` here for the next page.
- `limit`: the page size.

Returns 200, 404 `account_not_found` for an unknown address, or 404 `kind_not_found` for an unregistered kind. Each transfer has the same fields as in a group's transfers. `next` is the last transfer's timestamp, to pass back as `cursor` for the next page.

```json
{"address": "rider:123:wallet", "transfers": [ ... ], "next": "2026-11-06T10:00:00.123456789Z"}
```

For a pattern, it merges the history of every matching account, and a transfer between two of them appears once. A pattern matching more than 100 wallets returns 400 `pattern_too_broad`.
