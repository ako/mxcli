---
title: One Question, Two Answers
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/validate_program.go
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

**The tell is that the fix for the reported instance is obviously incomplete.**
When a symptom's cause is "this switch was missing a case", the next question is
how many other switches answer the same question — the answer has repeatedly been
two, three or five. Patching the named one closes the report and leaves the class
open, so the next instance arrives looking new.

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
