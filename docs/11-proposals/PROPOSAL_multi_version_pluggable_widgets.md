# Proposal: Multi-Version Pluggable Widget Support

**Status:** Draft
**Date:** 2026-06-12

## Problem Statement

mxcli must create pluggable-widget instances (ComboBox, DataGrid2, Gallery, …)
that Studio Pro accepts **without `CE0463` "the definition of this widget has
changed"** — for whatever Mendix version *and* whatever installed widget version
a project happens to be on. Today it can't do this reliably:

- **Legacy engine (`sdk/widgets`)** embeds static templates frozen at Mendix
  **11.6** (`templates/mendix-11.6/*.json`), hand-patched as gaps surface. They
  happen to work on 11.10 but are known to break on 10.x (see
  `sdk/versions/mendix-11.yaml`), and every new Mendix minor or widget version
  risks a fresh round of `CE0463`.
- **modelsdk engine (`modelsdk/widgets`)** vendored engalar's generator
  (`generate.go` / `augment.go` / `def.json`) **but kept the stale static
  templates as the primary source**, so it inherits the same fragility plus its
  own drift (the templates were a worse extraction — dirty Object defaults).

The cost shows up as `CE0463` on created widgets, a manual per-version
template-patching treadmill, and an existing (un-fixed) correctness gap on 10.x.

This proposal is **internal architecture only** — no new MDL syntax. It changes
how the widget Type+Object BSON is *resolved*, behind the existing
`WidgetObjectBuilder` interface.

## BSON Investigation: the CE0463 tolerance spike

Before designing anything, we measured **what `CE0463` actually checks**, because
the prevailing assumption (encoded in
`docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md`) turned out to be
wrong.

**Method.** Decode a real widget unit (`pymongo`), mutate one dimension, re-encode,
`mx check`. Every test has a no-op round-trip **control** proving the
decode→encode is byte-faithful (control always reproduces the baseline error
count). mxbuild **10.24.19** (project `test-1024`, loads clean) and **11.10.0**
(`test6-app`). Confirmed across **ComboBox, DataGrid2, Gallery**.

| Mutation | Layer | Result |
|---|---|---|
| Add `AllowUpload` to all `WidgetValueType` (10.24 ComboBox, 504×) | envelope field | **tolerated** — 0 errors |
| Remove `AllowUpload` (11.10 ComboBox, 522×) | envelope field | **tolerated** — baseline, no new CE0463 |
| Reverse `WidgetObject.Properties` order (ComboBox/DataGrid2/Gallery) | Object ordering | **tolerated** — 0 CE0463 |
| Rename one `WidgetPropertyType.PropertyKey` (ComboBox/DataGrid2/Gallery) | **schema** | **CE0463** — one per instance (18 DataGrid2, 15 Gallery) |
| Object carries instance-specific values inconsistent with schema (observed when extract-from-instance lifted a `System.Language` association ComboBox) | **Object↔schema** | **CE0463** |

**Conclusion.** `CE0463` is triggered by **schema mismatch** — the embedded
`CustomWidgetType`'s `PropertyKey` set / structure no longer matching the
*installed* widget definition — and by **Object↔schema inconsistency**. It is
**NOT** triggered by envelope field presence/absence (`AllowUpload`) or
`Properties` ordering, which Studio Pro **tolerates**.

This overturns the existing doc's "version-fragile envelope" framing: its 11.9
`AllowUpload`/ordering fixes worked because they were *holistic template
re-extractions*, not because those individual fields matter. (The doc must be
corrected — see Open Questions.)

### Two independent version axes

| Axis | Source | Drives | Evidence |
|---|---|---|---|
| **Widget version** (e.g. ComboBox `2.4.3` on 10.24 vs `2.5.0` on 11.10) | the project's installed `.mpk` | the **schema** (`PropertyKeys`, types) — *the axis `CE0463` checks* | `.mpk` declares it |
| **Mendix version** | runtime infra | the **envelope** (`WidgetValueType` field set incl. `AllowUpload`, ordering) — *tolerated* | `AllowUpload` is **0** in the `.mpk`, hard-coded in `sdk/widgets/augment.go:572`; 10.x instances carry 0 `AllowUpload`, 11.x carry 65 |

