# DESCRIBE STRUCTURE

The `DESCRIBE STRUCTURE` command gives you a compact, tree-style overview of a project. It is the fastest way to understand the overall shape of an application -- what modules exist, what types of documents they contain, and how large each module is.

## Default view

With no arguments, `DESCRIBE STRUCTURE` displays all user modules at depth 2 -- modules with their documents listed by type:

```sql
DESCRIBE STRUCTURE;
```

Example output:

```
MyFirstModule
  Entities
    Customer (Name, Email, Phone, DateOfBirth, IsActive)
    Order (OrderNumber, OrderDate, TotalAmount, Status)
    OrderLine (Quantity, UnitPrice, LineTotal)
  Microflows
    ACT_Customer_Save ($Customer: Customer) : Boolean
    ACT_Order_Process ($Order: Order) : Boolean
    DS_Customer_GetAll () : List of Customer
  Pages
    Customer_Overview
    Customer_Edit ($Customer: Customer)
    Order_Detail ($Order: Order)
  Enumerations
    OrderStatus (Draft, Submitted, Approved, Rejected, Completed)
Administration
  Entities
    Account (FullName, Email, IsLocalUser, BlockedSince)
  Microflows
    ChangeMyPassword ($OldPassword, $NewPassword) : Boolean
  Pages
    Account_Overview
    Login
```

This gives you a bird's-eye view without opening individual elements. Entity signatures show attribute names; microflow signatures show parameters and return types.

## Depth levels

The `DEPTH` option controls how much detail is shown:

### DEPTH 1 -- Module summary

Shows one line per module with element counts:

```sql
DESCRIBE STRUCTURE DEPTH 1;
```

```
MyFirstModule       3 entities, 3 microflows, 3 pages, 1 enumeration, 2 associations
Administration      1 entity, 1 microflow, 2 pages
```

This is useful for getting a quick sense of project size and where the complexity lives.

### DEPTH 2 -- Elements with signatures (default)

This is the default when you run `DESCRIBE STRUCTURE` with no depth specified. It shows modules, their documents grouped by type, and compact signatures for each element. See the example under [Default view](#default-view) above.

### DEPTH 3 -- Full detail

Shows typed attributes and named parameters:

```sql
DESCRIBE STRUCTURE DEPTH 3;
```

```
MyFirstModule
  Entities
    Customer
      Name: String(200)
      Email: String(200)
      Phone: String(50)
      DateOfBirth: DateTime
      IsActive: Boolean DEFAULT false
    Order
      OrderNumber: AutoNumber
      OrderDate: DateTime
      TotalAmount: Decimal
      Status: MyFirstModule.OrderStatus
    OrderLine
      Quantity: Integer
      UnitPrice: Decimal
      LineTotal: Decimal
  Microflows
    ACT_Customer_Save ($Customer: MyFirstModule.Customer) : Boolean
    ACT_Order_Process ($Order: MyFirstModule.Order) : Boolean
    DS_Customer_GetAll () : List of MyFirstModule.Customer
  Pages
    Customer_Overview
    Customer_Edit ($Customer: MyFirstModule.Customer)
    Order_Detail ($Order: MyFirstModule.Order)
  Enumerations
    OrderStatus
      Draft 'Draft'
      Submitted 'Submitted'
      Approved 'Approved'
      Rejected 'Rejected'
      Completed 'Completed'
  Associations
    Order_Customer: Order -> Customer (Reference)
    OrderLine_Order: OrderLine -> Order (Reference)
```

Depth 3 is verbose but gives you the most complete picture without running individual DESCRIBE commands.

At depths 2 and 3 each page also lists its outermost data widgets, the ones that set up a data context, for example `Page MyFirstModule.Customer_Edit [DataView<Customer>]`. A data widget nested inside another one is not listed. The widget index is built only by `REFRESH CATALOG FULL`, so on a default catalog pages print without the list.

## Filtering by module

Use `IN` to show only a single module:

```sql
DESCRIBE STRUCTURE IN MyFirstModule;
```

This produces the same tree format but limited to one module. Combine with `DEPTH` for control over detail:

```sql
DESCRIBE STRUCTURE DEPTH 3 IN MyFirstModule;
```

## Including system modules

By default, system and marketplace modules are hidden. Add `ALL` to include them:

```sql
DESCRIBE STRUCTURE DEPTH 1 ALL;
```

```
MyFirstModule       3 entities, 3 microflows, 3 pages, 1 enumeration, 2 associations
Administration      1 entity, 1 microflow, 2 pages
Atlas_Core          0 entities, 0 microflows, 12 pages
System              15 entities, 0 microflows, 0 pages
```

This is useful when you need to see system entities (like `System.Image` or `System.FileDocument`) or check what a marketplace module provides.

## Using DESCRIBE STRUCTURE from the command line

```bash
# Quick project overview
mxcli -p app.mpr -c "DESCRIBE STRUCTURE DEPTH 1"

# Detailed view of one module
mxcli -p app.mpr -c "DESCRIBE STRUCTURE DEPTH 3 IN Sales"

# Full project including system modules
mxcli -p app.mpr -c "DESCRIBE STRUCTURE ALL"
```

## When to use DESCRIBE STRUCTURE vs SHOW + DESCRIBE

| Goal | Command |
|------|---------|
| "What modules are in this project?" | `DESCRIBE STRUCTURE DEPTH 1` |
| "What does module X contain?" | `DESCRIBE STRUCTURE IN X` |
| "List all entities (just names)" | `LIST ENTITIES` |
| "What are the attributes of entity X?" | `DESCRIBE ENTITY X` |
| "Give me a complete overview of everything" | `DESCRIBE STRUCTURE DEPTH 3` |

`DESCRIBE STRUCTURE` is best for orientation -- understanding the shape of the project at a glance. For detailed work on specific elements, switch to `DESCRIBE`.

## What is next

Now that you can explore a project, you are ready to start making changes. Continue to [Your First Changes](first-changes.md) to learn how to create entities, microflows, and pages using MDL.
