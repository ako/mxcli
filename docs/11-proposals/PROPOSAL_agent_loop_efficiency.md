---
title: Agent loop efficiency — making an mxcli session cost what the work costs
status: draft
date: 2026-09-22
related:
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PROPOSAL_playwright_session_reuse.md
  - PROPOSAL_llm_mdl_assistance.md
  - PROPOSAL_session_logging.md
  - PROPOSAL_check_diagnostics_catalog.md
  - cmd/mxcli/init_claudemd_budget_test.go
---

# Agent loop efficiency — making an mxcli session cost what the work costs

**Status:** Draft
**Date:** 2026-09-22 (initial), revised 2026-09-23 — three complete builds
measured with `diag loop-report`, overturning the `check`-dominant assumption and
promoting the app restart to the wall-time lever (§"Three projects, measured");
revised 2026-10-10 — the BENCH-001 baseline recorded, putting skill and doc
reads at ~68% of the re-read cost against mxcli's ~15%, which promotes a new
item 3b, demotes item 4 and closes `mxcli apply` (§"The baseline, measured").
Revised again 2026-10-10 — a second BENCH-001 run shows the first pass at item 3b
was adopted (whole-file skill reads became `sed` ranges) but did not reduce the
bill, with one adverse shape it appears to have induced; the comparison is n = 1
per side and is recorded as unsettled (§"Run 2, measured").

## The report

A side-by-side test had Opus build an app twice: once on Vercel (TypeScript files
straight to disk), once with mxcli against Mendix.

| | mxcli / Mendix | Vercel |
|---|---|---|
| Duration | 2 h 33 | 1 h 07 |
| Model calls | **523** | **123** |
| Conversation re-read from cache | **228 M** | **42 M** |
| Cache writes | 1.27 M | 0.48 M |
| Output tokens | 500 k | 205 k |
| Avg conversation per call | 435 k | 345 k |
| Bash commands | 425 | 58 |

The bill is dominated by the 228 M re-read, and that figure is not a mystery:
523 × 435 k ≈ 228 M. It is the product of two numbers and nothing else.

## Why this is a square, not a line

Every model call re-reads the whole conversation. Conversation size grows with
the content the calls put into it. So for a session of **N** calls whose
transcript grows roughly linearly to size **S**:

```
total re-read  ≈  N × S/2
```

and when the removed calls take their own tool results out with them, **S falls
with N** — so the total falls with **N²**. Halving the call count on the same
work is a ~4× cut, not a 2× cut. The measured pair is consistent with this:
4.25× the calls and 1.26× the conversation gives 5.4×, which is what the logs
show.

That arithmetic sets the priority order, and it is not the intuitive one:

1. **Fewer calls** — enters quadratically. Everything else is second.
2. **Less text per call** — enters linearly, but it is also the multiplier on
   lever 1, so the two compound.
3. Output tokens (500 k of 228 M) are a rounding error. Do not optimise here.

## Three projects, measured — and what they overturn

This proposal's first draft reasoned from one session. `diag loop-report` has
since run against three complete builds, and the result contradicts two things
the draft assumed. Both corrections are kept here rather than quietly edited
away, because the way each assumption survived is the reusable part.

| project | Mendix | session | invocations | top commands by CALLS |
|---|---|---|---:|---|
| mxcli-ledger | — | 5 days | 301 | `check` 250 (83%) |
| CapTrackV6 | 11.14.0 | 2 h 45 | 585 | `exec` 180, `-c` 111, `check` 90 |
| mxcli-demo-2 | 11.13.0 | 5 h 07 | 408 | `-c` 120, `exec` 85, `check` 63 |

**Overturned 1: `check` is not the dominant call.** The ledger's 83% was read
as the shape of an mxcli loop, and the obvious follow-up — "why so many checks?"
— was queued as the next thing to attack. Two further projects put `check` at
15% and 22%. The ledger is the outlier, not the archetype. A distribution taken
from one project is an anecdote with a table around it, and the fix for reading
it that way is three, not a better argument about one.

All three logs predate ako/mxcli#629, so their `-c` and `exec` counts are
**inflated by mxcli's own child processes** — `mxcli test` spawns three before a
single test runs, so demo-2's 11 test runs contributed ~33 phantom calls and
CapTrack's 5 contributed ~15. The direction of that error matters: it inflates
exactly the two commands that displaced `check` at the top. It does not overturn
the overturning — `check` is still nowhere near 83% in either — but any figure
in this table is a pre-#629 figure.

**Overturned 2: the wall-time lever is `run`, and the report understates it.**

| project | closed-run wall time | `run` calls | `run` closed | `run` median |
|---|---:|---:|---:|---:|
| CapTrackV6 | 1,775 s | 34 | 7 | 90.7 s |
| mxcli-demo-2 | 838 s | 30 | 4 | 70.5 s |

Read naively, `run` is 27–29% of mxcli's wall time. That reading is wrong, and
wrong in the same direction both times: **a boot that is killed never writes a
summary record, so its duration is not counted at all.** 26 of 30 and 27 of 34
`run` invocations are uncounted. The totals above are floors.

Multiply the boot count by the per-boot median instead and the shape changes
completely: ~35 min of boots in demo-2, ~50 min in CapTrack — against sessions of
5 h and 2 h 45. Every other command in both tables is noise beside that.

**Since fixed (item 2d).** `diag loop-report` no longer drops an unclosed run's
time. Three separate reasons a boot wrote no summary record had to be dealt with
— `main()` exited through `os.Exit` on the error path; each command's own
deferred close got there first and recorded the run as clean; and `run --local`
is a `Run:` command that exits through `os.Exit` at every failure site, which no
close-on-error can reach. What covers all three, and a `SIGKILL` as well, is a
`session_alive` heartbeat every 30 s: an unclosed run is now bounded from below
by its last one. The report prints three figures rather than one — the closed
runs' measured total, the unclosed runs' **measured floor**, and an estimate at
the verb's median for the unclosed runs that left no heartbeat at all — and the
estimate is never added into the measured total.

One caveat for the numbers above: the logs behind this table predate the
heartbeat, so they can only be re-derived with the estimate, not the floor. The
table's figures stay floors; what changes is that the next session's will not.

