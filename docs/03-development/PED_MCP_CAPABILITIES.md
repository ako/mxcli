# PED / MCP Tool Capabilities per Studio Pro Version

Studio Pro ships an embedded **MCP server** (the "PED" — Progressive Element
Disclosure — server, exposing Mendix's "Maia" agent tools) on a local HTTP
port. The mxcli **MCP backend** (`mdl/backend/mcp/`) is a client of this server:
it routes model writes through PED so Studio Pro stays the authoritative
serializer while the project stays open. See
[`PROPOSAL_mcp_backend.md`](../11-proposals/PROPOSAL_mcp_backend.md) for the why.

**The tool surface changes between Studio Pro versions.** Each release can add,
remove, or change tools — which directly expands or limits what the MCP backend
can do. This document is the canonical record of *which PED tools exist in which
Studio Pro version* and *what capability gaps each version has*. Update it
whenever a new Studio Pro version is onboarded (procedure at the bottom).

This is a developer reference. It is the sibling of
[`WIDGET_BSON_VERSION_COMPATIBILITY.md`](WIDGET_BSON_VERSION_COMPATIBILITY.md)
for the MCP transport instead of on-disk BSON.

> **Provenance.** Every row below was captured live with `cmd/mcpprobe` against a
> running Studio Pro, not copied from documentation. Raw fixtures live in
> `mdl/backend/mcp/testdata/` (`tools.json`, `schema-entity.json`). Re-capture
> per the onboarding procedure; do not hand-edit claims you have not verified.

## Server identity

| Studio Pro | MCP server present | `serverInfo` | MCP protocol | Captured |
|------------|--------------------|--------------|--------------|----------|
| ≤ 11.10    | **No**             | —            | —            | —        |
| 11.11      | Yes                | `mendix-studio-pro` 1.0.0 | `2025-06-18` | 2026-06-05 |

`serverInfo.version` is the MCP server's own version, distinct from the Studio
Pro version. The MCP server first appears in **11.11**; earlier versions have no
endpoint and the backend must fail with an actionable error on connect.

## Tool matrix

A tool's cell is the first Studio Pro version it was observed in (✓ = present,
blank = absent, `—` = n/a). `cmd/mcpprobe -method tools/list` is the source.

| Tool | 11.11 | Purpose / used by the backend |
|------|:-----:|-------------------------------|
| `ped_get_schema` | ✓ | Schemas for element types (`$constructor` for create, `$element` for read). Backend: fetched before create/add. |
| `ped_find_document` | ✓ | Find docs by module + type. **Must NOT be used for `DomainModels$DomainModel`** (always exists, nameless). |
| `ped_read_document` | ✓ | Progressive read; JSON-pointer `paths` to descend. Backend: dirty-set read reconstruction. |
| `ped_list_folder` | ✓ | Immediate contents of a module/folder. (Not yet used.) |
| `ped_create_module` | ✓ | Create a module (+ its domain model). Flushes to disk immediately. |
| `ped_create_document` | ✓ | Create standalone documents (enum, microflow, page, …). "Never create domain models." Backend: `CreateEnumeration`. |
| `ped_update_document` | ✓ | Operation-based set/add/remove at JSON paths. Backend: entities, attributes, associations (incl. removes). |
| `ped_check_errors` | ✓ | Validate documents (run after the final write). Backend: after every write. |
| `pg_read_page` | ✓ | **Pages only** — separate read path. (Not yet used.) |
| `pg_write_page` | ✓ | **Pages only** — separate write path; PED is *forbidden* for pages. (Not yet used.) |
| `oql_generate` | ✓ | NL → OQL for a module. (Agent helper; not used.) |
| `search_mendix_knowledge_base` | ✓ | Docs/KB search. (Agent helper; not used.) |
| `read_skill` | ✓ | Load a Maia skill. (Agent helper; not used.) |
| `glob` | ✓ | List files in a virtual file domain. (Agent helper; not used.) |
| `read_file` | ✓ | Read a file in a virtual file domain. (Agent helper; not used.) |
| `write_file` | ✓ | Write Java/JS/CSS in a virtual file domain. (Potential alt to mounted-fs writes.) |

