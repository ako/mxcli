---
title: use building block copies the stored widgets instead of round-tripping them through MDL
status: draft
date: 2026-11-15
related:
  - ako/mxcli#2212 (the issue this answers)
  - show-describe-building-blocks.md (the proposal that chose the round trip)
  - ako/mxcli#2180, #2181, #2183 (three bugs the round trip caused)
---

# Proposal: Copy a building block's stored widgets

**Status:** Draft
**Date:** 2026-11-15
**Issue:** ako/mxcli#2212

## Problem Statement

`use building block M.Block [as prefix_] [(datasource: …, action: …)]` doesn't copy
the block. `expandBuildingBlockRef` (`mdl/executor/cmd_pages_builder_v3.go`) does this
instead:
1. renders the block's stored widgets as MDL with the `describe` printer;
2. wraps them in `create fragment __bbtmp as { … };`;
3. re-parses that with `visitor.Build`;
4. builds the result like hand-written MDL.

`show-describe-building-blocks.md` chose this to avoid a hand-written BSON→AST
converter, and checked only that the Atlas Card block re-parses.

A block therefore places correctly only if its widgets survive describe → parse →
build without loss. Each gap in that round trip has become a block bug. The
PlatformDesignSystem blocks found three in one batch:

| Issue | How the round trip broke the block |
|---|---|
| #2181 | `describe` printed `{1} = <unbound>`, which the parser refused, so the whole block failed |
| #2180 | a pluggable widget's own `icon` was dropped on the way back in |
| #2183 | Studio Pro's own values looked hand-written after re-parsing, and hit refusals meant for hand-written MDL; it needed `ast.WidgetV3.FromBuildingBlock` to tell them apart |

Each was fixed on its own; the class remains. Any value `describe` doesn't print
exactly, or that the builder writes differently from Studio Pro, silently changes a
placed block or refuses it. The #721 round-trip harness lists the losses known on the
test fixture, and every one is also a potential block bug. Blocks from other
Marketplace modules will hit gaps no one has measured yet.

**Who benefits:** anyone placing blocks, above all from Marketplace design systems
(PlatformDesignSystem, company design systems), which use pluggable widgets and
values mxcli's MDL can't express.

## BSON Structure

Measured on `testdata/testapp` (Mendix 11.14.0), all 40 building blocks, read
directly with the repo's MPR reader (`mpr.Reader.ListRawUnitsByType`).

**Document.** `$Type: Forms$BuildingBlock` on 11.14, where the 11.x reader also tries
`Pages$BuildingBlock` first (see `show-describe-building-blocks.md`). Top-level keys:
`$ID, $Type, Name, DisplayName, Documentation, DocumentationUrl, Excluded,
ExportLevel, ImageData (preview bytes), CanvasWidth, CanvasHeight, Platform,
TemplateCategory, TemplateCategoryWeight, Widgets`. `Widgets` is the widget tree,
ordinary `Forms$*` and `CustomWidgets$CustomWidget` elements, the same shapes a page
stores.

**Pointers inside the unit** (binary values equal to another element's `$ID`), across
all 40 blocks:

| Owner.field | Count | Points to |
|---|---|---|
| `CustomWidgets$WidgetObject.TypePointer` | 28 | `CustomWidgets$WidgetObjectType` |
| `CustomWidgets$WidgetProperty.TypePointer` | 669 | `CustomWidgets$WidgetPropertyType` |
| `CustomWidgets$WidgetValue.TypePointer` | 669 | `CustomWidgets$WidgetValueType` |

That's all of them. Every pointer is inside a pluggable widget and aims from its
`Object` into its own `Type`; none crosses widgets. **No `GUID`s** occur in a block.

**References by name** (to other documents): `CustomWidgets$WidgetValue.Image` (11,
e.g. `Atlas_Web_Content.Content.user`), `Forms$CallNanoflowClientAction.Nanoflow` and
a `WidgetValueType.DefaultValue` (1 each, `FeedbackModule.ACT_Open_Feedback_Modal`).
They're module-qualified, so they resolve the same on any page of the project.

**References to the block's own widgets, by name:** e.g.
`Forms$ListenTargetSource.ListenTarget = 'gallery2'` in `Master_Detail`, plus `$name`
tokens inside expression strings. These are the references #560 had to rename
together with the `as prefix_` rename.

**Snapshot of the pluggable widget definition.** Every `CustomWidget` carries its own
`Type` (`WidgetObjectType`, `PropertyTypes`, enumeration values): the definition as of
the block's authoring. The round trip rebuilds pluggable widgets through the widget
engine from the *installed* `.mpk`, so today a placed widget silently gets the current
definition. A raw copy would carry the block's snapshot, which may be stale (CE0463
"the definition of this widget has changed"; see `.claude/skills/diagnose-ce0463.md`).

