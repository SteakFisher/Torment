# Decisions

Postgres maps names to TigerBeetle ids, and TigerBeetle holds balances and transfers. Postgres is always written first. Every write is safe to retry.

## Currencies
`POST /currencies {code}`: one Postgres insert, with Postgres assigning the ledger number. One currency is one TigerBeetle ledger.

| Fails at | State | Recovery |
|---|---|---|
| insert | nothing written | retry |
| response lost | currency exists | a retry gets 409, which means it already exists |

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
- **Costs nothing when wallets are already confirmed:** one query, no TigerBeetle call.
- **Every wallet has TigerBeetle's `history` flag**, so past balances can be queried. TigerBeetle 0.17.9 already stores a balance snapshot for every transfer (for CDC); the flag indexes it by account and keeps it from future pruning, about one small index entry per transfer. Flags can't change after creation, so this was decided before any real data.

## Balances
A read only: Postgres wallets, then TigerBeetle `lookup_accounts`. A wallet with no TigerBeetle account reads as zero. With `?at=`, each wallet takes one `get_account_balances` call for its last snapshot at or before `at`, using TigerBeetle's clock.

## Transfers
Each posting has a client-chosen `id`, used as the TigerBeetle transfer id. A posting on its own is independent. An array of postings is an atomic chain. The response gives a status for each posting.

| Step | Fails → state | Recovery |
|---|---|---|
| 1. validate | nothing written | fix the input |
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
- **Groups:** `group_id` is stored in `user_data_128`. It defaults to the posting's own id, which keeps retries consistent.

## Groups
`GET /groups/{group_id}/transfers`: a read only. TigerBeetle `query_transfers` on `user_data_128`, then one Postgres query maps the account ids to addresses. Oldest first.

- **An unknown group returns 200 with an empty list.** TigerBeetle keeps no list of groups, so an unused id and an empty group look the same.
- **At most 8189 transfers**, the most one TigerBeetle query returns.
- **A TigerBeetle account with no wallet row returns 500.** Every TigerBeetle account is created from a Postgres row first, so this means the data is broken.

## Queries
- **Patterns are capped at 1000 wallets**; more returns 400 asking to narrow the pattern. The ledger answers operational queries (one user's pots, a few system accounts). Totals across all users are analytics: export through TigerBeetle CDC to something like ClickHouse. That export is not built.
- **Patterns:** `*` matches exactly one segment. In Postgres, a pattern becomes an anchored regex.
- **`at` reads a past balance**: the last TigerBeetle snapshot at or before `at`. A wallet with none (created later, or no transfers yet) reads as zero and is still listed, not hidden.
- **Pattern balances return the totals and each account's balance**, one total per (type, currency). Different types are never summed together. Both come from the same TigerBeetle read, so they always agree.

## Shutdown
SIGINT or SIGTERM stops new connections and waits up to 10 seconds for requests in flight. A request cut off after that is the crash row under Transfers: retry with the same ids.
