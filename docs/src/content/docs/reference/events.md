---
title: Events
description: The export of every committed transfer, and how to consume it.
---

Every committed transfer is published as an event to the RabbitMQ topic exchange `torment.events`, on the vhost `torment`. Bind your own queue to it and read it with anything that speaks AMQP 0.9.1: your own service, ClickHouse's RabbitMQ table engine, Redpanda Connect, or a bridge to Kafka. Torment writes no destination-specific exporters.

Running the export is covered in [Production](/operating/production/#export).

## The event

```json
{
  "id": "0195e6b2-7c1a-7000-8000-000000000001",
  "group_id": "0195e6b2-7c1a-7000-8000-000000000001",
  "debit": "world",
  "credit": "rider:1:wallet",
  "currency": "USD",
  "precision": 2,
  "amount": "10000",
  "kind": "top_up",
  "timestamp": "2026-11-16T09:30:00.123456789Z",
  "debit_balance": "-10000",
  "credit_balance": "10000"
}
```

The first nine fields are the transfer exactly as [`GET /groups/{group_id}/transfers`](/reference/api/) returns it. An event holds only what the API shows: addresses, currency and kind names, never TigerBeetle ids, ledgers, codes or flags.

| Field | Meaning |
|---|---|
| `id` | the posting's id. Use it to drop duplicates |
| `group_id` | the posting's group id, which is `id` when the posting set none |
| `debit`, `credit` | the two accounts' addresses |
| `currency`, `precision` | the currency, and its number of decimal places |
| `amount` | integer minor units, as a decimal string |
| `kind` | the transfer kind |
| `timestamp` | when TigerBeetle committed the transfer, with nanoseconds. Unique per transfer |
| `debit_balance`, `credit_balance` | each account's balance in this currency right after the transfer, by its normal side, as a decimal string. The same number `GET /accounts/{address}/balances?at=<timestamp>` returns |

Failed postings are never published, since TigerBeetle never committed them.

## Routing keys

The routing key is `<currency>.<kind>`, such as `USD.top_up`. Bind with a topic pattern to take what you need:

| Binding key | Gets |
|---|---|
| `#` | every event |
| `USD.*` | every USD transfer |
| `*.commission` | commissions in every currency |
| `EUR.refund` | EUR refunds only |

Each message is persistent JSON (`content_type: application/json`), with `message_id` set to the transfer id and `timestamp` to the commit time in seconds.

## Delivery

- **At least once.** An event can arrive twice, for example after the exporter restarts. Drop duplicates by `id`.
- **Committed order, mostly.** Events are published in commit order, but a redelivery repeats earlier events. Order by `timestamp` when order matters.
- **Only bound queues get events.** An event published while no queue matches its routing key is dropped by RabbitMQ. Bind your queue before you need its events; to fill a new queue with history, see [replaying](/operating/production/#replaying-events).
- **Delay.** Events trail the API by about a second, the time TigerBeetle's CDC job and the exporter take.

## Reading the events

Each consumer gets its own RabbitMQ user on the `torment` vhost, allowed to declare and read only its own queues. The dev stack has one, `consumer` (password `consumer`), whose queues must be named `consumer.*`:

```sh
# Declare a queue and bind it to every USD event, through the management API.
curl -u consumer:consumer -X PUT -H 'Content-Type: application/json' \
  -d '{"durable":true,"arguments":{"x-queue-type":"quorum"}}' \
  localhost:15672/api/queues/torment/consumer.analytics
curl -u consumer:consumer -X POST -H 'Content-Type: application/json' \
  -d '{"routing_key":"USD.*"}' \
  localhost:15672/api/bindings/torment/e/torment.events/q/consumer.analytics
```

Then consume `consumer.analytics` from `amqp://consumer:consumer@localhost:5672/torment`. Acknowledge each message after you store it, so a crash before then redelivers it.

A durable quorum queue keeps events while your consumer is down. It grows without limit until you read it, so a consumer you abandon should have its queue deleted.

## Not losing events

Torment never drops a committed transfer before it reaches `torment.events`. From there, keeping it is your queue's job:

- **Bind before you need events.** A queue gets events from the moment it's bound. Older ones need a [replay](/operating/production/#replaying-events).
- **Use a durable quorum queue.** A non-durable queue, or one with `x-expires`, disappears with everything in it.
- **Don't delete or unbind it.** Events published while it's missing are gone for it. For a consumer that must see everything, have the queue declared with the broker and give the consumer read-only access; see [Production](/operating/production/#queues-that-must-not-lose-events).
- **Acknowledge after storing.** An event you ack and then fail to store is gone.
- **Drop duplicates by `id`.** Redeliveries and replays send events again. In ClickHouse, a `ReplacingMergeTree` ordered by `id` does this.
