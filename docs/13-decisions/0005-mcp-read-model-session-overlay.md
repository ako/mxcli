# ADR-0005: MCP backend read model — disk base with session overlay

- **Status**: Proposed
- **Date**: 2026-06-14
- **Deciders**: mxcli maintainers
- **Related**: [ADR-0002](0002-backend-abstraction.md) (backend abstraction), the MCP backend proposal (`docs/11-proposals/PROPOSAL_mcp_backend.md`), the tool-call cost page (`docs-site/src/internals/mcp-backend-cost.md`), bug repro `mdl-examples/bug-tests/mcp-association-existing-module.mdl`. Relates to the "Delta / change-tracking system" on CLAUDE.md's *Not Yet Implemented* list.

## Context

The MCP backend has a split read/write model: **writes** go to a running Studio Pro over its embedded MCP server ("PED"), while **reads** are served from the local `.mpr` passed with `-p`. The `.mpr` is a pre-session snapshot — Studio Pro does not flush model changes to it until the user saves. So within a single run, anything written this session is absent from the on-disk file.

To keep in-session reads correct, the backend uses **dirty-set reconstruction**: a module written this session is reconstructed wholesale from Studio Pro's live model on each read. This is now centralized behind one `effectiveDomainModel` seam (after a bug where `ListDomainModels` skipped the routing and `CREATE ASSOCIATION` failed to resolve just-created entities in an existing module).

Reconstruction works, but it has structural costs:

1. **PED-dependent reads.** Every read of a dirty module is one or more live PED calls. PED is the flaky, slow part — it times out, and an `oql_generate` probe once hung Studio Pro entirely. Coupling reads to PED makes them as fragile as PED.
2. **Lossy.** PED reads expose attribute *names* but not types; recovering types needs an extra batched leaf read (`enrichReconstructedEntities`), and reconstructed elements get synthetic IDs, never the real PED `$ID`.
3. **Per-read cost.** ~2+ round trips per read of a dirty module (see the cost page).
4. **Partial coverage.** Only the domain model is reconstructed. Standalone documents (enumerations, microflows, pages) created this session have *no* overlay — reads of them fall straight through to the stale on-disk reader.

The recurring question: should reads instead take the **full-fidelity on-disk model as a base and overlay the changes made this session**, rather than reconstructing from PED?

## Decision

*(Proposed, not yet adopted.)* Introduce a **session write-overlay** read model for the MCP backend:

- The backend maintains an in-memory overlay of the elements it created or modified this session, built from the rich domain objects it already constructs at write time (entities with real types, validation rules, associations, enumerations, …), keyed by qualified name.
- A read returns the **on-disk base** with the overlay applied (add/replace by qualified name). Resolution (does `X` exist? its name/type?) is served entirely from base + overlay — **PED is no longer on the read path**.
- The overlay covers the domain model *and* standalone document types, closing the partial-coverage gap uniformly.
- mxcli assumes it is the **sole writer during a run** (an explicit, documented contract); a ground-truth `refresh` escape hatch re-reads from PED for the mixed-session case.

## Consequences

**Positive**

- **Resilience** (the primary motivation): reads no longer depend on PED, so they are immune to PED timeouts/hangs/unavailability. A flaky or wedged Studio Pro can't break read/resolution.
- **Speed**: no per-read PED round trips; reads are local file + in-memory map.
- **Fidelity**: the overlay carries the exact types/structure we wrote (better than lossy reconstruction), and the on-disk base keeps full fidelity for everything unchanged.
- **Uniformity**: one overlay layer serves domain-model and standalone-doc reads — the enum/microflow/page gap closes for free.

**Negative / risks**

- **Drift from ground truth.** The overlay reflects our *intent*, not Studio Pro's actual state. It diverges if the model is changed out of band mid-run: the user edits in the Studio Pro UI, PED normalizes/rejects/transforms a write, or another tool (Maia, Concord) touches the model. Reconstruction reads ground truth and cannot drift. Mitigations: the sole-writer contract, the `refresh` hatch, and the post-write `ped_check_errors` we already run (catches rejected writes so the overlay isn't blindly trusted).
- **IDs.** Overlay elements carry mxcli-generated IDs, not real PED `$ID`s. This is fine for name-based resolution (e.g. associations re-resolve to PED's `$id(/entities/N)` form at write time), but any operation that genuinely needs the real `$ID` must still query PED.
- **New subsystem to keep correct.** This *is* the delta/change-tracking system: every write path must update the overlay, and the apply/merge logic must be right. More surface than today's reconstruction.
- **Timing/coupling.** A model engine replacement (modelsdk) is in flight, and there is a standing constraint to avoid churning the shared/MPR read paths before it merges. A full overlay built on today's backend risks rework.

**Neutral**

- `DESCRIBE` of an in-session-edited document shows our intent by default; exact live state is available via the `refresh` hatch.

## Alternatives considered

1. **Status quo — centralized PED reconstruction** (shipped). All domain-model reads funnel through `effectiveDomainModel`; correct and ground-truth, but PED-dependent, lossy, costly, and only covers the domain model. Adequate for correctness today; this ADR is about whether to go further.
2. **Full session overlay** — the proposed decision.
3. **Hybrid** — overlay for *resolution* (existence/name/type, what `findEntity` needs), reconstruction for *authoritative* reads (`DESCRIBE`, consistency checks). Captures most of the resilience win for resolution while keeping ground truth where exactness matters, at the cost of two read paths to reason about.
4. **Require a save after writes** (the manual workaround: write, Cmd+S, re-read). Rejected — impractical for automated/scripted runs, which are mxcli's main use.
5. **Defer the overlay to the modelsdk engine.** Rather than retrofit the current backend, design the overlay as part of the new engine's read model. Recommended sequencing regardless of which read semantics win.

## Recommendation

Centralized reconstruction (already shipped) is sufficient for **correctness** now. The overlay's decisive advantage is **resilience** — getting flaky PED off the read path — with a secondary fidelity win. But it is a real subsystem with a genuine drift risk, and it overlaps the incoming engine work. Recommended path: keep reconstruction in the current backend; design and build the session overlay (option 2, or the option-3 hybrid) **with the modelsdk engine**, not as a retrofit. Revisit this ADR's status when that work starts.
