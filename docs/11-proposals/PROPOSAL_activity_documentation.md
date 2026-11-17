---
title: Activity Documentation in MDL
status: draft
date: 2026-11-17
---

# Proposal: Activity Documentation in MDL

**Status:** Draft
**Date:** 2026-11-17

## Problem Statement

Studio Pro lets a developer document every activity, decision and loop in a
microflow or nanoflow (the *Documentation* field in each object's properties).
MDL cannot express that text: no statement annotation writes it and `describe`
does not print it. Every path that rebuilds a flow object from MDL therefore
writes it empty.

Measured on 2026-11-17 (`main`, `modelsdk` backend, Mendix 9.24 test project
`modelsdk/mpr/testdata/v1-project`). Documentation was written directly onto
one `log` activity and one decision, as Studio Pro would, and then:

| Script | Activity documentation | Decision documentation |
|---|---|---|
| `describe` output re-run as `create or modify`, unchanged | kept | kept |
| same, the documented `log` activity's message text edited | **lost** | kept |
| `describe` output re-run as `drop` + `create` in one script | **lost** | **lost** |

The first row is the control: an unchanged statement is matched and nothing is
written. The second and third rows are the defect class of
[rewrite-drops-unauthored-state](../../docs-wiki/bug-patterns/rewrite-drops-unauthored-state.md):
`create or modify` splices a changed statement as a *replace* of the stored
activity (ADR-0012 decision 3, `mdl/executor/cmd_flow_modify.go`), and the
replacement is built from MDL that has nowhere to state the documentation.

Reported upstream as `mendixlabs/mxcli#1871` (mirrored as #2328): a `retrieve`
replaced by a `call microflow` at the same position loses its documentation.
The reporter asked for either an implicit carry or a way to write it in MDL.
This proposal does both, because neither is enough alone:

- **The annotation alone** fixes `drop` + `create` and lets a script restate
  the text, but an edit made without restating it still loses the
  documentation silently. That is the case that bit the reporter.
- **The carry alone** fixes in-place edits, but `drop` + `create`, `create or
  replace` under mdl 0, and generated flows still have no way to state the
  text, and `describe` keeps hiding it from anyone reviewing a flow as MDL.

Who benefits: teams who maintain Studio Pro-authored flows with mxcli scripts.
Their documentation is the explanation a reviewer reads in Studio Pro, and
today it disappears without a warning.

## BSON Structure

No new document type. The property already exists, and mxcli already reads it
for every object below except end events, and writes it for the same objects.

| `$Type` | Property | Type | mxcli reads | mxcli writes |
|---|---|---|---|---|
| `Microflows$ActionActivity` | `Documentation` | string | yes | yes (from the SDK struct) |
| `Microflows$ExclusiveSplit` | `Documentation` | string | yes | yes |
| `Microflows$InheritanceSplit` | `Documentation` | string | yes | yes |
| `Microflows$LoopedActivity` | `Documentation` | string | yes | yes |
| `Microflows$EndEvent` | `Documentation` | string | no | **always `""`** (`mdl/backend/modelsdk/microflow_write.go`, `case *microflows.EndEvent`) |

- Storage name equals qualified name for all five types; none appears in the
  CLAUDE.md storage-name table. Verified against `modelsdk/gen/microflows/types.go`
  (`property.NewPrimitive[string]("Documentation", …)`) and by the direct-write
  control above, which Studio Pro-equivalent bytes round-tripped through.
- Nanoflows use the same `Microflows$*` object types, so the same code covers
  both.
- Plain text with `\r\n` line breaks as Studio Pro writes it. No pointers, no
  identity, so the GUID and `$ID` rules in CLAUDE.md do not apply to this
  property itself. They do apply to the replace that carries it, which must keep
  doing what it does today.
- Still to verify against a Studio Pro-authored flow (see Open Questions):
  whether Studio Pro omits the key or writes `""` when the field is empty. The
  writer must match, or an unchanged flow is no longer byte-equal (CLAUDE.md
  rule 4, `canon.CompletePropertySets`).

## Proposed MDL Syntax

A statement annotation, alongside the existing `@position`, `@caption` and
`@color`:

```sql
create or modify microflow Sales.ACT_ApproveOrder ($Order: Sales.Order)
begin
  @documentation 'Only approved orders reach the warehouse queue.
Rejected orders stay in Sales for the account manager.'
  @position(300, 200)
  retrieve $Customer from $Order/Sales.Order_Customer;

  @documentation 'Credit limit is checked here, not in the UI.'
  if $Customer/CreditLimit < $Order/Total then
    ...
  end if;
end;
```

Rules:

1. **Absent means "keep what is stored", empty means "clear".** This is the
   rule documents already follow (`carriedDocumentation`,
   `mdl/executor/documentation_carry.go`): no `@documentation` keeps the stored
   text, `@documentation ''` clears it, and any other value replaces it. A text
   that equals the stored one in normal form keeps the stored bytes (same
   reasoning as #731: describe cannot spell `\r\n` or edge whitespace).
2. **A replace carries the documentation.** When `create or modify` splices a
   declared statement in place of a stored activity, the replacement's first
   activity takes the stored activity's documentation unless the declared
   statement states `@documentation`. This follows how `keepStoredNotes` already
   carries the stored activity's notes onto the replacement. When a run of
   several stored statements is replaced, each pairwise same-kind pair carries
   its own documentation (the `pairwiseSameKind` pairing). Any other stored
   activity's documentation goes out with it, as its notes do.
3. **`describe` prints it** as the first annotation of the statement, multi-line
   in one quoted string with `''` escaping (the CLAUDE.md expression rule), and
   only when it is not empty. Output for a flow without documentation is
   unchanged.
4. **Valid on** action activities, decisions (`if` and `split type`) and loops
   (`loop`, `while`). Elsewhere (`return`, `raise error`, `break`, `continue`)
   it is a parse-level diagnostic naming the statement. End events are covered
   only once the reader and writer gap above is fixed (see Open Questions).

Why an annotation and not a `/** … */` doc comment before the statement: in MDL
a doc comment documents a **document** (`create microflow`, `create entity`, …),
and every other piece of a flow statement's Studio Pro metadata is an `@`
annotation. Doc comments inside a flow body would give one concept two
spellings depending on depth. The name is `@documentation`, not `@doc`: it
matches the Studio Pro field and the `documentation` key the agent editor and
queues already use. It is deliberately not `@annotation`, which is the
sticky-note object, a different model element.

## Implementation Plan

Order: read and print first (so the round trip can be measured), then write,
then the carry.

1. **Grammar and visitor.** `annotation` is already generic in
   `MDLParser.g4`, so no grammar rule changes. In
   `mdl/visitor/visitor_microflow_statements.go`, handle `documentation` with
   `annotationValueString`, setting `Documentation` and `DocumentationSet` (the
   presence bit that separates absent from `''`, as `CaptionSet` does).
2. **AST.** `ast.ActivityAnnotations` gains `Documentation string` and
   `DocumentationSet bool`.
3. **Builder.** `cmd_microflows_builder*.go` copies it onto the built
   `ActionActivity`, `ExclusiveSplit`, `InheritanceSplit` and `LoopedActivity`.
4. **Describe.** `cmd_microflows_show.go` emits `@documentation '…'` from the
   object it already reads. `declaredMatches` then compares it like any other
   annotation, so an unchanged documented statement still matches and nothing
   is written.
5. **Carry on replace.** In `cmd_flow_modify.go`, at the point where
   `keepStoredNotes` prepares the replacement, set the stored documentation on
   the declared statement when it is not `DocumentationSet`. Apply the same in
   the loop-body in-place path (`replaceableInPlace`). `replaceableInPlace`
   must not treat a carried documentation as "states something the stored one
   does not have".
6. **Diagnostics.** Reject `@documentation` on statements that have no
   Documentation property, with the statement named.
7. **Docs.** `docs/01-project/MDL_QUICK_REFERENCE.md`, the microflow skill in
   `.claude/skills/mendix/`, and `mxcli syntax` for the annotation.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/ast/` (activity annotations) | `Documentation`, `DocumentationSet` |
| `mdl/visitor/visitor_microflow_statements.go` | parse `@documentation` |
| `mdl/executor/cmd_microflows_builder*.go` | set it on the built objects |
| `mdl/executor/cmd_microflows_show.go` | print it |
| `mdl/executor/cmd_flow_modify.go` | carry on replace; matching |
| `mdl/executor/documentation_carry.go` | reuse `carriedDocumentation` for the normal-form rule |
| `docs/01-project/MDL_QUICK_REFERENCE.md`, `.claude/skills/mendix/…` | syntax |
| `mdl-examples/doctype-tests/` | round-trip script (below) |

## Version Compatibility

The `Documentation` property on these objects predates every Mendix version
mxcli supports. It was present and round-tripped in the 9.24 test project used
for the measurement above, so no version gating is needed.

Language version: the annotation is new syntax, so it is accepted under both
`mdl 0` and `mdl 1`. The carry on replace changes what an existing script
writes: a replace that used to clear the documentation now keeps it. That is a
fix to silent data loss rather than a semantic change a script could rely on,
so it applies under both headers. Confirm with the ADR-0011 owner (Open
Questions).

## Test Plan

Each item has a control, per CLAUDE.md ("any test asserting nothing changed
must include a control"):

- **Visitor:** `@documentation 'x'`, `@documentation ''` and a multi-line value
  with `''` escaping parse to the expected AST, and absent leaves
  `DocumentationSet` false.
- **Executor, with the `modelsdk` backend:** write documentation onto an
  activity and a decision directly (the Studio Pro stand-in used in the
  measurement), then:
  - unchanged `create or modify`: kept, and **no write** (the idempotence
    control);
  - edit the documented activity in place: kept (fails on `main` today, which
    is the revert check);
  - replace it with a different activity at the same `@position` (#2328's
    case): kept;
  - `@documentation ''` on the edited statement: cleared (the control that the
    carry does not make documentation unclearable);
  - `describe` → `drop` + `create`: kept, because `describe` now prints it
    (fails on `main` today).
- **Round-trip script** in `mdl-examples/doctype-tests/`: a flow with
  documentation on an activity, a decision and a loop, run through `check`,
  `exec` and `describe`, with the output compared.
- **Integration (`-tags integration`):** `mx check` on the result, and opening
  a flow first edited in Studio Pro (Open Question 1) to confirm the stored
  bytes of an untouched documented activity are unchanged.

## Open Questions

1. **Empty-field shape.** Does Studio Pro write `"Documentation": ""` or omit
   the key on an object with no documentation? This needs a Studio Pro-authored
   flow (`mxcli bson dump`) before the writer is touched; the existing writer
   always sets the key.
2. **End events.** The reader ignores and the writer clears `EndEvent`
   documentation, so any rebuild of an end event drops it today. Should this
   proposal cover `@documentation` on `return`, or should that be a separate fix
   first? Recommendation: a separate fix (reader plus writer carry), since it is
   a silent drop in its own right; then this proposal extends to `return`.
3. **Carry when the kind changes.** The carry in #2328's case moves a
   `retrieve`'s text onto a `call microflow`. The text may describe the old
   activity, but keeping a possibly stale text is the same choice the upstream
   reporter asked for in `mendixlabs/mxcli#1870` (show-message translations), and
   it is visible and reversible, while losing it is neither. Confirm.
4. **ADR-0011.** Confirm that the carry needs no language-version gate.
5. **Upstreamability.** The issue is upstream's; the proposal is
   self-contained and fits the annotation pattern, so it should be acceptable
   upstream as-is. Decide whether to open an upstream PR after it lands here.
