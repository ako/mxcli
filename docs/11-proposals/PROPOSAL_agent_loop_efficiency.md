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

**`mxcli diag loop-report`** reads a session's log and prints: invocations by
verb, wall time by verb, `check`-immediately-before-`exec` pairs (pure waste),
restarts vs. hot reloads, and output bytes per command. That converts "the loop
feels expensive" into a ranked list of where the calls actually went — and, run
before and after each lever, into evidence that a lever worked.

**Then a benchmark.** One fixed app-brief, run end to end, recording model calls
and tokens. Without it, every claim here is an argument; with it, each lever
lands or does not. It also guards the result: the gate list drifted in three
places once already, and a loop regression is exactly as invisible.

---

## Sequencing

| | Lever | Effort | Expected effect |
|---|---|---|---|
| 1 | `diag loop-report` + benchmark harness (lever 6) | S | none directly — makes the rest falsifiable |
| 1b | Measure the check↔build gap rate: how many builds in a real session caught something `check` did not | S | sizes the batching prize, and feeds the parity programme's queue |
| 2 | Fix `projectGates` to teach `exec`, not `check`+`exec` (lever 1) | XS | ~1 call per change, every project, immediately |
| 2b | Measure `test --attach` on 11.14; pin the bootstrap default off 11.14 | XS | removes a forced 35 s/change from new projects |
| 3 | Publish the canonical `&&` chain in `projectGates` + skills (lever 1) | XS | the 5–8 → 1–2 collapse, with nothing built |
| 4 | Terse/delta output for `exec` and the noisy listings (lever 2) | M | the token half of the chain win; helps every call |
| 5 | Tiered verification rule in the skills (lever 3) | S | stops the default path at the cheapest sufficient gate |
| 6 | Subagent trigger in the skills (lever 4) | XS | caps the worst tail |
| 7 | Workarounds → diagnostics and skills (lever 5) | M, ongoing | compounds across all future sessions |

Item 1 first is deliberate. Items 2, 3, 5 and 6 are all XS-to-S and can ship
immediately after it — item 3 is now the one that changes the shape of the loop,
and it is a documentation change. Item 4 is the only substantial build, and it
is what makes item 3 pay in tokens rather than only in call count.

`mxcli apply` is deliberately **not** in this table. It is contingent on item 1
showing that the published chain is still being composed wrong.

## What this does not fix

- The last verification tier. "Does it render correctly" needs a running app,
  and that is true of React too. The tier table keeps it rare rather than
  per-change.
- The 11.14 serve-rebuild defect. It is mxbuild's, the controls are conclusive,
  and nothing mxcli does from outside repairs it. It should be reported upstream;
  meanwhile the version default is the only lever we hold.
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
