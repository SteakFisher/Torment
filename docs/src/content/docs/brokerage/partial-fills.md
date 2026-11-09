---
title: 4. Partial fills
description: One order filled in pieces, kept together as one group.
---

User 42 places an order for 3 Tesla shares. The market doesn't fill it all at once: it fills one share at $200.00, then one at $200.50, then the last at $199.75. Each fill arrives at a different time, and each one is real money moving, so each is recorded as it happens.

## One chain per fill

Each fill is its own chain: the user pays for one share and receives it. The fee is charged once, with the last fill. Every posting carries the order id, `3000`, as its `group_id`.

```json title="POST /transfers (fill 1)"
{
  "postings": [
    [
      { "id": "3001", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "20000", "group_id": "3000" },
      { "id": "3002", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "TSLA", "amount": "1000000", "group_id": "3000" }
    ]
  ]
}
```

```json title="POST /transfers (fill 2)"
{
  "postings": [
    [
      { "id": "3003", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "20050", "group_id": "3000" },
      { "id": "3004", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "TSLA", "amount": "1000000", "group_id": "3000" }
    ]
  ]
}
```

```json title="POST /transfers (fill 3, and the fee)"
{
  "postings": [
    [
      { "id": "3005", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "19975", "group_id": "3000" },
      { "id": "3006", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "TSLA", "amount": "1000000", "group_id": "3000" },
      { "id": "3007", "source": "user:42:cash", "destination": "revenue:commissions", "currency": "USD", "amount": "100", "group_id": "3000" }
    ]
  ]
}
```

## The whole order

A group is how you get everything that happened to one order back, newest first:

```http
GET /groups/3000/transfers
```

```json title="200 OK"
{
  "group_id": "3000",
  "transfers": [
    { "id": "3007", "group_id": "3000", "source": "user:42:cash", "destination": "revenue:commissions", "currency": "USD", "amount": "100", "timestamp": "2026-11-06T12:41:09.206050152Z" },
    { "id": "3006", "group_id": "3000", "source": "custodian:positions", "destination": "user:42:holdings", "currency": "TSLA", "amount": "1000000", "timestamp": "2026-11-06T12:41:09.206050151Z" },
    { "id": "3005", "group_id": "3000", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "19975", "timestamp": "2026-11-06T12:41:09.20605015Z" },
    ...
  ]
}
```

That's what support needs when a user asks "what happened to my order?": every fill, the fee, and the exact time each was recorded.

## Groups are just labels

- **A group isn't created.** Any number can be a `group_id`. An unused one returns an empty list, not a 404.
- **A group doesn't have to be one request.** These postings came in three requests, minutes apart.
- **A group isn't a chain.** The chain decides what happens together. The group only decides what you can find together.
- **Without a `group_id`,** a posting's group is its own id.

A group comes back 100 transfers at a time by default. That's plenty for an order. For bigger groups, see [Dividends](/brokerage/dividends/).

## Where this leaves us

| Account | USD | AAPL | TSLA |
|---|---|---|---|
| `user:42:cash` | 251.75 | | |
| `user:42:holdings` | | 1.0 | 3.0 |
