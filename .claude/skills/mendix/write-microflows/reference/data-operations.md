# Objects, lists, database and XPath

Supporting reference for [write-microflows](../SKILL.md).

## Object Operations

### CREATE Object

```mdl
$NewProduct = create Test.Product (
  Name = $Name,
  Code = $Code,
  IsActive = true,
  CreateDate = [%CurrentDateTime%]);
```

**Syntax Rules:**
- Variable assignment on left side (`$NewProduct =`)
- Entity type is fully qualified
- Attributes in parentheses, comma separated
- Closing `)` followed by semicolon
- Syntax aligned with CALL MICROFLOW/CALL JAVA ACTION

**The Commit flag** (Studio Pro's "Commit" dropdown) is the optional `commit`
modifier after the member list:

```mdl
$Order = create Sales.Order (Number = $Nr);                      -- Commit: No (default)
$Order = create Sales.Order (Number = $Nr) commit;               -- Commit: Yes
$Order = create Sales.Order (Number = $Nr) commit without events;-- Commit: YesWithoutEvents
```

Omit it for the default. This is a **modifier on the create**, not the separate
`commit $Var;` activity — see COMMIT Object below for that one.

### CHANGE Object

```mdl
change $Product (
  Name = $NewName,
  ModifiedDate = [%CurrentDateTime%]);

-- Commit the changed object as part of the change activity
change $Product (Name = $NewName) commit;
change $Product (Name = $NewName) commit without events;

-- Refresh the changed object in the client
change $Product (Name = $NewName) refresh;

-- Both: commit comes first
change $Product (Name = $NewName) commit refresh;
```

`refresh` is available on `create` and `delete` too — it is the activity's
"Refresh in client" property, and it defaults to No everywhere:

```mdl
$Order = create Sales.Order (Number = $Nr) refresh;
delete $Order refresh;
```

**Note**: Only specify attributes you want to change. Syntax aligned with CREATE.

### COMMIT Object

```mdl
-- Commit. Events are ON — this is Mendix's default and Studio Pro's.
commit $Product;

-- Turn the event handlers off. This is the only form that changes anything.
commit $Product without events;

-- Commit with refresh in client (updates UI after commit)
commit $Product refresh;

-- Both
commit $Product without events refresh;
```

**An omitted modifier means Mendix's default, on every activity.** For `commit`
that default is events **ON**, so a bare `commit $Product;` runs the before/after
commit handlers — same as dragging a fresh Commit activity onto the canvas. Reach
for `without events` only when you deliberately want them skipped (a bulk import
that would otherwise fire a handler per row is the usual reason).

`with events` still parses and means exactly the same as writing nothing. It is
accepted because every script written before mxcli #895 spells it out, and
because saying the default out loud is not an error — but `describe` prints the
bare form, so it will disappear from round-tripped output.

> **This changed in #895.** Before the fix a bare `commit $Product;` wrote events
> **OFF**, and nothing said so: `mxcli check`, `mxcli lint`, Studio Pro's
> consistency check and `mxbuild` were all clean, because a commit that skips its
> handlers is a valid model. If a script of yours relies on the old behaviour,
> write `without events` explicitly — `mxcli check` prints an MDL067 note naming
> each microflow with a bare commit to help you find them.

**Best Practice**: Use `refresh` when the committed object is displayed in the client and you want the UI to update immediately.

> **Re-sorting a database-datasource grid needs `refresh`.** A plain `commit $Obj;` updates the committed attribute *values* in the grid, but a grid backed by a **database** datasource does **not** re-run its sort — so after changing a sort key (e.g. a reorder that rewrites a `SequenceNumber`), the row stays in its old position until you `commit $Obj refresh;`. The `refresh` re-queries the datasource, which re-applies the sort. (Ledger #57.)

> **Binding a microflow to an entity event is MDL — you do NOT need to map it manually in Studio Pro.** After writing a handler microflow (e.g. a `BeforeCommit` validation), wire it directly:
> ```mdl
> alter entity Sales.Order
>   add event handler on before commit call Sales.ACT_ValidateOrder($currentObject) raise error;
> ```
> Works for `before`/`after` × `create`/`commit`/`delete`/`rollback`, and inside `create entity` too. See [generate-domain-model](../../generate-domain-model/SKILL.md) for the full syntax. (There is no need for an `EVT_*` naming convention plus a manual Studio Pro mapping step.)
## List Operations

```mdl
-- Existing variable form
add $Item to $Items;

-- Expression-valued add, useful when round-tripping Studio Pro list-add values
add head($SourceItems) to $Items;
```

Use expression-valued `add` only when the expression returns an object compatible with the target list element type.

### `range` — paging a list

`range` takes the **offset first, then the amount**, and Mendix requires at
least one of them:

```mdl
$Page  = range($Sorted, $Offset, $PageSize);  -- skip $Offset, take $PageSize
$First = range($Sorted, 0, 10);               -- first 10
$Rest  = range($Sorted, $Offset);             -- skip $Offset, take the rest
```

`range($List)` with no bound builds nothing useful and fails with **CE6520**
("Amount and offset are not specified. Either amount or offset or both must be
specified."); `mxcli check` refuses it first as **MDL068**. To use the whole
list, drop the activity and use the list variable directly.

`range` is a *list* operation — it pages a list that is already in memory, so
every row was fetched first. To page at the database instead, put the bounds on
the retrieve, where the rows never leave the database:

```mdl
retrieve $Page from Sales.Order where [Status = 'Open']
  sort by OrderDate desc limit $PageSize offset $Offset;
```

Note the clause order there is `limit` then `offset` — the reverse of `range`'s
argument order, because each mirrors the Mendix editor it comes from.

### `contains` is overloaded — string vs list

`contains(a, b)` is both a **string** function (`contains(haystack, needle)` → substring test) and a **list** operation (`contains(list, object)` → membership test). mxcli picks the right serialization automatically:

```mdl
-- STRING contains — assign to a PRE-DECLARED Boolean (a Change Variable action)
declare $HasAt Boolean = false;
set $HasAt = contains($Email, '@');

-- LIST contains — do NOT pre-declare the output (the list op creates it)
set $Found = contains($Items, $Item);
```

The distinction: a **literal or computed** second argument is always the string function. When both arguments are plain variables, the input variable's declared type decides — a **String** input becomes the string function (Change Variable, so declare the Boolean first), anything else stays a list operation (which creates its own output variable, so leave it undeclared). Getting the declare wrong is what triggers `CE0111 "Duplicate variable name"`.
## Database Operations

### RETRIEVE Statement

```mdl
-- Retrieve all
retrieve $ProductList from Test.Product;

-- Retrieve with WHERE
retrieve $ProductList from Test.Product
  where Code = $SearchCode;

-- Retrieve with multiple conditions
retrieve $ProductList from Test.Product
  where IsActive = true
    and Price > 100;

-- Retrieve single object
retrieve $Product from Test.Product
  where Code = $ProductCode;
```

**Important**:
- Use `from Module.Entity` (fully qualified)
- RETRIEVE with `limit 1` returns a **single entity**
- RETRIEVE without `limit 1` returns a **list** (`list of Module.Entity`)
- Use `limit 1` when you expect exactly one result (e.g., lookup by unique key)

**Sorting and paging** — use `sort by`, **not** `order by`:

```mdl
retrieve $Recent from Sales.Order
  where Status = Sales.OrderStatus.Open
  sort by Sales.Order.OrderDate desc, Sales.Order.OrderNumber asc
  limit $PageSize
  offset $Offset;
```

- The keyword is **`sort by`** (one or more `Module.Entity.Attr asc|desc`, comma-separated). `order by` is **not** valid on a microflow `retrieve` — it's reserved for `select ... from CATALOG.*` queries and will cause a parse error here.
- `limit` and `offset` accept **a variable or expression**, not only a literal — `limit $PageSize`, `offset $Offset`, even `limit $Base + 5` all work. A bare literal (`limit 20`) is just the simplest case.

### Retrieve by Association (in-memory, over an association path)

To get the object(s) related to one you already have, retrieve **over an
association** — `retrieve $out from $source/Module.Association;`. This is an
*association* (in-memory) retrieve, not a database query, so it has no `where` /
`sort by` / `limit`. **The result type depends on the direction you navigate:**

```mdl
-- FORWARD (from the reference-owner / "one" side) → a SINGLE object.
-- An Expense has one Employee (Expense_Employee: from Expense to Employee):
retrieve $Employee from $Expense/MyFirstModule.Expense_Employee;
change $Employee (Name = 'Updated');        -- change it directly — do NOT loop

-- REVERSE (from the "many" side) → a LIST of the related objects.
-- One Employee has many Expenses (same association, navigated the other way):
retrieve $Expenses from $Employee/MyFirstModule.Expense_Employee;
loop $Expense in $Expenses
begin
  change $Expense (Amount = 0);
end loop;
```

- **Forward Reference traversal returns a single object** — do **not** `loop` over
  it. Looping a single object passes `mxcli check` but produces an invalid project
  (mxbuild `StorageLoadException` — the loop's `change` writes an unqualified
  attribute). Loop only over the list-valued (reverse / ReferenceSet) form.
- The association is always fully qualified (`Module.Association`), and the start
  variable is an object you already have (a parameter, a prior retrieve/create, or
  a loop iterator). Both forms above are mxbuild-verified (0 errors).

**Enumeration attributes in WHERE**: XPath is a database query, so enum values are stored as plain strings. Both forms are valid — mxcli converts the qualified name to a string literal in BSON:

```mdl
-- Preferred: qualified name (mxcli converts to 'Open' in BSON)
retrieve $Open from Module.Order
  where [Status = Module.OrderStatus.Open];

-- Also accepted: string literal (the value key, case-sensitive)
retrieve $Open from Module.Order
  where [Status = 'Open'];

-- Multiple enum values with OR
retrieve $InProgress from Module.Order
  where [Status = Module.OrderStatus.Open or Status = Module.OrderStatus.Processing];
```

This is different from IF/SET expressions — see "Enumeration Comparisons" section above.
## XPath Navigation

### Attribute Access

```mdl
-- Read attribute
declare $ProductName string = $Product/Name;
declare $Price decimal = $Product/Price;

-- Write attribute (alternative to CHANGE)
set $Product/Price = $NewPrice;
set $Product/ModifiedDate = [%CurrentDateTime%];
```

### Association Navigation

```mdl
-- Navigate to related object
declare $CustomerName string = $Order/Shop.Order_Customer/Name;
declare $CategoryName string = $Product/Shop.Product_Category/Name;

-- Set association
set $Order/Shop.Order_Customer = $Customer;
set $Order/Shop.Order_Product = $Product;
```

**Critical**: Always use fully qualified association names (`Module.AssociationName`).

### XPath in Expressions

```mdl
-- Use in calculations
declare $MonthlyTotal decimal = $Product/MonthlyTotal;
declare $DailyAverage decimal = $MonthlyTotal div 30;

-- Use in conditions
if $Product/IsActive then
  set $count = $count + 1;
end if;

-- Combine with operators
set $TotalPrice = $Product/Price * $Quantity;
```
