---
title: 3. Buying shares
description: Paying, receiving shares and charging a fee, all or nothing.
---

User 42 buys 1.5 shares of Apple at $150, so $225.00, and the brokerage charges a $1 commission. That is three movements:

1. The user pays $225 for the shares.
2. The user receives 1.5 AAPL.
3. The user pays the $1 fee.

They have to happen together. If the user could pay without getting the shares, or get the shares without paying, the books would be wrong. A **chain** guarantees it: either every posting in it happens, or none do.

## The chain

A chain is an array of postings inside `postings`. All three share a `group_id`: the brokerage's order id, so the order's postings can be found later.

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "2001", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "22500", "group_id": "2000" },
      { "id": "2002", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "AAPL", "amount": "1500000", "group_id": "2000" },
      { "id": "2003", "source": "user:42:cash", "destination": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "2000" }
    ]
  ]
}
```

```json title="200 OK"
{
  "transfers": [
    [
      { "id": "2001", "group_id": "2000", "status": "created" },
      { "id": "2002", "group_id": "2000", "status": "created" },
      { "id": "2003", "group_id": "2000", "status": "created" }
    ]
  ]
}
```

The response has the same shape as the request: a chain in, a chain of results out.

## What each posting did

| Posting | Account | Change | Why |
|---|---|---|---|
| 2001 | `user:42:cash` | −$225 | the user paid |
| | `bank:client-money` | −$225 | the money left to pay the seller |
| 2002 | `custodian:positions` | +1.5 AAPL | the custodian now holds the shares for the brokerage |
| | `user:42:holdings` | +1.5 AAPL | the brokerage owes the user those shares |
| 2003 | `user:42:cash` | −$1 | the user paid the fee |
| | `revenue:commissions` | +$1 | the brokerage earned it |

`user:42:holdings` had no AAPL wallet before. Torment created it when the first AAPL posting touched the account.

## When the user can't afford it

Now the user tries to buy 10 Tesla shares at $200, for $2,000. They have $774.

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "2011", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "200000", "group_id": "2010" },
      { "id": "2012", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "TSLA", "amount": "10000000", "group_id": "2010" },
      { "id": "2013", "source": "user:42:cash", "destination": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "2010" }
    ]
  ]
}
```

```json title="200 OK"
{
  "transfers": [
    [
      { "id": "2011", "group_id": "2010", "status": "failed", "error": "insufficient_funds", "message": "insufficient funds: 200000 USD from \"user:42:cash\" to \"bank:client-money\"" },
      { "id": "2012", "group_id": "2010", "status": "failed", "error": "chain_failed", "message": "chain failed: transfer 2011 in the same chain failed" },
      { "id": "2013", "group_id": "2010", "status": "failed", "error": "chain_failed", "message": "chain failed: transfer 2011 in the same chain failed" }
    ]
  ]
}
```

Nothing moved. The posting that caused the failure says `insufficient_funds`, and the rest of the chain says `chain_failed`. The shares posting would have worked on its own, but the chain stopped it.

The status is still `200`: the request worked, and the order didn't. To retry after a deposit, send `2011` with a **new id**, because a failed id can't be used again (`id_already_failed`). The other two ids never applied, so they can be reused.

## Selling

Selling is the same chain the other way round. The user sells 0.5 AAPL at $160, so $80, minus the $1 fee:

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "2021", "source": "user:42:holdings", "destination": "custodian:positions", "currency": "AAPL", "amount": "500000", "group_id": "2020" },
      { "id": "2022", "source": "bank:client-money", "destination": "user:42:cash", "currency": "USD", "amount": "8000", "group_id": "2020" },
      { "id": "2023", "source": "user:42:cash", "destination": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "2020" }
    ]
  ]
}
```

A user can't sell shares they don't have, for the same reason they can't spend money they don't have: `user:42:holdings` can't go below zero.

## Where this leaves us

| Account | USD | AAPL |
|---|---|---|
| `user:42:cash` | 853.00 | |
| `user:42:holdings` | | 1.0 |
| `revenue:commissions` | 2.00 | |
