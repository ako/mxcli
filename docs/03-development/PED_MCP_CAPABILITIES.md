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
ASSOCIATION`, `CREATE ENUMERATION`, `CREATE VIEW ENTITY`, and `CREATE MICROFLOW`
(shell + return only), with a dirty-set read router that makes in-session edits
visible.

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
