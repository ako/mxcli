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

The one addition is a single marker line in the flow's `Documentation` string
property (see [Recording the style on the flow](#recording-the-style-on-the-flow)).
That is a plain string mxcli already reads and writes (`/** … */` in MDL); no new
field.

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

Two mechanisms keep this stable. The project style is a committed file (below),
so everyone lays out new flows the same way. And each flow records the style it
was laid out with ([below](#recording-the-style-on-the-flow)), so DESCRIBE checks
"derived" against the style that actually drew the flow — not against whatever the
project file says today.

Therefore the project style is read from a **committed** file next to the project, like
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
  case-split: lanes           # lanes | first-right-rest-columns
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

| Preset | `if-else` | `guard` | `case-split` | `wrap` | Intent |
|---|---|---|---|---|---|
| `happy-line` (default) | `lanes` | `return-below` | `lanes` | `rows` | Today's layout, byte for byte |
| `true-right-false-down` | `true-right-else-column` | `return-below` | `first-right-rest-columns` | `rows` | TRUE / first case continues the line, alternatives stack below |
| `snake` | `lanes` | `return-below` | `lanes` | `snake` | Long flows without connectors back across the canvas |

`error-handler` is `below` in every preset.

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
- **Empty THEN, non-empty ELSE:** the TRUE flow is still horizontal — it runs
  straight from the split's right side to the merge, which is placed one ordinary
  gap to the right of the split (or right of the ELSE column, if that is wider),
  on the centre line. The ELSE column hangs under the split as usual.
- **Enumeration and inheritance splits** are governed by `case-split` (below), so
  `true-right-false-down` treats every decision the same way.

### `case-split`

Enumeration splits (`case … when …`) and inheritance splits have no TRUE/FALSE, so
the equivalent rule is by case order:

- `lanes` (default) — today: the cases fan out in lanes under the split.
- `first-right-rest-columns` — the **first case in declaration order** leaves the
  split on the right and continues the centre line. Every other case leaves the
  split's bottom and is laid out as its own vertical column, exactly like an ELSE
  column; the columns stand side by side, the second case under the split and each
  further one to the right of the previous, separated by `BranchGap`. Each column
  rejoins the merge from below. The `else`/`empty` case, when present, is the last
  column.

Since the first case is the one on the line, the author picks the happy case by
putting it first — no extra syntax. A multiple-flows-per-side split is valid in
Studio Pro (several flows may share a connection index); still to be confirmed by
opening one, see the test plan.

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
  In scope; scheduled after the other options.

## Recording the style on the flow

Every flow mxcli lays out under a style other than `happy-line` gets one marker
line at the end of its `Documentation`:

```
@mxcli-layout: if-else=true-right-else-column
@mxcli-layout: if-else=true-right-else-column; wrap=snake
```

The value is every option that differs from the default, `key=value` in key
order, so the same settings always produce the same line and a re-layout is
idempotent (ADR-0008). It records options, not the preset name: a preset gains
options as they are implemented, and a marker naming a preset would silently
change meaning for flows already laid out with it.

Rules:

1. **Written by** `create`/`create or modify` (for a flow built without a marker)
   and by `mxcli layout flows`. Any existing marker line is replaced, never
   duplicated; the rest of the documentation is left byte for byte.
2. **No marker means `happy-line`.** Laying a flow out under `happy-line` removes the
   marker. So every flow that exists today, and every flow laid out with the
   default, is unchanged — the Phase 0 control depends on this.
3. **DESCRIBE** prints the documentation as today, marker included, and runs its
   derived-layout check (`cmd_microflows_derived_layout.go`) under the flow's
   **recorded** style. A flow drawn in `true-right-false-down` therefore describes
   with no `@position` noise, whatever the project file now says.
4. **exec** of a `create`/`create or modify` whose `/** … */` carries a marker builds
   the flow under that style; without one, under the project style. That is what
   makes rule 3 round-trip: the described MDL rebuilds the same geometry anywhere.
5. **A missing, edited or unknown marker is never an error.** A Studio Pro user who
   deletes the line, or a marker naming a preset this mxcli does not know, falls
   back to rule 2 with a warning. DESCRIBE then keeps the positions it cannot
   derive, which is still correct by construction — only more verbose.
6. Studio Pro shows the line in the flow's documentation pane. That visibility is
   intended: it tells a Studio Pro user that the layout was generated, and with which
   style.

This changes one statement in the `mxcli layout flows` help text, which today says
only positions change: it also maintains the marker line.

## CLI

```bash
# Uses layout-config.yaml (or the default)
mxcli layout flows -p app.mpr --module Orders

# Try a style without committing it (dry run shows what would move)
mxcli layout flows -p app.mpr --module Orders --style true-right-false-down --dry-run

# Show the effective configuration and where it came from
mxcli layout config -p app.mpr
```

`--style` takes a preset name; options are only set in the file. A flow laid out
with `--style` records that style in its marker, so DESCRIBE stays minimal; the
command warns that the flow now differs from the project style.

`create`/`create or modify`/`exec` use the flow's marker if its documentation has
one, otherwise the project file; there is no other per-statement style.

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
executor; `mxcli layout config`; `--style` on `layout flows`; the `@mxcli-layout`
marker (write, replace, remove, parse with fallback). The derived-layout check
(`cmd_microflows_derived_layout.go`) must rebuild with the flow's **recorded**
layout, or DESCRIBE silently prints every position.

### Phase 2 — options, one PR each

1. `if-else: true-right-else-column` (the request that started this)
2. `guard: true-right`
3. `wrap: snake` / `off`
4. `case-split: first-right-rest-columns`
5. `error-handler: row-end`

### Files to modify/create

| File | Change |
|---|---|
| `mdl/executor/layout.go` | Constants become the default `flowLayout`'s fields; measurer asks the layout how a body advances |
| `mdl/executor/layout_lanes.go` | Guard placement behind `guard` option |
| `mdl/executor/layout_rows.go` | `wrap: snake`/`off` |
| `mdl/executor/cmd_microflows_builder.go` | `flowBuilder` carries the `flowLayout` |
| `mdl/executor/cmd_microflows_builder_control.go` | `addIfStatement`: TRUE/FALSE sides, ELSE column, empty-THEN merge |
| `mdl/executor/cmd_microflows_builder_rule_case.go` and the enum/inheritance split builders | `case-split` columns |
| `mdl/executor/cmd_microflows_derived_layout*.go` | Rebuild with the flow's recorded layout |
| `mdl/executor/flowlayout_marker.go` (new) | Read/write/strip the `@mxcli-layout:` documentation line |
| `mdl/executor/cmd_microflows_layout.go` | Accept a layout override for `--style` |
| `mdl/executor/flowlayout_config.go` (new) | Parse/validate `layout-config.yaml`, presets |
| `cmd/mxcli/cmd_layout_flows.go` | `--style` flag; `layout config` subcommand; help text mentions the marker |
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
  column, loop in a column, ELSE ending in RETURN, empty THEN (merge on the centre
  line right of the split), guard both ways, case-split columns (first case on the
  line, `else` case last), snake row direction. Each with a control: the same flow under `happy-line` keeps today's
  coordinates.
- **No overlaps:** run `MPR008` (overlapping activities, `mdl/linter/rules/
  mpr008_overlapping_activities.go`) over every flow in `mdl-examples/` laid out
  under each preset; zero findings.
- **DESCRIBE round trip under each preset:** create without `@position` → describe →
  marker printed, no layout annotations → re-exec → identical geometry.
- **Marker independence from the project file:** lay out under preset A, change the
  project file to B, describe → still no layout annotations (checked under A, the
  recorded style) → re-exec under project style B reproduces A's geometry exactly.
  Control: the same with the marker line deleted → annotations kept, geometry still
  reproduced.
- **Marker hygiene:** existing documentation preserved byte for byte around the
  marker; re-layout replaces rather than appends; `happy-line` removes it; an
  unknown preset in the marker warns and falls back. A flow with no marker and the
  default style is written byte-identical to today (control for Phase 0).
- **Idempotence:** `layout flows` twice under any preset; the second run writes
  nothing (ADR-0008), with a control that the first run did write.
- **Studio Pro:** open a module laid out under each preset; flows display as laid
  out, `mx check` clean (layout writes no semantic change, so this is a smoke test).
  Specifically confirm a `case-split` with several flows leaving the split's bottom
  displays and edits normally.

## Alternatives considered

- **A generic graph layout (Sugiyama/layered — Graphviz `dot`, dagre, ELK).** These
  see a plain directed graph and must guess which path is the happy one; the MDL
  syntax tree already knows branches, merges, loops and handlers, which is why the
  structure-driven engine produces flows that look hand-drawn. No pure-Go
  implementation exists (ELK is Java/JS, dagre JS, Graphviz needs CGO or WASM, and
  mxcli is CGO-free). Worth revisiting only as a fallback for flows MDL cannot
  express, which `layout flows` currently skips.
- **Per-user setting.** Rejected: canonical DESCRIBE would differ per developer.
- **A per-flow style annotation in MDL syntax** (e.g. `@layout(...)`). The
  documentation marker gives the same per-flow recording without new grammar, and
  it is visible in Studio Pro, where an MDL annotation would not be.
- **Recording the style in the catalog.** The catalog is a rebuildable local index,
  not committed state, so it would be lost on a fresh clone; the documentation
  travels with the flow.

## Resolved Questions

1. **Config file:** a separate `layout-config.yaml`.
2. **Enumeration/inheritance splits:** yes — `case-split: first-right-rest-columns`,
   part of the `true-right-false-down` preset.
3. **Empty THEN:** the TRUE flow stays horizontal, straight to a merge on the centre
   line.
4. **Recording the style:** yes, as an `@mxcli-layout:` line in the flow's
   documentation; DESCRIBE and exec use it.
5. **`error-handler: row-end`:** kept in scope.

## Open Questions

None at the moment. Checked while writing: the visitor keeps an
`@mxcli-layout:` line inside `/** … */` as plain documentation text — a microflow
documented `Submits.` plus the marker parses to
`Documentation = "Submits.\n@mxcli-layout: true-right-false-down"` — so the
marker is not mistaken for an MDL annotation.
