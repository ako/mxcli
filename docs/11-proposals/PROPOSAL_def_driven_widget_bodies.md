---
title: Def-driven pluggable widget bodies
status: draft
date: 2026-09-04
related:
  - PROPOSAL_mcp_pluggable_widget_authoring.md
  - PROPOSAL_multi_version_pluggable_widgets.md
  - PROPOSAL_widget_property_visibility.md
---

# Proposal: Def-driven pluggable widget bodies

**Status:** Draft
**Date:** 2026-09-04

A pluggable widget's object lists and child slots are reachable from MDL only
when their keyword appears in a hand-maintained list of nine in the grammar.
Everything else — every other widget's object lists, and nearly every child slot
— is unreachable, while `mxcli widget init` documents all of it as though it
were available. This proposal deletes the list.

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

Fed back into `mxcli check` verbatim, that fails on the first line of its own
body:

```
line 4:4 mismatched input 'tagcontentcontainer' expecting '}'
```

The file is written *for LLM agents to follow* and carries a widget's real
property table, so it reads as authoritative. Multiple sessions concluded the
feature existed.

### The mechanism: two lists, nothing comparing them

| | |
|---|---|
| **The doc generator** derives a keyword for every object list and child slot, mechanically | `deriveObjectListKeyword` singularises any property key (`attributes` → `ATTRIBUTE`); child slots use `strings.ToUpper(child.Key)`. Unbounded. |
| **The grammar** accepts nine hardcoded object-list keywords | `MDLPage.g4:402–410` — GROUP, CUSTOMITEM, MARKER, DYNAMICMARKER, SERIES, LINE, SCALECOLOR, CUSTOMBUTTON, ALLOWEDFILEFORMAT |

They are structurally guaranteed to drift, and nothing checks one against the
other. Measured against this repo's fixture project (33 widget defs, 46
documented constructs):

| | parses | rejected |
|---|---|---|
| Object lists | 13/16 | `attribute`, `event`, `attr` |
| Child slots (named, under `pluggablewidget`) | 3/30 | 27 |
| **Total** | **16/46** | **30 (65%)** |

20 of 33 widgets document at least one keyword that cannot parse.

**Control** — identical widget, identical body shape, only list membership
differs:

```mdl
group g1 (headerText: 'x')              -- PARSES
attribute a1 (attributeName: 'title')   -- REJECTED
```

### Why "make the docs honest" is not enough

Option 2 of the issue (emit only what the grammar accepts) converts a silent
failure into a documented dead end. Worth doing, and it is proposed below as the
interim step — but it would not have unblocked the reporter. There is no other
route: `ALTER PAGE` rejects the same construct, verified with a working control.

```mdl
alter page M.Sandbox {
  insert into frame { dynamictext t1 (Content: 'hello') }   -- PARSES (control)
};
alter page M.Sandbox {
  insert into frame { attribute a1 (attributeName: 'sandbox') }   -- REJECTED
};
```

They would have had accurate documentation of a capability gap, and shipped the
same-origin fallback anyway.

## BSON Structure

**No new BSON, and no new write path.** This is the finding that makes the
proposal small, and it should be verified before any code is written, because
the whole design rests on it.

`PluggableWidgetEngine.applyObjectLists` (`widget_engine.go:1068`) is **already
fully def-driven**:

```go
byContainer := make(map[string]*ObjectListMapping, len(lists))
for i := range lists {
    byContainer[strings.ToUpper(lists[i].MDLContainer)] = &lists[i]
}
for _, child := range w.Children {
    mapping, ok := byContainer[strings.ToUpper(child.Type)]
    ...
}
```

It matches the AST child's `Type` **string** against whatever the def declares.
It has no knowledge of the nine keywords.

And the visitor sets that string from the token's literal text
(`visitor_page_v3.go:554`):

```go
widget.Type = strings.ToLower(typeCtx.GetText())
```

So the entire pipeline below the parser is text-driven and generic. The nine
keywords exist **only so ANTLR has a token to match**. That is an
implementation artefact of the parser generator, leaking out as a capability
boundary — and it is why `attribute` and `event` would be written correctly
today if the parser merely produced the node.

