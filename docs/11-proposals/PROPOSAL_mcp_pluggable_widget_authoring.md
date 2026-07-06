# Proposal: Pluggable Widget Authoring in MDL over the MCP Backend

**Status:** Draft — Phases 1 and 2 implemented and live-validated against Studio Pro 11.12 (2026-07-06, see the validation-results sections)
**Date:** 2026-06-30
**Related:** [`PROPOSAL_mcp_backend.md`](PROPOSAL_mcp_backend.md), [`PROPOSAL_v0_12_0_widget_consolidation.md`](PROPOSAL_v0_12_0_widget_consolidation.md), [`PROPOSAL_multi_version_pluggable_widgets.md`](PROPOSAL_multi_version_pluggable_widgets.md)

## Problem Statement

The MCP backend (`mdl/backend/mcp/`) can author pages into a running Studio Pro
(11.11+) over the embedded MCP server, using `pg_patch_page` (11.12+). But its
pluggable-widget coverage is capped at **seven hardcoded widgets** — ComboBox,
DataGrid 2, Gallery, and the four DataGrid column filters — declared in a private,
compile-time-embedded `mdl/backend/mcp/widgets.def.json`. Any other pluggable
widget is **rejected** (`mapCustomWidget`, `widget.go:60-65`).

Meanwhile mxcli *already* has a generic, `.def.json`-driven pluggable-widget
pipeline for the on-disk (MPR) path:

- `mxcli widget extract --mpk` parses a widget `.mpk` and auto-infers a
  `WidgetDefinition` with `propertyMappings` + an `mdlName` keyword
  (`cmd/mxcli/cmd_widget.go`).
- The generic MDL syntax `KEYWORD name ( key: value, … ) { template … }` is
  parsed into an `ast.WidgetV3`.
- `PluggableWidgetEngine` (`mdl/executor/widget_engine.go`) resolves the
  definition from the registry (project `.mxcli/widgets/*.def.json` → global →
  embedded) and drives a **backend-agnostic** `backend.WidgetObjectBuilder`
  via named operations (`applyOperation`).

The MCP backend already returns a `WidgetObjectBuilder` (`mcpWidgetBuilder`) that
implements most of those operations. **The only reason an installed third-party
widget can't be authored over MCP is that `mcpWidgetBuilder` consults its private
7-entry whitelist instead of the shared registry the engine already resolved.**

Maia Make in Studio Pro 11.12 advertises "all widget types incl. pluggable." This
proposal closes most of that gap for mxcli + MCP **without inventing new MDL
syntax** and **without writing widget BSON** — because over `pg_patch_page`,
Studio Pro owns serialization and expands defaults.

## BSON Structure

**This feature does not write Mendix BSON, by design.** The whole reason the MCP
path avoids the CE0463 "widget definition changed" class of bugs is that it never
constructs the Type+Object BSON template the MPR writer must. Instead,
`mcpWidgetBuilder` records the engine's semantic `Set*` calls into a high-level pg
`object` map, and Studio Pro expands every default when `pg_patch_page` is applied
(verified for ComboBox: ~5 props in → 37 expanded; DataGrid 2: ~34 object + 19/col).

Consequence for discovery: the MCP path needs **only** the `.def.json`
`propertyMappings` + `widgetId` from `mxcli widget extract`. It does **not** need
the hand-extracted template JSON (`.mxcli/widgets/<name>.json`) that the MPR path
requires — the harder half of widget onboarding is unnecessary for MCP.

The artifacts that *do* need verification are the **pg `object` shapes** (plain
JSON the tool accepts, not BSON `$Type` documents). Today's verified shapes:

| Operation | pg `object` shape (MCP) | Status |
|-----------|-------------------------|--------|
| `SetAttribute` | `{ $Type: DomainModels$AttributeRef, attribute: <path> }` | ✅ implemented |
| `SetAssociation` | `{ $Type: DomainModels$IndirectEntityRef, steps: [EntityRefStep] }` | ✅ implemented |
| `SetPrimitive` | scalar value | ✅ implemented |
| `SetSelection` | selection enum string | ✅ implemented |
| `SetDataSource` | `CustomWidgets$CustomWidgetXPathSource` (+ `xPathConstraint`, `GridSortBar`) | ✅ implemented |
| `SetChildWidgets` | recursive widget array (Widgets-typed slot) | ✅ implemented |
| `SetObjectList` | object-list items (DataGrid 2 `columns`) | ✅ implemented |
| `SetExpression` | **TBD — pg shape unverified** | ⛔ stubbed → reject |
| `SetTextTemplate` / `…WithParams` | **TBD — pg shape unverified** | ⛔ stubbed → reject |
| `SetAction` | `Pages$*ClientAction` (page/microflow/no-op already used elsewhere) | ⛔ stubbed → reject |

