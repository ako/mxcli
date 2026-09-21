# Structure

This page covers the structural elements of a microflow definition: the `CREATE MICROFLOW` syntax, parameters, variable declarations, return values, and annotations.

## Full CREATE MICROFLOW Syntax

```sql
CREATE [OR REPLACE] MICROFLOW <Module.Name>
  [FOLDER '<path>']
BEGIN
  [<declarations>]
  [<activities>]
  [RETURN <value>;]
END;
```

The `OR REPLACE` modifier overwrites an existing microflow of the same name. The `FOLDER` clause organizes the microflow within the module's folder structure.

## Parameters

Parameters are declared with `DECLARE` at the top of the `BEGIN...END` block. They define the inputs to the microflow.

### Primitive Parameters

```sql
DECLARE $Name String;
DECLARE $Count Integer;
DECLARE $IsActive Boolean;
DECLARE $Amount Decimal;
DECLARE $StartDate DateTime;
```

Supported primitive types: `String`, `Integer`, `Long`, `Boolean`, `Decimal`, `DateTime`.

### Entity Parameters

Entity parameters receive a single object:

```sql
DECLARE $Customer MyModule.Customer;
```

> **Important:** Do not use `= empty` or `AS` with entity declarations. The correct syntax is simply `DECLARE $Var Module.Entity;`.

### List Parameters

List parameters receive a list of objects:

```sql
DECLARE $Orders List of Sales.Order = empty;
```

The `= empty` initializer creates an empty list. This is required for list declarations.

### Parameter vs. Local Variable

In MDL, all `DECLARE` statements at the top of a microflow are treated as parameters. Variables created by activities (such as `$Var = CREATE ...` or `RETRIEVE $Var ...`) are local variables. If you need a local variable with a default value, use `DECLARE` followed by `SET`:

```sql
DECLARE $Counter Integer = 0;
SET $Counter = 10;
```

## Variables and Assignment

### Variable Declaration

```sql
DECLARE $Message String = 'Hello';
DECLARE $Total Decimal = 0;
DECLARE $Found Boolean = false;
```

### Assignment with SET

Change the value of an already-declared variable:

```sql
SET $Counter = $Counter + 1;
SET $FullName = $FirstName + ' ' + $LastName;
SET $IsValid = $Amount > 0;
```

The variable must be declared before it can be assigned.

### Variables from Activities

Activities like CREATE and RETRIEVE produce result variables:

```sql
$Order = CREATE Sales.Order (Status = 'New');
RETRIEVE $Customer FROM Sales.Customer WHERE Email = $Email LIMIT 1;
$Result = CALL MICROFLOW Sales.CalculateTotal (Order = $Order);
```

## Return Values

Every flow path should end with a `RETURN` statement. The return type is inferred from the returned value.

### Returning a Primitive

```sql
RETURN true;
RETURN $Total;
RETURN 'Success';
```

### Returning an Object

```sql
RETURN $Order;
```

### Returning a List

```sql
RETURN $FilteredOrders;
```

### Returning Nothing

If the microflow returns nothing (void), you can omit the `RETURN` or use:

```sql
RETURN;
```

## Annotations

Annotations are metadata decorators placed **before** an activity. They control visual layout and documentation in Mendix Studio Pro.

### Position

Set the canvas position of the next activity:

```sql
@position(200, 100)
$Order = CREATE Sales.Order (Status = 'New');
```

### Start event

The start event has no statement of its own, so `@start` goes on the **first**
statement — the one the start flows into:

```sql
@start(145, 200)
@position(260, 200)
$Order = CREATE Sales.Order (Status = 'New');
```

It is optional. Omitted, the start is placed one spacing unit left of the first
activity on that activity's centre line, and a rewrite re-derives it so the start
follows the activities when they move. A start that is *not* at that derived spot
was placed on purpose — in Studio Pro or with `@start` — so it survives a rewrite
that does not mention it, and `DESCRIBE` emits an `@start` line for it. An
explicit `@start` overrides both.

### Caption

Set a custom caption displayed on the activity in the canvas:

```sql
@caption 'Create new order'
$Order = CREATE Sales.Order (Status = 'New');
```

### Color

Set the background color of the activity:

```sql
@color Green
COMMIT $Order;
```

### Disabled

Mark a single activity as disabled — Studio Pro's right-click **Disable**. The step
stays in the flow, is drawn greyed out, and is skipped at runtime:

