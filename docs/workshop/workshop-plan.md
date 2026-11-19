# Workshop plan: build a real app with a coordinator agent and parallel subagents

Length: 90 minutes. Format: short talk, one live demo, one hands-on exercise in pairs, one debrief.

This plan comes from the Cinder exercise. See [timeline.md](timeline.md) for what happened, and [playbook.md](playbook.md) for the process people will practise. Keep the scope small: people should leave with one working habit, not a finished app.

## 1. Audience and prerequisites

Audience: developers who already write code and have used an AI coding assistant at least once. Group size: 6 to 16 people, in pairs.

Prerequisites (send these a week before):

- Comfortable with a terminal and git (`commit`, `branch`).
- Go 1.22 or newer installed (the starter project is Go; see Open questions if the group uses another language).
- Git 2.30 or newer (for `git worktree`).
- An AI coding tool that can start subagents, with a working account. This plan assumes Cursor. Any tool that can run several agents on separate directories will work.
- A laptop with a free port 8080 (see the pre-flight check).

No Docker, Redis or Node is needed for the workshop project.

## 2. Learning objectives

After the workshop, each person can:

1. Explain the difference between a coordinator agent and worker agents, and why the coordinator does not write code.
2. Use one git worktree per writing agent, so agents never edit the same files.
3. Hold a plan gate: read the plan, then approve it, before any code is written.
4. Write a delegation prompt with context, task, constraints, effort level, branch name, checks and report format.
5. Verify an agent's report with their own commands, and name what the agent did not check.
6. Merge a finished branch with a short, repeatable procedure.

## 3. Materials and setup checklist

Facilitator prepares (one week before):

- [ ] Starter repo `cinder-mini` published in a place everyone can clone (see section 5). Tag it `start`.
- [ ] A solution branch `solution` (hidden from learners until the debrief).
- [ ] `AGENTS.md` template in the starter repo (from [playbook.md](playbook.md), section 1).
- [ ] Slides: about 12, one per concept in section 4. Keep text short.
- [ ] Handouts printed or shared (section 12).
- [ ] A spare laptop that has everything installed, in case a learner's setup fails.
- [ ] Wi-Fi tested with 20 devices cloning one repo at the same time (or a USB stick copy of the repo).

Learner prepares:

- [ ] Clone the starter repo. Run the pre-flight check below.
- [ ] Sign in to the AI tool and confirm it can start a subagent.

### Pre-flight check

Each learner runs this in the starter repo. All lines must pass. The facilitator asks for a screenshot or a "green" in chat.

```bash
git --version                      # 2.30 or newer
go version                         # go1.22 or newer
git worktree list                  # shows the main checkout only
go build ./... && go test ./...    # build works; some starter tests fail on purpose
lsof -nP -iTCP:8080 -sTCP:LISTEN   # prints nothing. If it prints a process, stop it or use another port
```

Expected: the build passes, and the tests fail only in the three packages the exercise will fix.

## 4. Agenda (90 minutes)

| Minutes | Time | Block | What happens |
|---|---|---|---|
| 8 | 00:00 to 00:08 | Welcome and pre-flight | Goals, agenda, ground rules. Pairs form. Quick pre-flight check for late arrivals. |
| 12 | 00:08 to 00:20 | Concepts | Six short ideas, 2 minutes each (see below). |
| 15 | 00:20 to 00:35 | Guided demo | Facilitator runs one full cycle on a tiny task, live. |
| 45 | 00:35 to 01:20 | Hands-on in pairs | Plan, delegate, verify, merge (see section 5 for the sub-schedule). |
| 10 | 01:20 to 01:30 | Debrief | Share results, compare with the solution, collect lessons, handouts. |

### Minute-by-minute run sheet

