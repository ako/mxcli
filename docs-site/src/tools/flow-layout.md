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

## Only positions change

The result is patched onto the stored flow rather than written in its place, so
nothing but layout changes:

- positions and sizes of activities, events, splits, merges and loops;
- positions of parameters and notes (a note keeps its size);
- the connection sides and curves of sequence flows.

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
| `--include-marketplace` | Also lay out flows in Marketplace modules. A module update replaces them, so this is normally pointless. |

Either name flows or pass `--module`; laying out every flow in a project is too
broad to do by default.
