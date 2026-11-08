---
title: Limits
description: Every size limit in Torment, and why each exists.
---

| Limit | Value | Over the limit | Why |
|---|---|---|---|
| Postings per request | 8,189 | 400 | TigerBeetle's largest batch |
| Request body | 1 MiB (about 5,000 postings) | 400 | protects the server from huge bodies. Can be raised if a caller needs it |
| Address length | 255 bytes | 400 | |
| Currency precision | 0 to 18 decimal places | 400 | |
| Ids, group ids | 1 to 2¹²⁸−2 | 400 | TigerBeetle ids are 128-bit, and 0 and 2¹²⁸−1 are reserved |
| Amounts | 1 to 2¹²⁸−1 minor units | 400 | |
| Page size (`limit`) | default 100, max 1,000 | 400 | |
| Wallets in a pattern balance query | 1,000 | 400, asking you to narrow the pattern | each read covers every wallet at once |
| Wallets in a pattern history query | 100 | 400, asking you to narrow the pattern | each wallet is one query to TigerBeetle |
| Transfers in a group | 8,189, the newest | the rest aren't returned | TigerBeetle's largest query result |

## Paging

Lists don't have a total limit, only a page size. `GET /accounts` and history queries return `next` when more results remain: pass it back as `after` for accounts, or `before` for transfers. Each page costs the same, however deep you go.

## Things with no limit

- **Accounts, wallets and currencies.** As many as Postgres and TigerBeetle hold.
- **Postings over time.** TigerBeetle is built to store billions of transfers.
- **Concurrent requests.** Torment merges concurrent writes into shared TigerBeetle requests. The practical limit is the CPU; see [Performance](/internals/performance/).