| Clock | Facilitator does |
|---|---|
| 00:00 | Welcome. State goals and the rule: "Show me the command you ran." |
| 00:03 | Form pairs. Pairs run the pre-flight check. |
| 00:08 | Concept 1: coordinator vs workers. |
| 00:10 | Concept 2: worktrees. |
| 00:12 | Concept 3: plan first. |
| 00:14 | Concept 4: delegation prompts and effort levels (3 min). |
| 00:17 | Concept 5: merge policy. |
| 00:19 | Concept 6: trust but verify. |
| 00:20 | Demo: worktree and branch. |
| 00:21 | Demo: plan and approval. |
| 00:24 | Demo: delegation prompt. |
| 00:27 | Demo: read the report, find one unchecked item. |
| 00:30 | Demo: verification checklist. |
| 00:33 | Demo: merge and clean up. |
| 00:35 | Exercise starts: read the brief, make the todo list. |
| 00:40 | Plan gate. Walk the room and check each plan. |
| 00:50 | Pairs delegate and start the workers. Call time every 5 minutes. |
| 01:05 | Verify and merge. |
| 01:15 | Run and check with `curl`. Check port 8080. |
| 01:20 | Debrief: each pair names one unchecked claim. Reveal the `solution` branch. |
| 01:27 | Feedback form (3 questions). |
| 01:30 | End. |

### Concepts (12 minutes)

1. **Coordinator vs workers (2 min).** The coordinator plans, delegates, reports and talks to the human. Workers research, build and review. The coordinator never edits code. Reason: it keeps the plan and the work separate, so the human can check each one.
2. **Worktrees (2 min).** One writing agent, one worktree, one branch. Show `git worktree add`. Reason: parallel agents cannot overwrite each other.
3. **Plan first (2 min).** The coordinator shows the plan and the decisions. The human approves. Only then do builders start. Small tasks may skip this.
4. **Delegation prompts and effort levels (3 min).** Walk through the prompt template. Planning gets high effort. Feature work gets medium. Simple tasks get low. The tool may have no effort setting, so write the level in the prompt.
5. **Merge policy (2 min).** Agents never merge. A merge step runs only when tests pass. Show the procedure. Decisions go into a log in the repo.
6. **Trust but verify (1 min).** Reports can be wrong. You run the checks that matter, and you check what is really listening on the port.

### Guided demo (15 minutes)

Use a task unrelated to the exercise, for example: "add a `/version` endpoint". Show these steps, with the todo list visible:

1. Create worktree and branch (1 min).
2. Coordinator writes the plan; facilitator approves it (3 min).
3. Coordinator sends a delegation prompt to one worker (3 min).
4. Facilitator reads the report and finds one thing the worker did not check (3 min).
5. Facilitator runs the verification checklist (3 min).
6. Merge and clean up (2 min).

Rehearse this once. Have the finished result ready in case the live run is slow.

## 5. Hands-on project (45 minutes)

### Project: "cinder-mini"

A tiny slice of Cinder: a burn-on-read secret store, without encryption, Redis, UI polish or Docker. It is a Go standard-library server of about 150 lines. The starter repo already has:

- `main.go`: starts the server on `:8080`.
- `store/store.go`: the `Store` interface (frozen: pairs must not change it) and an in-memory skeleton with `TODO`s.
- `api/handlers.go`: handlers for `POST /secrets`, `GET /secrets/{id}/status`, `POST /secrets/{id}/consume`, with `TODO`s.
- `web/index.html`: a bare page with one `TODO`.
- Tests that fail on purpose, one set per task.
- `AGENTS.md` with the ways of working, and an empty Decisions log and Changelog.

### Three parallel tasks

Each pair acts as the coordinator. The pair dispatches three workers, one per task. Each worker gets its own worktree and branch.

| Task name | Branch | Goal | Done when |
|---|---|---|---|
| `store` | `feat/store` | Make `Consume` atomic: the first call returns the secret, every later call returns `ErrAlreadyViewed`. Add TTL expiry. | `go test ./store` passes (includes a concurrent test with `-race`). |
| `api` | `feat/api` | Finish the three handlers. Consume only works with `POST`. `GET` must never burn a secret. | `go test ./api` passes. |
| `web` | `feat/web` | A page with a text box and a button that creates a secret and shows the link. | The page loads from the server and the create call returns an id (checked by hand with `curl`). |

Design notes for the facilitator:

- `store` and `api` depend on the frozen interface only, so they can run in parallel.
- `web` is the "easy" task. It gives pairs a safe win.
- All three tasks append one line to the `AGENTS.md` Changelog. This causes a real merge conflict on purpose. Pairs learn the "keep all lines" rule (see failure modes).

