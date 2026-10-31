# Microflow and Nanoflow Layout

`mxcli layout flows` re-arranges existing microflows and nanoflows on their
canvas. It is the flow counterpart of [`mxcli layout`](domain-model-layout.md),
and it is how you "reset the layout" of a flow: there is no clause on
`CREATE MICROFLOW` for that, because layout is not part of what a flow *does*.

```bash
mxcli layout flows -p app.mpr Sales.ACT_Order_Submit        # one flow
mxcli layout flows -p app.mpr --module Sales --dry-run      # list what would move
mxcli layout flows -p app.mpr --module Sales                # every flow in a module
```

## The same layout as `CREATE`

There is one layout engine for flows, and this command uses it. A flow laid out
here ends up exactly where the same flow would be if you created it from MDL
with no `@position`: main path left to right, guard branches in the lane below,
long rows wrapped, loops sized to their bodies. See
[Microflow Structure](../language/microflow-structure.md#position) for the rules.

The flow is described to MDL, every layout annotation (`@position`, `@anchor`,
`@curve`, `@merge`, `@start`, and the placement of `@annotation` notes) is
dropped, and it is built again as `CREATE` would build it. Only the geometry of
that build is kept.

## Choosing a layout style

Teams draw flows differently, so the style is a project setting. Put a
`layout-config.yaml` in the project's `.claude/` directory or next to the
`.mpr` and commit it — everyone working on the project then lays flows out the
same way:

```yaml
flows:
  preset: true-right-false-down
```

| Preset | What it draws |
|---|---|
| `happy-line` (default) | The TRUE path on one horizontal line, the ELSE branch in a lane under it, left to right. |
| `true-right-false-down` | The TRUE path on the line; the ELSE branch stacked top to bottom as a column under its decision, rejoining the merge from below. |

A preset is a set of options, and an option overrides its preset:

| Option | Values |
|---|---|
| `if-else` | `lanes` (default), `true-right-else-column` |

An unknown key or value is an error, not a silent fallback to the default.
`mxcli layout config -p app.mpr` shows the style in force and the file it came
from. `CREATE MICROFLOW` / `CREATE NANOFLOW` without `@position` use it too.

To try a style without committing it, pass `--style`:

```bash
mxcli layout flows -p app.mpr --module Sales --style true-right-false-down --dry-run
```

### The style a flow was drawn in travels with it

A flow laid out in a style other than the default gets one line at the end of
its documentation, which Studio Pro shows in the documentation pane:

```
@mxcli-layout: if-else=true-right-else-column
```

`DESCRIBE` and `CREATE OR MODIFY` use that line for that flow, whatever the
project file says now. So a flow drawn in one style describes without `@position`
noise after the project switches to another, and its description rebuilds the
same geometry. Laying a flow out in the default style removes the line. If the
line is deleted or unreadable, nothing breaks: `DESCRIBE` then prints the
positions it can no longer derive, and a warning says why.

## What changes

The result is patched onto the stored flow rather than written in its place, so
nothing but layout changes:

- positions and sizes of activities, events, splits, merges and loops;
- positions of parameters and notes (a note keeps its size);
- the connection sides and curves of sequence flows.

- the `@mxcli-layout:` line in the flow's documentation (above) — the rest of
  the documentation is left byte for byte.

Element IDs, captions, expressions and every property MDL cannot express are
left exactly as stored. That is also why this works on flows drawn in Studio
Pro.

## Flows it will not touch

The rebuilt flow has to match the stored one object for object. If it does
not — the flow uses something MDL cannot yet express the same way, such as
several branches sharing one merge — the flow is skipped with the reason, and
the rest of the batch carries on:

```
FeedbackModule.VAL_Feedback: skipped — its ExclusiveMerge came back as a
ValidationFeedbackAction activity after a rebuild, so it does not round-trip
through MDL
```

A merge with one flow in and one flow out joins nothing, so it is not a reason
to skip: it is placed on the edge it sits on.

## It replaces positions you set by hand

Every flow it touches is re-arranged, including any you arranged yourself. Use
`--dry-run` first. Marketplace modules and `System` are never touched unless
you pass `--include-marketplace`, whether you name a module or a single flow.

Running it again changes nothing: a second run reports `already laid out` and
writes nothing.

## Flags

| Flag | Meaning |
|---|---|
| `Module.Flow ...` | Lay out these microflows or nanoflows. |
| `--module <name>` | Lay out every microflow and nanoflow in this module (repeatable). |
| `--dry-run` | Report what would move, write nothing. |
| `--style <preset>` | Lay out in this preset instead of the project's `layout-config.yaml`. |
| `--include-marketplace` | Also lay out flows in Marketplace modules. A module update replaces them, so this is normally pointless. |

Either name flows or pass `--module`; laying out every flow in a project is too
broad to do by default.