`initialize` instructs clients to first read the resource
`mendix://studio-pro/system-prompt` (the Maia system prompt + PED contract).

## Capability gaps (11.11)

These are the *absences* that bound what the backend can do. They are as
important as the tools that exist — re-check each when onboarding a new version,
since a gap closing (e.g. a delete or save tool appearing) unlocks features.

| Gap | Consequence for the backend | Status to recheck each version |
|-----|-----------------------------|-------------------------------|
| **No delete-document tool** | `DROP` of any *standalone document* (enum, microflow, page) is impossible. Only entities/associations delete, via a `ped_update_document` remove op on the domain-model array. Test docs created via MCP cannot be cleaned up — they persist until the user closes Studio Pro without saving. | Watch for a `ped_delete_document` / equivalent. |
| **No list-modules tool** | PED cannot enumerate modules. The backend must read modules/structure from the local mounted `.mpr` (hybrid model); `-p` must be the same project Studio Pro has open. | Watch for a modules-list tool. |
| **No save/flush tool** | `ped_update_document` edits stay in Studio Pro's in-memory model; the on-disk `.mpr` is stale until the user saves. Drives the dirty-set read router. (`ped_create_module` is the one op observed to flush immediately.) | Watch for a save tool / autosave. |
| **Reads omit `$ID`** | Reads expose `$QualifiedName` only. Association refs need entity GUIDs, recovered from the local reader (= live `$ID` for saved entities). Reconstructed reads use synthetic IDs mapped to names. | Recheck whether reads expose `$ID`. |
| **Array reads omit primitive types** | A `/entities/N/attributes` read gives attribute names (`$QualifiedName`) but not their primitive type — only a per-attribute deep read does. Reconstructed attributes use placeholder types. | Recheck attribute-array read shape. |
| **Two write protocols** | Pages **must** use `pg_*`; everything else uses `ped_*`. The system prompt forbids PED for pages. | Stable, but reconfirm. |

## Transport (per environment, not per version)

The server binds **IPv6 loopback only** (`[::1]:7782` observed) and enforces a
DNS-rebinding guard: the `/mcp` route requires HTTP `Host: localhost` (bare, no
port). From a devcontainer:

- Some sessions are reachable directly at `host.docker.internal:7782`.
- Otherwise bridge on the **host**: `socat TCP4-LISTEN:7783,reuseaddr,fork 'TCP6:[::1]:7782'`, then dial `host.docker.internal:7783`.

`cmd/mcpprobe` and the backend client pin the dial target while keeping the
`Host` header `localhost` (`-url http://localhost/mcp -dial host.docker.internal:<port>`).
The port can change between Studio Pro sessions — confirm with `lsof` on the host.

## How the mxcli MCP backend uses this surface

Implemented (11.11): `CREATE/ALTER(add,drop attr)/DROP ENTITY`, `CREATE/DROP
ASSOCIATION`, `CREATE ENUMERATION`, `CREATE VIEW ENTITY`, `CREATE MICROFLOW`
(broad activity + control-flow coverage), and `CREATE PAGE` (foundation), with a
dirty-set read router that makes in-session edits visible.

Pages use a **separate protocol**: `pg_write_page` / `pg_read_page` (PED is
forbidden for pages). The backend maps the executor's `pages.Page` (shell +
LayoutCall slots + widget tree) onto the high-level page content. Note pg's
container type is **`Pages$DivContainer`** (not `Container`); page reads
(layouts/snippets/folders) delegate to the local reader because the executor
resolves the layout through the container hierarchy. Validation success is
signalled by a result text containing "success" (not the PED "SUCCESS"-prefix
convention), and — unlike PED — there is **no pg validation tool**, so a bad
attribute/page reference still writes "successfully" but shows a CE error in
Studio Pro.

