# Operating Cinder

Cinder is a single Go binary plus Redis. This page covers TLS, reverse proxies, Redis hardening, configuration and backups. API details are in [api.md](api.md); design rationale is in [design.md](design.md).

## Configuration (`CINDER_*` environment variables)

| Variable | Default | Description |
|---|---|---|
| `CINDER_ADDR` | `:8080` | Listen address, `host:port` or `:port`. |
| `CINDER_REDIS_URL` | `redis://localhost:6379/0` | `redis://` or `rediss://` URL. Compose sets `redis://redis:6379/0`. |
| `CINDER_MAX_CIPHERTEXT_BYTES` | `65536` | Max ciphertext size (1 to 1048576). Applies to the whole encrypted envelope including the 16-byte GCM tag. Request body limits derive from it. |
| `CINDER_TRUSTED_PROXIES` | empty | Comma-separated IPs or CIDRs of reverse proxies. `X-Forwarded-For` / `X-Forwarded-Proto` are honoured only from these peers. |
| `CINDER_ALLOWED_ORIGINS` | empty | Comma-separated extra `scheme://host[:port]` origins accepted on `consume` (besides the request's own origin). |
| `CINDER_RATE_LIMIT_CREATE_PER_MIN` | `30` | Per-IP creates per minute. `0` disables. |
| `CINDER_RATE_LIMIT_CONSUME_PER_MIN` | `60` | Per-IP consumes per minute. `0` disables. |
| `CINDER_RATE_LIMIT_STATUS_PER_MIN` | `120` | Per-IP status checks per minute. `0` disables. |
| `CINDER_RATE_LIMIT_BURST` | `10` | Token bucket burst size (minimum 1). |

Invalid values make the process exit with code 2 and a message on stderr. Numeric limits must be between the stated bounds and 1000000.

## TLS and reverse proxy

Cinder serves plain HTTP. **Terminate TLS in front of it.** The browser needs a secure context for `crypto.subtle`, and share links carry the key in the URL fragment, so HTTPS is mandatory in production (localhost is the only exception).

Requirements for the proxy:

1. **Pass the real client IP** via `X-Forwarded-For`, and set `CINDER_TRUSTED_PROXIES` to the proxy's address or network. Without this, all users share the proxy's IP for rate limiting. Never trust a range that untrusted clients can reach, or they can spoof their IP.
2. **Pass `X-Forwarded-Proto: https`** so Cinder emits HSTS and computes the request origin correctly for the `Origin` check on consume. Preserve the `Host` header. If the public origin still differs, list it in `CINDER_ALLOWED_ORIGINS`.
3. **Limit the request body.** Base64 inflates ciphertext by about 4/3, so the default 64 KiB ciphertext needs about 88 KiB of body; use 128k with the default and scale if you raise `CINDER_MAX_CIPHERTEXT_BYTES` (max 1 MiB gives about 1.4 MiB, so 2m).
4. **Do not cache** anything. Cinder sends `Cache-Control: no-store`; don't override it.
5. **Do not log** request bodies or URLs with ids at the proxy if you can avoid it. Share links keep the key in the fragment, which never reaches the server, but ids still appear in `/s/{id}` paths.

With Docker Compose, find the proxy's source network for `CINDER_TRUSTED_PROXIES` (for example `docker network inspect <project>_default`) or give the network a fixed subnet.

### Caddy

```caddyfile
cinder.example.com {
	encode zstd gzip
	request_body {
		max_size 128KB
	}
	header Strict-Transport-Security "max-age=63072000; includeSubDomains"
	reverse_proxy 127.0.0.1:8080 {
		# Caddy sets X-Forwarded-For/Proto/Host by default.
		# Do not trust upstream X-Forwarded-* from other hops unless configured.
	}
}
```

Caddy obtains and renews certificates automatically. Set `CINDER_TRUSTED_PROXIES=127.0.0.1` (or the Docker gateway address if Caddy runs on the host and Cinder in a container, commonly `172.17.0.1` or your compose network gateway).

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name cinder.example.com;

    ssl_certificate     /etc/letsencrypt/live/cinder.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cinder.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;

    client_max_body_size 128k;
    access_log off;   # or a format without $request_uri

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $remote_addr;   # overwrite, do not append client-supplied values
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
    }
}

