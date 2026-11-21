---
title: 10. Notifications
description: Emailing users when money arrives or leaves, from Torment's events.
---

User 42 should get an email when money arrives or leaves their account: a deposit, a dividend, a withdrawal. Asking Torment every few seconds whether anything changed, for every user, would be slow and wasteful. Instead, Torment publishes every committed transfer as an [event](/reference/events/) to RabbitMQ, and a small notification service listens.

## Picking the events

Every event has a routing key `<currency>.<kind>`, such as `USD.dividend`. The notification service needs three kinds in any currency, so it binds its queue to three patterns:

| Binding key | Gets |
|---|---|
| `*.deposit` | money arriving from the bank |
| `*.dividend` | dividends |
| `*.withdrawal` | every step of a withdrawal |

Trades and commissions never reach this queue. The app already confirms an order when it fills, so emailing about it again would be noise. Corrections from the [last chapter](/brokerage/mistakes/) aren't bound either; add `*.correction` if users should hear about those.

A queue gets only the events published after it's bound, so create it before the brokerage takes its first deposit. Locally, the dev user `consumer` can create queues whose names start with `consumer.`:

```sh
curl -u consumer:consumer -X PUT -H 'Content-Type: application/json' \
  -d '{"durable":true,"arguments":{"x-queue-type":"quorum"}}' \
  localhost:15672/api/queues/torment/consumer.notifications
for key in '*.deposit' '*.dividend' '*.withdrawal'; do
  curl -u consumer:consumer -X POST -H 'Content-Type: application/json' \
    -d "{\"routing_key\":\"$key\"}" \
    localhost:15672/api/bindings/torment/e/torment.events/q/consumer.notifications
done
```

The service then reads `consumer.notifications` from `amqp://consumer:consumer@localhost:5672/torment`.

## An event

The dividend from [chapter 6](/brokerage/dividends/) arrives as:

```json title="USD.dividend"
{
  "id": "00000000-0000-0000-0000-000000006001",
  "group_id": "00000000-0000-0000-0000-000000006000",
  "debit": "bank:client-money",
  "credit": "user:42:cash",
  "currency": "USD",
  "precision": 2,
  "amount": "150",
  "kind": "dividend",
  "timestamp": "2026-11-06T12:41:09.4Z",
  "debit_balance": "91325",
  "credit_balance": "35325"
}
```

The user is the middle part of `user:42:cash`. The event also carries each account's balance right after the transfer, so the email can say "You received a $1.50 dividend. Your cash balance is now $353.25." without calling the API.

## Which email

The kind says what happened, and the two accounts say which step it was:

| Kind | Debit | Credit | Email |
|---|---|---|---|
| `deposit` | `bank:client-money` | `user:N:cash` | Your deposit of $X has arrived. |
| `dividend` | `bank:client-money` | `user:N:cash` | You received a dividend of $X. |
| `withdrawal` | `user:N:cash` | `transit:withdrawals` | We're sending $X to your bank. |
| `withdrawal` | `transit:withdrawals` | `bank:client-money` | Your withdrawal of $X has been paid. |
| `withdrawal` | `transit:withdrawals` | `user:N:cash` | Your bank rejected the withdrawal; $X is back in your account. |

The fourth row names no user: the money moves between two of the brokerage's own accounts. This is where the withdrawal's group pays off. Every step of one withdrawal shares a `group_id` (see [chapter 7](/brokerage/withdrawals/)), so the service reads the group and takes the user from the first step:

```http
GET /groups/00000000-0000-0000-0000-000000007000/transfers
```

The oldest transfer in the group debits `user:42:cash`, so the email goes to user 42.

## Sending each email once

RabbitMQ delivers every event at least once. After a restart of the exporter or of the notification service, the same event can arrive again, and nobody wants two "your withdrawal has been paid" emails. The event's `id` is the transfer's id, so it identifies the event:

1. If the `id` is already in the service's table of sent emails, skip to step 4.
2. Send the email.
3. Store the `id` in the table, under a unique key.
4. Acknowledge the message.

If the service crashes between steps 2 and 3, the event comes back and the user gets the email twice. Storing the id before sending would avoid that, but a crash then would lose the email instead. For money leaving an account, a rare duplicate is the better mistake.

Acknowledge only at step 4. An event acknowledged before the email went out is gone if the service crashes.

## In production

This queue mustn't lose events: an event published while it's missing is never emailed. Declare it with the broker rather than from the service, and give the service a user that can only read it. See [queues that must not lose events](/operating/production/#queues-that-must-not-lose-events). A queue nobody reads keeps growing, so alert on its depth as well; see [export alerts](/operating/production/#export-alerts).

## The end

User 42's ledger is complete: deposits, trades, partial fills, an exchange, a dividend, withdrawals, a statement, a correction, and an email for every dollar that came in or went out. Every one of those was an ordinary posting or chain, and every balance can be explained line by line from the history.

For every route and field, see the [API reference](/reference/api/). For the full event format, see [Events](/reference/events/).
