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

### The overlay ships with this skill: `narrate.js`

`narrate.js` sits beside this file and is copied into the project along with it.
Require it from the demo script rather than writing another one — the last three
projects each re-derived their own Stage 2 from this page's prose, which is why
no two demos look alike.

It asserts nothing and holds no selectors, so it is the same file in every
project:

| | |
|---|---|
| `say(page, text, step)` | caption, held for `max(2200, words * 280)` ms — a fixed hold rushes long lines and stalls on short ones |
| `point` / `unpoint` | pulsing outline around an element's rect, **drawn** — a real click ring would move the cursor and the page under it |
| `clickSlowly` | scroll in, mark, beat, click: a cursor that arrives and clicks in one frame reads as a glitch |
| `typeSlowly` | per-key typing, then commit |
| `bringIntoView` | includes **horizontal** scroll (`inline: 'center'`), for a grid whose action sits past a phone's right edge |

The spinning ring in the caption bar is the compositor fix described above, not
decoration and not a loading indicator — it is what keeps a reading pause from
collapsing to no frames. Removing it silently breaks the pause *and* any audio
timed against it.

What stays per-project is the walk itself: the persona, the steps and the
selectors (`narrated-walkthrough.js`). Only the library is shared.

### Spoken narration, if you add it

`recordVideo` writes a **silent** track — voice is not a setting, it is a second
pipeline you build and mux in. It has been produced ad hoc in a session before,
which is the problem: re-improvised each time, it lands on a different voice, a
different pace and different levels, so the demo's sound quality is luck. Pin it.

Neither dependency is guaranteed present — both were **absent** from a fresh web
container, and `apt-get install ffmpeg` failed there against a stale package index
(404s on superseded `libva`/`mesa` versions) until `apt-get update` ran first.
Check for them before promising audio; `pip install piper-tts` plus one voice
`.onnx` + `.onnx.json` is the rest.

Three things decide whether the result sounds professional. All are measured, not
matters of taste:

1. **Normalize, or it clips.** Raw Piper output measured **-17.5 LUFS with a
   +0.0 dBTP true peak** — at full scale, so it crunches audibly the moment it is
   encoded to AAC for the video. Two-pass `loudnorm` (measure with
   `print_format=json`, then feed the measured values back) to `I=-16:TP=-1.5`
   brought the same clip to -16.2 LUFS / **-4.5 dBTP**. Resample to 48 kHz stereo
   at the same time: Piper emits 22.05 kHz mono, which is not what a video
   container wants.
2. **Set the pace explicitly and then verify it.** `--length-scale` controls
   speaking rate, but only scales cleanly with `--sentence-silence 0`
   (measured 0.5 → 2.26s, 1.0 → 3.39s, 2.0 → 5.41s on one sentence; an earlier
   run varying the flag alone moved the duration by 6% across the same range).
   Read each clip's real duration back with `ffprobe` rather than trusting the
   flag.
3. **Time the video from the audio, not the reverse.** Synthesize first, measure
   each clip, and hold the step for that long. This is also why the pulsing
   indicator above is load-bearing rather than cosmetic: if an idle pause
   collapses to almost no frames, a pre-rendered voice track drifts against the
   picture no matter how good the synthesis is. Confirm the recorded file's
   duration matches the script's wall-clock before adding audio at all.

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
