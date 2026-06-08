# ADR-0005: The semantic model is the backend-interface currency; storage formats are swappable adapters

- **Status**: Accepted
- **Date**: 2026-06-08
- **Related**: [ADR-0002](0002-backend-abstraction.md) (backend abstraction), [ADR-0004](0004-full-codec-engine.md) (MPR backend uses the codec internally); `docs/11-proposals/ASSESSMENT_harvest_engalar_writes.md`

## Context

mxcli turns Mendix-model operations into changes against a project. Three layers can carry an operation:

- **AST** — the MDL parse tree. Transient, syntax-shaped, MDL-only, write-only (BSON is never parsed into an AST).
- **Semantic model** (`sdk/microflows`, `sdk/domainmodel`, `model`) — engine-agnostic Mendix concepts: clean names, resolved references, normalized.
- **gen + codec** (`modelsdk/gen`, `modelsdk/codec`) — a 1:1 mirror of the *current* MPR BSON storage shape, with its quirks (storage names, the `ConcurrenyErrorMessage` typo, binary refs, list markers) and a roundtrip-safe codec.

The engalar fork, having exactly **one front-end (MDL) and one back-end (MPR/BSON)**, builds AST→gen directly and exposes a **gen-typed** backend interface (`ListMicroflowsGen`); for that scope a semantic model is pure overhead. mxcli's scope is different:

- **Multiple front-ends today**: the MDL executor *and* the fluent `api/` builders (plausibly future importers / an LLM tool surface).
- **Multiple back-ends today**: MPR (BSON via codec) *and* MCP/PED (live Studio Pro, a JSON-operation API that is not BSON).
- **A third back-end is anticipated**: Mendix is considering replacing the BSON storage format with one that is more diff-friendly, separates interface from implementation storage (lazy/granular loading), is smaller, and enables runtime hot-reload.

The question: should operations flow through a semantic model, or go AST→gen directly like engalar?

## Decision

The **backend interface speaks the semantic model**, not storage (gen) or syntax (AST) types. Concretely:

- **Front-ends produce the model** (MDL executor, fluent `api/`, future importers).
- **Reads return the model**; the model is the currency for SHOW/DESCRIBE/catalog/lint/diff so consumers are backend-agnostic and storage-stable.
- **gen + codec are an implementation detail of the MPR backend** — one storage adapter among several (MPR, PED, future), never the contract.
- **CREATE goes model→storage** (the MPR backend does model→gen→BSON). **ALTER uses backend-internal mutation** (gen-mutation / the `OpenXForMutation` mutator pattern for MPR) rather than a model round-trip, so the model need not be a perfectly lossless mirror and edits preserve fidelity.
- The model distinguishes **interface (signature) vs implementation (body)** granularity, and backends may load them lazily (`List…` ≈ signatures, `Get…` ≈ body) — matching both current catalog/lint performance needs and the anticipated interface/implementation-split storage format.

## Consequences

**Positive.**
- A new storage format (the one Mendix is planning) becomes **just another backend** (`model→newformat`) — the executor, front-ends, `api/`, lint, catalog, and diff are untouched. The model is the anti-corruption layer against exactly the storage churn that is coming.
- MCP/PED and `api/` remain first-class without contorting them into BSON/gen shapes.
- The codec's byte-passthrough roundtrip-safety — a coping mechanism for *today's* BSON churn — is correctly scoped to one adapter, so it can be retired with BSON rather than being load-bearing.

**Negative.**
- `model→storage` per backend is more code than AST→storage for the single-backend create path (the "duplication" felt while hand-porting `microflowToGen`). Accepted: with N backends this is the DRY factoring (parse once → AST→model; serialize per backend), not waste.
- The model must be maintained alongside gen. Mitigation: keep it **semantic, not a gen mirror**, and route fidelity-sensitive ALTER through gen-mutation rather than forcing a lossless model round-trip.

**Neutral.**
- This sits *above* [ADR-0004](0004-full-codec-engine.md): ADR-0004 governs the MPR backend's *internal* choice (use the codec for all document types, not a codec/legacy hybrid); ADR-0005 governs the *interface* (model-typed, gen is internal). They compose.

## Alternatives considered

**engalar's gen-typed, AST-direct architecture.** Build AST→gen, expose a gen-typed interface, drop the model. Rejected for mxcli: it welds the backend contract to the BSON shape, which (a) makes the MCP/PED backend unnatural (it would have to emit/consume gen trees for a non-BSON API), and (b) would require reworking the contract when Mendix replaces BSON. It is the right call for engalar's single-backend scope and the wrong one for ours. We still **harvest engalar's `flowBuilderGen` as a reference** for *how to construct each gen element* inside the MPR backend's `model→gen` — but not its interface or executor architecture.

**Build the model around the future format now.** Rejected as premature (YAGNI): the future format is unspecified. The justification for the model seam is the three concrete consumers we have today (MPR, MCP, `api/`); the coming format only confirms the hedge is aimed at the right risk. We build the seam, not the format.
