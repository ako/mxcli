# CREATE ASSOCIATION

## Synopsis

    CREATE [ OR MODIFY ] ASSOCIATION module.name
        FROM from_module.from_entity
        TO to_module.to_entity
        TYPE { Reference | ReferenceSet }
        [ OWNER { Default | Both | Parent | Child } ]
        [ DELETE_BEHAVIOR { DELETE_BUT_KEEP_REFERENCES | DELETE_AND_REFERENCES | DELETE_IF_NO_REFERENCES | CASCADE | PREVENT } ]

## Description

`CREATE ASSOCIATION` defines a relationship between two entities in the domain model.

The `FROM` entity is the side that owns the foreign key (in database terms). The `TO` entity is the side being referenced. For a typical one-to-many relationship like "a Customer has many Orders," the `FROM` side is the parent entity (`Customer`) and the `TO` side is the child entity (`Order`).

Two association types are supported:

- **Reference** -- a many-to-one relationship. Each object on the TO side references at most one object on the FROM side. Stored as a foreign key column in the TO entity's database table.
- **ReferenceSet** -- a many-to-many relationship. Objects on both sides can reference multiple objects on the other side. Stored in a separate junction table.

The `OWNER` clause controls which side of the association can modify the relationship:

| Owner | Description |
|-------|-------------|
| `Default` | The default owner (child/TO side) can set and clear the reference |
| `Both` | Both sides can modify the association |
| `Parent` | Only the FROM side can modify the association |
| `Child` | Only the TO side can modify the association |

The `DELETE_BEHAVIOR` clause controls what happens when an object on the FROM side is deleted:

| Behavior | Description |
|----------|-------------|
| `DELETE_BUT_KEEP_REFERENCES` | Delete the object and set references to null (the default) |
| `DELETE_AND_REFERENCES` | Delete the object and all associated objects on the TO side |
| `CASCADE` | Alias for `DELETE_AND_REFERENCES` |
| `DELETE_IF_NO_REFERENCES` | Refuse the delete while anything still references the object |
| `PREVENT` | Alias for `DELETE_IF_NO_REFERENCES` |

If `OR MODIFY` is specified, the statement is idempotent: if the association already exists, it is updated to match the new definition.

A documentation comment (`/** ... */`) placed before the statement is preserved as the association's documentation.

## Parameters

**OR MODIFY**
: Makes the statement idempotent. If the association already exists, its properties are updated. Without this clause, creating a duplicate association is an error.

**module.name**
: The qualified name of the association in the form `Module.AssociationName`. By convention, association names follow the pattern `Module.Child_Parent` or `Module.EntityA_EntityB`.

**FROM from_module.from_entity**
: The entity on the "from" side of the relationship. This is the entity that owns the foreign key in the database. In a one-to-many relationship, this is typically the "one" (parent) side.

**TO to_module.to_entity**
: The entity on the "to" side of the relationship. In a one-to-many relationship, this is typically the "many" (child) side that holds the reference.

**TYPE**
: Either `Reference` (many-to-one) or `ReferenceSet` (many-to-many).

**OWNER**
: Which side can modify the association. Defaults to `Default` if omitted.

**DELETE_BEHAVIOR**
: What happens to associated objects when a FROM-side object is deleted. If omitted, references are kept (the default Mendix behavior).

## Examples

### Many-to-one: Order belongs to Customer

```sql
CREATE ASSOCIATION Sales.Order_Customer
    FROM Sales.Customer
    TO Sales.Order
    TYPE Reference
    OWNER Default
    DELETE_BEHAVIOR DELETE_BUT_KEEP_REFERENCES;
```

### Many-to-many: Order has Products

```sql
CREATE ASSOCIATION Sales.Order_Product
    FROM Sales.Order
    TO Sales.Product
    TYPE ReferenceSet
    OWNER Both;
```

### Cascade delete: Invoice deleted with Order

```sql
/** Invoice must be deleted when its Order is deleted */
CREATE ASSOCIATION Sales.Order_Invoice
    FROM Sales.Order
    TO Sales.Invoice
    TYPE Reference
    DELETE_BEHAVIOR DELETE_AND_REFERENCES;
```

### Idempotent with OR MODIFY

```sql
CREATE OR MODIFY ASSOCIATION Sales.Order_Customer
    FROM Sales.Customer
    TO Sales.Order
    TYPE Reference
    OWNER Default;
```

### Cross-module association

```sql
CREATE ASSOCIATION HR.Employee_Department
    FROM HR.Department
    TO HR.Employee
    TYPE Reference;
```

## Notes

- The naming convention `Module.Child_Parent` reflects that the child (TO) entity holds the reference to the parent (FROM) entity. This can be counterintuitive -- the `FROM` entity is the parent, and the `TO` entity is the child.
- Internally, Mendix stores the FROM entity pointer as `ParentPointer` and the TO entity pointer as `ChildPointer`. These BSON field names are inverted relative to what you might expect.
- Entity access rules for associations (member access) must only be added to the FROM entity. Adding them to the TO entity triggers validation errors.

## See Also

[DROP ASSOCIATION](drop-association.md), [CREATE ENTITY](create-entity.md)