This also corrects a figure CapTrack reported about itself. Its write-up says "of
~2.5 h building, only ~30 min was spent inside mxcli" and ranks its levers on
that basis. The 30 min is the closed-run total (1,775 s), which already contains
the 7 boots that closed; the 27 it excludes add ~41 min at the measured median,
so the real figure is around 70 min — more than double, and enough to change
which levers are worth pulling.

## The baseline, measured (BENCH-001, 2026-10-10)

The first `eval run` of the fixed brief, on Mendix 11.15.0, scoring 20 of 21 —
the one failure environmental (`postgresql: unrecognized service`, so the
`tests --local` tier never ran).

| | |
|---|---:|
| model calls | 47 |
| tool calls | 48 (Bash only) |
| wall | 7m00s |
| avg context | 103 k |
| cache read | 4.7 M |
| output | 27 k |
| tool results | 48 k, re-read cost **1.7 M (37% of cache read)** |
| mxcli invocations | 70 across 38 Bash calls, **17 chained >1**: `exec` 20, `check` 15, `syntax` 6, `-c describe` 5, `lint` 4, `test` 4 |
| categories | orientation 31%, validate 23%, apply 17%, verify 13%, retry 15%, write 2% |

`47 × 103 k ≈ 4.8 M` — the run behaves exactly as `N × S` predicts, and output
is a rounding error again.

**It is NOT comparable to the 523-call report.** Different brief, far smaller
scope, and the levers in this document had already shipped. Nothing here says
they worked; the pre-intervention "before" is not recoverable. This is a
reference point for *future* changes, which is all item 1 ever claimed.

**It is also n = 1.** Every share below is one sample of a stochastic process
with unmeasured variance. Treat the ordering as the finding and the percentages
as approximate.

### The dominant cost is reading the skills, not running mxcli

The top ten results carry ~1.34 M of the 1.7 M re-read cost, and they split:

| what | re-read cost | share of 1.7 M |
|---|---:|---:|
| skill and doc reads (three `cat …/SKILL.md`, a `wc -l` over three more, a `sed` range, a `grep` over a skill) | ~1.16 M | **68%** |
| mxcli's own output (`DESCRIBE STRUCTURE`, `syntax page datasource`, `syntax entity`) | ~252 k | 15% |
| `ls -la` | 35 k | 2% |

`cat write-microflows/SKILL.md` alone is 7.1 k tokens re-read 43 times — 307 k,
more than every mxcli invocation in the top ten put together. With
`DESCRIBE STRUCTURE` and `ls -la`, **orientation is ~77% of the re-read bill
against 31% of the calls.**

The cheap path exists and was used: `mxcli syntax page datasource` cost 2.6 k
and `syntax entity` 1.1 k, against 7.1 k for the skill that covers the same
ground. The agent also tried to read a skill in part (`sed -n 314,420p`), so the
intent is there and the structure is not.

**This is a lever this document did not have**, and it dominates the two it did:
make consuming the skills cheap. Cheapest first — point the skills at `mxcli
syntax` instead of restating syntax in prose; give them sections an agent can
read one of; and only then spend on item 4, which addresses the 15%.

### First pass at the fix, and what still has to be measured

Two changes, deliberately only two, so a second benchmark run stays
interpretable:

- **Routing.** The generated CLAUDE.md said "read the matching skill before
  writing microflows, pages, security…", which is the instruction that produced
  the `cat`. Its lookup table now reads *How to write any MDL — **before any
  skill*** against `./mxcli syntax`, and one line says to read a skill's section
  rather than the file. It had to be folded into the existing table row rather
  than added as a paragraph: that file is re-read every session and
  `TestGeneratedClaudeMDStaysWithinItsContextBudget` had 75 bytes of headroom,
  so prose about saving tokens would have cost more than it saved.
- **Storage.** Every `SKILL.md` now opens with a generated line-numbered
  section index (`scripts/skill-index.py`, `make check-skill-index` in CI), so
  `sed -n '<a>,<b>p'` is reachable without first paying for the file or a
  `grep`. Generated, never hand-written — a hand-kept table of contents is the
  drift this repo has paid for twice — and the ranges are computed to a fixed
  point, because the block's own height shifts every range below it. All 745
  rows were verified to land on their heading.

Both have a cost, and it is honest to state it: the index is ~0.17k tokens on
every skill read, so if the agent keeps reading whole files the change is a
small *loss*. That is the hypothesis a second run tests, and the measurement to
watch is not the total but whether `cat …/SKILL.md` leaves the costliest-results
list.

Measured consequence worth recording: **the naive version of this lever is
smaller than it looked.** Code blocks are 28% of all skill bytes (32-43% in the
two most-read), so "move the syntax out of the skills into `syntax`" could never
have reclaimed most of the 68% — the skills are mostly prose, which is the part
CLAUDE.md says belongs there. The reachable win is in *how much of a file gets
read*, not in what the file contains.

### Run 2, measured — the mechanism was adopted and the bill went up

`BENCH-001-2026-10-10T18-47-02`, session `03e4df6c`, same brief, same 20/21
(same environmental `postgresql` failure), run against a binary carrying both
changes above.

| | run 1 | run 2 |
|---|---:|---:|
| model calls | 47 | **61** |
| wall | 7m00 | 7m25 |
| avg context | 103 k | 104 k |
| cache read | 4.7 M | **6.2 M** |
| re-read cost | 1.7 M (37%) | **2.1 M (34%)** |
| mxcli invocations | 70 / 17 chained | 85 / 22 chained |
| `syntax` calls | 6 | 8 |
| doc lookups | 7 | 13 |
| orientation share of calls | 31% | **43%** |

**The mechanism was adopted.** Three of the four whole-file skill reads became
ranges — `sed -n 47,264p create-page` 3.7 k, `sed -n 112,265p write-microflows`
2.4 k, `sed -n 1,200p test-microflows` 2.1 k, against run 1's 7.1 k and 6.7 k
`cat`s — and `syntax` use rose. The index is being read and used for what it was
built for.

