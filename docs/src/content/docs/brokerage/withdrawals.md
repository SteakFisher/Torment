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

Move the money out of the user's cash straight away, so they can't spend it twice. Debit `user:42:cash`, so the brokerage owes the user $100 less, and credit `transit:withdrawals`, so it owes $100 more in transit:

```json title="POST /transfers"
{
  "chains": [
    [{ "id": "00000000-0000-0000-0000-000000007001", "debit": "user:42:cash", "credit": "transit:withdrawals", "currency": "USD", "amount": "10000", "group_id": "00000000-0000-0000-0000-000000007000" }]
  ]
}
```

If the user doesn't have $100, this fails with `insufficient_funds`, and you tell them before anything reaches the bank.

## Step 2a: the bank pays it

When the bank confirms the payout, the money leaves the brokerage. Debit `transit:withdrawals`, a liability, and credit `bank:client-money`, an asset:

```json title="POST /transfers"
{
  "chains": [
    [{ "id": "00000000-0000-0000-0000-000000007002", "debit": "transit:withdrawals", "credit": "bank:client-money", "currency": "USD", "amount": "10000", "group_id": "00000000-0000-0000-0000-000000007000" }]
  ]
}
```

Both go down: a debit lowers a liability and a credit lowers an asset. The bank holds $100 less, and nothing is owed in transit any more.

## Step 2b: the bank rejects it

Say the user also asked to withdraw $50, posted as `00000000-0000-0000-0000-000000007011` in group `00000000-0000-0000-0000-000000007010`, and the bank rejects that payout. The money goes back to the user: debit `transit:withdrawals` and credit `user:42:cash`, the reverse of step 1:

```json title="POST /transfers"
{
  "chains": [
    [{ "id": "00000000-0000-0000-0000-000000007012", "debit": "transit:withdrawals", "credit": "user:42:cash", "currency": "USD", "amount": "5000", "group_id": "00000000-0000-0000-0000-000000007010" }]
  ]
}
```

Either way, every withdrawal ends with its amount out of `transit:withdrawals`. At any moment, the balance of `transit:withdrawals` is the total of payouts the bank still owes an answer on. The `group_id` ties each request to its outcome, so `GET /groups/00000000-0000-0000-0000-000000007000/transfers` tells the story of one withdrawal.

## Why not a hold?

Some ledgers can reserve money without moving it, often called a pending or two-phase transfer. TigerBeetle supports this, but Torment doesn't expose it yet. A transit account and batched postings cover the cases we know of, with one advantage: money in transit is a balance you can read, not something hidden inside the user's account. Torment will add pending transfers when a use case turns up that a transit account and batched postings can't handle.

## Where this leaves us

| Account | USD |
|---|---|
| `user:42:cash` | 253.25 |
| `transit:withdrawals` | 0 |
