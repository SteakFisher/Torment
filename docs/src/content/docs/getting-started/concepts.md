---
title: Concepts
description: Currencies, accounts, wallets, postings, kinds, debits and credits, chains, groups and patterns.
---

## Currency
What money is counted in. `USD/2` means US dollars with 2 decimal places. Torment never uses decimals: you send amounts in the smallest unit, so `"1050"` is $10.50.

A currency doesn't have to be money. `AAPL/6` counts shares to six decimal places, so `"1500000"` is 1.5 shares.

## Account
A named pot of money, like `rider:123:wallet` or `platform:fees`. The name is up to you; the colons just keep related pots together. Every account has a type that says what it is to your business:

| Type | Is | Example |
|---|---|---|
| `asset` | money you own | your bank balance |
| `liability` | money you hold for someone else | a rider's wallet |
| `equity` | money put into the business | the owners' investment |
| `revenue` | money you earned | fees |
| `expense` | money you spent | card processing costs |

An account can't go below zero unless you register it with `allow_negative: true`. The type and `allow_negative` can never change.

## Wallet
One account can hold several currencies, and each currency gets its own wallet. You never create wallets: the first time money in a new currency touches an account, Torment makes the wallet.

## Posting
One movement of money between two accounts. A posting names a `debit` account, a `credit` account and an amount. It debits the first and credits the second by that same amount, so money is never created or lost.

## Kind
What a posting is: `deposit`, `trade`, `commission`, `dividend`. Every posting has one. You register kinds up front, like currencies, and a kind never changes once registered.

Kinds let you filter history: `?kind=dividend` on an account's transfers shows only its dividends. Totals and reports by kind belong in an analytics tool fed from TigerBeetle's change data capture, not in Torment.

## Debits and credits
A debit and a credit are the two sides of every posting. Neither one means "add" or "take away" on its own. What it does to a balance depends on the account's type:

| Type | A debit | A credit |
|---|---|---|
| `asset` | raises it | lowers it |
| `expense` | raises it | lowers it |
| `liability` | lowers it | raises it |
| `equity` | lowers it | raises it |
| `revenue` | lowers it | raises it |

So assets and expenses grow with debits. Liabilities, equity and revenue grow with credits.

**A user deposits $100.** The money lands in your bank, and you now owe it to the user:

- debit `bank:client-money`, an asset: it goes up by $100, because you have more money.
- credit `user:42:cash`, a liability: it goes up by $100, because you owe the user more.

Both balances went up, and that's right. One side says where the money is; the other says whose it is.

**The user pays a $1 fee.** Debit `user:42:cash`, a liability: it goes down, because you owe the user less. Credit `revenue:commissions`, revenue: it goes up, because you earned $1.

When you're unsure which side an account goes on, ask what should happen to its balance and read the table. TigerBeetle's [Financial Accounting](https://docs.tigerbeetle.com/coding/financial-accounting/) guide goes further in a few minutes.

## Ids and retries
You give every posting your own id. If a request times out, send it again with the same id: a posting that was already applied comes back as `exists` and isn't applied twice.

Ids are UUIDs. The examples write them as `00000000-0000-0000-0000-000000001001`, with a short number at the end, so they're easy to read. Real code should generate a UUIDv7 for each posting; any language's UUID library can. UUIDv7s increase over time, which TigerBeetle prefers: random UUIDs work, but they slow it down.

## Chain
A list of postings that must all happen or none of them. "Pay the merchant $25 and charge a $1 fee" is a chain: if the rider can't afford both, neither happens. Every request sends its postings in chains; a posting on its own is a chain of one.

## Group
A label (`group_id`) that ties related postings together so you can fetch them later. For example, a payment and its refund share a group.

## Pattern
An address with `*` in it, used to read many accounts at once. `rider:123:*` means all of rider 123's pots, and `rider:*:wallet` means every rider's wallet.
