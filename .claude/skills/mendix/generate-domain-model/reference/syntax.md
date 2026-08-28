# Domain model syntax reference

Supporting reference for [generate-domain-model](../SKILL.md).

## MDL Syntax Reference

**CRITICAL: All CREATE statements MUST have JavaDoc-style documentation**

Every CREATE statement (modules, entities, associations, enumerations, microflows) should have a /** ... */ comment explaining its purpose. This is essential for:
- Team collaboration and knowledge transfer
- Understanding domain model structure
- Long-term maintainability
- Auto-generated documentation

### Module Creation

```sql
/**
 * Module for financial transaction management
 *
 * Handles accounts, transactions, budgets, and reporting.
 *
 * @since 1.0.0
 */
create module Finance;
```

### Minimap Section Headers (MARK Comments)

**IMPORTANT: Large MDL files (300+ lines) MUST use MARK comments for navigation**

Use `-- MARK: Section Name` comments to create collapsible sections in code editors. This dramatically improves navigation and organization in large domain model files.

**Format**: `-- MARK: Section Name`

**Required for files:**
- 300+ lines: At least 3 MARK comments
- 500+ lines: At least 5 MARK comments

**Recommended sections:**
```sql
-- MARK: ENUMERATIONS

-- MARK: CORE ENTITIES

-- MARK: ASSOCIATIONS

-- MARK: VIEW ENTITIES

-- MARK: MICROFLOWS
```

**With subsections:**
```sql
-- MARK: - Core Entities (Persistent)

-- MARK: - View Entities for Reporting
```

**Benefits:**
- Creates outline/minimap view in VS Code, Xcode-style editors
- Makes large files navigable with jump-to-section
- Groups related code logically
- Improves team collaboration on complex models

### Enumerations

```sql
/**
 * Transaction type classification
 *
 * Categorizes financial transactions as income or expense
 * for proper accounting and reporting.
 *
 * @since 1.0.0
 */
create enumeration Module.TransactionType (
  INCOME 'Income',
  EXPENSE 'Expense'
);
```

**Editing an existing enumeration** — use `alter enumeration`, never drop + recreate
(a drop is blocked while the enum is referenced by an attribute):

```sql
alter enumeration Module.TransactionType add value REFUND caption 'Refund';
alter enumeration Module.TransactionType rename value INCOME to CREDIT;      -- changes the name/key
alter enumeration Module.TransactionType modify value EXPENSE caption 'Expense / Debit'; -- caption only, name unchanged
alter enumeration Module.TransactionType drop value REFUND;
```

