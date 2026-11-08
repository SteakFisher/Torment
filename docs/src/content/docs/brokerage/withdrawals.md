---
title: 7. Withdrawals
description: Money that has left the user's wallet but not yet left the bank.
---

User 42 asks to withdraw $100. The brokerage tells its bank to pay it out, and the bank takes a day or two. During that time, the money is in between: the user can't spend it any more, but it hasn't left the brokerage's bank account yet. And the payout might fail, for example because of a wrong account number.

## An account for money in transit

Register an account for withdrawals that have been requested but not paid out:

```json title="POST /accounts"
{ "address": "transit:withdrawals", "type": "liability" }
```

It's a liability because the brokerage still owes this money to someone: the user, until the bank pays them.

## Step 1: the user asks

Move the money out of the user's cash straight away, so they can't spend it twice:

```json title="POST /transfers"
{
  "postings": [
    { "id": "7001", "source": "user:42:cash", "destination": "transit:withdrawals", "currency": "USD", "amount": "10000", "group_id": "7000" }
  ]
}
```

If the user doesn't have $100, this fails with `insufficient_funds`, and you tell them before anything reaches the bank.

## Step 2a: the bank pays it

When the bank confirms the payout, the money leaves the brokerage:

```json title="POST /transfers"
{
  "postings": [
    { "id": "7002", "source": "transit:withdrawals", "destination": "bank:client-money", "currency": "USD", "amount": "10000", "group_id": "7000" }
  ]
}
```

Both go down: the bank holds $100 less, and nothing is owed in transit any more.

## Step 2b: the bank rejects it

Say the user also asked to withdraw $50, posted as `7011` in group `7010`, and the bank rejects that payout. The money goes back to the user:

```json title="POST /transfers"
{
  "postings": [
    { "id": "7012", "source": "transit:withdrawals", "destination": "user:42:cash", "currency": "USD", "amount": "5000", "group_id": "7010" }
  ]
}
```

Either way, every withdrawal ends with its amount out of `transit:withdrawals`. At any moment, the balance of `transit:withdrawals` is the total of payouts the bank still owes an answer on. The `group_id` ties each request to its outcome, so `GET /groups/7000/transfers` tells the story of one withdrawal.

## Why not a hold?

Some ledgers can reserve money without moving it, often called a pending or two-phase transfer. TigerBeetle supports this, but Torment doesn't expose it yet. A transit account and batched postings cover the cases we know of, with one advantage: money in transit is a balance you can read, not something hidden inside the user's account. Torment will add pending transfers when a use case turns up that a transit account and batched postings can't handle.

## Where this leaves us

| Account | USD |
|---|---|
| `user:42:cash` | 253.25 |
| `transit:withdrawals` | 0 |
