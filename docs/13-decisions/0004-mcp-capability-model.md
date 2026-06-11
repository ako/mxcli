# ADR-0004: Version-aware MCP capability model

- **Status**: Proposed
- **Date**: 2026-06-11
- **Related**: [PROPOSAL_mcp_backend.md](../11-proposals/PROPOSAL_mcp_backend.md), [`docs/03-development/PED_MCP_CAPABILITIES.md`](../03-development/PED_MCP_CAPABILITIES.md), [ADR-0002](0002-backend-abstraction.md)

## Context

The MCP backend authors model changes through Studio Pro's embedded MCP server
("PED"). That server's authoring surface **grows with every Studio Pro version** —
new tools appear (a delete tool, a save tool), and the set of document types
`ped_create_document` accepts (its "create whitelist") expands. So what the MCP
backend can do is `f(Mendix version) ∩ f(PED capabilities)`, where the second term
moves per release.

Two problems follow:

1. **The agent can't tell what's possible.** When an agent drives mxcli against a
   connected Studio Pro, it has no runtime way to know whether — say — a nanoflow
   or a business-event service can be authored against *this* version. There is a
   precedent for the Mendix-version axis (`show features` + the
   `version-awareness` skill tell the agent what MDL the project supports before it
   generates any), but nothing for the PED axis.

2. **PED-limit knowledge is scattered and version-blind.** "PED can't create X"
   lives in hardcoded rejections spread across the backend (`errJavaActionAuthoring`,
   `errNanoflowCreate`, `errBusinessEventAuthoring`, the create-whitelist checks,
   the "delete via Concord" fallbacks). None of it is keyed by version, so when a
   future PED lifts a limit, enabling the feature means hunting down scattered edits.

A complication: capability is only *partly* discoverable at runtime. Tool presence
is in `tools/list`, but the create-whitelist is in no schema — we learned it only by
*attempting* the create and reading the rejection.

## Decision

Model PED authoring capability as a **single source of truth computed on connect**:
the union of a live `tools/list` probe (tool-presence capabilities) and a maintained
**version-keyed capability table** (the create-whitelist and behavioral quirks that
are not schema-discoverable, keyed by MCP `serverInfo.version` / Studio Pro version).
The backend gates all authoring decisions on this model, and the agent-facing
capability report is generated from the same model — so behavior and report cannot
diverge.

## Consequences

- **(+) Multi-version support becomes table-driven.** A new Studio Pro version is
  onboarded by updating one table; the live probe auto-detects new tools. Lifting a
  limit (e.g. PED starts accepting `Microflows$Nanoflow`) flips one entry and the
  feature lights up — no scattered edits.
- **(+) The agent report can't drift.** `show mcp capabilities` (or a backend-aware
  `show features`) reads the same model the backend gates on, so "what it says you
  can do" always equals "what it does."
- **(+) Centralizes scattered knowledge.** The hardcoded per-type rejections collapse
  into `capabilities.canCreate(docType)` / `capabilities.hasTool(name)`.
- **(−) The version table must be maintained.** Some capabilities can't be
  auto-discovered, so onboarding a version still requires probing-by-trying and
  recording the result. Mitigated: this is already the onboarding procedure in
  `PED_MCP_CAPABILITIES.md`; the table just makes it machine-readable.
- **(neutral)** `PED_MCP_CAPABILITIES.md` shifts from being the authority to being
  the human-readable narrative *over* the machine table (kept consistent by the
  onboarding step, or generated from the table).

## Alternatives considered

- **Pure version-number gating** (hardcode "11.12 supports nanoflows"). Brittle —
  every version needs code edits, and it ignores that tool presence is reliably
  live-probeable. Rejected in favor of probe-where-possible.
- **Pure live probe.** Insufficient: the create-whitelist and behavioral quirks are
  not exposed by any tool/schema, so a probe alone can't answer "can I create a
  nanoflow?" without attempting it.
- **Keep the scattered hardcoded rejections.** Doesn't scale across versions and
  gives the agent no report — the status quo this ADR exists to replace.
- **Static per-version capability docs only.** Drifts from behavior and isn't
  machine-consumable by either the backend or the agent.
