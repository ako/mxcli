---
title: DESCRIBE Round-Trip Gaps
category: bug-pattern
last-synced: 643271e8
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - mdl/executor/cmd_workflows.go
  - mdl/executor/cmd_pages_describe_pluggable.go
  - mdl/executor/cmd_microflows_show.go
  - mdl/executor/cmd_microflows_show_crossed.go
  - mdl/executor/cmd_microflows_normalize.go
  - mdl/microflowgraph/structure.go
  - docs/11-proposals/PROPOSAL_structured_microflow_description.md
  - docs/13-decisions/0008-identity-and-idempotence.md
---

> **Do not duplicate**: the per-construct fix recipes live in the findings
> (`grep -l describe .claude/skills/fix-issue/findings/*.jsonl`), the MDL syntax
> in `docs/01-project/MDL_QUICK_REFERENCE.md`, the round-trip requirement in
> CLAUDE.md's PR checklist, and the irreducible-graph design in
> [its proposal](../../docs/11-proposals/PROPOSAL_structured_microflow_description.md).
> This page describes the failure class only. Counts are computed
> (`make digest-status`, or grep the findings), never quoted here.

## What this is

`DESCRIBE` is a **second implementation of MDL**, written in the opposite
direction and validated by nothing. Every construct has a writer (MDL → BSON)
and a describer (BSON → MDL) built separately, and no mechanism forces them to
agree. It is the single largest class of defect in the executor — roughly two
findings in five for `mdl/executor` touch a describe path.

The reason it accumulates is that the write path has an oracle and the read path
does not. `mxbuild` validates the *model*, and it never sees DESCRIBE output at
all — so a describer that drops a property produces a perfectly valid model, at
0 errors before and after. The only thing that notices is a person replaying the
output and finding their work gone.

That matters most where the feature is used most. `describe → edit → exec` is
mxcli's copy operation, and `DESCRIBE LAYOUT` emitting re-executable MDL is
explicitly why there is no `COPY DOCUMENT` verb. A lossy describer is costliest
exactly when someone is trying to reuse work.

## How it fits

**Five shapes, in increasing order of how long they survive.**

*Won't parse.* The emitter produces text MDL's own grammar rejects — a
reserved-word name emitted bare, an internal spelling (`call_microflow X`,
`ReadMode: CallMicroflow:…`) that no rule accepts, `Param = $v` where the
grammar wants `Param: $v`, a quote inside a string that was never doubled. Loud
and cheap: running `mxcli check` on the output finds it. The recurring cause is
that **storage form is not input form** — whenever DESCRIBE prints a value read
back from the backend, the question is whether the *parser* accepts that
spelling, and nothing else in the toolchain asks it.

*Silently drops.* A property is written correctly, present in the `.mxunit`, live
in the app — and absent from the description. The round trip deletes it. Output
parses, executes, and the model stays valid, so every automated signal is green.

*Invents.* The describer emits a clause the author never wrote: `comment 'Review'`
on a jump whose caption was only ever a default, `on error rollback` on an
activity with no error handling, a bare `else` on a split that has none. Each
round trip accumulates another, so the model drifts toward the emitter's
defaults. The governing rule is an **asymmetry**: omitting a value the writer
re-derives is lossless, while emitting it is lossy in the direction that
matters — it puts something in the user's script that they did not write. When a
formatter renders an enum whose fallback value is also a legal authored value,
read-back cannot invert the write; render only the values that are never
defaults.

*Destroys.* The round trip removes structure rather than losing a field. A list
view's specialization templates went 4 → 0; an accordion group's contents
vanished; a loop body was emitted empty because an annotation sat to its left.
`mx check` reported 0 errors on both sides of each.

*Means something else.* The worst, and the one that hides longest, because every
other shape leaves evidence. Here the description parses, executes, yields a
valid model, drops nothing and invents nothing — and **denotes a different
program**. A microflow whose activity ran on `¬c1 ∨ c2` described as `c1 ∧ c2`;
with the reporter's actual expressions, one that always logged described as one
that never did. Nothing in the toolchain can see it: there is no missing field to
notice and no error to raise, so the only detector is someone comparing
behaviour.

**Some documents cannot be described faithfully at all**, and that is a different
diagnosis from a careless describer. MDL's `if` is a single-entry/single-exit
block; a Mendix microflow is an arbitrary directed graph. When branches cross, no
amount of care in the describer helps, because the target language cannot spell
the graph — the remedy is to **extend the language** (named join points) rather
than to fix a bug. One sub-class provably has no faithful rendering at any
effort: per Böhm–Jacopini, nesting genuinely interleaved branches requires either
duplicating an activity the user drew once or inventing a boolean they never
wrote, and both are model rewrites rather than descriptions. Recognising that a
gap is a *vocabulary* limit tells you whether to reach for a fix or a proposal.

**The defect is usually the copy, not the case.** Describers get written
per-widget and per-container, so one lookup exists four or five times and some of
the copies are wrong. Patching the switch named in the report leaves the others to
drift again. A datasource means the same thing wherever it sits; so does an
action slot, and so does a text template. The fix that holds is one reader and one
renderer, with the type set taken from `generated/metamodel` rather than from the
copies. The same logic applies to a second *rendering mode*: build it by rewriting
the graph into one the existing describer already handles, never by writing a
second describer, or every activity renderer exists twice and drifts.