**The totals got worse anyway**, and one `cat
.ai-context/…/overview-pages/SKILL.md` (6.1 k × 55 = 338 k) is now the single
costliest result in the run. So the hypothesis as written — "watch whether `cat
…/SKILL.md` leaves the costliest-results list" — came back **mixed**, which is
not a pass.

**One adverse mechanism is visible and plausibly induced by the routing
change.** Run 2 contains three `for t in "…" "…" …; do ./mxcli syntax $t; done`
loops, at slots 3, 4 and 6 of the costliest results (3.4 k + 2.6 k + 1.9 k,
≈ 686 k combined). Run 1 has no such shape. Telling the agent to reach for
`syntax` before a skill appears to have produced *speculative sweeps across
topics*: per-topic `syntax` is still cheap, but a batched sweep is a single
result larger than the whole-file read it replaced, and it is paid on every
subsequent call. The cheap path was made more attractive without being made
narrower.

**n = 1 per side, so none of the deltas above are attributable.** Two independent
reasons to distrust the comparison, beyond the sample size:

- The two runs read **different skills** (run 1 write-microflows and
  test-microflows; run 2 overview-pages and create-page). Skill *selection* is
  itself stochastic, so the per-read costs are not measuring the same text.
- The 47 → 61 call delta is larger than anything the change plausibly touches
  directly, which is what a variance-dominated pair looks like.

So the honest position is: **the first pass at item 3b is not validated, and not
refuted.** What settles it is an A/B with repetition — two more runs of the
current binary and two of `9ce90f32` (the pre-change build), reported as four
points, not two. Until that exists, neither defending the change nor reverting
it has evidence behind it, and the pre-registered revert condition ("if the agent
keeps reading whole files, revert") did not fire cleanly: it kept reading *one*.

The one unambiguous read of run 2 is about **where the remaining cost is**: with
ranges in place, orientation went from 77% of the re-read bill to still being its
largest block, now spread across more and cheaper results rather than
concentrated in a few expensive ones. Making each read cheaper has not reduced
the number of reads, and the number of reads is the term that enters
quadratically. That points the next attempt at item 3 (state the chain, so fewer
orientation round-trips are needed) rather than at further compression of what
each read returns.

Separately found while reading the transcript, and not part of this measurement:
the run spent a call on a `brain` subcommand reporting that no store exists
before `brain init` has run. The generated CLAUDE.md routes to `brain plan` for
"planning, or picking work up" without naming that prerequisite. Verified
against the current binary, `brain plan` itself degrades gracefully (`No slices
yet`, exit 0) and `brain brief` is the one that stops; which of the two the run
hit is not recoverable from the pasted output, so this is logged as a routing
question to confirm, not a diagnosed defect.

### What that settles about `mxcli apply`

The criteria recorded above are answered. The second — chained output being a
material share of `tool results` — is **no**: mxcli's output is a minority of
the re-read cost. On this evidence the staged pipeline would be optimising 15%
while 68% sits untouched, so it is not the thing to build, and the entry is
closed in the sequencing table rather than left open.

One datum is still outstanding on the first criterion. `check` 15 against `exec`
20 looks like the redundant check-before-exec pattern, since `exec` already runs
the full check pass — but plain `check` without a project is a legitimate call,
so the verb counts cannot settle it. `diag loop-report`'s
`check_then_exec_pairs`, run in the same container, is what decides it; if it is
high, the answer is to **state the chain better**, which is item 3, not to wrap
it in a command.

## What is actually asymmetric — and how little of it is irreducible

An earlier draft said: "Vercel's agent writes a `.tsx` file and the work is
done." That is false, and it was doing real damage to this proposal by filing
the gap under platform tax instead of under work.

A `.tsx` file is not done when written. It needs type-checking, building and
rendering before anyone knows it worked, exactly like an MDL change. The
difference is not *whether* verification happens — it is four properties of the
verification, and **three of the four are things we can move**:

| | TypeScript / Next | MDL / Mendix |
|---|---|---|
| **Cost per verification** | `tsc --noEmit` ~1–3 s; HMR sub-second and **zero tool calls** — the dev server is already running and the browser updates itself | mxbuild ~25 s **and a tool call**; the HMR analogue (`reload_model`) is blocked on 11.14 |
| **How often it is needed** | low — TypeScript and React are saturated in training data, so first-attempt success is high | higher — MDL is a DSL invented in this repo and appears nowhere in training data |
| **Error locality** | `file:line:col`, expected vs actual | a CE number naming a *document*, at the far end of a build — and per ako/mxcli#568, `docker check` can report "0 errors" while the build fails |
| **Does the last tier need a running app?** | no for logic, yes for render — but the browser is already open on the changed component | yes, plus `reload_model`, plus login, plus navigation |

Row 2 is the one mxcli can only partly close, and
`PROPOSAL_llm_mdl_assistance.md` owns it. The other three are engineering, and
two of them are already in flight:

- **Row 1 is `mxcli check` vs mxbuild.** `check` is the `tsc` of MDL, and
  `PROPOSAL_check_mxbuild_gap_heuristics.md` is a **standing programme** to close
  the gap — ~17 rules shipped, each one a construct that used to be found only
  by a 25 s build and is now found in ~2 s with no build at all. Every rule moved
  across that line is a direct call-count *and* wall-time win. This is the
  highest-value structural work in the whole picture and it was already
  underway; this proposal's contribution is to say why it is a **token** lever
  and not only a correctness one.
- **Row 1's other half is the HMR analogue**, which exists (`reload_model`,
  ~3 s on 11.13) and is blocked by the mxbuild defect below.
- **Row 3 is diagnostic quality**, which `PROPOSAL_check_diagnostics_catalog.md`
  owns. It matters here because a vague error costs a *diagnosis*, and a
  diagnosis is the 40-call tail in lever 4.

### The real target: build once per batch, not once per change

That is what the Vercel agent does. It does not run a production build after
every file — it leans on a fast, trusted static check and batches the expensive
gate. mxcli can have the same shape, and the blocker is **trust**, not speed:
an agent runs the 25 s build after every change precisely because `check`
passing does not yet mean the build will pass.

So row 1 and row 3 compound. Closing check-vs-build parity does not merely make
the expensive gate faster — it makes the expensive gate *rarer*, because it
becomes reasonable to batch. And ako/mxcli#568 is a direct attack on that trust
from the other side: a check that can say "0 errors" over a build-failing model
teaches an agent never to believe it.

**What is left that is genuinely irreducible:** the final "does it render
correctly" tier needs a running app, in both worlds. That is one gate, rarely,
per the tier table above — not a per-change tax.

The scope caveat still stands and is separate: the two sessions did not build
the same thing. The mxcli one additionally covered login styling, documentation,
Docker and password lockout. Some unknown part of the 4× is simply more work.

## Lever 1 — collapse the per-change round trip (attacks N)

The reported loop is **5–8 calls per change**: write script → `mxcli check` →
`mxcli exec` → `mx check` → restart (~35 s) → log in → screenshot.

Four of those steps are already avoidable with today's binary:

- **`check` before `exec` is NOT redundant — I had this wrong.** An earlier
  draft claimed `exec` folds in everything `check` does, so the two-gate form
  wasted a call in every project. That is false, and the correction matters
  because it was Track A's headline.

  There are **two** validation passes, and `exec` runs only the first:

  | Pass | What it catches | `check -p` | `exec` |
  |---|---|---|---|
  | `executor.ValidateProgram(prog, path)` (package-level) | semantic rules — MDL0xx, reserved words, list-op nesting | ✅ | ✅ |
  | `exec.ValidateProgram(prog)` (method, project connected) | **reference resolution** — dangling entity/page/microflow/icon names | ✅ | ❌ |
  | `exec.CheckProjectConflicts(prog)` | plain `CREATE` over a document that already exists | ✅ | ❌ |

  So `mxcli check script.mdl -p app.mpr` genuinely catches things `exec` does
  not, *before* a partial write — and `exec` is not transactional, so that
  preflight is load-bearing. The gate list is teaching an additive step, not a
  wasted one.

  (`--references` in the gate line *is* redundant: it is implied by `-p`. That
  is cosmetic.)

  **How the error was made, since it is the instructive part:** the claim was
  read off `cmd_exec.go`'s doc comment — "the same semantic checks as
  `mxcli check`" — which is accurate about the pass it describes and silent
  about the two it does not. Reading the call graph takes one more step and was
  skipped. The repo's own checklist has the rule that would have caught it
  ("Fix proven to be the cause — revert it and confirm the symptom returns");
  the equivalent here was to diff what the two commands actually call.
- **The 35 s restart is NOT avoidable on Mendix 11.14** — see the section below.
  On 11.13 and earlier, `run --local --watch` hot-reloads a behavioural change
  in ~3 s, so the restart is opt-in slowness there and forced here.
- **Logging in by hand is solved.** Playwright storage state is captured and
  reused (`screenshot --load-storage`); see `PROPOSAL_playwright_session_reuse.md`.
- **The screenshot is usually the wrong instrument.** See lever 3.

### First: the agent can already chain this, and mostly should

The obvious objection to a new command is that `&&` exists:

```bash
mxcli exec changes.mdl -p app.mpr && mxcli docker check -p app.mpr
```

That is **one tool call**, needs nothing built, and captures most of lever 1's
value today. It works because the commands are exit-code-honest — `exec` exits
non-zero if any statement failed, and `docker check` propagates `mx check`'s
status through `cmd.Run()` rather than printing errors and exiting 0. Worth
stating explicitly, because a chain built on a command that reports failure only
in stdout would pass silently, and that is the failure mode that would make
chaining unsafe. It is not present here.

So the call-count win does **not** require a new command. What `&&` does not
give is the *token* win: it concatenates the stdout of every stage that ran, so
a five-stage chain puts five stages of output into the conversation forever —
the opposite of the compact verdict this proposal wants. The agent can paper
over that with per-invocation `tail`/`grep`, but then it is writing fragile
filters that encode each command's output shape, and getting them wrong is
silent.

**That reorders the proposal.** Lever 2 (output discipline) is the more
fundamental of the two, not the junior partner: with terse, delta-shaped output
from each command, `&&` chaining gets nearly all of `apply`'s value at zero new
surface area — and the improvement lands on every *other* invocation too, not
just the ones inside the chain.

### So what, if anything, is left for a command?

Two things, and both are weaker than the first draft claimed:

- **The reload/restart decision.** Mapping the serve build's `restartRequired`
  to reload-vs-restart is not expressible in `&&`. But when `run --local --watch`
  works it already does this in the background, and the agent orchestrates
  nothing; when it does not (11.14, below) the answer is a restart, which *is*
  `&&`-able. So this is thin.
- **Consistency.** An agent composing the chain fresh each session composes it
  differently, and sometimes wrongly — the cost report is the evidence, having
  run the redundant `check`, restarted when it need not have, and logged in by
  hand. But that is cured by **stating the chain**, not by shipping a wrapper
  around it.

**Revised recommendation: publish the one-liner, do not build the command.** Put
the canonical chain in `projectGates` and the skills, fix the outputs it
concatenates, and build `mxcli apply` only if `diag loop-report` (lever 6) shows
agents still composing it wrong after that. This is strictly cheaper, ships
sooner, and does not add a surface that has to stay in sync with the commands
underneath it.

**Re-raised 2026-10-10, as a staged pipeline with the agent naming how far to
run** (`check -> check --references -> mx check -> mxbuild -> run -> test`). The
deferral holds, and two facts narrow the idea further than the paragraphs above
already do. **`exec` runs the whole `check` pass, references included, before it
writes anything** — so the first two stages are not stages, they are inside the
third. And **`docker check` IS `mx check`**; mxbuild is what `mx` wraps, so those
are one stage rather than two. Six stages are four: apply, build-validate, serve,
behaviour.

To keep the decision falsifiable rather than a matter of taste, this is what the
baseline has to show for the command to be worth building:

- the transcript composes the chain **inconsistently or wrongly** across the run
  (a redundant `check` before `exec`, a restart where a reload would do, a
  verification tier above what the change needed), or
- the chained invocations' concatenated output is a materially large share of
  `session-report`'s `tool results` figure — which would argue for item 4
  first regardless, since that lands on every invocation and not only the
  chained ones.

If instead the chains are well-formed and the output volume sits elsewhere, the
command buys nothing that item 4 does not buy more cheaply, and this entry should
be closed rather than left open.

### The chain is tiered, not fixed — most changes stop at the first gate

The first draft put `--verify <playwright>` in the default chain. That is a
mistake of the same kind the cost report is complaining about: **an always-on
gate chain trains maximal verification.** If a browser run is in the default
path, every change pays browser cost, and the report's "I tested every admin
flow ... most of those checks included screenshots" stops being a choice the
agent made and becomes a property of the tool. Hard-wiring it would
institutionalise the expensive failure mode.

Verification tier is a **per-change decision**, and the routing rule already
exists in `.claude/skills/verify-in-runtime.md` — a table from symptom to
cheapest sufficient proof, with explicit counter-examples where the browser
would be waste. The chain should express those tiers and stop at the first one
that is sufficient:

| What changed | Sufficient gate | Cost |
|---|---|---|
| any MDL edit | `mxcli exec` (check folded in) | ~2 s, no build |
| structure a build can reject (pages, widgets, settings) | `+ docker check` / the serve build | ~25 s |
| microflow *behaviour* | `+ mxcli test` — no browser | ~2 s warm |
| what the app **renders or looks like** | `+` browser, once | expensive, rare |

Most changes stop at row 1 or 2. The browser row is the rare one, and it is the
only row that pays image input. Making the tier explicit is itself a lever: it
converts "verify everything, to be safe" into a decision with a stated default.

### The 35 s restart is blocked by mxbuild on 11.14, not by our defaults

An earlier draft of this proposal called the restart-per-change "opt-in
slowness". That is wrong on the version a new project most likely lands on, and
the correction matters because it changes what is actionable.

**On Mendix 11.14 the first build in an `mxbuild --serve` process succeeds and
every subsequent build in that process fails.** The first build does not leave
the deployment in a state its own incremental build can continue from: the
bundler config (`web/rollup.config.mjs` / `web/rspack.config.mjs`) and the
per-document client (`web/pages/`, `web/layouts/`) are both absent, and which
one the build dies on is only how far it gets before it needs one.

This is measured in `cmd/mxcli/docker/webclient_legacy_paths.go`, against
mxbuild 11.14.0 driven **directly over its HTTP API with mxcli removed from the
picture** — the same `/build` request POSTed twice with the model untouched:

```text
build 1   Success
build 2   Failure — ERR_MODULE_NOT_FOUND for web/rollup.config.mjs,
                    imported from mxbuild's own tools/node/rollup-runner.mjs