The object-list BSON itself is unchanged: `builder.SetObjectList(propertyKey,
items)` already writes the nine, and the item shape comes from the def's
`itemProperties` / `itemSlots`, not from the keyword.

## Proposed MDL Syntax

**The syntax does not change.** That is the point — the forms already
documented start working:

```mdl
create page Sales.Frame ( Title: 'Frame', Layout: Atlas_Core.Atlas_Default )
{
  pluggablewidget 'com.mendix.widget.web.htmlelement.HTMLElement' frame (
    tagName: 'div'
  ) {
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

Every element already has one spelling: `<container> <name> ( props )` for an
object-list item, `<slot> <name> { widgets }` for a child slot — identical to
how `group`, `series` and `controlbar` are written today. Nothing new to learn,
one-line diffs, and `DESCRIBE PAGE` round-trips as it already does for the nine.

### Design notes against `design-mdl-syntax.md`

- **Reuse existing keywords first.** This proposal goes further: it stops
  *adding* them. Today every new widget with an object list needs a new reserved
  word, which is an unbounded cost paid in name collisions (#619's quoting
  escape hatch exists for exactly this).
- **No keyword overloading**, no new verbs, no implicit context: the container
  name is resolved against the parent widget's own definition, which is as
  explicit as a qualified name.
- **One example is enough for an LLM**, and — critically — the example the
  generator already emits becomes the correct one.

### The one required doc change

The generated example omits the **name**, which even a working slot requires:

```
tagcontentcontainer { ... }        -- as generated today, rejected
tagcontentcontainer content { }    -- correct
```

So the three child slots that *could* parse today are documented in a form that
cannot either. Fixed alongside.

## Implementation Plan

### Slice 1 — Stop the bleeding (no design risk, ship first)

Independently useful, and correct whether or not slice 2 lands.

1. **Embed the four missing built-in defs.** Measured: `events`, `fileuploader`,
   `googletag`, `markdown` have no definition anywhere — not in `widgets/`
   (Studio Pro never materialises them; their `.mpk`s live inside
   `Mendix.Modeler.Core.dll`), and not in `modelsdk/widgets/definitions/`.
   All four pass `check` and fail `exec` with `no definition for widget`.
2. **Fix that error message.** It currently says
   `(run 'mxcli widget init -p app.mpr')` — a remedy that **provably cannot
   work**, since `widget init` scans `widgets/` and these are not there. It
   should say the widget is built into Studio Pro and has no bundled definition.
3. **Generated examples include the name** on child slots.
4. **`()` is accepted.** `widgetPropertiesV3` requires at least one property, so
   `container c ()`, `text t ()` and `pluggablewidget … pw ()` are all parse
   errors while bare `pw` and `pw (x: 'y')` are fine. One-line grammar fix,
   unrelated to the rest but found in the same investigation.

### Slice 2 — The def-driven body

5. **Grammar.** A parallel body rule used *only* by the two pluggable
   alternatives, so the ambiguity is contained and an ordinary widget body is
   untouched:

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
   would not match the very case that motivated this. `genericContainerV3` goes
   **last** so every enumerated widget type keeps winning.

6. **Visitor.** One branch setting `widget.Type` from the container's literal
   text — the same thing line 554 already does for `widgetTypeV3`.

7. **Validator.** The load-bearing half. Resolve the container against the
   parent's def and, on a miss, produce a better error than the parser's:

   ```
   widget `frame` (htmlelement) has no object list or child slot `attribut`
     did you mean: attribute, event, tagcontentcontainer, tagcontentrepeatcontainer
   ```

   `check` already calls `LoadWidgetRegistry`, so the data is in hand. **This
   must land in the same change as the grammar** — see Open Question 1.

8. **Delete the nine.** Remove the object-list keywords from `widgetTypeV3`.
   Their lexer tokens stay (several are used elsewhere), but they stop being a
   capability boundary.

9. **Doc generator.** Once the parser has no opinion, mechanical derivation is
   correct by construction and the generator needs no allow-list.

### Files to modify/create

| File | Change |
|------|--------|
| `modelsdk/widgets/definitions/{events,fileuploader,googletag,markdown}.def.json` | **new** — the four missing built-ins (slice 1) |
| `mdl/executor/widget_engine.go` | correct the `no definition for widget` remedy (slice 1) |
| `mdl/executor/widget_defs.go` | emit the name in generated examples (slice 1); drop any allow-list once slice 2 lands |
| `mdl/grammar/domains/MDLPage.g4` | `widgetPropertiesV3` accepts `()` (slice 1); `pluggableBodyV3` + `genericContainerV3`; remove the nine from `widgetTypeV3` (slice 2) |
| `mdl/visitor/visitor_page_v3.go` | set `Type` from a generic container's text |
| `mdl/executor/validate_widgets.go` | resolve containers against the def; suggest near-misses |
| `mdl-examples/doctype-tests/` | HTML Element attributes + events + a child slot |
| `mdl-examples/bug-tests/1036-*.mdl` | the reporter's four cases |
| `docs/01-project/MDL_QUICK_REFERENCE.md`, `cmd/mxcli/syntax/features_*.go` | the body vocabulary is per-widget, not fixed |

## Version Compatibility

None. This is MDL-side only: no new BSON, no Mendix API, no feature-registry
entry, no `checkFeature()` gate. A widget's object lists come from its own
`.mpk`, so version differences are already carried by the def
(`PROPOSAL_multi_version_pluggable_widgets.md`).

## Test Plan

- **The guard that makes this unreintroducible.** Derive every object-list and
  child-slot keyword from every `.def.json` in the fixture, generate a minimal
  page per keyword, and assert it parses. This is the test whose absence *is*
  the bug: two lists and nothing comparing them. Present numbers — 16 of 46 —
  become 46 of 46.
- **The reporter's four cases**, verbatim, in `mdl-examples/bug-tests/`.
- **Round-trip**: `create` → `describe` → `exec` for a widget with both an
  object list and a child slot, asserting the description re-parses.
- **`mx check` at 0 errors** on a project carrying an HTML Element with
  `attributes`, plus a Studio Pro open — the BSON path is unchanged, but the
  claim that it is unchanged is worth one measurement.
- **Error-quality tests** (the regression risk): an unknown container names the
  valid ones for *that* widget; a typo'd top-level widget still fails at parse.
- **Controls, per CLAUDE.md.** Reverting the validator must make the typo case
  report the raw parse error; reverting the grammar must reproduce
  `mismatched input 'attribute' expecting '}'`. A test that only passes against
  fixed code has not been shown to detect anything.

## Open Questions

1. **Does error quality actually survive?** The tradeoff is real: today a typo'd
   container is a clean parse error; afterwards it is a semantic one. The claim
   that semantic is *better* rests on the validator naming the widget's real
   containers, which is unproven until written. **If the validator cannot be
   made to fire everywhere the parser did — notably with no `--project`, where
   only built-in defs load — the grammar change should not ship.** This is the
   one question that could sink the slice.
2. **How far does the ambiguity actually reach?** `genericContainerV3` makes any
   two consecutive identifiers a container inside a pluggable body. Contained by
   construction (ordered last, pluggable bodies only), but ANTLR's ALL(\*)
   behaviour here deserves a measurement rather than an argument — build the
   grammar and diff the error messages across the whole `mdl-examples/` corpus.
3. **Should child slots stay named?** Consistency says yes and existing
   documents require it. But a slot is a fixed property, not a repeating item,
   so its name is never referenced — `tagcontentcontainer { }` reads better and
   is what the generator already emits. Allowing both would mean two spellings,
   which `design-mdl-syntax.md` forbids. Recommend: keep the name, fix the doc.
4. **`CUSTOMWIDGET` too?** The legacy dojo form takes the same body rule here.
   It has no `.def.json`, so every container would be unresolvable and rejected
   — correct, but the error should say the widget kind has no definitions rather
   than listing none.
5. **The other three built-ins.** The four in slice 1 came from scraping widget
   IDs out of `Mendix.Modeler.Core.dll`, which is a floor, not a census. Worth
   confirming against Studio Pro's widget toolbox before treating the list as
   complete.