The copies are not always in different files, and that is what makes this one
hard to see: one file held two renderers of the same document header, so a clause
added to one of them worked through `diff-local` and printed nothing under
`describe`. Grepping for the *call site* finds nothing wrong — the tell is
counting how many functions already emit the construct (`grep -c` returning 2),
before adding to either. The same instinct applies to the question the copies
are answering: whether a widget has two datasources is a property of the **stored
document**, not of which per-widget extractor happened to run, so the detection
has to run unconditionally rather than behind the fallback's `if nothing found`
guard. [[duplicate-resolver-drift]] is the structural cause in general; this is
its describe-side face.

**DESCRIBE's own comments are a to-do list.** Where the describer cannot state a
construct it emits a marker — a bare `-- [Type$Name]` line, a "not re-executable"
note — which reads as honesty and is one, but it is also the only inventory
anyone keeps of what a rewrite from that output will delete. The describer's
generic fallback decides what to comment; a separate guard decides what a rewrite
must refuse to drop; nothing holds the two lists against each other. The rule
that falls out belongs to [[rewrite-drops-unauthored-state]], where the loss
lands — what belongs here is that the markers exist, and that emitting one is a
report, not a resolution.

**Fixing one half is worse than the bug.** Where a describer has two defects at
once — say, quoting *and* a missing property — shipping the quoting fix alone
turns unparseable output into output that parses cleanly while silently dropping
something. That is strictly worse: a wrong page that validates.

**mxcli round-tripping its own output proves nothing about this class.** It is
the single most expensive measurement mistake here, and it is not a matter of
degree: mxcli writes the document and mxcli describes it, so every constant the
writer hardcodes agrees with itself and every inference the describer makes was
reverse-engineered from documents built under that inference. Measured on a page
round trip, the bug-test reported the healthy answer against the **unfixed**
build. A round trip over mxcli's own corpus is not a weak test of faithfulness;
it is not a test of it at all.

Two references restore the signal, and which one you need depends on where the
describer's belief came from. Where it encodes a value — a canvas width, a
default action flag — the reference must be a document **Studio Pro** wrote; a
committed CI fixture only counts if it is one, and the hardcoded value that looks
safe usually is not (one canvas width matched 4 of 67 real pages, so the round
trip moved the other 63). Where it encodes an inference rule — "an empty error
handler means the path falls through to here", "there is only one association
that could reach this entity" — no existing document disproves it, because the
corpus agrees with it by construction; the case has to be **constructed**,
by editing a stored document into the reading the rule gets wrong and
re-describing. That edit is often cheaper than it sounds: two names of equal
length can be swapped in the stored BSON without resizing anything. The real
microflow that motivated all of this round-tripped correctly by luck.

Four further measurement rules, each of which hid a defect until it was applied:

- **Let write elision answer the question.** Because storage refuses to write a
  unit whose rebuild is semantically equal to what is stored
  ([ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md)),
  replaying a description is self-scoring: the executor reports `Unchanged` when
  the round trip was faithful and `Replaced` when anything differs — including
  the differences no error and no `mx check` will ever mention. It is the nearest
  thing this class has to an oracle, and it costs one run. Its blind spot is
  exactly the trap above: over mxcli's own output it reports `Unchanged` whether
  or not the describer is right, so the document under test has to be one mxcli
  did not write.
- **Compare identities, not counts.** A count cannot tell "preserved" from
  "deleted and recreated", and it hid an equal-sized swap of which pages failed.
- **Diff the whole corpus against a baseline binary.** Keep a pre-change build,
  describe every document with both, and diff. A pure refactor must come out
  byte-identical; a feature must change only the documents it targets. Anything
  else moving is the bug — this is what caught a warning being retired on the
  strength of a label nothing ever emitted.
- **Algebra is not behaviour.** When a change reasons about equivalence, the
  proof is two documents side by side on a real runtime over the whole input
  space, not a convincing derivation. See `.claude/skills/verify-in-runtime.md`.

One consequence worth knowing: other code re-parses DESCRIBE output.
`use building block … (datasource: …)` matches against the rendered form, so
fixing a renderer can break a consumer that never reads the model. Grep for
callers of the emitter before changing what it emits.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-construct
  recipes; `grep -l describe *.jsonl` reaches this class
- [[flow-graph-geometry]] — the sibling class where the generated graph's
  coordinates and wiring are wrong, rather than its rendering
- [[widget-type-object-drift]] — the neighbouring class where the *written* widget
  is wrong rather than the described one
- [[silent-property-drop]] — the write-side twin of *silently drops*
- [[rewrite-drops-unauthored-state]] — where a description's unsayable constructs
  are actually lost, on the rewrite that replays it
- [[duplicate-resolver-drift]] — the structural cause behind *the defect is
  usually the copy*
- `.claude/skills/verify-in-runtime.md` — for the cases where neither the model
  nor its description is the thing that is wrong
