---
title: Introduction
description: What Torment is, how it works, and when to choose it.
---

Torment is a ledger service. It keeps track of who owns how much money and records every movement of it, so that money is never created or lost by mistake.

It sits on top of [TigerBeetle](https://tigerbeetle.com), a database built only for this job. TigerBeetle is extremely fast, but it only understands numbers: accounts and transfers are 128-bit ids. Torment adds what people need on top:

- **Readable names.** You write `rider:123:wallet`, not `0x01933f2a…`.
- **Accounting types.** Each account is an asset, liability, equity, revenue or expense, and its balance is reported the way an accountant expects.
- **Currencies by name.** You say `USD`; Torment handles TigerBeetle's ledger numbers.
- **History and patterns.** Read one user's statement, or the totals of every account matching `rider:123:*`.

## How it works

Torment uses two databases, each for what it's good at:

- **Postgres** stores names: currencies, accounts, and which TigerBeetle account belongs to each wallet.
- **TigerBeetle** stores money: every balance and every transfer.

When you post a transfer, Torment looks up the wallets (from memory after the first time) and sends the transfer straight to TigerBeetle. Postgres isn't involved, so the write runs at TigerBeetle's speed.

## Torment or Formance?

[Formance ledger](https://www.formance.com) offers the same model, with named accounts and postings, on Postgres alone.

**Choose Torment when write throughput matters,** especially on busy accounts. At medium contention, our benchmark measured 47,022 writes/s for Torment against 39 for Formance, with each system set up its best way. Formance locks each account's balance row, so writes to a busy account wait in line; TigerBeetle has no locks. See [Performance](/internals/performance/) for the full numbers and their caveats.

**Choose Formance when you need flexibility more than speed.** With Torment you give up:

- **Simplicity.** You run two databases, not one, and TigerBeetle needs its own machine and replicas in production.
- **Features.** Formance has Numscript, metadata on accounts and transactions, and filtered queries over everything. Torment has no metadata, no holds (pending transfers), no currency conversion and no scripting.
- **Reads.** Pattern balances stop at 1000 wallets and pattern history at 100, and nothing can total every user's balances. Those need an export to an analytics database, which isn't built yet.
- **Editing.** An account's type and `allow_negative` are fixed forever. A mistaken transfer is fixed with a new transfer that swaps its debit and credit, never by editing or deleting.
- **Ids.** You choose every posting's id, and ids should increase over time, because random ids slow TigerBeetle down.
- **Maturity.** Torment is young: no SDKs, no UI and only basic auth.