They move **independently**: a project can be bumped to a new Mendix without
updating its widgets, or a widget `.mpk` can be updated without updating the
project's existing instances (Studio Pro then shows `CE0463` on those stale
instances — the normal "Update widget" prompt). The authoritative schema for a
**newly created** instance is therefore the **currently installed `.mpk`**, never
an existing page instance (which may be stale on either axis).

## Proposed Architecture

Stop treating a widget template as one blob. Resolve the three concerns
separately:

1. **Schema ← the project's installed `.mpk`.** `GenerateFromMPK` becomes the
   **primary** source: the generated `CustomWidgetType` matches the installed
   widget *by construction*, on any Mendix version, for any widget version. This
   is the only layer `CE0463` actually checks.
2. **Object ← synthesized neutral defaults.** Generate the `WidgetObject`
   deterministically from the acquired Type: one default `WidgetValue` per
   `PropertyType` (empty `AttributeRef`/`DataSource`, type-appropriate
   `PrimitiveValue`), then let the existing `widgetobj` builder apply the user's
   actual property values on top. Never lift Object *values* from a page instance
   (the engalar failure) or a frozen template.
3. **Envelope ← tolerated superset.** The `WidgetValueType` field set / ordering
   need no per-Mendix-version exactness. Emit a stable superset (current
   `augment.go` defaults are fine); do **not** build a per-minor envelope model.

### Resolution chain (replaces `getOrGenerateTemplate`)

```
1. Session cache (per widgetID + project)
2. User-curated override: .mxcli/widgets/<name>.json   (hand-tuned escape hatch)
3. GenerateFromMPK(installed .mpk)  →  Type (schema-correct) + synthesized neutral Object
4. Static embedded template          →  last-resort fallback ONLY when no .mpk present
```

