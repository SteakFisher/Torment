---
title: 5. Currency exchange
description: A user deposits dirhams and buys in dollars.
---

User 42 lives in Dubai. They deposit 1,000 UAE dirhams (AED) and want to trade in dollars. The brokerage converts 367.25 AED into $100.00 at a rate of 3.6725.

Torment doesn't do currency conversion. It never invents a rate or turns one currency into another. Each posting moves one currency, and you record the exchange as two postings: AED in, USD out.

## Setup

Register the currency, and an account that holds the brokerage's currency position:

```json title="POST /currencies"
{ "code": "AED/2" }
```

```json title="POST /accounts"
{ "address": "fx:position", "type": "equity", "allow_negative": true }
```

This is the one account in the tutorial with `allow_negative`, and the next sections explain why it needs it.

The deposit works like [chapter 2](/brokerage/deposits/), in AED. `bank:client-money` and `user:42:cash` each get an AED wallet on first use.

```json title="POST /transfers"
{
  "postings": [
    { "id": "5001", "source": "bank:client-money", "destination": "user:42:cash", "currency": "AED", "amount": "100000" }
  ]
}
```

## The exchange

A chain of two postings, in two currencies. The user gives the brokerage AED, and the brokerage gives the user USD:

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "5011", "source": "user:42:cash", "destination": "fx:position", "currency": "AED", "amount": "36725", "group_id": "5010" },
      { "id": "5012", "source": "fx:position", "destination": "user:42:cash", "currency": "USD", "amount": "10000", "group_id": "5010" }
    ]
  ]
}
```

The chain matters here too: the user must never give dirhams without getting dollars.

Afterwards, `fx:position` holds +367.25 AED and −100.00 USD. That's the brokerage's open position: it has taken in dirhams and promised dollars it hasn't bought yet. A negative balance is the honest way to show that, which is why this account allows one.

## Closing the position

Later, the treasury team converts the dirhams at the bank. Two more postings: the dirhams leave the bank, and the dollars arrive.

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "5021", "source": "fx:position", "destination": "bank:client-money", "currency": "AED", "amount": "36725", "group_id": "5020" },
      { "id": "5022", "source": "bank:client-money", "destination": "fx:position", "currency": "USD", "amount": "10000", "group_id": "5020" }
    ]
  ]
}
```

`fx:position` is back to zero in both currencies. At any moment, its balance tells the treasury exactly how much it still has to convert.

In practice, the treasury converts many users' exchanges at once, so the amounts won't match one user's. The brokerage also makes money on exchanges by giving a slightly worse rate than it gets. That margin shows up as whatever is left in `fx:position` after converting, and it can be moved to a revenue account.

## Where this leaves us

| Account | USD | AED |
|---|---|---|
| `user:42:cash` | 351.75 | 632.75 |
| `fx:position` | 0 | 0 |
