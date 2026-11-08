---
title: Overview
description: Build the ledger of a stock brokerage with Torment, one requirement at a time.
---

This tutorial builds the ledger of a small stock brokerage. Users deposit dollars, buy and sell shares, get dividends and withdraw. Each chapter takes one thing the business needs and shows the Torment feature that handles it.

By the end, you'll have used every part of the API and seen why each part exists.

## What you need

- Torment running locally (see the [Quickstart](/getting-started/quickstart/)).
- A rough idea of [debits and credits](https://docs.tigerbeetle.com/coding/financial-accounting/). Each chapter explains the accounting it needs, so you don't need much.

## Chapters

1. **[Chart of accounts](/brokerage/chart-of-accounts/):** which accounts the brokerage needs, and why each has the type it has.
2. **[Deposits](/brokerage/deposits/):** money arriving, and making sure a payment is never counted twice.
3. **[Buying shares](/brokerage/buying-shares/):** paying, receiving shares and charging a fee, all or nothing.
4. **[Partial fills](/brokerage/partial-fills/):** one order filled in pieces, kept together as one group.
5. **[Currency exchange](/brokerage/currency-exchange/):** a user deposits dirhams and buys in dollars.
6. **[Dividends](/brokerage/dividends/):** paying thousands of users in a few requests.
7. **[Withdrawals](/brokerage/withdrawals/):** money that has left the user's wallet but not yet left the bank.
8. **[Statements](/brokerage/statements/):** a user's history, their balance on any past date, and checking the books.
9. **[Mistakes](/brokerage/mistakes/):** fixing a wrong transfer without editing anything.

Every request in the tutorial is also in `scripts/brokerage.sh`. With Torment running, `make brokerage` sends them all and checks each response, so the tutorial stays true to the API.