Extract-from-instance (engalar's `extract.go`) is **deliberately not adopted** as
a source: it inherits whatever staleness the project has on either axis, and it
lifts a real instance's Object values (the dirty-defaults `CE0463` we reproduced).

### Why each rejected alternative fails

| Alternative | Why rejected |
|---|---|
| Frozen static templates (today) | Wrong schema whenever the installed widget ≠ the frozen widget version → `CE0463`. Manual per-version maintenance. |
| Extract-from-instance (engalar) | Inherits stale schema (widget updated, instance not) *and* dirty Object values → reproduced `CE0463` this cycle. |
| Per-Mendix-version envelope model | The spike shows the envelope is **tolerated**; this would be effort spent on a non-problem. |
| Bulk-sync legacy templates to modelsdk | Still static/frozen; re-stales on the next widget or Mendix bump. |

## Implementation Plan

Behind the existing `backend.WidgetObjectBuilder` interface — no executor or MDL
changes. Applies to **both** engines (legacy and modelsdk) since both resolve
through the same `*/widgets` loaders.

### Files to modify/create

| File | Change |
|------|--------|
| `modelsdk/widgets/loader.go` (and `sdk/widgets/loader.go`) | Reorder `getOrGenerateTemplate`: prefer `GenerateFromMPK` over the static embed; static becomes last-resort fallback |
| `modelsdk/widgets/generate.go` | Harden `GenerateFromMPK`: (a) faithful XML→`PropertyType` schema; (b) **neutral Object synthesis** from the generated Type |
| `modelsdk/widgets/augment.go` | Confirm envelope defaults (incl. `AllowUpload`) are applied as a tolerated superset; reconcile with `sdk/widgets/augment.go` |
| `modelsdk/widgets/templates/mendix-11.6/*.json`, `sdk/widgets/templates/…` | Demote to fallback; eventually remove once MPK path covers all bundled widgets |
| `mdl/backend/widgetobj/builder.go` | Ensure the builder fully overrides neutral-Object slots it sets (no instance bleed-through) |
| `mdl/enginecompare/…` or a new `widgets/multiversion_test.go` | Per-version `mx check` matrix (see Test Plan) |
| `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` | Correct the "envelope fragile" framing per the spike |

### Phasing

1. **Spike→prove**: for ComboBox on 11.10, `GenerateFromMPK` Type + synthesized
   neutral Object + builder → `mx check` clean. (De-risks the whole thesis: if the
   MPK-derived Type matches the extracted Type, schema-from-MPK is sufficient.)
2. Extend Object synthesis to object-list widgets (DataGrid2 columns, Gallery
   items).
3. Flip the resolution chain to MPK-primary; keep static as fallback.
4. Per-version validation matrix; then retire the static templates.

## Version Compatibility

Not a version-gated *feature* — it is the mechanism that makes pluggable widgets
work across versions. The deliverable is a **cross-version validation matrix**:
for each installed mxbuild (currently **10.24, 11.9, 11.10**; add **11.11**),
create one of each pluggable widget against a fresh project and assert `mx check`
reports **0 `CE0463`**. This converts version drift from a field surprise into a
failing test.

Non-pluggable (`Forms$`) widgets need **no work** — they are already
version-resilient via the codec's declarative `VersionInfos` metadata
(`modelsdk/gen/*/version.go`, generated from reflection-data). Multi-version
support for them = regenerate gen from the target version's reflection-data
(mechanical, rare).

## Test Plan

- **Tolerance regression** (lock in the spike): unit tests that mutate
  `PropertyKey` (expect `CE0463`) vs envelope field/ordering (expect tolerated),
  so a future change that makes us *depend* on envelope exactness is caught.
- **Per-version matrix**: `widgets/multiversion_test.go` — create
  ComboBox/DataGrid2/Gallery on fresh 10.24 / 11.9 / 11.10 / 11.11 projects,
  `mx check`, assert 0 `CE0463`. Requires the respective mxbuilds (Docker/CI).
- **MPK fidelity**: assert a `GenerateFromMPK` Type is structurally equivalent
  (PropertyKey set) to a Studio-Pro-extracted Type for the same widget+version.
- **No-instance creation**: create a widget in a project with *no* existing
  instance of it (only the `.mpk`) — must succeed via the MPK path.

## Open Questions

1. **MPK→Type fidelity gap.** engalar's note that `GenerateFromMPK` produces
   "subtly different BSON" — is that difference in the **schema** (would cause
   `CE0463`, must fix) or only the **Object/envelope** (tolerated)? Phase-1 spike
   answers this directly; the whole proposal's cost hinges on it.
2. **Generalization.** Spike covered ComboBox/DataGrid2/Gallery. Confirm the
   schema-strict/envelope-tolerant pattern on object-list-heavy widgets with
   nested CustomWidgets (charts, Maps) and on **12.x** when available.
3. **Doc correction.** `WIDGET_BSON_VERSION_COMPATIBILITY.md` and the
   `pluggable_widgets` notes in `sdk/versions/*.yaml` currently assert envelope
   fragility — update to the schema-is-the-trigger model.
4. **ADR.** "Installed-`.mpk` is the authoritative widget schema source" is a
   cross-cutting decision; promote to an ADR once Phase 1 confirms feasibility.

## Relationship to existing artifacts

- Memory: `reference_ce0463_tolerance_spike.md` (the empirical basis),
  `reference_modelsdk_pluggable_widgets.md` (engine/registry history,
  extract-from-instance rejection).
- Complements `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` (which
  this proposal corrects on the trigger, and supersedes on the strategy).
- No overlap with existing widget proposals (`PROPOSAL_update_builtin_widget_properties`,
  `PROPOSAL_widget_property_visibility`, `PROPOSAL_v0_12_0_widget_consolidation`),
  which concern property *editing*, not template *resolution*.
