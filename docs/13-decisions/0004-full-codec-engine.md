# ADR-0004: Route all document types through the codec engine

- **Status**: Accepted
- **Date**: 2026-06-06
- **Related**: [ADR-0002](0002-backend-abstraction.md); `docs/plans/2026-06-05-adopt-modelsdk-engine.md`; engalar `feat/modelsdk-core`

## Context

We are adopting the engalar `modelsdk/` codec — a generated 53-domain metamodel
plus a roundtrip-safe encoder/decoder — behind the `MXCLI_ENGINE` flag, as an
experimental alternative to the hand-written `sdk/mpr` parser/serializer. The
codec's defining property is **passthrough**: a decoded element re-encodes its
clean fields from their original raw bytes, so fields the metamodel doesn't model
(version-specific, exotic, future) survive a read-modify-write untouched. The
hand-written `sdk/mpr` serializer only emits the fields it explicitly knows about.

A natural fork emerged once the modelsdk engine reached domain-model writes. The
engalar fork itself never resolved it: on `feat/modelsdk-core` the codec ships as
a standalone library, but **every domain-model write still delegates to
`sdk/mpr`** — the `DomainModelBackend` interface keeps trafficking in
`domainmodel.Entity`, and `mdl/backend/mpr` imports neither `modelsdk/codec` nor
`modelsdk/gen`. So engalar's implicit architecture is a **hybrid**: codec for the
roundtrip-fragile types (pages, workflows, microflows — where the legacy
serializer drops unknown widget/activity fields), legacy `sdk/mpr` for domain
models (where the hand-written serializer is already byte-faithful, as our own
parity tests independently confirmed).

The choice for *our* effort: continue routing domain models through the codec
too (uniform), or adopt engalar's hybrid split.

## Decision

Route **all** document types — domain models included — through the codec engine.
The `modelsdk` engine is the single write path we are building toward; we do not
keep a permanent hybrid where some document types use `sdk/mpr` writers and
others use the codec. Where the codec needs lossless `gen ↔ domainmodel`
conversion to satisfy the executor's existing `domainmodel.Entity`-based backend
contract, we build that conversion (per [ADR-0002](0002-backend-abstraction.md),
the executor and `domainmodel` types stay canonical; the translation lives in the
backend adapter).

## Consequences

**Positive.**
- One serialization path, one mental model, one place to fix BSON bugs — no
  per-document-type "which engine writes this?" routing.
- Passthrough roundtrip-safety applies everywhere, including domain models. This
  is not hypothetical: it is exactly the class of the legacy stale-`SortOrder`
  index bug we found — the codec preserves such fields structurally instead of
  re-deriving them from a partial model.
- A clean eventual cutover: once coverage is complete, `sdk/mpr` write paths can
  be retired wholesale rather than left as a permanent half.

**Negative.**
- More work per document type than the hybrid: each read-modify-write type needs
  a lossless `gen → domainmodel` adapter (e.g. `entityFromGen` had to grow full
  attribute types, Location, index segments, validation rules). Domain models,
  where legacy was already faithful, pay this cost for a benefit that is mostly
  future-proofing.
- Until coverage is complete, gaps must be **guarded, not silent**: where the
  codec path cannot yet reproduce a construct (e.g. entity access rules / event
  handlers), the backend refuses the operation ("use the legacy engine") rather
  than dropping data.

**Neutral.**
- The executor ↔ backend contract (`domainmodel.Entity`) is unchanged; this is an
  implementation decision inside the backend, not an interface change.
- The `legacy` engine remains the default and the fallback throughout the
  migration; this decision is about where the `modelsdk` engine is headed, not a
  flag-day switch.

## Alternatives considered

**Hybrid (engalar's implicit split): codec for fragile types, legacy `sdk/mpr` for
domain models.** Less work — domain models skip the lossless adapter entirely, and
legacy is already byte-faithful for them. Rejected because it makes the engine
boundary a permanent, type-by-type seam: two serializers to keep in parity, two
places to fix bugs, and no clean cutover. The roundtrip-safety benefit would be
unavailable precisely for the data model users edit most. The marginal adapter
cost is bounded and one-time; the hybrid's complexity is permanent.

**Defer the decision (keep building ad hoc).** Rejected: the fork point is real
and shapes every subsequent write slice, so the contributor needs the rationale
recorded now rather than reverse-engineered from PR history.
