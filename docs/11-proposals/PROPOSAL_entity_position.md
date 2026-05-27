---
title: Entity Positioning in ALTER ENTITY
status: draft
---

# Proposal: Entity Positioning in ALTER ENTITY

## Problem

Entity positions in the domain model canvas can only be set during `create entity` via the `@position` annotation. There's no way to reposition existing entities without dropping and recreating them — which destroys associations, access rules, and references.

This matters for:
- **Scripts that create multiple related entities** — auto-positioning stacks them linearly, making the domain model hard to read in Studio Pro
- **Domain model cleanup** — reorganizing entity layout after schema evolution
- **Auto-layout** — enabling tools to compute optimal positions and apply them in a single pass

## Proposed Syntax

### Single entity

```sql
alter entity Module.Customer set position (100, 200);
```

Consistent with existing `alter entity ... set documentation '...'` and `set comment '...'` patterns.

### Multiple entities (batch repositioning)

```sql
alter entity Module.Customer set position (100, 100);
alter entity Module.Order    set position (400, 100);
alter entity Module.Product  set position (400, 300);
```

### Auto-layout command (future extension)

A separate top-level command for automatic layout of all entities in a module:

```sql
ARRANGE DOMAIN model in module;
```

This is out of scope for the initial implementation but the grammar should not conflict with it.

## Grammar Change

Add one alternative to the `alterEntityAction` rule:

```ebnf
alterEntityAction
    : ...existing alternatives...
    | set position LPAREN NUMBER_LITERAL COMMA NUMBER_LITERAL RPAREN
    ;
```

`position` is already a keyword in the lexer (used by `@position` annotations and notebook actions).

## AST Change

Add a new operation constant and position field to `AlterEntityStmt`:

```go
const (
    ...
    AlterEntitySetPosition  // set position (x, y)
)

type AlterEntityStmt struct {
    ...
    position  *position  // for set position
}
```

## Executor Change

In `cmd_entities.go`, handle `AlterEntitySetPosition`:

1. Find the entity in the domain model
2. Update `entity.Location = model.Point{X: s.Position.X, Y: s.Position.Y}`
3. Write the updated entity back via `writer.UpdateEntityLocation(entityID, location)`

This is a lightweight operation — it only updates the `Location` field in the BSON, no structural changes.

## DESCRIBE Output

`describe entity` should include a `@position` annotation when the entity has a non-default position, so the output is round-trippable:

```sql
@position(100, 200)
create or replace persistent entity Module.Customer (
    Name: string(100),
    ...
);
```

Currently DESCRIBE omits the `@position` annotation. This should be added regardless of whether `alter entity set position` is implemented, since it improves roundtrip fidelity.

## Implementation Scope

| Component | Change |
|-----------|--------|
| `MDLParser.g4` | Add `set position (x, y)` to `alterEntityAction` |
| `MDLLexer.g4` | No change (`position` already exists) |
| `mdl/ast/ast_entity.go` | Add `AlterEntitySetPosition` op, `position` field |
| `mdl/visitor/visitor_entity.go` | Parse the new alternative |
| `mdl/executor/cmd_entities.go` | Handle `AlterEntitySetPosition` |
| `sdk/mpr/writer.go` | Add `UpdateEntityLocation()` (if not already present) |
| DESCRIBE output | Add `@position` annotation |

## Alternatives Considered

**`@position` annotation on ALTER ENTITY** — e.g., `@position(100,200) alter entity ...`. Rejected because annotations are a CREATE-time concept; SET is the established ALTER pattern.

**Dedicated MOVE POSITION command** — e.g., `move entity Module.E to position (x, y)`. Rejected because MOVE already means "move to different module" in MDL.

**ARRANGE command only** — Skip per-entity positioning, just auto-layout. Insufficient for scripts that need precise control (e.g., aligning entities in a specific pattern).
