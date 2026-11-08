---
title: 9. Mistakes
description: Fixing a wrong transfer without editing anything.
---

The market data feed was wrong. Fill 1 of order 3000 in [chapter 4](/brokerage/partial-fills/) was recorded at $200.00, but the real price was $190.00. User 42 was charged $10 too much.

## You can't edit or delete

A posting in Torment can never be changed or removed. That's deliberate: a ledger where history can be rewritten can't be trusted, by auditors or by you. Every correction is a new posting, and the mistake stays visible next to its fix.

## Reverse it, then post it again

Undo the wrong posting with one going the other way, and post the right one. Put both in one chain, so the user's balance is never halfway corrected. Use the order's `group_id`, so the fix shows up with the order.

```json title="POST /transfers"
{
  "postings": [
    [
      { "id": "9001", "source": "bank:client-money", "destination": "user:42:cash", "currency": "USD", "amount": "20000", "group_id": "3000" },
      { "id": "9002", "source": "user:42:cash", "destination": "bank:client-money", "currency": "USD", "amount": "19000", "group_id": "3000" }
    ]
  ]
}
```

The user gets $10 back, and `GET /groups/3000/transfers` now shows the original fill, its reversal and the corrected fill.

You could post just the $10 difference instead. A full reversal is clearer on a statement, because each line matches something that really happened: "fill at $200", "reversal of fill", "fill at $190".

## Don't fix by reusing an id

Sending a posting again with the same id but a different amount doesn't change it. Torment refuses:

```json title="200 OK"
{
  "transfers": [
    { "id": "1001", "group_id": "1001", "status": "failed", "error": "conflict", "message": "conflict: transfer 1001 exists with different fields: TransferExistsWithDifferentAmount" }
  ]
}
```

That's the same protection that makes retries safe. A retry must be identical, so a different posting under the same id is a mistake, and Torment says so instead of guessing.

## Mistakes that can't be fixed with a posting

Some choices are permanent:

- **An account's type and `allow_negative`.** If an account was registered with the wrong type, register a new account with the right one, and move the balance across with a posting.
- **A currency's precision.** Registering `USD` again with a different precision returns 409. Choose precisions carefully, including enough decimals for fractional shares.

## The end

User 42's ledger is complete: deposits, trades, partial fills, an exchange, a dividend, withdrawals, a statement and a correction. Every one of those was an ordinary posting or chain, and every balance can be explained line by line from the history.

For every route and field, see the [API reference](/reference/api/).
