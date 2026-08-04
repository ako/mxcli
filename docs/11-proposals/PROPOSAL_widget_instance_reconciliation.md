# Proposal: `mxcli widget sync` — reconcile stored widget instances against installed .mpk packages

**Status:** Draft
**Date:** 2026-08-03

## Problem Statement

mxcli authors pluggable-widget instances correctly for the widget package installed
**at authoring time**. It has nothing that reconciles instances **already stored in
the model** when that package later changes. Studio Pro has "Update widget" /
"Update all widgets"; mxbuild has `mx update-widgets`; mxcli has no equivalent.

So an mxcli-only workflow — the whole point of the tool — breaks the moment a
developer upgrades a widget module. The remedies today are both bad:

- **Open Studio Pro** and click Update all widgets, which defeats headless use.
- **Run `mx update-widgets`**, which on MPR v2 **destroys `mprcontents/`**, collapsing
  the project back to a single-file v1 layout. Documented as a data-loss trap in
  `.claude/skills/fix-issue.md`; observed again while investigating #716.

### Measured evidence (mendixlabs/mxcli#716)

Two real mxcli-built projects, Mendix 11.12, upgrading Data Widgets 3.4 → 3.11.3:

| Project | as authored (DW 3.4) | after upgrade to 3.11.3 | after `mx update-widgets` |
|---|---|---|---|
| Ledger | 0 errors | 36 CE0463 (7 mxcli-authored, 29 Studio Pro's own) | **0 errors** |
| TimeRegistration | 0 errors | 29 CE0463 (all 29 Studio Pro's own) | — |

The cause is mechanical. Diffing `dgTransactions` against the `update-widgets`
output (ids normalised by graph position, not stripped) gives 3213 lines, of which
exactly one is a real change: mxcli's stored instance carries a property the new
package dropped.

```
key="advanced"  ("Enable advanced options")
  Datagrid.xml @ 3.4.0   -> present
  Datagrid.xml @ 3.10.0  -> absent
  Datagrid.xml @ 3.11.3  -> absent
```

`update-widgets` deletes the `CustomWidgets$WidgetPropertyType` from the Type and
its paired `CustomWidgets$WidgetProperty` from the Object. Everything else in the
diff is `$ID` index shift.

**That `update-widgets` clears it to 0 is the key fact**: mxcli's BSON was
structurally valid and correct for the version it was written against. This is not
template drift — it is missing reconciliation. Genuine template bugs do *not* clear
this way (the Image stale default and the number-filter markerless array both
needed template fixes).

### Why this is not covered by existing proposals

[`PROPOSAL_multi_version_pluggable_widgets.md`](PROPOSAL_multi_version_pluggable_widgets.md)
covers the **creation** path and explicitly scopes *out* the case here:

> They move independently (update Mendix without widgets, or a widget `.mpk` without
> the project's existing instances → Studio Pro shows `CE0463` on those stale
> instances = the normal "Update widget" prompt). The authoritative schema for a
> *newly created* instance is the currently installed `.mpk`.

This proposal is the other half: the authoritative schema for an **already stored**
instance, when the `.mpk` moves under it. The two are complementary and share
`AugmentTemplate`'s reconciliation logic.

[`PROPOSAL_update_builtin_widget_properties.md`](PROPOSAL_update_builtin_widget_properties.md)
and [`PROPOSAL_bulk_widget_property_updates.md`](PROPOSAL_bulk_widget_property_updates.md)
are about the MDL `UPDATE WIDGETS SET … WHERE …` statement, which *sets property
values* the author chooses. This proposal *reconciles schema* the author does not
choose. Different operation, and the naming must not collide — see Open Questions.

## Related finding (2026-08-04): marketplace modules need the same primitive

While investigating whether an installed marketplace module can be diffed against its
published package
([`PROPOSAL_marketplace_module_upgrade.md`](PROPOSAL_marketplace_module_upgrade.md)),
the drift turned out to be **entirely** pluggable-widget BSON:

| Module | Pages / pluggable widgets | Drift inside `CustomWidgets$` | Outside |
|---|---|---|---|
| Administration 4.3.2 | 9 pages | 15,041 paths | 0 |
| WebActions 2.11.0 | none | 0 | 1 (`PackageId`, differs by construction) |

An installed module is byte-identical to its version-converted package except inside
widget subtrees — because those instances were reconciled against the *consuming
project's* widget packages on import. That is this proposal's subject seen from the
other side.

The consequence for both: a way to ask *"does this stored widget instance match this
installed package?"* is needed by `widget sync` (to decide what to reconcile) and by
`marketplace diff` (to compare a widget subtree without drowning in envelope noise).
It should be built once. Whichever of the two is implemented first should place it
where the other can use it — a comparison entry point in `modelsdk/widgets` next to
`AugmentTemplate` is the obvious home.

## BSON Structure

No new document type. The operation rewrites two paired arrays inside every stored
`CustomWidgets$CustomWidget` node on every page, snippet and building block.

| Node | Path | Role |
|---|---|---|
| `CustomWidgets$WidgetType` | `<widget>.Type.ObjectType.PropertyTypes[]` | the widget's schema, one `CustomWidgets$WidgetPropertyType` per property, keyed by `PropertyKey` |
| `CustomWidgets$WidgetObject` | `<widget>.Object.Properties[]` | the instance's values, one `CustomWidgets$WidgetProperty` per property, bound to its type by `TypePointer` |

The three reconciliation operations, all already implemented in
`sdk/widgets/augment.go` for the *template*:

1. **Remove stale** — a `PropertyKey` present in the stored instance but absent from
   the installed `.mpk`. This is the #716 case (`advanced`). `removeProperties`
   deletes the `PropertyType` and its paired `WidgetProperty` together.
2. **Add missing** — a property the `.mpk` declares that the instance lacks.
   `clonePropertyPair` / `createPropertyPair` emit both halves with the `.mpk`
   default value.
3. **Update definition attributes** — a surviving property whose own attributes
   changed (`Required`, `OnChangeProperty`). `syncDefinitionAttrs`, added on
   `claude/gallery-dropdownfilter-ce0463-716`; **update-in-place only, never add a
   key the node does not already carry** (adding one is the #759 failure shape, and
   `update-widgets` output omits both keys on Gallery PropertyTypes).

**Pairing invariant.** A `WidgetProperty` is bound to its `WidgetPropertyType` by
`TypePointer`. Removing or adding one half without the other yields the
`StreamingBsonUnitReader` "does not contain a constructor with a parameter of type
WidgetValue" class of load failure. Every mutation must move both halves together.

**Nested object types.** `DataGrid2` columns, `Gallery` items and chart series nest
their own `PropertyTypes` inside an `IsList` object property. `augmentNestedObjectType`
already walks these; instance reconciliation must recurse the same way, per stored
list entry rather than once per widget.

## Proposed CLI

```bash
# Reconcile every stored widget instance against the installed .mpk set.
mxcli widget sync -p app.mpr

# Preview without writing — the default posture for a destructive-ish operation.
mxcli widget sync -p app.mpr --dry-run

# Narrow the blast radius.
mxcli widget sync -p app.mpr --widget com.mendix.widget.web.datagrid.Datagrid
mxcli widget sync -p app.mpr --page MyModule.Overview
```

Dry-run output names every instance and every property, because silently deleting a
stored property is exactly what a user needs to see before it happens:

```
MyModule.Transactions_Overview
  dgTransactions  (com.mendix.widget.web.datagrid.Datagrid 3.4.0 -> 3.11.3)
    - remove  advanced            (dropped in 3.10.0)
    + add     loadingType         (added in 3.7.0, default "spinner")
    ~ update  refreshInterval     Required false -> true

3 pages, 7 widget instances, 12 property changes. Re-run without --dry-run to apply.
```

**No new MDL statement.** `UPDATE WIDGETS SET … WHERE …` already exists and means
"set a property value I chose". Reconciliation is a maintenance operation on schema
the author does not choose; overloading that verb would conflate the two. A CLI
subcommand next to `widget init` is the honest home.

## Rejected alternative: just re-run `widget init` + `refresh catalog`

The obvious cheaper answer is that after installing an updated widget package you
re-run the normal extraction and refresh the catalog. **Measured on the upgraded
Ledger project — it changes nothing:**

```
before:                                              36 CE0463
  mxcli widget init      -> Extracted: 33 new, 0 refreshed, 9 skipped
  refresh catalog full force -> Catalog cached
after:                                               36 CE0463
```

Both commands operate on **derived artifacts**, not on the model:

| Artifact | Written by | Consumed by |
|---|---|---|
| `.mxcli/widgets/*.def.json` | `widget init` | **future** authoring — which MDL keyword routes to which property |
| `.mxcli/catalog.db` | `refresh catalog` | queries, lint rules, `show`/`select` |
| `mprcontents/<page>.mxunit` | page writers only | **mxbuild and Studio Pro — this is what CE0463 reads** |

Confirmed directly after running both commands against Data Widgets 3.11.3:

- the regenerated `datagrid.def.json` is **current** — it no longer mentions `advanced`
- the stored page BSON for `dgTransactions` **still carries** `PropertyKey: "advanced"`

Extraction succeeded; nothing rewrote the pages. Re-running `widget init` after an
upgrade is still *necessary* — it is what makes newly authored widgets match the new
package — but it is not *sufficient*, and the two halves should not be conflated.
That split is precisely why this proposal exists as a separate operation rather than
as a flag on `widget init`.

## Implementation Plan

The reconciliation logic exists; what is missing is applying it to **stored
instances** rather than to a freshly loaded template.

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/cmd_widget.go` | New `sync` subcommand: flags, dry-run rendering, summary |
| `mdl/executor/widget_sync.go` *(new)* | Orchestration: enumerate documents → locate CustomWidget nodes → diff vs `.mpk` → apply → write back |
| `sdk/widgets/augment.go` | Extract the add/remove/attr-sync core so it operates on an arbitrary `(PropertyTypes, Properties)` pair, not only on a `WidgetTemplate`. No behaviour change to the existing call. |
| `modelsdk/widgets/augment.go` | Same extraction — the engines carry independent copies |
| `mdl/backend/*/widget_*.go` | Backend method to enumerate + mutate widget subtrees in place (mutator pattern, per ADR-0002; no BSON in the executor) |
| `mdl/backend/mock/` | `Func`-field stub with the standard "not configured" default |

### Order of operations

1. **Read-only inventory first.** `--dry-run` reporting with zero mutation, validated
   against the #716 projects: it must name exactly the 7 authored Ledger widgets and
   the one `advanced` removal per DataGrid2.
2. **Extract the reconciliation core** from `AugmentTemplate` with the existing
   template tests still green — a pure refactor, committed separately.
3. **Apply to stored instances**, one document type at a time (pages, then snippets,
   then building blocks).
4. **Nested object types** last; they are the part most likely to break the pairing
   invariant.

## Version Compatibility

No version gate. The operation is driven by the installed `.mpk` set, not by the
Mendix version, and it is a no-op on a project whose widgets already match.

It must, however, be **correct on MPR v1 and v2**. `mx update-widgets` collapsing v2
`mprcontents/` into a v1 `Unit` table is the specific failure this command exists to
avoid; an integration test must assert `mprcontents/` survives.

## Test Plan

| Tier | Test |
|---|---|
| Unit | Reconciliation core: remove-stale, add-missing, attr-sync, and the pairing invariant (removing a `PropertyType` removes its `WidgetProperty`) |
| Unit | Nested object-type recursion — a DataGrid2 column list where the column schema changed |
| Integration | Author the v0.10 fixture against Data Widgets 3.4, upgrade the `.mpk` set to 3.11.3, run `widget sync`, assert `mx check` = 0 CE0463 |
| Integration | `mprcontents/` still present and the project still opens after a sync on MPR v2 |
| Integration | Idempotence — a second sync reports zero changes |
| Regression | A project already matching its `.mpk` is byte-identical after a sync |
| Fixture | `mdl-examples/bug-tests/716-widget-package-upgrade.mdl` |

**Measure against a control.** The doctype fixtures live in a blank project whose own
Studio-Pro-authored widgets (`dataGrid2_*`, `gallery1/2`, `drop_downFilter1/2`) also
fail after a package upgrade. A raw CE0463 count mixes them with mxcli's; subtract by
widget **name** against a project that ran no mxcli command. Getting this wrong
produced a confident wrong diagnosis during the #716 investigation.

## Open Questions

1. **Naming.** `widget sync` vs `widget update` vs `widget reconcile`. `update` reads
   best but sits one word away from the MDL `UPDATE WIDGETS SET`, which does something
   different. Leaning `sync` for that reason — worth a second opinion.
2. **Value preservation on a retyped property.** If a property survives but changes
   type (`string` → `enumeration`), is the stored value coerced, reset to the `.mpk`
   default, or does the sync refuse and report? Refusing is safest and matches
   ADR-0005 guard-don't-drop, but may be too strict to be useful. Needs a real
   example — none was observed in the 3.4 → 3.11.3 upgrade.
3. **Should `run --local` / `docker build` sync automatically?** Convenient, but an
   implicit model mutation during a build is exactly the kind of surprise this repo
   avoids elsewhere. Recommend explicit only, with the build *reporting* drift.
4. **The built-in definition override is NOT the lever — resolved, negatively.**
   `widget_defs.go` skips any widget with a hand-crafted def (gallery,
   dropdownsort, four filters), so those never see the project's `.mpk`. It looks
   like a prerequisite for this work. It is not, because **`.def.json` carries
   routing, not schema**: `GenerateDefJSON` emits `propertyMappings`,
   `childSlots` and `objectLists` — which MDL keyword feeds which property key.
   The schema CE0463 compares comes from the embedded template plus
   `augmentFromMPK`, a different pipeline.

   Measured twice: making extraction generate all 42 defs from the project's
   `.mpk` left the fresh-authoring CE0463 count at **6, unchanged**. Attempted
   properly (built-in keeping identity and routing, generated def supplying the
   rest) it **regressed** `17-custom-widget-examples` — CE7006 "Selected value is
   not valid for attribute 'title'" and CE7247 "Move this widget into a data
   container" on the `TEXTFILTER`, because the hand-authored routing encodes
   behaviour the generator cannot derive (e.g. the `attrChoice="linked"` rule
   from #605). Reverted.

   The override should probably still become a fallback for its own sake, but it
   is a separate concern with its own risk, and it buys this proposal nothing.
   Instance reconciliation must read the installed `.mpk` **directly**, the way
   `augmentFromMPK` does — not via the definition registry.
5. **Does this subsume `mx update-widgets` in CI?** If reconciliation is faithful, the
   doctype harness could drop its "we deliberately do NOT run update-widgets" note.
   Not a goal, but a good confidence signal if it turns out true.
