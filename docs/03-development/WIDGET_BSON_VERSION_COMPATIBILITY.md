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
| BSON list marker on empty arrays (`[3]`/`[2]` vs bare `[]`) | Embedded template | ✗ fragile — **11.12 hard-fails**, ≤ 11.11 tolerates |

### Markerless empty arrays crash 11.12 load

Every Mendix list serializes as a BSON array whose first element is a marker int
(`Texts$Text.Items`→`[3]`, `Forms$ClientTemplate.Parameters`→`[2]`,
`Widgets`/`Objects`→`[2]`). A hand-authored template that writes an empty list as
a bare `[]` (no marker) is tolerated by Mendix ≤ 11.11 but **mis-parsed by 11.12's
`StreamingBsonUnitReader`**, which aborts the entire project load with:

```
System.InvalidOperationException: Type …CustomWidgets.WidgetProperty does not
contain a constructor with a parameter of type …CustomWidgets.WidgetValue.
```

This shipped in `datagrid-number-filter.json` (placeholder / screen-reader
`ClientTemplate` blocks had `"Items": []` / `"Parameters": []`), silently
corrupting any `.mpr` that used a `NUMBERFILTER` — it passed `mxcli check`. The
regression guard `TestTemplates_NoMarkerlessEmptyArrays` (in both `sdk/widgets`
and `modelsdk/widgets`) walks every embedded template and fails on any bare `[]`.
When onboarding or re-extracting a template, never emit an empty markerless array.

## CE0463 after a widget-package upgrade is usually NOT an mxcli bug

Before treating a CE0463 report as template drift, establish **when the widget was
authored relative to the installed package**. The two cases look identical in
`mx check` output and have completely different causes.

**Case 1 — authored against version A, package upgraded to version B.** Expected
Mendix behaviour, not an mxcli defect. A widget package that drops a property
leaves every *stored* instance carrying a property the new definition no longer
has, which is exactly what CE0463 reports and exactly what its message
("Update this widget / Update all widgets") tells you to fix.

