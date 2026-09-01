---
title: When `mxcli check` and mxbuild Disagree
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/validate_program.go
  - docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md
---

> **Do not duplicate**: the rule catalogue and its rationale live in
> `PROPOSAL_check_mxbuild_gap_heuristics.md`; each rule's exact predicate and CE
> number live in the findings and in `mdl/executor/validate_*.go`. This page
> describes the failure class on both sides.

## What this is

`mxcli check` is a **model of mxbuild**, not a port of it. It runs before a
build — often before a project even exists — so every rule is a prediction, and
predictions drift in two directions:

- **A gap.** `check` passes, `exec` writes, and the build then fails with a `CE`
  code. The user has already changed their project before anything told them.
- **A false refusal.** `check` rejects MDL that mxbuild accepts at 0 errors.

Both appear repeatedly in the executor findings. The second used to be a mere
annoyance and is not any more: since `exec` began refusing scripts whose check
reports an **error**, a false positive is a blocker rather than a warning.

## How it fits

**Verify a rule against mxbuild, not against intuition.** Rules have been added
on a plausible reading of a CE code and later measured to be wrong: one flagged
format functions over association navigation as a build error and was deleted
outright; another demanded a `return` on every path from a microflow that builds
cleanly, because the builder synthesises one. A rule that predicts mxbuild has to
be checked against mxbuild.

**A rule reproduced from failing neighbours can still be mis-premised.** The
deleted rule was written after reproducing several failures that shared the
construct it flagged — and the construct was not the cause. They shared a
*different* hidden defect, in the write path. When a write-path fix lands,
re-validate the checks that were derived from the same symptoms; a
correlation-based rule outlives the correlation.

**Mirror the builder's own condition rather than inventing a second one.** Where
`check` predicts something the builder decides, the two conditions must be the
same expression, not two readings of the same intent — otherwise they drift on
the first edit to either.

**Run a new rule over `mdl-examples/` before wiring it up.** One candidate hit 4
of 374 example files and 3 of the hits were false positives, because the rule
read the AST while the outcome depended on what the builder synthesises. The
corpus is the cheapest false-positive test available.

**Severity is the design decision, not an afterthought.** A rule whose vocabulary
cannot be proven complete — anything about widget properties, or about a name
that might be legal in a context the rule cannot see — must be a *warning*, or
it trades a silent defect for a false refusal. That only works if warnings
genuinely do not block: an exec guard written as `if len(violations) > 0` makes
every warning fatal, which is how a warning-severity rule silently became a
blocker for everything it touched.

**Close a gap in both passes or in neither.** A check-time rule does not protect
a script that runs `exec` directly, and `--no-check` exists. Both call the same
function so they cannot diverge — the convention exists because they did.

**When probing a gap, probe every sibling.** A missing reference check is almost
never one reference kind: the workflow case turned out to have *nothing*
validated — called microflow, called workflow, user task page, targeting
microflow, context entity, and the workflow's own module. Fixing the reported one
leaves the class open and the next report looks new.

**Grade by consequence.** The same statement can produce a recoverable build
error or an unopenable project depending on one detail — a qualified-but-missing
name versus an unqualified one. Those want different gates: the first needs a
project and belongs in the reference check; the second is a static property of
the statement and can be refused with no project at all, which is also what makes
it testable as a `.fail.mdl` fixture in CI.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — every rule's
  predicate, its CE code, and the measurement behind it
- [[unloadable-model-writes]] — the failures that have no CE code because the
  build never gets that far
- `PROPOSAL_check_mxbuild_gap_heuristics.md` — the design rationale for
  predicting mxbuild at all
