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
| **No security document ops** | PED's `ped_find/read/update/create_document` reject every security type (`Security$ModuleSecurity`, `Security$ProjectSecurity`, …) as "Unknown document type" — only `ped_get_schema` knows them as nested *elements*. Concord exposes only security *reads* (`audit_security`, `read_entity_access_rules`, `read_microflow_security`, `read_security_info`). So **security cannot be authored via MCP** (module/user roles, entity access rules, GRANT/REVOKE, demo users, project security level) — neither server has a write path. Determine support with `ped_read_document`, NOT `ped_find_document`: `find` also reports the (supported) nameless `DomainModels$DomainModel` as "Unknown", so it is not a reliable probe. | Watch for security write tools on either server. |

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

Implemented (11.11): `CREATE MODULE`, `CREATE/ALTER(add,drop attr)/DROP ENTITY`,
`CREATE/DROP ASSOCIATION`, `CREATE ENUMERATION`, `CREATE VIEW ENTITY`, `CREATE
MICROFLOW` (broad activity + control-flow coverage), `CREATE PAGE` + `ALTER PAGE`
(INSERT/DROP/REPLACE/SET property/DataSource/Layout), and `CREATE WORKFLOW` +
`DROP WORKFLOW`, with a dirty-set read router that makes in-session edits visible.

`CREATE WORKFLOW` maps the executor's workflow onto `Workflows$Workflow`
(parameter + a linear `Workflows$Flow` of activities Start … End, connected via
condition outcomes that may nest sub-flows). Activity-type coverage grows one at a
time (currently Start/End, CallMicroflow with outcomes, single **UserTask** —
task page, XPath/Microflow user targeting, task name/description, and named
outcomes each with a recursive sub-flow), and **Decision** (ExclusiveSplit: expression + boolean/enum outcomes); MultiUserTask, ParallelSplit,
JumpTo, WaitForTimer, boundary events, etc. and `ALTER WORKFLOW` are rejected with
a clear error. **Type-name gotcha:** PED's element type for the call-microflow
activity is `Workflows$CallMicroflowActivity`, NOT the on-disk BSON `$Type`
`Workflows$CallMicroflowTask` — they differ; use `ped_get_schema
Workflows$WorkflowActivity` to enumerate the valid concrete subtypes. Workflow
documents are addressed by qualified name (`<module>.<workflow>`), not a bare
name. Entity references into a session-created module (a workflow's context
entity, a microflow parameter) resolve because `ListDomainModels`/`GetDomainModel`
reconstruct the session module's live entities from PED.

`CREATE MODULE` routes through `ped_create_module` (which flushes to disk
immediately) and registers the module in a session list merged into
`ListModules`/`GetModule(ByName)`, so a later op in the same run — e.g. `create
module X; create enumeration X.Y`, or `create module X; create entity X.Foo` —
resolves the freshly created module (the local reader does not yet know about it).
For entities, `GetDomainModel` returns an empty **synthetic** domain model for a
session module (ID `mcp~dm~<module>`, which `moduleNameForDomainModel` decodes
back to the module name) since the reader has no on-disk domain model to read.
**Quirk:** a module created via `ped_create_module` lags briefly before
`ped_update_document` can mutate it (errors `Module ... not found` even though the
create flushed), so `pedUpdateDoc` retries on that transient with a short backoff. The standalone-doc create paths
(enumeration/page/microflow) resolve their module via `GetModule` (session-aware)
rather than the reader directly. Note `ped_create_module`'s success text is
`"Module 'X' created successfully."`, NOT the `SUCCESS`-prefix the document ops
use, so it has its own success check (contains "success").

**ALTER PAGE** is a read-modify-write on the pg tree: `OpenPageForMutation` loads
the page via `pg_read_page`, the mutator edits the in-memory tree, and `Save()`
writes it back via `pg_write_page`. Supported in-place ops: INSERT (before/after a
widget), DROP widget, REPLACE widget, SET DataSource, SET Layout, and **SET widget
property** (`set (prop = value, …) on <widget>`) — plus the introspection the
executor needs (FindWidget, WidgetScope, ParamScope, EnclosingEntity). The widget
ref is the widget name (recursive tree search). The executor passes the AST
position token (`"AFTER"`/`"BEFORE"`), so the mutator compares case-insensitively.
SET maps the MDL property name (also case-insensitive) to its pg key: Class/Style
→ the widget's `appearance`; Caption/Content/Label → the `ct:`-prefixed client
templates; ButtonStyle → pg's normalized enum; TabIndex/RenderMode/Editable/Name →
direct keys; Visible → a conditional-visibility expression. Not yet mapped: unknown
SET properties, column INSERT/REPLACE/property, design properties,
pluggable-property SET, and page variables — each returns a clear error.

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
  source's `xPathConstraint`. (Page datasources have no grouping concept.)

`sort`/`where` are supported wherever the **official metamodel** has a source
type that carries them (verified against the `modelsdk` branch's generated
types): `GridXPathSource` (= pg `CustomWidgetXPathSource`, DataGrid 2 / Gallery /
association ComboBox) and `ListViewXPathSource` (pg `Pages$ListViewXPathSource`,
list views with a database source) both have `xPathConstraint` + `sortBar`. A
**DataView**, by contrast, has *no* XPath source type — `DataViewSource` is
context/parameter-only (`pageParameter`/`snippetParameter`/`entityRef`) — so a
constraint/sort on a data-view database source is correctly rejected. This is a
metamodel fact, not a pg limitation: emit the right source `$Type` and pg expands
it. (List-view database sources must use `Pages$ListViewXPathSource`, NOT
`Pages$DataViewSource`, or pg drops the constraint/sort.)
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

