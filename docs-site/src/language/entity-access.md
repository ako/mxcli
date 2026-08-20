# Entity Access

Entity access rules control which module roles can create, read, write, and delete objects of a given entity. Rules can restrict access to specific attributes and apply XPath constraints to limit which rows are visible.

## GRANT on Entities

```sql
GRANT <Module>.<Role> ON <Module>.<Entity> (<rights>) [WHERE '<xpath>'];
```

The `<rights>` list is a comma-separated combination of:

| Right | Syntax | Description |
|-------|--------|-------------|
| Create | `CREATE` | Allow creating new objects |
| Delete | `DELETE` | Allow deleting objects |
| Read all | `READ *` | Read access to all attributes and associations |
| Read specific | `READ (<attr>, ...)` | Read access to listed members only |
| Write all | `WRITE *` | Write access to all attributes and associations |
| Write specific | `WRITE (<attr>, ...)` | Write access to listed members only |

## Examples

### Full Access

Grant all operations on all members:

```sql
GRANT Shop.Admin ON Shop.Customer (CREATE, DELETE, READ *, WRITE *);
```

### Read-Only Access

```sql
GRANT Shop.Viewer ON Shop.Customer (READ *);
```

### Selective Member Access

Restrict read and write to specific attributes:

```sql
GRANT Shop.User ON Shop.Customer (READ (Name, Email, Status), WRITE (Email));
```

### XPath Constraints

Limit which objects a role can see or modify using an XPath expression in the `WHERE` clause:

```sql
-- Users can only access their own orders
GRANT Shop.User ON Shop.Order (READ *, WRITE *)
  WHERE '[Sales.Order_Customer/Sales.Customer/Name = $currentUser]';

-- Only open orders are editable
GRANT Shop.User ON Shop.Order (READ *, WRITE *)
  WHERE '[Status = ''Open'']';
```

Note that single quotes inside XPath expressions must be doubled (`''`), since the entire expression is wrapped in single quotes.

### Additive Behavior

GRANT is **additive**. If a role already has an access rule on the entity, the new rights are merged in without removing existing permissions:

```sql
-- Initial grant
GRANT Shop.User ON Shop.Customer (READ (Name, Email));

-- Add Notes access — Name and Email are preserved
GRANT Shop.User ON Shop.Customer (READ (Notes));
-- Result: READ (Name, Email, Notes)

-- Upgrade Email to writable — existing reads preserved
GRANT Shop.User ON Shop.Customer (WRITE (Email));
-- Result: READ (Name, Notes), WRITE (Email)
```

Merging only ever widens access — rights rank `None < ReadOnly < ReadWrite`, and a
GRANT takes whichever is higher. To take access away, use [REVOKE](#revoke-on-entities);
that is what makes the two commands inverses rather than two spellings of "set".

### One Rule per Constraint

The constraint is part of a rule's identity. Two GRANTs for the same role with the
**same** `WHERE` update one rule; with **different** constraints they produce two,
which is how row-level security is normally written:

```sql
-- Two separate rules for one role
GRANT Shop.User ON Shop.Order (READ *) WHERE '[Status = ''Open'']';
GRANT Shop.User ON Shop.Order (READ (Total)) WHERE '[Owner = $currentUser]';
```

Mendix combines them at runtime: a role's effective access is the union of every
rule naming it. An unconstrained rule is distinct from a constrained one, so
adding a `WHERE` to an existing GRANT creates a second rule rather than narrowing
the first — and because a rule is matched on its constraint, re-running a script
updates the same rules instead of accumulating new ones.

### Multiple Roles on the Same Entity

Each GRANT creates a separate access rule. An entity can have rules for multiple roles:

```sql
GRANT Shop.Admin ON Shop.Order (CREATE, DELETE, READ *, WRITE *);
GRANT Shop.User ON Shop.Order (READ *, WRITE *) WHERE '[Status = ''Open'']';
GRANT Shop.Viewer ON Shop.Order (READ *);
```

## REVOKE on Entities

Remove an entity access rule entirely or revoke specific rights:

```sql
-- Full revoke (removes entire rule)
REVOKE <Module>.<Role> ON <Module>.<Entity>;

-- Partial revoke (downgrades specific rights)
REVOKE <Module>.<Role> ON <Module>.<Entity> (<rights>);
```

Examples:

```sql
-- Remove all access for Viewer
REVOKE Shop.Viewer ON Shop.Customer;

-- Remove read access on a specific attribute
REVOKE Shop.User ON Shop.Customer (READ (Notes));

-- Downgrade write to read-only on Email
REVOKE Shop.User ON Shop.Customer (WRITE (Email));
```

A full `REVOKE` (without rights list) removes the entire access rule. A partial `REVOKE` downgrades specific rights: `REVOKE READ (x)` sets member x to no access, `REVOKE WRITE (x)` downgrades from ReadWrite to ReadOnly.

## Viewing Entity Access

```sql
-- See which roles have access to an entity (the two spellings are synonyms)
SHOW ACCESS ON ENTITY Shop.Customer;
SHOW ACCESS ON Shop.Customer;

-- Full matrix across a module
SHOW SECURITY MATRIX IN Shop;
```

## See Also

- [Security](./security.md) -- overview of the security model
- [Module Roles and User Roles](./roles.md) -- defining the roles referenced in access rules
- [Document Access](./document-access.md) -- microflow and page access
- [GRANT / REVOKE](./grant-revoke.md) -- complete GRANT and REVOKE reference
