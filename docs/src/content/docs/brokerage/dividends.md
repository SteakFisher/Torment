---
title: 6. Dividends
description: Paying thousands of users in a few requests.
---

Tesla pays a dividend of $0.50 per share. The custodian pays the total into the brokerage's bank account, and the brokerage has to credit every user who held Tesla: user 42 has 3 shares and gets $1.50, and thousands of other users get their share too.

## One request, many payments

Each payment is one posting from `bank:client-money` to the user's cash. They aren't in a chain: if one user's payment fails, the others should still go through.

```json title="POST /transfers"
{
  "postings": [
    { "id": "6001", "source": "bank:client-money", "destination": "user:42:cash", "currency": "USD", "amount": "150", "group_id": "6000" },
    { "id": "6002", "source": "bank:client-money", "destination": "user:43:cash", "currency": "USD", "amount": "1200", "group_id": "6000" },
    { "id": "6003", "source": "bank:client-money", "destination": "user:44:cash", "currency": "USD", "amount": "25", "group_id": "6000" }
  ]
}
```

```json title="200 OK"
{
  "transfers": [
    { "id": "6001", "group_id": "6000", "status": "created" },
    { "id": "6002", "group_id": "6000", "status": "created" },
    { "id": "6003", "group_id": "6000", "status": "created" }
  ]
}
```

The response has one result per posting, in the same order, so you can tell exactly which payments went through.

## Why batch

Most of the cost of a write is the HTTP request around it, not the ledger. In our benchmark, sending 100 writes per request was 5 to 25 times faster than sending them one at a time, depending on the workload; see [Performance](/internals/performance/).

For a job like this:

- **Put up to about 4,000 postings in a request.** The limits are 8,189 postings and a 1 MiB body, and the body is usually reached first, at about 5,000 postings.
- **Send several requests at once.** Torment merges them on the way to TigerBeetle.
- **Store the ids before sending.** Write each user's payment and its posting id to your own database first. If the job crashes halfway, rerun it with the same ids: payments already made come back `exists`, and the rest are made.

## Batching everyday writes too

The same idea works for live traffic, such as order fills from the market. Keep a queue, and whenever a request returns, send everything that arrived in the meantime as the next request. Never wait on a timer to fill a batch: when it's quiet you send batches of one with no delay, and when it's busy batches grow on their own. See [Batching](/internals/decisions/#batching).

## Finding the payments again

`group_id` is the dividend's id, so `GET /groups/6000/transfers` lists the payments. A group is read a page at a time, so for a dividend paid to thousands of users, follow `next` until it is gone to see every payment.

## Where this leaves us

| Account | USD |
|---|---|
| `user:42:cash` | 353.25 |
