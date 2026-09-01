---
title: DESCRIBE Round-Trip Gaps
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/cmd_workflows.go
  - mdl/executor/cmd_pages_describe_pluggable.go
---

> **Do not duplicate**: the per-construct fix recipes live in the findings
> (`grep -l describe .claude/skills/fix-issue/findings/*.jsonl`), the MDL syntax
> in `docs/01-project/MDL_QUICK_REFERENCE.md`, and the round-trip requirement in
> CLAUDE.md's PR checklist. This page describes the failure class only.

## What this is

`DESCRIBE` is a **second implementation of MDL**, written in the opposite
direction and validated by nothing. Every construct has a writer (MDL → BSON)
and a describer (BSON → MDL) built separately, and no mechanism forces them to
agree. This is the single largest class of defect in the executor: **83 of the
248 findings** for `mdl/executor` involve a describe path.

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

**Four shapes, in increasing order of how long they survive.**

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

*Destroys.* Rarest and worst — the round trip removes structure rather than
losing a field. A list view's specialization templates went 4 → 0; an accordion
group's contents vanished; a loop body was emitted empty because an annotation
sat to its left. `mx check` reported 0 errors on both sides of each.

**The defect is usually the copy, not the case.** Describers get written
per-widget and per-container, so one lookup exists four or five times and some of
the copies are wrong. Patching the switch named in the report leaves the others to
drift again. A datasource means the same thing wherever it sits; so does an
action slot, and so does a text template. The fix that holds is one reader and one
renderer, with the type set taken from `generated/metamodel` rather than from the
copies.

**Fixing one half is worse than the bug.** Where a describer has two defects at
once — say, quoting *and* a missing property — shipping the quoting fix alone
turns unparseable output into output that parses cleanly while silently dropping
something. That is strictly worse: a wrong page that validates.

**The round trip is the only sound test**, and it has to be measured against the
model rather than the text. A describe-to-describe text diff matched exactly on a
page that had lost its datasource. `mx check` on the source proves nothing,
because the lossy model is valid. Counting failures before and after hid an
equal-sized swap of which pages failed. What works: describe → exec → describe
byte-identical, plus `mx check` on the *rebuilt* model.

One consequence worth knowing: other code re-parses DESCRIBE output.
`use building block … (datasource: …)` matches against the rendered form, so
fixing a renderer can break a consumer that never reads the model. Grep for
callers of the emitter before changing what it emits.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-construct
  recipes; `grep -l describe *.jsonl` reaches this class
- [[widget-type-object-drift]] — the neighbouring class where the *written* widget
  is wrong rather than the described one
- `.claude/skills/verify-in-runtime.md` — for the cases where neither the model
  nor its description is the thing that is wrong
