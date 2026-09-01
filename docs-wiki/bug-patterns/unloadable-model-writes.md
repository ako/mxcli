---
title: Writes That Make the Project Unloadable
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/validate_workflow.go
  - mdl/executor/validate_association_module.go
---

> **Do not duplicate**: the per-construct refusals and their CE numbers live in
> the findings; the storage-name and overlay rules are canonical in CLAUDE.md.
> This page describes the failure class and why it is graded differently from a
> build error.

## What this is

A Mendix model can be wrong in two ways that look nothing alike. **Validation**
errors — the `CE####` codes — are found by mxbuild in a model it has already
loaded successfully: one document is wrong, the rest of the project is fine, and
the message names the document. **Load** errors happen before validation starts.
The `.mpr` is structurally malformed, `mx check` dies with a .NET exception and
no error code, no document is named, and **Studio Pro will not open the project
at all**.

Twelve of the executor findings are in the second category. They are the most
expensive defects mxcli has produced, because the blast radius is the whole
project rather than one page, and because the diagnostic is a stack trace.

## How it fits

**The recurring cause is a reference whose *shape* is wrong**, not a reference
that points at the wrong thing. Mendix reconstructs each stored property into a
typed identifier as it loads, and a value that cannot be parsed into that type
takes the loader down. The shapes seen so far: a one-qualifier member name
written where an attribute reference is expected (an attribute is bare or
`Module.Entity.Attribute`, never `Module.Name`); an unqualified entity name in a
generalization; a literal string where the property is a `ConstantIdentifier`;
an empty `DestinationEntity`; an index column pointing at a GUID that no longer
exists; a sequence flow dangling from a `break`; an association whose `ParentPointer`
addresses an element in another unit; a child appended to a list whose parent
type has no constructor taking that parent.

**`mx check` passing is not evidence that the project opens.** mxbuild's
deserializer tolerates properties it does not recognise; Studio Pro resolves
every stored property against its type's property list and throws when there is
no match. A settings write with three wrong storage names built at 0 errors and
made the unit unopenable. Where a write touches preserved BSON, only a real
Studio Pro reference document settles the shape — which is the reasoning behind
CLAUDE.md's overlay rules.

**The remedy is almost always to refuse, not to write something else.** In each
of these the tempting alternative turned out to be unavailable or harmful:
minting a Constant from a literal password would bake a secret into the model as
a design-time default; a cross-module association has no storage form at all
(neither `Association` nor `CrossAssociation` can express a by-name *parent*);
the correct container for a floating workflow annotation is not determinable from
the gen model. Refusing costs the user one statement; writing costs them the
project.

**Refuse in both passes.** A check-time rule does not help a script that never
ran `check`, and `exec` is reachable directly — the lesson that has now been
relearned enough times to be a convention. Both call the same function, so they
cannot drift.

**Grade the two failure modes separately when a statement can produce either.**
A qualified-but-missing name is recoverable: the project loads and the build
reports `CE1613`. The *unqualified* form of the same name makes the `.mpr`
unopenable. The first needs a project to detect and belongs in the reference
check; the second is a static property of the statement and can be refused with
no project at all — which is also what makes it catchable by a `.fail.mdl`
fixture in CI.

**Diagnosis without Studio Pro.** The .NET stack names a setter, and that
setter's property is the lead — `EntityRefStep.set_AssociationId`,
`MprProperty.cs`, `ResolvePostponedProperties`. From there, dump the stored unit
and compare the property against a document Studio Pro wrote. Verifying a
storage-name hypothesis by swapping *only* the name is the cheap control: if the
error is byte-identical afterwards, the bug is somewhere else.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  refusals, their rule IDs and their reference documents
- [[element-identity]] — why a pointer's target and a GUID are different questions
- [[association-pointers]] — the `ParentPointer` / `ChildPointer` inversion behind
  the cross-module association case
- `.claude/skills/debug-bson.md` — the workflow for comparing a written document
  against a Studio Pro reference
