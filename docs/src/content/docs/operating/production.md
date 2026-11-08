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

## Things to watch

- **`POST /transfers` latency and 500s.** A rising 500 rate usually means TigerBeetle or Postgres is unreachable.
- **Postings failing with `internal`.** These are logged and shouldn't happen in normal operation.
- **Slow history reads.** History reads for many different users can go to disk in TigerBeetle, and TigerBeetle handles one request at a time, so slow reads can delay writes. See [Performance](/internals/performance/).
