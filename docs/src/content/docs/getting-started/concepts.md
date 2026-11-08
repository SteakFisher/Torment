---
title: Concepts
description: Currencies, accounts, wallets, postings, chains, groups and patterns.
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

New to debits and credits? TigerBeetle's [Financial Accounting](https://docs.tigerbeetle.com/coding/financial-accounting/) guide explains them and these five types in a few minutes.

An account can't go below zero unless you register it with `allow_negative: true`. The type and `allow_negative` can never change.

## Wallet
One account can hold several currencies, and each currency gets its own wallet. You never create wallets: the first time money in a new currency touches an account, Torment makes the wallet.

## Posting
One movement of money: take this amount from `source` and give it to `destination`. Every posting changes two accounts by the same amount, so money is never created or lost.

## Ids and retries
You give every posting your own id. If a request times out, send it again with the same id: a posting that was already applied comes back as `exists` and isn't applied twice.

Use ids that increase over time, such as a UUIDv7 read as a number. Random ids work, but they slow TigerBeetle down.

## Chain
A set of postings that must all happen or none of them. "Pay the merchant $25 and charge a $1 fee" is a chain: if the rider can't afford both, neither happens.

## Group
A label (`group_id`) that ties related postings together so you can fetch them later. For example, a payment and its refund share a group.

## Pattern
An address with `*` in it, used to read many accounts at once. `rider:123:*` means all of rider 123's pots, and `rider:*:wallet` means every rider's wallet.
