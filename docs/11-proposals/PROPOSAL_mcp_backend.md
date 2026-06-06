---
title: MCP Backend — execute MDL against a live Studio Pro via its MCP server
status: draft
date: 2026-05-29
author: Generated with Claude Code
related:
  - PROPOSAL_mcp_bson_benchmark
  - PROPOSAL_concurrent_access
---

# Proposal: MCP Backend — execute MDL against a live Studio Pro

**Status:** Draft
**Date:** 2026-05-29
**Author:** Generated with Claude Code

## Problem Statement

Today `mxcli` writes model changes by editing the `.mpr`/`mprcontents` files
directly on disk. This forces an awkward constraint: **Studio Pro must be
closed** while mxcli works, because two processes editing the same files race
(see [`PROPOSAL_concurrent_access`](PROPOSAL_concurrent_access.md)). It also
makes mxcli solely responsible for BSON fidelity — every `$Type` storage name,
default value, and pointer-inversion gotcha must be reproduced exactly, or the
project fails to open with `CE0463` / `TypeCacheUnknownTypeException`. The whole
`CLAUDE.md` "BSON Storage Names vs Qualified Names" section exists to manage
this risk.

Studio Pro 11.11 now ships an **MCP server** (the "PED" — Progressive Element
Disclosure — server) that exposes document-level model operations over HTTP on a
local port (observed: `http://localhost:7782/mcp`). If mxcli could *route its
writes through that server* instead of editing files, then:

1. **Studio Pro stays open.** Changes apply to the live, running model. No
   close/reopen cycle, no file-lock races.
2. **Studio Pro becomes the authoritative serializer.** mxcli hands over a
   document; Studio Pro writes the BSON. This eliminates the entire class of
   BSON-fidelity bugs mxcli currently has to defend against, and `ped_check_errors`
   validates against the real type cache.
3. **The MDL authoring advantage is preserved.** Unlike an agent driving the MCP
   server tool-call-by-tool-call (the token cost analysed in
   [`MXCLI_STRATEGIC_POSITIONING.md`](../01-project/MXCLI_STRATEGIC_POSITIONING.md)),
   here *mxcli itself* is the MCP client. The agent still authors one compact MDL
   script; mxcli parses and plans it deterministically and only the resulting
   document writes cross the wire. We get MDL's compactness **and** Studio Pro's
   serializer.

The target invocation, run from inside the devcontainer with the project
mounted:

```bash
mxcli -p http://localhost:7782/mcp -c "create entity Sales.Order (...)"
```

Model (`.mpr`) mutations flow through the MCP server; non-model files
(Java/JavaScript/CSS) remain on the mounted filesystem and are inspected
directly as today.

## Why this is feasible: the backend abstraction already supports it

`mdl/backend/backend.go` defines `FullBackend`, composed of ~23 sub-interfaces
(~226 methods). The executor never touches `sdk/mpr` for writes — it goes
through `ctx.Backend` (`mdl/executor/exec_context.go:31`,
[ADR-0002](../13-decisions/0002-backend-abstraction.md)). The concrete backend
is produced by a single `BackendFactory` set once:

```go
// cmd/mxcli/main.go:205
exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })
```

A new backend plugs in at exactly **one dispatch site**. Nothing in the grammar,
AST, visitor, or executor handlers changes — the abstraction is opaque to them.
The `mock` backend (`mdl/backend/mock/`) already proves a second, *partial*
implementation works: a nil `Func` field returns a safe default, so unsupported
operations can return a clean `"not supported by MCP backend"` error instead of
requiring all 226 methods up front.

## The MCP tool surface (verified against Studio Pro 11.11)

> The per-version tool matrix and capability gaps now live in
> [`docs/03-development/PED_MCP_CAPABILITIES.md`](../03-development/PED_MCP_CAPABILITIES.md)
> — the canonical, living record. Update that doc when onboarding a new Studio
> Pro version. The summary below is the original snapshot.

Dumped live with `cmd/mcpprobe` on 2026-05-29. Server identifies as
`mendix-studio-pro` 1.0.0, protocol `2025-06-18`; `initialize` instructs clients
to first read the resource `mendix://studio-pro/system-prompt`. The server is
exposing **Mendix's own "Maia" agent tools and PED contract**. Full captures
(`tools.json`, the system-prompt resource) were taken locally with `cmd/mcpprobe`;
they contain Mendix-internal content and are **not committed** pending a decision
on whether to vendor them (see Open Questions).