## Concord (optional second client — gap-filler)

Some deployments run a second MCP server, **Concord** (a Studio Pro extension;
`concord-mcp`), alongside the built-in PED server. Concord is **not** an authoring
server — it has none of the `ped_*`/`pg_*` create tools — but it provides
operational/refactor capabilities PED lacks. The backend uses **PED for authoring
by default** and reaches for Concord **only** for these gaps. Configure it with
`--mcp-concord` / `--mcp-concord-dial` (a second `Client`, dialed independently);
it stays `nil` when not configured, and every Concord-backed op errors clearly if
it's missing.

Wired so far:
- **`delete_document`** — real `DROP` of standalone documents (enumeration,
  microflow, page), which PED cannot delete at all. `DROP ENUMERATION/MICROFLOW/
  PAGE` resolves the document's module + name and calls
  `delete_document {module_name, document_name}`. Unlike `save_all` this is
  **model-based, not keystroke automation**, so it is robust. (Entities and
  associations still delete via PED's array-element removal — no Concord needed.)
- **`check_model`** (`--mcp-check`) — domain-model consistency check after writes
  (PED has no validation for the live model). Parses `{success, healthy, summary
  {errorCount, warningCount, …}, errors[], warnings[]}` and prints a report to
  stderr on Disconnect. Model-based (robust). Note: `healthy: true` means zero
  *errors*, not zero warnings — the report shows both.
- **`save_all`** (`--mcp-save`) — PED has no save tool, so writes live only in
  Studio Pro's in-memory model until the user saves. `--mcp-save` flushes via
  Concord's `save_all` on Disconnect. **Unreliable — keystroke automation.**
  Concord's `save_all` synthesizes a macOS Cmd+S (osascript → System Events), with
  two failure modes observed against 11.11 Beta:
  1. **Permission.** Needs macOS **Accessibility** on the *responsible* process.
     If Studio Pro is launched from a shell that exec's the binary directly, the
     responsible process is the **terminal**, not Studio Pro — relaunch via
     `open -n -a "<app>" --args …` (launchd → app is its own responsible process)
     and grant Studio Pro Accessibility. Otherwise it fails `osascript is not
     allowed to send keystrokes (1002)`.
  2. **The keystroke does not save, and hangs Studio Pro — confirmed broken in
     11.11 Beta (2026-06-08).** First observed from a devcontainer
     (`{"status":"save_command_sent"}`, no disk change). Then re-tested the
     authoritative way — from Claude Code in the Concord terminal, **single active
     Studio Pro instance, app frontmost** — and it still did **not** persist *and*
     **hung Studio Pro** while Concord tried to drive the save. So it is **not**
     the two-instance ambiguity; it is a genuine **Concord/Studio Pro `save_all`
     bug** (the synthetic Cmd-S hangs the IDE). **Do not rely on `--mcp-save`;
     save manually (Cmd-S) in Studio Pro, and report `save_all` upstream to the
     Concord/Studio Pro team.** The backend wiring is correct and will work
     unchanged once `save_all` is fixed; a silently-no-op `save_command_sent` it
     cannot detect. Model-based gap-closers (`delete_document`, `check_model`) are
     unaffected — they do not use keystroke automation.

- **`get_app_status`** — the API call works and returns well-formed
  `{data:{running, runningUrl, projectName, …}}` (exposed as `GetAppStatus()`,
  printed by `--mcp-run`). **But `running`/`runningUrl` are effectively a
  port/process probe**, not the current session's console-managed runtime: it
  reported `running | :8080` while the Studio Pro runtime console was empty —
  because an **orphaned runtime from a previous run** was still bound to `:8080`
  (restarting Studio Pro doesn't kill the separate runtime process; `:8080` was
  confirmed listening). So trust the API shape, but treat `running: running` as
  "something is bound to the runtime port," which may be stale.
- **`run_app` / `stop_app`** (`--mcp-run` starts the app) — ⚠️ **same
  UI-automation failure as `save_all`.** They are "click the Run/Stop button"
  automations: `stop_app` returned `{"status":"command_sent"}` but the app stayed
  running across repeated `get_app_status` polls (2026-06-08, 11.11 Beta). So like
  `save_all` they report success without taking effect; `run_app` almost certainly
  behaves the same. Wired correctly (will work once Concord's UI automation is
  fixed), but **don't rely on `--mcp-run`/`stop_app` — start/stop the app manually
  in Studio Pro.** Report upstream.

**Pattern (important):** Concord's **model-editing** tools work (`delete_document`,
`check_model`); its **UI-automation** tools that synthesize button clicks /
keystrokes do **not** in this environment (`save_all`, `stop_app`, `run_app` all
return a `*_command_sent` status with no actual effect, and `save_all` can hang
Studio Pro). `get_app_status` is read-only and returns valid data but reflects raw
port state (can be a stale runtime). Net: only `delete_document` and
`check_model` are dependable today.

Candidate gap-closers not yet wired: `delete_model_element` (entity/attribute/
association — but PED already deletes these, so low priority; snake_case args
`{element_type, entity_name}`), `check_project_errors` (full-project validation —
currently stubbed in Concord), `refresh_project`. Concord identity captured
2026-06-08: `concord-mcp` (proto
`2025-03-26`), 44 tools, on port 7783 (directly container-reachable; no socat).

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
