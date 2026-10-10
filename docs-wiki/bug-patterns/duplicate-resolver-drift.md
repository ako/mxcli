---
title: One Question, Two Answers
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/validate_program.go
  - mdl/executor/validate_duplicates.go
  - mdl/executor/widget_attribute_scope.go
  - mdl/backend/backend.go
---

> **Do not duplicate**: the individual resolvers and their fixes live in the
> findings; the backend-abstraction rationale is canonical in ADR-0002 and
> [[backend-abstraction]]. This page describes the shape.

## What this is

The same question — *does this name resolve? what entity is this? is this
property set?* — gets answered in more than one place, the answers differ, and
the disagreement surfaces as a defect in whichever consumer asked the losing
copy. Roughly twenty-three executor findings are this shape, which makes it the
most common *structural* cause in the area.

It is worth naming separately from the symptoms it produces, because the
symptoms look unrelated to each other. A forward reference accepted by `check`
and rejected by `exec`, a guard that works on one engine and is inert on the
other, a widget property read by one describer and not its twin — all the same
defect wearing different clothes.

## How it fits

**Five places the duplication keeps appearing.**

*`check` versus `exec`.* The two passes historically ran different validator sets
over the same script, so `check` could reject what `exec` wrote and `exec` could
write what `check` rejected. They also disagreed about *order*: reference
validation collected every definition in the script up front, making forward
references invisible, while `exec` resolves in statement order — so "Check
passed!" and an exec failure on the same file, with earlier statements already
written.

*Legacy versus modelsdk.* A guard that walks stored BSON can be correct on one
engine and a silent no-op on the other, because the document reaches it in a
different shape. The failure is invisible from either side alone: the test suite
is green, the guard reports nothing, and nothing distinguishes "no violations"
from "never ran".

*Write path versus read path.* A property written correctly and read back by
nobody is the [[silent-property-drop]] class; a property read by one describer
and not by its twin is this one. Several fixes here amount to deleting a copy —
one reader, one renderer, one resolver — and taking the type set from
`generated/metamodel` rather than from whichever copy the report named.

*Per-doctype copies.* A behaviour implemented per document type drifts by
construction. The folder clause is the clean example: every doctype's `FOLDER`
handling had the same bug, and the report that named one of them read as a
doctype-specific defect.

*Two walks over one tree.* The duplication does not need two *resolvers* — two
traversals of the same structure drift the same way, and faster, because each
hand-enumerates the branches it descends into. Both walks over a workflow's
activity tree carried their own switch over `ConditionOutcome`; each was missing
a different subset (one skipped enum outcomes and every boundary-event body, the
other only the boundary events), so a `call microflow` in a decision's enum
branch went unwired while the same statement in the main flow was fine. The
interface already exposed the accessor the switches were standing in for
(`GetFlow`), which is the tell for this variant: a hand-written switch over an
interface's implementations answers a question the interface answers already.

*A preview versus the command it previews.* `mxcli diff` predicted exec by
rendering each statement as MDL and comparing it with the stored document's
description. Every normalisation exec applies — implicit defaults, positions,
the splice verdict, write elision — had to be re-derived by that renderer, and
each round of fixes (#997, #794, #839) synchronised one more spot while the
next rehearsal found phantoms on 66 scripts (#907). The remedy was the general
one taken literally: diff now runs exec itself on a scratch copy and compares
units, so there is no second answer left to drift.

*A hand-kept list of statement kinds.* The most prolific variant found this
cycle, because a `switch` over AST types is how everything in the executor is
wired. `check`'s project tier resolves the module of every module-scoped
`CREATE` — except the kinds whose `case` was never added, and the absence of a
case is indistinguishable from a deliberate skip. The same thing happened five
times over for "statements that carry an `ON ERROR` clause": one shared adapters
table existed *precisely because* per-walk lists drift, its comment said so, and
a fifth copy was still written. So for a check-tier gap, **grep the switch for
the AST type before reading any resolver** — the missing `case` is usually the
whole bug, and the resolver it would have called is already correct.

**A guard that compares two hand-maintained lists only catches drift between
them.** The test asserting that every create statement is project-checked
compared the two switches *with each other*, so a doctype absent from both
passed forever — fifteen of them did. A drift guard needs a third, independent
source: the AST package's own declarations, `generated/metamodel`, a `go/parser`
scan of the builders for the keys they read. The reliable version of this test is
not "the lists agree" but "the list agrees with something nobody edits".

**The tell is that the fix for the reported instance is obviously incomplete.**
When a symptom's cause is "this switch was missing a case", the next question is
how many other switches answer the same question — the answer has repeatedly been
two, three or five. Patching the named one closes the report and leaves the class
open, so the next instance arrives looking new.

**Where one answer must be shared, make it a function of plain data.** The
binding-scope rule for a widget attribute is the clean example: it takes the
property's declared link, the enclosing entity and the resolved datasources as
arguments and returns the entity and *why*, so `exec` feeds it the engine's live
state, the validator feeds it the statically-known state, and neither re-derives
the rule. A test asserts the exec error contains the check message verbatim,
which is the cheapest way to keep two callers of one function honest about
reporting the same verdict.

**The durable remedy is to remove the second answer**, not to synchronise the two.
Route both callers through one function; take the enumeration from the metamodel
rather than a hand-maintained copy; make the check and the exec guard *the same
function* so they cannot diverge. Where a second implementation genuinely has to
exist — the two engines — the protection is a test that asserts the *structure*,
such as every exported validator being reachable from the one entry point, rather
than a test per rule.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  resolvers, and which copy was wrong
- [[check-mxbuild-drift]] — the special case where one of the two answers is
  mxbuild's
- [[backend-abstraction]] — why two engines exist at all
