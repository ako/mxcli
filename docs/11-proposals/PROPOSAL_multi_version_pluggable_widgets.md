# Proposal: Multi-Version Pluggable Widget Support

**Status:** Draft
**Date:** 2026-06-12

## Problem Statement

mxcli must create pluggable-widget instances (ComboBox, DataGrid2, Gallery, …)
that Studio Pro accepts **without `CE0463` "the definition of this widget has
changed"** — for whatever Mendix version *and* whatever installed widget version
a project is on.

### How widget creation actually works today (both engines)

Resolution is **three layers**, not a single frozen template:

1. **`def.json` / `WidgetRegistry`** — tells the engine *which MDL keyword routes
   into which widget property key* (+ object-list / child-slot structure). Built-ins
   (ComboBox/DataGrid/Gallery/filters) are hand-crafted in
   `sdk/widgets/definitions/*.def.json`; everything else is **extracted from the
   project's `.mpk`** by `mxcli widget init` → `.mxcli/widgets/<name>.def.json`
   (`RefreshWidgetDefinitions`, auto-refreshes on drift). Derived from the project.
2. **`GetTemplateFullBSON`** — loads the static `templates/mendix-11.6/*.json` as the
   Type+Object **base skeleton**.
3. **`augmentFromMPK`** (`AugmentTemplate`) — reconciles that skeleton's **property
   set against the installed `.mpk`**: adds keys in the `.mpk` but missing from the
   template, removes stale ones, emitting *both* a `PropertyType` (Type) and a
   `WidgetProperty` (Object) per added key.

**The schema is already version-reconciled.** Measured: a ComboBox created by the
legacy engine on a **10.24** project (installed ComboBox `2.4.3`, base template
`11.6`) has **56 `PropertyKey`s — identical** to a pristine Studio-Pro 2.4.3
ComboBox (0 stale, 0 missing), and produces **no `CE0463`**. So the long-standing
"frozen 11.6 templates cause `CE0463` on 10.x" belief (encoded in
`sdk/versions/*.yaml` and `WIDGET_BSON_VERSION_COMPATIBILITY.md`) is **not the
schema** — `augmentFromMPK` handles it.

### So what actually breaks?

Two real, narrow gaps remain:

- **Dirty Object defaults.** `augmentFromMPK` reconciles property *keys*; it does
  **not** clean a template's Object *default values*. The modelsdk engine's
  `CE0463` this cycle came from a `combobox.json` extracted from a *configured*
  Studio-Pro instance (a `System.Language` association ComboBox), so its neutral
  defaults carried `optionsSourceType:"association"` + a baked-in datasource. The
  builder applied `attribute: Country` on top without resetting them → an Object
  inconsistent with the schema → `CE0463`. The fix that worked (commit `827bffd4b`)
  was simply swapping in the legacy template's **clean/neutral** Object defaults.
- **No cross-version guarantee.** Nothing tests that creation stays `CE0463`-free
  as Mendix minors and widget versions move; regressions surface in the field.

This proposal is **internal architecture only** — no new MDL syntax. It hardens the
existing three-layer mechanism rather than replacing it.

## BSON Investigation: the CE0463 tolerance spike

We measured **what `CE0463` actually checks**, because the prevailing assumption
(in `WIDGET_BSON_VERSION_COMPATIBILITY.md`) was wrong.

**Method.** Decode a real widget unit (`pymongo`), mutate one dimension, re-encode,
`mx check`. Every test has a no-op round-trip **control** proving the decode→encode
is byte-faithful. mxbuild **10.24.19** (`test-1024`, loads clean) + **11.10.0**
(`test6-app`). Confirmed across **ComboBox, DataGrid2, Gallery**.

| Mutation | Layer | Result |
|---|---|---|
| Add `AllowUpload` to all `WidgetValueType` (10.24, 504×) | envelope field | **tolerated** — 0 errors |
| Remove `AllowUpload` (11.10, 522×) | envelope field | **tolerated** — no new CE0463 |
| Reverse `WidgetObject.Properties` order (all 3 widgets) | Object ordering | **tolerated** — 0 CE0463 |
| Rename one `WidgetPropertyType.PropertyKey` (all 3) | **schema** | **CE0463** (18 DataGrid2, 15 Gallery) |
| Object carries instance-specific values inconsistent with schema | **Object↔schema** | **CE0463** (the modelsdk dirty-defaults case) |

**Conclusion.** `CE0463` is triggered by **schema mismatch** (embedded
`CustomWidgetType` `PropertyKey` set ≠ installed widget) and by **Object↔schema
inconsistency**. It is **NOT** triggered by envelope field presence/absence
(`AllowUpload`) or `Properties` ordering — Studio Pro tolerates those. The doc's
11.9 `AllowUpload`/ordering "envelope fragility" fixes worked because they were
*holistic template re-extractions*, not because those fields matter.