The stubbed three are recorded in `unsupported` and the widget is **rejected
loudly** — no silent drop (consistent with the CE0463 / "don't fake ops"
discipline).

## Proposed MDL Syntax

**Phase 1 introduces no new MDL syntax.** It reuses the existing generic
pluggable-widget statement, so it is automatically within
[ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md):

```sql
-- 'MYWIDGET' is the mdlName from .mxcli/widgets/mywidget.def.json
-- (produced by `mxcli widget extract --mpk widgets/MyWidget.mpk`)
MYWIDGET myWidget1 ( datasource: database Module.Entity, attribute: Name ) {
  template content1 {
    dynamictext label1 ( content: '{1}', contentparams: [{1} = Name] )
  }
}
```

The same statement already works on the MPR path; this proposal makes it work on
the MCP path too. Adding a property is a one-line diff inside the `( … )` bag.

**Phase 2** adds no statement-level syntax either — it widens which `( key: value )`
property *kinds* the MCP builder can emit (`expression:`, `texttemplate:`,
`action:`), reusing the existing colon-separated property format.

## Implementation Plan

### Phase 1 — point the MCP backend at the shared registry (small, high-leverage)

The engine has already resolved a `WidgetDefinition` before it calls
`LoadWidgetTemplate`, so by the time operations reach `mcpWidgetBuilder` the widget
is, by construction, registered. Two private lookups must stop being private:

1. **Whitelist** — `mapCustomWidget` rejects any `widgetId` not in the embedded
   `mcpWidgetDefs`. Replace with "accept any widget the engine resolved a
   definition for." Cleanest wiring: pass the resolved `WidgetDefinition` (or its
   `WidgetID` + datasource property keys) into the builder rather than re-reading
   disk — `LoadWidgetTemplate` currently *ignores* its `projectPath` arg
   (`widget.go:134`), so the plumbing point already exists.
2. **`dataSourceProperties`** — derive from the resolved def's `propertyMappings`
   where `operation == "datasource"`, instead of from the private JSON.

Net effect: every widget whose `.def.json` maps only to the already-implemented
operations (attribute / association / primitive / selection / datasource / widgets
/ object-list) becomes authorable over MCP — exactly the widgets `mxcli widget
extract` can auto-infer.

### Phase 2 — implement the already-declared, MCP-stubbed operations

The `backend.WidgetObjectBuilder` interface (`mdl/backend/mutation.go:291`) and the
engine's `applyOperation` (`widget_engine.go:434`) **already** support
`expression`, `texttemplate`, and `action`. Only the *MCP* implementations are
stubs (`note(...)` → reject). Phase 2 implements them by mapping each to its pg
`object` shape, verifying the shape live before enabling (see Test Plan).

