---
title: Backend Abstraction
category: rationale
last-synced: 4e185f73
sources:
  - mdl/backend/doc.go
  - mdl/backend/backend.go
  - mdl/backend/domainmodel.go
  - CLAUDE.md
  - docs/13-decisions/0002-backend-abstraction.md
---

> **Do not duplicate**: the active compliance rule (CLAUDE.md "Backend abstraction compliance" is canonical) or the interface signatures (read `mdl/backend/*.go`). [ADR-0002](../../docs/13-decisions/0002-backend-abstraction.md) is the immutable decision; this page is mutable synthesis pointing back to it.

## What this is

The MDL executor never reaches past `ctx.Backend` for write paths. All storage operations go through domain-grouped interfaces in `mdl/backend/` (`ctx.Backend.*`), with concrete implementations in sibling packages — `mdl/backend/mpr/` for production and `mdl/backend/mock/` for tests. Shared value types live in `mdl/types/` so the interface package depends on no concrete storage at all.

## How it fits

The forcing problem was that the executor was the wrong layer to know about BSON. Early on it constructed BSON inline, which produced three compounding costs: `$type` strings and pointer semantics bled into business logic, handler tests required real `.mpr` files or hand-built BSON, and any alternative store (in-memory REPL, remote cloud) was blocked by the assumption that MDL implies MPR.

The chosen approach is a thin seam. The executor's job is "given an MDL statement, perform the operation"; BSON is one possible serialization, not the operation itself. Each domain (DomainModel, Microflow, Page, Workflow, ...) gets its own small interface, and `FullBackend` composes them only as a construction-time constraint — handlers receive just the sub-interface they need. Mock stubs return a loud `"MockBackend.X not configured"` error by default rather than `nil, nil`, because a silent test pass is a worse failure than a noisy one.

The key trade-off is **per-feature overhead**: every new operation needs four touches (interface method, MPR implementation, mock stub, compile-time check) and adds indirection. That is accepted to quarantine BSON drift bugs to the packages whose maintainers understand BSON. The boundary was once enforced by convention and PR review alone. It is now structural: `sdk/mpr` was deleted, so reaching past the abstraction is a compile error rather than a habit to resist. See [ADR-0002](../../docs/13-decisions/0002-backend-abstraction.md) for the full alternatives and consequences.

## See also

- [ADR-0002: Backend Abstraction Layer](../../docs/13-decisions/0002-backend-abstraction.md) — the immutable decision record
- [[architecture/mdl-execution]] — the executor pipeline this layer sits inside
- [[architecture/mpr-read-write]] — the MPR implementation behind the interface
