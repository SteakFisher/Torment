---
title: Decisions
---

Postgres maps names to TigerBeetle ids, and TigerBeetle holds balances and transfers. Postgres is always written first. Every write is safe to retry.

## Currencies
`POST /currencies {code}`: one Postgres insert, with Postgres assigning the ledger number. One currency is one TigerBeetle ledger.

| Fails at | State | Recovery |
|---|---|---|
| insert | nothing written | retry |
| response lost | currency exists | retry: the same code returns 200 |

Registering the same name with a different precision returns 409.

## Kinds
`POST /kinds {name}`: one Postgres insert, with Postgres assigning the code. One kind is one TigerBeetle transfer `code`, so kinds are numbered 1 to 65,535 and the 65,536th fails. A replay doesn't take a code. Every posting must name a registered kind.

- **Cached for good:** a kind never changes, so each instance keeps every kind it has read in memory, by name and by code. A warm transfer looks up no kind in Postgres. An unregistered name is read from Postgres each time, since it may be registered later.
- **Filtering is TigerBeetle's:** `?kind=` sets `code` on the TigerBeetle query, so TigerBeetle uses its own index and doesn't scan an account's whole history for a rare kind. Paging works the same as without it.
- **A transfer whose code has no kinds row returns 500**, like a TigerBeetle account with no wallet: the data is broken.
- **No totals by kind.** "How much commission this month" is analytics: export through TigerBeetle CDC to something like ClickHouse.

## Accounts
`POST /accounts`: the account is an address, a type and `allow_negative`. Accounts are never created implicitly.

| Step | Fails → state | Recovery |
|---|---|---|
| 1. check the currencies exist | nothing written | fix the input, retry |
| 2. insert the account | nothing written | retry |
| 3. ensure wallets (below) | the account exists without some of its wallets | retry: the same fields return 200 and finish the wallets |

Registering again with different fields returns 409.

## Wallets
One per (account, currency), and each is one TigerBeetle account. The type goes in `code`. The flags stop the balance going below zero, unless the account has `allow_negative`. Wallets are created by registration or lazily by a transfer, through the same steps:

| Step | Fails → state | Recovery |
|---|---|---|
| 1. look up the wallets | nothing written | retry |
| 2. insert missing wallets with ids from `tb.ID()` | nothing written (one statement) | retry |
| 3. create the TigerBeetle accounts for wallets with `tb_created = false` | Postgres rows exist, and TigerBeetle has some, all or none of the accounts | the next transfer or registration repeats step 3. `exists` counts as success |
| 4. set `tb_created = true` | the TigerBeetle accounts exist but the flag is still false | the next use repeats step 3 (`exists`), then step 4 |

- **Why the flag exists:** a transfer to a wallet with no TigerBeetle account fails with `account_not_found`, and TigerBeetle then permanently rejects that transfer id. So step 3 must succeed before any transfer is sent.
- **Two requests creating the same wallet:** `unique(account_id, currency)` means the first insert wins, and both requests read back its id.
- **Costs nothing when wallets are already confirmed:** no query, no TigerBeetle call. Confirmed wallets are kept in memory (`cache.go`); an unconfirmed one is read from Postgres every time until it is confirmed.
- **Why the cache is safe:** a confirmed wallet never changes, and neither does the address and currency behind a TigerBeetle id. So nothing goes stale, nothing needs invalidating, and each instance fills its own. Benchmarked on 8 cores, the wallet lookup was half of all CPU at about 7k requests/s.
- **Not cached:** "which wallets does this address or pattern have" (balances, history, account lists). New wallets appear, so only a complete copy of the table could answer it, and none has been needed yet.
- **Every wallet has TigerBeetle's `history` flag**, so past balances can be queried. TigerBeetle 0.17.9 already stores a balance snapshot for every transfer (for CDC); the flag indexes it by account and keeps it from future pruning, about one small index entry per transfer. Flags can't change after creation, so this was decided before any real data.

## Balances
`GET /accounts/{address}/balances`, optionally one `?currency=`. A read only: Postgres wallets, then TigerBeetle `lookup_accounts`. A wallet with no TigerBeetle account reads as zero. With `?at=`, each wallet takes one `get_account_balances` call for its last snapshot at or before `at`, using TigerBeetle's clock.

## Transfers
Each posting has a client-chosen `id`, used as the TigerBeetle transfer id, and becomes one TigerBeetle transfer: its `debit` account's wallet is the transfer's debit account and its `credit` account's wallet the credit account. The request is a list of chains, and each chain is atomic: TigerBeetle links every posting but the last to the next. A posting on its own is a chain of one. The response gives a status for each posting, nested in the same chains.

| Step | Fails → state | Recovery |
|---|---|---|
| 1. validate, and look up the kinds | nothing written | fix the input |
| 2. ensure wallets | wallets as in the table above, no transfer sent | retry with the same ids |
| 3. TigerBeetle `create_transfers` errors, or the process crashes | unknown: the transfers may or may not exist | retry with the same ids: each one is `created` or `exists`, never applied twice |
| 4. response lost | transfers exist | retry with the same ids → `exists` |

When TigerBeetle rejects postings, the result is reported for each one:

| Result | Meaning | Recovery |
|---|---|---|
| `insufficient_funds` | that chain applied nothing. The rest of its chain is `chain_failed` | top up. **Use a new id for the posting that failed**: TigerBeetle permanently rejects the old one (`id_already_failed`). The others' ids can be reused |
| `conflict` | the id already exists with different fields | use a new id |
| `exists` | already applied | none |

