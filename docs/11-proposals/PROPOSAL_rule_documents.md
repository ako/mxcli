---
title: Rule documents — read, author, describe, catalog
status: done
date: 2026-08-21
related:
  - PROPOSAL_microflow_inheritance_split_statement.md
  - PROPOSAL_codegen_ownership.md
---

# Proposal: Rule documents — read, author, describe, catalog

**Status:** Done — slices 1–4 shipped 2026-08-21
**Date:** 2026-08-21

A `Microflows$Rule` is the one document type mxcli can *reference* but cannot
read, write, describe, move, or count. Two upstream issues have now landed on
the consequences of that gap rather than on the gap itself, and both were fixed
one symptom at a time.

## Problem Statement

[Mendix's reference](https://docs.mendix.com/refguide/rules/) calls a rule "a
special kind of microflow" that returns a Boolean or an enumeration, can only be
used from a decision, and cannot modify data, talk to the client, or do
integration. Structurally it is a microflow: the same object collection, the
same flows, the same return type.

mxcli treats it as a foreign object. Measured on a Mendix 11.13 project
carrying one rule (`Sample.Rule_IsActive`) and two microflows:

| Surface | Today |
|---|---|
| `LIST FOLDERS IN Sample` | ✅ `Rule Rule_IsActive` — works, for free, via `ListDocumentUnits` + `types.DocumentKind` |
| `if Sample.Rule_IsActive(...) then` | ✅ authorable (a decision's condition), after #939 |
| `show callers of Sample.Rule_IsActive` | ✅ after #939 |
| `SHOW RULES` | ❌ no such statement |
| `DESCRIBE RULE Sample.Rule_IsActive` | ❌ parse error |
| `CREATE` / `ALTER` / `DROP RULE` | ❌ no grammar; a rule can only be authored in Studio Pro |
| `MOVE RULE` / `FOLDER` clause | ❌ absent from `ast.MoveDocumentTypeByKeyword` (31 doctypes, no rule) |
| `CATALOG.OBJECTS` | ❌ a rule is not an object; `SELECT … WHERE ModuleName='Sample'` returns only the microflows |
| `search 'Rule_IsActive'` | ❌ no matches |
| lint / `GRAPH_DEAD_ASSETS` | ❌ a microflow called **only** from inside a rule's body is reported dead |

The last row is the one that silently misleads. A rule's body is never walked,
so every document it calls is invisible to the reference graph — the same shape
as the scheduled-event gap CLAUDE.md already records, one layer deeper.

### Why this keeps producing issues

Both rule issues so far were *absences* dressed as bugs:

- **#723 §A4** — `IsRule` was unimplemented on the modelsdk backend, so every
  `if Module.SomeRule(…)` became an expression split (CE0117). Fixed the read
  half; left the write half.
- **#939** — the write half. `splitConditionToGen` had no `RuleSplitCondition`
  case, so a decision was stored with no condition at all (CE0080), and the
  reporter's three secondary symptoms each had their own cause.

Neither is the last one, because the underlying state is unchanged: the model
has a document type mxcli cannot round-trip. Every feature that enumerates
documents has to remember to exclude rules, and each one that forgets is a new
issue.

## The shape of the fix: a rule is a third flow flavour

The repo already carries this pattern twice. `createNanoflowStatement` is a
**verbatim mirror** of `createMicroflowStatement` (`MDLMicroflow.g4:16` and
`:27`), sharing `microflowBody`, the flow builder, the describer and the
validator; the differences are a distinct `$Type`, a `flowBuilder.isNanoflow`
flag, and a disallowed-activity list. A rule is the same relationship with a
different restriction list.

That is what makes "full support" a tractable change rather than a second
microflow implementation: almost none of the work is new, and the parts that
are new are a validator and a `$Type`.

## BSON Structure

`Microflows$Rule` — properties in `initRule`'s declared order
(`modelsdk/gen/microflows/types.go`), cross-checked against
`generated/metamodel` (`MicroflowsRule`, an 11.6.0 snapshot):

| Key | Type | Note |
|---|---|---|
| `Name` | string | |
| `Documentation` | string | |
| `Excluded` | bool | |
| `ExportLevel` | enum | |
| `ObjectCollection` | `Microflows$MicroflowObjectCollection` | identical to a microflow's |
| `Flows` | list | identical to a microflow's |
| `MicroflowReturnType` | `DataTypes$*Type` | `DataTypes$BooleanType` or `DataTypes$EnumerationType` (with `Enumeration: "Mod.Enum"`) |
| `MarkAsUsed` | bool | |
| `ReturnVariableName` | string | Studio Pro wrote `"Variable"` on both reference rules (and `""` on a microflow in the same app) |
| `ApplyEntityAccess` | bool | |

A rule carries **none** of the microflow-only keys: no `AllowedModuleRoles`, no
`AllowConcurrentExecution` / `ConcurrenyErrorMessage` / `ConcurrencyErrorMicroflow`,
no `Url` / `UrlSearchParameters`, no `MicroflowActionInfo` / `WorkflowActionInfo`,
no `StableId`.

**Pinned against Studio Pro.** [`ako/TestApp`](https://github.com/ako/TestApp)
(Mendix 11.13.0) carries two rules authored in Studio Pro — `Rules.Rule1`
(Boolean return, String parameter, `ReturnValue: "length($pName)>0"`) and
`Rules.Rule2` (enumeration return `Rules.RuleResult`, entity parameter
`Pages.Bus`, `ReturnValue: "Rules.RuleResult.Approved"`). Both store exactly the
ten properties above and nothing else.

Two things the reference settles that inference had wrong or unproven:

- **`ReturnType` is not written.** gen declares it (a pre-7 legacy sibling of
  `MicroflowReturnType`, and absent from `generated/metamodel` 11.6.0); Studio
  Pro 11.13 writes only `MicroflowReturnType`. mxcli must not invent it. This
  is *not* the `Interval`/`IntervalType` carry-through case — there is nothing
  to carry.
- **The microflow-only keys really are absent.** A Studio Pro microflow in the
  same app (`Microflows.SplitMerge`) stores 19 properties including
  `AllowConcurrentExecution`, `AllowedModuleRoles`, `ConcurrencyErrorMicroflow`,
  `ConcurrenyErrorMessage`, `MicroflowActionInfo`, `StableId`, `Url`,
  `UrlSearchParameters` and `WorkflowActionInfo`. A rule stores none of them —
  measured on both rules, not assumed from the type definition.

### The parameter type — the question the reference rules answered

`generated/metamodel` lists three sibling types:

- `Microflows$MicroflowParameterObject` — the canvas object (`RelativeMiddlePoint`,
  `Size`, `VariableType`, `IsRequired`, `DefaultValue`)
- `Microflows$MicroflowParameter` — a `MicroflowParameterBase` (`Name`, `ParameterType`)
- `Microflows$RuleParameter` — the same `MicroflowParameterBase` shape

Measured against real documents, the metamodel's split is **not** what storage
does: every microflow in a blank 11.13 app — Studio Pro-authored marketplace
modules included (Administration, FeedbackModule, NanoflowCommons) — stores its
canvas parameter as `$Type: Microflows$MicroflowParameter` carrying the
*ParameterObject* shape. mxcli writes the same thing, so mxcli is right and the
metamodel naming is an SDK-side view.

**Resolved by the reference rules: a rule's canvas parameter is
`Microflows$MicroflowParameter`, carrying the ParameterObject shape**
(`DefaultValue`, `Documentation`, `HasVariableNameBeenChanged`, `IsRequired`,
`Name`, `RelativeMiddlePoint`, `Size`, `VariableType`) — byte-for-byte the same
shape a microflow uses. `Microflows$RuleParameter` appears in neither reference
rule, so it is an SDK-side name with no storage counterpart here, and the rule
writer reuses `microflowParameterToGen` unchanged.

### `RuleCall` — already fixed, recorded here for completeness

`Microflows$RuleCall` stores the rule reference under **`Microflow`**, not
`Rule` (rules share the microflow namespace). `modelsdk/gen` bound `Rule` on
both the encode and decode side; #939 applied a `STORAGE-NAME OVERRIDE` and
struck the row off `modelsdk/gen/keyaudit_test.go`. Any new code touching a rule
call must not reintroduce the SDK name.

## Proposed MDL Syntax

The whole surface mirrors microflows, because the document does.

```mdl
create or modify rule Sample.Rule_IsActive ($Customer: Sample.Customer) returns Boolean
folder 'Rules'
begin
  return $Customer/IsActive and $Customer/Balance > 0;
end
/
```

`returns` accepts Boolean or an enumeration and nothing else — the restriction
is Mendix's, and omitting it is CE-level invalid rather than a default.

```mdl
show rules;
show rules in Sample;

describe rule Sample.Rule_IsActive;   -- round-trippable, like describe microflow

drop rule Sample.Rule_IsActive;

move rule Sample.Rule_IsActive to folder 'Rules/Customer';
```

The decision form that calls one already exists and does not change:

```mdl
if Sample.Rule_IsActive(Customer = $Customer) then
  ...
end if;
```

No new verbs, no new property syntax, `Module.Element` throughout — the design
checklist in `.claude/skills/design-mdl-syntax.md` is satisfied by construction
because every form is the microflow form with one word changed.

## Implementation Plan

Four slices. Each is independently shippable and the first two are read-only.

### Slice 1 — read and describe (no new BSON writes)

| File | Change |
|---|---|
| `sdk/microflows/microflows.go` | `microflows.Rule` — its own struct, mirroring `Nanoflow` |
| `mdl/backend/microflow.go` | `ListRules` / `GetRule` on the interface |
| `mdl/backend/modelsdk/microflow.go` | implement via `mprread.ListUnitsWithContainer[*genMf.Rule]` — the decoder already registers `Microflows$Rule` |
| `mdl/backend/mpr/` | legacy implementation via `listUnitsByType("Microflows$Rule")`, which `IsRule` already calls |
| `mdl/backend/mock/` | `Func`-field stubs |
| grammar `MDLCatalog.g4` | `showOrList RULES (IN …)?` beside `NANOFLOWS`; `DESCRIBE RULE qualifiedName` |
| `mdl/ast/`, `mdl/visitor/` | nodes + listener |
| `mdl/executor/cmd_microflows_show.go` | `SHOW RULES`, and `DESCRIBE RULE` reusing the microflow describer |

### Slice 2 — catalog, references and lint

| File | Change |
|---|---|
| `mdl/catalog/builder.go` | `ListRules()` on `CatalogReader`; `cachedRules()` |
| `mdl/catalog/builder_objects.go` | a `RULE` row in `CATALOG.OBJECTS` |
| `mdl/catalog/builder_references.go` | call `emitActionRefs("RULE", …)` over each rule's object collection, so documents a rule calls stop reading as dead |
| `mdl/catalog/builder_strings.go` | index rule text for `search` |

**Both halves at once, or neither.** Adding the object type without walking rule
bodies would report every rule as dead; walking bodies without the object type
leaves `show callers` half-populated. #939 deliberately stopped at the edge for
this reason.

### Slice 3 — authoring

| File | Change |
|---|---|
| grammar `MDLMicroflow.g4` | `createRuleStatement` / `dropRuleStatement`, mirroring `createNanoflowStatement` verbatim |
| `mdl/ast/ast_microflow.go` | `CreateRuleStmt` (or a `FlowKind` on the existing node) |
| `mdl/executor/cmd_rules_create.go` | thin handler; `flowBuilder` gains `isRule` beside `isNanoflow` |
| `mdl/executor/validate_rule.go` | the restriction list (below) — the `ValidateNanoflowBody` precedent |
| `mdl/backend/*/` | `CreateRule` / `UpdateRule` / `DeleteRule`; modelsdk writes `Microflows$Rule` with the ten keys, reusing `microflowToGen`'s object/flow serialization |
| `mdl/ast/ast.go` | `"RULE"` in `MoveDocumentTypeByKeyword`; `FOLDER` clause on create |

The validator refuses what Mendix refuses, quoting the doc:

- return type is not Boolean or an enumeration
- create / change / delete / commit / rollback object
- show page, close page, show message, validation feedback, download file
- call web service, generate document, import/export XML

Each needs a measured CE number before it ships as an error rather than a
warning — the #931/#939 rule: measure against mxbuild, do not argue from docs.

### Slice 4 — surfacing

`mxcli syntax` topic, `docs-site/src`, `MDL_QUICK_REFERENCE.md`, the
`write-microflows` skill, LSP completion/hover, and `describe`-roundtrip
coverage in `mdl-examples/doctype-tests/`.

## Version Compatibility

Rules have existed since Mendix 5 and the document shape is unchanged across the
supported range (9/10/11). No `sdk/versions/*.yaml` gate is expected. The one
version-sensitive point is `ReturnType` vs `MicroflowReturnType` (see Open
Questions), which is a pre-7 legacy sibling of the kind `Interval` /
`IntervalType` already is for scheduled events — carry it through untouched on
modify, derive it on create, and pin it against a real document per version.

## Test Plan

- `mdl-examples/doctype-tests/` — a rule with a Boolean return, one with an
  enumeration return, a decision calling each, and a `describe` → re-exec
  round trip **into a module where the rule does not exist** (replaying over
  the original reports "Unchanged" whether or not the clause survived).
- `mdl-examples/bug-tests/939-rule-split-condition.mdl` becomes fully
  self-contained: it currently needs a Studio Pro-authored rule because MDL
  cannot create one.
- Backend round-trip tests per engine, asserting the ten stored keys and the
  **absence** of the microflow-only ones.
- A catalog test that a microflow called only from a rule's body is not in
  `GRAPH_DEAD_ASSETS` — with the pre-change control.
- Negative tests (`*.fail.mdl`) for each restriction, each carrying the CE
  number it prevents.

## Design decisions

Settled 2026-08-21. **A rule is handled the way a nanoflow is** — its own
statements, its own listing, its own semantic type — not as a variant of a
microflow.

1. **A rule gets its own semantic type**, `microflows.Rule`, mirroring
   `microflows.Nanoflow` (which is already a distinct struct, not an alias of
   `Microflow`). Its own `CreateRuleStmt`, its own
   `ListRules`/`GetRule`/`CreateRule`/`UpdateRule`/`DeleteRule` on the backend
   interface. The document shape supports this: a rule is a microflow minus nine
   properties, and the nine are exactly the ones a rule has no concept of.
2. **`SHOW`/`LIST MICROFLOWS` lists microflows only** — not nanoflows, not
   workflows, and not rules. `SHOW RULES` / `LIST RULES` is the separate
   listing, via the existing `showOrList` rule so both spellings work, as they
   do for nanoflows.
3. **There is no `GRANT EXECUTE ON RULE`.** Both reference rules store no
   `AllowedModuleRoles` — a rule is not independently callable, so it has no
   module-role security to grant. This is a real divergence from the nanoflow
   mirror and the one place the parallel stops.

Two consequences worth stating, because they are what "like a nanoflow" buys:

- A rule's surface is **much smaller than a nanoflow's**. A nanoflow is
  reachable from pages, navigation and widget actions; a rule is reachable only
  from a decision. None of `mdl/backend/widgetobj`, `sdk/pages`,
  `modelsdk/gen/navigation` or the page grammar needs to learn about rules.
- `microflowBody`, the flow builder, the describer and the validator are shared,
  exactly as the nanoflow shares them.

### Still open

- **`ReturnVariableName`.** Both reference rules carry `"Variable"`; the
  microflow in the same app carries `""`. Whether that is a Studio Pro default
  for the rule editor or authored text is not established by two samples. It
  must at minimum be **preserved** on modify; whether MDL grows a surface for it
  is a separate call.

## Measured: a foldered rule is stored like any other document

TestApp now places both rules in a `MyRulesRule` folder, and the containment is
exactly a microflow's: the rule's unit row is `ContainmentName: "Documents"`
with `ContainerID` pointing at the folder unit, which is itself `Folders` under
the module. Nothing about a rule's placement is special.

Two consequences, both shrinking the plan:

- `LIST FOLDERS IN Rules` already renders the foldered rules correctly, with no
  change — #932's `ListDocumentUnits` walk is containment-generic.
- The MOVE/`FOLDER` work in Slice 3 is **one entry in
  `ast.MoveDocumentTypeByKeyword`**, not a placement implementation.

## Measured: the rule call #939 writes matches Studio Pro's

`Rules.MicroflowUsingRule` now carries a decision calling `Rules.Rule1`, so the
shape is no longer validated only against the legacy engine. Studio Pro stores:

```
SplitCondition  Microflows$RuleSplitCondition
  RuleCall      Microflows$RuleCall
    Microflow            "Rules.Rule1"          ← the storage key, not "Rule"
    ParameterMappings    [2, {Microflows$RuleCallParameterMapping,
                              Parameter: "Rules.Rule1.pName", Argument: "'abc'"}]
```

Every element of that is what #939 now writes: the same `$Type`s, the same
`Microflow` storage key (independent confirmation of the `initRuleCall`
override), the same typed-array marker **2**, and the same fully-qualified
`Parameter` — which is how the flow builder already constructs it
(`call.Name + "." + name`).

Two round-trip results on that document:

- `DESCRIBE MICROFLOW` renders `if Rules.Rule1(pName = 'abc') then`, and
  `show callers of Rules.Rule1` lists the microflow — the read and reference
  paths exercised against a Studio Pro document for the first time.
- Re-executing that describe output over the app and re-checking gives **0
  errors, against a 0-error baseline** (mxbuild 11.13.0, baseline run first).
  Diffing the two documents, the `SplitCondition` block does not appear in the
  diff at all — the rebuild reproduces it exactly.

What *does* differ in that diff is the surrounding flow graph, and all of it is
pre-existing and general to microflows: the connection-index widths and
`CaseValues` shape below, plus two more of the same kind — Studio Pro keeps a
flow's bezier control vectors (`"15;0"`, `"0;15"`) where a `@curve` annotation
did not survive describe, and the object collection's ordering differs. None of
these are rule-specific and none change the check result.

## Adjacent findings, out of scope

Measured while pinning the rule shape, both on Studio Pro documents in TestApp
and both **general to microflows**, not rule-specific:

- **`CaseValues` on a sequence flow.** Studio Pro writes the bare marker `[2]`
  where mxcli writes an explicit `[2, {Microflows$NoCase}]`, and on a boolean
  split it labels only the *false* branch, leaving the true branch's list empty
  where mxcli labels both. Both load; mxcli's form is what the legacy writer has
  always produced.
- **`OriginConnectionIndex` / `DestinationConnectionIndex` width.** Studio Pro
  stores int64; mxcli writes int32. Same class as the widths recorded as out of
  scope in #931 (`CanvasHeight`, `TabIndex`, `PopupWidth`), now observed on
  `Microflows$SequenceFlow` as well.

Neither has a known symptom. They are recorded here so the next person to diff a
rule against Studio Pro does not mistake them for something this feature
introduced.
