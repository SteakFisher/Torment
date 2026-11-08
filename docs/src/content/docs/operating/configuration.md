---
title: Configuration
description: Environment variables, API keys and the Docker image.
---

Torment is one binary, configured entirely by environment variables.

| Variable | Required | Meaning |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string, such as `postgres://user:pass@host:5432/torment` |
| `TB_ADDRESS` | yes | TigerBeetle address, such as `3000` or `10.0.0.1:3000`. Use an IP address: the TigerBeetle client doesn't look up host names |
| `API_KEYS` | yes | comma-separated API keys |
| `ADDR` | no | address to listen on. Default `:8080` |

On startup, Torment connects to Postgres, applies any new database migrations, and connects to TigerBeetle. If any of these fail, it exits with an error.

## API keys

Every route except `GET /healthz` needs `Authorization: Bearer <key>`, where the key is one of `API_KEYS`. A missing or wrong key returns 401.

To rotate a key without downtime:

1. Add the new key: `API_KEYS=old-key,new-key`, and restart.
2. Move every caller to the new key.
3. Remove the old one: `API_KEYS=new-key`, and restart.

Every key can do everything. There are no per-key permissions.

The key travels in clear text, so keep Torment on a private network, or put TLS in front of it.

## The Docker image

The repo's `Dockerfile` builds a production image:

```sh
docker build -t torment .
docker run -p 8080:8080 \
  --security-opt seccomp=unconfined \
  -e DATABASE_URL=... -e TB_ADDRESS=10.0.0.1:3000 -e API_KEYS=... \
  torment
```

`seccomp=unconfined` is needed because the TigerBeetle client uses `io_uring`, which Docker's default security profile blocks.

## Health

`GET /healthz` returns 200 with `ok` and needs no key. It tells you the process is serving, not that Postgres and TigerBeetle are reachable.

## Shutdown

On SIGINT or SIGTERM, Torment stops accepting connections and waits up to 10 seconds for requests in flight to finish. A request cut off after that is like any other failed request: the caller retries it with the same ids.