### Sub-schedule (45 minutes)

| Minutes | Step | Output |
|---|---|---|
| 5 | Read the brief and the starter `AGENTS.md`. Create the todo list. | Todo list with at least 5 items. |
| 10 | **Plan gate.** The coordinator agent writes a plan for the three tasks. The pair reads it, changes one thing, and approves it. A facilitator checks each plan. | Approved plan, with effort levels. |
| 15 | Delegate. Pair writes three delegation prompts using the template and starts the workers in three worktrees. While they run, pair reads the diffs. | Three branches with commits. |
| 10 | Verify and merge. Pair runs the verification checklist for each branch, then merges one by one. Resolve the changelog conflict. | `main` has all three branches, `go test ./...` passes. |
| 5 | Run and check. Start the server, create and consume a secret with `curl`, and check that a second consume fails. Check what listens on port 8080. | Written proof (paste the `curl` output). |

Stretch goal for fast pairs: add a `review` task, where a fourth agent reviews the combined diff and reports risks.

## 6. Facilitator notes

- Keep the talk short. People learn from the exercise, not the slides.
- Walk the room during the plan gate. A wrong plan is cheaper to fix than wrong code.
- Say the rule aloud: "The agent says it works. Show me the command you ran."
- Ask each pair to name one thing their agent did not check.
- Do not fix things for pairs. Point to the playbook section instead.
- Keep a visible clock. Announce each sub-schedule step.
- Pair "driver" and "navigator" switch roles at the plan gate and at verify time.
- Run the whole exercise yourself once, with a stopwatch, before the day.

## 7. Common failure modes and how to handle them

| Problem | Why it happens | What to do |
|---|---|---|
| **Stale process on port 8080.** The browser shows old behaviour, or a new server cannot start. | An earlier `go run` is still running. In the Cinder exercise this hid a correct Docker build for about an hour. | Run `lsof -nP -iTCP:8080 -sTCP:LISTEN`. Stop the process. Use `127.0.0.1` and the same port in `curl`. Put "stop your server" in the done criteria. |
| **Port clash between pairs on a shared machine.** | Two servers want 8080. | Give each pair a port (`PORT=8081`, ...). Write it in the handout. |
| **Merge conflict in the shared changelog.** | Every branch appends a line to `AGENTS.md`. | Keep every line from both sides. Remove only the conflict markers. Run the build after. |
| **Agent claims success without running anything.** | Reports are written from the plan, not from results. | Ask for the exact commands and their output. Re-run the check. Require the report format in the playbook ("ran" and "did not run" lists). |
| **Two agents edit the same file.** | A task was not split cleanly. | Stop one agent. Move the shared piece into the frozen interface. Give one owner per file. |
| **Agent merges or pushes on its own.** | The prompt did not forbid it. | Put "do not push, do not merge" in every prompt. Check `git log` and `git remote -v`. |
| **Worktree in the wrong place.** | The prompt gave a relative or vague path. | Always give the absolute path. Run `git worktree list`. |
| **Tool or quota failure for one learner.** | Account or rate limit. | Pair them with a neighbour. Use the spare laptop. |
| **Pair finishes early.** | Fast agents. | Give the stretch goal (review task). |
| **Pair is far behind.** | Slow setup. | Use the contingencies in section 9. |

## 8. Assessment and success criteria

No grades. Each pair self-checks against this list, and the facilitator spot-checks.

Must have (pass):

- [ ] The coordinator did not edit code. All commits come from worker branches.
- [ ] Three worktrees were used, one per task.
- [ ] A plan was approved before the workers started.
- [ ] Each delegation prompt has all seven parts of the template.
- [ ] `gofmt -l .` prints nothing, and `go build ./...`, `go vet ./...`, `go test -race ./...` pass on `main` after the merges.
- [ ] A consumed secret cannot be read again. A `GET` never burns a secret.
- [ ] The pair can name one thing an agent claimed that they checked, and one thing they did not check.

Nice to have:

