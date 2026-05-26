# ADR-0001: Record Architecture Decisions

- **Status**: Accepted
- **Date**: 2026-05-24

## Context

mxcli has accumulated significant cross-cutting architectural decisions:
pure-Go SQLite (no CGO), the backend abstraction layer that keeps
`sdk/mpr` out of the executor, MDL's SQL-shaped syntax, the V3 widget
engine with `.def.json` over hardcoded builders, inverted association
pointer semantics, and others. These decisions are currently scattered
across PR descriptions, commit messages, CLAUDE.md rules, and contributor
folklore.

Tracing the *why* behind a load-bearing pattern requires git archaeology,
and load-bearing patterns can be eroded by well-intentioned refactors
when the original reasoning is no longer reachable. CLAUDE.md captures
the *rule* but is deliberately terse — it tells you what to do, not why,
and so cannot answer "is this constraint still load-bearing?"

## Decision

Record cross-cutting architectural decisions as numbered ADRs in
`docs/13-decisions/`, using the template in this folder's
[README](README.md). ADRs are immutable once accepted — superseded by
new ADRs, never edited in place.

## Consequences

**Positive:**

- The *why* behind durable patterns is reachable in one place.
- Decisions become first-class artifacts rather than implicit in code or
  buried in PR history.
- The wiki's `rationale/` pages cite ADRs as canonical sources, keeping
  synthesized prose grounded.
- A future contributor weighing "can we refactor this?" can read the ADR
  and decide on informed grounds whether the original forces still apply.

**Negative:**

- One more artifact type to maintain. Mitigated by keeping ADRs short
  (one page) and writing them only for cross-cutting decisions —
  feature-specific rationale stays in proposals.
- Risk of ADRs decaying into trivia ("we use tabs not spaces"). Mitigated
  by the "when to write an ADR" checklist in the README.

**Neutral:**

- Existing decisions are back-filled opportunistically, not in a single
  pass. ADRs accrue when an area is touched and someone notices there is
  no ADR yet. The README lists known candidates.

## Alternatives considered

- **Embed rationale only in proposals.** Works for feature-specific
  decisions but leaves cross-cutting choices (no CGO, backend abstraction,
  MDL syntax shape) homeless — they aren't tied to a single proposal.
- **Rely on CLAUDE.md alone.** CLAUDE.md captures the active *rule* but
  not the context behind it. A future contributor sees the rule and can
  follow it but cannot reason about whether it still applies, or what
  alternatives were rejected.
- **Use the wiki's `rationale/` pages directly.** The wiki is mutable
  synthesis; ADRs are immutable history. Folding them together loses the
  audit trail of when and why a decision was made — and re-synthesis
  could quietly rewrite a decision's original reasoning.
