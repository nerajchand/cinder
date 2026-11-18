# AGENTS.md — Ways of Working

## 1. Roles

- The main agent is a **coordinator only**. It never researches, builds, edits code, or merges.
- The coordinator delegates all work to subagents (researcher, developer, reviewer, etc.) and reports results back to the user.

## 2. Planning

- For most work, the coordinator first produces a plan and shows the user the decisions and the documented intent.
- The coordinator waits for the user's approval before dispatching builders.
- Trivial tasks may skip this step.

## 3. Parallel work

- Every writing agent works in its own git worktree, for example:
  `git worktree add ../Cinder-worktrees/<task> -b <branch>`
- One writer per worktree. No two agents edit the same worktree.
- Read-only research agents may use the main checkout.
- Clean up worktrees after the task completes.

## 4. Merging

- Agents never merge.
- The user (or whoever they designate) merges.
- Agents prepare branches and report what is ready.

## 5. Open Questions

- ~~`/Users/nchand/Workspace/temp/Cinder` is currently **not a git repo**. Worktrees require `git init` plus an initial commit.~~ **Resolved 2026-11-18:** repo initialized on `main`.

## 6. Living document

- This file is updated as new ways-of-working information or user input arrives.
- When that happens, the coordinator dispatches an agent to append it here and add a Changelog entry.

## 7. Asking the user questions

- Whenever the coordinator needs a decision or clarification from the user, it must use the structured question tool (the ask-user-question tool), not free-text questions in chat.
- Max 4 questions per call, 2-4 options each, put the recommended option first.

## 8. Decisions log

Design decisions live in `docs/design.md`. MVP decisions:

- Server-assigned IDs (no AAD).
- No revoke in MVP.
- 64 KiB size limit.
- Plain JS frontend with no build step.
- Dark mode is the default theme.
- Secrets may contain markdown with a selectable code language; the link page shows a QR code.
- Language lives inside the encrypted payload as a JSON envelope `{lang, text}`; the 64 KiB limit applies to the whole envelope.
- Markdown is rendered with a sanitising DOM-construction renderer (no innerHTML); links limited to http/https/mailto with rel noopener noreferrer nofollow; images not rendered; copy copies raw source; preview uses the same pipeline.
- Markdown parser, syntax highlighter and QR generator are vendored static files: no CDN, no inline styles.
- QR is generated client-side only (never a third-party service) and hidden behind a Show QR button by default.
- Buttons on the create and reveal screens are smaller.
- Logo: padlock with glowing ember outline and keyhole flame (docs/logo.svg)

## 9. Recording decisions

- Decisions are recorded in docs/design.md and the AGENTS.md decisions log without asking the user for permission.

## 10. Thinking level

- Match reasoning effort to the work.
- Planning and big research = HIGH.
- Feature work (developers building features) = MEDIUM.
- Simple tasks and mockups = LOW.
- The coordinator states the intended effort level in each subagent prompt, because the Task tool has no effort parameter.

## 11. Task naming

- Tasks and agents are referred to by short meaningful names instead of T0-T7.
- Mapping: T0=scaffold, T1=store, T2=api, T3=middleware, T4=web, T5=deploy, T6=tests, T7=harden (final integration and hardening).
- Branch and worktree names follow the same names, e.g. `feat/scaffold`, `feat/store`, `feat/api`.

## 12. Merge policy

- Agents prepare branches.
- When a branch is ready and its tests pass, a merge agent merges it without asking the user (user instruction 2026-11-18).
- Section 11 is intentionally unused on this branch; it may be reserved by another in-flight branch.

## 13. Polling over sleep

- Agents (and the coordinator) should prefer polling (check a condition in a loop, or use await-style tools) over `sleep` commands.
- Use `sleep` only when absolutely necessary.

## Changelog

- 2026-11-18: Initial version.
- 2026-11-18: Repo initialized with git init on main; worktrees now available.
- 2026-11-18: Design docs authored on branch docs/design (worktree ../Cinder-worktrees/docs-design), pending user review/merge.
- 2026-11-18: Design decisions recorded (Go, self-hosted, Redis, no passphrase in MVP, tombstones, 7-day max TTL, no creator notification).
- 2026-11-18: Rule added: always ask the user questions via the structured ask tool. Design docs merged to main. MVP decisions: server-assigned IDs (no AAD), no revoke in MVP, 64 KiB size limit, plain JS frontend with no build step.
- 2026-11-18: UI mockups on branch docs/ui-mockups; dark mode is the default theme.
- 2026-11-18: mockups revised after user review: markdown/code-language input, QR code on link page, smaller tombstone image
- 2026-11-18: Rule added: decisions are recorded in docs/design.md and the AGENTS.md decisions log without asking the user. Recorded UI/content decisions (markdown envelope, sanitised rendering, vendored deps, client-side QR behind Show QR, dark default, smaller buttons).
- 2026-11-18: Reveal secret button on interstitial made compact
- 2026-11-18: Rule added: thinking level by task type (planning/research high, features medium, simple tasks/mockups low).
- 2026-11-18: T0 scaffold built on feat/scaffold, pending user merge.
- 2026-11-18: Task IDs T0-T7 replaced by short names: scaffold, store, api, middleware, web, deploy, tests, harden.
- 2026-11-18: store (memory + Redis) branch merged-ready.
- 2026-11-18: middleware (security headers, rate limit, body limit, logging, origin) branch merged-ready.
- 2026-11-18: api (create/status/consume/healthz) branch merged-ready.
- 2026-11-18: web (embedded static frontend) branch merged-ready.
- 2026-11-18: deploy (Dockerfile, compose, Redis config, CI) branch merged-ready.
- 2026-11-18: harden (final integration: Redis wiring, full middleware stack, integration + CSP tests) merged-ready.
- 2026-11-18: Rule added (section 12): when a branch is ready and tests pass, a merge agent merges it without asking the user.
- 2026-11-18: Rule added (section 13): prefer polling over sleep.
- 2026-11-18: Makefile docker targets added.
- 2026-11-18: Logo mockups on branch docs/logo-mockups
- 2026-11-18: Padlock-flame logo variants added
- 2026-11-18: README rewritten (docs/readme branch): features, flow diagram, config table, security model, testing, layout.
- 2026-11-18: Logo 09-padlock-ember-flame chosen; added to top of README
- 2026-11-18: dockerize: Dockerfile on golang:1.26, `cinder healthcheck` subcommand + HEALTHCHECK/compose healthcheck, redis.conf reachable on compose network, CINDER_HOST_PORT override; verified end to end with docker compose.
- 2026-11-18: api-usable: verified curl create/status/consume flow (memory + Redis); added scripts/cinder-cli.mjs (browser-compatible AES-256-GCM CLI), "Using the API with curl" docs, CLI integration and interop tests; fixed docker-compose Redis bind (Redis was unreachable from the cinder container).
