# ADR-0006: Version-aware MCP capability model

- **Status**: Proposed
- **Date**: 2026-06-11
- **Revised**: 2026-08-11 — see [Revision](#revision-2026-08-11). Amended in place
  rather than superseded because this ADR is still *Proposed*: the core decision
  (probe ∪ table, one source of truth for gate and report) survives intact; only
  the **keying** and the **division of labour** between the two halves changed.
- **Related**: [PROPOSAL_mcp_backend.md](../11-proposals/PROPOSAL_mcp_backend.md), [`docs/03-development/PED_MCP_CAPABILITIES.md`](../03-development/PED_MCP_CAPABILITIES.md), [ADR-0002](0002-backend-abstraction.md)

## Context

The MCP backend authors model changes through Studio Pro's embedded MCP server
("PED"). That server's authoring surface **changes with every Studio Pro version** —
new tools appear (a delete tool, a save tool) and the set of document types
`ped_create_document` accepts (its "create whitelist") expands, but tools are also
**removed** (11.12 dropped `pg_write_page`, 11.13 dropped `oql_generate`) and
existing tools change shape. So what the MCP backend can do is
`f(Mendix version) ∩ f(PED capabilities)`, where the second term moves per release.

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
the union of a live `tools/list` probe and a maintained capability table, split by
what each can actually answer. **Everything observable in `tools/list` — tool
presence and tool input schemas — is probe-only; the table never asserts it.** The
table carries only the non-discoverable facts (the create-whitelist, behavioural
quirks), **keyed on the project's Mendix version**, never on `serverInfo.version`.
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
- **(+) The report stops lying about a togglable tool.** Gating presence on the live
  probe means a tool the user has switched off reads as absent, which is the truth
  for that session.
- **(−) Capability now depends on session state, not just versions.** Two runs against
  the same Studio Pro can report different capabilities if a preference changed or an
  MCP server was connected. That is a faithful model of the system rather than a
  regression, but it means a capability report is only valid for the session that
  produced it, and bug reports must carry it rather than just a version number.
- **(−) The probe becomes load-bearing.** If `tools/list` fails, presence is unknown;
  the gates must fail closed (treat as absent) rather than assume. That trades a
  false "yes" for a false "no", which is the safe direction for a write path.

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
- **Key the table on `serverInfo.version`** (the original form of this ADR).
  Rejected on evidence — see the Revision below.

## Revision (2026-08-11)

Onboarding Studio Pro 11.13 falsified two assumptions in the original decision (1
and 2 below) and widened the scope of a third (3). All were reasonable when
written; none survived contact with three releases.

**1. `serverInfo.version` cannot key anything.** It has read `1.0.0` for 11.11,
11.12 *and* 11.13, while the tool surface changed in every one. The `available_since`
mechanism this ADR designed as its escape hatch ("flip one entry and the feature
lights up") is therefore not merely unused but **structurally dead**: it resolves
through `serverVersionAtLeast(b.server.Version, want)`, which for a frozen `1.0.0`
is false for every `want` above it. No entry uses it today, which is why the defect
went unnoticed. The replacement axis is the **project's Mendix version**, already
the precedent in `gateAttributeDefaults` (`ProjectVersion().IsAtLeast(11,12)`).

**2. The table must not assert tool presence.** The original framing treated presence
as static per version. 11.13 shows it is neither static nor version-derived:

- **Tools are user-togglable.** `oql_generate` disappears when the new "OQL
  Generation" preference is off. A table asserting it is present would be wrong for
  a supported configuration, not merely stale.
- **Studio Pro federates other MCP servers into its own surface.** Its system prompt
  states it: *"Capabilities can be extended via MCP tools provided by the user. MCP
  tools are prefixed `mcp_{serverName}_{toolName}`."* An `mcp_mendix-marketplace_*`
  tool now appears in `tools/list`. So the surface varies per **user configuration**,
  which no version table can model.
- **`tools.listChanged: true`** means it can change *within* a session.

Federated `mcp_*` tools are reported but never gated on: they are third-party tools
whose contract mxcli does not control, and a capability mxcli claims must be one it
can guarantee.

**3. Probing extends to input schemas, not just names.** 11.13 renamed nothing mxcli
calls, yet `pg_read_page` gained a `depth` argument defaulting to 4 that broke ALTER
PAGE — a behaviour change delivered entirely through an existing tool's schema, and
applied by the server whether or not the client knows about it. `Client.SupportsToolArg`
(shipped with that fix) is the general form: an argument mxcli must send is gated on
the live schema, defaulting to *not sent*, because tool schemas are
`additionalProperties:false` and an unknown argument fails the whole call.

The net effect on the split: the probe half **grows** (presence + schemas, and it
becomes the gate rather than decoration), and the table half **shrinks** to what is
genuinely unobservable, re-keyed on the project's Mendix version.
