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

## The PED tool surface

From `MXCLI_STRATEGIC_POSITIONING.md` and
[`PROPOSAL_mcp_bson_benchmark`](PROPOSAL_mcp_bson_benchmark.md), the Studio Pro
MCP server exposes document/BSON-level tools:

| Tool | Purpose | Maps to backend concept |
|------|---------|-------------------------|
| `ped_get_schema` | Property structure + valid enum values for a `$Type` | type metadata / validation |
| `ped_read_document` | Read a document's current BSON | unit read |
| `ped_find_document` | Locate a document by name (idempotency check) | "does X exist?" |
| `ped_create_document` | Create a new document from BSON | `Create*` / unit write |
| `ped_update_document` | Update an existing document's BSON | `Update*` / mutator `Save()` |
| `ped_check_errors` | Validate the model (one-shot fix protocol) | post-write validation |

> **Open question (must verify against the running 11.11 server):** the exact
> JSON-RPC tool names, argument schemas, and document-addressing scheme
> (qualified name? unit ID? path?). The names above are from existing repo docs
> and the archived `PROPOSAL_schema_extract`; they may have drifted in 11.11.
> The first implementation task is to dump the real `tools/list` (see Test Plan).

Crucially, these operate at the **document/BSON level** — which is exactly what
mxcli's writer already *produces*. mxcli builds the BSON for an entity/microflow;
instead of writing a `.mxunit`, it ships that BSON to `ped_create_document`.

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

### Transport gotcha — UNRESOLVED from inside a devcontainer

Two facts established empirically with the `cmd/mcpprobe` Go client (and raw
sockets) against a running Studio Pro 11.11 on a macOS host:

1. **The server validates the HTTP `Host` header** as a DNS-rebinding guard.
   A request to `/mcp` with `Host: host.docker.internal` → **404**; with
   `Host: localhost` the route matches (no 404). So any client must dial the
   host gateway while pinning `Host: localhost`.

2. **But the `/mcp` response never reaches the container.** With `Host: localhost`,
   *every* request to `/mcp` — POST `initialize` or GET SSE, with any
   `Accept`/`Origin`/protocol-version combination — returns **zero response
   bytes** and hangs (verified up to 25 s). Meanwhile a fixed-length response on
   another route (`GET /` → 404) comes back through the **same** Docker Desktop
   gateway in milliseconds. `mcp-remote` against `localhost:7782` works fine
   **on the host**. So the client/protocol is not the problem — the `/mcp`
   handler's (streaming/SSE) response does not traverse the host gateway.

Two candidate root causes, distinguished by one host-side command
(`lsof -nP -iTCP:7782 -sTCP:LISTEN`):

- **(a) loopback-bound server.** Studio Pro binds the MCP port to `127.0.0.1`
  only; `host.docker.internal:7782` reaches a *different* listener that 404s and
  hangs. Fix: a host-side TCP forwarder bound to a gateway-reachable interface
  (`0.0.0.0:7783 → 127.0.0.1:7782`); the container then dials
  `host.docker.internal:7783` (raw TCP forwarding, so SSE passes), still sending
  `Host: localhost`.
- **(b) gateway doesn't pass the stream.** Docker Desktop's `host.docker.internal`
  path buffers/blocks the long-lived SSE/chunked response. Same host-side
  forwarder is the likely fix (raw TCP, no HTTP-aware buffering); if not, the
  devcontainer must reach the server by another route (host networking, explicit
  published port, or running the MCP backend on the host).

**This is now the top feasibility risk** for "run mxcli in a devcontainer and
write via MCP," because the whole premise is the devcontainer. It must be
resolved in Phase 0 before the backend work. The `cmd/mcpprobe` tool is the
vehicle: run it **on the host** (`mcpprobe -dial localhost:7782`) to confirm the
handshake + dump `tools/list`, then iterate on the container bridge.

## Read path: hybrid (decided)

**Writes** always go through MCP. **Reads** (`SHOW`/`DESCRIBE`, reference
validation, catalog, search, `show structure`) read the **mounted local
`.mpr`/`mprcontents` files** directly, reusing all existing read code and the
SQLite catalog unchanged.

Rationale: PED is document-addressed and (as far as the docs show) has **no bulk
enumerate**, so catalog/search/structure can't be served from MCP without N
round-trips. The local files give full, fast read coverage for free.

**Accepted limitation:** local files lag Studio Pro's *unsaved in-memory* state
until Studio Pro flushes to disk. Reads taken immediately after an MCP write may
not reflect that write. Mitigations to evaluate (not first-cut):

- If PED exposes a "save"/"flush" tool, call it after a write batch before
  reads.
- A `--freshness` guard that warns when the open Studio Pro project has unsaved
  changes (if detectable). This is the "Hybrid + freshness guard" path deferred
  from the design questions.

This means the MCP backend is, concretely, a **composite**: a local MPR reader
for the read half of `FullBackend`, and an MCP client for the write half.

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

- **Phase 0 probe:** a small harness (Go MCP client, or `mcp-remote` on the host)
  capturing `tools/list` and one full create+validate exchange, committed as a
  fixture. Note: raw `curl` through the relay reproduced the route match but did
  **not** complete the streamable-HTTP handshake (0-byte responses) — the probe
  must use a real MCP client, not `curl`.
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

## Open Questions

1. **Exact PED tool schemas in 11.11** — names, argument shapes, document
   addressing, session/auth, and whether any bulk-enumerate exists (would expand
   the read-path options). Resolved by Phase 0.
2. **Streamable-HTTP handshake details** — `cmd/mcpprobe` (this PR's recon tool)
   implements the full handshake (initialize → `notifications/initialized` →
   call, JSON **and** SSE framing, `Mcp-Session-Id`, `Host` override). Confirm
   it completes the handshake **on the host** and dump the real tool surface;
   decide whether to adopt an MCP client library or grow `mcpprobe`'s core into
   `mdl/backend/mcp/client.go`.
3. **Unsaved-state semantics** — does PED apply writes to Studio Pro's in-memory
   model immediately, and does it expose save/flush? This determines how severe
   the hybrid-read staleness actually is.
4. **Devcontainer networking (top risk)** — the `/mcp` response does not traverse
   the Docker Desktop host gateway today (see "Transport gotcha"). Resolve with
   `lsof -nP -iTCP:7782` on the host to confirm loopback-vs-all-interfaces, then
   settle on a supported bridge (host-side TCP forwarder, host networking, or
   published port) and document a per-OS support matrix. Blocks Phase 2.
5. **Error-recovery UX** — `ped_check_errors` is a one-shot, post-application fix
   protocol. mxcli's strength is *pre-flight* `check`. How do we reconcile: run
   `mxcli check` locally before shipping any write, so MCP writes are only
   attempted for scripts that already pass static validation?
6. **Concurrency** — with writes going through the single Studio Pro process,
   does this *sidestep* the file-lock problem in
   [`PROPOSAL_concurrent_access`](PROPOSAL_concurrent_access.md), or introduce a
   new serialization point? Likely the former for writes; reads still touch local
   files.
