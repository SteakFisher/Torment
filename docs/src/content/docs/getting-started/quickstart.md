---
title: Quickstart
description: Run Torment locally and move your first money in five minutes.
---

You need Go 1.26 and Docker.

## Start it

```sh
make up    # Postgres 17 and a single-replica TigerBeetle
make run   # migrates the database and serves on :8080
```

Every example below is one HTTP request with a JSON body. To send one with curl:

```sh
curl -H 'Authorization: Bearer dev-key' -d '<body>' localhost:8080/<path>
```

`dev-key` is the development API key. Every route except `/healthz` and `/readyz` needs it.

## 1. Register a currency

Currencies are registered once. `USD/2` means amounts are in cents.

```json title="POST /currencies"
{ "code": "USD/2" }
```

```json title="201 Created"
{ "name": "USD", "precision": 2 }
```

## 2. Register two accounts

Every posting debits one account and credits another, so you need two. The first is your bank account, which is an `asset` because it's money you own.

```json title="POST /accounts"
{ "address": "bank:settlement", "type": "asset" }
```

The rider's wallet is a `liability`, because it's money you hold for the rider.

```json title="POST /accounts"
{ "address": "rider:1:wallet", "type": "liability" }
```

## 3. Move money

The rider tops up $100. The money arrives in your bank, and you now owe it to the rider. A posting always debits one account and credits another. Debiting an asset raises it, and crediting a liability raises it, so this posting debits the bank and credits the rider. [Debits and credits](/getting-started/concepts/#debits-and-credits) has the full table.

```json title="POST /transfers"
{
  "chains": [
    [
      {
        "id": "00000000-0000-0000-0000-000000000001",
        "debit": "bank:settlement",
        "credit": "rider:1:wallet",
        "currency": "USD",
        "amount": "10000"
      }
    ]
  ]
}
```

```json title="200 OK"
{ "chains": [[{ "id": "00000000-0000-0000-0000-000000000001", "group_id": "00000000-0000-0000-0000-000000000001", "status": "created" }]] }
```

Each element of `chains` is a chain: a list of postings that all happen or none do. This request has one chain with one posting, and the response has the same shape.

## 4. Read the balance

```http
GET /accounts/rider:1:wallet/balances
```

```json title="200 OK"
{
  "address": "rider:1:wallet",
  "totals": [{ "type": "liability", "currency": "USD", "precision": 2, "amount": "10000" }],
  "accounts": [
    {
      "address": "rider:1:wallet",
      "type": "liability",
      "balances": [{ "currency": "USD", "precision": 2, "amount": "10000" }]
    }
  ]
}
```

A single account comes back in the same shape as a [pattern](/reference/api/#get-accountsaddressbalances), so `totals` here is just the rider's balance.

Both balances went up: the bank holds $100, and you owe the rider $100. That's double-entry: every posting debits one account and credits another by the same amount.

## 5. Send it again

Send the same transfer from step 3 again. It isn't applied twice:

```json title="200 OK"
{ "chains": [[{ "id": "00000000-0000-0000-0000-000000000001", "group_id": "00000000-0000-0000-0000-000000000001", "status": "exists" }]] }
```

That's what makes retries safe: if a request times out, send it again with the same ids.

## 6. Try to overspend

The rider tries to withdraw $200 they don't have. Money leaving goes the other way: debit the rider's wallet to lower what you owe, and credit the bank to lower what it holds.

```json title="POST /transfers"
{
  "chains": [
    [
      {
        "id": "00000000-0000-0000-0000-000000000002",
        "debit": "rider:1:wallet",
        "credit": "bank:settlement",
        "currency": "USD",
        "amount": "20000"
      }
    ]
  ]
}
```

```json title="200 OK"
{
  "chains": [
    [{ "id": "00000000-0000-0000-0000-000000000002", "group_id": "00000000-0000-0000-0000-000000000002", "status": "failed", "error": "insufficient_funds", "message": "..." }]
  ]
}
```

Nothing moved. To try again after a top-up, use a new id: a failed id can't be reused.

## Next

- [Concepts](/getting-started/concepts/) explains accounts, debits and credits, postings and chains properly.
- [API](/reference/api/) lists every route.