Worked example (mendixlabs/mxcli#716, Ledger on Mendix 11.12):

| | |
|---|---|
| Data Widgets 3.4 (as authored) | **0 errors** |
| upgraded to 3.11.3 | 36 CE0463 — 7 mxcli-authored, 29 Studio Pro's own template widgets |
| after `mx update-widgets` | **0 errors** |

The cause was a single dropped property: `key="advanced"` ("Enable advanced
options") is present in `Datagrid.xml` at 3.4 and absent at 3.10 and 3.11.3.
`update-widgets` deletes both the `WidgetPropertyType` and its `WidgetProperty`;
everything else in the diff is index shift.

**Two controls make this diagnosis, and neither is optional:**

1. **Do Studio Pro's own widgets fail too?** A blank project's `dataGrid2_*`,
   `gallery1/2`, `drop_downFilter1/2` are authored by Mendix. If they fail
   alongside mxcli's, the tool is not the variable. (Here: 29 of the 36.)
2. **Does `mx update-widgets` clear it?** If yes, mxcli's BSON was structurally
   valid — it was correct for the version it was written against. Genuine
   template bugs do *not* clear this way; the Image stale-default and the
   number-filter markerless array both needed template fixes.

**Case 2 — authored fresh against the new package and still failing.** This is
the mxcli defect. Create a project with the new package installed, run
`widget init`, author the widgets, then `mx check`. On Data Widgets 3.10/3.11
that isolates a much narrower failure than #716 as filed: freshly authored
DataGrid2 is **clean**, while Gallery and DatagridDropdownFilter still produce
CE0463 (6 instances across the v0.10 fixture).

**Do not measure Case 2 with the doctype fixtures alone.** Their pages sit in a
blank project whose own template widgets are already failing from Case 1, so a
raw CE0463 count mixes the two. Subtract by widget *name* against a control
project that ran no mxcli command at all.

### The residual #716 failures are NOT explained by template drift

The obvious model — "the embedded template is behind the installed package, so
widgets whose property set drifted the most fail" — is **wrong**. Measured on Data
Widgets 3.10, comparing each embedded template's `PropertyKey` set against the
installed `.mpk` XML, alongside whether freshly authored instances pass `mx check`:

| Template | must ADD | must REMOVE | in sync | fresh authoring |
|---|---|---|---|---|
| `datagrid` | 19 | 1 | 60 | **passes** |
| `gallery` | 11 | 0 | 33 | **fails** (4 instances) |
| `datagrid-dropdown-filter` | 0 | 0 | 27 | **fails** (2 instances) |
| `datagrid-text-filter` | 0 | 0 | 13 | **passes** |

Drift does not predict failure in either direction. `datagrid` has by far the most
churn and is clean; `dropdown-filter` and `text-filter` are byte-for-byte in sync
with the package and disagree with each other. Whatever distinguishes them is in the
*content* of specific properties, not in which properties exist.

**A field-level prune is also disproven, and dangerously so.** Deleting
`OnChangeProperty` / `Required` from every `CustomWidgets$WidgetValueType` — the
fields `mx update-widgets` omits on Gallery — takes fresh-authoring CE0463 from 6 to
4 on Data Widgets 3.10, but takes the **shipped 3.4 from 0 to 139**. Those fields are
required on the version the project ships with; removing them unconditionally breaks
every widget. Do not treat "the reference output omits it" as "we should never emit
it" without testing the version the project actually uses.

The cause of the four Gallery failures is **open**. It is not the property set, not
`OnChangeProperty`/`Required` values, not `Appearance.DesignProperties`,
`LabelTemplate`, the `GridSortBar` list marker, `SortDirection`/`SortOrder`, or
`AttributeRef.EntityRef` — each was patched in isolation and re-checked, none moved
the count.

### There IS a template-free generic path — and it does not fix Gallery either

`modelsdk/widgets/loader.go:getOrGenerateTemplate` resolves a widget template in
three steps: embedded template, session cache, then **`GenerateFromMPK`** — a
complete Type+Object built from the project's `.mpk` with no embedded snapshot at
all. It is not theoretical: Charts (Pie/Column/Line/Bar/Area) ship no template and
are authored entirely this way (`91b054b`).

Because step 1 wins whenever an embedded template exists, Gallery never reaches it.
Forcing it to (temporary env switch, since reverted) on Data Widgets 3.10:

- The output genuinely changed — 17 lines of Type diff, and `OnChangeProperty`
  moved from the embedded `"onConfigurationChange"` to `""`, which is what
  `mx update-widgets` produces.
- **All four galleries still failed CE0463.**

So "derive the template from the package instead of the frozen snapshot" is
available today, demonstrably takes effect, and is still not sufficient. Combined
with the `SynthesizeNeutralObject` spike in
[`PROPOSAL_multi_version_pluggable_widgets.md`](../11-proposals/PROPOSAL_multi_version_pluggable_widgets.md),
two independent generic-construction approaches have now failed on the same class
of widget, which is evidence the missing information is genuinely not in the `.mpk`.

### The untested lead: CE0463 from a VALUE, not a schema

Every CE0463 fix landed in the past week was value-shaped, not schema-shaped, and
the error message named the widget version in each case:

| Fix | Cause |
|---|---|
| `3cb8ab6` (ledger #54) | a column header serialized as an **empty** `TextTemplate` where Studio Pro wants the attribute name filled in |
| `455c43a` | a hidden chart-series `markerColor` serialized as an **empty** `Forms$ClientTemplate`; Studio Pro stores **null** |
| `4ea402c2` (#548) | object-list item TextTemplate slots emitting a placeholder `" "` ClientTemplate instead of null — CE0463 on Accordion, AreaChart, Maps |
| `abba773` | an unset chart-series String emitted as `" "` instead of `""` |

The Gallery investigation for #716 went the other way — Type/schema first — and ruled
out the whole schema axis. The empty-vs-null-vs-placeholder axis inside the Gallery's
`Object` (its content slots, item templates, and the `Forms$ClientTemplate` nodes
underneath) has **not** been examined, and it is where four of the last five CE0463
fixes actually lived.

### #716 is TWO bugs, separated by one experiment

The failing set — 4 galleries + 2 drop-down filters on Data Widgets 3.10 — is not one
defect. Authoring the same fixture two ways on Mendix 11.13 splits it cleanly:

| | authored on bundled 3.4, then package upgraded to 3.10 | authored fresh on 3.10 | fresh on 3.10, `augmentFromMPK` disabled |
|---|---|---|---|
| Galleries | **4 fail** | 4 fail | **4 fail** |
| Drop-down filters | **0 fail** | **2 fail** | **0 fail** |

**Drop-down filters: `augmentFromMPK` introduces the fault.** They are clean when
authored against the package the template matches (3.4), clean when that project is
upgraded, and clean on 3.10 with augmentation switched off — but fail when
augmentation runs against the 3.10 `.mpk`. Augmentation is *making them worse*. This
is a real mxcli bug and the actionable half of #716. Note the template needs **0
additions and 0 removals** against 3.10, so whatever augmentation changes is at the
attribute level, not the property set.

**Galleries: the embedded template is simply 3.4-shaped.** They fail identically with
augmentation, without it, and when authored on 3.4 and merely upgraded — the same
behaviour as the blank project's own Studio-Pro-authored `gallery1`/`gallery2`. mxcli
emits the same gallery whatever package is installed: correct on 3.4 (0 errors),
stale on 3.10. That is **Case A**, the normal "Update all widgets" situation, not an
authoring defect — and it is fixed by instance reconciliation
([`PROPOSAL_widget_instance_reconciliation.md`](../11-proposals/PROPOSAL_widget_instance_reconciliation.md)),
not by patching the template.

This also explains why the earlier elimination pass found nothing: it was hunting an
authoring bug in the gallery, and there isn't one.

**Method note.** No Studio Pro required — a blank project ships Studio-Pro-authored
`gallery1`/`gallery2`, and authoring the same fixture against two package versions
gives the comparison. An earlier note here claiming a Studio Pro reference was needed
was wrong.

### #716 Gallery: what was ruled out while hunting the wrong bug

Investigated exhaustively on Mendix 11.12.2 + Data Widgets 3.10, fixture 31, against
an `mx update-widgets` reference of the same project. **Unresolved** — recorded so the
next attempt starts from the eliminations rather than repeating them.

**The constraining fact.** Replacing mxcli's whole `galCustomers` widget node with the
reference node clears its CE0463 (35 → 34 errors). Replacing only its `Type`, or only
its `Object`, **crashes the project load** — mx check reports "0 errors" because it
never loads, which is an artifact, not a fix. So the cause is inside the widget node
and requires Type and Object to stay consistently paired.

**Ruled out, each by patch-and-recheck:**

| Axis | Method | Result |
|---|---|---|
| Property set drift | template `PropertyKey` set vs `.mpk` XML | does not predict failure — `datagrid` ADD 19/REMOVE 1 passes, `datagrid-dropdown-filter` 0/0 fails |
| All value differences | full path-level diff → **16** differing paths, applied to the failing widget alone | still fails |
| `OnChangeProperty`, `Required` | value sync, then field prune | prune fixes 2 filters but takes shipped DW 3.4 from 0 → **139** |
| `PrimitiveValue` `below`→`bottom` | patched | no change |
| `GridSortBar` marker 3→2, `SortDirection`→`SortOrder`, `AttributeRef.EntityRef` | patched | no change |
| `Appearance.DesignProperties`, `LabelTemplate` | patched | no change |
| Pointer integrity | every `TypePointer` resolves; no orphan `PropertyType` | identical to reference |
| Pointer semantics | each property mapped to the `PropertyKey` it points at, all depths, document order | **identical** to reference |
| Property ordering | Object order vs Type order | identical to reference |
| BSON key order | raw key sequence of the widget node | mxcli is non-alphabetical, reference is — but **`tfSearch` passes with the identical non-alphabetical order**, so key order is not the discriminator |
| Generic MPK-derived template | forced Gallery through `GenerateFromMPK` | output changed (17 Type lines), still fails |
| Definition-registry precedence | built-in as fallback instead of override | no change; regressed `17-custom-widget-examples` |

**Where that leaves it.** By every measure computable from the decoded BSON — values,
keys, ordering, pointer topology — mxcli's failing Gallery is identical to a reference
that passes. The difference is therefore in something a Python BSON round-trip
normalises: binary field values, or an encoding detail below the document model. The
next attempt should work at the **byte level** (compare the encoded unit ranges
directly) rather than on decoded documents, or obtain a Studio-Pro-authored Gallery on
Data Widgets 3.10 for a third reference point.

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

- **WidgetValueType serializer**: `mdl/backend/modelsdk/widget_pluggable_write.go`
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

## 11.13 onboarding: the drift was outside the widget layer

Adding a Mendix minor to the nightly matrix is not a one-line change to
`.github/workflows/nightly.yml` — run the doctype corpus against it first:

```bash
mxcli setup mxbuild --version 11.13.0
MX_BINARY=~/.mxcli/mxbuild/11.13.0/modeler/mx \
  go test -tags integration -count=1 -run TestMxCheck_DoctypeScripts ./mdl/executor/
```

11.13 produced **no** widget-envelope drift. The one failure came from a
different layer entirely — the Database Connector — and shows the shape of
problem to expect from any new minor: a renamed, re-encoded property.

| Construct | ≤ 11.12 | 11.13 | Cause |
|---|---|---|---|
| any `EXECUTE DATABASE QUERY` activity | accepted | **CE5277** "Please re-run and save the query to fix the error" | `DatabaseConnector$DatabaseQuery.QueryType` (int, 1 = custom SQL) became `Type` (string enum `Select`/`NonSelect`/`Unknown`). mxcli wrote only the legacy key, so `Type` was absent — and absent reads as Unknown. Fixed in `mdl/dbconnector` |
| a `PostgreSQL`/`MSSQL` connection in a bare project | accepted | **CE5278** "The … JDBC driver is missing from the module settings" | A new check on the module's Java dependencies, not on anything mxcli writes. Not fixed: mxcli has no way to author module settings. Invisible in the doctype harness, which imports the connector mpk |

**The method that settled it without guessing**: run the *new* mxbuild's own
migration over an old project and diff the result.

```bash
mx convert -p -s /path/to/11.12-project    # with the 11.13 mx binary
```

Mendix ships a one-time conversion per renamed property
(`ExternalDatabaseConnectionQueryTypeConversion` here), so the converted
document *is* the authoritative target shape. This is the non-widget counterpart
of the "Studio Pro Update Widget" diff above, and it needs no Studio Pro.

Two rules carried over from the settings rename (#759) apply verbatim:

1. **Write exactly one spelling.** Writing both is not a safe hedge — a property
   the target version's metamodel does not define is what Studio Pro fails to
   resolve on open. mxbuild tolerates it, so `mx check` is not the gate here.
2. **The read side must accept either**, or the next ALTER of a new-version
   project writes the old key's default straight back over the new one.

## References

- [`.claude/skills/debug-bson.md`](../../.claude/skills/debug-bson.md) — investigation procedure for CE0463 and related widget BSON errors
- [`docs/03-development/PAGE_BSON_SERIALIZATION.md`](PAGE_BSON_SERIALIZATION.md) — page-level BSON serialization design
- [`docs/03-development/BSON_TOOLING_GUIDE.md`](BSON_TOOLING_GUIDE.md) — `mxcli bson dump` reference
- [Issue #541](https://github.com/mendixlabs/mxcli/issues/541) — the CE0463 case study that motivated this doc
- [Issue #529](https://github.com/mendixlabs/mxcli/issues/529) — unified schema registry proposal (long-term fix)
