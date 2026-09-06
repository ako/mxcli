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

## Seed the data before you record

The failure this skill is most prone to, and it is not subtle. The first real
ContactBook capture was an empty grid reading `0 to 0 of 0` with a column header
rendering as `colActions` — which reads as **a broken app, not a new one**. The
journey passed; the app was live; the recording was worthless.

Before any take:

- **Populate every list the camera will see**, with plausible values — real names
  and amounts, never `test1` / `asdf` / `aaa`.
- **Caption every column and button the camera will see.** A header showing its
  attribute name is the single clearest tell that nobody looked at the screen.
- **Open on something already interesting.** The first thing on screen should be
  a populated state, not an empty one waiting to be filled.

`mxcli` seeds this itself — see [demo-data](../demo-data/SKILL.md). Seeding is
part of recording, not a nicety before it.

## Recording mechanics

Four things that decide whether the video is watchable. Each has a reason; none
is a style preference.

### Record at a human pace, not the harness's

A cursor moving at test speed reads as **broken**, not fast. The gating runner is
tuned for signal and should stay that way — slow the demo script down on its own,
and leave reading pauses where a viewer would actually need to read.

Two numbers, both from films that were re-cut for being too fast:

- **Hold every screen for `max(caption read time, screen read time)`, floor 2.5s.**
  Caption read time is roughly `words ÷ 3.5` seconds — which is what `narrate.js`
  computes. Screen read time is how long it takes a viewer to *find the thing that
  changed*, and it is always longer than it feels while authoring. `narrate.js`
  knows only the caption; when the screen is the slower of the two, pass `holdMs`.
- **About two events per ten seconds.** A click, then its result. Not a click, a
  scroll, a filter and a result — that is four things a viewer is asked to track
  in the time they can follow one.

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

## The take has to be true, not only watchable

`narrate.js` makes a recording watchable. Nothing in it — by design — checks that
what you filmed actually happened, or that the timestamp you cut on points at it.
Four failures, each of which cost a take and each of which *looked like success*:

### The recorder's clock is not the video's clock

It is wrong in **two** ways at once, and fixing only the first is the trap:

- an **offset** — recording starts when the browser context is created, before
  your first navigation has settled;
- a **scale** — the capture drops frames while the page is busy, so the file plays
  back longer than the session it recorded. Measured at **~1.065**.

A constant offset that was right at the start was **four seconds wrong by the
end** — the difference between cutting to the payoff screen and cutting to the one
before it. Three rounds of cuts showed the wrong moment in every beat before this
was found, and each one was plausible in isolation.

So: record both anchors and map linearly, `video_t = A + B × mark_t`. Then
**verify by looking** — one frame from the middle of every clip, tiled into a
contact sheet. Spot-checking two clips is exactly how the wrong offset survived
those three rounds.

### Assert the state the beat is about

A click can be swallowed while the previous action's request is still in flight,
and the result looks fine: a board ended up *full but not solved*, so the payoff
never arrived and the control that depended on it stayed disabled. Check the DOM
for the state the beat is **about** — not that the click returned.

A beat that cannot be asserted is a beat you cannot trust. This is not a verdict
about the app: the demo still never gates the build. It is a check on the
**recording**, and it belongs here for the same reason a camera has a viewfinder.

### Pace to the app, not to the script

Driving entries faster than the runtime committed them made two microflows overlap
and deadlock in Postgres. The `UpdateConflictException` surfaced as a modal dialog
that then swallowed every later click and killed the take. Two defences: never act
faster than a floor found experimentally per app, and detect-and-dismiss the error
dialog so one failure does not cost the session.

Test the dialog guard on **visibility, not presence** — Mendix ships the error
dialog container in the DOM hidden, so a presence test fires on every click.

### `recordVideo.size` pads; it does not scale

A viewport smaller than the video size lands in the top-left corner with grey
around it. For a fixed-width page — which most Mendix layouts are — set the
viewport **to** the video size and apply CSS `zoom`: the page then lays out at the
smaller effective width while Chromium rasterizes at full device resolution.
Sharp and full-frame, where a smaller viewport is soft and letterboxed. A
stylesheet does not survive a navigation, so re-apply it after every `goto`.

### These ship as code: `take.js` and `cut-clips.js`

Both sit beside this file and are copied into the project with it. CommonJS, like
`narrate.js`, and required the same way.

| | |
|---|---|
| `openTake(browser, opts)` | a context with `recordVideo`, both clock anchors, the zoom fix |
| `take.mark(name)` | a beat, timed from the settled first screen |
| `take.click` / `take.type` | paced to `minGap` and guarded against the error dialog |
| `take.assertBeat(name, probe, why)` | records whether the beat held; `finish()` **throws** if one did not |
| `take.finish()` | closes the context, writes `capture/beats.json` with `offset_s` |
| `node cut-clips.js` | cuts the raw take on the linear map, refuses implausible anchors, writes the contact sheet |

`cut-clips.js` reads a project-owned `capture/clips.json` edit list, so the script
is the same everywhere and only the edit is per-film. It **refuses** a clip
shorter than its target unless that clip is explicitly marked `"freeze": true` —
holding a final frame is legitimate on a static screen, never to stretch an
interaction, and every pad is reported.

What stays per-project is the walk and the selectors. Only the machinery is shared.

## What to narrate

Narrate only what a viewer with **no build context** would understand.

Cut:

- anything that reads as *"here's proof the database is right"* — that is Stage 1,
  and to a viewer it is noise about a claim they were not disputing
- anything implying *"this used to be broken"* — the audience did not see the
  before, so it lands as an apology for a bug they never met
- **every word of Mendix vocabulary.** No entity, microflow, page, association,
  domain model, catalog. The test is sharper than a word list: *if the visual
  needs those words to make sense, the visual is wrong.*

Keep it to the persona's own motivation: what they are trying to do, what they
see, and what changed for them. Name them — "Sam", not "the user".

**Unless the product is single-player.** A puzzle, a calculator, a personal tool
has no task-persona, and inventing one is affectation. Write about the thing in
present tense instead.

## Where this stops

This skill owns the **capture**. How a capture is framed, cut and scored into a
finished film is the video system's — `video-system/` in `ako/mxcli-intro-video`,
which defines the product-demonstration type this skill feeds.

Two boundaries worth keeping: the recording is **full-bleed** (no browser chrome,
no window frame, no laptop mockup — the capture *is* the frame), and the
narration plate stays `narrate.js`'s. **One caption system per film**; a second
one layered on in the edit reads as two designs.

## Checklist

- [ ] `journeys/<Module>.journey.json` exists and the journey run is `PASS`
- [ ] The positive-control run has shown every rung can go red — no `UNPROVEN`
- [ ] The demo is a **separate script**; no PASS/FAIL verdict lives in it
- [ ] Pace is human, with real reading pauses
- [ ] Something animates during pauses, so idle time survives into the video
- [ ] Recorded at **both** a desktop viewport and a real mobile device profile
- [ ] The mobile pass runs the same steps, with nothing simplified
- [ ] Narration mentions no database proof and no past bugs
- [ ] No Mendix vocabulary anywhere in the narration
- [ ] Every list the camera sees is populated with plausible data; every column
      and button the camera sees is captioned
- [ ] Every beat that has a payoff is asserted, and the take reported none failed
- [ ] Both clock anchors recorded, and the contact sheet **looked at** — each tile
      shows its own beat
- [ ] Every frozen tail is on a static screen, and reported
- [ ] App was live and the database real — this instrument mocks nothing
