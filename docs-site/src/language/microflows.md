# Microflows and Nanoflows

Microflows and nanoflows are the primary logic constructs in Mendix applications. They define executable sequences of activities -- retrieving data, creating objects, calling services, showing pages, and more. In MDL, you create and manage them with declarative SQL-like syntax.

## What is a Microflow?

A microflow is a server-side logic flow. It executes on the Mendix runtime, has access to the database, can call external services, and supports transactions with error handling. Microflows are the workhorse of Mendix application logic.

Typical uses:
- CRUD operations (create, read, update, delete objects)
- Validation logic before saving
- Calling external web services or Java actions
- Batch processing and data transformations
- Scheduled event handlers

## What is a Nanoflow?

A nanoflow is a client-side logic flow. It executes in the user's browser (or on mobile devices), providing fast response times without server round-trips. Nanoflows have a more limited set of available activities -- they cannot access the database directly or use Java actions.

Typical uses:
- Client-side validation
- Showing/closing pages
- Changing objects already in memory
- Calling nanoflows or microflows

## Basic Syntax

Both microflows and nanoflows follow the same structural pattern in MDL:

```sql
CREATE MICROFLOW Module.ActionName
FOLDER 'OptionalFolder'
BEGIN
  -- parameters, variables, activities, return
END;
```

```sql
CREATE NANOFLOW Module.ClientAction
BEGIN
  -- activities (client-side subset)
END;
```

## SHOW and DESCRIBE

List microflows and nanoflows in the project:

```sql
SHOW MICROFLOWS
SHOW MICROFLOWS IN MyModule
SHOW NANOFLOWS
SHOW NANOFLOWS IN MyModule
```

View the full MDL definition of an existing microflow or nanoflow (round-trippable output):

```sql
DESCRIBE MICROFLOW MyModule.ACT_CreateOrder
DESCRIBE NANOFLOW MyModule.NAV_ShowDetails
```

## DROP

Remove a microflow or nanoflow:

```sql
DROP MICROFLOW MyModule.ACT_CreateOrder;
DROP NANOFLOW MyModule.NAV_ShowDetails;
```

## OR REPLACE

Use `CREATE OR REPLACE` to overwrite an existing microflow or nanoflow if it already exists, or create it if it does not:

```sql
CREATE OR REPLACE MICROFLOW MyModule.ACT_CreateOrder
BEGIN
  -- updated logic
END;
```

### Document properties

Four microflow properties live in the header, between the signature and `begin`:

```sql
CREATE OR MODIFY MICROFLOW Shop.ACT_ShowOrder ($Order: Shop.Order, $Tab: String)
URL 'order/{Order}'
URL SEARCH PARAMETERS ($Tab)
EXPORT LEVEL API
DISALLOW CONCURRENT EXECUTION ERROR MESSAGE 'This order is already being processed'
BEGIN
  -- ...
END;
```

| Clause | Sets | Clear with |
|---|---|---|
| `URL 'order/{Order}'` | the deep link (Mendix 10.6+) | `DROP URL` |
| `URL SEARCH PARAMETERS ($Tab)` | parameters passed as query arguments | `DROP URL` |
| `EXPORT LEVEL API` | the module's public surface on export | `EXPORT LEVEL HIDDEN` |
| `DISALLOW CONCURRENT EXECUTION ERROR MESSAGE '…'` | what a second caller gets | `ALLOW CONCURRENT EXECUTION` |

`ERROR MICROFLOW Module.Name` is the other form of the last one.

**An omitted clause preserves what is stored.** A rewrite that only changes the
body leaves every one of them alone — the same rule as `@excluded` and
`@applyentityaccess`. Clearing is always explicit.

Studio Pro's **Mark as used** has no clause; it is carried across a rewrite and
cannot be set from MDL.

#### Rules checked before the write

These are Mendix constraints that otherwise surface only when you build:

- Every `{Name}` must name a parameter of this microflow (**MDL-MF01**). A
  segment may hold an attribute path — `{Customer/Name}` binds the **Customer**
  parameter — so the leading identifier is what matches.
- A parameter used in the **path** may not also be a **search parameter**
  (**MDL-MF02**, mxbuild's CE5612). The two sets are disjoint.
- `DISALLOW CONCURRENT EXECUTION` needs a message or a microflow
  (**MDL-MF03**, CE4899).
- Two microflows may not share a URL (CE0570). Needs `-p`, since a script alone
  cannot see the other microflows.

#### Copying a microflow

`DESCRIBE` emits all four clauses, so **describe → rename → exec copies them
faithfully** — that is what the clauses are for. Give the copy a different `URL`,
or the build fails with CE0570.

`DROP` followed by `CREATE` is a new document and keeps nothing unless the script
restates it.

An error message is translatable and MDL states one language. Rewriting the same
microflow keeps the others; a **copy** gets only the one shown, and `DESCRIBE`
says so on the line.

## Folder Organization

Place microflows and nanoflows into folders for project organization:

```sql
CREATE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
  -- ...
END;
```

Move an existing microflow to a different folder:

```sql
MOVE MICROFLOW Sales.ACT_CreateOrder TO FOLDER 'Orders/Actions';
MOVE NANOFLOW Sales.NAV_ShowDetail TO FOLDER 'Navigation';
```

Nested folders use `/` as the separator. Missing folders are created automatically.

## Quick Example

```sql
CREATE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
  DECLARE $Order Sales.Order;
  $Order = CREATE Sales.Order (
    OrderDate = [%CurrentDateTime%],
    Status = 'Draft'
  );
  COMMIT $Order;
  SHOW PAGE Sales.Order_Edit ($Order = $Order);
  RETURN $Order;
END;
```

This microflow creates a new Order object with default values, commits it to the database, opens the edit page, and returns the new order.

## Next Steps

- [Structure](microflow-structure.md) -- parameters, variables, return types, and the full `CREATE MICROFLOW` syntax
- [Activity Types](activity-types.md) -- all available activities (RETRIEVE, CREATE, CHANGE, COMMIT, DELETE, CALL, LOG, etc.)
- [Control Flow](control-flow.md) -- IF/ELSE, LOOP, WHILE, error handling
- [Expressions](expressions.md) -- expression syntax for conditions and calculations
- [Nanoflows vs Microflows](nanoflows.md) -- differences and nanoflow-specific syntax
- [Common Patterns](microflow-patterns.md) -- CRUD, validation, batch processing, and anti-patterns to avoid