`modify value … caption` re-captions in place — the value keeps its identity, so it
works even while the enumeration is in use. (Value names in `alter` must be plain
identifiers; a value whose name is a reserved word can't be targeted by `alter`.)

### Entities

**IMPORTANT: All entities MUST have @Position annotation**

The `@position(x, y)` annotation specifies where the entity appears in the domain model diagram. Without it, entities appear at (0,0) or random locations.

**Position Guidelines:**
- Use increments of 50 or 100 for spacing (e.g., 100, 200, 300)
- Leave space between entities (at least 200 pixels)
- Organize related entities in logical groups
- Example layout: Categories at y=100, Transactions at y=300, Reports at y=500

**Association line anchors** — where the connector attaches to each entity box —
are set with `@anchor`, as a **percentage of the box** (0..100, whole numbers):

```sql
@anchor(from: (0, 54), to: (100, 54))
create association Sales.Order_Customer
  from Sales.Order to Sales.Customer;
```

`from` is the anchor on the FROM entity's box, `to` the anchor on the TO
entity's. `(0, 50)` is the middle of the left edge, `(100, 50)` the middle of the
right, `(50, 100)` the bottom centre.

Retune a line without restating the association:

```sql
alter association Sales.Order_Customer set anchor from (50, 100) to (50, 0);
```

**Naming an end sets it; not naming one preserves what is stored.** An
association written without `@anchor` keeps whatever the line was dragged to in
Studio Pro, so a `create or modify association` about the delete behaviour never
flattens someone's layout. `describe association` re-emits a non-default pair as
the same `@anchor(...)` annotation, so describe → edit → exec round-trips.

Cross-module associations have no anchors at all — Mendix stores none, and
`set anchor` on one is refused.

#### Persistent Entity

```sql
/**
 * Entity description
 *
 * Detailed explanation of what this entity represents.
 *
 * @since 1.0.0
 * @see Module.RelatedEntity
 */
@position(100, 100)
create persistent entity Module.EntityName (
  /** Unique identifier */
  Id: long not null error 'ID is required' unique error 'ID must be unique',
  /** Attribute description */
  attributename: string(200) not null error 'Attribute name is required',
  /** Numeric value */
  Amount: decimal,
  /** Date field */
  CreationDate: date,
  /** Boolean flag */
  IsActive: boolean not null error 'IsActive flag is required' default true,
  /** Enumeration field */
  status: enumeration(Module.StatusEnum) not null error 'Status is required'
);
```

#### Entity Indexes (Performance Optimization)

**CRITICAL: INDEX syntax goes AFTER the closing parenthesis, with NO comma before**

**Re-running an `alter entity ... add index` needs `if not exists`.** An index has
no name — its columns, in order and direction, are its identity — so a second
`add index on (Level)` is the same index twice, which mxbuild rejects with
**CE0072 "Duplicate indexes"**. The bare form refuses it; `add index if not
exists on (Level)` skips. Dropping works the same way and takes the same
selector: `drop index if exists (Level)`.

Indexes improve query performance for frequently filtered or sorted columns. Add them to persistent entities when:
- Column is used in WHERE clauses frequently
- Column is used for sorting (ORDER BY)
- Composite indexes for multi-column filters

**Syntax:**
```sql
create persistent entity Module.Transaction (
  TransactionDate: datetime not null,
  status: enumeration(Module.Status) not null,
  Amount: decimal not null,
  IsRecurring: boolean default false
)
index (TransactionDate desc)
index (status, TransactionDate)
index (IsRecurring);
```

**Index Guidelines:**
- **Position**: AFTER closing parenthesis, NO comma before first INDEX
- **No names**: Unlike SQL CREATE INDEX, MDL indexes don't have names
- **Sort direction**: ASC or DESC are optional (default is ASC)
- **Composite indexes**: Order matters - put most selective columns first
- **Limit**: Don't over-index - each index has storage/write overhead

**Common index patterns:**
- Date fields: `index (CreatedDate desc)` - for recent-first queries
- Status filters: `index (status, CreatedDate desc)` - for filtered date ranges
- Boolean flags: `index (IsActive)` - for active/inactive filtering
- Foreign keys: Automatically indexed by associations

#### Entity Generalization (EXTENDS)

**CRITICAL: EXTENDS goes BEFORE the opening parenthesis, not after!**

Use `extends` to inherit from a parent entity. Common for file/image storage using System entities.

```sql
-- Correct: EXTENDS before (
create persistent entity Module.ProductPhoto extends System.Image (
  PhotoCaption: string(200),
  SortOrder: integer default 0
);

-- Correct: File document specialization
create persistent entity Module.Attachment extends System.FileDocument (
  AttachmentDescription: string(500)
);

-- Correct: Custom entity inheritance
create persistent entity Module.Employee extends Module.Person (
  EmployeeNumber: string(20)
);
```

**Wrong** (parse error):
```sql
-- EXTENDS after ) = parse error!
create persistent entity Module.Photo (
  PhotoCaption: string(200)
) extends System.Image;
```

**Note:** `mxcli syntax entity` output may show EXTENDS after `)` — this is misleading. Always place EXTENDS before `(`.

**The parent must exist, and must be qualified.** The generalization is stored by
NAME, so mxcli cannot make one up for you and Mendix reports an unresolved one at
build time. `mxcli check --references` resolves it against the project plus the
script:

```sql
-- Fine: the parent is created LATER in the same script. A generalization is
-- resolved lazily, so order does not matter.
create persistent entity Module.Manager extends Module.Person (Reports: integer);
create persistent entity Module.Person (Name: string(100));

-- Refused: no such entity.        CE1613 "The selected entity … no longer exists"
create persistent entity Module.X extends System.Thumbnail (N: integer);

-- Refused: unqualified.           MDL069 — and this one is worse. A bare name is
-- stored as-is and Mendix cannot load the project AT ALL, so `mx check` dies
-- before reporting anything. Write `extends Module.Person`, even when the parent
-- is in the same module.
create persistent entity Module.Manager extends Person (Reports: integer);
```

**Security follows inheritance.** Mendix inheritance is multi-table: all of the
parent's attributes are members of the child, so a specialized entity's access rule
must cover them. Grant an inherited member exactly like one of the entity's own —
`grant Module.Viewer on Module.Attachment (read (AttachmentDescription, "Name", Size));`
— and `read *` / `write *` cover them too. Skipping them is Mendix CE0066 "Entity
access is out of date". The one exception is entities extending `System.User`, whose
inherited platform members Mendix manages and which must not be granted. See
`manage-security`.

#### System Attributes (Auditing)

Mendix supports four built-in auditing properties on persistent entities. Declare them as regular attributes using pseudo-types (like `autonumber`):

| Pseudo-Type | System Attribute | Set When |
|-------------|-----------------|----------|
| `autoowner` | `System.owner` (→ System.User) | Object created |
| `autochangedby` | `System.changedBy` (→ System.User) | Every commit |
| `autocreateddate` | `CreatedDate` (DateTime) | Object created |
| `autochangeddate` | `ChangedDate` (DateTime) | Every commit |

```sql
/**
 * Order with full audit trail
 */
create persistent entity Sales.Order (
  OrderNumber: autonumber default 1,
  TotalAmount: decimal not null,
  status: enumeration(Sales.OrderStatus) not null,
  owner: autoowner,
  ChangedBy: autochangedby,
  CreatedDate: autocreateddate,
  ChangedDate: autochangeddate
);
```

To enable/disable on existing entities, use ALTER ENTITY ADD/DROP ATTRIBUTE:

```sql
alter entity Sales.Order add attribute owner: autoowner;
alter entity Sales.Order add attribute ChangedDate: autochangeddate;
alter entity Sales.Order drop attribute ChangedBy;
```

**When to use auditing:**
- Compliance/regulated domains (finance, healthcare) — use all four
- User-generated content — use AutoOwner for ownership-based access rules
- "Recently modified" lists — use AutoChangedDate
- Avoid on high-volume system tables (every write touches the audit columns)

#### Non-Persistent Entity

**IMPORTANT: Non-persistent entities cannot have validation rules** (`not null error`, `unique error`) on attributes. They can only have `default` values.

```sql
/**
 * Non-persistent entity description
 *
 * @since 1.0.0
 */
@position(200, 100)
create non-persistent entity Module.TemporaryData (
  SessionId: string(100),
  data: string(1000),
  IsActive: boolean default false
);
```

#### View Entity (with OQL)

```sql
/**
 * View entity description
 *
 * @since 1.0.0
 */
@position(300, 500)
create view entity Module.ViewName (
  Attribute1: type,
  Attribute2: type
) as (
  select
    e.Id as Id,
    e.Name as Name,
    e.Amount as Amount
  from Module.Entity as e
  where e.IsActive = true
);
```

**Enumeration Comparisons in OQL:**

When comparing enumeration attributes in OQL WHERE clauses, use the **enumeration value** (identifier), not the caption:

```sql
-- Enumeration definition
create enumeration Module.OrderStatus (
  PENDING 'Pending',
  PROCESSING 'Processing',
  CANCELLED 'Cancelled'
);

-- OQL comparison - use the VALUE, not the caption
where e.Status != 'CANCELLED'   -- Correct: uses enum value
where e.Status != 'Cancelled'   -- Wrong: this is the caption
```

### Entity Event Handlers

Microflows can run before/after entity Create, Commit, Delete, or Rollback. Use the optional `raise error` clause to make a handler act as a validation microflow — if it returns false, the operation is aborted.

```sql
-- In CREATE ENTITY (handlers go after attributes/indexes)
create persistent entity Sales.Order (
  Total: decimal,
  status: string(50)
)
on before commit call Sales.ACT_ValidateOrder raise error
on after create call Sales.ACT_InitDefaults;

-- Add via ALTER ENTITY
alter entity Sales.Order
  add event handler on before delete call Sales.ACT_CheckCanDelete raise error;

-- Drop via ALTER ENTITY
alter entity Sales.Order
  drop event handler on before commit;
```

**Moments**: `before`, `after`
**Events**: `create`, `commit`, `delete`, `rollback`

Each (Moment, Event) combination can only have one handler per entity. The microflow must exist (the executor validates the reference). `raise error` is optional — without it, the handler runs but its return value doesn't affect the operation.

### Associations

**CRITICAL: Association Directionality**

In Mendix, associations are defined **FROM the entity that contains the foreign key TO the entity that is referenced**.

Think of it like this:
- A `Transaction` knows which `Account` it belongs to → Transaction contains the foreign key
- Therefore: `from Transaction to Account`
- **NOT** `from Account to Transaction` ❌

**Common Patterns**:

```sql
-- ❌ INCORRECT: Account doesn't store transaction references
create association Finance.Account_Transaction
from Finance.Account to Finance.Transaction
type reference;

-- ✅ CORRECT: Transaction stores the account reference (foreign key)
create association Finance.Transaction_Account
from Finance.Transaction to Finance.Account
type reference;

-- ✅ One-to-Many: Customer has many Orders (each order knows its customer)
create association Sales.Order_Customer
from Sales.Order to Sales.Customer
type reference;

-- ✅ Many-to-Many: Use ReferenceSet and choose which side stores the relationship
create association Sales.Order_Products
from Sales.Order to Sales.Product
type ReferenceSet
owner both;
```

**Full Association Syntax**:

```sql
/**
 * Association description
 *
 * Explain the relationship and directionality.
 *
 * @since 1.0.0
 */
create association Module.EntityWithFK_ReferencedEntity
from Module.EntityWithFK to Module.ReferencedEntity
type reference
owner default
delete_behavior DELETE_BUT_KEEP_REFERENCES
comment 'Additional documentation';
```

**Idempotency**: plain `create association` is **not** idempotent — re-running it
errors with `association already exists`, which aborts the rest of the script (and
any associations defined *after* it are never created). Two spellings fix that,
and they mean different things:

```sql
-- Converge on this definition, replacing whatever is stored.
create or modify association Module.Child_Parent
from Module.Child to Module.Parent
type reference;

-- Leave an existing association exactly as it is.
create association if not exists Module.Child_Parent
from Module.Child to Module.Parent
type reference;
```

Prefer `if not exists` when the statement is a *delta* rather than the element's
complete definition. `or modify` rebuilds the element from the statement, so a
partial `create or modify entity` drops every attribute it does not list. Writing
both is refused as **MDL067**.

**Association Types**:
- `reference` - One-to-one or many-to-one (foreign key on FROM entity)
- `ReferenceSet` - One-to-many or many-to-many (collection)

**Owner Options**:
- `default` - Standard ownership (FROM entity owns the reference)
- `both` - Both sides own the association (bidirectional)
- `Parent` - Only parent (TO) entity owns
- `Child` - Only child (FROM) entity owns

> **Use `default` ownership for a normal to-one reference.** Reserve `owner both`
> for a `ReferenceSet` (many-to-many). On a plain `type reference`, `owner both`
> makes the association navigable **to-one from *both* sides** — so the reverse
> direction is a single object, not a collection. A **list** widget (listview/
> datagrid/gallery) over the reverse then fails MxBuild **CE8812** "A grid
> association path must result in a list." (a DataView over the reverse is fine —
> it's a single object). With `default` ownership, the reverse is the expected
> to-many collection and a list widget works. See master-detail-pages for the
> widget patterns.

**Delete Behaviors**:
- `DELETE_AND_REFERENCES` - Delete object and all referencing objects
- `DELETE_BUT_KEEP_REFERENCES` - Delete object, keep references (nullify)
- `DELETE_IF_NO_REFERENCES` - Only delete if no objects reference it
- `cascade` - Cascade delete to associated objects
- `prevent` - Prevent deletion if references exist

**Naming Convention**: `{FromEntity}_{ToEntity}` (e.g., `Order_Customer`, `Transaction_Account`)

#### Calculated Attributes

Calculated attributes derive their value from a microflow at runtime. Use `calculated by Module.Microflow` to specify the calculation microflow.

**IMPORTANT: CALCULATED attributes are only supported on PERSISTENT entities.** Using CALCULATED on non-persistent entities will produce a validation error.

```sql
@position(100, 100)
create persistent entity Module.OrderLine (
  /** Unit price */
  UnitPrice: decimal not null,
  /** Quantity ordered */
  Quantity: integer not null,
  /** Total price, calculated by microflow */
  TotalPrice: decimal calculated by Module.CalcTotalPrice
);
```

**Syntax variants:**
- `calculated by Module.Microflow` — recommended, binds the calculation microflow directly
- `calculated Module.Microflow` — also valid (`by` keyword is optional)
- `calculated` — bare form, marks as calculated but requires manual microflow binding in Studio Pro

**The microflow's signature is checked, and mxcli refuses a mismatch before
writing** — Mendix reports these as **CE7247** at build time (verified on 11.13.0):

| Microflow | Result |
|-----------|--------|
| takes the owning entity (`$Order: Module.Order`) | ✅ stored with `PassEntity = true` |
| takes **no** parameter | ✅ stored with `PassEntity = false` — equally valid |
| takes a *different* entity | ❌ refused: CE7247 *"Microflow parameter 'X' should be of type Module.Order."* |
| takes two or more parameters | ❌ refused |
| returns the wrong type | ❌ refused: CE7247 *"Microflow return type should be …"* |
| returns `Long` for an `integer` attribute (or vice versa) | ✅ accepted — Integer and Long are one family here |

A microflow **created earlier in the same script** cannot be inspected yet, so
its signature is not checked; the build has the last word on those.

> **Before mxcli 0.17 the binding was silently discarded** on the default
> engine: the attribute was written as an ordinary stored value, `mx check`
> reported 0 errors, and the attribute stayed empty at runtime (#917). If you
> have attributes that were declared `calculated by` and never calculated, they
> need re-running through a current mxcli — re-executing the same statement is
> enough.

### Data Types

| Type | Example | Description |
|------|---------|-------------|
| `string(length)` | `string(200)` | Text field with max length |
| `integer` | `integer` | 32-bit integer |
| `long` | `long` | 64-bit integer (use for IDs) |
| `decimal` | `decimal` | Decimal number |
| `boolean` | `boolean` | True/false |
| `datetime` | `datetime` | Date and time |
| `date` | `date` | Date only |
| `binary` | `binary` | Binary data |
| `autonumber` | `autonumber default 1` | Auto-incrementing number (requires DEFAULT start value) |
| `enumeration(Module.Enum)` | `enumeration(Shop.Status)` | Enumeration reference |

### Constraints

**Basic Constraints:**
- `not null` - Field is required
- `unique` - Value must be unique
- `default value` - Default value

**Validation Error Messages:**

Each constraint can have a custom error message using `error 'message'` syntax:

```sql
create persistent entity Module.Customer (
  /** Customer name - required with custom error */
  Name: string(200) not null error 'Name is required',
  /** Email - required and unique with separate error messages */
  Email: string(200) not null error 'Email is required' unique error 'Email must be unique',
  /** Age with default value */
  Age: integer default 0,
  /** Active status flag */
  IsActive: boolean not null error 'IsActive flag is required' default true
);
```

**Error Message Guidelines:**
- Place `error 'message'` immediately after the constraint
- Multiple constraints can each have their own error message
- Keep messages clear and user-friendly
- Follow the pattern: `not null error 'X is required'` for required fields
- For UNIQUE: `unique error 'X must be unique'`
- Error messages are shown to end users during validation

**Common patterns:**
```sql
-- Required field
Name: string(200) not null error 'Name is required',

-- Required and unique
Email: string(200) not null error 'Email is required' unique error 'Email must be unique',

-- Required with default
IsActive: boolean not null error 'IsActive flag is required' default true,

-- Enum with required error
status: enumeration(Module.Status) not null error 'Status is required',

-- Enum with default value (use fully qualified Module.Enum.Value)
Priority: enumeration(Module.Priority) default Module.Priority.Normal
```
## Reserved Keywords

**Best practice: Always quote all identifiers** (entity names, attribute names) with double quotes. This escapes every **MDL parser** keyword conflict — quotes are stripped automatically by the parser. So `"create"`, `"status"`, `"end"` become valid attribute names.

```sql
create persistent entity Module."VATRate" (
  "create": datetime,
  "Rate": decimal,
  "status": string(50)
);
```

> **Caveat — quoting does not exempt *platform*-reserved member names.** Some names are
> reserved by the Mendix *platform*, not just the MDL parser, and are rejected **even when
> quoted** (the check strips the quotes and still flags them): `Type` (CE7247, MDL021),
> the audit attributes `CreatedDate` / `ChangedDate` / `Owner` / `ChangedBy` (MDL020 — use
> the `AutoCreatedDate` / `AutoChangedDate` / `AutoOwner` / `AutoChangedBy` pseudo-types
> instead), plus `ID`, `GUID`, `CurrentUser` and the Java-keyword list. `"Type": String`
> fails MDL021 — rename to `ResourceType` / `TypeValue`.

Both `"Name"` and `` `Name` `` syntax are supported. Prefer double quotes for consistency.

**Boolean attributes** auto-default to `false` when no `default` is specified:
```sql
create persistent entity Module.Item (
  IsActive: boolean,           -- auto-defaults to false
  IsPublished: boolean default true
);
```
## Entity Positioning

Use `@position(x, y)` to control layout in Studio Pro:
- Place related entities near each other
- Use consistent spacing (e.g., 250 pixels horizontal, 200 vertical)
- Group by domain concept

Example layout:
```sql
@position(50, 50)      -- Top-left: Core entity
create persistent entity Module.Customer (...);

@position(300, 50)     -- Same row: Related entity
create persistent entity Module.Address (...);

@position(50, 250)     -- Below: Dependent entity
create persistent entity Module.Order (...);
```