## Proposed MDL Syntax

None. `use building block` keeps its syntax and meaning. What changes is that the
widgets placed are the block's stored ones, with only what the statement adds applied
on top:
- the `as prefix_` rename;
- the `datasource:` override;
- the `action:` override, with the three tiers from #2171.

## Design

### Pipeline

1. **Read** the block's raw `Widgets`, as today: `Backend.GetRawUnit` in
   `mdl/executor/cmd_building_blocks.go`, which `expandBuildingBlockRef` uses.
2. **Clone with fresh identity.** Deep-copy the tree and mint a new `$ID` for every
   element. In the same pass, re-aim every `TypePointer` from its old target `$ID` to
   that target's new `$ID`.
   - An old→new map per unit is enough, because the measurement shows every pointer
     resolves within the tree.
   - This is CLAUDE.md's rule "never rewrite an element `$ID` without rewriting every
     reference to it in the same pass" (PR #125). The clone must refuse, not guess,
     when it meets a binary value equal to an old `$ID` in a field it doesn't know.
   - Pieces exist: `cloneWithFreshIDs` (`mdl/settingsoverlay`, IDs only) and
     `remapTypePointers` (`mdl/backend/pagemutator/pluggable_replace.go`, by path).
     The new routine combines them. A `GUID` met in the tree gets a fresh one (rule 3
     of the GUID section), though none was measured.
3. **Prefix rename** on the BSON:
   - every widget `Name`;
   - every field that names a widget of the block (`ListenTarget`, the association
     context variable, `delete $w` / open-link variable targets);
   - `$name` tokens in expression strings, reusing #560's token rewrite, which today
     runs on the AST.
   
   Names that don't belong to the block keep their spelling.
