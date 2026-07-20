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
  **A second confirmed instance (commit `549c44f`): `image.json` carried a baked-in
  static image `WidgetValue.Image = "Atlas_Core.Content.Mendix"` (Atlas's Mendix logo,
  captured from a configured instance) where a fresh Image widget has `""`.** Any
  Image authored on 11.7+ tripped `CE0463`; the documented remedy (`mx update-widgets`)
  then destroyed MPRv2 storage (#763) — so a dirty default was silently funnelling
  users into data loss. Isolated by the reusable method below and fixed by clearing
  the value; verified `mx check --no-update-widgets` = 0 errors. The dirty-default
  class is therefore broader than datasources: it includes **image-asset refs, page
  refs, and configured client actions**, not just entity/attribute/flow bindings.

  **Reusable diagnosis method (this whole `CE0463` class).** Dump the widget BSON,
  `mx update-widgets` on a **copy**, dump again, and diff the
  `CustomWidgets$CustomWidget` subtree **order-independently** (canonicalise key order
  + mask `$ID`/`TypePointer` blobs). Confirms the spike's tolerance result *per case*:
  reordering and generic instance chrome (`LabelTemplate`, `Appearance.DesignProperties`)
  are cosmetic — a *passing* widget gets those too; the real cause is whatever
  value/structure survives that normalisation. For Image it was exactly one field.
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
| `sdk/widgets/templates/`, `modelsdk/widgets/templates/` | Audit every embedded template's Object for instance bindings; re-extract any dirty one from a fresh Studio-Pro widget (ComboBox `827bffd4b`; **Image `549c44f` — done**) |
| `modelsdk/widgets/dirty_template_test.go` | **Built + broadened (this cycle).** Dirty-template guard (`dirtyBindings` + `TestEmbeddedTemplates_NoDirtyBindings`) rejects Objects with concrete entity/attribute/flow refs **and now image-asset / page / configured-action bindings** — the class the original guard missed, which is how the Image bug shipped. Scans **both** engines' template sets; proven non-vacuous (detect-dirt + clean-is-clean cases) and proven to catch the real Image regression. Follow-up: promote `dirtyBindings` from test-only to a runtime `widget init` check |
| `modelsdk/widgets/loader.go`, `sdk/widgets/loader.go` | Consolidate so both engines share one augment path |
| `sdk/widgets/augment.go` / `modelsdk/widgets/augment.go` | Reconcile to a single implementation |
| `mdl/backend/widgetobj/builder.go` | Ensure the builder fully overrides every Object slot it sets (no bleed-through) |
| `modelsdk/widgets/multiversion_test.go` (new) | Cross-version `mx check` matrix |
| `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md`, `sdk/versions/*.yaml` | Correct the "envelope fragile / frozen schema" framing |

### Phasing

1. **Audit embedded templates for dirty Objects** + add the guard. **In progress:**
   ComboBox clean (`827bffd4b`); Image cleaned (`549c44f`); the guard is built and
   broadened to the image/page/action class over both engines. Remaining: a broader
   any-node scan surfaces a **configured delete `Forms$ActionButton`**
   (`Forms$DeleteClientAction` + `Atlas_Core.Atlas.trash-can`) baked into
   `datagrid.json` — nested-Forms-widget dirt, out of the WidgetValue-scoped guard's
   safe reach (see Open Question 1). Confirm/clean it as part of the DataGrid2 pass.
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

1. **Within-key PropertyType drift on object-list widgets — root-caused.** Verified
   directly against cached mxbuild (10.24.19 / 11.10.0 / 11.12.1):
   - **DataGrid2 is clean with the *bundled* Data Widgets** (3.0.0@10.24, 3.4.0@11.10/
     11.12) across attribute columns (the exact `#600` repro), custom content, filters,
     and selection — all **0 CE0463**. The old-v0.11.0 mxcli-side bugs (NestedKeyOrder
     `f12aba2`, custom-content `58508d4`) are resolved on HEAD. **But DataGrid2 drifts
     exactly like Gallery when the installed Data Widgets is *updated from the
     marketplace* past what the 11.6 template reconciles with.** `#600`'s latest report
     (3 days ago: mxcli **v0.16.0**, Mendix **11.12.0**, "still present") is this case —
     the original reporter noted **Data Widgets 3.10.0**, newer than the 3.4.0 bundled
     with 11.12.0. Confirmed material facts: mxcli's widget code is **identical
     v0.16.0→HEAD** (the only deltas are the Image fix + guard), so **no mxcli upgrade
     fixes it** — it is a widget-version drift needing the deeper-augment fix below.
     (An earlier note that DataGrid2 custom-content "still reproduces on 11.12.1" was a
     **test-syntax artifact** — `Content: showContentAs` injected a spurious
     TextTemplate — not a real defect; the real driver is the updated `.mpk`.)
   - **Gallery custom content produces CE0463 on 10.24** (clean on 11.10 / 11.12).
     The before/after-`update-widgets` subtree diff shows the cause is **NOT** a dirty
     default and **NOT** merely nested: the 11.6 template and the 10.24-installed
     Gallery share the same PropertyType *keys* but differ in their *definitions* —
     `pagingPosition` enum values changed (`bottom`/`top` → `below`/`above`, default
     `bottom`→`below`), a `Category` was renamed (`General::Pagination` →
     `General::Items`), and a property's `Type`/order shifted. `augmentFromMPK`
     reconciles key *presence* (add/remove) but **not within-key definition changes**,
     so the emitted Type ≠ the installed widget → CE0463.

   **This is the general object-list drift, not a Gallery quirk** — DataGrid2 with a
   marketplace-updated Data Widgets (3.10.0, the #600 case) drifts the same way.
   Root-caused via the key-indexed PropertyType diff into **two independent axes:**

   - **Axis 1 — within-key PropertyType metadata drift. FIXED (`8b65f06`).**
     `augment` reconciled key presence and enum option sets but left the rest of a
     matched PropertyType stale: a `DefaultValue` outside the reconciled enum set
     (Gallery `pagingPosition` options → `{below,above}` but default stayed `bottom`),
     a `Category` rename (`General::Pagination`→`General::Items`), a `Caption` change.
     `reconcilePropertyMetadata` now overwrites Category/Caption/DefaultValue from the
     `.mpk` for every matched key. Verified: after it, **every Gallery@10.24
     PropertyType matches `update-widgets`**; no regression on 11.12. (An earlier draft
     claimed this was "the likely complete fix for #600" — **disproven**; see the
     large-drift measurement below, which shows the #600 DataGrid2 case needs more.)
     *(modelsdk engine; the legacy sdk `augment` lacks even `reconcileEnumValues` and is
     behind — consolidate per item 3 above.)*
   - **Axis 2 — datasource structure + PropertyType order. FIXED (`10b5bcf`).**
     Gallery@10.24 *additionally* drifted two ways: (a) mxcli emitted `Forms$GridSortBar`
     without `SortItems` and `CustomWidgets$CustomWidgetXPathSource` without
     `SourceVariable` (the 10.24 widget expects both, empty/null) — fixed with codec
     `TypeDefaults` in `widget_write.go` (`MandatoryListMarkers: SortItems=2`,
     `NullFields: SourceVariable`); and (b) the top-level PropertyType **order** differed
     — the 3.x widget moved `pagingPosition` ahead of `showTotalCount`, and augment kept
     the template order. **The WidgetType's PropertyType order IS checked by CE0463**
     (unlike the WidgetObject's Properties order, which the spike proved tolerated);
     `reorderPropertyTypes` now sorts the Type's PropertyTypes to the `.mpk` declaration
     order (refs are by `$ID`, so it's safe).

   **Result (moderate drift): Gallery@10.24 passes `mx check` with 0 errors and NO
   `update-widgets`.** Regression clean — DataGrid2 (attribute + filter + custom-content)
   and Gallery both 0 errors on 10.24 / 11.10 / 11.12.0 / 11.12.1 with the *bundled*
   Data Widgets. *(modelsdk engine; the legacy sdk `augment` remains behind — item 3.)*

   **The three axes are necessary but NOT sufficient for large version jumps — measured
   against the #600 reporter's exact stack.** Downloaded Data Widgets **3.10.0** from the
   marketplace, swapped it into a Mendix 11.12.0 project (over the bundled 3.4.0), and
   authored a DataGrid2 with the fixed binary: **still CE0463.** Isolating that one grid
   from the Atlas template pages (which legitimately drift and are a separate "update
   widgets" concern), the installed 3.10.0 schema differs from the 11.6-era template in
   **far more than the three axes reconcile** — an aggregated field diff over the
   `CustomWidgetType`:

   | Differing `ValueType` field | # properties |
   |---|---|
   | `AllowUpload` (null vs false) | 79 |
   | `Type` (property type changed) | 15 |
   | `Translations` | 14 |
   | `DefaultValue` | 13 |
   | `Required` | 7 |
   | `EnumerationValues` | 7 |
   | `AllowedTypes` | 4 |
   | + `Category`, `Description`, `ReturnType`, nested `ObjectType.PropertyTypes` | |

   So **augment's key-presence + metadata/enum/order reconciliation scales to a *moderate*
   drift (Gallery@10.24: Category + one DefaultValue + order + datasource) but not to a
   *large* one (DataGrid2 11.6-era → 3.10.0).** This directly disproves the earlier
   working assumption that Axis 1 alone would close #600. The Phase-1 measurement that
   "augment already delivers version-correct schemas (56=56 for ComboBox 2.4.3)" held only
   because that was a *small* version delta; a big jump exposes per-field schema evolution
   (`Type`, `Required`, `AllowedTypes`, `Translations`) that key-level augment never touches.

   **RESOLVED — path (a) is structurally impossible; only mxbuild can produce the
   envelope.** Full end-to-end reproduction of the #600 stack (fresh 11.12.0 project, DW
   3.10.0 swapped in over the bundled 3.4.0, DataGrid2 `dg` authored with the fixed binary,
   isolated from the Atlas template pages) with an order-independent, `$ID`/`TypePointer`-masked
   before/after-`update-widgets` subtree diff of `dg` alone shows the residual drift is
   dominated by fields that **do not exist in the widget XML source at all**:

   | Residual `dg` drift after augment | count | in `Datagrid.xml`? |
   |---|---|---|
   | `AllowUpload` (absent → `false`) | 105 | **no** — not in source |
   | `Required` (`false` → `true`) | 54 | partially — XML has **3** `required="true"`, mxbuild emits **54** |
   | `DesignProperties`, `LabelTemplate` | several | **no** — not in source |
   | `ReturnType` (`null` → `{Type,IsList,…}`), `Type`, `DefaultValue` | ~30 | computed/partial |

   The nested-column **PropertyKey sets are already identical** before/after (augment adds
   every new 3.10.0 key — `exportType`, `exportNumberFormat`, `exportDateFormat`, … —
   correctly). What augment *cannot* produce is the mxbuild-**computed** BSON envelope:
   `AllowUpload`/`DesignProperties`/`LabelTemplate` appear **nowhere** in `Datagrid.xml`, and
   `Required` is computed (3 declared vs 54 emitted). Since these fields are not derivable
   from the `.mpk`, **no parser/template/augment approach — including path (a), full-`ValueType`
   reconciliation — can ever reproduce a version-faithful `WidgetType`.** (A speculative
   `reconcileValueTypeSchema` was implemented and reverted: setting `Required` from the XML
   `required` attr would mis-set the 51 mxbuild-computed keys to `false`.) `update-widgets`
   on the same copy takes `dg` from CE0463 → **0** — confirming only mxbuild owns the envelope.

   **Revised path forward — delegate the envelope to mxbuild, made v2-safe (was path (d)):**
   - **(c) v2-safe `update-widgets` as the CE0463 remediation.** The augment layer already
     closes *moderate* drift (Gallery@10.24, bundled DW) and that stays. For *large* jumps,
     the correct fix is to run mxbuild's own `update-widgets` — which is exactly what the
     skills now route users to (`mxcli docker check`/`build`, post the earlier skill-guidance
     fix). The remaining blocker is that bare `mx update-widgets` destroys MPRv2 (deletes
     `mprcontents/`, converts to v1 — issue #763), which **mendixlabs PR #764** fixes.
     Adopt/port #764 so `mxcli docker check`/`build` (and the warm local loop) can reconcile
     widgets without downgrading the project. This is the only path that is faithful for
     *arbitrary* widget versions, because only mxbuild computes the envelope.
   - **(b) Per-version templates** — extract a clean `datagrid-<ver>.json` per Data Widgets
     release. Rejected as a primary fix: doesn't scale (3.11.2 already shipped) and still
     wouldn't carry the computed `Required`; kept only as a note.

   Remaining: implement (c) (v2-safe `update-widgets` via #764), keep the moderate-drift
   augment as-is, and fold the matrix into `scripts/widget-version-matrix.sh` as a standing
   gate (feasible now — `mxcli marketplace download` fetches any widget version for the
   fixture) asserting *bundled*-DW authoring stays at 0 CE0463.

   **Also noted (dirty-template audit):** `datagrid.json`'s Object carries a configured
   delete `Forms$ActionButton` (`Forms$DeleteClientAction` + `Atlas_Core.Atlas.trash-can`)
   — a separate nested-Forms dirty default the flat WidgetValue-scoped guard deliberately
   does not flag; benign on the tested versions but worth cleaning during the object-list pass.
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
