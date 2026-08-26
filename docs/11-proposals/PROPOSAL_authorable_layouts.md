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

### `LayoutType` lives on the content wrapper, and gen's `Layout.LayoutType` is a phantom

Open question, now measured across all 22 Atlas layouts on 11.13.0.

**Where it is.** Not on `Forms$Layout`. That document's complete top-level key
set is `$ID`, `$Type`, `Appearance`, `CanvasHeight`, `CanvasWidth`, `Content`,
`Documentation`, `Excluded`, `ExportLevel`, `Name`. The type is one level down,
on `Forms$WebLayoutContent` / `Forms$NativeLayoutContent` — exactly where
`generated/metamodel` puts it. **The arbiter rule holds.**

`modelsdk/gen` nonetheless exposes `Layout.LayoutType()`, bound to a key the
document does not have, so it reads `""` for every layout ever written. That is
a phantom property, not a wrong key — the `keyaudit_test.go` ledger does not
list it because there is no correct key to point at.

It had a live symptom. `DESCRIBE LAYOUT` reported **"Responsive" for all 22**
Atlas layouts, including five Phone, eight Tablet, one ModalPopup and five
native ones, because the modelsdk backend read the phantom and the describe
output defaulted `""` to `"Responsive"`. Two faults compounding: a failed read
rendered as a fact. Fixed in Stage 0; the legacy reader had it right all along.

**What the values are.**

| Platform | Content wrapper | Observed `LayoutType` |
|---|---|---|
| Web | `Forms$WebLayoutContent` | `Responsive` (3), `Tablet` (8), `Phone` (5), `ModalPopup` (1) |
| Native | `Forms$NativeLayoutContent` | `Default` (4), `Popup` (1) |

Six values, splitting by platform. Against what the code declares:

- **`modelsdk/gen`** — `PagesLayoutType {Default, Popup}`: the *native*
  vocabulary only, unable to express any of the four web values.
- **`sdk/pages`** — six values, but **`Legacy` instead of `Default`**: a value
  no Atlas layout uses, missing one that four of them do.
- **`generated/metamodel`** — right about the location, and it types both
  wrappers `PagesLayoutType`, so it also lists 2 of the 6.

So the property location is settled by the metamodel and the *value set* is not
settled by anything but real documents — which is what
`MODELSDK_ENGINE_ARCHITECTURE.md` means by "capture, don't guess".

The approach that follows:

1. **Read and write it on the content wrapper**, never on `Layout`.
2. **Validate against the platform**, not a flat list — `Responsive` on a native
   layout is meaningless, and so is `Default` on a web one. The wrapper's
   `$Type` decides which vocabulary applies.
3. **Accept `Legacy` on read, do not offer it for authoring.** Declared in
   `sdk/pages`, present in no Atlas layout, presumably Mendix 7-era. Refusing to
   author a value never seen written is the honest default; reading it is free.
4. **Never default an empty value.** Every real layout stores one, so `""` means
   the read failed — reporting it as `Responsive` is how this stayed hidden.

### Known gen mislabel, already patched on the page side

`genPg.NewLayoutCallArgument()` produces `Forms$LayoutCallArgument`; the real
BSON is `Forms$FormCallArgument`. `page_write.go` already overrides it with an
explicit `SetTypeName`. The layout side will hit the same thing.

## Proposed MDL Syntax

### CREATE LAYOUT

Named region slots map straight onto the BSON. As shipped:

```sql
create or replace layout Ledger.Ledger_Default (
  layouttype: 'Responsive'
) {
  scrollcontainer scMain {
    region top (size: 60, sizemode: 'Fixed', class: 'region-topbar') {
      snippetcall SNIP_TopBar (snippet: Ledger.SNIPPET_TopBar)
    }
    region left (size: 232, sizemode: 'Pixels', class: 'region-sidebar') {
      navigationtree navMenu (profile: 'Responsive')
    }
    region center (class: 'region-content') {
      placeholder Main
    }
  }
}
```

Two departures from the draft above, both forced by measurement (see Stage 2):

- **`region top`, not a bare `top`.** Every other widget in MDL is
  `<type> <name> (props) { body }`, and a region is the one place where the
  "name" is a fixed position rather than free text. Keeping the shape means one
  keyword instead of five and no special case in the widget grammar.
- **No `mainplaceholder:`.** `Forms$Layout` has no such property; naming a
  placeholder `Main` is the mechanism.

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

### Stage 2 — `CREATE LAYOUT` — **shipped**

Document type through the `MODELSDK_ENGINE_ARCHITECTURE.md` recipe: `sdk/pages.Layout`
expanded past its five fields, `layoutToGen` in `mdl/backend/modelsdk/layout_write.go`,
grammar + AST + visitor + executor handler, and a legacy refusal (that writer's
serializer emitted a string `$ID`, no `Content` wrapper and a `LayoutType` on the
wrong node — it had never been called, because until now nothing created a layout).

