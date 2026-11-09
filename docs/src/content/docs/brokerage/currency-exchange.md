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

Exchanges get their own kind, `fx`. They aren't trades in shares, and the treasury will want to find them on their own:

```json title="POST /kinds"
{ "name": "fx" }
```

The deposit works like [chapter 2](/brokerage/deposits/), in AED, with the kind `deposit`. `bank:client-money` and `user:42:cash` each get an AED wallet on first use.

```json title="POST /transfers"
{
  "chains": [
    [{ "id": "00000000-0000-0000-0000-000000005001", "debit": "bank:client-money", "credit": "user:42:cash", "currency": "AED", "amount": "100000", "kind": "deposit" }]
  ]
}
```

## The exchange

A chain of two postings, in two currencies. The user gives the brokerage AED: debit `user:42:cash`, so the brokerage owes the user fewer dirhams, and credit `fx:position`. The brokerage gives the user USD: debit `fx:position`, and credit `user:42:cash`, so the brokerage owes the user more dollars.

```json title="POST /transfers"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000005011", "debit": "user:42:cash", "credit": "fx:position", "currency": "AED", "amount": "36725", "kind": "fx", "group_id": "00000000-0000-0000-0000-000000005010" },
      { "id": "00000000-0000-0000-0000-000000005012", "debit": "fx:position", "credit": "user:42:cash", "currency": "USD", "amount": "10000", "kind": "fx", "group_id": "00000000-0000-0000-0000-000000005010" }
    ]
  ]
}
```

The chain matters here too: the user must never give dirhams without getting dollars.

`fx:position` is equity, so a credit raises it and a debit lowers it. Afterwards it holds +367.25 AED and −100.00 USD. That's the brokerage's open position: it has taken in dirhams and promised dollars it hasn't bought yet. A negative balance is the honest way to show that, which is why this account allows one.

## Closing the position

Later, the treasury team converts the dirhams at the bank. Two more postings. The dirhams leave the bank: credit `bank:client-money`, an asset, so it goes down, and debit `fx:position` back to zero. The dollars arrive: debit `bank:client-money`, so it goes up, and credit `fx:position` back to zero.

```json title="POST /transfers"
{
  "chains": [
    [
      { "id": "00000000-0000-0000-0000-000000005021", "debit": "fx:position", "credit": "bank:client-money", "currency": "AED", "amount": "36725", "kind": "fx", "group_id": "00000000-0000-0000-0000-000000005020" },
      { "id": "00000000-0000-0000-0000-000000005022", "debit": "bank:client-money", "credit": "fx:position", "currency": "USD", "amount": "10000", "kind": "fx", "group_id": "00000000-0000-0000-0000-000000005020" }
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
