# Cinder API contract (frozen by T0)

Server is crypto-free: it stores and returns opaque `ciphertext` + `iv`. Language/markdown metadata lives **inside** the encrypted envelope `{lang, text}`; the server never sees it. IDs are server-assigned (no AAD binding).

## Conventions

- All binary values are **base64url without padding** (RFC 4648 §5, no `=`).
- `iv` decodes to **exactly 12 bytes**. `ciphertext` decodes to 1..`CINDER_MAX_CIPHERTEXT_BYTES` bytes (default 65536 = 64 KiB, hard cap 1 MiB; applies to the whole encrypted envelope incl. the 16-byte GCM tag). Request bodies are limited accordingly (base64 overhead plus small slack).
- `id`: 16 CSPRNG bytes, base64url, 22 chars (`internal/idgen`). Malformed ids get the same response as unknown ids (404 / `not_found`).
- `expiresAt`: RFC 3339 UTC timestamp.
- Request/response bodies are `application/json`. Unknown JSON fields are rejected (400).
- **Every `/api/*` response carries `Cache-Control: no-store`.**
- Error shape (all non-2xx API responses):

```json
{"error": {"code": "not_found", "message": "Secret not found or expired."}}
```

Error codes: `bad_request` (400), `bad_origin` (403), `not_found` (404), `method_not_allowed` (405), `already_viewed` (410), `payload_too_large` (413), `unsupported_media_type` (415), `rate_limited` (429, with `Retry-After`), `unavailable` (503, storage down), `internal` (500).

## Endpoints

### `POST /api/secrets`

Request (`Content-Type: application/json`):

```json
{"ciphertext": "<b64url>", "iv": "<b64url, 12 bytes>", "ttlSeconds": 86400}
```

- `ttlSeconds` must be one of the presets **300, 3600, 86400, 604800** (5 min, 1 h, 1 day, 7 days). Default in UI: 86400. Max TTL 7 days. Other values → 400.
- Response `201`: `{"id": "<id>", "expiresAt": "2026-11-19T03:00:00Z"}`
- Errors: 400, 413, 415, 429, 503. On `ErrIDCollision` the server regenerates the id and retries (bounded) before 500.

### `GET /api/secrets/{id}/status`

Non-destructive; never burns. Safe for bots.

- `200` `{"state": "available", "expiresAt": "..."}`
- `410` `already_viewed`, body is the standard error shape (tombstone present)
- `404` `not_found` (never existed, expired, or malformed id)
- 429, 503

### `POST /api/secrets/{id}/consume`

Atomic read-and-burn. Requirements:

- `Origin` header present and equal to the request origin (scheme+host as seen by the server, honouring trusted proxies) or in `CINDER_ALLOWED_ORIGINS`; otherwise `403 bad_origin`. Missing Origin is rejected.
- `Content-Type: application/json` (else 415) and body exactly `{}` (else 400).
- `200` `{"ciphertext": "<b64url>", "iv": "<b64url>", "expiresAt": "..."}`. The secret is destroyed in the same atomic step.
- `410 already_viewed`: tombstone exists.
- `404 not_found`: unknown, expired, or malformed id.
- `403 bad_origin`, `429 rate_limited`, `503 unavailable`.
- Any method other than POST → `405 method_not_allowed` with `Allow: POST`. GET/HEAD/OPTIONS never touch storage state.

### `GET /s/{id}`

Static HTML shell, **byte-identical for every id** (valid, unknown, consumed). Never touches storage. Contains no `og:` metadata. Reveal happens only after the user clicks, which issues `POST .../consume`. HEAD also served.

### `GET /healthz`

`200 {"status":"ok"}` when `Store.Ping` succeeds, else `503`. No secret data.

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

Expected failures: a missing or foreign `Origin` gives `403 bad_origin` (and does not burn the secret); consuming again gives `410 already_viewed`; unknown or expired ids give `404 not_found`. Behind a reverse proxy use the public URL as `Origin` (see `CINDER_ALLOWED_ORIGINS` in [operations.md](operations.md)). For real use, encrypt with Option A or any AES-256-GCM tool that follows the format above.

## Security headers

Applied to all responses:

```
Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'
Referrer-Policy: no-referrer
X-Content-Type-Options: nosniff
Cross-Origin-Opener-Policy: same-origin
X-Robots-Tag: noindex, nofollow
Strict-Transport-Security: max-age=63072000; includeSubDomains   # ONLY when request is HTTPS (TLS or trusted X-Forwarded-Proto=https)
```

Note: `img-src 'self'` (no `data:`) intentionally tightens `docs/design.md`; the QR code must be rendered with canvas/SVG elements served from `'self'`, not data URIs. No inline scripts or styles.

## Logging

Never log bodies, ciphertext, IVs, or full ids (redact ids to a short prefix at most). No IPs/User-Agents are stored.

## Redis key schema

| Key | Value | TTL |
|---|---|---|
| `cinder:s:{id}` | binary string: `iv (12 bytes) \|\| ciphertext` | `ttl` (PX), the secret's lifetime |
| `cinder:t:{id}` | tombstone: `"1"` (no content) | remaining original TTL when consumed |

Expiry is Redis key expiry; `expiresAt = now + PTTL`. A tombstone disappears at exactly the secret's original expiry.

### Create (Lua, atomic)

```lua
-- KEYS[1]=cinder:s:{id}  KEYS[2]=cinder:t:{id}  ARGV[1]=value  ARGV[2]=ttl_ms
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
  return 0   -- ErrIDCollision
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
```

### Status

`PTTL cinder:s:{id}` > 0 → Available (expiresAt = now+PTTL); else `PTTL cinder:t:{id}` > 0 → Consumed (expiresAt = now+PTTL); else Gone. (Two reads inside one Lua script or pipeline.)

### Consume (Lua, atomic; never GET-then-DEL outside a script)

```lua
-- KEYS[1]=cinder:s:{id}  KEYS[2]=cinder:t:{id}
local pttl = redis.call('PTTL', KEYS[1])
if pttl <= 0 then                       -- -2 missing, -1 no expiry (invalid)
  if redis.call('EXISTS', KEYS[2]) == 1 then return {'consumed'} end
  return {'notfound'}
end
local v = redis.call('GET', KEYS[1])
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], '1', 'PX', pttl)
return {'ok', v, pttl}
```

Mapping: `consumed` → `ErrConsumed`, `notfound` → `ErrNotFound`, `ok` → split `v` into IV (first 12 bytes) and ciphertext. Exactly one concurrent consume succeeds. Redis persistence (RDB/AOF) should be off or tightly controlled; replicas and backups also hold ciphertext copies.
