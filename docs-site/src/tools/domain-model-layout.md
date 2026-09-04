# Domain Model Layout

A domain model generated from an MDL script has to be *placed* somewhere, and
MDL says nothing about placement unless you write `@position` on every entity.
`mxcli layout` arranges a module from its association graph, so you do not have
to.

```bash
mxcli layout -p app.mpr --module Sales --dry-run   # list the moves
mxcli layout -p app.mpr --module Sales             # apply them
mxcli layout -p app.mpr                            # every module the project owns
```

## What it does

Entities are layered on the associations between them:

- an entity that references nothing else in the module is a **lookup**, and goes
  in the leftmost column;
- everything else sits one column past the furthest thing it references;
- entities with **no association at all** — non-persistent helpers, mostly — go
  in a band underneath, rather than being mixed in with the lookups.

Association lines then mostly run one way instead of crossing the diagram.

On a real 16-entity model the layering falls out of the associations with no
hints:

| column | entities |
|---|---|
| 1 | `Department`, `EmploymentType`, `MovementReason`, `PlanType`, `PlanningYear`, `Region` |
| 2 | `Team`, `GoalBucket` |
| 3 | `GoalChange`, `GoalRegionValue`, `PlanScope`, `CapTrackUser` |
| 4 | `Employee`, `ScopeMonth` |
| 5 | `EmployeeMonth`, `Movement` |

## It replaces positions you set by hand

This is the reason it is a command you run rather than something `exec` does on
its own. Inside the modules it touches, every entity is repositioned — including
any you arranged yourself in Studio Pro. Use `--dry-run` first; it prints every
move as `Module.Entity (x, y) -> (x, y)` and writes nothing.

Marketplace modules and `System` are never touched. Naming one explicitly is an
error rather than a silent skip:

```
$ mxcli layout -p app.mpr --module Administration
Administration comes from the Marketplace, and a module update replaces it —
pass --include-marketplace to lay it out anyway
```

## Running it more than once

Positions are a function of the model alone, so the command is safe to leave in
a build script:

- **Idempotent.** A second run reports `already laid out` and does not write, so
  it produces no version-control noise.
- **Local.** Adding an entity and re-running moves only what the new
  relationships require — measured at 3 of 17 for one added entity with one
  association, not a reshuffle of the diagram.

## Flags

| Flag | Meaning |
|---|---|
| `--module <name>` | Lay out this module (repeatable). Default: every module the project owns. |
| `--dry-run` | Print the moves, write nothing. |
| `--include-marketplace` | Also lay out Marketplace modules. A module update replaces them, so this is normally pointless. |

## When you do want explicit positions

`@position(x, y)` on `CREATE ENTITY`, and `ALTER ENTITY … SET POSITION (x, y)`
to move one afterwards, both still work — and `describe entity` emits the stored
position, so arranging a model and describing it back is a way to capture a
layout into MDL.

Two things to know if you place entities yourself:

- The coordinate is the box's **centre**, not its top-left corner.
- An entity created with no position takes the next slot in a wrapping grid.
  That is a default, not a layout: it keeps a large model on screen and stops
  boxes overlapping, but it knows nothing about which entities are related.
