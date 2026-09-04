---
title: Widgets as first-class MDL, not a second dialect
status: draft
date: 2026-09-04
related:
  - PROPOSAL_mcp_pluggable_widget_authoring.md
  - PROPOSAL_multi_version_pluggable_widgets.md
  - PROPOSAL_widget_property_visibility.md
---

# Proposal: Widgets as first-class MDL, not a second dialect

**Status:** Draft
**Date:** 2026-09-04

Using a widget should feel like calling a Java action. It does not. A widget is
named differently, described differently, and is invisible to the reference
graph — and where every other extension point resolves against the project, a
widget resolves against a hardcoded list in the grammar. This proposal closes
the four gaps, in order of how much each one costs.

## Problem Statement

### What a user hit

[mendixlabs/mxcli#1036](https://github.com/mendixlabs/mxcli/issues/1036). A team
needed a sandboxed iframe: the HTML Element widget's `attributes` object list
would carry `sandbox` and `srcdoc`. MDL could not express it, so they fell back
to `tagContentHTML`, **which executes same-origin — precisely the risk the
sandbox exists to prevent.**

The generated documentation is what cost them the time. `widget init` writes
`.claude/skills/widgets/htmlelement.md`, whose lead example is:

```sql
PLUGGABLEWIDGET 'com.mendix.widget.web.htmlelement.HTMLElement' widget1 {
  tagcontentcontainer { ... }
  attribute item1   -- one entry of `attributes`
  event item1       -- one entry of `events`
}
```

Fed back into `mxcli check` verbatim, it fails on the first line of its own body:

```
line 4:4 mismatched input 'tagcontentcontainer' expecting '}'
```

The file is written *for LLM agents to follow* and carries the widget's real
property table, so it reads as authoritative. Multiple sessions concluded the
feature existed.

### The real shape of the problem

The reported bug is one symptom of a widget being a second dialect inside MDL.
Every other extension point — a submicroflow, a Java action, a JavaScript
action — is referenced by qualified name, described in-language, and findable in
the reference graph:

```mdl
$R = CALL MICROFLOW          Module.Name (Param = value)
$R = CALL NANOFLOW           Module.Name (Param = value)
$R = CALL JAVA ACTION        Module.Name (Param = value)
$R = CALL JAVASCRIPT ACTION  Module.Name (Param = value)
```

A widget matches none of that:

| | reference | `DESCRIBE` | "who uses it?" |
|---|---|---|---|
| microflow / nanoflow | `Module.Name` | MDL statement | `call` edge |
| java action | `Module.Name` | MDL statement, round-trips | `call` edge |
| javascript action | `Module.Name` | MDL statement | `call` edge |
| **widget** | keyword *if blessed*, else a string ID | **CLI command only** | **no edge at all** |

Four gaps follow, and each is measured below.

### Gap 1 — Two reference forms, gated on a hardcoded list

```
htmlelement.def.json:  mdlName: HTMLELEMENT   widgetId: com.mendix.widget.web.htmlelement.HTMLElement

htmlelement h (tagName: 'div')     -- REJECTED
combobox c (Attribute: Name)       -- PARSES
```

A widget gets a keyword only if it appears in the grammar's `widgetTypeV3` list.
Everything else must be written by string ID.

**The resolution machinery already exists and is already wired.**
`cmd_pages_builder_v3.go:425` tries the MDL name *first*:

```go
// Try by MDL name first
if def, ok := pb.widgetRegistry.Get(strings.ToUpper(w.Type)); ok {
    return pb.buildPluggable(def, w)
}
```

Every `.def.json` carries an `mdlName`; `WidgetRegistry.Get(mdlName)` exists.
The only thing missing is a parser that will produce `w.Type == "htmlelement"`.
Today `MDLName` is read **only to build error messages**.

### Gap 2 — Object lists and child slots, the same defect one level down

Inside a widget body, containers are gated on nine hardcoded keywords
(`MDLPage.g4:402–410`), while the doc generator derives one for **every** object
list and child slot mechanically (`deriveObjectListKeyword` singularises any
property key; child slots use `strings.ToUpper(child.Key)`). Two lists, nothing
comparing them.

Measured against the fixture project (33 widget defs, 46 documented constructs):

| | parses | rejected |
|---|---|---|
| Object lists | 13/16 | `attribute`, `event`, `attr` |
| Child slots (named, under `pluggablewidget`) | 3/30 | 27 |
| **Total** | **16/46** | **30 (65%)** |

20 of 33 widgets document at least one keyword that cannot parse. **Control** —
identical widget, identical body shape, only list membership differs:

```mdl
group g1 (headerText: 'x')              -- PARSES
attribute a1 (attributeName: 'title')   -- REJECTED
```

### Gap 3 — No `DESCRIBE WIDGET`, which is why the generated doc exists at all

`DESCRIBE JAVA ACTION FeedbackModule.ValidateEmail` emits re-executable MDL.
There is no MDL equivalent for a widget — only `mxcli widget describe`, a CLI
command.

This reframes the reported bug. Actions need no generated documentation because
`DESCRIBE` answers in-language, against the live project. **The widget `.md` is a
workaround for a missing statement**, and the reason it could drift is that
nothing else could answer the question. Fixing the generator alone treats the
symptom.

### Gap 4 — Widgets are absent from the reference graph

`CATALOG.REFS` carries 15 edge kinds and none is widget use:

```
action associate call change create datasource delete generalize
home_page layout menu_item parameter retrieve return show_page
```

So `show references to <a java action>` returns
`MICROFLOW FeedbackModule.VAL_Feedback | call`, and the same question about a
widget returns nothing. Impact analysis for a widget upgrade is unanswerable.
This is the same class as the scheduled-event gap recorded in CLAUDE.md, where a
microflow run only by a scheduled event read as dead until a `schedule` edge was
added.

### Why honest documentation is not sufficient

Option 2 of the issue (emit only what the grammar accepts) converts a silent
failure into a documented dead end. Worth doing, and it is slice 1 below — but
it would not have unblocked the reporter. There is no other route: `ALTER PAGE`
rejects the same construct, verified with a working control.

```mdl
alter page M.Sandbox {
  insert into frame { dynamictext t1 (Content: 'hello') }          -- PARSES (control)
};
alter page M.Sandbox {
  insert into frame { attribute a1 (attributeName: 'sandbox') }    -- REJECTED
};
```

They would have had accurate documentation of a capability gap, and shipped the
same-origin fallback anyway.

## BSON Structure

**No new BSON and no new write path.** This is what makes the proposal small,
and it should be re-verified before code is written, because everything rests on
it.

`PluggableWidgetEngine.applyObjectLists` (`widget_engine.go:1068`) is already
fully def-driven:

```go
byContainer[strings.ToUpper(lists[i].MDLContainer)] = &lists[i]
...
mapping, ok := byContainer[strings.ToUpper(child.Type)]
```

It matches the AST child's `Type` **string** against whatever the def declares,
and knows nothing about the nine keywords. The visitor sets that string from the
token's literal text (`visitor_page_v3.go:554`):

```go
widget.Type = strings.ToLower(typeCtx.GetText())
```

So the whole pipeline below the parser is text-driven and generic. The keyword
lists exist **only so ANTLR has a token to match** — an artefact of the parser
generator leaking out as a capability boundary.

`PLUGGABLEWIDGET` and `CUSTOMWIDGET` are additionally **already the same thing**:
both take the `buildPluggable` branch (`cmd_pages_builder_v3.go:430`) and both
store `CustomWidgets$CustomWidget`.

## Proposed MDL Syntax

### A widget is named like everything else

```mdl
create page Sales.Frame ( Title: 'Frame', Layout: Atlas_Core.Atlas_Default )
{
  htmlelement frame ( tagName: 'div' ) {
    attribute sandboxAttr (
      attributeName: 'sandbox',
      attributeValueType: 'template',
      attributeValueTemplate: 'allow-scripts'
    )
    event onClickEvent ( eventName: 'onClick' )
    tagcontentcontainer content {
      dynamictext note ( Content: 'Sandboxed' )
    }
  }
}
```

Nothing here is new syntax — it is the shape `combobox`, `datagrid` and `group`
already use, applied to every widget instead of a blessed subset. `DESCRIBE PAGE`
emits this form, replacing today's `pluggablewidget '<id>' frame ( … )`.

### Describing a widget is a statement

```mdl
describe widget htmlelement;              -- property table, enums, containers
list widgets;                             -- every widget with a definition
show references to widget htmlelement;    -- which pages use it
```

`DESCRIBE WIDGET` is the statement that retires the drift risk: once the answer
is available in-language and against the live project, the generated `.md` stops
being the only source and can be regenerated from — or replaced by — it.

### The ID form remains, as an escape hatch

```mdl
widget 'com.acme.widget.Unlisted' w1 ( someProp: 'x' )
```

For a widget whose definition is not loaded, or to be explicit. After slice 2
this is rarely written and never emitted by `DESCRIBE`.

### Design notes against `design-mdl-syntax.md`

- **Reuse existing keywords first.** This proposal goes further: it stops
  *adding* them. Today every new widget with an object list needs a new reserved
  word — an unbounded cost paid in name collisions (#619's quoting escape hatch
  exists for exactly this).
- **One way to do each thing.** Two spellings collapse to one: `combobox c (…)`
  and `pluggablewidget 'com.mendix.widget.web.combobox.Combobox' c (…)` are the
  same widget today, and `CUSTOMWIDGET` is a third spelling of the same
  behaviour.
- **No implicit context**: a container name resolves against the parent widget's
  own definition, which is as explicit as a qualified name.
- **One example is enough for an LLM** — and the example the generator already
  emits becomes the correct one.

### The one required doc change

The generated example omits the **name**, which even a working slot requires:

```
tagcontentcontainer { ... }        -- as generated today, rejected
tagcontentcontainer content { }    -- correct
```

So the three child slots that *could* parse today are documented in a form that
cannot either.

## Implementation Plan

Six slices. Each ships alone; 1 is independent of the rest.

### Slice 1 — Stop the bleeding

Correct whether or not anything else lands. **Items 2–4 are implemented.**
Item 1 was dropped: the premise behind it turned out to be false (below).

1. ~~**Ship the four missing built-in widgets.**~~ **Dropped — the premise was
   wrong.** The draft assumed `events`, `fileuploader`, `googletag` and
   `markdown` are bundled with Studio Pro and therefore unreachable to mxcli.
   Measured instead:

   | | |
   |---|---|
   | A blank Mendix 11.13 project (`mx create-project`) | 33 widgets, **none of the four** |
   | Installing File Uploader (Marketplace module 235351) | `widgets/` goes 33 → 34, `.mpk` present |
   | `exec` of a page using it, straight after | **builds** — no `widget init` needed |

   So a widget whose package is absent is one **Studio Pro cannot use either**;
   it is not a gap mxcli can paper over, and there is nothing to ship. The
   moment the widget is usable at all, the `.mpk` is in the project and mxcli
   picks it up on its own, because `initPluggableEngine` refreshes definitions
   from installed packages before reading them.

   Two wrong turns are worth recording, since both looked settled at the time.
   Their `.mpk`s are **not** inside `Mendix.Modeler.Core.dll` — that came from a
   bare ID-string match, and the assembly has 690 embedded zips with *zero*
   `widgets.mendix.com` hits. And embedding a `.def.json` would not have worked
   anyway: `getOrGenerateTemplate` (`modelsdk/widgets/loader.go:215`) derives the
   template from the `.mpk` **in `widgets/`**, so a def alone only moves the
   error to `template not found: fileuploader` — verified before the premise
   itself was checked, which is the lesson. The remaining item below is the
   whole fix.

2. **Fix that error message.** It currently says
   `(run 'mxcli widget init -p app.mpr')` — a remedy that **provably cannot
   work**, since `widget init` scans `widgets/` and these are not there.
3. **Generated examples include the name** on child slots.
4. **`()` is accepted.** `widgetPropertiesV3` requires at least one property, so
   `container c ()`, `text t ()` and `pluggablewidget … pw ()` are all parse
   errors while bare `pw` and `pw (x: 'y')` are fine. One-line grammar fix, found
   in the same investigation.

### Slice 2 — The widget keyword is def-driven

Smallest of the capability slices, because the builder already resolves by MDL
name and the visitor already sets `Type` from token text. Grammar and visitor
only.

5. Accept an identifier as a widget type in a page body, resolved against the
   registry. `htmlelement frame ( … )` works for every widget with a definition.
6. `DESCRIBE PAGE` emits the keyword form.

### Slice 0 — The validator knows what a widget is (blocks slices 2–3)

Report at `check` time what only `exec` catches today: an unknown widget kind or
id, and a container the parent's definition does not declare — each naming the
near misses. The detection already exists in `validateWidgetTreeIn`; only the
reporting is missing. Worth shipping on its own, and the thing that makes
slices 2–3 safe (Open Question 1).

### Slice 3 — The widget body is def-driven

7. **Grammar.** A parallel body rule used *only* by the widget alternatives, so
   the ambiguity is contained and an ordinary widget body is untouched:

   ```antlr
   pluggableBodyV3
       : LBRACE (widgetV3 | genericContainerV3)* RBRACE
       ;

   // Any container the parent widget's def declares — an object-list item or a
   // child slot. Resolved at executor time; the parser has no opinion.
   genericContainerV3
       : (IDENTIFIER | keyword) (IDENTIFIER | QUOTED_IDENTIFIER | keyword)
         widgetPropertiesV3? widgetBodyV3?
       ;
   ```

   **`keyword` is load-bearing, not defensive**: `attribute` lexes as the
   `ATTRIBUTE` token, never as `IDENTIFIER`, so an `IDENTIFIER`-only alternative
   would not match the case that motivated this. `genericContainerV3` goes
   **last** so every enumerated widget type keeps winning.

8. **Validator.** The load-bearing half. Resolve the container against the
   parent's def and, on a miss, beat the parser's message:

   ```
   widget `frame` (htmlelement) has no object list or child slot `attribut`
     did you mean: attribute, event, tagcontentcontainer, tagcontentrepeatcontainer
   ```

   `check` already calls `LoadWidgetRegistry`. **This must land with the grammar**
   — see Open Question 1.

9. **Delete the nine** from `widgetTypeV3`. Their lexer tokens stay (several are
   used elsewhere); they stop being a capability boundary.

### Slice 4 — `DESCRIBE WIDGET` / `LIST WIDGETS`

10. An MDL statement returning what `mxcli widget describe` returns, from the
    live project. This is what makes the generated `.md` optional rather than
    load-bearing, and it is the slice that actually retires #1036's failure mode.
    `LIST WIDGETS` follows the repo convention (`list`, not `show`, for new
    commands); the existing `SHOW WIDGETS` is unrelated — it lists widget
    *instances on pages*, not definitions.

### Slice 5 — A `widget` edge in `CATALOG.REFS`

11. Emit one edge per widget instance on a page, so
    `show references to widget htmlelement` and `show impact of` work. Catalog
    work, not grammar; independent of slices 2–4.

### Slice 6 — `PLUGGABLEWIDGET` → `WIDGET` (and collapse `CUSTOMWIDGET`)

12. Deliberately last: after slice 2 the ID form is rarely written and never
    emitted, so this is a readability change on an escape hatch rather than a
    headline. Its real value is collapsing `CUSTOMWIDGET` — which already takes
    the identical code path and writes the identical BSON — into one keyword,
    removing a genuine "two ways to do one thing".

    Old spellings stay accepted (never emitted), so 164 occurrences across 42
    example, skill and doc files keep working and can be migrated at leisure.

    **The cost to weigh**: `WIDGET` already appears in
    `ALTER/DESCRIBE STYLING ON PAGE … WIDGET name`, where it means *the widget
    named X* rather than *a widget of type X*. The positions are disjoint so it
    parses, and both readings are still "widget" — the same way `PAGE` is both
    declared and referenced — but it is the argument against, and it should be
    made explicitly rather than discovered.

### Files to modify/create

| File | Change |
|------|--------|
| `modelsdk/widgets/definitions/{events,fileuploader,googletag,markdown}.def.json` | **new** — the four missing built-ins (slice 1) |
| `mdl/executor/widget_engine.go` | correct the `no definition for widget` remedy (slice 1) |
| `mdl/executor/widget_defs.go` | emit the name in generated examples (slice 1) |
| `mdl/grammar/domains/MDLPage.g4` | `()` accepted (1); identifier widget type (2); `pluggableBodyV3` + `genericContainerV3`, remove the nine (3) |
| `mdl/visitor/visitor_page_v3.go` | set `Type` from an identifier widget type and a generic container |
| `mdl/executor/validate_widgets.go` | resolve containers against the def; suggest near-misses |
| `mdl/executor/cmd_pages_describe_output.go` | emit the keyword form (slice 2) |
| `mdl/grammar/domains/MDLCatalog.g4`, `mdl/ast/`, `mdl/executor/cmd_widgets.go` | `DESCRIBE WIDGET` / `LIST WIDGETS` (slice 4) |
| `mdl/catalog/builder_references.go` | `widget` edge (slice 5) |
| `mdl-examples/doctype-tests/`, `mdl-examples/bug-tests/1036-*.mdl` | the reporter's four cases; HTML Element attributes + events + a child slot |
| `docs/01-project/MDL_QUICK_REFERENCE.md`, `cmd/mxcli/syntax/features_*.go` | the widget vocabulary is per-project, not fixed |

## Version Compatibility

None. MDL-side only: no new BSON, no Mendix API, no feature-registry entry, no
`checkFeature()` gate. A widget's object lists come from its own `.mpk`, so
version differences are already carried by the def
(`PROPOSAL_multi_version_pluggable_widgets.md`).

## Test Plan

- **The guard that makes this unreintroducible.** Derive every widget keyword,
  object-list keyword and child-slot keyword from every `.def.json` in the
  fixture, generate a minimal page per keyword, and assert it parses. This test's
  absence *is* the bug: two lists and nothing comparing them. Present numbers —
  16 of 46 containers, and 1 of 33 widget keywords — become all of them.
- **The reporter's four cases**, verbatim, in `mdl-examples/bug-tests/`.
- **Round-trip**: `create` → `describe` → `exec` for a widget with both an object
  list and a child slot, asserting the description re-parses **and** that
  `DESCRIBE` now emits the keyword form.
- **`mx check` at 0 errors** on a project carrying an HTML Element with
  `attributes`, plus a Studio Pro open. The BSON path is unchanged, but the claim
  that it is unchanged deserves one measurement.
- **Error-quality tests** (the regression risk): an unknown container names the
  valid ones for *that* widget; an unknown widget keyword names near-misses; a
  typo'd built-in widget still fails informatively.
- **Controls, per CLAUDE.md.** Reverting the validator must make the typo case
  report the raw parse error; reverting the grammar must reproduce
  `mismatched input 'attribute' expecting '}'`. A test that only passes against
  fixed code has not been shown to detect anything.

## Open Questions

1. ~~**Does error quality actually survive?**~~ **Settled: no, not as things
   stand — and the fix is a prerequisite slice, not a risk to accept.**

   The question assumed the validator would need to match what the parser
   catches. Measured, the validator catches **less than assumed**, and the gap
   is already open for every case that reaches it today:

   | written | today's verdict |
   |---|---|
   | `contaner c1 (…)` — typo'd widget kind | **parse error** (the parser is the allow-list) |
   | `pluggablewidget 'com.acme.NotAWidget' w1` — unknown widget | `check` **passes**; fails at `exec` |
   | `group g1 (…)` inside HTML Element — real keyword, wrong widget | `check` **passes**; fails at `exec` |

   So **`widgetTypeV3` is currently the widget-kind validator.** The validator
   has no independent notion of "is this a real widget kind" and does not need
   one, because nothing else can parse. Slices 2–3 remove that enforcement, so
   as written they would move *every* container mistake into the hole the last
   two rows already occupy: `check` green, failure at `exec`.

   **But the detection is already computed.** `validateWidgetTreeIn` holds the
   parent's declared object lists and looks the child up in them
   (`mapping := parentObjectLists[strings.ToUpper(w.Type)]`), and
   `lookupWidgetDef` says whether the type is a known widget. Nothing *reports*
   when both miss — the branch routes to `validateStaticWidgetUnknownProps`,
   which checks properties of a presumed static widget rather than questioning
   the kind.

   That reframes the work. Closing the hole is an improvement **today**,
   independent of any grammar change: it makes `check` catch two mistakes that
   currently reach `exec`. And once `check` reports them, this question is
   answered affirmatively **by construction**, because the semantic error exists
   before the parse error is given up.

   **Resolution: a new Slice 0 — "the validator knows what a widget is" — lands
   before slices 2–3, and they are blocked on it rather than on a decision.**
   It must report, with near-miss suggestions: an unknown widget kind or id, and
   a container the parent's definition does not declare. Its own control is that
   a *correct* widget and a *correct* container stay silent.

2. **How far does the ambiguity reach?** A generic identifier as a widget type
   makes any two consecutive identifiers a widget. Contained by construction
   (ordered last), but ANTLR's ALL(\*) behaviour deserves a measurement rather
   than an argument — build the grammar and diff error messages across the whole
   `mdl-examples/` corpus.
3. **Should child slots stay named?** Consistency says yes and existing documents
   require it. But a slot is a fixed property, not a repeating item, so its name
   is never referenced — `tagcontentcontainer { }` reads better and is what the
   generator emits today. Allowing both would be two spellings.
   Recommend: keep the name, fix the doc.
4. **Does slice 4 make the generated `.md` redundant?** It should, for an agent
   that can run `mxcli`. It does not for one reading a repo cold, which is the
   case `widget init` was built for. Likely answer: keep generating, but from the
   same code path `DESCRIBE WIDGET` uses, so they cannot disagree.
5. **What is a widget edge's source granularity?** Per instance (page + widget
   name) is most useful for impact analysis but multiplies rows on a page with 40
   widgets. Per (page, widget type) is cheaper and answers the upgrade question.
   Measure both on a real app before choosing.
6. **The built-in census.** The four in slice 1 came from scraping widget IDs out
   of `Mendix.Modeler.Core.dll` — a floor, not a census. Confirm against Studio
   Pro's widget toolbox before treating the list as complete.
