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

## Changelog

- 2026-11-18: Initial version.
- 2026-11-18: Repo initialized with git init on main; worktrees now available.
- 2026-11-18: Design docs authored on branch docs/design (worktree ../Cinder-worktrees/docs-design), pending user review/merge.
- 2026-11-18: Design decisions recorded (Go, self-hosted, Redis, no passphrase in MVP, tombstones, 7-day max TTL, no creator notification).
- 2026-11-18: Rule added: always ask the user questions via the structured ask tool. Design docs merged to main. MVP decisions: server-assigned IDs (no AAD), no revoke in MVP, 64 KiB size limit, plain JS frontend with no build step.