Widgets so far: DivContainer, LayoutGrid/Row/Column, TabContainer/TabPage,
ActionButton, DynamicText, DataView, ListView, TextBox, TextArea, CheckBox,
RadioButtonGroup, DatePicker (+ No/Microflow/Page/CreateObject client actions;
page-variable / direct-entity / database / microflow data sources). Button styles
are normalized to pg's canonical enum (`primary` → `Primary`); an unknown style
falls back to `Default` (pg rejects unknown values). A DataGrid 2 control bar is
just the `filtersPlaceholder` slot holding action buttons. TextArea and the executor's
RadioButtons (→ `Pages$RadioButtonGroup`) are attribute-bound inputs that share
the same minimal `attributeRef` + `ct:labelTemplate` shape as TextBox; the server
fills in the rest of their defaults (rows, render direction, placeholder, …),
which are not yet mapped. **Conditional visibility** (`visible: [xpath]`, i.e.
VISIBLE IF) maps onto a `Pages$ConditionalVisibilitySettings {expression}` and
is attached uniformly to every mapped widget; the MDL `visible:` property only
ever produces an expression, so module-role / attribute / source-variable
conditions are not mapped. (The static `visible: false` form sets a separate
`Visible` value, not conditional visibility, and is not yet mapped.) Tab-page
captions use the `t:caption` key (a plain
string the server wraps in `Texts$Text`), not the `ct:` ClientTemplate prefix
that button captions use. pg's widget
union (from the tool schema) is the limit of native support: ActionButton,
CheckBox, Content, DataView, DatePicker, DivContainer, DynamicText, LayoutGrid/
Row/Column, ListView, RadioButtonGroup, TabContainer/TabPage, TextArea, TextBox,
plus `CustomWidgets$CustomWidget` (pluggable). **No `Pages$DataGrid`** — the
legacy DataGrid is rejected; DataGrid 2 is a pluggable custom widget. Coverage
grows one widget/data-source type at a time.

**Pluggable widgets (ComboBox, DataGrid 2).** The reference/dropdown selector — the Mendix 11
ComboBox (`com.mendix.widget.web.combobox.Combobox`) — is supported in both
enumeration and association modes. Crucially, the MCP path does *not* build the
BSON widget template the MPR writer must: it implements `LoadWidgetTemplate` with
an `mcpWidgetBuilder` that records the engine's semantic property operations
(`SetAttribute`/`SetAssociation`/`SetPrimitive`/`SetDataSource`) into a high-level
pg `object`, and Studio Pro expands every default on `pg_write_page` (37 props
filled from ~5 for ComboBox; 34 object + 19/column for DataGrid 2). **This
sidesteps the entire CE0463 "widget definition changed" template-mismatch class
of bugs** that the on-disk BSON writer hits, because the server owns
serialization. One ComboBox quirk: the def.json enum mode maps only
`attributeEnumeration` (the MPR template carries `optionsSourceType`'s default),
so the MCP backend infers `optionsSourceType: "enumeration"` — otherwise pg
defaults it to `association` and prunes the enum binding.

Supported pluggable widgets: **ComboBox**, **DataGrid 2**, and **Gallery**. Which
ones are supported, and each widget's DataSource property, are declared in
`mdl/backend/mcp/widgets.def.json` — an MCP-owned capability registry
**deliberately not** added to the shared widget registry, so it cannot change the
MPR datagrid path (which is being replaced by the new engine). The builder
translates the shared engine's storage-agnostic calls:
- `SetDataSource` → `CustomWidgets$CustomWidgetXPathSource` (DataGrid 2 reaches it
  via auto-datasource, which reads the DataSource property `PropertyTypeIDs`
  reports from the def; ComboBox/Gallery map it explicitly in their shared
  def.json). A `sort by` clause becomes the `Pages$GridSortBar` (`sortItems` with
  `attributeRef` + `sortDirection`), and a `where [...]` clause becomes the
  source's `xPathConstraint`. (Page datasources have no grouping concept.) Both
  are supported only on the pluggable `CustomWidgetXPathSource` (DataGrid 2 /
  Gallery / association ComboBox); pg's `Pages$DataViewSource` (DataView /
  ListView) has no such fields and silently drops them, so a constraint/sort on a
  data-view/list-view database source is rejected with a clear error rather than
  written and lost.
- `SetObjectList` → generic object-list items (DataGrid 2 `columns`): operation
  kind → pg shape, text-template keys take pg's `ct:` prefix.
- `SetChildWidgets` → Widgets-typed slots (Gallery `content` template), mapped
  recursively through the normal widget mapper so nested pluggable widgets and
  conditional visibility work inside a slot.
