---
title: check heuristics for constructs MxBuild rejects
status: in-progress
date: 2026-07-02
---

# Proposal: `check` heuristics for constructs MxBuild rejects

**Status:** In progress — Wave 1 shipped; ongoing as a standing programme (see
[§ Implementation status](#implementation-status))
**Date:** 2026-07-02 (updated 2026-07-29)
**Relates to:** `PROPOSAL_expression_type_checking.md` (shares the `ModelResolver`
resolution core — see [§ Relation to expression type checking](#relation-to-expression-type-checking))

## Implementation status

This proposal has become a **standing programme**: every time a real project
reports a construct that passes `mxcli check` but `mx check`/MxBuild rejects, a
static heuristic is added to close the gap (verified against the bundled `mx`
where feasible). The originally-cataloged six cases:

| Case | Rule | Status |
|------|------|--------|
| 2 — user task without a page (CE1834) | `MDL-WF01` | ✅ shipped (`validate_workflow.go`) |
| 4 — single outcome with nested activities (CE1876) | `MDL-WF02` | ✅ shipped |
| 1 — free-text decision outcome | `MDL-WF03` | ✅ shipped |
| 5 — control-bar button `$currentObject` (CE1571) | `MDL-BUTTON01` | ✅ shipped (`validate_page_button_context.go`) |
| 3 — user-task page context ≠ `System.WorkflowUserTask` (CE7412) | — | ⬜ **deferred** — needs `--references` + page-context resolution, and is FP-prone (WorkflowUserTask specializations, snippet/microflow-sourced dataviews). No current test app exercises workflows, so the CE7412 trigger and the FP boundary can't be verified against `mx` yet. Pick up when a workflow app enters the test rotation. |
| 6 — dynamictext contentparams/content on enum/date (CE0117) | — | 🟡 **partial** — the non-String→`toString()` write path landed (RSS-reader follow-up); the standalone `--references` warn + a focused fail-open repro remain |

**Ledger-driven catalog expansion (2026-07).** The `ako/mxcli-ledger` project has
since surfaced ~10 more cases of exactly this class, each implemented on the same
pattern (rule + `.fail.mdl` repro + `fix-issue.md` CE→rule row):

| Rule | Construct → MxBuild error | Verdict |
|------|---------------------------|---------|
| `MDL045` | `/` used as division → CE0117 (`/` is member access; use `div`) | syntax-only |
| `MDL046` | `dateTime()`/`dateTimeUTC()` with a non-literal arg → CE0117 | syntax-only |
| `MDL047` | association `= empty` in a retrieve **or** widget datasource → CE0161 | syntax-only |
| `MDL048` | `[id = <value>]` retrieve (vs the valid `[id != $obj]`) → CE0161 | syntax-only (operand-typed) |
| `MDL049` | association-object path passed as a call argument → CE0117 | syntax-only |
| `MDL-WIDGET13` | association traversal in a widget expression prop → CE0117 | syntax-only |
| `MDL-WIDGET14` | a client expression in `contentparams`/`captionparams` → CE1613 | syntax-only |
| `MDL-WIDGET15` | adjacent inline (Text/Paragraph) dynamictexts fuse | info (layout) |
| `MDL031` (pass-through) | view pass-through string column length ≠ source → CE6770 | `--references` |
| (assoc validate) | `create association` to/from a view entity → CE6771 | `--references` |
| `MDL-WF06` | enumeration decision / call-microflow outcomes with no empty-valued branch → CE6686 | syntax-only |

Two more ledger cases in this class were closed by **fixing the write path** rather
than adding a check — the MDL is structurally valid, so `check` can't see it; the
model was being *written* wrong: navigation-list item names (CE7247/CE0495) and an
orphaned index left by `create or modify` dropping an indexed attribute (which
*crashed* `mx check`). These belong to the same "check ↔ build parity" mission but
are writer fixes, not heuristics.

**`MDL-WF06` (2026-09).** Mendix generates a decision's outcomes as *one per
enumeration value plus one for the empty value*, and MxBuild compares the stored
set against that generated set — so an enumeration decision written without
`'' -> { }` is CE6686 ("Regenerate the outcomes"), which `check` and `exec` both
accepted. Measured on mxbuild 11.10.0 in a blank app: the two-value decision is 1
error and adding the empty outcome takes it to 0; a **required (`not null`)**
attribute does *not* exempt it (still CE6686), which is what settles the severity
as error rather than warning; and a `call microflow` activity branching on an
enumeration return fails and clears identically, so the rule runs at both call
sites. mxbuild wants set *equality*, so a missing enumeration **value** is CE6686
too — that half needs the enumeration's definition and belongs to the reference
pass, not to a syntax-only rule. It is also the first workflow rule to run on
`ALTER WORKFLOW`: an inserted or replaced activity reaches the same build error
(measured), and nothing had ever validated one. The other workflow rules stay
CREATE-only on purpose — MDL-WF01/WF02 describe a state a later op in the same
script can repair, and MDL-WF05 needs activities the ALTER statement cannot see.

**Standing policy:** when a new missing check is reported, implement it here (or as
a write-path fix when the construct is valid MDL). The two remaining originally-
cataloged gaps are **case 3 (CE7412)** and the standalone **case 6 warn**.

## Problem Statement

`mxcli check … --references` reports "Check passed" for several constructs that
the real MxBuild (`docker build` / `mx check`) then rejects. Each miss costs a
full build round-trip to discover. This proposal adds targeted static heuristics
so `check` catches them up front.

Structurally, the gap has one dominant cause: **`CreateWorkflowStmt` has no case
in `validateWithContext`** (`mdl/executor/validate.go:271`) and there is no
`ValidateWorkflow` at all — workflows receive *zero* semantic validation. That
explains four of the six reported cases. The other two are missing widget-context
checks.

## Reported cases and verdicts

Investigation confirmed the AST shapes and validator plumbing needed for each.
Verdict legend: **Syntax-only** = detectable with no project; **With-project** =
needs `--references`.

| # | Construct | MxBuild error | Verdict | FP risk | Effort |
|---|-----------|---------------|---------|---------|--------|
| 2 | Workflow user task without a page | CE1834 "The 'Page' property is required" | Syntax-only | none | trivial |
| 4 | Single-outcome user task containing nested activities | CE1876 "Single outcome must not contain any activities in its flow" | Syntax-only | none | trivial |
| 5 | DataGrid **control-bar** button passing `$currentObject` | CE1571 "No argument has been selected for parameter …" | Syntax-only | low | low |
| 1 | Workflow decision with free-text outcomes | outcome not a valid `EnumerationValueIdentifier` | Syntax-only (partial) | med | low |
| 6 | `dynamictext` `content`/`contentparams` on enum/date attr | CE0117 "Error(s) in expression" | With-project (+ builder bug) | low | med |
| 3 | User-task page typed to context entity, not `System.WorkflowUserTask` | CE7412 | With-project | med | med-high |

### Case details

**Case 2 — user task without a page (CE1834).** `WorkflowUserTaskNode.Page` is a
`QualifiedName` (`mdl/ast/ast_workflow.go:42`) left zero-valued when the `PAGE`
keyword is absent (`mdl/visitor/visitor_workflow.go:476`). Flag
`Page.Module=="" && Page.Name==""`. Unambiguous.

**Case 4 — single outcome with nested activities (CE1876).**
`WorkflowUserTaskOutcomeNode` carries `Activities []WorkflowActivityNode`
(`ast_workflow.go:66`). Flag `len(Outcomes)==1 && len(Outcomes[0].Activities)>0`.
Exactly the rejected shape.

**Case 5 — control-bar button with `$currentObject` (CE1571).** A control-bar
button is a `WidgetV3` under a `controlbar`-typed parent (`ast_page_v3.go`;
grammar `MDLPage.g4` `CONTROLBAR`), distinct from a `column`-typed parent. A
control bar is **not** row-scoped, so `$currentObject` has no binding there.
Detect by walking the button's parent chain to the enclosing container type and
flagging `$currentObject` action args. *Caveat:* verify the row-scoped vs
non-row-scoped widget set against real widgets — flag only confirmed
non-row-scoped containers (`controlbar`, and likely `filter`) to avoid false
positives; do **not** assume `header`/`footer` without checking.

**Case 1 — decision free-text outcomes.** `WorkflowDecisionNode.Outcomes[].Value`
is a free string (`ast_workflow.go:100`). A Mendix Decision branches on an enum,
so outcome labels must be valid `EnumerationValueIdentifier`s. Cheap heuristic:
flag any `Value` that isn't `True`/`False`/`Default` and isn't a valid Mendix
identifier (e.g. contains a space, like `'Confirmed closed'`). **Language-gap
note:** the deeper problem is that MDL decision syntax cannot bind the enum /
deciding-microflow the Decision branches on, so such a decision is arguably
unbuildable as authored. That is a syntax-design issue (tracked separately), not
something `check` resolves; ship the identifier heuristic and file the gap.

**Case 6 — dynamictext contentparams on enum/date (CE0117).** Two problems:
1. **A `check` heuristic** (`--references`, warn): a `contentparams` entry
   referencing a non-String attribute → suggest the `Attribute:` shorthand.
2. **A likely builder bug.** `isNonStringAttribute` (`mdl/executor/cmd_pages_builder_v3.go:1432`)
   **fails open** — when the attribute type can't be resolved it returns `false`
   (assume String) and skips the `toString()` wrapping enum/date require, emitting
   a bad expression → CE0117. The cleaner fix is to resolve the type reliably
   rather than guess (see cross-reference below). **Unconfirmed:** *why* the
   `Attribute:` shorthand succeeds where `contentparams` fails — both go through
   the same wrapping path, so the divergence is in whether type resolution
   succeeds per authoring form. Needs a focused repro before committing the
   builder fix.

**Case 3 — user-task page context entity (CE7412).** Requires resolving the
referenced page and reading its data-view context entity, then asserting it is
`System.WorkflowUserTask`. Lives in the `validateWithContext` workflow case;
reuses page-context resolution machinery adjacent to `validate_page_context.go`.

## Implementation Plan

Existing static validators return `[]linter.Violation` and are dispatched from
`cmd/mxcli/cmd_check.go` (unconditional) and `validateWithContext`
(`--references`). New checks follow that pattern.

### Files to create / modify

| File | Change |
|------|--------|
| `mdl/executor/validate_workflow.go` | **New.** `ValidateWorkflow(*ast.CreateWorkflowStmt) []linter.Violation` — cases 2, 4, and the case-1 identifier heuristic. Recursively walk activities. |
| `cmd/mxcli/cmd_check.go` | Call `ValidateWorkflow` alongside `ValidateMicroflow`. |
| `mdl/executor/validate.go` | Add `*ast.CreateWorkflowStmt` case to `validateWithContext` for case 3 (project-backed). |
| `mdl/executor/validate_page_button_context.go` | **New.** `MDL-BUTTON01` — case 5 control-bar `$currentObject` heuristic. |
| `mdl/executor/validate_widgets.go` | `MDL-WIDGET07` — case-6 contentparams-on-non-String warning (project-backed). |
| `mdl/executor/cmd_pages_builder_v3.go` | Case-6 builder fix (resolve attribute kind rather than fail open). |
| `mdl-examples/bug-tests/*.fail.mdl` | One negative repro per case (`make check-mdl` inverts exit for `.fail.mdl`). |
| `.claude/skills/fix-issue.md`, `.claude/skills/mendix/cheatsheet-errors.md` | New CE→rule/symptom mappings (CE1834, CE1876, CE1571, CE7412; CE0117 already partly documented). |

### Suggested waves

- **Wave 1 (zero/low risk, no project):** cases 2, 4, 5, and the case-1
  identifier heuristic. One focused PR. Depends on nothing else.
- **Wave 2 (`--references`):** case 3 and the case-6 warning, plus the case-6
  builder repro + fix. Benefits from the resolution core below.

## Relation to expression type checking

`PROPOSAL_expression_type_checking.md` is scoped to microflow/nanoflow
**expression** typing, so it does not cover these workflow-structural and
page/widget cases as rules. But its **build-order step 1 — the headless
`ModelResolver` core** (`AttributeKind`, `EnumCases`, `MicroflowReturn` +
memoized index + script overlay) — is the resolution infrastructure two of these
cases want:

- **Case 6** is a genuine type question ("is this attribute String or
  enum/date?"). `ModelResolver.AttributeKind` is a reliable answer, turning the
  fail-open `isNonStringAttribute` bug into a root-cause fix instead of a warning
  band-aid. The builder and the `MDL-WIDGET07` check would consume the same
  resolver.
- **Case 1's stronger form** — "outcomes ∈ the enum cases of the decision's
  deciding-microflow return" — needs exactly `MicroflowReturn` + `EnumCases`. It
  additionally requires extending typing scope to workflows and the workflow AST
  exposing the decision return type (today `WorkflowDecisionNode.Expression` is a
  raw string; the enum binding isn't representable — the language gap noted above).

Cases 2, 4, 5 need none of the type system. The two efforts are complementary:
Wave 1 ships independently; the type-checking resolver core is the clean home for
case 6's fix and case 1's stronger check.

## Open Questions

1. Exact non-row-scoped widget set for case 5 (verify `filter`/`header`/`footer`
   against real widgets before flagging).
2. Case 6: confirm the `Attribute:`-vs-`contentparams` divergence with a repro;
   decide whether Wave 2 ships the warning first and the builder fix after the
   resolver core lands, or both together.
3. Severity: workflow structural violations (2, 4) as `Error`; the case-1
   identifier and case-6 type mismatches as `Warning` initially?
4. Should these live in one proposal/PR series or split workflow vs page/widget?
