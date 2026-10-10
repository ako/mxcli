# MDL Domain Model

This document describes how MDL represents Mendix domain model concepts: entities, attributes, associations, validation rules, indexes, and access rules.

## Table of Contents

1. [Entities](#entities)
2. [Attributes](#attributes)
3. [Validation Rules](#validation-rules)
4. [Indexes](#indexes)
5. [Associations](#associations)
6. [Generalization (Inheritance)](#generalization)
7. [Access Rules](#access-rules)
8. [Event Handlers](#event-handlers)

---

## Entities

### Entity Types

| Type | MDL Keyword | Description |
|------|-------------|-------------|
| Persistent | `persistent` | Stored in database, has table |
| Non-Persistent | `non-persistent` | In-memory only, session-scoped |
| View | `view` | Based on OQL query, read-only |
| External | `external` | From external data source (OData, etc.) |

### Entity Syntax

```sql
[/** <documentation> */]
[@position(<x>, <y>)]
create [or modify] <entity-type> entity <module>.<Name> (
  <attribute-definitions>
)
[<index-definitions>]
[;|/]
```

### Entity Properties

| Property | MDL Representation | Description |
|----------|-------------------|-------------|
| Name | `Module.EntityName` | Qualified name |
| Documentation | `/** ... */` | Documentation comment before CREATE |
| Position | `@position(x, y)` | Location in domain model diagram |
| Persistable | Entity type keyword | Whether stored in database |

### Examples

```sql
/** Persistent entity with all features */
@position(100, 200)
create persistent entity Sales.Customer (
  CustomerId: autonumber not null unique default 1,
  Name: string(200) not null,
  Email: string(200) unique
)
index (Name);
/

/** Non-persistent entity for filtering */
create non-persistent entity Sales.CustomerFilter (
  SearchName: string(200),
  IncludeInactive: boolean default false
);
/

/** View entity with OQL */
create view entity Reports.CustomerStats (
  CustomerName: string,
  OrderCount: integer
) as
  select c.Name, count(o.Id)
  from Sales.Customer c
  join Sales.Order o on o.Customer = c
  GROUP by c.Name;
/
```

---

## Attributes

### Attribute Syntax

```sql
[/** <documentation> */]
<name>: <type> [<constraints>] [default <value>]
```

### Attribute Properties

| Property | MDL Representation | Description |
|----------|-------------------|-------------|
| Name | `attributename:` | Attribute identifier |
| Documentation | `/** ... */` | Doc comment before attribute |
| Type | See [Data Types](./02-data-types.md) | Attribute data type |
| Required | `not null` | Value cannot be empty |
| Unique | `unique` | Value must be unique |
| Default | `default value` | Default value on create |
| Localize (DateTime only) | `not localized` / `localized` | `DomainModels$DateTimeAttributeType.LocalizeDate`. Unstated: true on a new attribute, the stored value on a rewrite. `describe` prints only `not localized` |

### Attribute Ordering

Attributes are defined in order, separated by commas. The last attribute has no trailing comma:

```sql
create persistent entity Module.Entity (
  FirstAttr: string(200),      -- comma after
  SecondAttr: integer,         -- comma after
  LastAttr: boolean            -- no comma
);
```

---

## Validation Rules

Validation rules are expressed as attribute constraints in MDL.

### Supported Validations

| Validation | MDL Syntax | Description |
|------------|------------|-------------|
| Required | `not null` | Attribute must have a value |
| Required with message | `not null error 'message'` | Custom error message |
| Unique | `unique` | Value must be unique across all objects |
| Unique with message | `unique error 'message'` | Custom error message |

### Validation Syntax

```sql
AttrName: type not null [error '<message>'] [unique [error '<message>']]
```

### Examples

```sql
create persistent entity Sales.Product (
  -- Required only
  Name: string(200) not null,

  -- Required with custom error
  SKU: string(50) not null error 'SKU is required for all products',

  -- Unique only
  Barcode: string(50) unique,

  -- Required and unique with custom errors
  ProductCode: string(20) not null error 'Product code required'
                          unique error 'Product code must be unique',

  -- Optional field (no validation)
  description: string(unlimited)
);
```

### Validation Rule Mapping

| MDL | BSON RuleInfo.$Type | Description |
|-----|---------------------|-------------|
| `not null` | `DomainModels$RequiredRuleInfo` | Required validation |
| `unique` | `DomainModels$UniqueRuleInfo` | Uniqueness validation |
| (future) `range(min, max)` | `DomainModels$RangeRuleInfo` | Range validation |
| (future) `regex(pattern)` | `DomainModels$RegexRuleInfo` | Pattern validation |

---

## Indexes

Indexes improve query performance for frequently searched attributes.

### Index Syntax

```sql
index (<column> [asc|desc] [, <column> [asc|desc] ...])
```

### Index Properties

| Property | MDL Syntax | Description |
|----------|------------|-------------|
| Columns | `(col1, col2, ...)` | Indexed columns in order |
| Sort Order | `asc` / `desc` | Sort direction (default: ASC) |

### Examples

```sql
create persistent entity Sales.Order (
  OrderId: autonumber not null unique,
  OrderNumber: string(50) not null,
  CustomerId: long,
  OrderDate: datetime,
  status: enumeration(Sales.OrderStatus)
)
-- Single column index
index (OrderNumber)

-- Composite index
index (CustomerId, OrderDate desc)

-- Multiple indexes
index (status);
/
```

### Index Guidelines

1. **Primary lookups** - Index columns used in WHERE clauses
2. **Foreign keys** - Index association attributes
3. **Sorting** - Index columns used in ORDER BY
4. **Composite order** - Put high-selectivity columns first

---

## Associations

Associations define relationships between entities.

### Association Types

| Type | MDL Keyword | Cardinality | Description |
|------|-------------|-------------|-------------|
| Reference | `reference` | Many-to-One | Child references one parent |
| ReferenceSet | `ReferenceSet` | Many-to-Many | Both can have multiple |

### Association Syntax

**Direction (important):** an association goes **`from` the entity that owns the
foreign key `to` the entity it references**. In a many-to-one, that means
`from <the "many" entity> to <the "one" entity>` — e.g. each `Order` knows its
`Customer`, so `from Order to Customer`. This is the opposite of what many people
expect; getting it backwards passes `mxcli check` but fails the Mendix build with
`CE0854 "not reachable from entity"`.

```sql
[/** <documentation> */]
create association <module>.<AssociationName>
  from <ForeignKeyOwnerEntity>   -- owns the reference (the "many" side)
  to <ReferencedEntity>          -- is referenced (the "one" side)
  type <reference|ReferenceSet>
  [owner <default|both|Parent|Child>]
  [delete_behavior <behavior>]
[;|/]
```

### Association Properties

| Property | MDL Clause | Description |
|----------|------------|-------------|
| Name | `Module.Name` | Association identifier |
| From | `from entity` | The entity that **owns the foreign key** — the "many" side of a many-to-one |
| To | `to entity` | The **referenced** entity — the "one" side |
| Type | `type reference/ReferenceSet` | Cardinality type |
| Owner | `owner` | Which side can modify |
| Delete Behavior | `delete_behavior` | What happens on delete |

### Owner Options

| Owner | Description |
|-------|-------------|
| `default` | Child owns (can set/clear reference) |
| `both` | Both sides can modify the association |
| `Parent` | Only parent can modify |
| `Child` | Only child can modify |

### Delete Behavior Options

| Behavior | MDL Keyword | Description |
|----------|-------------|-------------|
| Keep references | `DELETE_BUT_KEEP_REFERENCES` | Delete object, nullify references |
| Delete referenced too | `CASCADE` | Delete the associated objects too (there is **no** `DELETE_CASCADE`) |
| Only if unreferenced | `DELETE_IF_NO_REFERENCES` | Delete only when nothing references it |
| Delete both | `DELETE_AND_REFERENCES` | Delete this object and the ones it references |
| Prevent | `PREVENT` | Block the delete while a reference exists |

### Examples

```sql
/** Order belongs to Customer (many-to-one): Order owns the FK → from Order to Customer */
create association Sales.Order_Customer
  from Sales.Order
  to Sales.Customer
  type reference
  owner default
  delete_behavior DELETE_BUT_KEEP_REFERENCES;
/

/** Order has many Products (many-to-many) */
create association Sales.Order_Product
  from Sales.Order
  to Sales.Product
  type ReferenceSet
  owner both;
/

/** Invoice must be deleted with its Order */
create association Sales.Order_Invoice
  from Sales.Order
  to Sales.Invoice
  type reference
  delete_behavior CASCADE;
/
```

---

## Generalization

Generalization (inheritance) allows entities to extend other entities.

### Generalization Syntax

```sql
create persistent entity <module>.<Name>
  extends <ParentEntity>
(
  <additional-attributes>
);
```

Both `extends` (preferred) and `generalization` (legacy) keywords are supported. The `extends` keyword can appear before the attribute list or as an entity option after it.

### System Generalizations

Common system entity generalizations:

| Parent Entity | Purpose |
|---------------|---------|
| `System.User` | User accounts |
| `System.FileDocument` | File storage |
| `System.Image` | Image storage |

### Examples

```sql
/** Employee extends User with additional fields */
create persistent entity HR.Employee extends System.User (
  EmployeeNumber: string(20) not null unique,
  Department: string(100),
  HireDate: date
);

/** Image entity for product photos */
create persistent entity Catalog.ProductPhoto extends System.Image (
  caption: string(200),
  SortOrder: integer default 0
);

/** File attachment entity */
create persistent entity Docs.Attachment extends System.FileDocument (
  description: string(500)
);
```

---

## Access Rules

Access rules control entity-level security. They are managed via `grant` and `revoke` statements.

### Syntax

```sql
-- Grant entity access to a module role
grant <rights> on entity <module>.<entity> to <module>.<role> [, ...] [where [<xpath>]];

-- Revoke entity access
revoke <module>.<role> on <module>.<entity>;

-- Show access on an entity
show access on <module>.<entity>;

-- Show security matrix
show security matrix [in <module>];
```

Where `<rights>` is a comma-separated list of:
- `create` — allow creating instances
- `delete` — allow deleting instances
- `read *` — read all members, or `read (<attr>, ...)` for specific attributes
- `write *` — write all members, or `write (<attr>, ...)` for specific attributes

### Examples

```sql
-- Full access
grant create, delete, read *, write * on entity Sales.Customer to Sales.Admin;

-- Read-only
grant read * on entity Sales.Customer to Sales.Viewer;

-- Selective member access
grant read (Name, Email), write (Email) on entity Sales.Customer to Sales.User;

-- With XPath constraint
grant read *, write * on entity Sales.Order to Sales.User where [Status = 'Open'];

-- Revoke
revoke all on entity Sales.Order from Sales.User;
```

### Access Rule Properties

| Property | Description |
|----------|-------------|
| Role | Module role that rule applies to |
| Create | Can create new objects |
| Read | Can read objects (all or specific members) |
| Write | Can modify objects (all or specific members) |
| Delete | Can delete objects |
| XPath | Constraint on which objects (optional) |

---

## Event Handlers

Event handlers trigger microflows on entity lifecycle events.

**Note:** Event handlers are not yet expressible in MDL syntax.

### Planned Syntax

```sql
create persistent entity Sales.Order (
  ...
)
events (
  on create call Sales.Order_OnCreate,
  on commit call Sales.Order_Validate RAISE_ERROR,
  on delete call Sales.Order_OnDelete
);
```

### Event Types

| Event | When Triggered |
|-------|----------------|
| Create | After object is created in memory |
| Commit | Before object is committed to database |
| Delete | Before object is deleted |
| Rollback | When transaction is rolled back |

---

## Complete Domain Model Example

```sql
-- Connect to project
connect local './MyApp.mpr';

-- Create enumeration
create enumeration Sales.OrderStatus (
  Draft 'Draft',
  Pending 'Pending',
  Confirmed 'Confirmed',
  Shipped 'Shipped',
  Delivered 'Delivered',
  Cancelled 'Cancelled'
);

-- Create Customer entity
/** Customer master data */
@position(100, 100)
create persistent entity Sales.Customer (
  CustomerId: autonumber not null unique default 1,
  Name: string(200) not null error 'Customer name is required',
  Email: string(200) unique error 'Email already registered',
  Phone: string(50),
  IsActive: boolean default true,
  CreatedAt: datetime
)
index (Name)
index (Email);
/

-- Create Order entity
/** Sales order */
@position(300, 100)
create persistent entity Sales.Order (
  OrderId: autonumber not null unique default 1,
  OrderNumber: string(50) not null unique,
  OrderDate: datetime not null,
  TotalAmount: decimal default 0,
  status: enumeration(Sales.OrderStatus) default 'Draft',
  Notes: string(unlimited)
)
index (OrderNumber)
index (OrderDate desc);
/

-- Create association (Order owns the FK to Customer → from Order to Customer)
create association Sales.Order_Customer
  from Sales.Order
  to Sales.Customer
  type reference
  owner default
  delete_behavior DELETE_BUT_KEEP_REFERENCES;
/

-- Show result
show entities in Sales;
describe entity Sales.Customer;
describe entity Sales.Order;
describe association Sales.Order_Customer;

commit message 'Created Sales domain model';
```