16 tools. The ones the backend needs:

| Tool | Purpose | Maps to backend concept |
|------|---------|-------------------------|
| `ped_get_schema` | Schemas for element types (**mandatory before create/add**). Returns a `$constructor` (simplified, flattened — for creation) **and** a `$element` (full, for reads/updates) | type metadata |
| `ped_read_document` | **Progressive** read — reads to element boundary; pass JSON paths (`/flows/0`) to descend | unit / element read |
| `ped_find_document` | Find by `moduleName` + `documentType` (**mandatory before create**, idempotency) | "does X exist?" |
| `ped_list_folder` | Immediate contents of a module/folder | partial enumeration |
| `ped_create_document` | Create one or more documents. **"Never create domain models."** | `Create*` (most doctypes) |
| `ped_create_module` | Create a module (and its domain model) | `CreateModule` |
| `ped_update_document` | **Operation-based**: atomic set/add/remove ops at JSON paths | `Update*`, `ALTER`, mutator `Save()` |
| `ped_check_errors` | Validate documents (**mandatory after final create/update**) | post-write validation |
| `pg_read_page` / `pg_write_page` | **Pages only** — a separate write path; PED is *forbidden* for pages | page `Create/Update` |

Other tools (`search_mendix_knowledge_base`, `read_skill`, `oql_generate`,
`glob`/`read_file`/`write_file` over virtual "file domains") are agent helpers,
not needed by the backend — though `write_file` is notable: the server itself can
write Java/JS/CSS, an alternative to the mounted-filesystem approach.

### Implications that reshape the design

1. **Two write protocols, not one.** Pages **must** use `pg_*`; everything else
   uses `ped_*`. The system prompt is explicit: "Never use PED … for pages." The
   backend's page methods fork to `pg_write_page` while domain model / microflow /
   enum / workflow methods go through `ped_*`. (Convenient: pages already have the
   most fragile BSON in mxcli, and `pg_write_page` takes a high-level widget tree,
   not raw BSON.)
