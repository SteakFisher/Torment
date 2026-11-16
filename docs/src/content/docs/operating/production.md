---
title: Production
description: Running TigerBeetle, Postgres and Torment for real money.
---

The local setup runs everything on one machine with a single TigerBeetle replica. That's fine for development and wrong for production.

## TigerBeetle

- **Run a cluster, not one replica.** Production TigerBeetle runs 3 or 6 replicas, so it survives a machine failing without losing a transfer. One replica means one disk away from losing the ledger.
- **Give it its own machines.** TigerBeetle is built to own a machine: it manages its own memory and disk. Sharing a machine with Postgres or Torment slows all three down.
- **Give it enough memory to cache.** TigerBeetle uses Direct IO, so the operating system's file cache doesn't help it. Its own cache (`--cache-grid`) is all it has, and reads that miss it go to disk.
- **Follow TigerBeetle's own guide** for formatting, upgrades and hardware: [docs.tigerbeetle.com](https://docs.tigerbeetle.com/operating/).

## Postgres

Postgres holds names, not money, so it's small and quiet once wallets exist: writes touch it only to create new wallets. Run it with your usual backups and a standby.

Postgres and TigerBeetle must be backed up and restored together. A TigerBeetle account with no Postgres row is treated as broken data.

## Torment

- **Run several instances behind a load balancer.** Torment keeps no state of its own. Each instance keeps its own in-memory copy of wallets it has seen, and that copy never goes stale, because a wallet never changes once it exists.
- **Up to 32 instances.** Each instance opens two TigerBeetle client sessions, one for writes and one for reads, and a cluster allows 64 sessions by default.
- **Roll out one instance first.** Migrations run at startup. When a release adds a migration, start one instance, let it migrate, then start the rest.
- **Keep it close to TigerBeetle.** Network round trips don't limit throughput, because writes are batched, but they add directly to each request's latency.

## Export

The [event export](/reference/events/) has three parts, all outside the API's request path:

```
TigerBeetle ─▶ tigerbeetle amqp ─▶ RabbitMQ vhost tigerbeetle ─▶ torment cdc ─▶ RabbitMQ vhost torment
               (CDC job)           queue torment.cdc            (exporter)     exchange torment.events
```

- **The CDC job** is TigerBeetle's own `tigerbeetle amqp`, run beside the cluster. It publishes every committed transfer to the exchange `tigerbeetle.cdc` and keeps its progress in RabbitMQ, so a restart carries on where it stopped. It takes IP addresses only, has no TLS, and exits on any connection error without restarting itself: run it under a supervisor that restarts it (the compose file uses `restart: unless-stopped`).
- **The exporter** is `torment cdc`, the same binary. It reads `torment.cdc`, adds names from Postgres, publishes to `torment.events`, and acknowledges a message only after RabbitMQ confirms its event. It needs only Postgres, not TigerBeetle, and runs no migrations. It exits non-zero when a connection fails or a message can't be turned into an event; restart it.
- **RabbitMQ** holds both vhosts. `rabbitmq/definitions.json` declares the vhosts, exchanges, users and permissions, and the queue `torment.cdc` bound before the CDC job starts. Messages published before that queue exists are lost.

### One exporter

Events must leave in order, so only one exporter reads at a time. `torment.cdc` is a single-active-consumer queue: start two exporters and the second waits as a standby, taking over within seconds when the first stops.

A message the exporter can't turn into an event (a TigerBeetle account with no wallet row, a code with no kind) stays at the head of the queue, and the exporter exits each time it reaches it. Export stops until the data is fixed. This only happens with broken data, such as a TigerBeetle cluster shared with something else.

### One broker or two

By default one RabbitMQ holds both vhosts, and `CDC_SOURCE_URL` and `CDC_EVENTS_URL` point at the same broker. When a memory or disk alarm fires, RabbitMQ blocks every publisher on that broker, so a slow consumer filling its queue also stalls the CDC job. Split into two brokers when that risk matters: move the `torment` vhost to its own broker and point `CDC_EVENTS_URL` at it. Nothing else changes.

### Replaying events

A queue bound today doesn't get yesterday's events. To fill it, replay everything from TigerBeetle:

1. Declare and bind the new queue.
2. Restart the CDC job once with `--timestamp-last=0`, then without it.

Every queue already bound gets every event again, so their consumers must drop duplicates by `id`, as they already should. Deleting RabbitMQ's data also replays everything, since the CDC job's progress lives there.

## Things to watch

- **`POST /transfers` latency and 500s.** A rising 500 rate usually means TigerBeetle or Postgres is unreachable.
- **Postings failing with `internal`.** These are logged and shouldn't happen in normal operation.
- **`torment.cdc` depth.** It should stay near zero. A growing queue means the exporter is down, stuck on a broken message, or slower than the write rate.
- **Consumer queue depth.** A queue nobody reads grows until RabbitMQ's alarms block publishing for everyone on that broker.
- **Slow history reads.** History reads for many different users can go to disk in TigerBeetle, and TigerBeetle handles one request at a time, so slow reads can delay writes. See [Performance](/internals/performance/).
