# Widget BSON Version Compatibility

How mxcli's pluggable-widget BSON output stays in sync (or doesn't) with
specific Mendix versions, and how to extend support to a new minor release.

## Two-layer model

mxcli's widget BSON output is assembled from two sources with very different
version-resilience characteristics:

```
┌────────────────────────────────────────────────────────────────────┐
│                       Widget BSON output                           │
│                                                                    │
│   ┌─────────────────────────────────┐                              │
│   │ Widget-specific structure       │  ← project's .mpk file       │
│   │ (PropertyKeys, sub-properties,  │  version-tracked per widget  │
│   │  attribute types, etc.)         │                              │
│   └─────────────────────────────────┘                              │
│                                                                    │
│   ┌─────────────────────────────────┐                              │
│   │ Mendix BSON envelope shape      │  ← embedded templates        │
│   │ (WidgetValueType fields,        │  tied to Mendix 11.6 base    │
│   │  CustomWidget envelope,         │  patched manually for 11.9   │
│   │  array marker conventions)      │                              │
│   └─────────────────────────────────┘                              │
└────────────────────────────────────────────────────────────────────┘
```

**Widget-specific structure** is version-resilient out of the box. `widget init`
parses each project's installed `.mpk` files to derive the per-widget shape;
`sdk/widgets/augment.go` syncs additions and removals between the embedded
template and the installed widget. A new pluggable widget version (e.g.
DataGrid `2.30.1` → `2.31.0`) just works after `mxcli widget init`.

**Mendix BSON envelope shape** is brittle. The embedded templates at
`sdk/widgets/templates/mendix-11.6/*.json` were extracted at Mendix 11.6 and
manually patched as gaps surfaced. Each new Mendix minor that adds a field
to `CustomWidgets$WidgetValueType` or restructures the envelope requires
another round of patching.

## Why this split exists

`.mpk` files declare the widget's own contract (what properties exist, what
types they accept, what defaults the widget author chose). They're shipped
by widget authors and versioned independently of Mendix.

The BSON envelope (`CustomWidgets$WidgetValueType` field set, `WidgetObject`
ordering rules, `Forms$Appearance` structure, etc.) is Mendix runtime infra
that evolves with Mendix itself. There's no per-project file declaring it —
Studio Pro hardcodes the expected shape, and "right" depends on which
Mendix version is reading the BSON.

## Fixes landed for Mendix 11.9 compatibility

