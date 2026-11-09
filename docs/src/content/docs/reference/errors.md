---
title: Errors and retries
description: Every way a request can fail, and what to do about each.
---

There are two kinds of failure. A **request** fails when the whole thing is rejected or its outcome is unknown. A **posting** fails when the request worked but the ledger refused that posting.

## Request failures

The response has this shape:

```json
{"error": {"code": "currency_not_found", "message": "not found: currency \"EUR\" is not registered"}}
```

Match on `code`, which is stable. `message` explains the problem to a person and may change.

| Status | `code` | Meaning | What to do |
|---|---|---|---|
| 400 | `invalid_request` | malformed JSON, an unknown field or an invalid value | fix the request. Retrying won't help |
| 400 | `pattern_too_broad` | an address pattern matches more wallets than allowed (1000 for balances, 100 for transfers) | narrow the pattern |
| 401 | `unauthorized` | missing or wrong API key | fix the `Authorization` header |
| 404 | `account_not_found` | no account has the address | register the account, or fix the address |
| 404 | `currency_not_found` | the currency is not registered | register the currency, or fix the name |
| 409 | `conflict` | the input clashes with something already stored, such as an account registered with a different type | use the stored values, or a new name |
| 500 | `internal` | something failed inside Torment, and it was logged. The message is always `internal error` | retry the same request |
| 503 | `not_ready` | from `GET /readyz` only: Postgres or TigerBeetle didn't answer | wait, and check again |

When a request names both a missing account and a missing currency, either 404 code may come back.

For `POST /transfers`, a 400 or 404 means nothing was applied. A 500, a timeout or a lost connection means you don't know: some postings may have been applied. Resend the **same request with the same ids**. Each posting then comes back `created` or `exists`, so nothing is applied twice.

## Posting failures

`POST /transfers` returns 200 with a result for every posting, nested in the same chains as the request, even when some fail:

| `error` | Meaning | What to do |
|---|---|---|
| `insufficient_funds` | the debit or the credit would take an account below zero, and it wasn't registered with `allow_negative`. Nothing in its chain was applied | top up first, then resend this posting **with a new id** |
| `chain_failed` | another posting in the same chain failed, so this one wasn't applied | fix the other posting. This id can be reused |
| `conflict` | the id was already used for a posting with different fields | use a new id. If you meant to correct a posting, see [Mistakes](/brokerage/mistakes/) |
| `id_already_failed` | the id belongs to a posting that failed before | use a new id |
| `overflow` | an account's total debits or credits would pass 2¹²⁸−1 minor units, the most TigerBeetle stores. Nothing in its chain was applied | use a smaller amount and a new id. Real amounts never get near this; it means the amount is wrong |
| `internal` | an unexpected error, which was logged | resend with the same id |

## Retrying safely

Every write in Torment is safe to retry when it's identical:

- **Currencies:** the same code returns 200 instead of 201.
- **Accounts:** the same fields return 200 instead of 201, and finish creating any missing wallets.
- **Postings:** the same id and fields return `exists`.

So the rule for clients is: on any failure where you don't know the outcome, send exactly the same request again. Never change the ids of a retry, and never reuse an id for a different posting.

## Why a failed id can't be reused

TigerBeetle remembers every id that failed with `insufficient_funds`, and rejects it forever after. Without that, a posting that failed for lack of money could suddenly succeed on a retry hours later, after a deposit, which is rarely what anyone intended. A new attempt gets a new id.