A genuinely-new tier (`object`, `icon`, `image`, `file` property kinds) has **no**
operation today — those would need a new interface method + engine dispatch + both
backends + extract auto-inference. Out of scope here; track separately.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/backend/mcp/widget.go` | Phase 1: drop private-whitelist gate in `mapCustomWidget`; source `dataSourceProperties` from the resolved def. Phase 2: implement `SetExpression`/`SetTextTemplate`/`SetTextTemplateWithParams`/`SetAction` as pg `object` shapes (replace `note(...)`). |
| `mdl/backend/mcp/widgets.def.json` | Phase 1: demote to fallback (or remove) once the shared registry is consulted; keep only MCP-specific quirks (e.g. ComboBox enum-mode inference). |
| `mdl/executor/widget_engine.go` | Phase 1: thread the resolved `WidgetDefinition`/datasource keys to `LoadWidgetTemplate` (or a new builder-init call) so MCP doesn't need the private list. |
| `mdl/backend/mcp/widget_test.go` | Update the "unlisted widget rejected" test (`BarcodeScanner`) to reflect registry-driven acceptance; add an accept-from-registry test. |
| `cmd/mxcli/cmd_widget.go` (extract) | Phase 2: extend auto-inference to emit `expression`/`texttemplate`/`action` mappings (currently skipped). |
| `.claude/skills/mendix/custom-widgets.md` | Document MCP support + the "no template JSON needed over MCP" distinction. |
| `docs/03-development/PED_MCP_CAPABILITIES.md` | Update the pluggable-widget capability note (no longer "3 curated"). |
| `mdl-examples/doctype-tests/` | Add an MCP-path third-party-widget example (see Test Plan). |

### No-change (intentionally)

- **No grammar / AST / visitor changes** — generic `ast.WidgetV3` already parses
  the syntax.
- **No BSON writer changes** — MCP authoring goes through `pg_patch_page`.
- **No MPR-path changes** — the private `widgets.def.json` was deliberately kept
  out of the shared registry to avoid perturbing the MPR datagrid path during the
  engine replacement; Phase 1 must preserve that isolation for MPR while letting
  MCP read the shared definitions.

## Version Compatibility

- **MCP transport**: Studio Pro **11.11+** (MCP server first appears in 11.11).
- **Page patching**: `pg_patch_page` is **11.12+** (it replaced `pg_write_page`;
  see the 11.12 delta in `PED_MCP_CAPABILITIES.md`). Pluggable-widget authoring
  over MCP therefore effectively requires **11.12+**.
- **Gating note**: the MCP `serverInfo.version` is frozen at `1.0.0` across 11.11
  and 11.12, so any version gate must key on the **project** Mendix version, not
  the server version (same rule the 11.12 attribute-default + navigation features
  already follow).
- Which widgets are installed is a **runtime** fact (read from the project's
  `.mpk`s / resolved registry), not a version gate.

## Test Plan

1. **Live verification (mandatory before enabling each operation)** — per the
   don't-guess-BSON discipline, the design bet is that `pg_patch_page` accepts an
   arbitrary `widgetId` + sparse `object` and expands defaults. Proven for
   ComboBox/DataGrid 2/Gallery; **must be confirmed on at least one third-party
   widget** end-to-end: `mxcli widget extract` → minimal MDL → author over MCP →
   `mx check` clean. Repeat per Phase-2 operation (`expression`/`texttemplate`/
   `action`) to pin the pg shape.
2. **`mdl-examples/doctype-tests/`** — add `NN-mcp-pluggable-thirdparty.mdl`
   authoring a non-builtin widget via the generic syntax; runs green over the MCP
   backend on a live 11.12 instance.
3. **Unit** — `mdl/backend/mcp/widget_test.go`: registry-driven acceptance (a
   widget present in the resolved registry is built, not rejected); honest
   rejection for an operation kind still unimplemented on MCP.
4. **Isolation regression** — confirm the MPR datagrid path is byte-unchanged
   (the private→shared registry move must not leak into MPR): run the existing
   widget doctype-tests (29/30/31/32) on the MPR engine and diff `mx check`.

## Phase 1 validation results (2026-07-06, Studio Pro 11.12.0, test8-app)

Live end-to-end run against a running Studio Pro 11.12 (`mendix-studio-pro` MCP
1.0.0, protocol 2025-06-18, `pg_patch_page` toolset) with the Phase 1 change
(registry-driven acceptance, whitelist removed):

1. **Baseline (regression)** — `create enumeration` + `alter entity add attribute`
   + `create page` with a whitelisted ComboBox over MCP: page created,
   `pg_read_page` shows `attributeEnumeration` → the new enum attribute with all
   defaults expanded by Studio Pro, `ped_check_errors` clean.
2. **Phase 1 target** — BarcodeScanner
   (`com.mendix.widget.web.barcodescanner.BarcodeScanner`, registry-resolved,
   **outside** the old 7-entry whitelist) authored via the generic
   `pluggablewidget '<id>' name (attribute: Name)` syntax: widget created with
   `object.datasource` → `AttributeRef MyFirstModule.CE0488Thing.Name`; Studio Pro
   expanded all remaining defaults (`showMask`, `useAllFormats`, sizing,
   `detectionLogic`); `ped_check_errors`: **no errors, no CE0463**.
3. **Negative (reject-loudly)** — Image widget
   (`com.mendix.widget.web.image.Image`, def maps `texttemplate`/`action` ops):
   rejected client-side before any tool call with
   `uses properties not yet supported by the MCP backend: [textTemplate:imageUrl
   textTemplate:alternativeText action:onClick]`, exit 1, no partial page in
   Studio Pro.

Observations from the run (follow-ups, not blockers):

- **Studio Pro 11.12 persists PED/pg-created documents to `mprcontents/`
  immediately** (no explicit save needed) — differs from the documented 11.11
  Concord `--mcp-save` requirement; update `PED_MCP_CAPABILITIES.md`.
- **`pg_patch_page` can return MCP `-32000 Request timed out` while the patch
  still applies** — mxcli reported failure for a create that succeeded. Client
  timeout/retry-read handling needed.
- **`--mcp-trace` regression** — prints the `▸` MDL command lines but no PED
  tool-call lines beneath them; tracer isn't reaching the MCP client.
- Docs: `MDL_QUICK_REFERENCE.md` shows `alter entity … add (attr: type)`, which
  does not parse; the working form is `add attribute attr: type`.

## Phase 2 validation results (2026-07-06, Studio Pro 11.12.0, test8-app)

The pg shapes for all three previously-stubbed operations were captured live
(server-published sources: the `page-gen-common` skill + its
`references/common-objects.md` / `references/actions.md`, plus the per-widget
schemas Studio Pro exposes at `/pagegen/customWidgetsVFS/*.schema.json`) and
then pinned empirically with direct `pg_patch_page` probes before enabling:

| Operation | pg shape inside a custom widget's `object` | Probe |
|---|---|---|
| texttemplate | `"ct:<key>": "plain string"` — Studio Pro expands to a full `Pages$ClientTemplate` | accepted, check clean |
| texttemplate (params) | `ct:<key>` = `Pages$ClientTemplate` with `t:template: "… {1} …"` + `Pages$ClientTemplateParameter` `attributeRef`s | attributeRef persisted, check clean |
| expression | `"<key>": "expr string"` | accepted, check clean |
| action | documented `Pages$*ClientAction` shapes; **deviation**: `MicroflowClientAction` must nest the reference in `microflowSettings` — a flat `microflow` key (as the native LightPage constructor accepts) is *silently dropped*, yielding "Select a microflow" | flat → dropped; nested → clean |

Implementation (`mdl/backend/mcp/widget.go`): `SetExpression`,
`SetTextTemplate`, `SetTextTemplateWithParams` (with `{AttrName}` → `{1}` +
attributeRef rewriting resolved against the entity context), and `SetAction`
via a new `customWidgetClientAction` mapper (NoClientAction /
MicroflowClientAction / PageClientAction). Actions with parameter mappings and
other action kinds are still rejected loudly — their pg value shapes
(PageVariable-typed `variable`, fully-qualified `parameter` names) are not yet
pinned live.

One shared-engine fix rode along: `applyOperation`'s `texttemplate` case now
routes text containing `{AttrName}` placeholders through
`SetTextTemplateWithParams` (placeholder-less text keeps the byte-identical
`SetTextTemplate` path). Previously the braces were emitted literally, which
Studio Pro's translatable-text parser rejects ("Brace should be followed by a
number of digits") — on the MCP *and* MPR paths.

End-to-end: an Image widget with `ImageType: 'imageUrl'`, `imageUrl:`,
`alternativeText: '… {Name}'`, and `onClick: microflow …` authored over MCP →
`ped_check_errors` clean, read-back shows the URL template, the `{1}` +
`attributeRef MyFirstModule.CE0488Thing.Name` parameter, and the nested
microflow action. The same script executed on the modelsdk engine against a
local copy passes `mx check` (11.12 mxbuild) with zero errors from the page.

Additional run observations:

- The Image widget requires `ImageType: 'imageUrl'` alongside `imageUrl:` —
  with the def-default `datasource: "image"`, Studio Pro prunes `ct:imageUrl`
  from the patch (same class of pruning as the ComboBox `optionsSourceType`
  quirk).
- Refinement of the Phase 1 persistence observation: Studio Pro 11.12 flushes
  PED-**created** documents to `mprcontents/` immediately, but
  `ped_update_document` changes (e.g. `alter entity add attribute`) stay
  in-memory until save — an on-disk copy taken mid-session can reference
  attributes that don't exist on disk yet (CE1613 on `mx check`).

## Open Questions

1. ~~**pg shape for `expression` / `texttemplate`** — unverified.~~ **Resolved
   2026-07-06** — captured live and pinned; see "Phase 2 validation results".
   Still open within actions: parameter-mapping value shapes
   (PageVariable-typed `variable`, fully-qualified `parameter`) and the
   remaining action kinds (save/cancel/close/delete/create/open-link/nanoflow).
2. **Registry plumbing** — pass the resolved `WidgetDefinition` into
   `LoadWidgetTemplate`, or give the MCP backend read access to the same registry
   resolver the engine uses? The former keeps MCP from re-reading disk and reuses
   the engine's resolution order; preferred unless it complicates the
   `WidgetObjectBuilder` contract for other backends.
3. **Relationship to `PROPOSAL_v0_12_0_widget_consolidation.md`** — that proposal
   folds `widgets.def.json` into the shared definitions *after* the engine merge.
   Phase 1 here is adjacent (MCP-side consumption) and could be sequenced before or
   alongside it; confirm we're not duplicating the consolidation work.
4. **Required-but-unmapped properties** — a widget may declare required properties
   the extract tool skipped. Over MCP, Studio Pro fills defaults — but does it
   reject when a *required* property has no default? Verify; if so, surface an
   actionable error naming the missing property rather than a raw tool error.