server {
    listen 80;
    server_name cinder.example.com;
    return 301 https://$host$request_uri;
}
```

Use `$remote_addr` (not `$proxy_add_x_forwarded_for`) when nginx is the edge, so clients cannot inject their own `X-Forwarded-For`. Set `CINDER_TRUSTED_PROXIES` to nginx's address.

### HSTS

Cinder sends `Strict-Transport-Security: max-age=63072000; includeSubDomains` itself, but only when it sees HTTPS (TLS or a trusted `X-Forwarded-Proto=https`). Setting it at the proxy as well, as above, is harmless and covers error responses the proxy generates. Don't add `preload` until you are sure every subdomain is HTTPS.

## Redis

Cinder stores `iv || ciphertext` under `cinder:s:{id}` and tombstones under `cinder:t:{id}`, both with TTLs (see [api.md](api.md#redis-key-schema)). Only ciphertext is stored, but treat it as sensitive anyway.

- **Never expose Redis publicly.** The compose file does not publish port 6379; only the `cinder` service reaches it on the internal network. If you run Redis elsewhere, use a private network and enable `requirepass`/ACLs and TLS (`rediss://`).
- **Run it as shipped: `redis-server /etc/redis/redis.conf`** with `./deploy/redis.conf` mounted read-only. That config is expected to disable persistence.
- **Persistence warning.** If you enable RDB snapshots or AOF, burned and expired secrets can linger in files, replicas and backups long after Cinder deleted them. The data is ciphertext, but the link key may be in chat history, so ciphertext should not be kept either. Keep persistence off, or accept and document the trade-off. The compose file defines no data volume for the same reason.
- **Observability commands leak data.** `MONITOR` streams every command including values, and `SLOWLOG GET` can retain command arguments (ciphertext and ids). Don't leave `MONITOR` running, don't ship it to logs, restrict who can run `MONITOR`/`SLOWLOG`/`DEBUG` (ACLs), and consider `CONFIG SET slowlog-max-len 0` or a high `slowlog-log-slower-than`.
- Replicas hold copies too. Avoid replication unless you need it.

## Backups

- **Do not back up Redis.** Backups defeat burn-on-read and expiry. Exclude the Redis container and any data directory from snapshots.
- **Restarting Redis (or the host) loses all secrets by design** when persistence is off. That is expected: unread secrets disappear, and recipients see "not found". Tell users that links are not durable. Plan restarts and upgrades accordingly (for example, off-hours, short-TTL secrets).
- **Only configuration needs backing up:** `docker-compose.yml`, `deploy/redis.conf`, proxy config and environment values.
- **Lost-response burn caveat.** Consume is atomic and there is no grace window: if the server burns a secret but the response is lost (network drop, proxy timeout, browser closed), the secret is gone and the recipient sees "already viewed". The creator must send a new secret. Keep proxy timeouts generous and don't retry `consume` automatically at the proxy layer (for example nginx `proxy_next_upstream`).

## Health checks

`GET /healthz` returns `200 {"status":"ok"}` when Redis answers `PING`, else `503`. The runtime image is distroless (no shell, curl or wget), so the compose file has no in-container healthcheck for `cinder`; probe `/healthz` from your proxy, orchestrator or monitoring. Redis has a `redis-cli ping` healthcheck and `cinder` waits for it to be healthy.

## Running without Docker

```sh
make build
CINDER_REDIS_URL=redis://127.0.0.1:6379/0 ./bin/cinder
```

Run it as an unprivileged user (for example under systemd with `NoNewPrivileges=true`, `ProtectSystem=strict`), behind a TLS proxy as above.