| Commit | Field / behavior | Why 11.9 cared |
|---|---|---|
| `ec99cdff` | `AllowUpload: false` on every `WidgetValueType` | Field added in some 11.x; absence triggers CE0463 across every widget |
| `b1f4de3a` | `WidgetObject.Properties` order = `WidgetType.PropertyTypes` order | Studio Pro 11.9 checks for matching ordering; bulk-reordered 5 templates |
| `7e6fee84` | Filter widgets carry full CustomWidget envelope (`Appearance`, `ConditionalEditabilitySettings`, `ConditionalVisibilitySettings`, `LabelTemplate`) | Studio Pro flags incomplete envelopes on nested CustomWidgets |
| `f9818394` | `TextTemplate.Template.Items` populated from `PropertyType.ValueType.Translations` defaults; `Editable: "Always"` on filter widgets | Studio Pro copies translation defaults at widget creation; mxcli left them empty |
| `aea000b7` | `columnsFilterable` and `sortable` Boolean values aligned with their `PropertyType.ValueType.DefaultValue` | Template-extraction bug: stored `false` vs schema-default `true`; Studio Pro detects mismatch |
| `4ea402c2` | Object-list item TextTemplate slots emit `null` (not placeholder `" "` ClientTemplate) when unset | `createDefaultWidgetValue` + `overlayItemValue` were manufacturing `Text: " "` ClientTemplates for every TextTemplate-typed sub-property of every object-list item. Studio Pro CE0463 on Accordion, AreaChart, Maps engine-routed widgets (#548) |

After the first five fixes the v0.10 acceptance fixture
(`mdl-examples/doctype-tests/31-pluggable-datagrid-gallery-v010-examples.mdl`)
emits zero CE0463 errors on a fresh Mendix 11.9 project. The sixth fix
extends this to the engine-routed widgets in
`mdl-examples/doctype-tests/32-pluggable-widget-object-lists-v010.mdl`
— `mx check` reports zero CE0463 across all six failing widget instances
(`acc2`, `map1`, `map2`, `menu1`, `menu2`, `chart1`). Remaining errors on
that fixture are authoring issues (missing `menuTrigger` child slot
keyword, missing Maps API key), not BSON drift.

## What's version-stable vs version-fragile

| Element | Source | Version-fragile? |
|---|---|---|
| Widget PropertyKeys (top-level) | MPK XML via `widget init` | ✓ stable |
| Widget property types (Attribute / Expression / TextTemplate / etc.) | MPK XML | ✓ stable |
| Object-list properties (`columns`, `groups`, `series`...) | MPK XML | ✓ stable |
| Sub-property trees inside object lists | MPK XML | ✓ stable |
| Widget version (DataGrid 2.22 → 2.30) | MPK file metadata + augmentation | ✓ stable |
| `CustomWidgets$WidgetValueType` field set | Embedded template + manual patches | ✗ fragile |
| `CustomWidgets$WidgetObject` Properties array ordering | Embedded template | ✗ fragile |
| Required CustomWidget envelope fields | Embedded template + filter widget builder | ✗ fragile |
| TextTemplate default translation population | Embedded template | ✗ fragile |
| Boolean property default consistency | Embedded template | ✗ fragile |

## Onboarding a new Mendix minor (e.g. 11.10, 12.0)

The CE0463 fix methodology used for 11.9 generalizes. Steps:

1. **Download mxbuild for the target version**:
   ```bash
   mxcli setup mxbuild --version 11.10.0
   ```

2. **Run the v0.10 fixture against a fresh 11.10 project**:
   ```bash
   mxcli new TestApp --version 11.10.0
   mxcli widget init -p TestApp/TestApp.mpr
   mxcli exec mdl-examples/doctype-tests/31-pluggable-datagrid-gallery-v010-examples.mdl -p TestApp/TestApp.mpr
   ```

3. **Check with mx**:
   ```bash
   ~/.mxcli/mxbuild/11.10.0/modeler/mx check TestApp/TestApp.mpr
   ```

4. **For each new CE0463** (or other widget validation error):

   Use the **"Studio Pro Update Widget" diff** methodology documented in
   [`.claude/skills/debug-bson.md`](../../.claude/skills/debug-bson.md#ce0463-widget-definition-changed):
   - Snapshot the failing BSON
   - Open in Studio Pro 11.10
   - "Update widget" on one failing widget instance
   - Snapshot again
   - Diff (with UUID normalization)
   - The diff tells you exactly what to patch in the embedded templates or
     the filter widget builder

   Each pattern that appears (new envelope field, ordering change, default
   value) typically yields a one-line fix and unblocks dozens of widgets.

5. **Add a non-regression test** — see "Cross-version validation" below.

## Where the patches live

- **Embedded templates**: `sdk/widgets/templates/mendix-11.6/*.json` —
  for envelope-level fixes that apply to every widget instance loaded from
  the embedded template. Most CE0463 fixes land here as bulk-edits across
  files (the `AllowUpload` fix added 581 fields across 30 files in one go).

- **Filter widget builder**: `mdl/backend/mpr/datagrid_builder.go`
  (`buildFilterWidgetBSON`, `buildMinimalFilterWidgetBSON`, the
  `defaultEmptyAppearance` helper) — for the CustomWidget envelope mxcli
  constructs around filter widgets inside DataGrid columns.

- **WidgetValueType serializer**: `sdk/mpr/writer_widgets_custom.go`
  (`serializeWidgetValueType`) — for the structured-data path (not the
  RawType clone path) when building widget BSON from typed inputs.

- **Template augmentation**: `sdk/widgets/augment.go`
  (`createDefaultValueType`) — for MPK-derived widget templates when no
  embedded template exists.

When patching a field, **update all four paths** if the field is supposed to
be ubiquitous. The CE0463 fixes for `AllowUpload` touched the embedded JSON,
both serializers, and the augment helper.

### Gotcha: `$ID` placeholders must be unique

When bulk-adding entries with a `$ID` field to embedded templates (e.g.
`Texts$Translation` entries inside `TextTemplate.Template.Items`), each
entry **must** have a unique placeholder `$ID` value. The loader's
`collectIDs` remapping (in `sdk/widgets/loader.go`) treats identical `$ID`
strings as the same logical entity and remaps them to a single new UUID at
load time. Multiple widget instances on a page then end up referencing the
same UUID, triggering `Duplicate Guid in unit page ...` from `mx
update-widgets` and a subsequent `Root unit not found` corruption.

**Convention**: follow the `placeholderID()` function in
`sdk/widgets/augment.go` — `aa000000000000000000000000XXXXXX` with a unique
counter per entry. Caught by the integration test
`TestMxCheck_DoctypeScripts`, fixed in commit
[`8ead1cff`](https://github.com/mendixlabs/mxcli/commit/8ead1cff).

## 11.10 onboarding: one drift found (textfilter attrChoice)

Running the widget fixtures through `exec` + `mx check` on **both** Mendix 11.9
(`test5-app`) and 11.10 (`test6-app`) surfaced exactly one real envelope drift,
in the **`DatagridTextFilter`** widget:

| Construct | 11.9 | 11.10 | Cause |
|---|---|---|---|
| filter with explicit `attributes: [...]` (Gallery `filter` block) | accepted | **CE0463** | `attrChoice="auto"` + a populated `attributes` list. 11.9 tolerated it; 11.10+ rejects it. Fixed (#605): emit `attrChoice="linked"` when attributes are explicit |
| bare filter inside a DataGrid column | accepted | accepted | `attrChoice="auto"` with no attributes — correct on both |

That was the **only** field whose validation changed between 11.9 and 11.10 in
the fixtures exercised. Everything else (the `AllowUpload` field set,
`WidgetObject` property ordering, `Appearance`/`TextTemplate` conventions) is
stable across 11.9 → 11.10. After the #605 fix, all fixtures pass the
cross-version gate with no drift.

> **Caution — the first "no drift" pass was a false negative.** An earlier run
> concluded 11.9 ↔ 11.10 had *no* drift. It was wrong for two reasons, both now
> fixed:
> 1. **Coverage gap.** The gate's fixtures (31/32) only used *column-bound*
>    filters. The one Gallery-filter case (`tf1` in 30) failed on *both*
>    versions (it had its own bug), so the gate read "identical sets = no drift"
>    and never exercised the drifting construct. Fixture `03-page-examples`
>    (which has Gallery-filter textfilters) is now in the gate's set.
> 2. **Divergent baselines.** The gate compared `test5-app` (11.9) against
>    `test6-app` (11.10), but they had drifted apart — `test5-app` carried a
>    leftover `PgTest` module from an earlier `exec` that `test6-app` lacked, so
>    the two weren't equivalent baselines. The gate now drops each fixture's
>    `create module` targets before running it, so leftover/divergent state in a
>    reference project no longer skews the comparison.
>
> Lesson: a cross-version gate is only as good as (a) its fixture coverage of
> *every* construct and (b) the equivalence of its reference baselines. "No
> drift" means nothing if the drifting construct isn't in a fixture, or if the
> two projects being compared aren't identical apart from Mendix version.

So Stream A's planned per-version conditionals (threading `MendixVersion` into
the serializer, gating `AllowUpload`/`Appearance` shape) were **not** needed —
the one real drift was a widget-property convention (`attrChoice`) fixed in the
builder, not a per-version envelope field. Future minors that *do* change an
envelope field would surface through the gate below.

## Cross-version validation gate

`make check-widget-versions` (script: `scripts/check-widget-versions.sh`) runs
a widget fixture through `exec` + `mx check` against several Mendix versions and
**fails if the CE0463 set differs between them** — i.e. it detects envelope
drift specifically, ignoring version-independent bugs (which appear on every
version). It does *not* require zero CE0463.

```bash
# Defaults to test5-app (11.9) + test6-app (11.10); override the project paths:
make check-widget-versions \
  MX_PROJECT_119=/path/to/app-11.9.mpr \
  MX_PROJECT_1110=/path/to/app-11.10.mpr

# Or run the script directly against any fixture + version set:
scripts/check-widget-versions.sh mdl-examples/doctype-tests/31-pluggable-datagrid-gallery-v010-examples.mdl \
  11.9.0:../ModelSDKGo/mx-test-projects/test5-app/test5.mpr \
  11.10.0:../ModelSDKGo/mx-test-projects/test6-app/test6.mpr
```

Each version needs its mxbuild installed (`~/.mxcli/mxbuild/<ver>/`) and a
reference project with the fixture's widgets (`.mpk`) installed. The 11.10
`mx` binary's libSkiaSharp crash is handled automatically (the script checks
via `scripts/mx-check.sh`). Before running a fixture the gate drops that
fixture's `create module` targets in each sandbox, so leftover or divergent
state in a reference project (e.g. a stale `PgTest`) doesn't skew the
comparison — the reference projects only need the same installed widgets, not
identical document content. This catches version drift the moment it happens,
rather than at user-report time. The long-term replacement (build-time
templates per version) is tracked under the unified schema registry effort
([#529](https://github.com/mendixlabs/mxcli/issues/529), Phase 5).

## The long-term answer

The brittleness of the embedded-template layer is exactly what the unified
schema registry proposal addresses
([`docs/11-proposals/UNIFIED_SCHEMA_REGISTRY.md`](../11-proposals/UNIFIED_SCHEMA_REGISTRY.md)).
Phase 4 of that proposal replaces the embedded `mendix-11.6/*.json`
snapshots with templates generated at build time from
`mx dump-mpr` output, parameterized by Mendix version. New Mendix release
support becomes "run codegen against `mx` from that version" rather than
manual patching after CE0463 reports come in.

In the meantime, this doc + the `.claude/skills/debug-bson.md` methodology
keep the patch cadence manageable.

## References

- [`.claude/skills/debug-bson.md`](../../.claude/skills/debug-bson.md) — investigation procedure for CE0463 and related widget BSON errors
- [`docs/03-development/PAGE_BSON_SERIALIZATION.md`](PAGE_BSON_SERIALIZATION.md) — page-level BSON serialization design
- [`docs/03-development/BSON_TOOLING_GUIDE.md`](BSON_TOOLING_GUIDE.md) — `mxcli bson dump` reference
- [Issue #541](https://github.com/mendixlabs/mxcli/issues/541) — the CE0463 case study that motivated this doc
- [Issue #529](https://github.com/mendixlabs/mxcli/issues/529) — unified schema registry proposal (long-term fix)
