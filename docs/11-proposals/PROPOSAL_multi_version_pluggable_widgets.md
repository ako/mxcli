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

   **RESOLVED (definition side) — the `WidgetType` IS generically reproducible; an
   earlier "only mxbuild can produce the envelope" conclusion was WRONG.** `update-widgets`
   is fully generic (no per-widget knowledge), so everything it emits is derivable from the
   package + the generic metamodel/spec defaults — the "not in `Datagrid.xml`" fields are
   generic defaults, not widget-specific computation:
   - `AllowUpload` (on **all 105** ValueTypes) is a generic `WidgetValueType` metamodel
     default (`false`) — emitted for every ValueType, widget-agnostic.
   - `Required` "computed 3→54" is just the **pluggable-widget spec default**: `required`
     defaults to **true** when the attribute is absent. The `.mpk` parser had the wrong
     default (missing→false); fixed.
   - `Type`/`DefaultValue` drift was augment cloning new keys from **wrong-typed exemplars**
     (our bug), plus stale template values — fixed by reconciling each matched ValueType's
     `Type`/`Required`/`DefaultValue`/`AllowedTypes`/`IsList`/`ReturnType`/`Translations`
     from the `.mpk` and normalizing the mutually-exclusive type-specific fields.
   - `Translations`/`ReturnType`/nested `Category`/nested order — all parsed from the
     widget XML (`<translations>`, `<returnType>`, propertyGroup captions) and emitted.

   **Committed (`c7fe714`): the emitted `CustomWidgetType` is byte-identical (canonical,
   `$ID`/`TypePointer`-masked) to `update-widgets` output for the #600 stack (DW 3.10.0 /
   Mendix 11.12.0) — definition diff 326 → 0 — with no regression on bundled DW across
   11.12.0 / 11.10.0 / 10.24.** The generic-reconciliation thesis holds for the definition.

   **NOT resolved (instance side) — the last-mile Object default-template instantiation is
   config-conditional applicability that lives in the widget's editor code, not the
   declarative package.** With the definition matching, the residual CE0463 is entirely in
   the `WidgetObject`: a handful of `textTemplate` properties whose default template
   (`Forms$ClientTemplate` with the shipped caption translations) mxbuild instantiates in
   the instance. Even a **minimal** DataGrid2 (Selection:None, one column) reproduces it —
   definition identical, only ~9 `textTemplate` default-templates differ, `update-widgets` →
   0. **Which textTemplates get a default template is config-dependent**: aria/status labels
   are always instantiated, but `clearSelectionButtonLabel` / `loadMoreButtonCaption` /
   `singleSelectionColumnLabel` stay `null` because their feature is off. Empirically: an
   "always-populate" rule closes 9→3 but over-populates the 3 feature-gated ones (still
   CE0463); a `Required`-gate closes 3→9 the other way. Neither declarative rule matches,
   because the rule isn't declarative.

   **CONFIRMED mechanism — the widget's compiled `editorConfig.js` decides applicability.**
   The `.mpk` ships `Datagrid.editorConfig.js` (16 KB) alongside `Datagrid.xml`. Its
   `getProperties(values, defaultProperties)` function calls `hidePropertyIn` (24×) /
   `hidePropertiesIn` (6×) / `changePropertyIn` conditionally on the *instance's* current
   values. The exact hides for the three null properties are present verbatim:

   ```js
   "Multi"    !== r            && hidePropertiesIn(e, t, ["selectionCounterPosition","clearSelectionButtonLabel","enableSelectAll"])
   "loadMore" !== e.pagination && hidePropertyIn(t, e, "loadMoreButtonCaption")
                                  hidePropertyIn(e, t, "singleSelectionColumnLabel")   // conditional
   ```

   This maps 1:1 onto the measurements: our minimal grid has `Selection:None` (≠ "Multi") →
   `clearSelectionButtonLabel` hidden → `null`; `Pagination:buttons` (≠ "loadMore") →
   `loadMoreButtonCaption` hidden → `null`. The always-populated properties
   (`selectRowLabel`, `cancelExportLabel`) appear **0×** in `editorConfig.js` — never hidden,
   so their default is instantiated. **A hidden property does not get its default template
   instantiated in the Object.** The applicability graph is imperative JS keyed on the
   instance config, not declarative XML — which is why no `.mpk` parsing reproduces it, and
   why the null-vs-populated property *definitions* in `Datagrid.xml` are byte-identical.

   **Object-from-definition rebuild — tried, REJECTED (regresses the common case).**
   Regenerating the whole `WidgetObject` from the reconciled definition (one WidgetValue per
   PropertyType, defaults built from each ValueType) makes the count match (127=127) and is
   architecturally clean, but it **discards the byte-exact extracted template Object** — and
   because it can't replicate the editor-applicability rule, it **regressed bundled DataGrid2
   from 0 → 1 CE0463 on both 11.12.0 and 10.24**. The extracted template Object is the best
   available source for the no-drift case; a rebuild trades that away for an approximation.
   Reverted.

   **RESOLVED (instance side) — a static editorConfig extractor closes it, natively in
   mxcli. No mxbuild / update-widgets dependency needed for DataGrid2.** The codebase
   already had the scaffold — `WidgetVisibilityRule` + `Builder.ApplyPropertyVisibility`
   (nulls a hidden textTemplate's ClientTemplate), fed by a *hand-transcribed*
   `widgetVisibilityRules` table (VideoPlayer/Timeline only; the comment: "Until the JS
   extractor lands (#574 Phase 2)"). The three landed commits automate that table and wire
   it through:

   - **Extractor** (`4b8c4f5`, `mdl/executor/editorconfig_extract.go`) — a static analyzer
     lifts the dominant `getProperties` idioms from the compiled `editorConfig.js`
     (`"V"===/!==ref && hide`, `ref && hide`, `ref || hide`, `ref ? hide : …`) into
     `WidgetVisibilityRule`s, with **scoped** alias resolution (`var r=e.itemSelection`) so
     minified single-letter identifiers don't leak across functions, and a boundary check
     that **skips compound/ternary-nested and object-list-nested guards** rather than emit a
     partial (wrong) rule — degrading safely to "not hidden". Coverage on the real DW 3.10.0
     `Datagrid.editorConfig.js`: **9/28 hide-calls lifted** (12 object-list-nested, 7
     compound — safely skipped, honestly counted), including the three that drive #600.
   - **Wiring** — built-in widgets (DataGrid2/Gallery), which the `.def.json` generator
     skips, resolve rules on the fly from the project's installed `.mpk` editorConfig at
     build time (`resolveWidgetVisibilityRules`); `ApplyPropertyVisibility` then nulls the
     hidden textTemplates. Object-side textTemplate defaults are populated with the `.mpk`'s
     shipped caption translations (fixes CE4899 required-textTemplate) and the visibility
     pass nulls the hidden ones afterward.
   - **Selection-value fix** (`666a65b`) — conditions keyed on a Selection-typed property
     (`itemSelection` = None/Single/Multi) must read the WidgetValue's `Selection` field, not
     `PrimitiveValue`; reading the wrong field mis-fired under `Selection:Single`.

   **Result: DataGrid2 on Mendix 11.12.0 + Data Widgets 3.10.0 (the exact #600 stack) →
   0 errors, minimal AND full (Selection:Single + column textfilters).** No regression on
   bundled DW across 11.12.0 / 11.10.0 / 10.24.

   **Phase 2 landed** (`458c52a`): extraction moved into `.def.json` **generation**
   (generated defs now carry `propertyVisibility`, superseding the hand table), and the rules
   are wired into **`check`** as **MDL-WIDGET10** — a config-aware warning when the user sets
   a property the widget hides under the current config (e.g. `ClearSelectionButtonLabel`
   with `Selection:None`). Conservative: only warns when the property is explicitly set and
   the condition value is determinable.

   **Phase 3 landed** (`da30bab`) — all 9 Data Widgets, plus the filter build path.
   The DW 3.10.0 package ships **9 widgets**, all with an `editorConfig.js`: DataGrid2,
   Gallery, DropdownSort, SelectionHelper, TreeNode (the outline tree), and the 4 filters
   (Date/Dropdown/Number/Text). The extractor recognized DataGrid only; the rest used guard
   forms it skipped, so three generalisations were needed — strip **any** `<ident>.`
   namespace (the minifier names it `D.`/`M.`/`j.`/`A.` per widget, not just `_.`), strip a
   leading `return` (getProperties' first statement is `return <cond> && hide…`), and handle
   grouping parens `cond && ( hide, … )`. Coverage after (was 0 for everything but DataGrid):

   | Widget | lifted | Widget | lifted |
   |---|---|---|---|
   | DataGrid2 | 9/28 | DropdownFilter | 5/13 |
   | Gallery | 5/13 | DateFilter | 3/5 |
   | TreeNode (outline tree) | 4/7 | Number/TextFilter | 2–3 |
   | SelectionHelper | 1/2 | DropdownSort | 0/0 (no hides) |

   Remaining skips are grouped-subsequent and compound guards — safely skipped, never
   mis-lifted (DataGrid matrix + bundled stay 0). The filter **build path** was separate
   (`BuildFilterWidget` never applied visibility → a filter's hidden textTemplate stayed
   populated → CE0463); the nulling is now factored into engine-agnostic
   `widgetobj.ApplyVisibilityRules`, threaded through `FilterWidgetSpec`, and resolved by the
   executor. **Text/Number/Date filters → 0.**

   **Open — Dropdown filter definition drift (not a visibility gap).** `ddf` still shows 1
   CE0463, but its **definition** diff vs `update-widgets` is ~1529 lines (~30 `Type`, 28
   `ObjectType`): its nested filter-options `ObjectType` drifts from the 3.10.0 widget. This
   is the DataGrid-class definition reconciliation applied to the dropdown filter's
   object-list structure, a distinct follow-up — visibility can't touch it. TreeNode's
   remaining 3/7 skips are its "dynamic structure" guards (`transformGroupsIntoTabs`, nested
   `advancedMode`/`headerType` ternaries, `e.hasChildren||hide([…children…])`) — the ones the
   latest DW release expanded; a JS AST would be needed to lift them.

   **Superseded options** (kept for the record): (c) v2-safe `update-widgets` via #764 — no
   longer needed for DataGrid2 (mxcli now closes it natively); still the fallback for widgets
   whose editorConfig the static extractor can't fully lift. (d) in-process `goja` execution
   — moot: static lifting suffices for the common cases *and* yields declarative rules that
   `check`/lint/an LLM can consume, which JS execution wouldn't. (b) per-version templates —
   rejected.

   Remaining (follow-ups, not blockers): (1) the Dropdown filter's definition drift (above);
   (2) a real JS AST to lift the compound/ternary and object-list-nested guards (incl.
   TreeNode's dynamic structures) the regex extractor skips; (3) the same rules could feed
   LLM "property cards"; (4) fold the matrix into `scripts/widget-version-matrix.sh` as a
   standing gate (feasible now — `mxcli marketplace download` fetches any widget version)
   asserting the DW widgets across versions stay at 0.

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
