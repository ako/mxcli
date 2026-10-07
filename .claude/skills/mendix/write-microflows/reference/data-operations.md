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
> mdl 1;
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

The **Change list** activity has four operations, one statement each:

| Change list operation | MDL statement |
|---|---|
| Add | `add $Item to $Items;` |
| Remove | `remove $Item from $Items;` |
| Clear | `clear $Items;` |
| Replace (stored `Set`) | `set $Items = $Other;` |

`set` on a **list** variable is the Replace operation of a Change list activity,
not a Change variable: Mendix's Change variable takes only primitive variables,
and one on a list fails the build with CE7247 ("Variable '…' does not have a
primitive type"). mxcli knows a variable is a list when it is a list parameter,
a `create list`, a list retrieve or a list operation's result. Both work in
microflows and nanoflows.

`set` on an **object** variable is refused (MDL-SET01 in `check`; `check --references` and `exec` also catch an object from an association retrieve):
Mendix has no action that reassigns an object variable, and a Change variable on
one is the same CE7247. To walk a chain (`$Cursor = $Next` in a `while` loop),
write a sub-microflow that **returns** the next object and recurse; to change the
object itself, use `change $Obj (…)`.

`set` on a **parameter** is refused as well (MDL-SET01), whatever its type unless
it is a list: a Change variable cannot target a parameter (CE7247 "Parameter 'N'
cannot be changed."), in microflows, nanoflows and rules. Copy it into a variable
first — `declare $Value Integer = $N;` — and change that.

### One statement per activity

Every list operation and aggregate is **one Studio Pro activity**, and it is
written as one statement that mirrors it: the keyword is the operation's name
in the activity's dialog, and the inputs are the ones the dialog asks for. The
operand is always a **variable**, because the dialog selects a variable — so
one activity cannot be nested inside another, just as it cannot be drawn.

| Studio Pro activity: operation | MDL statement |
|---|---|
| List operation: Filter | `$Open = filter $Orders by Status = Shop.Status.Open;` |
| List operation: Filter by expression | `$Big = filter $Orders where $currentObject/Total > 1000;` |
| List operation: Find | `$Order = find $Orders by Number = $Number;` |
| List operation: Find by expression | `$Late = find $Orders where $currentObject/DueDate < [%CurrentDateTime%];` |
| List operation: Sort | `$Sorted = sort $Orders by OrderDate desc, Number asc;` |
| List operation: Head / Tail | `$First = head $Orders;` / `$Rest = tail $Orders;` |
| List operation: Range | `$Page = range $Orders offset 20 limit 10;` |
| List operation: Union / Intersect / Subtract | `$All = union $A with $B;` / `$Both = intersect $A with $B;` / `$Left = subtract $B from $A;` |
| List operation: Contains / Equals | `$Has = contains $Order in $Orders;` / `$Same = equals $A and $B;` |
| Aggregate list: Count | `$N = count $Orders;` |
| Aggregate list: Sum / Average / Minimum / Maximum | `$Total = sum $Orders by Amount;` or `$Total = sum $Orders of $currentObject/Amount * 1.21;` |
| Aggregate list: All / Any | `$AllPaid = all $Orders where $currentObject/Paid;` |
| Aggregate list: Reduce | `$Csv = reduce $Orders from '' as String using $currentResult + $currentObject/Name;` |

`by` picks a member (the dialog's attribute or association selector) and `where`
/ `of` take an expression — the "… by expression" variants. `subtract $B from $A`
is A minus B.

Two statements, never one nested call:

```mdl
$Approved = filter $Orders where $currentObject/Status = Shop.Status.Approved;
$Count    = count $Approved;
```

The older **call forms** (`$x = filter($L, …)`, `$n = count($L)`, `sum($L.Attr)`)
still parse and build the same activity, but they are deprecated
(`MDL-DEPR003`, `MDL-DEPR004`) — do not write them. Under the `mdl 1;` header:

- `set` is **mandatory** to change a variable: `set $Total = $Total + 1;`. A
  statement without `set` is only ever an activity (`MDL-V1-SET` under mdl 0).
- `$x = find(…)` and `$x = contains(…)` are refused: the call is also Mendix's
  **string** function. `set $Pos = find($Text, 'a');` is the string function;
  `$Match = find $L where …;` is the list operation (`MDL-V1-LIST`).
- a nested call, or a list operation after `set`, is refused (`MDL-V1-LIST`).
  Without the header a nested operand is refused at check time as `MDL-LISTOP02`.

### `range` — paging a list

`range` takes an `offset` and a `limit` (Studio Pro's *Offset* and *Amount*), and
Mendix requires at least one of them:

```mdl
$Page  = range $Sorted offset $Offset limit $PageSize;  -- skip $Offset, take $PageSize
$First = range $Sorted limit 10;                        -- first 10
$Rest  = range $Sorted offset $Offset;                  -- skip $Offset, take the rest
```

`range $List;` with no bound builds nothing useful and fails with **CE6520**
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

### `filter` / `find` — `by` a member, or `where` an expression over `$currentObject`

`by Member = value` is Studio Pro's *Filter* / *Find*: an attribute or
association of the list's entity and the value it must have. Anything else is
the *by expression* operation, written after `where`, which Mendix evaluates
once per item with the item bound to **`$currentObject`** — the only iterator
name there is:

```mdl
$Pending = filter $Orders by Status = Shop.Status.Pending;
$Large   = filter $Orders where $currentObject/Amount > 1000;
$Match   = find $Orders by OrderNumber = $Wanted;
```

A **bare attribute name** after `where` means the same thing — mxcli resolves it
against the list's entity and writes `$currentObject/Attr`:

```mdl
$Open = filter $Orders where Status != 'Closed';   -- stored as $currentObject/Status
```

`by` accepts only `Member = value`; `filter $L by Amount > 3` is refused with a
pointer to `where`. Two things are refused after `where` rather than passed
through to the build:

- a bare name that is **not** a member of the list's entity (this used to reach
  mxbuild as `CE0117 "Error(s) in expression."`);
- any **other** iterator name — `filter $L where $item/Amount > 0` is
  `MDL-LISTOP01`, pre-empting `CE0109 "Undefined variable 'item'"`.

`$item` is still fine when it is genuinely in scope, which is how the O(N) lookup
idiom is written: inside `loop $item in $L`, `find $Others by Key = $item/Key`
navigates the **loop's** variable on the right-hand side.

**`sort` is not an expression.** It takes attribute names directly —
`sort $Orders by CreateDate desc` — and any word works as a name there, so an
attribute called `Count` or `Date` needs no quotes. Writing `$currentObject/`
there is wrong.

### `contains` — string function vs list operation

The list operation is `contains $Object in $List`, and it creates its own
Boolean output variable, so do not declare it first. The **string** function
`contains(haystack, needle)` is an expression, assigned with `set` to a variable
declared first:

```mdl
-- LIST contains — the statement creates $Found
$Found = contains $Item in $Items;

-- STRING contains — assign to a PRE-DECLARED Boolean (a Change Variable action)
declare $HasAt Boolean = false;
set $HasAt = contains($Email, '@');
```

Under `mdl 1;` that is the whole rule. Without the header, the call form
`$Found = contains($Items, $Item)` is still read as the list operation when both
arguments are plain variables and the first is not a declared String — the
guess that `mdl 1` removes. Getting the declare wrong is what triggers `CE0111
"Duplicate variable name"`.

### Aggregates — all eight, including `reduce`, `all` and `any`

An Aggregate list activity folds a list into one value. `count` takes only the
list; `sum`, `average`, `minimum` and `maximum` aggregate an attribute (`by`) or
an expression over `$currentObject` (`of`) — the dialog's *Aggregate with*.

```mdl
$Count   = count $Orders;
$Total   = sum $Orders by Amount;                           -- attribute
$Total   = sum $Orders of $currentObject/Amount * 1.21;     -- expression
$Avg     = average $Orders by Amount;
$Min     = minimum $Orders by Amount;
$Max     = maximum $Orders by Amount;

-- Boolean predicates over every item. No seed, always Boolean.
$AllPaid = all $Orders where $currentObject/Paid;
$AnyLate = any $Orders where $currentObject/DueDate < [%CurrentDateTime%];

-- REDUCE folds with a running total. $currentResult is the accumulator.
$Discounted = reduce $Orders from 0 as Decimal
  using $currentResult + $currentObject/Amount * 0.9;
```

**`reduce` needs the initial value (`from`) and the return type (`as`), and
neither can be inferred.** Mendix stores both beside the expression, and the
fold is meaningless without a seed and a result type — so MDL makes them
mandatory rather than guessing. `all` and `any` take neither: they never
accumulate, and always fold to Boolean.

Do not reach for `reduce` where `sum` will do. It exists for folds Mendix has no
dedicated function for — running a string together, or carrying a value forward
that depends on the previous item.

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
- RETRIEVE with `first` returns a **single entity**
- RETRIEVE without a range, or with `limit n [offset n]`, returns a **list** (`list of Module.Entity`)
- Use `first` when you expect exactly one result (e.g., lookup by unique key). `limit 1`
  is a list of one under `mdl 1;` and the object without the header (warning `MDL-V1-LIMIT1`)

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
- A bare attribute name is qualified with the entity that **declares** it, which may be an ancestor — `sort by Name` on a specialization of `System.User` stores `System.User.Name`, which is what mxbuild resolves.

**Sorting over an association** — one `/` per hop, the last segment is the attribute:

```mdl
retrieve $Orders from Sales.Order
  sort by Sales.Order_BillTo/Sales.Address.City asc;

-- the hop may be unqualified, and the attribute bare
retrieve $Orders from Sales.Order
  sort by Order_BillTo/City asc;
```

- **Name the hop when more than one association reaches the same entity.** Writing the
  attribute alone (`sort by Sales.Address.City`) makes mxcli infer the association: it
  walks the generalization chain and crosses modules — `Administration.Account` reaches
  `System.Language.Code` through `System.User_Language`, declared on `System.User` — but it
  cannot tell `Order_ShipTo` from `Order_BillTo` and takes the nearest one. Measured on
  11.12.3: a microflow sorting by the billing address came back from `describe → exec`
  sorting by the shipping one, at 0 errors on both sides (mendixlabs/mxcli#1152).
- `describe microflow` emits the hop whenever one is stored, so a described sort replays
  to the same model. An association that does not exist, or does not start at the entity
  in hand, is **refused** rather than written — Mendix stores a sort over an association
  as an `EntityRef` beside the attribute, and an attribute of a far entity without one is
  **CE7247** "Cannot sort on attribute …".

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
