# Cinder

Cinder is a self-hosted, **burn-on-read** secret sharing tool. Paste a secret, get a link, send it. The first person to open and reveal it sees it once; after that it is gone.

- Go single binary with an embedded static frontend (no build step).
- Redis storage with TTLs: 5 minutes, 1 hour, 1 day (default) or 7 days.
- The server never sees plaintext (end-to-end encryption in the browser).
- Secrets can be markdown with a selectable code language, up to 64 KiB.

> Status: under active development. See [docs/design.md](docs/design.md) for what is decided and built.

## Quickstart (Docker Compose)

```sh
docker compose up --build
```

Then open <http://localhost:8080>. Compose starts Cinder and Redis 7; Redis is not published to the host and has no data volume. The Redis config is mounted from `./deploy/redis.conf`.

Plain HTTP works on `localhost` only. For anything else, put a TLS reverse proxy in front (the browser needs HTTPS for WebCrypto). See [docs/operations.md](docs/operations.md) for Caddy and nginx examples and all `CINDER_*` settings.

## How it works

**End-to-end encryption.** The browser generates a random AES-256-GCM key and IV, encrypts the secret (a small `{lang, text}` envelope), and sends only ciphertext and IV to the server. The link is `https://host/s/<id>#<key>`. The key lives in the URL fragment, which browsers never send to the server, and the page scrubs it from the address bar after reading it.

**Burn on read.** Opening `/s/<id>` serves a static page that never touches storage, so link scanners and unfurlers cannot burn a secret. Only an explicit click on **Reveal** sends `POST /api/secrets/<id>/consume`, which reads and deletes the secret in one atomic Redis step. A minimal tombstone is kept until the original expiry, so "already viewed" is distinguishable from "expired". A non-destructive `GET /api/secrets/<id>/status` is also available. See [docs/api.md](docs/api.md).

## Security model and limits

- The server, its logs, and the Redis database hold only ciphertext. Whoever has the **full link** (including `#key`) can read the secret, once.
- Share the link over a channel you trust; anyone who sees it can burn or read it. "Already viewed" can signal interception.
- Not protected: a compromised recipient device, a recipient copying the secret, or a malicious operator serving altered JavaScript (self-host and review what you run).
- Limits: 64 KiB per secret (configurable up to 1 MiB), max TTL 7 days, per-IP rate limiting, strict CSP and `no-store` caching.
- No revoke, passphrase, or creator notification in the MVP.
- If a response is lost after the burn, the secret is gone; send a new one.
- Redis persistence should stay off, and Redis must never be exposed publicly. See [docs/operations.md](docs/operations.md).

## Development

Requires Go 1.22+.

```sh
make build   # go build -o bin/cinder ./cmd/cinder
make test    # go test ./...
make vet     # go vet ./...
make run     # go run ./cmd/cinder
```

Configuration is via `CINDER_*` environment variables (see [docs/operations.md](docs/operations.md#configuration-cinder_-environment-variables)); a local Redis is needed at `redis://localhost:6379/0` by default.

## Docs

- [docs/design.md](docs/design.md): decisions, threat model, encryption and UX design
- [docs/api.md](docs/api.md): HTTP API contract, headers, Redis schema
- [docs/operations.md](docs/operations.md): deployment, TLS, Redis, backups, configuration