### Two independent version axes

| Axis | Source | Drives | Handled by |
|---|---|---|---|
| **Widget version** (ComboBox `2.4.3`@10.24 vs `2.5.0`@11.10) | installed `.mpk` | **schema** (`PropertyKeys`) — *what CE0463 checks* | `augmentFromMPK` reconciles it ✓ (proven 56=56) |
| **Mendix version** | runtime infra | **envelope** (`WidgetValueType` field set, ordering) | tolerated — no action needed |

They move independently (update Mendix without widgets, or a widget `.mpk` without
the project's existing instances → Studio Pro shows `CE0463` on those stale
instances = the normal "Update widget" prompt). The authoritative schema for a
*newly created* instance is the **currently installed `.mpk`** — which augment
already uses.

## Phase-1 spike result: neutral Object CANNOT be synthesized (decisive)

The first-cut plan was to **synthesize a neutral Object from the (augmented)
Type** — correct-by-construction, no stored Object to keep clean. **This was built
and disproven.**

`SynthesizeNeutralObject` emitted one neutral `WidgetProperty` per non-System
`PropertyType`, valued from the `ValueType.DefaultValue`. A/B `mx check` on test6
(11.10), modelsdk engine: the clean template produced **0 `CE0463`**; the
synthesized Object produced **1 `CE0463` on the ComboBox**. Root cause:

> **Studio Pro's fresh-widget instantiation defaults are not in the Type schema or
> the `.mpk`.** ComboBox `optionsSourceType`: schema/`.mpk` `DefaultValue` =
> `"association"`, but a *freshly dropped* widget instantiates `"enumeration"`
> (likewise `readOnlyStyle` `text`→`bordered`, `optionsSourceDatabaseItemSelection`
> `""`→`Single`). The widget's React `getDefaultProperties` overrides the declared
> defaults; only a **real extraction** of a fresh widget captures them.

`GenerateFromMPK` walks the **same** `createDefaultWidgetValue(.mpk default)` path,
so it has the identical flaw and would also `CE0463`. **Consequence: you cannot
derive a correct Object purely from schema/`.mpk`. A clean static template
(extracted from a fresh Studio-Pro widget) is *necessary*, not merely convenient.**

## Proposed Direction

The existing `def.json`-routing + static-base + `augmentFromMPK` pipeline is the
right architecture: it already delivers version-correct **schemas** (proven 56=56),
and the static Object carries the fresh-widget defaults that nothing else can. The
modelsdk `CE0463` was simply a **dirty template** (extracted from a *configured*
instance). So the work is **hardening + discipline**, not synthesis:

1. **Clean embedded templates are required, and must stay clean.** Every embedded
   template's Object must be a *fresh, unconfigured* widget extraction (no populated
   `AttributeRef`/`DataSource`/`EntityRef`). The ComboBox band-aid (`827bffd4b`) did
   this; audit the rest of the embedded set the same way.
2. **Add a dirty-template guard.** A check/lint that fails if any embedded (or
   `widget init`-generated) template's Object contains instance bindings — catches
   the modelsdk-class bug at build time instead of as a field `CE0463`.
3. **Ensure both engines augment.** legacy and modelsdk both call `augmentFromMPK`;
   consolidate the two `*/widgets` loaders so schema reconciliation can't diverge.
4. **Do *not* build a per-Mendix-version envelope model.** The spike shows the
   envelope is tolerated; the current `augment.go` superset (incl. `AllowUpload`)
   is sufficient.
5. **Cross-version validation matrix.** Per installed mxbuild (10.24 / 11.9 / 11.10
   / 11.11): create one of each pluggable widget on a fresh project, assert
   `mx check` reports **0 `CE0463`**. Converts version drift into a failing test.

### Why the alternatives are rejected

