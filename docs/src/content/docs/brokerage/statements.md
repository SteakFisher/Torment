---
title: 8. Statements
description: A user's history, their balance on any past date, and checking the books.
---

At the end of the month, user 42 needs a statement: everything that happened in their accounts, and their balances at the start and end of the month. User 42 has two accounts, `user:42:cash` and `user:42:holdings`, and a pattern reads both at once.

## Everything at once

`user:42:*` means every account whose address is `user:42:` followed by one more segment.

```http
GET /accounts/user:42:*/balances
```

```json title="200 OK"
{
  "address": "user:42:*",
  "totals": [
    { "type": "liability", "currency": "AAPL", "precision": 6, "amount": "1000000" },
    { "type": "liability", "currency": "AED", "precision": 2, "amount": "63275" },
    { "type": "liability", "currency": "TSLA", "precision": 6, "amount": "3000000" },
    { "type": "liability", "currency": "USD", "precision": 2, "amount": "25325" }
  ],
  "accounts": [
    {
      "address": "user:42:cash",
      "type": "liability",
      "balances": [
        { "currency": "AED", "precision": 2, "amount": "63275" },
        { "currency": "USD", "precision": 2, "amount": "25325" }
      ]
    },
    {
      "address": "user:42:holdings",
      "type": "liability",
      "balances": [
        { "currency": "AAPL", "precision": 6, "amount": "1000000" },
        { "currency": "TSLA", "precision": 6, "amount": "3000000" }
      ]
    }
  ]
}
```

`totals` adds up every matching account, one total per type and currency. Different types are never added together, because adding what the brokerage owns to what it owes would mean nothing.

## The history

```http
GET /accounts/user:42:*/transfers?from=2026-11-01T00:00:00Z&cursor=2026-12-01T00:00:00Z&limit=100
```

```json title="200 OK"
{
  "address": "user:42:*",
  "transfers": [
    { "id": "00000000-0000-0000-0000-000000007012", "group_id": "00000000-0000-0000-0000-000000007010", "debit": "transit:withdrawals", "credit": "user:42:cash", "currency": "USD", "amount": "5000", "kind": "withdrawal", "timestamp": "2026-11-06T12:41:09.6Z" },
    ...
  ],
  "next": "2026-11-06T12:41:09.21Z"
}
```

- **Newest first.**
- **Every currency,** unless you add `?currency=USD`.
- **Every kind,** unless you add `?kind=`, as below.
- **Once each.** A transfer between two of the user's own accounts would match twice, but is listed once.
- **Paged.** A page holds the transfers before `cursor`, so the first request sets it to the end of the month. When `next` is there, pass it as `cursor` to get the next page. When it's missing, you've reached the start of the range.

A pattern can cover at most 100 wallets in a history query. Each wallet is one query to TigerBeetle, so this is meant for one user's pots, not a feed across many users.

## Just the dividends

For a tax statement, the user needs their dividends on their own. Filter the history by kind:

```http
GET /accounts/user:42:*/transfers?kind=dividend
```

```json title="200 OK"
{
  "address": "user:42:*",
  "transfers": [
    { "id": "00000000-0000-0000-0000-000000006001", "group_id": "00000000-0000-0000-0000-000000006000", "debit": "bank:client-money", "credit": "user:42:cash", "currency": "USD", "amount": "150", "kind": "dividend", "timestamp": "2026-11-06T12:41:09.4Z" }
  ]
}
```

The filter runs inside TigerBeetle, so a rare kind is quick to find even in a long history. Paging works the same way.

## Balances on a past date

Opening and closing balances come from `at`: the balance just after the last transfer at or before that moment.

```http
GET /accounts/user:42:*/balances?at=2026-11-01T00:00:00Z
```

```http
GET /accounts/user:42:*/balances?at=2026-12-01T00:00:00Z
```

A wallet that had no transfers by then reads as zero, and is still listed. Times are TigerBeetle's commit times, the same `timestamp` the history shows, so balances and history always agree.

## Checking the books

Double-entry has a built-in check: what the brokerage holds must equal what it owes plus what it earned. For dollars:

> `bank:client-money` = every user's cash + money in transit + commissions + the FX position

The brokerage's own accounts all have two-part addresses, so `*:*` reads them in one call:

```http
GET /accounts/*:*/balances?currency=USD
```

The users' side is `GET /accounts/user:*:cash/balances?currency=USD`. That works for small numbers of users, but a pattern balance stops at 1,000 wallets and returns 400 beyond that. Checking every user's balance belongs in an analytics database fed from TigerBeetle, which Torment doesn't provide yet.

## Where this leaves us

The user has a statement, and the brokerage can check its books. Last, what to do when something was recorded wrong.
