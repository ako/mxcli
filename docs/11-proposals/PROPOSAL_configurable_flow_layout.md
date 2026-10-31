---
title: Configurable Microflow and Nanoflow Layout
status: draft
---

# Proposal: Configurable Microflow and Nanoflow Layout

**Status:** Draft
**Date:** 2026-10-31

## Problem Statement

mxcli lays out every flow it creates (`create microflow` / `create nanoflow` without
`@position`) and every flow it re-arranges (`mxcli layout flows`) with one fixed
style: the TRUE path of each decision runs straight left to right, ELSE branches
drop one lane and run left to right under it, guards put their early return in the
lane below, and a flow wider than `MaxRowWidth` (2880px) wraps back to the left
edge (`mdl/executor/layout.go`, `layout_lanes.go`, `layout_rows.go`).

Teams do not agree on how a flow should look. Conventions seen in practice:

- **"Happy path on one line"** — everything that is not the happy path hangs off it
  (today's style).
- **"TRUE goes right, FALSE goes down"** — the exit side of a decision always says
  which outcome it is, and an ELSE body is a *column* under its decision, so the eye
  reads alternatives top to bottom instead of as a second horizontal line.
- **Wrapping style** — some want long flows on as few rows as possible, some want a
  snake (alternate rows reversed) so no connector runs back across the diagram, some
  never want wrapping.

A Studio Pro user re-arranges by hand to match the house style, and every
`mxcli layout flows` run undoes it. The layout should be a choice the project makes
once.

### Who benefits

- Teams with a modelling style guide, who today hand-fix every generated flow.
- Reviewers: a consistent layout across a module makes flows comparable at a glance.
- Agents generating MDL, which never write `@position` and so always get the default.

## Not an MDL feature, and not a BSON feature

**No new MDL syntax.** The style is a project setting; MDL already has `@position`,
`@anchor`, `@merge` and `@curve` for a flow that deviates from whatever style is in
force.

**No new BSON.** The layout only produces values mxcli already writes:

| Property | On | Meaning |
|---|---|---|
| `RelativeMiddlePoint` | every flow object | centre of the object |
| `Size` | every flow object | width/height (loops are sized to their body) |
| `OriginConnectionIndex` / `DestinationConnectionIndex` | `SequenceFlow` | anchor side: 0 top, 1 right, 2 bottom, 3 left (`AnchorTop`… in `layout.go`) |
| `OriginBezierVector` / `DestinationBezierVector` | `SequenceFlow` | curve control points |

`mxcli layout flows` already patches exactly these into stored BSON and leaves
`$ID`, `GUID` and every other property alone (`cmd_microflows_layout.go`), so the
identity rules in CLAUDE.md are not touched by this proposal.

## Why the setting must be committed project state

Canonical DESCRIBE (ADR-0010, R12; `cmd_microflows_derived_layout.go`) omits every
`@position`/`@anchor`/`@curve` the layout engine *would produce on its own*, by
rebuilding the flow with the engine and keeping only annotations whose node moves.
So "derived" is relative to a style:

- If two developers use different styles, `describe` of the *same* flow prints
  different MDL for each of them — noisy diffs, and `create or modify` from one
  developer's script re-lays out the flow for the other.
- If the style differs from the one a flow was drawn with, DESCRIBE stays **correct**
  by construction: nodes land elsewhere, so their annotations are kept and
  re-executing the description reproduces every stored position. The only cost is
  verbosity.

Therefore the style is read from a **committed** file next to the project, like
`lint-config.yaml` (which `mxcli init` writes to `.claude/`) — not from `~/.mxcli/` and not from `<project>/.mxcli/`, which
`mxcli init` git-ignores because it holds secrets and handshakes
(`cmd/mxcli/constant_gitignore.go`). A per-invocation flag exists only on
`mxcli layout flows`, for trying a style out.

## Proposed configuration

`layout-config.yaml`, looked up the same way `lint-config.yaml` is
(`linter.FindConfigFile`: `<project>/.claude/`, then the project directory) — both
committed locations:

```yaml
# layout-config.yaml — how mxcli lays out microflows and nanoflows
flows:
  preset: happy-line          # happy-line (default) | true-right-false-down | snake
  # Any key below overrides the preset.
  if-else: lanes              # lanes | true-right-else-column
  guard: return-below         # return-below | true-right
  wrap: rows                  # off | rows | snake
  max-row-width: 2880
  error-handler: below        # below | row-end
  spacing:
    horizontal: 160
    vertical: 90
    row-gap: 90
```

A missing file, or a missing key, means today's value. Unknown keys and values are
an error (a typo silently falling back to the default is the failure this file
exists to prevent).

### Presets

| Preset | `if-else` | `guard` | `wrap` | Intent |
|---|---|---|---|---|
| `happy-line` (default) | `lanes` | `return-below` | `rows` | Today's layout, byte for byte |
| `true-right-false-down` | `true-right-else-column` | `return-below` | `rows` | TRUE continues the line, ELSE stacks below |
| `snake` | `lanes` | `return-below` | `snake` | Long flows without connectors back across the canvas |

### `if-else: true-right-else-column`

For an IF with an ELSE (or an explicit empty ELSE):

```
  [act] ──▶ ◇ cond ──true──▶ [A] ──▶ [B] ──▶ ◆ merge ──▶ [next]
            │                              ▲
          false                            │
            ▼                              │
           [C]                             │
            │                              │
            ▼                              │
           [D] ────────────────────────────┘
```

- TRUE leaves the split on the right (`OriginConnectionIndex = 1`); the TRUE body is
  laid out on the centre line exactly as today.
- FALSE leaves the bottom (`OriginConnectionIndex = 2`); the ELSE body is stacked
  **top to bottom** in the split's column, one `ActivityHeight + VerticalSpacing`
  pitch per statement (`ErrorHandlerPitch` where the previous one has a handler
  under it, so its output-variable text is not overlapped — #1598).
- The ELSE's last element connects to the merge from its right side into the
  merge's bottom (`1 → 2`). The merge sits after the wider of the TRUE body and the
  ELSE column.
- An ELSE that ends in RETURN ends in its own end event at the bottom of the column,
  and there is no merge input from it — the same rule as today.
- **Nesting:** a nested IF inside the ELSE column applies the same rule from where it
  stands: its TRUE body runs to the right of the column, its own ELSE stacks below.
  A loop inside the column is placed as a box in the column, sized to its body as
  today. The column's width is the widest of these.
- **Enumeration and inheritance splits** are not affected (they have no TRUE/FALSE).
  Whether they get a matching "first case right, the rest stacked" option is an open
  question.

### `guard`

A guard is `if X then …; return; end if` with no ELSE (`isGuard`, `layout_lanes.go`).

- `return-below` (default) — today: the TRUE branch with its early return goes in the
  lane under the split; the happy path continues on the FALSE exit, horizontally.
- `true-right` — the TRUE branch (early return) continues on the line to the right
  and the happy path leaves the bottom of the split. Consistent with
  `true-right-else-column`, but the happy path turns at every guard. It is a separate
  key on purpose: teams that want TRUE-right for IF/ELSE mostly do not want it for
  guards.

### `wrap`

- `rows` — today (`layout_rows.go`): past `max-row-width` the next row starts at the
  left edge, under everything the current row occupies.
- `snake` — alternate rows run right to left, so the row-to-row connector is a short
  vertical drop at the end of the row instead of a line back across the diagram.
  Branch lanes, guard lanes and error-handler rows are mirrored with their row.
- `off` — never wrap.

### `error-handler`

- `below` — today: a custom error handler's body hangs `ErrorHandlerPitch` under its
  activity, in the lane under the main line.
- `row-end` — the handler's body goes at the end of the current row's lane, reached
  by a connector along the lane; it keeps the area straight under the main line free.
  Lower priority than the rest; may be dropped from the first version.

## CLI

```bash
# Uses layout-config.yaml (or the default)
mxcli layout flows -p app.mpr --module Orders

# Try a style without committing it (dry run shows what would move)
mxcli layout flows -p app.mpr --module Orders --style true-right-false-down --dry-run

# Show the effective configuration and where it came from
mxcli layout config -p app.mpr
```

`--style` takes a preset name; options are only set in the file. Writing with a
`--style` that differs from the file prints a warning that DESCRIBE will keep
positions for the flows it touched (see above).

`create`/`create or modify`/`exec` read the file; there is no per-statement style.

## Implementation Plan

### Phase 0 — separate *placing* from *building* (refactor, no behaviour change)

Today placement is interleaved with construction: `flowBuilder` holds `posX`/`posY`
and each `add*Statement` assigns positions while creating objects
(`cmd_microflows_builder*.go`), with measurement in `layoutMeasurer` (`layout.go`).
A second style written into the same code doubles every branch.

Introduce a `flowLayout` value carried by the builder that owns the decisions a
style changes — where a branch starts, which side a flow leaves and enters, how a
body advances (horizontally or as a column), when to wrap — and route the existing
code through it. The default implementation reproduces today's numbers. This phase
ships alone: **every existing layout test must pass unchanged**, and the
derived-layout DESCRIBE tests are the control that nothing moved.

This is a refactor touching many files, so per CLAUDE.md it is its own PR.

### Phase 1 — configuration plumbing

Load `layout-config.yaml`, validate it, hand the resolved `flowLayout` to the
executor; `mxcli layout config`; `--style` on `layout flows`. The derived-layout
check (`cmd_microflows_derived_layout.go`) must rebuild with the **same** resolved
layout as CREATE, or DESCRIBE silently prints every position.

### Phase 2 — options, one PR each

1. `if-else: true-right-else-column` (the request that started this)
2. `guard: true-right`
3. `wrap: snake` / `off`
4. `error-handler: row-end` (optional)

### Files to modify/create

| File | Change |
|---|---|
| `mdl/executor/layout.go` | Constants become the default `flowLayout`'s fields; measurer asks the layout how a body advances |
| `mdl/executor/layout_lanes.go` | Guard placement behind `guard` option |
| `mdl/executor/layout_rows.go` | `wrap: snake`/`off` |
| `mdl/executor/cmd_microflows_builder.go` | `flowBuilder` carries the `flowLayout` |
| `mdl/executor/cmd_microflows_builder_control.go` | `addIfStatement`: TRUE/FALSE sides and ELSE column |
| `mdl/executor/cmd_microflows_derived_layout*.go` | Rebuild with the project's layout |
| `mdl/executor/cmd_microflows_layout.go` | Accept a layout override for `--style` |
| `mdl/executor/flowlayout_config.go` (new) | Parse/validate `layout-config.yaml`, presets |
| `cmd/mxcli/cmd_layout_flows.go` | `--style` flag; `layout config` subcommand |
| `docs-site/src/` | User manual page: choosing a flow layout |
| `.claude/skills/mendix/` (flow skill) | One line: layout is project config, not `@position` |

## Version Compatibility

None needed. Coordinates, sizes and connection indexes are stored identically in
every Mendix version mxcli supports; nothing here is gated.

## Test Plan

- **Phase 0 control:** the full existing suite (`layout_*_test.go`,
  `cmd_microflows_builder_layout_test.go`, `cmd_microflows_derived_layout_test.go`,
  `cmd_microflows_layout_test.go`) passes with no expectation edited. A diff in any
  expected coordinate fails the refactor.
- **Per option, unit tests on the built graph** (executor, `MockBackend`): positions
  and connection indexes for TRUE/FALSE exits, ELSE column pitch, nested IF in a
  column, loop in a column, ELSE ending in RETURN, guard both ways, snake row
  direction. Each with a control: the same flow under `happy-line` keeps today's
  coordinates.
- **No overlaps:** run `MPR008` (overlapping activities, `mdl/linter/rules/
  mpr008_overlapping_activities.go`) over every flow in `mdl-examples/` laid out
  under each preset; zero findings.
- **DESCRIBE round trip under each preset:** create without `@position` → describe →
  no layout annotations printed → re-exec → identical geometry. And the mismatch
  case: lay out under preset A, describe under preset B → annotations kept → re-exec
  under B reproduces A's geometry exactly.
- **Idempotence:** `layout flows` twice under any preset; the second run writes
  nothing (ADR-0008), with a control that the first run did write.
- **Studio Pro:** open a module laid out under each preset; flows display as laid
  out, `mx check` clean (layout writes no semantic change, so this is a smoke test).

## Alternatives considered

- **A generic graph layout (Sugiyama/layered — Graphviz `dot`, dagre, ELK).** These
  see a plain directed graph and must guess which path is the happy one; the MDL
  syntax tree already knows branches, merges, loops and handlers, which is why the
  structure-driven engine produces flows that look hand-drawn. No pure-Go
  implementation exists (ELK is Java/JS, dagre JS, Graphviz needs CGO or WASM, and
  mxcli is CGO-free). Worth revisiting only as a fallback for flows MDL cannot
  express, which `layout flows` currently skips.
- **Per-user setting.** Rejected: canonical DESCRIBE would differ per developer.
- **A per-flow style annotation in MDL.** Possible later; it would let one module
  mix styles, at the cost of a new annotation every DESCRIBE has to carry. Not
  needed for the stated problem.

## Open Questions

1. File name and shape: a separate `layout-config.yaml`, or a `layout:` section in a
   general project config file if one is introduced?
2. Should enumeration/inheritance splits get an equivalent "first case right, rest
   stacked" option for consistency with `true-right-else-column`?
3. `true-right-else-column` with an empty THEN and a non-empty ELSE: TRUE goes
   straight to the merge — should the merge then sit directly right of the split,
   with the ELSE column under it?
4. Should `layout flows` record the style it used on each flow (e.g. in the flow's
   documentation or a catalog table) so a later DESCRIBE under a different project
   style can explain why it prints positions?
5. Is `error-handler: row-end` wanted by anyone, or drop it?
