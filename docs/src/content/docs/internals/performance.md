---
title: Performance
description: Benchmarks against Formance, and what they do and don't show.
---

## The benchmark
A simulated brokerage (deposits, withdrawals, buys, sells, dividends, plus 10% balance and statement reads) on one AWS `i4i.2xlarge` (8 vCPU), with the database and ledger on the same machine. Every run was checked afterwards: each balance moved exactly as expected and nothing was overdrawn. Formance is v2.4.17, in whichever of its default or minimal feature sets did better. The harness is [TorBench](https://github.com/SteakFisher/TorBench).

**The typical case is medium contention: trades spread over 16 sets of shared accounts, with a few busy users. Each system at its best there:**
- **Torment, with 100 writes per request: 47,022 writes/s.**
- **Formance, with minimal features, one write per request and 16 clients: 39 writes/s.** Batching made it slower.

Applied writes per second for every case, the best of 16, 64 or 256 concurrent clients:

| Contention | Torment, 1 per request | Torment, 100 per request | Formance, 1 per request | Formance, 100 per request |
|---|---|---|---|---|
| Hot: every trade touches the same few accounts | 7,081 | 35,995 | 3 | 0 (all timed out) |
| **Medium: 16 sets of accounts** | **6,610** | **47,022** | **39** | **16** |
| Spread: 256 sets, users picked at random | 1,759 (7,891 on a fresh database) | 45,074 | 687 | 1,053 |

The big gap under contention is a design difference. Formance locks each account's balance row, so writes to a busy account wait in line. TigerBeetle applies writes in batches with no locks.

## Be fair about the numbers
- **We wrote the benchmark,** and we know Torment far better than Formance. Formance ran with the settings from its example compose file, and someone who knows it well could probably tune it further.
- **One run each, not medians.**
- **The spread result depends on the data.** After about 16 minutes of earlier runs, Torment dropped from about 7,900 to 1,759 writes/s. Our best explanation, from the numbers rather than a direct measurement, is that statement reads for random users miss TigerBeetle's cache and go to disk, and because TigerBeetle handles one request at a time, writes wait behind them.
- **With everything on one machine, latency is low for both.** Across a real network, every request pays the round trip as well.
