---
title: 2. Deposits
description: Money arriving, and making sure a payment is never counted twice.
---

User 42 sends $1,000 by bank transfer. The money lands in the brokerage's bank account, and the brokerage now owes it to the user.

## The posting

One posting records both sides. It debits `bank:client-money`, an asset, and credits `user:42:cash`, a liability.

```json title="POST /transfers"
{
  "chains": [
    [
      {
        "id": "1001",
        "debit": "bank:client-money",
        "credit": "user:42:cash",
        "currency": "USD",
        "amount": "100000"
      }
    ]
  ]
}
```

```json title="200 OK"
{ "chains": [[{ "id": "1001", "group_id": "1001", "status": "created" }]] }
```

`chains` is a list of chains. A chain is a list of postings that all happen or none do. This deposit is one chain with one posting in it. The response has the same shape.

Both balances go up, as [chapter 1](/brokerage/chart-of-accounts/#which-way-balances-move) said they would:

| Account | Type | Side | Change | Why |
|---|---|---|---|---|
| `bank:client-money` | asset | debit | +$1,000 | the bank holds $1,000 more |
| `user:42:cash` | liability | credit | +$1,000 | the brokerage owes the user $1,000 more |

## Counting a payment once

Payment providers tell you about a deposit with a webhook, and they often send the same webhook more than once. If each delivery added $1,000, the user would get free money.

The posting `id` prevents that. Send the same posting again, and Torment recognises it:

```json title="200 OK"
{ "chains": [[{ "id": "1001", "group_id": "1001", "status": "exists" }]] }
```

`exists` means "already applied, nothing changed". It's a success, not an error.

The trick is to choose the id **before** you send anything, and store it with the payment:

1. When the provider first tells you about a payment, create a row for it in your own database with a new posting id.
2. Send the posting with that id.
3. On every later webhook for the same payment, look up the row and send the same id again.

Provider ids like `pay_8f3k2` can't be used directly, because Torment ids are numbers. That's why you keep your own.

## Choosing ids

Ids are numbers from 1 to 2¹²⁸−2. This tutorial uses short ones like `1001` so they're easy to read. In production, use ids that increase over time, such as a UUIDv7 read as a 128-bit number. Random ids work, but they slow TigerBeetle down.

## When the request itself fails

If the request times out or returns a 500, you don't know whether the deposit was applied. Send the exact same request again. Each posting comes back `created` (it wasn't applied before) or `exists` (it was). Either way, it's applied exactly once.

## Where this leaves us

| Account | USD |
|---|---|
| `bank:client-money` | 1,000.00 |
| `user:42:cash` | 1,000.00 |

Next, the user spends it.
