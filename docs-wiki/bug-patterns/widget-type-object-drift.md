---
title: Widget Type / Object Drift (CE0463)
category: bug-pattern
last-synced: 4e185f73
sources:
  - .claude/skills/fix-issue/findings/
  - .claude/skills/debug-bson.md
  - sdk/widgets/templates/README.md
  - sdk/mpr/writer_widgets.go
---

> **Do not duplicate**: the specific CE0463 fix recipes live in the `fix-issue/findings/*.jsonl` records, the diff workflow lives in `.claude/skills/debug-bson.md`, and the template-extraction procedure lives in `sdk/widgets/templates/README.md`. This page describes the pattern only.

## What this is

A family of pluggable-widget bugs (DataGrid2, ComboBox, Gallery, filter widgets) where the embedded `WidgetType` and `WidgetObject` drift out of structural sync. Studio Pro detects the mismatch and raises CE0463 "the definition of this widget has changed" — and on master-detail pages that cascades into CE3637 on the dependent DataView.

## How it fits

A pluggable widget stores two coupled structures: the `type` (the PropertyTypes schema — what properties exist) and the `object` (the WidgetObject — the actual values). Studio Pro enforces that they match exactly: every PropertyType needs a corresponding property, cross-reference `TypePointer`s must resolve, ordering must match, and no field may appear that is absent from the reflection schema. Any deviation marks the definition as "drifted."

The drift recurs because the two structures are written from different code paths, so it is easy for one to gain or lose a field the other does not. Concrete triggers include emitting a field outside the reflection schema (the `TimeFormat` regression in `Forms$FormattingInfo`, see [`sdk/mpr/writer_widgets.go`](../../sdk/mpr/writer_widgets.go)), property ordering mismatches, and `null` where a `Forms$ClientTemplate` is required. The tell-tale is CE0463 on a widget you just wrote via MDL.

A key trap: `mx check` is tolerant and passes anyway — only `mx diff` and Studio Pro are strict — so green checks do not mean the project opens. The diagnostic is the "Update widget" diff: let Studio Pro re-save the widget and diff its output against yours. The per-trigger recipes are in the symptom table; the diff methodology is in [`debug-bson.md`](../../.claude/skills/debug-bson.md).

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-instance CE0463/CE3637 fix recipes
- [[architecture/widget-engine]] — how `.def.json` definitions and templates produce widget BSON
- [[models/version-gating]] — why a field can be valid in one Mendix minor and drift in another
