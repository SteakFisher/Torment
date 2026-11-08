---
title: 1. Chart of accounts
description: The currencies and accounts a brokerage needs, and the type of each.
---

Before any money moves, a business decides which accounts it keeps. Accountants call that list the chart of accounts. For our brokerage it is short.

## Currencies

The brokerage deals in dollars, so it registers `USD` with 2 decimal places:

```json title="POST /currencies"
{ "code": "USD/2" }
```

It also deals in shares. Torment doesn't care whether a currency is money: a share is just another thing to count. Registering each stock as a currency with 6 decimal places lets users own fractions of a share:

```json title="POST /currencies"
{ "code": "AAPL/6" }
```

```json title="POST /currencies"
{ "code": "TSLA/6" }
```

So `"1500000"` in `AAPL` is 1.5 shares, the same way `"1500"` in `USD` is $15.00. You register a new ticker the day you list it.

## Accounts

The brokerage needs two kinds of accounts: its own, and one set per user.

| Address | Type | What it is |
|---|---|---|
| `bank:client-money` | asset | users' dollars, sitting in the brokerage's bank account |
| `custodian:positions` | asset | shares the custodian holds for the brokerage |
| `revenue:commissions` | revenue | the fees the brokerage earns |
| `user:42:cash` | liability | dollars the brokerage owes user 42 |
| `user:42:holdings` | liability | shares the brokerage owes user 42, one wallet per stock |

The custodian is the firm that actually holds shares on the stock market. The brokerage doesn't hold them itself.

### Why these types

The type says what the account is from the **brokerage's** point of view:

- **Asset: things the brokerage has.** The money in its bank account and the shares at its custodian.
- **Liability: things the brokerage owes.** A user's cash isn't the brokerage's money. The brokerage holds it and owes it back. Shares bought for a user are owed to that user too.
- **Revenue: what the brokerage earns.** The commission on each trade.

A user's wallet is a liability, which feels backwards at first: from the user's side, it's their asset. A ledger always takes the business's side.

### Which way balances move

Every posting has a `source` and a `destination`. How each one's balance changes depends on its type:

| Type | As `source` | As `destination` |
|---|---|---|
| asset, expense | goes **up** | goes **down** |
| liability, equity, revenue | goes **down** | goes **up** |

So when a user deposits $100, the posting goes from `bank:client-money` to `user:42:cash`, and **both** go up: the bank holds $100 more, and the brokerage owes the user $100 more. You'll see this in the next chapter.

## Registering the brokerage's accounts

These are registered once, when the brokerage starts:

```json title="POST /accounts"
{ "address": "bank:client-money", "type": "asset" }
```

```json title="POST /accounts"
{ "address": "custodian:positions", "type": "asset" }
```

```json title="POST /accounts"
{ "address": "revenue:commissions", "type": "revenue" }
```

Registering is safe to repeat: the same request again returns `200` instead of `201`. You can run these on every deploy.

## Registering a user

When user 42 signs up, register their two accounts:

```json title="POST /accounts"
{ "address": "user:42:cash", "type": "liability", "currencies": ["USD"] }
```

```json title="201 Created"
{
  "address": "user:42:cash",
  "type": "liability",
  "allow_negative": false,
  "created_at": "2026-11-06T10:00:00.123456Z",
  "wallets": [{ "currency": "USD", "precision": 2 }]
}
```

```json title="POST /accounts"
{ "address": "user:42:holdings", "type": "liability" }
```

`currencies` creates the dollar wallet now, so the user's first deposit doesn't have to. Holdings get no currencies: a wallet for each stock appears the first time the user buys it.

## Nothing can go negative

None of these accounts set `allow_negative`, so none can go below zero. A user can't spend dollars they don't have, the brokerage can't owe a negative number of shares, and the bank account can't pay out money it doesn't hold. Torment refuses any posting that would break that.

## Where this leaves us

The brokerage has its currencies and accounts, and every balance is zero. Next, money comes in.