```sql
@disabled
CALL JAVASCRIPT ACTION NanoflowCommons.RefreshEntity (
    EntityToRefresh = $Item
);
```

This is how a step that is not yet right is generated **off** rather than live: the
developer fixes it in Studio Pro and re-enables it, instead of deleting and
recreating the activity.

Mendix stores the flag on `Microflows$ActionActivity` and on no other microflow
object, so `@disabled` on an `IF`, `CASE`, `SPLIT TYPE`, `LOOP`, `WHILE`, `MERGE`,
`JOIN`, `RETURN`, `RAISE ERROR`, `BREAK` or `CONTINUE` is refused (**MDL087**)
rather than quietly ignored — an ignored request would ship a live step the author
believed was off.

`@excluded` before a **statement** is the older spelling of the same flag and still
works. Before a `CREATE MICROFLOW` the same word means *Exclude from project*, which
is a different setting.

### Disabling activities in a flow that already exists

`@disabled` states the flag while *authoring* a flow. To turn a step off in a
microflow that already exists — usually several at once — use `ALTER`:

```sql
-- the everyday one: turn off debug logging across a module before a release
ALTER MICROFLOWS IN Sales DISABLE ACTIVITIES
  WHERE action = log AND level IN (debug, trace);

-- and back on
ALTER MICROFLOWS IN Sales ENABLE ACTIVITIES
  WHERE action = log AND level IN (debug, trace);

-- one step in one flow
ALTER NANOFLOW Sales.ACT_Save DISABLE ACTIVITIES
  WHERE action = 'call javascript action';

-- everything anyone turned off, whatever it was
ALTER MICROFLOWS ENABLE ACTIVITIES WHERE disabled = true;
```

`MICROFLOW` / `NANOFLOW` / `RULE` take one document; the plurals take many, and
**omitting `IN <module>` means the whole project** — the same rule
`ALTER PAGES … SET LAYOUT` follows.

This is deliberately not `CREATE OR MODIFY MICROFLOW` with one line changed. A
rewrite rebuilds the document from MDL and is only as faithful as what MDL can
spell; the flows worth reaching into are the ones holding constructs it cannot.
`ALTER` sets one boolean on the **stored** document and leaves the rest alone.

Filter columns, joined by `AND` (there is no `OR` — `IN (a, b)` covers it):

| Column | Values |
|--------|--------|
| `action` | The MDL keyword (`log`, `commit`, `'call javascript action'`), quoted when it has a space; or the storage name `CATALOG.ACTIVITIES.ActionType` prints |
| `level` | `critical`, `error`, `warning`, `info`, `debug`, `trace` — a **log** activity's level, and a property of nothing else |
| `caption` | The caption DESCRIBE shows; also `caption LIKE '%TODO%'` |
| `disabled` | `true` / `false`, the activity's current state |

A filter that cannot select anything — an unknown action, `level` beside a
non-log `action`, `LIKE` on an enumeration — is **MDL088**, refused by
`mxcli check` and by `exec` rather than run to a zero count that looks like
success. What remains is told apart in the report: "No activity matched" is not
the same message as "all 2 matching activities already disabled".

### Annotation (Visual Note)

Attach a visual annotation note to the next activity:

```sql
@annotation 'This step validates the input before saving'
VALIDATION FEEDBACK $Customer/Email MESSAGE 'Email is required';
```

### Combining Annotations

Multiple annotations can be stacked before a single activity:

```sql
@position(300, 200)
@caption 'Validate and save'
@color Green
@annotation 'Final step: commit the validated order'
COMMIT $Order;
```

## Complete Example

```sql
CREATE MICROFLOW Sales.ACT_ProcessOrder
FOLDER 'Orders/Processing'
BEGIN
  -- Parameters
  DECLARE $Order Sales.Order;
  DECLARE $ApplyDiscount Boolean;

  -- Local variable
  DECLARE $Total Decimal = 0;

  -- Retrieve order lines
  RETRIEVE $Lines FROM $Order/Sales.OrderLine_Order;

  -- Calculate total
  @caption 'Calculate total'
  $Total = CALL MICROFLOW Sales.SUB_CalculateTotal (
    OrderLines = $Lines
  );

  -- Apply discount if requested
  IF $ApplyDiscount THEN
    SET $Total = $Total * 0.9;
  END IF;

  -- Update order
  CHANGE $Order (
    TotalAmount = $Total,
    Status = 'Processed'
  );
  COMMIT $Order;

  RETURN $Order;
END;
```