- [ ] A decision was written in the Decisions log.
- [ ] A review task was run.
- [ ] Worktrees and branches were cleaned up.

Learner feedback (3 questions, on paper, 2 minutes): What will you use on Monday? What was unclear? What would you drop?

## 9. Timing contingencies

If you are running late, cut in this order:

1. Shorten the concepts block to 8 minutes (drop effort-level detail; it is in the handout).
2. Shorten the guided demo to 10 minutes: skip the merge step and show it in the debrief.
3. Reduce the project to two tasks: drop `web`.
4. Skip the "run and check" step; the facilitator shows it on one screen.
5. Cut the debrief to 5 minutes; move feedback to a form.

Never cut: the plan gate, the verification step.

If you have extra time (up to 10 minutes): run the stretch review task, or compare the pairs' plans.

## 10. Remote and in-room options

| Topic | In-room | Remote |
|---|---|---|
| Pairs | Sit together, one laptop each. | Breakout rooms of 2. Share screens. |
| Facilitator help | Walk the room. | One facilitator per 5 pairs. Use a help queue in chat. |
| Starter repo | Clone, or USB copy. | Clone. Share a zip as a fallback. |
| Pre-flight | Check at the door. | Chat check 15 minutes before. |
| Pace | Visible clock on the wall. | Timer in the shared slides and chat reminders. |
| Debrief | Pairs show on a projector. | Each pair pastes its `git log --oneline` in chat. |
| Length | 90 minutes. | Add a 5-minute break at 00:35; total 95 minutes. |

## 11. Assumptions

- The AI tool supports subagents and git worktrees (or the learner can run `git worktree` by hand).
- Learners have working accounts and can use the tool for 90 minutes without hitting a limit.
- Go is acceptable as the exercise language.
- The facilitator has run the exercise once before.
- Agents may behave differently each time, so results will vary. The process matters more than the output.

## 12. Handout list

1. Cheat sheet (the last page of [playbook.md](playbook.md)).
2. Delegation prompt template (playbook, section 3).
3. Verification checklist (playbook, section 6).
4. `AGENTS.md` template (playbook, section 1).
5. Pre-flight check commands (section 3 of this plan).
6. Exercise brief: the three tasks and the done criteria (section 5).
7. Failure modes table (section 7).
8. Feedback form (3 questions).

## 12a. Repeatable-run checklist

Use this for each new run of the workshop.

Before (one week):
- [ ] Starter repo cloned fresh and tagged `start`; `solution` branch tested with `go test -race ./...`.
- [ ] Pre-flight commands run on a clean laptop.
- [ ] `AGENTS.md` template matches the current [playbook.md](playbook.md).
- [ ] Participants have the prerequisites email.
- [ ] Spare laptop and offline copy of the repo ready.

On the day:
- [ ] Port 8080 free on the facilitator laptop.
- [ ] Demo rehearsed within the last 24 hours.
- [ ] Clock visible. Handouts ready.

After:
- [ ] Collect the feedback forms and note timings that ran over.
- [ ] Update this plan and the playbook. Add one dated line to the `AGENTS.md` Changelog of the starter repo.
- [ ] Remove worktrees and branches from the facilitator machine (`git worktree list`).

## 13. Open questions for the user

1. Which AI tool will learners use? This plan assumes Cursor with subagents. If it is another tool, the demo and prompts need small changes.
2. Is Go the right language for the audience? A Python or TypeScript starter is possible but needs a new starter repo.
3. Who owns the starter repo `cinder-mini`, and where should it live? It does not exist yet. Someone must build it and test it (about half a day).
4. Who will facilitate? Do you want a second facilitator for groups above 10?
5. In-room, remote, or both? The remote option adds 5 minutes.
6. Should the workshop use the real Cinder repo for the demo, or only the small starter?
7. Is a 90-minute slot fixed, or can it grow to 2 hours? A longer slot would allow the review task and a security discussion.
8. Do learners need to keep their work? If yes, where should they push it? (Agents in this process never push.)
9. Should the thinking-level rule be MEDIUM or LOW for feature work? The user said "low" in the chat, and `AGENTS.md` says MEDIUM. The playbook currently uses MEDIUM.