4. **Overrides.** Pick the target with the existing rules (`isDataSourceWidget`,
   `rebindBuildingBlockAction`'s tiers), using a read-only view of the tree. Write the
   override with the existing writers (`listViewSourceToGen` and friends for a data
   source, the action writer for an action), replacing that one field in the raw
   element.
5. **Refresh pluggable definitions** against the installed `.mpk`, as
   `mxcli fix widgets` does (`modelsdk/widgets.AugmentTemplate`). Whether to do this
   depends on how Studio Pro places a block (Open question 1).
6. **Emit** the tree into the page through a new raw widget node (`pages.RawWidget{Doc
   bson.D}`). `widgetToGen` decodes it with `decodeRawBSON` (the codec preserves
   unknown `$Type`s byte for byte, as it already does for a `CustomWidget`'s `Type` and
   `Object`), so it's written exactly as copied.

### The read-only view

Target selection (step 4) and `check`'s name expansion (#2182, MDL-ALTER02) need a
widget's type, name and children. A small view built directly from the BSON
(`$Type`, `Name`, children) answers that without the describe printer. It can't fail
on a value MDL can't express, so `check` and `exec` stay on one expansion.

### What goes away

- `FromBuildingBlock` stops being needed for #2183's refusal: no hand-written-MDL rule
  ever sees the block's widgets. It still drives the "list view without a data
  source" warning, which would read the raw source instead.
- The `<unbound>` grammar from #2181 stays, because `describe page` still needs it,
  but block placement no longer depends on it.

## Implementation Plan

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/executor/bb_raw_copy.go` (new) | Clone with fresh `$ID`s and remapped `TypePointer`s; prefix rename; refuse an unknown pointer |
| `mdl/executor/cmd_pages_builder_v3.go` | `expandBuildingBlockRef`: build the raw copy and the read-only view instead of describe → parse |
| `sdk/pages/` | `RawWidget{Doc bson.D}`: a widget written as stored |
| `mdl/backend/modelsdk/widget_write.go` | `widgetToGen`: a `*pages.RawWidget` case through `decodeRawBSON` |
| `mdl/executor/validate_*` (#2182's pass) | Names from the read-only view |
| `modelsdk/widgets/augment.go` | Reused for step 5, if Open question 1 says so |
| `mdl/executor/cmd_pages_builder_v3_widgets.go` | List-view warning reads the raw source |

### Order

1. The clone and its identity tests, on all 40 testapp blocks: every `TypePointer`
   resolves after the copy, no old `$ID` survives, and a planted unknown pointer is
   refused.
2. Emit raw, behind an internal switch, and compare against today's output on the
   Atlas blocks.
3. The prefix rename, then the overrides, then `check`.
4. Remove the round trip from block expansion once the comparison below passes.

## Version Compatibility

No new Mendix capability. The block's `$Type` differs by version (`Pages$BuildingBlock`
or `Forms$BuildingBlock`); the reader's fallback already handles it. A raw copy writes
the shapes the block was saved in. A block saved by an older Studio Pro than the
project's would carry older element shapes, which Studio Pro converts when it opens
the project; whether mxbuild accepts them unconverted is part of Open question 1.

## Test Plan

- **Unit (identity):** for every testapp block, the clone has no `$ID` in common with
  the source, every `TypePointer` points at an element of the clone, and the element
  count and `$Type` multiset are unchanged. A control with one `TypePointer` left
  un-remapped must fail the test.
- **Unit (rename):** Master_Detail `as md_` renames `gallery2` in `Name` and in
  `ListenTarget`, and a name outside the block keeps its spelling.
- **Unit (overrides):** each tier from #2171, and a data source override on Master_Detail.
- **Equivalence (the gate for removing the round trip):** for each of the 40 Atlas
  blocks, place it with today's path and with the copy, and compare:
  - the copy must lose nothing the round trip keeps;
  - each difference is either a known round-trip loss or a definition refresh,
    explained in the PR.
- **Studio Pro reference:** place the same blocks in Studio Pro and compare the stored
  page BSON, normalising `$ID`s. This is the measurement the CLAUDE.md working rules
  ask for.
- **mx check and a build:** the 40 Atlas blocks, plus the PDS blocks from
  #2180/#2181/#2183 where a PDS project is available, each placed plain, with a
  prefix, and with each override. The result must be 0 errors except the blocks'
  expected configure-me errors (CE0402 unbound parameters, CE0488 a list view without
  a source).
- **Regression:** the existing building-block tests (#560, #1822/#2171, #2181–#2183)
  keep passing.

## Open Questions

1. **Studio Pro reference: how does it place a block?** This needs a Studio Pro
   example, which this environment can't produce. Drag a few blocks onto a page
   (Atlas Card_WithImage, Master_Detail, and a PDS block with a pluggable widget),
   then compare the page's widgets with the block's. Specifically:
   - Does Studio Pro keep the block's `CustomWidget.Type` snapshot, or regenerate it
     from the installed `.mpk`? This decides step 5.
   - Does it rename widgets that collide with names already on the page, and how
     (`container1` → `container3`)? Today mxcli refuses a duplicate name, and MDL
     scripts rely on the predictable `prefix_` names.
   - Does it rebind anything to the page's context: entity or attribute references,
     or a data view's source?
2. **Keep the round trip as a fallback?** If the copy meets a pointer it doesn't know,
   should it fall back to today's path with a warning, or refuse? Refusing is safer
   for identity. Falling back keeps scripts working while the copy matures.
3. **Context rebinding.** None of the 40 Atlas blocks binds an attribute or entity, so
   the measurement doesn't show what a bound block needs. PDS blocks may. If Studio
   Pro rebinds on placement (Open question 1), the copy has to do the same, or report
   what the author must bind.
4. **`describe building block`** keeps the printer and is unaffected. Should it gain a
   warning when the printed MDL wouldn't round-trip, given that placement no longer
   depends on it?
