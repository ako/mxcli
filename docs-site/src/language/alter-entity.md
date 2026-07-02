# ALTER ENTITY

`ALTER ENTITY` modifies an existing entity's attributes, indexes, or documentation without recreating it. This is useful for incremental changes to entities that already contain data.

Each `ALTER ENTITY` statement performs **one** action — add, drop, modify, or rename a single attribute; add or drop one index; etc. Apply several changes with several statements.

## ADD Attributes

Add a new attribute with `ADD ATTRIBUTE`:

```sql
ALTER ENTITY Sales.Customer ADD ATTRIBUTE Phone: String(50);
```

New attributes support the same [constraints](./constraints.md) as in `CREATE ENTITY`. Add several with one statement each:

```sql
ALTER ENTITY Sales.Customer ADD ATTRIBUTE LoyaltyPoints: Integer DEFAULT 0;
ALTER ENTITY Sales.Customer ADD ATTRIBUTE MemberSince: DateTime NOT NULL;
```

## DROP Attributes

Remove an attribute with `DROP ATTRIBUTE`:

```sql
ALTER ENTITY Sales.Customer DROP ATTRIBUTE Notes;
```

Drop several with one statement each:

```sql
ALTER ENTITY Sales.Customer DROP ATTRIBUTE Notes;
ALTER ENTITY Sales.Customer DROP ATTRIBUTE TempField;
ALTER ENTITY Sales.Customer DROP ATTRIBUTE OldStatus;
```

## MODIFY Attributes

Change the type or constraints of an existing attribute with `MODIFY ATTRIBUTE`:

```sql
ALTER ENTITY Sales.Customer MODIFY ATTRIBUTE Name: String(400) NOT NULL;
```

## RENAME Attributes

Rename an attribute with `RENAME ATTRIBUTE`:

```sql
ALTER ENTITY Sales.Customer RENAME ATTRIBUTE Phone TO PhoneNumber;
```

## ADD INDEX

Add an index to the entity (the index name is optional):

```sql
ALTER ENTITY Sales.Customer ADD INDEX (Email);
```

Composite indexes, with optional sort direction:

```sql
ALTER ENTITY Sales.Customer ADD INDEX (Name, CreatedAt DESC);
```

## DROP INDEX

Remove an index by name:

```sql
ALTER ENTITY Sales.Customer DROP INDEX idx_customer_email;
```

## SET DOCUMENTATION

Update the entity's documentation text:

```sql
ALTER ENTITY Sales.Customer
  SET DOCUMENTATION 'Customer master data for the Sales module';
```

## ADD/DROP System Attributes

System attributes use the same `ADD ATTRIBUTE` / `DROP ATTRIBUTE` syntax as regular attributes:

```sql
-- Add system attributes
ALTER ENTITY Sales.Order ADD ATTRIBUTE Owner: AutoOwner;
ALTER ENTITY Sales.Order ADD ATTRIBUTE ChangedBy: AutoChangedBy;
ALTER ENTITY Sales.Order ADD ATTRIBUTE CreatedDate: AutoCreatedDate;
ALTER ENTITY Sales.Order ADD ATTRIBUTE ChangedDate: AutoChangedDate;

-- Drop system attributes (by name)
ALTER ENTITY Sales.Order DROP ATTRIBUTE Owner;
ALTER ENTITY Sales.Order DROP ATTRIBUTE ChangedDate;
```

## ADD/DROP EVENT HANDLER

Register microflows to run before or after entity operations:

```sql
-- Before commit: validates and can abort (RAISE ERROR)
ALTER ENTITY Sales.Order
  ADD EVENT HANDLER ON BEFORE COMMIT CALL Sales.ValidateOrder($currentObject) RAISE ERROR;

-- After commit: runs after successful commit (no RAISE ERROR)
ALTER ENTITY Sales.Order
  ADD EVENT HANDLER ON AFTER COMMIT CALL Sales.LogOrderChange($currentObject);

-- Without passing the entity object
ALTER ENTITY Sales.Order
  ADD EVENT HANDLER ON AFTER CREATE CALL Sales.NotifyNewOrder();

-- Remove an event handler
ALTER ENTITY Sales.Order
  DROP EVENT HANDLER ON BEFORE COMMIT;
```

| Moment | Returns | RAISE ERROR | Use case |
|--------|---------|-------------|----------|
| `BEFORE` | Boolean | Yes — aborts on `false` | Validation, permission checks |
| `AFTER` | Void | No | Logging, notifications, side effects |

Events: `CREATE`, `COMMIT`, `DELETE`, `ROLLBACK`

Parameter: `($currentObject)` passes the entity to the microflow, `()` does not.

## Syntax Summary

```sql
ALTER ENTITY <Module>.<Entity> ADD ATTRIBUTE <name>: <type> [constraints];

ALTER ENTITY <Module>.<Entity> DROP ATTRIBUTE <name>;

ALTER ENTITY <Module>.<Entity> MODIFY ATTRIBUTE <name>: <type> [constraints];

ALTER ENTITY <Module>.<Entity> RENAME ATTRIBUTE <old-name> TO <new-name>;

ALTER ENTITY <Module>.<Entity> ADD INDEX [<name>] (<column> [ASC|DESC] [, ...]);

ALTER ENTITY <Module>.<Entity> DROP INDEX <index-name>;

ALTER ENTITY <Module>.<Entity> SET DOCUMENTATION '<text>';

ALTER ENTITY <Module>.<Entity> SET POSITION (<x>, <y>);
```

## See Also

- [Entities](./entities.md) -- CREATE ENTITY syntax
- [Attributes](./attributes.md) -- attribute definition format
- [Indexes](./indexes.md) -- index creation and management
- [Constraints](./constraints.md) -- NOT NULL, UNIQUE, DEFAULT
