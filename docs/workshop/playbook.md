# Playbook: run a coordinator-and-subagents project

This is the repeatable process. You can follow it without the people who wrote it. It is written for a Go project with git, but the steps work for any language: replace the check commands in section 6.

Terms:

- **Coordinator:** the main agent (or you, acting as one). It plans and delegates. It does not write code.
- **Worker:** a subagent that does one task in its own worktree.
- **Worktree:** a second working directory of the same git repo, on its own branch.
- **Plan gate:** the point where a human approves the plan before any building starts.

Contents:

1. [Set up the repo and rules](#1-set-up-the-repo-and-rules)
2. [Planning gate](#2-planning-gate)
3. [Write a delegation prompt](#3-write-a-delegation-prompt)
4. [Parallel worktrees](#4-parallel-worktrees)
5. [Read the report](#5-read-the-report)
6. [Verification checklist](#6-verification-checklist)
7. [Merge procedure](#7-merge-procedure)
8. [Record decisions](#8-record-decisions)
9. [Cleanup](#9-cleanup)
10. [Show documents to people](#10-show-documents-to-people)
11. [Repeat with a different app idea](#11-repeat-with-a-different-app-idea)
12. [Cheat sheet](#12-cheat-sheet)

## 1. Set up the repo and rules

1. Make a repo with at least one commit on `main`. Worktrees need a commit.

   ```bash
   mkdir myapp && cd myapp
   git init -b main
   git commit --allow-empty -m "chore: initial commit"
   ```

2. Pick a folder for worktrees, next to the repo, outside it: `../myapp-worktrees/`. Use the full absolute path in every prompt.

3. Copy the template below into `AGENTS.md` in the repo root. Edit the names in `[brackets]`. Commit it.

4. Tell your coordinator agent to read `AGENTS.md` first, every time.

### AGENTS.md template

```markdown
# AGENTS.md: Ways of Working

## 1. Roles
- The main agent is a coordinator only. It never researches, builds, edits code or merges.
- The coordinator delegates all work to subagents (researcher, developer, reviewer) and reports results to the human.

## 2. Planning
- For most work, the coordinator first writes a plan and shows the human the decisions and the intent.
- The coordinator waits for approval before it starts builders.
- Trivial tasks may skip the plan.

## 3. Parallel work
- Every writing agent works in its own git worktree and branch:
  `git worktree add [ABSOLUTE_PATH]/[repo]-worktrees/[task] -b [type]/[task]`
- One writer per worktree. No two agents edit the same worktree.
- Read-only research agents may use the main checkout.
- Branch types: feat/, fix/, docs/. Task names are short words, for example store, api, web.
- Clean up worktrees after the task is done.

## 4. Merging
- Agents never push.
- Agents never merge, except the merge step in section 5.
- A branch is ready when its checks pass and the report lists what was and was not run.

## 5. Merge policy
- [CHOOSE ONE] The human merges. OR: a merge agent merges without asking when the branch is ready and tests pass.
- Merge by fast-forward after a rebase on main. Then delete the branch and remove the worktree.

## 6. Questions
- The coordinator asks the human questions with the structured question tool, not free text in chat.
- At most 4 questions per call. Put the recommended option first.

## 7. Effort level
- Planning and big research: HIGH.
- Feature work: MEDIUM.
- Simple tasks and mockups: LOW.
- The coordinator writes the effort level in each subagent prompt.

## 8. Verification
- Every report separates "ran" from "did not run".
- Before reporting a service as working, check what is listening on its port.
- Do not claim a check you did not run.

## 9. Todo list
- The coordinator keeps a todo list for every task. One item in progress at a time. Mark items done as soon as they finish.

## 10. Polling
- Prefer polling (a loop that checks a condition) over `sleep`.

## 11. Decisions log
- Decisions are recorded in this file (below) and in `docs/design.md`, without asking permission.

### Decisions
- (none yet)

## 12. Showing documents
- To show a markdown file to the human, run `mo --open <file>`. [Remove this if you do not use mo.]

## Changelog
- [YYYY-MM-DD]: Initial version.
```

Notes on the template:

- Every branch adds a Changelog line, so rebases will conflict there. Resolve by keeping all lines (see section 7).
- Add a rule only when something goes wrong. Keep the file short enough to read in two minutes.

## 2. Planning gate

Use this for any task that takes more than about 15 minutes of work or touches more than one area.

1. The coordinator writes a plan with high effort. The plan must have:
   - Goal in one sentence.
   - Tasks, each with a short name, a branch, and an owner (worker type).
   - Which files or folders each task owns. No file has two owners.
   - What is frozen first (for example an interface) so tasks can run in parallel.
   - Checks that prove each task is done.
   - Risks and open questions.
2. The human reads the plan and asks questions.
3. The human replies "approved", or changes something. Only then do workers start.
4. The coordinator records the decisions in the decisions log.

Plan review questions:

- Can each task finish without waiting for another task?
- Does any file have two owners?
- Is every "done" check a command someone can run?

## 3. Write a delegation prompt

A worker knows nothing about your chat. Give it everything it needs in one prompt. Use this template.

```text
Effort: [LOW | MEDIUM | HIGH].

CONTEXT
[Repo, absolute path. What the app is. What was decided. Which docs to read first,
for example AGENTS.md and docs/design.md. What is frozen and must not change.]

TASK
[One clear goal. List the files this task owns. List what is out of scope.]

CONSTRAINTS
[Language and version. No new dependencies unless listed. Style rules.
Do not edit files outside the owned list. Do not change frozen interfaces.]

WORKTREE AND BRANCH
Work only in /absolute/path/myapp-worktrees/[task] on branch [type]/[task].
Create it with: git worktree add /absolute/path/myapp-worktrees/[task] -b [type]/[task]
Commit in small logical commits with clear messages.
Add one dated line to the AGENTS.md Changelog.

CHECKS TO RUN
gofmt -l .        (must print nothing)
go build ./...
go vet ./...
go test -race ./...
[Any task-specific check, and how to run the thing.]

REPORT FORMAT
Reply with these sections, in this order:
1. Branch and worktree path.
2. Commits (hash and subject).
3. Files changed.
4. What I ran: each command and its result (pass or fail).
5. What I did NOT run or could not verify.
6. Problems, decisions and anything you want the coordinator to know.

DO NOT push. DO NOT merge. Do not delete other branches or worktrees.
Leave the worktree in place when you finish.
```

Prompt tips:

- Put the effort level on the first line.
- Say "do not" things in plain words.
- One task per prompt. If it has more than one goal, split it.
- For a read-only research task, replace the worktree section with "Read only. Do not edit, commit or merge."
- Say how long it should take, so the worker does not wander.

## 4. Parallel worktrees

Run these from the main repo folder. Use absolute paths.

```bash
BASE=/absolute/path/myapp-worktrees

# One worktree and branch per writing task
git worktree add $BASE/store -b feat/store
git worktree add $BASE/api   -b feat/api
git worktree add $BASE/web   -b feat/web

# See all worktrees
git worktree list

# See where each branch is compared with main
git log --oneline main..feat/store
```

Rules:

- One writer per worktree.
- Create worktrees from an up-to-date `main`.
- Freeze shared interfaces and commit them to `main` first. Then create the worktrees.
- Do not check out the same branch in two worktrees (git refuses).
- If an agent made a worktree in the wrong place, run `git worktree list` and move or remove it (section 9).

## 5. Read the report

When a worker finishes, read the report before you read the code.

- Does it have all six sections?
- Is the "did NOT run" section filled in honestly? An empty section is a warning.
- Do the commit hashes exist? (`git log --oneline [branch]`)
- Did it touch files outside the owned list? (`git diff --stat main...[branch]`)
- Did it push or merge? (`git remote -v`, `git branch -a`)

If the report is weak, send the worker back with a short, specific request. Do not accept "all tests pass" without the command output.

## 6. Verification checklist

Run these yourself for each branch. Do not rely on the report.

In the worktree:

```bash
cd /absolute/path/myapp-worktrees/[task]
gofmt -l .                # prints nothing
go build ./...
go vet ./...
go test -race -count=1 ./...
git status --short        # prints nothing (everything committed)
git diff --stat main...HEAD
```

Run the thing:

```bash
# 1. Is something already using the port?
lsof -nP -iTCP:8080 -sTCP:LISTEN

# 2. Start the app (in the background, log to a file)
go run ./cmd/myapp > /tmp/myapp.log 2>&1 &
echo $!                   # note the process id

# 3. Poll until it is up (no sleep): up to 30 tries
for i in $(seq 1 30); do curl -fsS http://127.0.0.1:8080/healthz && break; done

# 4. Try the main flow with curl. Check the status codes.

# 5. Check who answers the port. It must be the process you just started.
lsof -nP -iTCP:8080 -sTCP:LISTEN

# 6. Stop it
kill <process id>
lsof -nP -iTCP:8080 -sTCP:LISTEN   # prints nothing
```

Checklist:

- [ ] Format, build, vet and tests pass (commands above, run by you).
- [ ] Only the owned files changed.
- [ ] The app starts, and the main flow works with `curl`.
- [ ] The process that answers the port is the one you started. No old server is running.
- [ ] Use `127.0.0.1` and `localhost` both if the app listens on IPv6 (`localhost` may resolve to `::1` first).
- [ ] If you use Docker: rebuild the image after the code change (`docker compose up -d --build`) and check the container is `healthy`.
- [ ] Browser check done by a human for anything visual. Agents cannot confirm how a page looks to you. Hard-reload (favicons cache).
- [ ] The server you started is stopped.
- [ ] The report's "did NOT run" list is read, and each item is either checked or written down as unverified.

## 7. Merge procedure

Merge one branch at a time. Do this only when the checks in section 6 pass and the merge policy allows it. (If the human merges, the human runs these steps.)

```bash
cd /absolute/path/myapp                    # the main checkout, on main
git status --short                         # must print nothing
git checkout main

# 1. Rebase the branch on the latest main
git -C /absolute/path/myapp-worktrees/[task] rebase main

# 2. If AGENTS.md conflicts: keep every Changelog line from both sides.
#    Remove only the conflict markers. Then:
#      git add AGENTS.md && git rebase --continue

# 3. Re-run the checks in the worktree (section 6)

# 4. Fast-forward merge
git merge --ff-only [type]/[task]

# 5. Delete the branch and remove the worktree
git branch -d [type]/[task]
git worktree remove /absolute/path/myapp-worktrees/[task]
```

Order: merge the task that others depend on first (for example the store before the API). After each merge, re-run the tests on `main` and re-base the next branch.

Never `git push` as part of this process unless the human asks.

## 8. Record decisions

- Write each decision as one line in the Decisions list of `AGENTS.md`, and in the design doc if it affects the design. Do it without asking.
- Format: what was decided, and why in one phrase. Example: "Server assigns IDs (no client IDs): simpler and safer."
- Record rule changes the same way, and add a dated Changelog line.
- If a recorded cause or decision turns out to be wrong, add a new dated line that corrects it. Do not leave the wrong line alone.
- Read the new rule back to the human once.

## 9. Cleanup

```bash
# List what exists
git worktree list
git branch --list

# Remove a finished worktree and its merged branch
git worktree remove /absolute/path/myapp-worktrees/[task]
git branch -d [type]/[task]

# Remove records of worktrees whose folders were deleted by hand
git worktree prune

# Stop leftover servers and containers
lsof -nP -iTCP:8080 -sTCP:LISTEN
docker compose ps        # if you used Docker
```

Use `git branch -d` (not `-D`). It refuses to delete an unmerged branch, which protects you.

At the end, `git worktree list` shows only the main checkout.

## 10. Show documents to people

When a human should read a markdown file or folder, open it with `mo`:

```bash
mo --open docs/workshop/timeline.md
mo --open docs/workshop        # a whole folder
```

`mo` prints a local URL for each file. Give the URLs to the human. Do not paste long documents in chat. Do not use other openers unless the human asks.

## 11. Repeat with a different app idea

Use this to run the process again for a new app, for example a link shortener, a polling tool or a file drop. Cinder is only the example.

1. **Write the idea in three lines.** What it does, who uses it, what it must never do (for Cinder: never burn a secret on `GET`). Put this at the top of `docs/design.md`.
2. **Research first (HIGH effort, read-only).** One research worker writes `docs/design.md` with options, a recommendation, and an "Items to verify" list. State if it came from memory, not live sources.
3. **Decide.** The human answers the open questions. Record each answer in the Decisions list of `AGENTS.md`.
4. **Mock up the screens (LOW effort).** One worker makes static mockups. The human reviews them one by one. Revise once.
5. **Scaffold and freeze (MEDIUM).** One worker makes the module, the shared interface, the config and the API contract. Merge it to `main` before anything else starts. This is what lets the next step run in parallel.
6. **Split into tasks with one owner per file.** A good split has a data layer, an API layer, a web layer, middleware or cross-cutting code, and deploy. Write the file ownership into each prompt. Check for overlap: if two tasks must touch the same file or fix the same problem, give it to one task.
7. **Build in parallel (MEDIUM).** One worktree per task (section 4). Use the delegation prompt (section 3).
8. **Harden (MEDIUM).** One worker wires the pieces together and adds integration tests.
9. **Run it for real.** Check the port first (section 6). Start the app, use `curl`, stop the app.
10. **Add extras one at a time** (Docker, README, CLI, logo). Each one gets its own branch. Re-check docs against the code after the branches merge, because a doc written before a merge can make claims that are not true.
11. **Close.** Clean up (section 9), update the changelog, and list what was not verified.

Adapting the checks: replace the Go commands in section 6 with your stack's format, build, lint and test commands. Keep the port check, the "did NOT run" list and the human browser check.

## 12. Cheat sheet

One page. Print it.

**Roles**
- Coordinator: plans, delegates, reports. Never edits code, never merges.
- Worker: one task, one worktree, one branch.

**The loop**
1. Todo list first.
2. Plan (HIGH effort). Human approves. No approval, no building.
3. Freeze shared interfaces on `main`.
4. One worktree per writer. Absolute paths.
5. Send delegation prompts (effort on line 1).
6. Read each report. Check "did NOT run".
7. Verify with your own commands.
8. Merge one branch at a time (rebase, fast-forward).
9. Record decisions. Clean up.

**Delegation prompt: 7 parts**
Context, Task, Constraints, Worktree and branch, Checks, Report format, "Do not push, do not merge".

**Effort levels**
Plan or big research: HIGH. Feature work: MEDIUM. Simple task or mockup: LOW.

**Commands**
```bash
git worktree add <abs-path>/<task> -b feat/<task>
git worktree list
gofmt -l . && go build ./... && go vet ./... && go test -race ./...
lsof -nP -iTCP:8080 -sTCP:LISTEN
git -C <worktree> rebase main
git merge --ff-only feat/<task>
git branch -d feat/<task> && git worktree remove <abs-path>/<task>
mo --open <file>
```

**Always**
- Check what is listening on the port before and after you test.
- Poll, do not sleep.
- Keep all Changelog lines on conflict.
- Rebuild Docker after a code change.
- Ask: "What did you not check?"

**Never**
- Push or merge from a worker.
- Let two agents edit the same worktree or file.
- Accept "it works" without command output.
- Leave a dev server running.
- Write a cause in the log before you have checked it.