2. **Domain model is update-only.** `ped_create_document` refuses domain models.
   The chosen vertical slice creates entities via `ped_update_document` (add-ops
   on the module's existing DomainModel), and new modules via `ped_create_module`.
   This is *more* aligned with mxcli's mutator pattern than with its `Create*`
   path.
3. **Operation-based updates map to ALTER and mutators directly.**
   `ped_update_document(documentType, operations:[set/add/remove @ path])` is
   almost a wire format for `PageMutator`/`WorkflowMutator` and `ALTER`
   statements — set property, add widget, remove activity.
4. **Construct vs read-path schema duality.** Creation uses the `$constructor`
   shape (flattened, e.g. `objects`); updates/reads use real JSON paths (e.g.
   `/objectCollection/objects`). The backend must hold both mappings per type
   (from `ped_get_schema`) — a real implementation gotcha to budget for.
5. **A mandatory call choreography** per write:
   `ped_find_document` → `ped_get_schema` → `ped_create_document`/`ped_update_document`
   → `ped_check_errors`. The backend encodes this sequence internally; mxcli's
   own pre-flight `check` can run *before* any of it.
6. **Non-user modules are read-only**; reserved attribute names (`Type`, `id`, …)
   are rejected — both already partly enforced by mxcli's validation.

These operate at the document/element level — which is what mxcli's writer
already *produces*. mxcli builds the model change; instead of writing a `.mxunit`
it ships document/operation JSON to `ped_*` (or a widget tree to `pg_write_page`).

## Connection model

### `-p` / `CONNECT` URL detection

`-p` and the `CONNECT` statement currently accept a file path only
(`ast.ConnectStmt.Path`, threaded into the `BackendFactory` at
`cmd/mxcli/main.go`). The change:

- If the project argument parses as an `http(s)://` URL → instantiate the MCP
  backend. Otherwise → the existing MPR backend.
- Surface an explicit form too: `CONNECT MCP 'http://localhost:7782/mcp'`
  alongside `CONNECT LOCAL '...'`, for scripts and the REPL.

The `BackendFactory` signature stays the same shape; the factory body inspects
the descriptor and returns the right `FullBackend`.

### Transport — root-caused and bridged (works today)

Established empirically with `cmd/mcpprobe` against a running Studio Pro 11.11 on
a macOS host:

1. **The server binds IPv6 loopback only.** `lsof` on the host shows
   `studiopro … TCP [::1]:7782 (LISTEN)` — not `127.0.0.1`, not `0.0.0.0`. On the
   host, `localhost`/`mcp-remote` works because macOS resolves `localhost` to
   `::1`. From a devcontainer, `host.docker.internal` is an **IPv4** gateway, and
   `::1` is non-routable loopback — so the container cannot reach it directly
   (connection refused). The earlier *hang* was a second Studio Pro (11.10)
   answering on another interface; killing it left only the `[::1]` listener.
2. **The server validates the HTTP `Host` header** (DNS-rebinding guard): the
   `/mcp` route only matches when `Host: localhost` (else 404). So the client
   must pin `Host: localhost` regardless of where it dials.

**Working bridge:** a raw TCP forwarder **on the host** from an IPv4
all-interfaces port to the IPv6 loopback:

```
# on the host
socat TCP4-LISTEN:7783,reuseaddr,fork 'TCP6:[::1]:7782'
```

The container then dials `host.docker.internal:7783` while sending
`Host: localhost`. With this in place, `cmd/mcpprobe` completes the full
handshake and `tools/list`/`resources/read` succeed (this is how the surface
above was captured). `cmd/mcpprobe` bakes the `Host` override in via a custom
dialer (dial target ≠ `Host` header).

**Remaining productisation questions (Phase 3, not blockers):**

- Ship the forwarder so users don't hand-run `socat`: a devcontainer
  `postStartCommand`, a `make` target, or a tiny `-listen` mode added to the Go
  client (no `socat` dependency). The bridge must run **on the host** (it reaches
  `::1`), so a container-side `make` target alone won't do — document the host
  step, or detect/instruct.
- Confirm whether Studio Pro can be configured to bind `0.0.0.0`/IPv4 (would
  remove the forwarder entirely). Needs a per-OS support matrix.

## Read path: hybrid — but with a CONFIRMED consistency hole

The plan was: **writes** via MCP; **reads** (`SHOW`/`DESCRIBE`, reference
validation, catalog, search, `show structure`) from the **mounted local
`.mpr`/`mprcontents` files**, reusing all existing read code and the SQLite
catalog. Rationale held: PED is document-addressed with only `ped_list_folder`
for enumeration, so catalog/search/structure can't be served from MCP without N
round-trips; local files give full, fast read coverage for free.

> **⚠ Disproven assumption (measured 2026-05-29 against test7-app).** While Studio
> Pro is open, **the on-disk files are stale by default** — Studio Pro is the
> system of record, and most MCP edits are *not persisted to disk until the user
> saves*. Measured behaviour:
> - `ped_create_module` → **flushed to disk immediately** (4 new `.mxunit`, `.mpr`
>   touched; `mxcli show modules` saw the new module).
> - `ped_update_document` (add an entity) → applied to Studio Pro's **in-memory
>   model only**. MCP read-back showed the entity; the user saw it in the UI
>   marked **unsaved**; disk was unchanged and `mxcli describe entity` returned
>   "not found".
> - `ped_check_errors` validated cleanly but did **not** flush either.
> - There is **no save/flush tool** among the 16 exposed tools.
>
> So a hybrid read taken right after a write returns *stale* data for any edit
> that stays in memory — which is most of them. This is not a rare edge case; it
> is the default during a live editing session.

This forces a real decision the original "hybrid (decided)" answer did not
anticipate. Options, in order of preference given the evidence:

1. **Read-through-MCP for just-written documents; local files for bulk/catalog.**
   After a write, reads of *that* document/module go through `ped_read_document`
   (consistent with memory); catalog/search/structure still use local files
   (accepting they reflect last-saved state). The backend tracks a "dirty set"
   of documents touched this session and routes their reads to MCP.
2. **Require a save.** If Mendix adds a save/flush tool (or one exists undocumented),
   call it after each write batch, then local reads are correct. Cleanest if
   available — **action: ask Mendix / re-scan tool surface across versions.**
3. **Pure-MCP reads.** Always consistent, but loses the catalog/search/fast
   enumerate that motivated hybrid, and needs many round-trips.

Recommendation shifts toward **Option 1** for the vertical slice: it keeps the
catalog/search win while staying correct for the documents the session just
edited. The MCP backend is therefore a **composite with a dirty-set router**:
local MPR reader for cold/bulk reads, MCP client for writes *and* for reads of
documents written this session.

## Implementation seam — two options, with a recommendation

The MPR backend currently mixes **BSON-document construction** (entity → BSON)
with **persistence** (write the `.mxunit`, update the SQLite `_units` table). An
MCP backend needs the *same construction* but a *different persistence target*.

### Option A — Extract a shared persistence seam (recommended)

Refactor so document construction is shared and only the bottom layer differs:

```
CreateEntity(...) ──> build BSON document ──> persist(unit)
                          (shared)              │
                                                ├─ MPR:  write .mxunit + _units
                                                └─ MCP:  ped_create_document(bson)
```

Introduce a narrow `UnitStore`-style seam (read unit / write unit / find unit /
schema / validate) that both backends implement; the high-level `FullBackend`
methods are implemented once on top of it. `RawUnitBackend`
(`mdl/backend/infrastructure.go:20`) is already close to this shape and is a
natural anchor.

- **Pros:** no duplication of BSON construction; the MCP backend is small
  (implement the seam + the hybrid read delegation); future doctypes light up on
  both backends at once.
- **Cons:** touches the MPR backend (the most load-bearing code); needs careful
  regression testing to prove the refactor is behaviour-preserving (the
  `feedback_validation_rigor` lesson applies — green tests must exercise the
  actual construct, not a proxy).

### Option B — Standalone parallel backend

A new `mcpbackend` package implements `FullBackend` independently, calling
existing serializers where reachable and the local reader for the read half.

- **Pros:** zero changes to the MPR backend; faster to start.
- **Cons:** high risk of duplicating construction logic that already lives
  entangled in the MPR backend; the two will drift as doctypes are added.

**Recommendation: Option A.** The value of this feature is *Studio Pro does the
serialization*; that only pays off if mxcli isn't separately re-deriving BSON in
two places. Extracting the seam is the difference between "an MCP backend" and
"a second copy of the MPR backend that happens to call HTTP." Do the extraction
behind the existing MPR backend first (pure refactor, MPR-only, fully tested),
*then* add the MCP implementation of the seam.

## Scope — first cut: domain-model vertical slice (decided)

Prove the whole pipe on the smallest meaningful surface before committing to 226
methods:

- `CREATE ENTITY` / `ALTER ENTITY` (add/modify/drop attributes), `CREATE
  ASSOCIATION`, `DROP ENTITY` — executed via `ped_create_document` /
  `ped_update_document`, validated via `ped_check_errors`.
- Reads (`SHOW`/`DESCRIBE ENTITY`, reference validation) served from the local
  mounted files (hybrid).
- Everything else returns a clear `"not supported by MCP backend (use a local
  .mpr)"` error via the mock-style default.

This exercises: URL dispatch, the Host-header transport, the seam, the
create+validate round-trip, and the hybrid read split — i.e. every risky part —
on one doctype.

## Implementation Plan

### Phase 0 — Reconnaissance (blocks everything)
Use `cmd/mcpprobe` (built in this PR — a minimal Go streamable-HTTP MCP client)
to dump the real `tools/list` and a sample `ped_get_schema` /
`ped_create_document` exchange from the running 11.11 server, pinning down tool
names, argument schemas, document addressing, session/auth handling. **Run it on
the host** until the devcontainer transport (Open Question 4) is bridged — a raw
`curl` cannot complete the handshake, and from the container the `/mcp` response
does not return (see "Transport gotcha").

### Phase 1 — Seam refactor (Option A), MPR-only
Extract the unit-persistence seam under the existing MPR backend. No behaviour
change. Land with full existing-test green + targeted construction tests.

### Phase 2 — MCP backend, domain-model slice
New `mdl/backend/mcp/` package implementing the seam over PED + delegating reads
to a local MPR reader. URL dispatch in `cmd/mxcli/main.go` + `CONNECT MCP`.

### Phase 3 — Hardening & docs
Freshness handling, error mapping (`ped_check_errors` → mxcli diagnostics), and
the devcontainer networking prerequisite doc.

### Files to modify / create

| File | Change |
|------|--------|
| `cmd/mcpprobe/main.go` | **Done (this PR)** — minimal streamable-HTTP MCP client for Phase 0 recon; seed for the real client |
| `mdl/backend/<seam>.go` | New narrow unit-persistence interface (read/write/find/schema/validate) |
| `mdl/backend/mpr/*.go` | Refactor MPR backend to implement high-level methods on top of the seam (Option A) |
| `mdl/backend/mcp/` (new) | MCP client backend: PED tool calls + local-reader delegation for reads |
| `mdl/backend/mcp/client.go` (new) | Streamable-HTTP MCP client with `Host: localhost` override |
| `cmd/mxcli/main.go` | `BackendFactory` dispatch: URL → MCP backend, path → MPR backend |
| `mdl/ast/ast_connection.go` | Distinguish `CONNECT LOCAL` vs `CONNECT MCP` (or a transport field) |
| `mdl/grammar/MDLParser.g4` | `CONNECT MCP 'url'` rule (regenerate with `make grammar`) |
| `mdl/visitor/` | Bridge the new CONNECT form to AST |
| `mdl/executor/executor_connect.go` | Route to the right backend based on transport |
| `mdl/backend/mock/backend.go` | Stubs for any new seam methods (`"MockBackend.X not configured"`) |
| `docs-site/src/` | "Connecting to a live Studio Pro" user doc + devcontainer networking note |

## Version Compatibility

- Requires Studio Pro **11.11+** with the MCP server enabled (the feature is
  gated on the *running Studio Pro*, not the project's Mendix version). The
  backend should probe the server on connect and fail with an actionable error
  if the MCP endpoint is absent or the tool surface is unrecognised.
- No `sdk/versions/*.yaml` entry needed for MDL syntax (the slice reuses existing
  domain-model statements); a registry/probe for the *MCP server* version may be
  warranted in Phase 3.

## Test Plan

- **Phase 0 probe:** ✅ done — `cmd/mcpprobe` captured `tools/list` and the
  `mendix://studio-pro/system-prompt` resource (locally; see note above).
  Remaining: capture one full `ped_get_schema` → `ped_update_document` (add
  entity) → `ped_check_errors` exchange as a fixture.
- **Seam refactor (Phase 1):** existing `mdl/executor` + `mdl/backend/mpr` tests
  must stay green; add construction-level tests asserting the BSON produced is
  byte-identical before/after the refactor.
- **MCP slice (Phase 2):** integration test against a running Studio Pro 11.11 +
  test project (e.g. `test6-app`, the 11.10/11.x reference fixture). Round-trip:
  `CREATE ENTITY` via MCP → `ped_check_errors` clean → entity visible in Studio
  Pro and in a subsequent local read.
- **Convergence with the BSON oracle:** this backend is the natural executor for
  [`PROPOSAL_mcp_bson_benchmark`](PROPOSAL_mcp_bson_benchmark.md) — running the
  same MDL through the MPR backend and the MCP backend and diffing the resulting
  `.mxunit` files is both a correctness test *and* the benchmark that proposal
  describes.
- **`mdl-examples/doctype-tests/`:** domain-model scripts run against both
  backends in CI where a Studio Pro instance is available.

## Multi-agent orchestration (concurrent writes through one PED server)

Because the PED server is session-based (each client gets its own
`Mcp-Session-Id`) but backed by a **single** Studio Pro process with **one
in-memory model**, several agents can edit the model concurrently *if they are
orchestrated to avoid writing the same document at the same time*. The
connection multiplexes fine; the model is the shared resource. Verified
empirically: a read succeeds on one client while another session is connected.

**The unit of isolation is the document, not the model.** Every write tool acts
on one named document (`ped_create_document`, `ped_update_document`,
`ped_check_errors`). Two agents on *different* documents are isolated; the
hazards are all on the *same* document.

Orchestration rules:

1. **Partition by document — and domain models partition by MODULE.** A module's
   whole domain model is one document, so two agents both adding entities to
   `Sales` are mutating the same `/entities` array. Assign whole modules (not
   entities within a module) to agents; assign whole microflows/pages likewise.
2. **Same-array add/remove is the sharp edge.** `ped_update_document` uses
   positional paths (`/entities/N`, `remove index N`); the skill's own
   "re-read after every mutation, indices shift" rule is unenforceable across two
   writers (TOCTOU). If two agents must touch one document, funnel those writes
   through a single serialized owner.
3. **Order cross-document dependencies first.** PED resolves references by name
   against the live model *at write time*: the enum before the entity that uses
   it, the view-entity source doc before the entity, the callee before the
   caller. The orchestrator topologically sorts dependency edges and creates
   dependencies before dependents.
4. **Make writes idempotent for timeout-retry.** Slow creates can return
   `-32000 Request timed out` *even though they succeeded* (Studio Pro appears to
   serialize work on its UI thread). Retries must check-then-act
   (`ped_find_document`/read before create) and treat "already exists" as done;
   blind retry produces duplicates.
5. **`ped_check_errors` sees everyone's in-progress state.** It validates the
   live model as-is, including another agent's half-finished edits. Validate at a
   quiescent point, scoped to your own documents, tolerant of unrelated transient
   errors.
6. **No per-agent commit; one human save at the end.** There is no flush tool —
   all agents accumulate into one unsaved in-memory model and a human saves once
   in Studio Pro. No agent can durably commit its slice independently.

**Hybrid-backend wrinkle.** mxcli reads/enumerates from the local `.mpr` and
writes via MCP, so an agent's existence checks (`findModule`, `ListMicroflows`,
enum validation) read *last-saved disk state* and the dirty-set router only
tracks the current process's own writes. Multiple mxcli agents therefore cannot
see each other's *unsaved* work through the local reader. The orchestrator must
own the dependency graph (create dependencies first and tell dependents they
exist), or route shared-dependency lookups through MCP (`ped_read_document`)
rather than the local reader.

The recurring procedure for this is captured in
[`.claude/skills/orchestrate-mcp-agents.md`](../../.claude/skills/orchestrate-mcp-agents.md).

## Open Questions

1. **MCP tool surface** — ✅ resolved (see "MCP tool surface"; captured locally).
   **Decision needed:** do we vendor the `tools.json` schemas and/or the Maia
   system-prompt into the repo as reference? The schemas are useful for
   implementation; the system prompt is Mendix-internal content. Follow-ups: the exact
   `$constructor` schema for adding an entity via `ped_update_document`, and the
   per-tool `taskSupport: forbidden` flag (does it block any of our calls, or
   only async "tasks"?).
2. **Streamable-HTTP handshake** — ✅ resolved. `cmd/mcpprobe` completes
   initialize → `notifications/initialized` → call, handling JSON **and** SSE,
   `Mcp-Session-Id`, and the `Host` override. Server is protocol `2025-06-18`,
   single-JSON responses observed (no SSE needed so far). Decide: grow
   `mcpprobe`'s client core into `mdl/backend/mcp/client.go` vs adopt a library.
3. **Unsaved-state semantics** — ✅ **answered (measured).** MCP edits apply to
   Studio Pro's in-memory model; most (`ped_update_document`) are **not persisted
   to disk until the user saves**, `ped_check_errors` does not flush, and there is
   no save tool. Only `ped_create_module` flushed immediately. → drives the
   "consistency hole" + dirty-set router in the Read-path section. Open follow-up:
   is there *any* flush path (a save tool in another version, an autosave timer,
   a `pg_*` equivalent), and does `pg_write_page` flush like module-create or stay
   in memory like `ped_update_document`?
4. **Devcontainer networking** — ✅ root-caused (IPv6-loopback bind) and bridged
   with a host-side `socat` IPv4→`[::1]` forwarder (see "Transport"). Remaining is
   *productisation* (ship the forwarder / host-bind option / per-OS matrix), not
   feasibility.
5. **Error-recovery UX** — `ped_check_errors` is a one-shot, post-application fix
   protocol. mxcli's strength is *pre-flight* `check`. How do we reconcile: run
   `mxcli check` locally before shipping any write, so MCP writes are only
   attempted for scripts that already pass static validation?
6. **Concurrency** — ✅ characterised (see "Multi-agent orchestration"). Writes
   through the single Studio Pro process sidestep the file-lock problem in
   [`PROPOSAL_concurrent_access`](PROPOSAL_concurrent_access.md): the server is
   session-multiplexed and the model is the shared resource, so concurrent agents
   are safe when partitioned by document (modules for domain models) with
   dependency ordering and idempotent retries. Open follow-up: confirm whether
   the server processes sessions truly in parallel or serialises on the Studio
   Pro UI thread (the `-32000` timeouts under load suggest serialisation).
