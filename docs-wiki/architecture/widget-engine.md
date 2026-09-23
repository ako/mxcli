---
title: Widget Engine
category: architecture
last-synced: 4e185f73
sources:
  - sdk/widgets/definitions/loader.go
  - sdk/widgets/definitions/combobox.def.json
  - sdk/widgets/templates/README.md
  - mdl/backend/modelsdk/widget_write.go
  - mdl/executor/cmd_pages_builder_v3_widgets.go
  - docs/03-development/PAGE_BSON_SERIALIZATION.md
  - docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md
---

> **Do not duplicate**: widget property reference (see `sdk/widgets/templates/README.md` and source), CE0463 fix recipe (see `.claude/skills/fix-issue.md` and `.claude/skills/debug-bson.md`), or per-widget BSON schemas (read source).

## What this is

The machinery that turns a one-line MDL widget statement (e.g. `COMBOBOX myCombo (...)`) into the BSON a Mendix pluggable widget requires. Built-in page widgets are serialized directly by [`widget_write.go`](../../mdl/backend/modelsdk/widget_write.go); pluggable widgets (ComboBox, DataGrid2, Gallery, filters) are far harder, because their BSON is a self-referential `type`/`object` pair that Studio Pro validates strictly. The widget engine exists to assemble that pair declaratively instead of by hand.

## How it fits

A pluggable widget's BSON has two halves that must agree: a `type` (the PropertyTypes schema) and an `object` (default values), cross-linked by `TypePointer` references. If the object's structure drifts from the type's, Studio Pro raises CE0463 "widget definition has changed" — see [[bug-patterns/widget-type-object-drift]]. The engine guarantees agreement by starting from a complete template extracted from a real Studio Pro project, never from a programmatically guessed structure. The [template README](../../sdk/widgets/templates/README.md) documents this dual requirement and the load-time ID-remapping pipeline that rewrites placeholder `$ID`s to fresh UUIDs across both halves at once.

Two declarative inputs drive it. [`.def.json` definitions](../../sdk/widgets/definitions/) (e.g. [combobox.def.json](../../sdk/widgets/definitions/combobox.def.json)) describe how MDL properties *map* onto template property keys — including conditional "modes" like association vs. enumeration. A [3-tier registry](../../sdk/widgets/templates/README.md) resolves project, user, then [embedded](../../sdk/widgets/definitions/loader.go) definitions. The MDL [page builder](../../mdl/executor/cmd_pages_builder_v3_widgets.go) consumes these to populate widget BSON during execution. Hardcoded BSON builders were rejected because every Mendix minor and every widget version shifts the envelope; a declarative definition plus `.mpk` augmentation absorbs that drift in one place.

That drift splits into two layers with different resilience, detailed in [WIDGET_BSON_VERSION_COMPATIBILITY.md](../../docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md): the widget-specific structure (version-resilient, synced from each project's `.mpk`) and the Mendix BSON envelope (brittle, tied to the extracted template version and patched by hand per minor). Knowing which layer a CE0463 belongs to is the first triage step. See [[models/version-gating]].

## See also

- [sdk/widgets/templates/README.md](../../sdk/widgets/templates/README.md) — dual `type`/`object` requirement, ID remapping, extraction process
- [docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md](../../docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md) — what's version-safe vs. version-fragile
- [docs/03-development/PAGE_BSON_SERIALIZATION.md](../../docs/03-development/PAGE_BSON_SERIALIZATION.md) — built-in widget BSON tables
- [[bug-patterns/widget-type-object-drift]] — the CE0463 failure mode
- [[architecture/mpr-read-write]] — the BSON layer underneath
- [[models/version-gating]] — how version-specific behavior is gated
