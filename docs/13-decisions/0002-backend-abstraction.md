# ADR-0002: Backend Abstraction Layer

- **Status**: Accepted
- **Date**: 2026-05-24
- **Related**: [PROPOSAL_agentic_architecture_improvements.md](../11-proposals/PROPOSAL_agentic_architecture_improvements.md); `mdl/backend/doc.go`

## Context

Early in mxcli's development, the MDL executor imported `sdk/mpr` directly and constructed BSON inline. This produced three compounding problems:

1. **BSON detail bled into business logic.** Executor handlers carried knowledge of `$type` strings, `ParentPointer` semantics, and reflection-data quirks alongside the actual MDL operation. Refactoring storage concerns required touching every handler.
2. **Testing was structural.** Handler tests needed real `.mpr` files or hand-built BSON. Unit-testing the *MDL semantics* of a handler was impossible without also asserting on BSON layout.
3. **No path to alternative storage.** Even speculative work — an in-memory backend for the REPL, a remote backend for cloud projects, a fast read-only catalog — was blocked by the assumption that MDL ⇒ MPR.

The deeper issue was that the executor was the wrong layer to know about BSON. The executor's job is "given an MDL statement, perform the operation"; BSON is one possible serialization, not the operation itself.

## Decision

Introduce a domain-grouped interface layer in `mdl/backend/` that the executor uses exclusively for storage operations. Shared value types live in `mdl/types/` so the backend package itself depends on neither `sdk/mpr` nor any other concrete storage. Concrete implementations live in sibling packages — `mdl/backend/mpr/` (production) and `mdl/backend/mock/` (tests) — with a compile-time `var _ backend.SomeInterface = (*impl)(nil)` check on each.

Specifically:

- The executor **must not import `sdk/mpr` for write paths.** All mutations go through `ctx.Backend.*`.
- Each domain (DomainModel, Microflow, Page, Security, Workflow, etc.) has its own interface; `FullBackend` composes them as a construction-time constraint.
- ALTER operations on container-shaped artifacts (pages, workflows) go through mutator handles (`OpenPageForMutation()`, `OpenWorkflowForMutation()`) rather than inline BSON construction.
- The mock package provides `Func`-field stubs returning a descriptive `"MockBackend.X not configured"` error by default, never `nil, nil` — silent test passes are a worse failure mode than loud ones.
- Map iteration for serialized output must sort keys (`sort.Strings(keys)`); non-deterministic ordering makes BSON diffs flaky.

## Consequences

**Positive:**

- Executor handlers are testable in isolation — `MockBackend` lets tests assert MDL semantics without touching BSON.
- BSON drift bugs (numeric width, storage-name mismatches, version-fragile fields) are quarantined to `mdl/backend/mpr/` and `sdk/mpr/`, where the people who understand BSON work.
- Future non-MPR backends are mechanical to add: implement the interfaces and pass the compile-time check.
- The PR checklist can enforce the boundary by greping for forbidden imports.

**Negative:**

- Every new operation needs four touches: interface method, MPR implementation, mock stub, compile-time check. The overhead is real for small features.
- Two extra layers of indirection between an MDL statement and a BSON write. Stack traces are longer.
- Contributors must learn the convention before adding handlers — the wrong instinct ("just call `sdk/mpr` directly") is the obvious one.
- Shared types in `mdl/types/` create a third package contributors must understand; the `sdk/mpr` re-export pattern (`type Foo = types.Foo`) adds further indirection.

**Neutral:**

- The boundary is enforced by convention + PR review, not by Go's package visibility — `sdk/mpr` is still importable from anywhere. Strict enforcement would require splitting the module, which is not warranted today.

## Alternatives considered

- **Status quo — direct `sdk/mpr` usage.** Rejected: testing, BSON detail leakage, and storage flexibility were all degrading as feature count grew.
- **Single facade interface (`Backend` with all methods).** Rejected: a one-interface design leaks every domain to every handler and breaks Go's "interfaces should be small" idiom. The composed-domain approach lets handlers receive only the sub-interface they need.
- **Full hexagonal architecture / ports-and-adapters.** Rejected as overkill: the rest of mxcli (parsing, AST, visitor) doesn't benefit from the same separation, and full hexagonal style would require renaming and restructuring beyond the storage boundary.
- **Generate the backend from a schema.** Rejected for now: the interface surface is still evolving, and hand-written interfaces let us shape ergonomics per domain (mutator handles for pages, query helpers for catalogs). Revisit if churn becomes a maintenance problem.
