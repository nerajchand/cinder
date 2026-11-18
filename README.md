<p align="center"><img src="docs/logo.svg" alt="Cinder logo" width="160"></p>

# Cinder

Cinder is a self-hosted, **burn-on-read** secret sharing tool. Paste a secret, get a link, send it. The first person to open the link and click **Reveal** sees the secret once; after that it is gone.

## Features

- **Burn on read**: the secret is read and deleted in one atomic step. Link previews and scanners cannot burn it, because only an explicit **Reveal** click consumes it.
- **Client-side encryption**: AES-256-GCM in the browser (WebCrypto). The key lives in the URL fragment (`#key`), which browsers do not send to the server. The server only stores ciphertext.
- **Markdown and code**: secrets may be markdown with a selectable code language. Rendering uses a sanitising DOM-building renderer (no `innerHTML`, no images, links limited to http/https/mailto). Copy returns the raw source.
- **QR code**: generated client-side only (vendored library, no third-party service) and hidden behind a **Show QR code** button.
- **Dark mode by default**, with a theme toggle.
- **Tombstones**: after a secret is read, a content-free marker stays until the original expiry, so "already viewed" (possible interception) is distinguishable from "expired".
- **Limits**: TTL presets of 5 minutes, 1 hour, 1 day (default) or 7 days (max); 64 KiB per secret by default (the whole encrypted `{lang, text}` envelope, configurable up to 1 MiB).
- Single Go binary with an embedded static frontend (plain ES modules, no build step, no CDN, no inline scripts or styles).

## How it works

```mermaid
sequenceDiagram
    participant C as Creator browser
    participant S as Cinder server
    participant R as Redis
    participant V as Recipient browser

    C->>C: generate key + IV, encrypt {lang, text}
    C->>S: POST /api/secrets {ciphertext, iv, ttlSeconds}
    S->>R: store iv||ciphertext with TTL
    S-->>C: {id, expiresAt}
    C->>C: build link https://host/s/ID#KEY
    C-->>V: share link (out of band)
    V->>S: GET /s/ID (static page, no storage access)
    V->>V: user clicks Reveal
    V->>S: POST /api/secrets/ID/consume
    S->>R: atomic read + delete + write tombstone
    S-->>V: {ciphertext, iv}
    V->>V: decrypt with key from fragment
```

1. The browser generates a random AES-256-GCM key and IV, encrypts the envelope, and sends only ciphertext and IV. The server assigns the 22-character ID.
2. The link is `https://host/s/<id>#<key>`. The recipient page reads the key from the fragment and scrubs it from the address bar (`history.replaceState`).
3. `GET /s/<id>` returns the same static page for every ID and never touches storage.
4. `consume` is a Lua script in Redis that reads, deletes, and writes the tombstone atomically. Exactly one concurrent consume succeeds.
5. Later visits get `410 already_viewed` until the original expiry, then `404 not_found`.

## Quick start (Docker Compose)

```sh
docker compose up --build
```

Open <http://localhost:8080>. Compose starts Cinder and Redis 7. The Cinder port is published on `127.0.0.1` only; set `CINDER_HOST_PORT` to use another host port (default 8080), e.g. `CINDER_HOST_PORT=9090 docker compose up --build`. Redis is not published (`expose` only, no `ports`) and has no data volume; its config is mounted read-only from `deploy/redis.conf`. Cinder waits for Redis to be healthy before it starts.

**Redis bind note.** `deploy/redis.conf` sets `bind * -::*` and `protected-mode no` so the Cinder container can reach Redis over the private Compose network. That is only safe because Redis is never published and has no password. Do not reuse this file for a host-reachable Redis; bind to a private interface and set `requirepass` instead.

Makefile shortcuts:

```sh
make docker-build   # docker build -t cinder .   (IMAGE=name to override the tag)
make docker-run     # single container, in-memory store, -p 8080:8080
make docker-up      # docker compose up -d --build
make docker-down    # docker compose down -v
make docker-logs    # docker compose logs -f
make docker-ps      # docker compose ps
```

