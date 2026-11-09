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

A chain is one array of postings inside `chains`. All three postings go in the same array. They share a `group_id`: the brokerage's order id, so the order's postings can be found later.

```json title="POST /transfers"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000002001", "debit": "user:42:cash", "credit": "bank:client-money", "currency": "USD", "amount": "22500", "group_id": "00000000-0000-0000-0000-000000002000" },
      { "id": "00000000-0000-0000-0000-000000002002", "debit": "custodian:positions", "credit": "user:42:holdings", "currency": "AAPL", "amount": "1500000", "group_id": "00000000-0000-0000-0000-000000002000" },
      { "id": "00000000-0000-0000-0000-000000002003", "debit": "user:42:cash", "credit": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "00000000-0000-0000-0000-000000002000" }
    ]
  ]
}
```

```json title="200 OK"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000002001", "group_id": "00000000-0000-0000-0000-000000002000", "status": "created" },
      { "id": "00000000-0000-0000-0000-000000002002", "group_id": "00000000-0000-0000-0000-000000002000", "status": "created" },
      { "id": "00000000-0000-0000-0000-000000002003", "group_id": "00000000-0000-0000-0000-000000002000", "status": "created" }
    ]
  ]
}
```

The response has the same shape as the request: a chain in, a chain of results out.

## What each posting did

| Posting | Account | Side | Change | Why |
|---|---|---|---|---|
| `…2001` | `user:42:cash` | debit | −$225 | the user paid |
| | `bank:client-money` | credit | −$225 | the money left to pay the seller |
| `…2002` | `custodian:positions` | debit | +1.5 AAPL | the custodian now holds the shares for the brokerage |
| | `user:42:holdings` | credit | +1.5 AAPL | the brokerage owes the user those shares |
| `…2003` | `user:42:cash` | debit | −$1 | the user paid the fee |
| | `revenue:commissions` | credit | +$1 | the brokerage earned it |

Check each row against [chapter 1](/brokerage/chart-of-accounts/#which-way-balances-move). A debit lowers a liability like `user:42:cash` and raises an asset like `custodian:positions`. A credit raises a liability or revenue and lowers an asset like `bank:client-money`.

`user:42:holdings` had no AAPL wallet before. Torment created it when the first AAPL posting touched the account.

## When the user can't afford it

Now the user tries to buy 10 Tesla shares at $200, for $2,000. They have $774.

```json title="POST /transfers"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000002011", "debit": "user:42:cash", "credit": "bank:client-money", "currency": "USD", "amount": "200000", "group_id": "00000000-0000-0000-0000-000000002010" },
      { "id": "00000000-0000-0000-0000-000000002012", "debit": "custodian:positions", "credit": "user:42:holdings", "currency": "TSLA", "amount": "10000000", "group_id": "00000000-0000-0000-0000-000000002010" },
      { "id": "00000000-0000-0000-0000-000000002013", "debit": "user:42:cash", "credit": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "00000000-0000-0000-0000-000000002010" }
    ]
  ]
}
```

```json title="200 OK"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000002011", "group_id": "00000000-0000-0000-0000-000000002010", "status": "failed", "error": "insufficient_funds", "message": "insufficient funds: 200000 USD debiting \"user:42:cash\", crediting \"bank:client-money\"" },
      { "id": "00000000-0000-0000-0000-000000002012", "group_id": "00000000-0000-0000-0000-000000002010", "status": "failed", "error": "chain_failed", "message": "chain failed: transfer 00000000-0000-0000-0000-000000002011 in the same chain failed" },
      { "id": "00000000-0000-0000-0000-000000002013", "group_id": "00000000-0000-0000-0000-000000002010", "status": "failed", "error": "chain_failed", "message": "chain failed: transfer 00000000-0000-0000-0000-000000002011 in the same chain failed" }
    ]
  ]
}
```

Nothing moved. The posting that caused the failure says `insufficient_funds`, and the rest of the chain says `chain_failed`. The shares posting would have worked on its own, but the chain stopped it.

The status is still `200`: the request worked, and the order didn't. To retry after a deposit, send `00000000-0000-0000-0000-000000002011` with a **new id**, because a failed id can't be used again (`id_already_failed`). The other two ids never applied, so they can be reused.

## Selling

Selling is the same chain the other way round: each account that was debited is now credited, and the other way round. The fee is still a debit to the user's cash and a credit to revenue. The user sells 0.5 AAPL at $160, so $80, minus the $1 fee:

```json title="POST /transfers"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000002021", "debit": "user:42:holdings", "credit": "custodian:positions", "currency": "AAPL", "amount": "500000", "group_id": "00000000-0000-0000-0000-000000002020" },
      { "id": "00000000-0000-0000-0000-000000002022", "debit": "bank:client-money", "credit": "user:42:cash", "currency": "USD", "amount": "8000", "group_id": "00000000-0000-0000-0000-000000002020" },
      { "id": "00000000-0000-0000-0000-000000002023", "debit": "user:42:cash", "credit": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "00000000-0000-0000-0000-000000002020" }
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