```

Three controls place it in mxbuild rather than here: a one-shot
`mxbuild --target=deploy` run twice into the same directory succeeds both times
(so it is the serve process's state, not the 11.14 deployment shape); flipping
to Rspack gives an identical failure naming the other config (so it is not the
bundler choice); and 11.13.0 hot-reloads normally — measured, build #2 applied
via reload in 3.4 s. Restoring the deleted config rescues only the
model-unchanged case, which is the case nobody needs. `rm -rf deployment/` costs
a cold build and changes nothing.

mxcli cannot fix this from outside, and does not pretend to: it recognises the
failure **by its own shape** rather than by version, so a fixed mxbuild goes
quiet on its own. The docs say plainly that `--watch` is not usable on 11.14.

Three consequences for this proposal:

1. **The cost report's 35 s-per-change was very likely forced, not chosen.** The
   project that first reported this "routed around it with a restart per
   change", which is exactly the pattern in the session log.
2. **It does not weaken lever 1.** Wall time and call count are separate axes.
   The mxbuild defect taxes *time*; the 228 M bill is *calls*. `mxcli apply`
   collapses 5–8 calls into 1 whether the build underneath it is warm or cold —
   so on 11.14 it is the only lever left on that axis, and therefore more
   important, not less.
3. **`mxcli test --attach` is probably blocked too, and this is unmeasured.**
   `--attach` must rebuild to pick up its test microflows, and it rebuilds
   through the attached app's own serve process (`runner_attach.go`) — whose
   first build happened at boot. That makes the attach rebuild a *second* serve
   build, which is precisely the failing one. This is read off the code, not
   measured; it needs one run on 11.14 to confirm or kill. If it holds, the warm
   *test* loop is blocked on 11.14 as well, and the skills that recommend
   `--attach` need the same version caveat `--watch` already carries.

**But 11.14 is not the whole story, and a 11.13 project proves it.** When this
section was written the defect explained the restart-per-change, and that closed
the question. It should not have. mxcli-demo-2 ran **11.13.0** — the version this
proposal's own control measured hot-reloading in 3.4 s — and still took **30 full
restarts**, its findings saying plainly:

> `run` without `--watch`: every model change meant a full restart (the 30 runs,
> 243 s). `--watch` would hot-apply most changes as one long-running invocation.

So the two projects are a natural experiment, and they answer differently:

| | Mendix | warm loop | restarts |
|---|---|---|---:|
| CapTrackV6 | 11.14.0 | blocked by the defect | 34 |
| mxcli-demo-2 | 11.13.0 | **available, ~3 s** | 30 |

On 11.14 the restart is forced. On 11.13 it is chosen — by a loop that never
reached for `--watch`. An earlier draft of this proposal said exactly that ("the
fast paths exist — the session just didn't take them, which makes this a defaults
problem more than a capability one"), the 11.14 measurement contradicted it, and
the correction went one step too far: it replaced "defaults problem" with
"mxbuild problem" when both are true on different versions. A measurement that
explains a symptom on one version does not retire the hypothesis on the others —
and the tell was available, since the same control that proved the defect
(11.13 reloads in 3.4 s) also proved a working warm loop existed to be unused.

**That makes the restart lever real on both versions, by different routes:**
fix the default *version* for 11.14, and fix the default *invocation* for
everything else. `run --local --watch` is not what a session reaches for, and
neither the generated CLAUDE.md gate list nor the skills tell it to. A one-line
default is the whole intervention on 11.13; it is worth ~30 boots.

**The version default makes this worse than it needs to be.** `bootstrap-app`
chooses the newest version on the CDN when the environment has nothing cached —
11.14.0 at the time of writing — so a freshly bootstrapped project lands on
exactly the version where the warm loop does not work, without anyone choosing
it. Until mxbuild is fixed, the default should prefer the newest version whose
warm loop is known to work (11.13.0), and say in one line why. A user who asks
for 11.14 still gets it, with the caveat.

**This is also a standing strategic risk worth recording.** `--serve`, `--host`
and `--port` appear in `mxbuild --help` but not in the reference guide at
docs.mendix.com/refguide/mxbuild/, which documents only the four `--target`
modes. The entire warm loop is built on an undocumented interface carrying no
compatibility promise. That argues for (a) reporting this defect to Mendix
rather than only routing around it, and (b) keeping the cold-build path a
first-class supported mode rather than a fallback.

**What the gate list actually gets wrong.** Not the `check` gate — the framing
around it. The generated CLAUDE.md says the gates are "**the definition of done,
not a menu** — a change is finished when they have all been run", and the list
has `docker check` (~25 s), `test` (~30 s cold) and `run --local` in it. Read
literally, that mandates all seven gates on **every change**, which is precisely
the maximal-verification pathology the cost report describes. It also
contradicts the sentence immediately above it, which offers an escalation rule
("each is only worth paying for once the one above is clean").

The fix is **batching, not deletion**: the gates are the definition of done for
a *change*, where a change is a coherent unit of work — not per statement and
not per file write. Iterate with `exec` until the script is right, then run the
gates once. That preserves every gate (the three-copy tests exist because `test`
fell off this list once) while removing the per-micro-edit repetition, and it is
the same "build once per batch" shape as the `tsc` comparison above.

**And a real code change worth making:** `exec` already connects to the project,
so it *could* run the reference pass in its preflight. If it did, the `check`
gate would become genuinely redundant for the apply path and the call would be
saved for real — and `exec` would stop being able to half-apply a script with a
dangling reference. `CheckProjectConflicts` is a separate question and probably
has to stay check-only, since a plain `CREATE` over an existing document is an
error for `check` but ordinary for a re-run. This is the one place in Track A
where the win is a code change rather than wording.

## Lever 2 — shrink what each call adds (attacks S)

A tool result is written into the conversation once and **re-read by every
subsequent call**. A 200-line `exec` transcript early in a 500-call session is
not 200 lines of cost; it is 200 lines × ~400 remaining calls.

- **Report the delta, not the transcript.** `exec` prints a line per statement.
  The idempotence work already distinguishes `Created` / `Replaced` /
  `Unchanged` per unit (`ExecContext.ReportMutation`), so the information to
  collapse this is in hand: `Created 4, replaced 2, unchanged 196` plus the six
  names that changed. A re-run of a settled script should be **one line**.
- **Default to terse; make verbose opt-in.** `MXCLI_QUIET` exists and the skills
  set it in places. It should be the default posture for non-interactive runs.
- **Prefer a terse agent format over `--json`.** `--json` exists globally, but
  JSON is frequently *more* tokens than good prose for the same facts. The
  target is fewest tokens that stay unambiguous, not machine-readability for its
  own sake.
- **Cap the long tails.** `docker check` error dumps, `show microflows` on a big
  module, catalog listings: add `--top N` and severity filters so a 400-line
  result becomes the 10 lines the agent will act on.

This lever is worth doing carefully rather than aggressively: an output trimmed
past the point of usefulness buys back its savings immediately in re-runs.

## Lever 3 — stop paying image input on a loop

Screenshots are expensive input, they recur, and they persist for the rest of
the session. The feedback names them explicitly.

`mxcli playwright verify <file>` already exists and returns **text** pass/fail
against a running app. That is the correct default instrument for "does this
flow work". A screenshot answers a different and much rarer question — "does
this *look* right" — and should be taken deliberately, once, when appearance is
genuinely the subject.

The rule for the skills: **assert in text; screenshot when the question is
visual, and then once.** `.claude/skills/verify-in-runtime.md` already has the
right shape for this (a table routing a symptom to the cheapest sufficient
proof); it needs the text-vs-pixel row added and `run-local` / `test-app`
pointed at it.

## Would the LSP help? Modestly, and not where the money is

mxcli ships a language server (`mxcli lsp --stdio`) with diagnostics, hover,
completion and go-to-definition. The natural question is whether pointing an
agent at it collapses the loop.

**It does not, and the reason is one line of the implementation.**
`runSemanticValidation` in `cmd/mxcli/lsp_diagnostics.go` "runs the same
validators as `cmd_check.go`". The LSP and `mxcli check` are the *same checker*
behind two front ends. So the LSP finds exactly what `check` finds — and the
check↔build gap, which is what forces the 25 s mxbuild per change, is completely
unaffected. The LSP makes the tier that is already cheap slightly cheaper, and
does nothing to the tier that dominates.

Two further reasons it is a wash rather than a win as normally used:

- **The call it saves is one we are removing for free anyway.** The redundant
  `check` call goes away because `exec` folds the check in. The LSP would be
  deleting a call already on the chopping block.
- **Reading diagnostics is itself a tool call.** Unless the harness attaches
  them to the edit result, it is one `getDiagnostics` instead of one
  `mxcli check` — the same arithmetic.

### The one condition under which it becomes a real lever

**If diagnostics ride along with the `Write`/`Edit` result at zero extra tool
call.** That is precisely the property identified as the Vercel agent's biggest
structural advantage in the asymmetry table above: its cheapest verification
tier is not a cheap tool call, it is *not a tool call at all*. An LSP wired that
way is mxcli's only available route to that property for static errors. Wired
any other way, it is a front end onto a command we already have.

### And one way it could make things worse

Diagnostics auto-attached to every edit are paid on **every** edit, including the
intermediate ones. An MDL script written top to bottom is incomplete at every
save but the last, so per-edit diagnostics on it are mostly noise about
incompleteness — potentially more tokens than one terse `check` at the end of the
batch. If this is wired up, it wants to fire on batch completion, not per
keystroke or per edit.

### What it is genuinely good for

**It is the second delivery channel for the parity programme.** Because the
validators are shared, every rule added by
`PROPOSAL_check_mxbuild_gap_heuristics.md` appears in the editor *and* in the
agent's checker with no extra work. That is an argument for spending on parity
rather than on the LSP: parity pays into both channels at once, while LSP work
pays into neither checker.

It is also a genuine win for the **human** in VS Code, and for wall time — the
server caches the widget and theme registries so the filesystem is not walked
per keystroke, which `mxcli check` does per invocation. Neither of those is a
token lever.

## Lever 4 — keep long investigations out of the main conversation

One self-inflicted bug (a stub script that wiped real microflow bodies) took
**~40 calls** to trace. Those 40 results then sat in context for the rest of the
session, taxing every later call.

A subagent pays for its own 40 round trips once and returns a paragraph. The
skills should say so with a trigger rather than a preference: **a diagnosis
expected to take more than ~5 probes is delegated, not run inline.** The same
applies to log spelunking and "which of these 30 files mentions X".

## Lever 5 — turn each discovered workaround into tool knowledge (mostly already done)

The first draft listed the five Mendix limitations the cost report named and proposed
turning each into a `check` diagnostic or a skill. **That was written from the
report's framing without verifying any of them, and checking all five afterwards
found that four were already covered and the fifth was not a Mendix limitation at
all.**

| Reported as a Mendix limitation | What it actually is |
|---|---|
| inputs inside lists are read-only, so admin editing became pop-ups | **An mxcli bug, fixed 2026-09-06.** Mendix's List View has its *own* `Editable` (default No) which wins over the textbox's; the parser accepted `Editable: true`, `buildListViewV3` never read it and the writer wrote false. See the finding in `mdl-executor.jsonl` |
| pop-up styling breaks outside the app's styled area | covered in `theme-styling/SKILL.md` — the class lands on `<html>`, so popups rendered at `<body>` follow it |
| login fields did not register scripted input | covered in `test-app/SKILL.md`, with the `playwright-cli eval` workaround, and it says the fill fails *silently* |
| the Docker image had the wrong Java version | handled in code — `docker/javaversion.go` knows 11.14 is the first version wanting Java 25 rather than 21 |
| the sidebar went stale after actions | the only one still open, and it is runtime refresh behaviour with no static signal to check for |

So the lever as originally written would have produced one diagnostic that is now
**actively wrong** (inputs in lists are editable, and saying otherwise sends a reader
back to the pop-up workaround they no longer need) and three that duplicate existing
skills.

**What the correction actually reveals is a routing problem, not a knowledge problem.**
The knowledge existed, in the skill whose `description` is supposed to surface it, and
the session hit the wall anyway. That is the same failure as the skill table drifting
to 12 of 68 (#906): an index that does not route is indistinguishable from missing
content. Effort here belongs in making skills *findable* at the moment of need, not in
writing more of them.

The general principle survives and is worth keeping: a workaround discovered in a
session is a cost paid once per session per user until it becomes a diagnostic, a
refusal or a skill. The lesson added by measuring is the prior step — **check whether
it is already known, and whether it is even true, before encoding it.** Encoding a
platform limitation that was really a bug outlives the bug.

## Lever 6 — measure it, then claim it

Everything above is a hypothesis until it moves a number, and this proposal
should not be merged on plausibility.

mxcli already logs every invocation as JSON Lines under `~/.mxcli/logs/`
(`diaglog`, wired at `newLoggedExecutor` in `cmd/mxcli/main.go` — so it covers
all commands, not a curated subset). That is the instrument.

**`mxcli diag loop-report`** reads a session's log and prints invocations by
verb, wall time by verb, and `check`-immediately-before-`exec` pairs. It shipped,
and it has now run against three complete builds — which is where the two
corrections above came from, so the instrument has already paid for itself by
falsifying its author.

Testing it against real projects also found three defects in it, each of which
had been silently skewing the very numbers it exists to produce: it logged a
minority of invocations and presented it as all (ako/mxcli#617), its `failed`
field counted a population its name did not describe (#620), and it counted
mxcli's own child processes as calls the agent made while reporting their parent
as a failure (#629). A measurement tool is not exempt from needing its own
evidence, and none of the three was visible from inside the tool.

**Two things it still cannot see, both of which matter to the lever above:**

- **A killed `run` contributes no wall time.** mxcli writes its summary on a
  normal exit, so the 26-of-30 boots stopped with a signal are counted as
  invocations and as zero seconds. The largest cost in the session is the one
  the report is least able to size — sequencing item 2d.
- **Hot reloads inside a long-running `run`.** One `--watch` invocation that
  applies forty changes is one row. That is the right unit for counting
  processes and the wrong one for showing that item 2c worked, so claiming 2c
  needs the app's own reload count, not this report.

**Then a benchmark.** One fixed app-brief, run end to end, recording model calls
and tokens. Without it, every claim here is an argument; with it, each lever
lands or does not. It also guards the result: the gate list drifted in three
places once already, and a loop regression is exactly as invisible.

### The second instrument: `diag session-report`

`loop-report` sees mxcli processes and nothing else — not the agent's
Read/Edit/Grep/Skill calls, not how large any result was, not why a call
happened. **`mxcli diag session-report <transcript.jsonl>...`** reads the agent's
own Claude Code transcript instead, which records all of it, and reports per
session: model calls (distinct API responses, not records), tool calls by tool,
cache-read / cache-write / output tokens, wall and active time; Bash calls split
into mxcli verbs, playwright, build, git and other; every call in one category
(orientation, write, validate, apply, verify, diagnosis, retry, delegate,
other); the costliest results ranked by **re-read cost = size × the model calls
after it**; error → retry chains aggregated by normalised error; and repeated
file reads and syntax/help/skill lookups. The rules are deliberately simple and
stated in the docs ("Measuring Agent Sessions") so a number can be argued with.

First run, over the 20 transcripts on the development machine (+76 subagents —
mxcli *contributor* sessions, not app builds, so the shape is not an app loop's):

| | |
|---|---:|
| model calls | 19,438 |
| tool calls | 22,084 (Bash 94%) |
| cache read | 6.2 G |
| tool results, all together | 7.4 M tokens |
| their re-read cost (Σ size × later calls) | 1.3 G — **20% of cache read** |
| mxcli invocations | 2,909: `exec` 904, `check` 512, `-c describe` 333, `syntax` 221 |
| categories | orientation 52%, write 11%, verify 10%, validate 9%, retry 3% |

**The 20% is the figure that matters for ordering the levers.** Tool results
explain about a fifth of the re-read bill; the other four fifths are the
model's own turns, prompts and system context being re-read by every call.
Lever 2 (shrink what each result adds) can only ever act on that fifth, and a
large output is cheap when it arrives late — so a byte trimmed matters less than
a call removed, which takes its whole turn's context growth with it. That is the
quadratic argument from §"Why this is a square", now with a measured split
rather than an assumed one. It needs re-measuring on app-build sessions before
it is quoted as the shape of one.

**The benchmark harness exists; the baseline does not yet.** `mxcli eval run
docs/14-eval/eval-bench-001.md --version 11.15.0` creates a fresh project with
`mxcli new` (CLAUDE.md and skills as a user gets them), runs `claude -p` on the
fixed `BENCH-001` brief — four entities, three associations, overview and edit
pages, a validated submit microflow, two module roles, navigation and a
microflow test — copies the transcript, runs the checks and writes the session
report. The first attempt to record a baseline stopped at authentication: the
headless `claude` on the development container had no usable credentials. The
baseline is the first thing to record wherever it does.

---

## Sequencing

| | Lever | Effort | Expected effect |
|---|---|---|---|
| 1 | `diag loop-report`, `diag session-report` + benchmark harness (`eval run`, `BENCH-001`) (lever 6) — **shipped, baseline recorded 2026-10-10** (§"The baseline, measured") | S | none directly — makes the rest falsifiable, and it did: the baseline overturned both candidate levers in favour of orientation cost |
| 1b | Measure the check↔build gap rate: how many builds in a real session caught something `check` did not | S | sizes the batching prize, and feeds the parity programme's queue |
| 2 | Fix `projectGates` to teach `exec`, not `check`+`exec` (lever 1) | XS | ~1 call per change, every project, immediately |
| 2b | Measure `test --attach` on 11.14; pin the bootstrap default off 11.14 | XS | removes a forced 35 s/change from new projects |
| 2c | **Make `--watch` the default invocation** in the skills and the generated gate list, wherever the version supports it | XS | the largest measured wall-time item: ~30 boots on a 11.13 project that had the warm loop and never used it |
| 2d | **Count a killed `run` in `diag loop-report` rather than dropping it** — **shipped** | S | was: the restart bill is invisible, `run`'s reported wall time a floor built from 4 of 30 invocations. Now a 30 s `session_alive` heartbeat bounds every unclosed run from below, whatever killed it, and the report separates the measured total, the measured floor and an explicitly-labelled estimate |
| 2e | **App lifecycle as commands** — `run --local --detach`, `run status`, `run wait`, `run stop`, `run restart`, taught in the run-local/run-app skills and the generated gate list — **shipped** | M | ~25–28% of all tool calls in measured sessions were hand-rolled `nohup`/poll/`pkill` loops; each becomes one call, and `exec … && run wait` is the per-change chain |
| 3 | Publish the canonical `&&` chain in `projectGates` + skills (lever 1) | XS | the 5–8 → 1–2 collapse, with nothing built |
| 3b | **Make consuming the skills cheap** — **first pass shipped 2026-10-10**: the generated CLAUDE.md now routes `syntax` before any skill, and `scripts/skill-index.py` puts a verified line-numbered section index at the head of all 74 (`make check-skill-index` in CI). Not yet done: moving the syntax that skills restate into `syntax` itself — code blocks are only 28% of skill bytes, so that is a smaller prize than it looked | M | the largest measured token item, and it displaces item 4. **Measured, unsettled**: run 2 shows the cheap path taken (ranges replaced three of four whole-file reads) with the bill up anyway and one induced sweep shape; n = 1 per side, so an A/B of 2 + 2 against `9ce90f32` is what decides keep-or-revert (§"Run 2, measured") |
| 4 | Terse/delta output for `exec` and the noisy listings (lever 2) | M | the token half of the chain win; helps every call — but the baseline puts mxcli's own output at ~15% of the re-read cost against the skills' ~68%, so this is the junior partner to 3b |
| 5 | Tiered verification rule in the skills (lever 3) | S | stops the default path at the cheapest sufficient gate |
| 6 | Subagent trigger in the skills (lever 4) | XS | caps the worst tail |
| 7 | Workarounds → diagnostics and skills (lever 5) | M, ongoing | compounds across all future sessions |

Item 1 first is deliberate. Items 2, 3, 5 and 6 are all XS-to-S and can ship
immediately after it — item 3 is now the one that changes the shape of the loop,
and it is a documentation change.

**Item 1 has now run, and it reordered this table rather than confirming it.**
The baseline put the skills, not mxcli, at the top of the token bill, which
promotes the new item 3b above item 4 and closes `mxcli apply`. That is the
instrument working as intended: the two levers this document argued hardest for
were both aimed at the smaller share. Re-read §"The baseline, measured" before
picking the next item, and remember it is one sample.

**Items 2c and 2d were added after three projects were measured, and 2c is the
largest single item in this table by wall time.** It is also the cheapest: a
default, in prose, in files that already exist. Note that 2c and 2d attack
different axes — 2c removes the boots, 2d makes the remaining ones visible — and
that 2d has to land for 2c to be claimable, because a lever that removes
uncounted time cannot be shown to have worked. Item 4 is the only substantial build, and it
is what makes item 3 pay in tokens rather than only in call count.

`mxcli apply` is deliberately **not** in this table, and as of the 2026-10-10
baseline it is **closed rather than pending**: the measurement answered its
criterion no — mxcli's output is ~15% of the re-read cost where the skills are
~68% — so the command would optimise the smaller share. Re-open it only if
`check_then_exec_pairs` shows the published chain still being composed wrong,
and even then prefer item 3 (state the chain) over a wrapper.

## What this does not fix

- The last verification tier. "Does it render correctly" needs a running app,
  and that is true of React too. The tier table keeps it rare rather than
  per-change.
- The 11.14 serve-rebuild defect. It is mxbuild's, the controls are conclusive,
  and nothing mxcli does from outside repairs it. It should be reported upstream;
  meanwhile the version default is the only lever we hold **on that version** —
  on versions where the warm loop works, the restart is ours to remove (item 2c),
  and a 11.13 project measured 30 restarts it did not have to take.
- Scope. The reported sessions did not build the same thing.
- MDL not being in training data (`PROPOSAL_llm_mdl_assistance.md` owns that).
  It is row 2 of the asymmetry table and the one property here that is not
  engineering. Every MDL statement wrong on the first try is a full loop
  iteration, so that proposal and this one multiply rather than overlap — a
  first-attempt success rate is a call-count lever in disguise.

Note what has moved OUT of this list since the first draft: "Mendix builds, and
that is a per-change tax forever." It is not. It is a per-change tax for as long
as `check` is not trusted enough to batch the build behind it, which is a
programme already running.