Plain HTTP works on `localhost` only. Anywhere else you need HTTPS (WebCrypto requires a secure context); see [Deployment](#deployment).

## Running locally

Requirements:

- **Go**: the version in `go.mod` (currently `go 1.26.0`); the Dockerfile builds with `golang:1.26`.
- **Node 22+**, for the frontend unit tests. The CLI (`scripts/cinder-cli.mjs`) needs Node 18+. No npm dependencies, no build step.
- **Redis 6.2+**, optional for local development.

```sh
make run      # go run ./cmd/cinder
make build    # go build -o bin/cinder ./cmd/cinder
make test     # go test ./...
make vet      # go vet ./...
```

**Store selection** (`cmd/cinder/main.go`):

- Cinder uses the **in-memory store** when `CINDER_REDIS_URL` is unset or empty, or when `CINDER_STORE=memory`. It logs a warning and all secrets are lost on restart. This is for development only.
- Otherwise it uses **Redis** at `CINDER_REDIS_URL`.

```sh
go run ./cmd/cinder                                        # memory store, :8080
CINDER_REDIS_URL=redis://localhost:6379/0 go run ./cmd/cinder   # Redis
```

## Configuration

All settings are environment variables. Invalid values make the process exit with an error (see `internal/config/config.go`).

| Variable | Default | Description |
|---|---|---|
| `CINDER_ADDR` | `:8080` | Listen address (`host:port` or `:port`). |
| `CINDER_REDIS_URL` | `redis://localhost:6379/0` | `redis://` or `rediss://` URL. If the variable is unset or empty, the in-memory store is used (see above). |
| `CINDER_STORE` | unset | Set to `memory` to force the in-memory store. |
| `CINDER_MAX_CIPHERTEXT_BYTES` | `65536` | Max ciphertext size, 1 to 1048576. Covers the whole envelope including the 16-byte GCM tag. |
| `CINDER_TRUSTED_PROXIES` | empty | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` / `X-Forwarded-Proto` are honoured. |
| `CINDER_ALLOWED_ORIGINS` | empty | Comma-separated extra `scheme://host[:port]` origins accepted on `consume`. |
| `CINDER_RATE_LIMIT_CREATE_PER_MIN` | `30` | Per-IP creates per minute; `0` disables. |
| `CINDER_RATE_LIMIT_CONSUME_PER_MIN` | `60` | Per-IP consumes per minute; `0` disables. |
| `CINDER_RATE_LIMIT_STATUS_PER_MIN` | `120` | Per-IP status checks per minute; `0` disables. |
| `CINDER_RATE_LIMIT_BURST` | `10` | Token bucket burst size (minimum 1). |

`CINDER_HOST_PORT` is not read by Cinder; it is a Docker Compose variable for the published host port (see Quick start).

Numeric rate-limit values must be at most 1000000.
## Using the API with curl

The server is zero-knowledge, so **encryption happens on your side**. Two ways to use it from a shell:

### Option A: the CLI (recommended)

`scripts/cinder-cli.mjs` (Node 18+, no dependencies) encrypts locally with AES-256-GCM, uses the same `{lang,text}` envelope, IV and key encoding as the browser, and prints a normal share link. Links work in both directions (CLI to browser and browser to CLI).

```sh
export CINDER_URL=http://localhost:8080    # or pass --url
echo -n 'hunter2' | scripts/cinder-cli.mjs create --ttl 3600 --lang text
# http://localhost:8080/s/<id>#<key>          (expiry goes to stderr)

scripts/cinder-cli.mjs fetch 'http://localhost:8080/s/<id>#<key>'   # burns it, prints plaintext
```

`create` reads `--file PATH`, a text argument, or stdin. `fetch` sends the required `Origin` header (taken from the link) and fails with `410 already_viewed` on a second run.

### Option B: raw ciphertext with curl

You must supply base64url (no padding) `ciphertext` (AES-256-GCM output with the 16-byte tag appended) and a 12-byte `iv`. The plaintext should be the JSON envelope `{"lang":"text","text":"..."}`; the key is never sent. Any ciphertext is accepted by the server, which is handy for testing the API shape:

```sh
B=http://localhost:8080
IV=$(head -c12 /dev/urandom | basenc --base64url | tr -d =)
CT=$(head -c32 /dev/urandom | basenc --base64url | tr -d =)   # stand-in for real ciphertext

# create -> 201 {"id":"...","expiresAt":"..."}
curl -s -X POST $B/api/secrets -H 'Content-Type: application/json' \
  -d "{\"ciphertext\":\"$CT\",\"iv\":\"$IV\",\"ttlSeconds\":3600}"

ID=<id from above>
# status (never burns) -> 200 {"state":"available",...}
curl -s $B/api/secrets/$ID/status

# consume (burns). Origin MUST equal the server's own origin (scheme://host[:port]),
# Content-Type must be application/json and the body exactly {}.
curl -s -X POST $B/api/secrets/$ID/consume \
  -H "Origin: $B" -H 'Content-Type: application/json' -d '{}'
# -> 200 {"ciphertext":"...","iv":"...","expiresAt":"..."}
```

Expected failures: a missing or foreign `Origin` gives `403 bad_origin` (and does not burn the secret); consuming again gives `410 already_viewed`; unknown or expired ids give `404 not_found`. Behind a reverse proxy use the public URL as `Origin` (see `CINDER_ALLOWED_ORIGINS` in [docs/operations.md](docs/operations.md)). For real use, encrypt with Option A or any AES-256-GCM tool that follows the format above.

Full contract: [docs/api.md](docs/api.md).

## API summary

Binary values are base64url without padding. Full contract: [docs/api.md](docs/api.md).

| Method and path | Purpose |
|---|---|
| `POST /api/secrets` | Create. Body `{ciphertext, iv, ttlSeconds}`; `ttlSeconds` is one of 300, 3600, 86400, 604800. Returns `201 {id, expiresAt}`. |
| `GET /api/secrets/{id}/status` | Non-destructive. `200 {state: "available", expiresAt}`, `410 already_viewed`, or `404 not_found`. |
| `POST /api/secrets/{id}/consume` | Atomic read and burn. Requires a matching `Origin` header and body `{}`. Returns `{ciphertext, iv, expiresAt}`. |
| `GET /s/{id}` | Static view page, identical for every ID, never touches storage. |
| `GET /healthz` | `200 {"status":"ok"}` if Redis answers `PING`, else `503`. |

Errors use `{"error": {"code", "message"}}`. All `/api/*` responses are `Cache-Control: no-store`.

## Security model

**What the design provides**

- The server, its logs, and Redis only ever see ciphertext, IV, and ID. The key is in the URL fragment and never sent to the server. Logs never record bodies, ciphertext, or full IDs.
- Whoever holds the **full link** can read the secret, once. "Already viewed" signals that someone else may have opened it first.
- Strict headers on all responses: CSP (`default-src 'none'; script-src 'self'; style-src 'self'; ...`), `Referrer-Policy: no-referrer`, `nosniff`, COOP, `X-Robots-Tag: noindex`, HSTS when served over HTTPS.
- `consume` checks `Origin`; per-IP rate limits and body limits are applied.

**Honest limitations**

- **A malicious or compromised operator can serve altered JavaScript** and capture secrets and keys. End-to-end encryption does not protect against the party that delivers the code. Self-host and review what you run.
- **A lost response burns the secret.** Consume is atomic with no grace window; if the response is lost after the burn, the recipient sees "already viewed" and the creator must send a new secret.
- **Redis persistence is off by design** (`deploy/redis.conf`), so restarting Redis loses all unread secrets. Enabling RDB/AOF, replicas, or backups can retain ciphertext after deletion.
- Anyone who sees the full link (chat history, shoulder surfing) can read it; a compromised recipient device or a recipient copying the secret is out of scope.
- Tombstones reveal that an ID once existed.
- No revoke, passphrase, or creator notification in the MVP.
- Security headers and rate limits are only as good as your proxy setup (see below).

More detail: [docs/design.md](docs/design.md).

## Deployment

Cinder serves plain HTTP. **Terminate TLS in a reverse proxy** (Caddy and nginx examples are in [docs/operations.md](docs/operations.md)). The proxy should:

- Pass the real client IP via `X-Forwarded-For`, and set `CINDER_TRUSTED_PROXIES` to the proxy's address or network. Never trust a range untrusted clients can reach.
- Pass `X-Forwarded-Proto: https` and preserve `Host`, so HSTS and the `Origin` check on `consume` work. If the public origin still differs, add it to `CINDER_ALLOWED_ORIGINS`.
- Limit request bodies (about 128k with the default ciphertext limit, since base64 inflates it by about 4/3), not cache responses, and not log request URLs if avoidable.
- Not retry `consume` requests automatically.

Redis: never expose it publicly, keep persistence off, and do not back it up.

Health: `GET /healthz` returns 200 when Redis answers `PING`. The runtime image is distroless (no shell, curl or wget), so the binary has a built-in probe: `cinder healthcheck` GETs `http://127.0.0.1:<port>/healthz` (port from `CINDER_ADDR`) and exits non-zero unless it gets 200. It backs both the Dockerfile `HEALTHCHECK` and the Compose healthcheck for Cinder; the Redis service uses `redis-cli ping`.

Container hardening (Cinder service in `docker-compose.yml` and `Dockerfile`): distroless `static-debian12:nonroot` base, runs as `nonroot`, `read_only: true`, `cap_drop: [ALL]`, `no-new-privileges`. The Redis service is not hardened this way; it relies on being unpublished and on the private Compose network.

## Testing

```sh
go test ./...                 # unit and integration tests (memory store)
node --test web/test/*.test.mjs   # frontend and CLI interop tests (Node 22+), from the repo root
CINDER_TEST_REDIS_URL=redis://localhost:6379/0 go test ./...   # also exercise real Redis
```

- Without `CINDER_TEST_REDIS_URL`, the Redis integration tests in `test/integration` are skipped; the store tests in `internal/store` skip if no Redis is found on `127.0.0.1:6379`.
- Real-browser behaviour (clipboard, canvas QR, theme toggle, CSP enforcement in a browser) is not covered by automated tests.
- The CLI round-trip test in `test/integration/cli_test.go` runs under `go test` and is skipped if `node` is not installed. `web/test/cli-interop.test.mjs` checks browser/CLI compatibility.
- CI (`.github/workflows/ci.yml`) runs `go vet ./...` and `go test ./...`. It does not run the Node tests or a Redis service.

## Project layout

```
cmd/cinder/          entrypoint, store selection, Redis wiring, `healthcheck` subcommand
scripts/cinder-cli.mjs  Node CLI to create and fetch secrets (see API section)
internal/api/        HTTP handlers and error shapes
internal/app/        handler assembly (middleware stack + routes + web)
internal/config/     CINDER_* parsing and validation
internal/idgen/      server-side ID generation
internal/middleware/ security headers, rate limit, body limit, origin, logging, proxy
internal/store/      Store interface, memory and Redis implementations
internal/web/dist/   embedded frontend (source of truth, no build step)
web/test/            Node tests for the frontend
test/integration/    end-to-end HTTP tests
deploy/redis.conf    Redis config used by Compose
docs/                design, API, operations, UI mockups
```

## Contributing

Work is coordinated as described in [AGENTS.md](AGENTS.md) (roles, worktrees, merge policy, decisions log). Design decisions are recorded in [docs/design.md](docs/design.md) and the AGENTS.md decisions log.

## Docs

- [docs/design.md](docs/design.md): decisions, threat model, encryption and UX design
- [docs/api.md](docs/api.md): HTTP API contract, headers, Redis schema
- [docs/operations.md](docs/operations.md): deployment, TLS, Redis, backups
- [web/README.md](web/README.md): frontend layout and tests
