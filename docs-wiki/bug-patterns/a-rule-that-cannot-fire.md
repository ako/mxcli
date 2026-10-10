---
title: A Rule That Cannot Fire
category: bug-pattern
last-synced: ce063188
covers:
  - mdl/linter
sources:
  - .claude/skills/fix-issue/findings/
  - mdl/linter/context.go
  - mdl/catalog/lint_rule_vocabulary_test.go
  - .claude/skills/mendix/write-lint-rules.md
---

> **Do not duplicate**: what each rule checks is in `mxcli lint --list-rules`;
> how to write one is in `.claude/skills/mendix/write-lint-rules.md`; each fix is
> in the findings. This page is about the two ways a rule's verdict stops meaning
> anything, and why they are the same class.

## What this is

A lint rule has exactly two failure modes that matter, and they are mirrors.
It can report **nothing** when it should fire — indistinguishable from a clean
project, and indistinguishable from a rule that ran. Or it can fire on **correct
code** — at which point the reader stops believing the rule, which costs the same
as the first failure plus the time they spend acting on it.

The `mdl/linter` findings are almost entirely one or the other. Neither announces
itself: in both directions the run exits successfully and the report looks
authoritative.

## How it fits

**"No issues found" has two readings and the command used to print both.** A rule
that never loaded, an id that matched nothing, a `.claude/lint-rules/` directory
that was not found, an iterator whose query broke, a stale catalog missing the
table the rule reads — every one of those produced a clean report. The remedies
are all the same shape: make the absence loud. Iterators still degrade to "no
rows" so one broken view cannot take down a run, but they now *record* the
failure and the command exits non-zero; an unknown `-r` id is an error naming the
directory rules came from. **Silently passing CI on a run that could not read the
model is the worst available outcome**, and it was reachable from five directions.

**The vocabulary a rule matches on is not the vocabulary it reads.** CONV010
listed Mendix *storage* names where the catalog reports *SDK* names, so its
allowlist matched nothing and it flagged every ACT_ microflow that showed a page
— eleven false positives out of thirteen findings. The deeper point is that
**the catalog deliberately does not use storage names**, which makes this trap
specific to rule authors and invisible to everyone else. Pinning the allowlist to
what the labeller actually returns is what stops the drift, and
`lint_rule_vocabulary_test.go` exists for exactly that.

**An allowlist is a liability that grows.** That same list has since been short
three more times: a missing `ExclusiveMerge` — which a permitted `if`
necessarily creates — fired 122 times on one project, and a missing
`NanoflowCallAction` meant an ACT_ nanoflow could satisfy the rule in **no way at
all**. A rule that cannot be satisfied does not read as a broken rule; it reads
as broken code, so authors refactor around it or patch their local copy and the
defect never returns upstream. One only came back because a reporter wrote
"report upstream" in their findings.

**Fix the document the author read, not only the rule that was reported.** The
storage-name defect was found, fixed and pinned — then recurred, because the
*skill the author copied from* was never corrected. A rule pinned to the labeller
and a doc that is not is one fix, not two.

**A rule's scope is a correctness property, not a performance one.** Rules that
need the full catalog returned nothing on a fast build — `widgets()` yielding 0
rows against 46 — and that silence was per-builtin, so a project rule looked
clean. The guard is generated rather than hand-maintained: build both depths, run
every builtin on each, and require anything that differs to be classified.
Scoping is the same story at the other end: `lint` could narrow by module and by
rule but not by document, which made a per-document question cost a project-wide
run and pushed the gate to once per session.

**Platform-specific rules need their platform in the predicate.** MPR010, MPR012
and MDL-WIDGET-class checks fired on native pages where mxbuild builds the same
content clean — and following one rule's advice produced a *new* consistency
error. That last part is the method worth keeping: **measure the advice, not only
the warning**. It settles "does this rule apply here" faster than reasoning about
how the client renders.

**A rule naming a database effect must enumerate the actions that have it, not
the action named after it.** CONV011 looked for the commit *activity* and missed
`change … commit` and `create … commit`, where commit is a property on another
action. The generalisation: grep the action types for the field before trusting a
rule's coverage.

**A check-time rule guards only what passes through check.** Anything writable
around it — an older binary, `--no-check`, Studio Pro, a hand edit — needs a
model-level twin. When a check rule is added for a runtime-only failure, the
question to ask immediately is whether the same defect can already be *in*
models; if so the lint half ships with it. That is the seam this page shares with
[[check-mxbuild-drift]].

**The fixture that encodes the bug cannot detect it.** QUAL005 missed eleven real
translation gaps because the test harness synthesised the very id whose absence
was the defect, giving every sibling the same value. Elsewhere unit tests set an
activity field by hand and so passed against a shape the reader never produces.
Build test objects the way the production path builds them, and prefer the real
schema builder to a hand-rolled `CREATE TABLE` — a test double drifts from the
schema silently, and the assertion text for "the query broke" is identical to
"the filter is too strict".

**An iterator serving three document types must derive the noun.** `ctx.Microflows()`
yields microflows, nanoflows and rules, so a literal "Microflow" in a message
reported a rule as a microflow — into the JSON and SARIF `documentType` too.
Widening a shared iterator is not a compile error anywhere it is formatted.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  rules, their vocabularies and their controls
- [write-lint-rules skill](../../.claude/skills/mendix/write-lint-rules.md) — the
  doc whose correctness is part of this class
- [[absent-edge-reads-as-absent-fact]] — the index these rules read, and its own silence
- [[misleading-diagnostics]] — the same cost shape for parser hints
- [[check-mxbuild-drift]] — why some rules need a model-level twin
