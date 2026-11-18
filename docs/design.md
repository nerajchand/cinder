# Cinder: Design Research and Architecture

> **Honesty note: please read first.**
> The research behind this document was done **from prior knowledge, not live web lookups**. No sources were fetched or verified while writing it. Version numbers, iteration counts, header values and product behaviours may be out of date or wrong. Treat the URLs in [References to check](#references-to-check) as *references to check*, **not verified citations**. Anything under [Items to verify](#items-to-verify) must be confirmed before it is relied on.

Cinder is a planned web tool for sharing secrets on a **burn-on-read** basis: the secret is destroyed once it has been viewed.

## Contents

1. [Items to verify](#items-to-verify)
2. [Threat model and goals](#threat-model-and-goals)
3. [Encryption design](#encryption-design)
4. [Burn-on-read correctness](#burn-on-read-correctness)
5. [Expiry, storage and size](#expiry-storage-and-size)
6. [API and data model](#api-and-data-model)
7. [UX](#ux)
8. [Pitfalls and stack options](#pitfalls-and-stack-options)
9. [Testing and scope](#testing-and-scope)
10. [Open decisions for the product owner](#open-decisions-for-the-product-owner)
11. [References to check](#references-to-check)

## Items to verify

These claims come from memory and are **unverified**:

- [ ] Current OWASP Password Storage Cheat Sheet parameters: PBKDF2-HMAC-SHA256 about 600,000 iterations; Argon2id minimums (for example m=19 MiB, t=2, p=1).
- [ ] Redis `GETDEL` exists from Redis 6.2.
- [ ] SQLite `RETURNING` exists from 3.35. PostgreSQL `DELETE ... RETURNING` behaviour and isolation.
- [ ] WebCrypto AES-GCM: 96-bit IV recommended, 128-bit tag default, and the `additionalData` parameter.
- [ ] `crypto.subtle` is only available in secure contexts (HTTPS or localhost).
- [ ] Argon2 WASM libraries (`hash-wasm`, `argon2-browser`): maintenance status and size.
- [ ] In-app browsers (Slack, Teams, mobile mail clients) may strip or mangle URL fragments.
- [ ] Link scanners and unfurlers (Slack, Teams, Outlook Safe Links, email gateways) issue GET requests, and some execute JavaScript.
- [ ] Vault response-wrapping and cubbyhole semantics as described below.
- [ ] Header values (HSTS preload requirements, COOP behaviour, CSP directives) against current browser behaviour.
- [ ] Behaviour of OneTimeSecret, Snappass, Yopass, PrivateBin and Bitwarden Send as characterised here.
- [ ] `history.replaceState` removes the fragment from session history and from browser history sync.

## Threat model and goals

### Adversaries

| Adversary | Concern |
|---|---|
| Honest-but-curious or compromised server operator | Reads stored data, logs and live traffic |
| Database or backup leak | Stored records exposed, including old snapshots |
| Network MITM | Intercepts or alters traffic |
| Link scanners, unfurlers, prefetchers (Slack, Teams, email gateways) | Issue GET/HEAD requests and may burn the secret before the human does |
| Link interception in chat | Link is read by a third party, or a past message is searched later |
| Shoulder-surfing | Secret visible on screen |
| Browser-side risks | Extensions, history, `Referer`, XSS |
| Abusers | Use Cinder for spam, malware or storage, or enumerate IDs |

### Out of scope

- A compromised recipient endpoint.
- The recipient copying the secret.
- A malicious operator serving modified JavaScript. Mitigate partly with SRI, self-hosting and a CLI. This is not fully solvable in a web app.

### Goals

- The server **never sees plaintext**.
- A secret is revealable at most N times (default 1).
- Secrets expire.
- Bots cannot burn a secret.
- Nothing sensitive appears in logs.

## Encryption design

### Recommended default: client-side end-to-end

1. The browser generates a 256-bit AES-GCM key with WebCrypto, and a fresh random 96-bit IV for every encryption.
2. It sends `{ciphertext, iv, ttl, maxViews}` to the server.
3. The share link is `https://host/s/<id>#<base64url(key)>`.
4. The fragment is never sent to the server.
5. After reading the key, the page scrubs the fragment with `history.replaceState`.
6. Bind the id and metadata as AES-GCM **AAD** (additional authenticated data).
7. Use base64url encoding throughout.
8. A secure context (HTTPS) is required for `crypto.subtle`.
9. Optional later: pad plaintext to hide its length.

```js
// sketch only
const key = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, ['encrypt', 'decrypt']);
const iv = crypto.getRandomValues(new Uint8Array(12));
const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: aad }, key, plaintextBytes);
```

### Optional passphrase

- KDF choices: **Argon2id** is preferred but needs WASM (for example `hash-wasm` or `argon2-browser`). **PBKDF2-HMAC-SHA256** is native, with roughly 600k iterations *[verify OWASP numbers]*. **scrypt** is an alternative.
- Recommended design: **combine** the random fragment key with a passphrase-derived key, using a per-secret salt.
- A wrong passphrase shows up as an AES-GCM authentication failure.
- The server stores **no passphrase hash**.
- Limitation: someone holding both the DB and the link can brute-force a weak passphrase offline.
- **MVP: PBKDF2. Defer Argon2id.**

### Alternative: server-side encryption

OneTimeSecret (classic), Snappass (Fernet) and Vault do this. It is simpler and curl-friendly, but **the server sees plaintext**.

### Trade-offs

| Aspect | Client-side E2E (recommended) | Server-side encryption |
|---|---|---|
| Server sees plaintext | No | Yes |
| DB or backup leak | Ciphertext only, key is in the link | Depends on server key management |
| curl / API friendliness | Needs a client or CLI | Trivial |
| Needs JavaScript in browser | Yes | No |
| Operator serving malicious JS | Risk | Not applicable, but operator already sees everything |
| Complexity | Higher | Lower |

### Lesson from Vault cubbyhole and response wrapping

Provide a **non-destructive status check** plus an **atomic consume**. If an unwrap fails because the secret is already gone, that reveals possible interception.

## Burn-on-read correctness

### Atomic read-and-delete

- **Redis:** `GETDEL` (>= 6.2), or a Lua script for N views.
- **SQL:** `DELETE ... RETURNING`, or
  `UPDATE secrets SET views_left = views_left - 1 WHERE id = $1 AND views_left > 0 AND expires_at > now() RETURNING ...`
- **SQLite:** `RETURNING` needs >= 3.35.
- **Never** GET-then-DEL.

### Lost in transit

If the response is lost after the burn, the secret is gone. MVP has **no grace window**. The creator must resend.

### Bot safety

- `GET /s/<id>` serves a **static shell only**.
- Optional non-destructive `GET /api/secrets/<id>/status`.
- The user clicks **Reveal**, which sends `POST /api/secrets/<id>/consume`.
- GET, HEAD and OPTIONS never burn (RFC 9110 safe methods).
- `noindex`, and no `og:` metadata, so unfurlers show nothing useful.
- Optional PoW or CAPTCHA on consume.
- Check `Origin` on consume.
- `Cache-Control: no-store` everywhere.

## Expiry, storage and size

- **TTL presets:** 5m, 1h, 1d, 7d. Hard maximum 7d. Default 24h.
- Enforce `expires_at` **in the read query** and also run a cleanup job.
- **Max views:** default 1, capped around 10.

| Storage | Persistence risk |
|---|---|
| Redis | RDB and AOF files, replicas |
| SQL | WAL, free pages, backups retain remnants |

- **Secure deletion has limits.** Deleted data may linger in snapshots, WAL, free pages and backups. This is exactly why E2E matters: what lingers is ciphertext.
- **Size limits:** roughly 10 KB to 1 MB, enforced at both the reverse proxy and the app.
- Files are a later feature.

## API and data model

### Endpoints

| Method and path | Purpose |
|---|---|
| `POST /api/secrets` | Body `{ciphertext, iv, salt?, kdfParams?, passphraseProtected, ttlSeconds, maxViews}`. Returns `{id, expiresAt, deleteToken}` |
| `GET /api/secrets/{id}/status` | Non-destructive existence check |
| `POST /api/secrets/{id}/consume` | Atomic read and burn |
| `DELETE /api/secrets/{id}` | Revoke using `deleteToken` |
| `GET /s/{id}` | Static shell page |
| `GET /healthz` | Health check |

### Table `secrets`

| Column | Notes |
|---|---|
| `id` | PK, at least 128 bits from a CSPRNG, base64url |
| `ciphertext` | Encrypted blob |
| `iv` | Per-encryption IV |
| `salt` | NULL unless passphrase-protected |
| `kdf_params` | NULL unless passphrase-protected |
| `views_left` | Remaining views |
| `created_at` | Timestamp |
| `expires_at` | Timestamp |
| `delete_token_hash` | Hash of the delete token |

**No IPs or User-Agents are stored.**

### Anti-abuse

- IP rate limiting (token bucket), stricter on failed lookups.
- Identical 404 for unknown, expired and consumed secrets, **or** a deliberate minimal tombstone. **This is a decision to flag** (see [open decisions](#open-decisions-for-the-product-owner)).
- Optional CAPTCHA or PoW.

### Logging

- Never log bodies or ciphertext.
- Beware client-side error trackers capturing `location.hash`.
- Redact IDs in logs.

### Security headers

```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'
Referrer-Policy: no-referrer
Cache-Control: no-store
X-Content-Type-Options: nosniff
Cross-Origin-Opener-Policy: same-origin
X-Robots-Tag: noindex, nofollow
```

TLS 1.2 or newer only.

## UX

- **Interstitial** before reveal, so a bot or accidental visit does not burn it.
- Secret is masked with a show toggle, a copy button and auto-hide.
- Warn that closing the page loses the secret.
- Messaging: **expired** versus **already viewed**. This is the tombstone trade-off. "Already viewed" signals possible interception.
- Creator notification options: poll using `deleteToken`, email or webhook, or none in MVP.
- Nudge users to send the passphrase over a **separate channel**.
- Revoke support.
- Accessibility: keyboard, screen reader labels, contrast.
- In-app browsers may strip fragments *[verify]*.
- A clear message when JavaScript is disabled.

## Pitfalls and stack options

### Pitfalls

- Burning on GET.
- Non-atomic read and delete races.
- Fragment leaks: `Referer`, analytics, history sync, redirects.
- IV reuse.
- `Math.random` instead of CSPRNG.
- XSS: use `textContent`, never `innerHTML`.
- Redis persistence, and CDN caching of responses.
- Secrets in logs or WAF captures.
- Malicious JS and trust-on-first-use.
- Lost in transit.
- Clock skew.
- DoS via large payloads.
- Passphrase sent in the same channel as the link.
- Clipboard history.

### Stack options

1. **Node/TypeScript:** Fastify or Hono, with Redis, and a vanilla TS or Svelte frontend.
2. **Go single binary:** `net/http` plus chi, SQLite or Postgres, frontend via `go:embed`. (Yopass is written in Go.)

Either way the server is crypto-free. Keep the server dumb.

## Testing and scope

### Tests

- Crypto: round trip, tamper detection, IV uniqueness.
- Concurrency: 100 parallel consumes give exactly 1 success, and exactly K for `maxViews=K`.
- Time-mocked expiry.
- Bot safety: GET, HEAD and OPTIONS never burn.
- Headers check.
- Grep logs for secrets.
- Playwright E2E: fragment is stripped, and the secret is not in the DOM before the click.
- Fuzzing, ZAP, dependency audit.
- Failure injection.

### MVP versus later

| MVP | Later |
|---|---|
| Client-side AES-GCM, single view default, TTL presets, interstitial reveal, atomic consume, delete token, headers, rate limiting, optional PBKDF2 passphrase (pending decision) | Argon2id, multi-view, notifications, CLI/API, files, accounts, CAPTCHA, padding, signed builds |

## Open decisions for the product owner

1. **Stack:** Node/TypeScript or Go.
2. **Tombstones versus generic gone page:** better UX and interception signal, versus leaking existence.
3. **Passphrase in MVP?**
4. **Creator notification:** none, polling or email/webhook.
5. **TTL maximum:** 7d proposed.
6. **Self-hosted versus SaaS.**
7. **Storage engine:** Redis, SQLite or Postgres.

## References to check

These are **not verified citations**. They were recalled from memory and have not been opened.

- OneTimeSecret: https://github.com/onetimesecret/onetimesecret
- PrivateBin: https://privatebin.info
- Yopass: https://github.com/jhaals/yopass
- Bitwarden Send: https://bitwarden.com/help/about-send/
- Snappass: https://github.com/pinterest/snappass
- Vault response wrapping: https://developer.hashicorp.com/vault/docs/concepts/response-wrapping
- OWASP cheat sheets: https://cheatsheetseries.owasp.org
- Redis `GETDEL` command docs
- PostgreSQL `DELETE ... RETURNING` docs
- MDN SubtleCrypto
- NIST SP 800-38D (GCM)
- RFC 9110 (HTTP semantics)
- RFC 9106 (Argon2)
- RFC 8018 (PKCS #5 / PBKDF2)
