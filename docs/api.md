# API

JSON over HTTP. The server listens on `ADDR` (default `:8080`).

## Conventions
- **Auth:** every route except `GET /healthz` needs `Authorization: Bearer <key>`, with a key from `API_KEYS`. A missing or wrong key returns 401.
- **Errors:** a failed request returns `{"error": "<message>"}`.

  | Status | Meaning |
  |---|---|
  | 400 | malformed JSON, an unknown field or an invalid value |
  | 401 | missing or wrong API key |
  | 404 | the account, currency or wallet does not exist |
  | 409 | the input clashes with something already stored |
  | 500 | internal error, logged on the server; safe to retry |

- **Numbers:** ids, group ids and amounts are base-10 strings, because they are 128-bit. Amounts are integers in minor units: `"1050"` in `USD/2` is 10.50 USD.
- **Times:** RFC 3339, with optional fractional seconds and offset. Transfer timestamps are TigerBeetle's commit time in nanoseconds.
- **Bodies:** at most 1 MiB.
- **Addresses:** colon-separated segments of `[a-zA-Z0-9_-]`, at most 255 bytes, such as `rider:123:wallet`. An address containing `*` is a pattern: each `*` matches exactly one segment, so `rider:*:wallet` matches every rider's wallet and `rider:123:*` matches every pot of rider 123.
- **Paging:** a list response has `next` when more results remain. Pass it back as `after` for accounts or `before` for transfers. `limit` defaults to 100, and the maximum is 1000.

## Health

### `GET /healthz`
Returns 200 with the body `ok`. It needs no API key.

## Currencies

### `POST /currencies`
Registers a currency. `code` is `NAME/PRECISION`: an uppercase name, and the number of decimal places in one minor unit, from 0 to 18 (0 if omitted).

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
| 400 | invalid address or type, or an unregistered currency |
| 409 | already registered with a different type or `allow_negative` |

### `GET /accounts/{address}`
For an address, returns 200 with that account, in the same shape as above, or 404.

For a pattern, returns 200 with the matching accounts ordered by address. It takes `after` and `limit`.

```json
{"accounts": [ ... ], "next": "rider:123:wallet"}
```

### `GET /accounts`
Lists every account, the same way a pattern does.

## Balances

### `GET /accounts/{address}/balances`
Query parameters:
- `currency`: only this currency.
- `at`: the balance after the last transfer at or before this time. Without it, the balance is the current one.

For an address, returns 200 with one balance per wallet, ordered by currency, or 404. A wallet with no transfers reads as zero.

```json
{"address": "rider:123:wallet", "balances": [{"currency": "USD", "precision": 2, "amount": "7000"}]}
```

`amount` follows the account type's normal side, so it is negative only for accounts with `allow_negative`.

For a pattern, returns 200 with a total for each type and currency, plus each matching account's balances. Types are never added together. Accounts with no wallet in scope are left out. A pattern matching more than 1000 wallets returns 400.

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

## Transfers

### `POST /transfers`
Moves money between accounts. Each posting debits `source` and credits `destination`. Both accounts must be registered, and missing wallets are created.

| Field | |
|---|---|
| `id` | required. Your id for the posting, from 1 to 2^128-2, unique within the request. Use time-ordered ids such as UUIDv7 read as a number: random ids slow TigerBeetle down |
| `source`, `destination` | account addresses, which must differ |
| `currency` | a registered currency name |
| `amount` | greater than zero, in minor units |
| `group_id` | optional. Links related postings, such as a payment and its refund. Defaults to `id` |

Each element of `postings` is either a single posting or an array of postings. An array is a chain: either all of it is applied, or none of it is. Separate elements are independent of each other.

```json
{"postings": [
  {"id": "1001", "source": "world", "destination": "rider:123:wallet", "currency": "USD", "amount": "10000"},
  [
    {"id": "1002", "source": "rider:123:wallet", "destination": "merchant:9", "currency": "USD", "amount": "2500"},
    {"id": "1003", "source": "rider:123:wallet", "destination": "platform:fees", "currency": "USD", "amount": "100"}
  ]
]}
```

Returns 200 with one result per posting, in the same shape as the request:

```json
{"transfers": [
  {"id": "1001", "group_id": "1001", "status": "created"},
  [
    {"id": "1002", "group_id": "1002", "status": "failed", "error": "insufficient_funds", "message": "..."},
    {"id": "1003", "group_id": "1003", "status": "failed", "error": "chain_failed", "message": "..."}
  ]
]}
```

| `status` | `error` | Meaning | What to do |
|---|---|---|---|
| `created` | | applied now | nothing |
| `exists` | | already applied by an earlier request | nothing |
| `failed` | `insufficient_funds` | the source can't go that low. Nothing in the chain was applied | top up, and resend this posting **with a new id** |
| `failed` | `chain_failed` | another posting in the chain failed | fix that one; this id can be reused |
| `failed` | `conflict` | the id was already used with different fields | use a new id |
| `failed` | `id_already_failed` | the id belongs to a posting that failed before | use a new id |
| `failed` | `internal` | unexpected error, logged on the server | resend with the same id |

The whole request fails, and nothing is applied, only for:

| Status | When |
|---|---|
| 400 | invalid input, a duplicate id in the request, or more than 8189 postings |
| 404 | an account or currency is not registered |
| 500 | an error before or while sending to TigerBeetle. The outcome is unknown, so resend the same request with the same ids |

**Retries are safe.** Resending a posting with the same id and fields never applies it twice; it returns `exists`.

**Batch when you have volume.** One request can carry thousands of independent postings and chains, and that is much faster than one request per posting. Send whatever has queued since your last request returned, and never wait to fill a batch. See [decisions.md](decisions.md#batching).

### `GET /groups/{group_id}/transfers`
Returns 200 with the group's transfers, newest first, up to 8189. An unknown group returns an empty list. A malformed id returns 400.

```json
{
  "group_id": "1001",
  "transfers": [
    {"id": "1004", "group_id": "1001", "source": "rider:123:wallet", "destination": "world", "currency": "USD", "amount": "3000", "timestamp": "2026-11-06T10:05:00.123456789Z"},
    {"id": "1001", "group_id": "1001", "source": "world", "destination": "rider:123:wallet", "currency": "USD", "amount": "10000", "timestamp": "2026-11-06T10:00:00.123456789Z"}
  ]
}
```

### `GET /accounts/{address}/transfers`
An account's transfer history, newest first.

Query parameters:
- `currency`: only this currency.
- `from`: the earliest time, inclusive.
- `before`: the latest time, exclusive.
- `limit`: the page size.

Returns 200, or 404 for an unknown address. `next` is the last transfer's timestamp, to pass back as `before` for the next page.

```json
{"address": "rider:123:wallet", "transfers": [ ... ], "next": "2026-11-06T10:00:00.123456789Z"}
```

For a pattern, it merges the history of every matching account, and a transfer between two of them appears once. A pattern matching more than 100 wallets returns 400.