- **A chain that fails on `exists`:** TigerBeetle stops a chain at the first `exists`. That makes a resent chain look the same as a new chain reusing an old id, so we look up the chain's ids. If every one exists with the same fields, the result is `exists`, otherwise `conflict`.
- **Other chains:** each one is applied separately, so failures are reported per posting and never fail the whole request.
- **Kinds:** the kind's number is the transfer's `code`. Resending an id with another kind is `conflict`.
- **Groups:** `group_id` is stored in `user_data_128`. It defaults to the posting's own id, which keeps retries consistent.

## Batching
Most of a write's cost is the HTTP request around it, not TigerBeetle. Benchmarked on 8 cores with the wallet cache, with every write touching the same few accounts: 7,081 applied writes/s with one write per request, and 35,995 with 100 per request ([Performance](/internals/performance/)). One request can carry many independent postings and chains, each with its own result, so callers with volume should batch.

- **Torment already batches toward TigerBeetle.** Concurrent `create_transfers` calls on the write client merge into one TigerBeetle request, so Torment has no batcher of its own. What callers save by batching is the per-request work: HTTP, JSON, auth and building results.
- **Which writes to batch:**

  | Writer | Batch? |
  |---|---|
  | Order fills | yes, whatever has queued since the last send |
  | Deposits and withdrawals a user is waiting on | not needed: one request each, merged toward TigerBeetle anyway |
  | Dividends, fees, interest, end-of-day jobs | yes, thousands of chains per request |
  | Imports, replays, reconciliation | yes, as large as the limits allow |

- **Batch whatever is waiting; never wait for more.** When a request returns, send everything that queued while it was in flight as the next request. Never hold writes on a timer to fill a batch. A quiet caller then sends batches of one with the lowest latency, and under load batches grow on their own. A timer adds latency when traffic is quiet and does nothing under load.
- **Limits:** at most 8189 postings per request, which is TigerBeetle's batch size on a production cluster (a `--development` cluster takes about 250, and going over the cluster's limit returns 400 `batch_too_large`), and a 1 MiB body, roughly 5,000 postings of JSON. The body cap is the tighter one and can be raised when a caller needs it.
- **A failed request fails every chain in it:** a 500, a timeout or a lost connection. The outcome is unknown, so resend the same chains with the same ids: each posting comes back `created` or `exists`, never applied twice. Per-posting failures such as `insufficient_funds` affect only that posting's chain.

## Groups
`GET /groups/{group_id}/transfers`: a read only. TigerBeetle `query_transfers` on `user_data_128`, then one Postgres query maps the account ids to addresses. Newest first, like account transfers.

- **An unknown group returns 200 with an empty list.** TigerBeetle keeps no list of groups, so an unused id and an empty group look the same.
- **Paged by timestamp.** Each page is one query for the transfers committed before the cursor, so every page costs the same however large the group. TigerBeetle gives every transfer a unique timestamp, so no transfer is skipped or repeated between pages.
- **A TigerBeetle account with no wallet row returns 500.** Every TigerBeetle account is created from a Postgres row first, so this means the data is broken.

## Queries
- **Patterns go in the path:** `GET /accounts/{address}`, `GET /accounts/{address}/balances` and `GET /accounts/{address}/transfers` treat an address containing `*` as a pattern, since a valid address never contains `*`. Each route returns one shape: an exact address is a list of one account, or totals and balances for one account, so clients parse a single response either way. `GET /accounts` lists every account.
- **Pattern transfers are capped at 100 wallets**, not 1000: each page costs one TigerBeetle request per wallet. The use is one user's pots in one timeline (`rider:123:*`); a feed across many users is an analytics or export query. A transfer between two matching accounts is returned once.
- **Pattern balances are capped at 1000 wallets**; more returns 400 asking to narrow the pattern. The ledger answers operational queries (one user's pots, a few system accounts). Totals across all users are analytics: export through TigerBeetle CDC to something like ClickHouse. That export is not built.
- **Patterns:** `*` matches exactly one segment. In Postgres, a pattern becomes an anchored regex.
- **`at` reads a past balance**: the last TigerBeetle snapshot at or before `at`. A wallet with none (created later, or no transfers yet) reads as zero and is still listed, not hidden.
- **Pattern balances return the totals and each account's balance**, one total per (type, currency). Different types are never summed together. Both come from the same TigerBeetle read, so they always agree.

## TigerBeetle clients
- **Writes and reads use separate clients.** A client has at most one request in flight, and calls of different kinds queue one after another. On a shared client, every slow read delays the writes queued behind it. The compiler enforces the split: `tbWriter` has only the create calls and the transfer lookup that settles a failed chain, and `tbReader` has the rest.
- **Two sessions per instance.** A cluster allows 64 client sessions by default, so up to 32 Torment instances.
- **Reads that query one account at a time send their queries at once** (history, balances at a time). TigerBeetle's API takes one account per history or balance query, but the client merges concurrent calls of the same kind into one request. Measured locally for 21 wallets: 75 ms one after another, 7 ms at once.

## Shutdown
SIGINT or SIGTERM stops new connections and waits up to 10 seconds for requests in flight. A request cut off after that is the crash row under Transfers: retry with the same ids.

## Auth
Torment is an internal service, so every route except `GET /healthz` and `GET /readyz` takes a shared API key as `Authorization: Bearer <key>`; a missing or wrong key is 401. Keys come from `API_KEYS` (comma-separated) and the server refuses to start without one; several keys let a key be rotated without downtime. The key travels in clear over HTTP, so traffic must stay on a private network or go through TLS in front of Torment. No per-key permissions and no keys in Postgres until a caller needs them.