Three things the implementation settled that the design above did not anticipate:

**The header has exactly one property, and `mainplaceholder:` is not it.** The
draft syntax carried one, and it was wrong. `Forms$Layout` has exactly ten keys —
`$ID`, `$Type`, `Appearance`, `CanvasHeight`, `CanvasWidth`, `Content`,
`Documentation`, `Excluded`, `ExportLevel`, `Name` — identical across all 22
layouts Atlas ships on 11.13.0, and `generated/metamodel`'s `PagesLayout` declares
exactly that set. `modelsdk/gen` additionally exposes seven placeholder properties
on `Layout` (`MainPlaceholderName`, `AcceptPlaceholderName`,
`UseMainPlaceholderForPopups`, …) that no real document carries. Writing one gives
a layout **mxbuild accepts at 0 errors** — measured — **and Studio Pro cannot
open**, because it resolves every stored property against the type's list. Which
placeholder is "main" is a naming convention: 22 of 22 name one `Main`, and a page
binds by qualified name (`Atlas_Core.Atlas_Default.Main`) regardless. So the
header takes `layouttype` and refuses anything else rather than ignoring it —
silently ignoring `mainplaceholder:` would report acceptance for a value that was
never stored.

**The platform is inferred, not declared.** The two layout-type vocabularies are
disjoint (web: Responsive/Phone/Tablet/ModalPopup; native: Default/Popup), so a
`native:` flag could only ever contradict the type. A cross-platform value is
refused.

**The marketplace guard needed a backend fix to be real.** `FromAppStore` was
populated only by `ListModules`; `GetModuleByName` and `GetModule` returned
ID+Name, so a guard reading it would have been inert for every module — a guard
that never fires. The enrichment is now wired into every module lookup.

Verified end to end on a real 11.13.0 app: `mx check` 0 errors, describe →
re-exec → describe is byte-stable, the key set matches Atlas's exactly, and the
layout **renders in a browser** with all three regions in place and the page's own
widget in the `Main` placeholder — the check `mx check` cannot make.

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

### Stage 3 — `ALTER PAGE … SET LAYOUT`, then `ALTER LAYOUT` — **shipped**

`ALTER PAGE … SET Layout = QN [MAP (…)]` already existed and already rewrote
`Form` and every `Parameter` (verified on a real 11.13 project). What Stage 3
added:

**The bulk form.** `ALTER PAGES [IN <module>] SET LAYOUT = X [MAP (…)]
[WHERE LAYOUT = Y]`, reusing `alterLayoutMapping`. Marketplace pages are
**skipped and named**, not refused — a project-wide repoint that stopped dead on
Administration's pages would be unusable. A `WHERE LAYOUT` that names no real
layout is an **error**, because matching nothing would report "0 pages": success,
for a typo.

**A placeholder check on both forms.** A page bound to a placeholder the target
does not declare was written happily; mxbuild catches it (CE1613, measured) but
at the far end of a build and naming the page, not the statement. The check runs
*after* MAP, because MAP is the remedy — and the message names that remedy in the
syntax that parses. The grammar is `MAP (Old AS New)`; the comment beside the
rule had said `MAP (Main -> Main)` since it was written, and the first version of
this error message copied it.

**`ALTER LAYOUT`**, taking the whole `ALTER PAGE` operation vocabulary rather
than a syntax of its own — a layout's widget tree *is* a page's plus four element
types. Two finder gaps had to close first: the page finder starts at a `FormCall`
a layout does not have, and nothing descended into a scroll container's five
named slots, so every widget in every layout was unreachable and so was anything
inside a scroll container on a page. A region is addressed as
`layoutContainer.top`, reusing the dotted widgetRef that also serves DataGrid2
columns; which one it means is decided by the named widget's `$Type` rather than
by new syntax. Marketplace targets are refused, naming the copy-then-repoint
route.

**The reason ALTER LAYOUT is a capability and not a convenience.** The proposal
argued it saves rewriting the document. The stronger argument is fidelity, and it
is measurable: describe → rename → exec of `Atlas_Core.Atlas_SideBar` **loses both
`Forms$SidebarToggleButton` widgets**, and an `image` widget loses its image
reference (CE0463 until `mxcli fix widgets`, then "No image selected"). A rewrite
is only ever as complete as the describe it came from. Those widgets were already
emitted as comments; the comment now says so — `-- NOT re-executable` — because a
bare `-- Forms$X (name)` line reads as informational. The image round-trip loss is
a separate defect in pluggable-widget describe, not fixed here.

### Stage 3 (original plan)

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
