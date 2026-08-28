---
name: record-narrated-demo
description: "Record a narrated walkthrough video of a working Mendix app — the human-facing half of the journey/demo pair. Use when asked to demo, show off, or produce a walkthrough recording, after the app's end-to-end journey already passes."
---

# Record a Narrated Demo

Proving a feature works and showing it off are two different jobs, and one script
cannot do both well. This skill covers the second. It assumes the first is done.

| | Stage 1 — journey | Stage 2 — demo |
|---|---|---|
| Script | `journey-runner.js` | `narrated-walkthrough.js` |
| Job | **asserts** | **explains** |
| Gates the build | yes | no |
| Optimised for | signal — no narration, no reading pauses | a viewer — human pace, on-screen narration |
| Verdict lives here | **yes** | **never** |

Both walk the same persona down the same path. The demo reuses the journey's
shape and its OQL-backed data checks, so what it shows on screen is still true —
but the PASS/FAIL judgment stays in the gating runner. A demo that can fail the
build is a test with worse ergonomics; a test that narrates is a demo that misses
regressions.

## Before recording: the journey must exist and pass

Write `journeys/<Module>.journey.json` first — one persona, one path, with
carried state, not a list of page stops. Its canonical definition is
**`skills/journey-proof.md` in `mxcli-project-toolkit`**; do not re-derive the
protocol here. In outline, each step asserts five independent rungs:

1. **Landing guard** — the step's `ready` widget is visible. Without it every
   later assertion silently runs against the *previous* page.
2. **What the screen says** — `textPresent` / `textAbsent`. The backend can be
   correct and the screen can still lie about it.
3. **Ordered spans** — the right microflows fired, in order, plus `mustNotFire`.
4. **Data effects** — row created, association actually set (not null), pointing
   at the *right* seeded value. Three claims, not one.
5. **Outcome** — one query over the final state. Per-step deltas can each be
   right while the net result is wrong.

Every rung is proved falsifiable by re-running with one broken precondition each
(7 mutants — rungs 3 and 4 make two claims apiece) and requiring the *targeted*
check to fail. A rung nobody could break is `UNPROVEN`, which is a **fault**, not
a pass. Verdicts are `PASS` / `FAIL` / `INVALID` and never collapse into each
other: `INVALID` means the instrument did not run, which is a finding of its own.

**Writing the journey first is what makes the demo cheap.** The persona, the
path, and the definition of success already exist by the time you record.

## Recording mechanics

Four things that decide whether the video is watchable. Each has a reason; none
is a style preference.

### Record at a human pace, not the harness's

A cursor moving at test speed reads as **broken**, not fast. The gating runner is
tuned for signal and should stay that way — slow the demo script down on its own,
and leave reading pauses where a viewer would actually need to read.

### Give the compositor something to draw during pauses

Playwright's video captures only frames the compositor actually produces. A
genuinely idle screen during a reading pause can collapse to almost no video, so
a 5-second pause plays back in a blink and the narration desyncs. Keep something
continuously animating — a small pulsing indicator is enough — so idle time is
recorded *as* idle time.

### Record the same walk at desktop and at a real mobile profile

Same steps, same assertions, **nothing simplified for the smaller screen**. A
pass that trims the small-screen steps would report success on an app nobody can
use on a phone. This is the pass that catches layout failures nothing static
sees — an mxcli-built app recorded at 414×896 completed the whole flow while both
DataGrid2 screens were unreadable: eight columns compressed to eight single
characters, headers degraded to bare sort arrows. The app *functioned* on a phone
and was not *usable* on one, and only the mobile recording showed the difference.

### `recordVideo` needs a Node script, not `playwright-cli`

`mxcli verify`'s browser checks run bash scripts against a persistent
`playwright-cli` session (see [test-app](../test-app/SKILL.md)). Video is a
**context-creation** option, so the demo script owns its own browser context via
the Playwright library instead. That is a second reason Stage 2 is a separate
script rather than a flag on the gating runner.

Browser and headless-shell setup is [test-app](../test-app/SKILL.md)'s — do not
duplicate it here. Data assertions under the demo use
[verify-with-oql](../verify-with-oql/SKILL.md). Boot the app with
[run-local](../run-local/SKILL.md) (`mxcli run --local`); for a still-image set
rather than a video, `--screenshot` with repeated `--screenshot-url` already does
that without any script.

## What to narrate

Narrate only what a viewer with **no build context** would understand.

Cut:

- anything that reads as *"here's proof the database is right"* — that is Stage 1,
  and to a viewer it is noise about a claim they were not disputing
- anything implying *"this used to be broken"* — the audience did not see the
  before, so it lands as an apology for a bug they never met

Keep it to the persona's own motivation: what they are trying to do, what they
see, and what changed for them.

## Checklist

- [ ] `journeys/<Module>.journey.json` exists and the journey run is `PASS`
- [ ] The positive-control run has shown every rung can go red — no `UNPROVEN`
- [ ] The demo is a **separate script**; no PASS/FAIL verdict lives in it
- [ ] Pace is human, with real reading pauses
- [ ] Something animates during pauses, so idle time survives into the video
- [ ] Recorded at **both** a desktop viewport and a real mobile device profile
- [ ] The mobile pass runs the same steps, with nothing simplified
- [ ] Narration mentions no database proof and no past bugs
- [ ] App was live and the database real — this instrument mocks nothing