- An object-list item's own Widgets-typed sub-slots are mapped recursively too,
  which gives **DataGrid 2 column filters** (`textfilter`/`numberfilter`/
  `datefilter`/`dropdownfilter` → the column's `filter` slot) and custom-content
  cells (the column's `content` slot). The filter widgets are added to
  `widgets.def.json`. Their def.json always sets `attrChoice: "auto"`, under which
  Studio Pro auto-binds the filter to the column attribute and rejects a non-empty
  `attributes` list — so `SetAttributeObjects` is a deliberate no-op (emitting the
  derived attribute would drop the widget).

Client templates with `{N}` parameters (common in Gallery/DataGrid cells) emit a
full `Pages$ClientTemplate` with `attributeRef`/`expression`/`sourceVariable`
parameters — otherwise the literal `{1}` would show. DataGrid 2 columns with
custom-content child widgets or parameterised header templates, pluggable widgets
not in the registry, and any property op the builder doesn't translate are
rejected, not silently emitted with missing content. The broader consolidation
(removing the hardcoded Go maps in `widget_defs.go` and migrating the MPR path to
def.json) is deferred until after the engine replacement merges.

Data sources for DataView/ListView: page-variable (`Pages$PageVariable`),
direct-entity (`DomainModels$DirectEntityRef`), and **microflow**
(`Pages$MicroflowSource` wrapping `Pages$MicroflowSettings {microflow,
parameterMappings:[], outputMappings:[], progressBar:"None", asynchronous:false,
formValidations:"All"}`). Microflow sources with parameter mappings, and
database sources with XPath/sorting, are not yet mapped.

Microflow support is now broad: name, parameters, return type, and a recursive
object/flow graph (positions reused from the executor's layout engine, so the
MCP-authored canvas matches the file-written one). Supported activities:
declare/set variable; create/change/commit/delete/retrieve/rollback object;
create list, change list, aggregate, list operations (head/tail, filter/find by
expression or attribute, sort, union/intersect/subtract); show message; log;
call microflow / nanoflow / java action; download file; close page; validation
feedback. Control flow: if/else ExclusiveSplit + ExclusiveMerge, for-each/while
LoopedActivity + break/continue. Rejected (PED can't express them faithfully):
show page (constructor omits the page ref — pages are pg_*), cast, inheritance
splits, rule-split conditions, contains/equals/range list ops, queue settings.

View-entity choreography (verified): `ped_create_document
DomainModels$ViewEntitySourceDocument {name}` → `ped_update_document` set
`/oql` → entity add with `source: {OqlViewEntitySource, sourceDocument:
"<qualified>"}` and each attribute carrying `value: {OqlViewValue, reference:
<column>}` (without the OqlViewValue the entity is "out of sync", CE-6770). The
source document's name must equal the view entity's name. Because there is no
delete-document tool, dropping a view entity removes the entity but orphans its
source document, and `CREATE OR REPLACE` of an existing view entity fails at
the duplicate source-document create. See `mdl/backend/mcp/` and the
[proposal](../11-proposals/PROPOSAL_mcp_backend.md). Operations outside the slice
return a clear "not supported by the MCP backend" error via the generated
`unsupportedBackend` base.

## Onboarding a new Studio Pro version

1. Open a project in the new Studio Pro; establish transport (direct or socat).
2. `go run ./cmd/mcpprobe -url http://localhost/mcp -dial host.docker.internal:<port> -method tools/list`
   → save to `mdl/backend/mcp/testdata/tools-<version>.json`.
3. **Diff against the previous `tools.json`** — added/removed/renamed tools.
4. Update the **server identity** and **tool matrix** tables above (new column).
5. Re-run the **capability gaps** checks — especially delete / save / modules /
   `$ID` exposure. Any gap that closed is a feature to build; note it here and
   in `docs/11-proposals/PROPOSAL_mcp_backend.md`.
6. Re-capture changed schemas (`ped_get_schema`) for doctypes the backend uses;
   refresh `testdata/` fixtures and any affected tests.
7. If a tool's input schema changed, update the backend call sites and the
   `version-awareness` skill if a workaround is needed for older versions.