| Alternative | Why rejected |
|---|---|
| Synthesize neutral Object from Type | **Disproven** (Phase-1 spike): fresh-widget defaults aren't in the schema → `CE0463`. |
| `GenerateFromMPK` (.mpk → Type+Object) | Same `createDefaultWidgetValue` path → same wrong fresh-defaults → `CE0463`. Demoted to last-resort (used only when no embedded template *and* no instance exist). |
| Extract-from-instance (engalar's `extract.go`) | Lifts a real instance's Object → dirty-defaults `CE0463` (reproduced); inherits stale schema if instances lag the `.mpk`. Only safe if extracting from a *fresh* instance, which projects rarely have. |
| Per-Mendix-version envelope model | Spike shows the envelope is tolerated — effort on a non-problem. |

## Implementation Plan

Behind the existing `backend.WidgetObjectBuilder` interface — no executor or MDL
changes. Applies to both engines.

| File | Change |
|------|--------|
| `sdk/widgets/templates/`, `modelsdk/widgets/templates/` | Audit every embedded template's Object for instance bindings; re-extract any dirty one from a fresh Studio-Pro widget (as done for ComboBox in `827bffd4b`) |
| `modelsdk/widgets/` (+ `sdk/widgets`) | New dirty-template guard: a test/`widget init` check that rejects Objects with populated `AttributeRef`/`DataSource`/`EntityRef` |
| `modelsdk/widgets/loader.go`, `sdk/widgets/loader.go` | Consolidate so both engines share one augment path |
| `sdk/widgets/augment.go` / `modelsdk/widgets/augment.go` | Reconcile to a single implementation |
| `mdl/backend/widgetobj/builder.go` | Ensure the builder fully overrides every Object slot it sets (no bleed-through) |
| `modelsdk/widgets/multiversion_test.go` (new) | Cross-version `mx check` matrix |
| `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md`, `sdk/versions/*.yaml` | Correct the "envelope fragile / frozen schema" framing |

### Phasing

1. **Audit embedded templates for dirty Objects** + add the guard. ComboBox is
   already clean (`827bffd4b`); confirm DataGrid2/Gallery/the rest.
2. Consolidate the two engines' widget loaders / augment onto one path.
3. Cross-version validation matrix.
4. (Optional) Evaluate whether a fresh-instance extraction could *generate* clean
   templates on demand for widgets with no embedded template — the only way to
   avoid hand-extraction for the long tail of marketplace widgets.

## Version Compatibility

Not a version-gated feature — it is the mechanism that keeps pluggable widgets
correct across versions, and its deliverable is the validation matrix above. The
schema axis is already handled by `augmentFromMPK`; this proposal protects the
Object axis and proves the whole thing per Mendix minor.

Non-pluggable (`Forms$`) widgets need **no work** — already version-resilient via
the codec's declarative `VersionInfos` metadata (reflection-data); multi-version =
regenerate gen from the target version's reflection-data.

## Test Plan

- **Tolerance regression**: unit tests asserting `PropertyKey` rename → `CE0463`
  vs envelope field/ordering → tolerated (lock in the spike; catch any future
  dependence on envelope exactness).
- **Dirty-template guard**: assert no embedded template's Object contains populated
  `AttributeRef`/`DataSource`/`EntityRef` (catches the modelsdk-class bug).
- **Per-version matrix**: create ComboBox/DataGrid2/Gallery on fresh 10.24 / 11.9 /
  11.10 / 11.11 projects, `mx check`, assert 0 `CE0463`.
- **Augment fidelity** (lock in the measured result): augmented Type `PropertyKey`
  set == pristine Studio-Pro Type for the same widget+version (56=56 for ComboBox
  2.4.3 today).

## Open Questions

1. **Per-widget-version templates for object-list widgets.** The cross-version
   matrix (`scripts/widget-version-matrix.sh`) answered part of this concretely:
   ComboBox and DataGrid2 reconcile across versions via augment (flat PropertyKeys),
   but **Gallery `3.0.1`@10.24 produces CE0463 on both engines** — the 11.6-extracted
   template's *nested object-list* sub-schema (items/content) doesn't match the
   10.24-installed widget, and augment's flat-key add/remove doesn't reach it. So
   object-list widgets likely need a clean template **per widget-version**, not one
   shared 11.6 base. Open: root-cause the Gallery nested diff, then decide
   per-version template vs deeper (nested) augment.
2. **Long-tail marketplace widgets.** Built-ins can be hand-extracted clean. For
   arbitrary marketplace widgets with no embedded template, the only correct Object
   source is a fresh extraction — can `widget init` extract from a freshly-dropped
   instance, or must the user drop one first? (`GenerateFromMPK` is ruled out as the
   primary source by the Phase-1 result.)
3. **Generalization.** Confirm the dirty-vs-clean / fresh-default behaviour on
   nested-CustomWidget widgets (charts, Maps) and on 12.x when available.
4. **Doc + version-YAML correction**, and an **ADR** for "a clean static template
   (fresh Studio-Pro extraction) is required for the Object; `augmentFromMPK`
   reconciles the schema" once Phase 1 lands.

## Relationship to existing artifacts

- Memory: `reference_ce0463_tolerance_spike.md` (tolerance + augment-boundary
  evidence), `reference_modelsdk_pluggable_widgets.md` (engine/registry history).
- Corrects `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` (on the
  trigger) and the `pluggable_widgets` notes in `sdk/versions/*.yaml`.
- No overlap with the property-*editing* widget proposals
  (`PROPOSAL_update_builtin_widget_properties`, `PROPOSAL_widget_property_visibility`,
  `PROPOSAL_v0_12_0_widget_consolidation`).
