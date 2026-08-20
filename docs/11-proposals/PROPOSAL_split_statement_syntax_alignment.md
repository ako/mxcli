---
title: Align the two microflow split statements on one branch syntax
status: proposed
date: 2026-08-20
related:
  - mendixlabs/mxcli#913
  - docs/12-bug-reports/2026-08-20-issue-913-split-statement-syntax.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - PROPOSAL_workflow_microflow_syntax_alignment.md
  - PROPOSAL_structured_microflow_description.md
---

# Proposal: Align the two microflow split statements on one branch syntax

## Problem Statement

MDL spells Mendix's two multi-way decisions in unrelated shapes, and one of the
two uses a keyword that means the opposite of what it says.

```mdl
case $Status                    split type $Animal
  when Open, Pending then         case Zoo.Dog
    …                               …
  when (empty) then               case Zoo.Cat
    …                               …
end case;                         else
                                    …
                                end split;
```

Three problems, established in the
[#913 investigation](../12-bug-reports/2026-08-20-issue-913-split-statement-syntax.md):

1. **`else` on a `split type` is not a default branch.** It is the `(empty)`
   flow — taken when the object is **null**. Proven on mxbuild 11.13.0: a split
   with `case Zoo.Dog` plus `else` still fails **CE0090** demanding flows for
   `Zoo.Cat` and the base `Zoo.Animal`; adding those cases (keeping the `else`)
   gives 0 errors. An author who reads `else` as `if`'s `else` writes a branch
   that never fires for a real object, and nothing warns them.
2. **`case` has two meanings.** Subject introducer in the enum split and in
   `caseExpression`; branch introducer in the type split. Two of three agree.
3. **Nothing transfers.** Knowing one split teaches you nothing about the other:
   different opener, different branch keyword, different catch-all, different
   terminator.

The reporter's own summary is accurate: *"use case for cases (not when…), and
not use case where switch/split is meant"*.

### Relationship to other proposals

[PROPOSAL_workflow_microflow_syntax_alignment.md](PROPOSAL_workflow_microflow_syntax_alignment.md)
makes the same argument one level up — MDL spells one concept differently
depending on the document type, against `design-mdl-syntax` principle 2
("never create a second syntax for the same concept") — and observes that
defects cluster in the divergent surface, because a construct with no second
consumer has nothing keeping it honest. The two splits are that pattern inside a
single document type: the inheritance split's branch handling is the surface with
no sibling, and it is where finding 1's silent `else` semantics live. The two
proposals are independent and complementary; neither blocks the other.

[PROPOSAL_structured_microflow_description.md](PROPOSAL_structured_microflow_description.md)
(#923) owns how DESCRIBE renders graphs that do not nest. This proposal owns what
the statements are *spelled* like. Open question 4 is the one place they meet.

## Why not `switch`

The issue proposes aligning on a Java/TypeScript `switch`. Rejecting that, for
three reasons:

1. **[ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md) is Accepted and
   explicit**: MDL is SQL-shaped, for citizen developers, in the PL/SQL lineage.
   `CASE x WHEN a THEN … END CASE` is PL/SQL's statement-CASE verbatim. The
   enum split is not an accident — it is the ADR being followed.
2. **MDL already has a second `case … when … then … else … end`** — the
   `caseExpression` at `MDLSettings.g4:312`, SQL's searched CASE. A `switch`
   statement sitting next to a `case WHEN` expression is *less* consistent than
   what exists today, not more.
3. **It would make three spellings, not one.** Existing scripts use `case` and
   `split type`; adding `switch` without removing them is the outcome the issue
   is complaining about, one iteration later.

The reporter's *consistency* argument survives all of this. It is the
inheritance split, not the enum split, that is out of line — so that is what
this proposal changes.

## Proposed MDL Syntax

Move the type split onto the enum split's branch syntax. `split type` and
`end split` stay (they are descriptive, already shipped, and keep the two
terminators distinguishable in error messages); only the branch keyword and the
catch-all change.

```mdl
split type $Animal
  when Zoo.Dog then
    return 'woof';
  when Zoo.Cat then
    return 'meow';
  when Zoo.Animal then
    return 'generic';
  when (empty) then
    return 'no animal';
end split;
```

Against the enum split, now the same statement with a different subject:

```mdl
case $Status
  when Open, Pending then
    return 'active';
  when Closed then
    return 'done';
  when (empty) then
    return 'unset';
end case;
```

What this buys:

| | before | after |
|---|---|---|
| branch keyword | `case` / `when` | `when` in both |
| catch-all | `else` / none | `when (empty) then` in both |
| meaning of the last branch | hidden | stated |
| `case` distinct meanings | 2 | 1 |
| indentation rule | 2 (both wrong) | 1 |

`when (empty) then` is not a cosmetic rename. It is the accurate name for the
flow, it matches what Studio Pro shows, and it removes the "any other type"
misreading that `else` invites — a misreading mxbuild catches with CE0090 only
because Mendix separately requires every subtype to be covered.

### Backwards compatibility

`case Module.Entity` and `else` keep parsing, indefinitely — scripts in the wild
use them, and this is a readability change, not a correctness one. They become
deprecated spellings:

- `mxcli check` emits a **warning** (new rule, e.g. `MDL0xx`) naming the
  replacement, with `else` → `when (empty) then` called out as a *meaning*
  clarification rather than a rename.
- `DESCRIBE` emits only the new form.
- No behavioural difference: both spellings build the identical flow.

## Implementation Plan

### Files to modify

| File | Change |
|---|---|
| `mdl/grammar/domains/MDLMicroflow.g4` | `inheritanceSplitCase`: accept `WHEN qualifiedName THEN microflowBody` alongside today's `CASE qualifiedName microflowBody`; accept `WHEN LPAREN EMPTY RPAREN THEN microflowBody` as the empty branch alongside `ELSE microflowBody`. Unambiguous — the rule is only reachable inside `inheritanceSplitStatement`. |
| `mdl/visitor/visitor_microflow*.go` | Map the new forms onto the existing `ast.InheritanceSplitStmt` (`Cases`, `ElseBody`). No AST change. |
| `mdl/ast/ast_microflow.go` | Record which spelling was parsed, for the deprecation warning only. |
| `mdl/executor/validate_microflow.go` | New warning rule for the deprecated spellings. Register in `ValidateProgram` so `check` and `exec` agree. |
| `mdl/executor/cmd_microflows_show_helpers.go` | `emitInheritanceSplitStatement`: emit `when … then`, emit the empty branch as `when (empty) then`, keep the existing suppression when its body is empty. Fix both emitters' indentation (see below). |
| `.claude/skills/mendix/write-microflows.md`, `docs/01-project/MDL_QUICK_REFERENCE.md`, `cmd/mxcli/syntax/features_*.go` | New spelling; state what the empty branch means. |
| `docs/11-proposals/PROPOSAL_microflow_inheritance_split_statement.md` | Correct the `else` description; status is stale at `draft`. |
| `docs/11-proposals/archive/PROPOSAL_microflow_enum_split_statement.md` | Its example shows an `else` on a `case`, which MDL008 rejects. |

The builder (`cmd_microflows_builder_actions.go`) does **not** change: both
spellings already funnel into `s.ElseBody` → `addBranch("", …)`, which is the
`(empty)` flow that CE0089 requires.

### Indentation (can land first, independently)

Both emitters are wrong, in opposite directions, and no test pins either:

- enum split — `when` at `indentStr+"  "`, body traversed at `indent+1`: the
  same column. Body should traverse at `indent+2`.
- inheritance split — branch at `indentStr`, flush with `split type` and
  `end split`. Branch should be at `indentStr+"  "` and its body at `indent+2`.

Target, matching `if/else` and the repo's hand-written fixtures:

```
  case $Status
    when Open then
      return 'open';
  end case;
```

This is a two-line change with no grammar or AST impact. It is worth landing on
its own, ahead of the syntax change, because it fixes the unreadable output in
finding 2 of the bug report without waiting on a language decision.

## Version Compatibility

None. This is an MDL surface change with no Mendix version dependency — both
spellings produce byte-identical documents. No `sdk/versions/*.yaml` entry, no
`checkFeature()` gate.

## Test Plan

| Test | Where | Asserts |
|---|---|---|
| `913-split-type-when-syntax.mdl` | `mdl-examples/bug-tests/` | New spelling parses and executes; `mx check` 0 errors |
| `913-split-type-legacy-syntax.mdl` | `mdl-examples/bug-tests/` | Old spelling still parses and executes |
| equivalence test | `mdl/backend/modelsdk/` | Both spellings produce the same flows and case values |
| `913-split-type-else-is-empty.mdl` | `mdl-examples/bug-tests/` | Pins finding 1: `case Dog` + `else` fails CE0090 for `Cat` and the base; the same split with every type covered passes. **The control is the point** — without the second run the CE0090s could be read as an unrelated coverage bug |
| deprecation warning | `mdl/executor/` | Old spelling warns, new spelling does not; warning is a warning, so `exec` still runs |
| describe roundtrip | `mdl/executor/` | DESCRIBE emits the new spelling; describe → exec → describe is stable |
| indentation | `mdl/executor/` | Branch bodies indent one level from their branch keyword, in both splits and matching `if`. No such test exists today, which is why the bug shipped |

Per the repo checklist, the indentation test must be shown to fail against the
current emitters before the fix lands.

## Open Questions

1. **Should `split type` become `case type`?** It would make the two statements
   textually identical apart from the subject (`case type $x … end case`).
   Rejected here because a shared `end case` makes a mismatched terminator
   ambiguous to diagnose, and `split type` is descriptive and already shipped —
   but it is the cleaner endpoint if churn is acceptable.
2. **Should the enum split accept `else` as an alias for `when (empty) then`?**
   No, as proposed — MDL008 rejects `else` on a `case` today and the error is
   good. Noted only because the archived enum-split proposal's example uses it.
3. **Does the deprecation ever end?** Suggest: never remove. The parse cost is
   one alternative and the warning does the teaching.
4. **Multi-value type branches** — `when Zoo.Dog, Zoo.Cat then` is expressible in
   the new grammar and is what the enum split allows. Not proposed, because it
   would render as the non-nesting graph described in
   [#923](https://github.com/mendixlabs/mxcli/issues/923). Worth revisiting once
   that is resolved.
