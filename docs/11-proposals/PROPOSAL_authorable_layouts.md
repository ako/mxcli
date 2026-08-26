---
title: Authorable layouts, page templates and building blocks — CREATE, not COPY
status: draft
date: 2026-08-26
related:
  - PROPOSAL_marketplace_module_upgrade.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - docs/13-decisions/0005-semantic-model-interface-currency.md
  - .claude/skills/mendix/migrate-design-prototype.md
---

# Authorable layouts, page templates and building blocks — CREATE, not COPY

## Problem Statement

An app built entirely through MDL cannot put anything in its own topbar.

The topbar belongs to the layout; `SHOW LAYOUTS` lists 22 of them and every one
lives in `Atlas_Core`. `DESCRIBE LAYOUT` prints:

```
-- Layouts cannot be created via MDL; they must be created in Studio Pro.
```

Reported by [ako/mxcli-ledger #136](https://github.com/ako/mxcli-ledger/blob/main/FINDINGS.md),
whose brief was two dropdowns in the topbar. The workaround was a snippet
repeated on nine pages, sitting under the page title rather than above it —
"the honest position for something the layout does not own."

Building blocks are in the same position (`-- Building blocks are read-only`),
and page templates likewise. These are exactly the three document types Mendix's
own guidance groups together.

### Editing the Atlas one is not the answer

[Design Principles](https://docs.mendix.com/refguide/mobile/designing-mobile-user-interfaces/design-principles/):

> The default Atlas theme comes pre-bundled with a set of layouts. If these do
> not fit your app's design, you can create or customize the layouts and name
> them accordingly. **Do not change the supplied layouts. Either create a
> separate module with the custom layouts, page templates, and building blocks
> or create your own.**

(That page is native-mobile-scoped. The web-side equivalent is softer —
[Create a Company Design System](https://docs.mendix.com/howto/front-end/create-a-company-design-system/)
tells you to make a company theme module and add or change layouts *there*. The
underlying rule is the general marketplace one, not layout-specific: a
marketplace module's contents are replaced on update.)

`Atlas_Core` is the module `mxcli marketplace diff` exists to warn you about
editing. `ALTER LAYOUT Atlas_Core.Atlas_Default` would manufacture the drift
another mxcli feature reports, so it must be refused.

## Rejected alternative: a `COPY DOCUMENT` verb

The first draft of this proposal was built around `COPY LAYOUT Atlas_Core.Atlas_Default
TO Ledger.Ledger_Default` — clone the unit, re-mint identities, repoint pages —
on the reasoning that a copy needs no semantic model for what it duplicates, so
it sidesteps authoring entirely.

**Two things kill it.**

**1. A copy is opaque, and that is worse than not having it.** `DESCRIBE LAYOUT`
prints nothing for an 84-element document today. Copying would put a document in
your module that no mxcli command can show you, diff, lint or round-trip — and
it would no longer be a marketplace module, so `marketplace diff` could not
describe it either. That trades "can't edit Atlas's layout" for "owns something
illegible", which is a bad trade for a tool whose premise is that the model is
expressible as re-executable text.

**2. Copying is already `DESCRIBE` → rename → `exec`,** wherever the type can be
authored. So COPY is only distinguishable while authoring is missing — it is a
workaround for the gap this proposal closes, and it stops paying rent the moment
the gap is closed. It also produces reviewable MDL, which a unit clone does not.

**And the cost estimate that justified it was wrong.** I claimed `CREATE LAYOUT`
was "substantially bigger". Measured, against `Atlas_Core.Atlas_Default` on
11.13.0:

| Element type | in `modelsdk/gen` | already written by the page path |
|---|---|---|
| `LayoutGrid` / `Row` / `Column` | yes | **yes** |
| `SnippetCallWidget` / `SnippetCall` | yes | **yes** |
| `Appearance` | yes | **yes** |
| `DesignPropertyValue` / `OptionDesignPropertyValue` | yes | **yes** |
| `ClientTemplate` | yes | **yes** |
| `Layout` | yes | no |
| `WebLayoutContent` | yes | no |
| `ScrollContainer` | yes | no |
| `ScrollContainerRegion` | yes | no |
| `Placeholder` | yes | no |
| `NavigationTree` | yes | no |

**Ten of sixteen are already authored for pages** — including all the tedious
scaffolding. What is missing is one document type and five element types, *all
of which already exist in `modelsdk/gen/pages`*. `widgetToGen` is a type switch
with 36 cases at ~12 lines each; four more is a normal afternoon, not a project.

The 84-element / 22-type figure that made a layout look daunting is mostly
scaffolding mxcli emits on every page it writes.

## BSON Structure

Measured on `Atlas_Core.Atlas_Default`, Mendix 11.13.0, via `mxcli bson dump`.

### The layout document

```
Forms$Layout
└── Content → Forms$WebLayoutContent
              ├── LayoutCall  : null          ← a master layout, when nested
              ├── LayoutType  : "Responsive"
              └── Widgets     → Forms$ScrollContainer
```

### ScrollContainer regions are named slots, not a list

```
Forms$ScrollContainer
├── Top          : Forms$ScrollContainerRegion | null
├── Right        : … | null
├── Bottom       : null
├── Left         : Forms$ScrollContainerRegion   ← the sidebar
└── CenterRegion : Forms$ScrollContainerRegion   ← note the name
```

`CenterRegion` is spelled differently from its four siblings. Each region
carries `Class`, `Size`, `SizeMode` (`"Auto"` / fixed) and `Widgets`.

In `Atlas_Default` those slots hold:

| slot | element |
|---|---|
| top | `Forms$SnippetCallWidget` → `Forms$SnippetCall` — **the topbar** |
| left | `Forms$NavigationTree` |
| center | `Forms$Placeholder` — `Name: "Main"` |

### How a page binds to a layout — two qualified-name keys

Measured on `Administration.Account_Overview`:

| Key | Value | On |
|---|---|---|
| `Form` | `Atlas_Core.Atlas_Default` | `Forms$LayoutCall` |
| `Parameter` | `Atlas_Core.Atlas_Default.Main` | `Forms$FormCallArgument` |

Three consequences:

1. **The layout reference is stored under `Form`, not `Layout`** — the same
   "Form was the original term for Page" convention as `ShowFormAction` /
   `CloseFormAction` in CLAUDE.md's storage-name table. `modelsdk/gen` calls it
   `LayoutQualifiedName`; the BSON key is `Form`.
2. **The placeholder binding embeds the layout's module and name**, so
   repointing a page is *two* rewrites — one `Form`, plus one `Parameter` per
   placeholder argument.
3. **Placeholder names are API.** A page says `Ledger.Ledger_Default.Main`; the
   layout must declare a placeholder called `Main`. Renaming one silently
   unbinds every page that used it.

### `LayoutType` — neither declaration is right, and it is stored twice

Open question, now measured across all 22 Atlas layouts on 11.13.0.

| Platform | Content wrapper | Observed `LayoutType` |
|---|---|---|
| Web | `Forms$WebLayoutContent` | `Responsive` (3), `Tablet` (8), `Phone` (5), `ModalPopup` (1) |
| Native | `Forms$NativeLayoutContent` | `Default` (4), `Popup` (1) |

Six values, splitting by platform. Set against what the code declares:

- **`modelsdk/gen`** — `PagesLayoutType {Default, Popup}`. That is the *native*
  vocabulary only; it cannot express any of the four web values.
- **`sdk/pages`** — six values, but **`Legacy` instead of `Default`**. It has a
  value no Atlas layout uses and is missing one that four of them do.
- **`generated/metamodel`** (the 11.6 snapshot) declares `LayoutType` on
  `PagesNativeLayoutContent` and `PagesWebLayoutContent` only, both typed
  `PagesLayoutType` — so it too lists 2 of the 6, and does **not** record that
  `Forms$Layout` itself carries the property.

**This is a documented counter-example to CLAUDE.md's arbiter rule.** That rule
says `generated/metamodel` decides when gen and it disagree. Here they agree
with each other and both disagree with every real document. The tiebreaker is
the one the architecture doc already names: *capture, don't guess*. `Responsive`,
`Phone` and `Tablet` are Atlas-era concepts that long predate 11.6, so this is
not the documented "property introduced after the snapshot" caveat — the
reflection data simply under-declares this enum.

**And the value is stored twice** — on `Forms$Layout` *and* on its content
wrapper, identical in all 22 (spot-checked on `Atlas_Default`, `PopupLayout`,
`NativePhone_PopOver`). That is the `Interval`/`IntervalType`-beside-`Schedule`
shape from CLAUDE.md: a redundancy Studio Pro maintains and a writer must
maintain too, not one to pick a winner from.

The approach that follows:

1. **Write both**, always the same value.
2. **Validate against the platform**, not against one flat list — `Responsive`
   on a native layout is meaningless, and so is `Default` on a web one. The
   content wrapper's `$Type` is what decides which vocabulary applies.
3. **Accept `Legacy` on read, do not offer it for authoring.** It is declared in
   `sdk/pages` and appears in no Atlas layout; it is presumably a Mendix 7-era
   web layout. Refusing to author a value we have never seen written is the
   honest default, and reading it costs nothing.
4. **Do not regenerate the enum from reflection data** without re-measuring —
   it would silently narrow the accepted set back to two.

### Known gen mislabel, already patched on the page side

`genPg.NewLayoutCallArgument()` produces `Forms$LayoutCallArgument`; the real
BSON is `Forms$FormCallArgument`. `page_write.go` already overrides it with an
explicit `SetTypeName`. The layout side will hit the same thing.

## Proposed MDL Syntax

### CREATE LAYOUT

Named region slots map straight onto the BSON:

```sql
create layout Ledger.Ledger_Default (
  layouttype: Responsive,
  mainplaceholder: 'Main'
) {
  scrollcontainer scMain {
    top (size: 60, sizemode: Fixed, class: 'region-topbar') {
      snippetcall SNIP_TopBar (snippet: Ledger.SNIPPET_TopBar)
    }
    left (size: 200, sizemode: Auto, class: 'region-sidebar') {
      navigationtree navMenu (profile: 'Responsive')
    }
    center (class: 'region-content') {
      placeholder Main
    }
  }
}
```

`create or replace layout` follows the house convention for re-runnable scripts.

Reproducing Atlas's structure is ~15 lines you wrote and can read — against 84
elements you cloned and cannot. That is the trade this proposal is making.

### CREATE PAGETEMPLATE / BUILDINGBLOCK

Same document-type recipe, same widget vocabulary, and both already have gen
types. Worth doing in the same arc because Mendix's guidance names all three
together and a company design-system module wants all three.

### Repointing pages

Still needed — a layout nobody uses changes nothing:

```sql
alter page Ledger.Dashboard set layout Ledger.Ledger_Default;

alter pages in Ledger set layout Ledger.Ledger_Default
  where layout = Atlas_Core.Atlas_Default;
```

An app has one layout and many pages, so the bulk form is the real one. Both
must rewrite `Form` **and** every `Parameter`.

### ALTER LAYOUT

Iterating without rewriting the whole document, through `pagemutator` (which
already knows `Forms$ScrollContainerRegion`):

```sql
alter layout Ledger.Ledger_Default
  replace in top
    snippetcall SNIP_TopBar (snippet: Ledger.SNIPPET_ThemeBar);
```

**Refuse a target in a marketplace module**, naming `create layout` instead.

### DESCRIBE

`DESCRIBE LAYOUT` prints an empty widget structure today (measured: 0 lines for
an 84-element document). It is a prerequisite for everything here — it is the
acceptance test for authoring, *and* it is the copy mechanism, since
describe → rename → exec is how you clone a layout once one can be created.

## Implementation Plan

### Stage 0 — `DESCRIBE LAYOUT` emits the widget tree

A read bug on its own. Prerequisite for the round-trip gate and for copying.

### Stage 1 — the five missing element types

`ScrollContainer`, `ScrollContainerRegion`, `Placeholder`, `NavigationTree`,
`WebLayoutContent` — four widget cases in `widgetToGen`/`widgetFromGen` plus the
content wrapper. All present in gen.

### Stage 2 — `CREATE LAYOUT`

Document type through the `MODELSDK_ENGINE_ARCHITECTURE.md` recipe: expand
`sdk/pages.Layout` past its current five fields, `layoutToGen`/`FromGen`,
parity test in `mdl/enginecompare/`.

### Stage 4 — `mxcli new` scaffolds a project-owned layout

**Decided.** A new project gets `MyFirstModule.App_Default` — a layout it owns,
created from MDL, with its pages pointed at it.

Three reasons it belongs in the arc rather than after it:

- It makes the documented practice the **default** one. Today every generated
  app starts out in the position finding #136 describes: a layout it cannot
  touch, in a module it must not edit. Scaffolding one inverts that from an
  obstacle you discover to a starting point you already have.
- It is the **first real consumer** of everything above, so it is also the
  acceptance test. If `mxcli new` can emit a layout, repoint the pages and build
  clean, the feature works.
- It gives `mxcli theme switcher install` somewhere to put its button. That
  command's standing limitation is that mxcli cannot reach a layout, so the
  generated toggle has to be wired by hand onto a page — the loose end from the
  theme work closes here.

Cost: one more document in a blank app, and `mxcli new --layout none` for anyone
who would rather start from Atlas's.

Two things to get right. The scaffold must reproduce Atlas's structure closely
enough that pages render unchanged — same placeholder name (`Main`), same region
classes — so switching to it is invisible until someone edits it. And it must be
**generated as MDL**, not special-cased in Go, so what a new project contains is
something the user could have written and can re-run.

### Stage 3 — `ALTER PAGE … SET LAYOUT`, then `ALTER LAYOUT`

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/grammar/MDLLexer.g4` | `SCROLLCONTAINER`, `NAVIGATIONTREE`, `REGION` / slot names |
| `mdl/grammar/domains/MDLPage.g4` | `createLayoutStatement`, region slots, `placeholder` declaration, `SET LAYOUT` |
| `mdl/ast/ast.go` | `CreateLayoutStmt`, `AlterLayoutStmt`, `SetLayoutStmt` |
| `mdl/visitor/visitor_entity.go` | Dispatch; **watch the MOVE discriminator** — a doctype keyword added to one rule and not the other silently reparses |
| `mdl/executor/cmd_layouts.go` | New — thin handler over `ctx.Backend` |
| `mdl/executor/cmd_pages_describe.go` | Stage 0; drop the "cannot be created" notice at Stage 2 |
| `mdl/backend/page.go` | `CreateLayout` exists on the interface already; add `SetPageLayout`, `OpenLayoutForMutation` |
| `mdl/backend/modelsdk/layout_write.go` | New — `layoutToGen` / `layoutFromGen` |
| `mdl/backend/modelsdk/widget_write.go` | 4 new cases |
| `sdk/pages/pages.go` | Expand `Layout` (today: Name, Documentation, LayoutType, MainPlaceholderID, Widget) |
| `sdk/mpr/writer_pages.go` | Replace the `serializeLayout` **stub** — it writes 5 header keys, no content, and `$ID` as a string where snippets use `idToBsonBinary` |
| `mdl/enginecompare/bsoncompare.go` | `LayoutCanonBSON` dumper |
| `cmd/mxcli/syntax/features_page.go` | New `SyntaxFeature` entries — **#136's literal ask** |
| `docs/01-project/MDL_QUICK_REFERENCE.md` | Layout rows |
| `cmd/mxcli/cmd_new.go` | Stage 4: scaffold `App_Default`, `--layout none` to opt out |

## Version Compatibility

None needed. Layouts, placeholders and scroll containers are not new. The
`Form` / `Parameter` keys were measured on 11.13.0 and should be re-checked
against a 9.x project before release, since this proposal rests on them.

## Test Plan

- **Round-trip** — `DESCRIBE LAYOUT` of a created layout re-parses. This is the
  gate; it also delivers copying for free.
- **Parity** — `mdl/enginecompare/` legacy vs modelsdk, byte-identical, per the
  recipe. Note legacy's serializer is a stub, so **real Studio Pro BSON is the
  tiebreaker**, not legacy.
- **Integration** (`-tags integration`) — create a layout, repoint every page,
  `mx check` → 0 errors. A page whose `Parameter` still names the old layout is
  the failure this feature can cause.
- **Runtime** — per `.claude/skills/verify-in-runtime.md`, boot with
  `run --local` and assert the topbar renders. A layout can serialize cleanly,
  pass `mx check` and still render wrong.
- **Drift** — with a project-owned layout in use, `mxcli marketplace diff` on
  Atlas_Core reports no local edits. The thesis as an assertion.
- **MDL example** — `mdl-examples/doctype-tests/`.

## Open Questions

1. **Is `mainplaceholder` worth a header property, or inferable?** `Forms$Layout`
   carries `mainPlaceholder` (a `ByNameRef`) *and* `mainPlaceholderName` (a
   string), plus accept/cancel pairs. Two representations of one fact — the
   ADR-0005 overlay shape. Read what is stored, do not write both blindly. A
   layout with exactly one placeholder could default it.
2. **Nested layouts.** `WebLayoutContent.LayoutCall` is `null` in
   `Atlas_Default`, so a layout *can* have a master. Does `create layout … using
   Atlas_Core.Atlas_Default` earn its place, or is it scope creep?
3. **`alter pages … where layout =` is a new clause shape.** Nothing else in MDL
   filters an ALTER by a reference. Check against
   `.claude/skills/design-mdl-syntax.md` before committing to it.
4. **Native layouts are out of scope, but the shape is now known.** `Atlas_Core`
   ships five `NativePhone_*` layouts: same `Forms$Layout` document, wrapper
   `Forms$NativeLayoutContent` instead of `Forms$WebLayoutContent` (measured).
   Their widget vocabulary is native, so authoring them is a separate arc.
