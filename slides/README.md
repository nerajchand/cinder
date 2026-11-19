# Workshop slides

Slidev deck for the 90-minute workshop "build an app with a coordinator agent and subagents", using Cinder as the worked example. It follows `docs/workshop/workshop-plan.md` (8 + 12 + 15 + 45 + 10 minutes) and the facts in `docs/workshop/timeline.md`. Facilitator timing and cues are in the speaker notes of each slide.

This folder is outside the Go build. It adds nothing to `go.mod`. Node 18+ is needed (built here with Node 26 and npm 11).

## Run

```bash
cd slides
npm install
npm run dev        # opens http://localhost:3030 ; press "p" for presenter mode with notes
```

## Build a static site

```bash
npm run build      # writes slides/dist (git-ignored); serve it with any static server
```

## Export to PDF

Export needs a browser driver:

```bash
npm i -D playwright-chromium
npm run export     # writes slides-export.pdf
```

## Adapt for another app

1. Replace `public/logo.svg` with your logo. The ember colours live in `style.css`.
2. Edit the "worked example" and "burn on read" slides (sequence diagram) for your app.
3. Replace the debrief "Mishap" slides with your own project's timeline: real asks, real outcomes, real gaps. Do not invent any.
4. Update the exercise brief, sub-schedule and done criteria to match your starter repo, and the check commands to your stack.
5. Keep the agenda timings and the plan-gate and verification slides.
