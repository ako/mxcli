# Control Flow

MDL microflows support conditional branching, loops, and error handling to control the execution path of your logic.

## IF / ELSE

Conditional branching executes different activities based on a boolean expression.

### Basic IF

```sql
IF $Order/TotalAmount > 1000 THEN
  CHANGE $Order (DiscountApplied = true);
END IF;
```

### IF / ELSE

```sql
IF $Customer/Email != empty THEN
  CALL MICROFLOW Sales.SUB_SendEmail (Customer = $Customer);
ELSE
  LOG WARNING 'Customer has no email address';
END IF;
```

### Nested IF

For multi-way branching on anything other than an enumeration, use nested `IF...ELSE`
blocks. (To branch on an **enumeration**, use [CASE (Enum Split)](#case-enum-split)
instead — it maps to a Mendix enum split rather than a chain of decisions.)

```sql
IF $Order/TotalAmount > 10000 THEN
  CHANGE $Order (DiscountPercentage = 15);
ELSE
  IF $Order/TotalAmount > 5000 THEN
    CHANGE $Order (DiscountPercentage = 10);
  ELSE
    IF $Order/TotalAmount > 1000 THEN
      CHANGE $Order (DiscountPercentage = 5);
    ELSE
      CHANGE $Order (DiscountPercentage = 0);
    END IF;
  END IF;
END IF;
```

### Comparing Enumerations

An enumeration is compared against its **qualified value**, never a string literal.
A string literal here fails the build with **CE0117** *"Error(s) in expression"* —
`mxcli check` does not catch it (see
[expression type checking](https://github.com/mendixlabs/mxcli/blob/main/docs/11-proposals/PROPOSAL_expression_type_checking.md)),
so the first sign is a failed build.

```sql
-- CORRECT
IF $Order/Status = Sales.OrderStatus.Draft THEN ... END IF;

-- WRONG: CE0117 "Error(s) in expression."
IF $Order/Status = 'Draft' THEN ... END IF;
```

The same applies to putting an enumeration into a string — concatenating it directly
is CE0117, so render it first:

```sql
-- CORRECT
LOG WARNING 'Unexpected status: ' + getCaption($Order/Status);

-- WRONG: CE0117
LOG WARNING 'Unexpected status: ' + $Order/Status;
```

The string form **is** accepted outside comparisons — in a `CREATE`/`CHANGE` member
value, an attribute `DEFAULT`, and an XPath constraint (where enums live as strings at
the database level). The qualified form works everywhere, so prefer it.

### Complex Conditions

Conditions support `AND`, `OR`, and parentheses:

```sql
IF $Amount > 0 AND $Customer != empty THEN
  COMMIT $Order;
END IF;

IF ($Status = 'Active' OR $Status = 'Pending') AND $IsValid = true THEN
  CALL MICROFLOW Sales.ProcessOrder (Order = $Order);
END IF;
```

## LOOP (FOR EACH)

Iterates over each item in a list:

```sql
LOOP $Line IN $OrderLines
BEGIN
  CHANGE $Line (
    LineTotal = $Line/Quantity * $Line/UnitPrice
  );
  COMMIT $Line;
END LOOP;
```

The loop variable (`$Line`) is automatically declared and takes the entity type of the list.

### LOOP with Nested Logic

```sql
LOOP $Order IN $PendingOrders
BEGIN
  IF $Order/TotalAmount > 0 THEN
    CHANGE $Order (Status = 'Confirmed');
    COMMIT $Order;
  ELSE
    DELETE $Order;
  END IF;
END LOOP;
```

### BREAK and CONTINUE

Use `BREAK` to exit a loop early, and `CONTINUE` to skip to the next iteration:

```sql
LOOP $Item IN $Items
BEGIN
  IF $Item/IsInvalid = true THEN
    CONTINUE;
  END IF;

  IF $Item/Type = 'StopSignal' THEN
    BREAK;
  END IF;

  CALL MICROFLOW Sales.ProcessItem (Item = $Item);
END LOOP;
```

## WHILE Loop

Executes a block repeatedly as long as a condition remains true:

```sql
DECLARE $Counter Integer = 0;

WHILE $Counter < 10
BEGIN
  SET $Counter = $Counter + 1;
  LOG INFO 'Iteration: ' + toString($Counter);
END WHILE;
```

> **Caution:** Ensure the condition will eventually become false to avoid infinite loops.

## Error Handling

### ON ERROR Suffix

Error handling is applied as a suffix to individual activities. There is no `TRY...CATCH` block in MDL. Instead, you specify error handling behavior on specific activities.

#### ON ERROR CONTINUE

Ignores the error and continues to the next activity:

```sql
COMMIT $Order ON ERROR CONTINUE;
```

#### ON ERROR ROLLBACK

Rolls back the current transaction and continues:

```sql
DELETE $Order ON ERROR ROLLBACK;
```

#### ON ERROR with Handler Block

Executes a custom error handling block when the activity fails:

```sql
COMMIT $Order ON ERROR {
  LOG ERROR 'Failed to commit order: ' + $Order/OrderNumber;
  ROLLBACK $Order;
};
```

The handler block can contain any activities -- logging, rollback, showing validation messages, etc.

### Error Handling Examples

```sql
-- Continue despite retrieval failure
RETRIEVE $Config FROM Admin.SystemConfig LIMIT 1 ON ERROR CONTINUE;

-- Custom error handler for external call
$Response = CALL MICROFLOW Integration.CallExternalAPI (
  Payload = $RequestBody
) ON ERROR {
  LOG ERROR NODE 'Integration' 'External API call failed';
  SET $Response = empty;
};

-- Rollback on commit failure
COMMIT $Order ON ERROR ROLLBACK;
```

> **Note:** `ON ERROR` is not supported on `EXECUTE DATABASE QUERY` activities.

## CASE (Enum Split)

`CASE` branches on an **enumeration** and compiles to a Mendix enum split. It is not a
general-purpose switch: the source is an enum attribute or variable, and the values are
bare enum member names.

```sql
CASE $Order/Status
  WHEN Draft, Submitted THEN
    LOG INFO 'Not shipped yet';
  WHEN Approved THEN
    LOG INFO 'Ready to ship';
  WHEN Shipped, (empty) THEN
    LOG INFO 'Nothing to do';
END CASE;
```

Rules, each of which `mxcli check` enforces:

| Rule | Why |
|------|-----|
| Values are **bare identifiers** — not `'Quoted'`, not `Module.Enum.Value` | Parse error otherwise |
| One branch per enum value, **including `(empty)`** | A missing `(empty)` is **MDL056**; mxbuild reports **CE0079** for any uncovered value. Required even when the attribute is `not null` |
| **No `ELSE`** | **MDL008**. An enum split is exclusive with one outgoing flow per value; mxbuild reports CE0079 per uncovered value *and* CE0773 on the else flow |
| **No `AS` alias** | Parse error: `mismatched input 'as' expecting WHEN` |

Several values may share a branch (`WHEN Draft, Submitted THEN …`). `CASE` works in
nanoflows on the same terms.

## SPLIT TYPE (Object Type Split)

`SPLIT TYPE` branches on an object's **runtime specialization** and compiles to a Mendix
object type decision. Branches use the same `WHEN … THEN` shape as [CASE](#case-enum-split),
so the two splits differ only in what they branch on.

```sql
SPLIT TYPE $Animal
  WHEN Zoo.Dog THEN
    CAST $Dog;
    LOG INFO 'woof';
  WHEN Zoo.Cat THEN
    LOG INFO 'meow';
  WHEN Zoo.Animal THEN
    LOG INFO 'some other animal';
  WHEN (empty) THEN
    LOG INFO 'no animal at all';
END SPLIT;
```

Use `CAST` inside a branch to bind the specialized variable its body needs.

| Rule | Why |
|------|-----|
| A branch for **every** subtype **and** the base entity | mxbuild reports **CE0090** *"The 'X' value should be configured for an outgoing flow"* for each type with no branch. The base entity is what covers "none of the more specific ones" |
| `WHEN (empty) THEN` is the **null-object** branch, not a default | It is taken when the variable is empty. It does **not** cover unnamed types — measured on 11.13.0, one named type plus an empty branch still gives CE0090 for every other type. Branch on the base entity for that |
| The empty branch cannot be omitted | **CE0089**. mxcli emits the flow unconditionally, so MDL cannot express a split without one |
| A non-void microflow needs a `RETURN` after `END SPLIT;` | Branches converge on a merge that continues to the end event — otherwise **MDL003** and **CE0067** |

The pre-#913 spelling — `CASE Zoo.Dog` for a branch and `ELSE` for the empty one — still
parses and builds the identical flow, but warns **MDL065**. It was replaced because `CASE`
introduced a *branch* here while introducing the *subject* in `CASE $x WHEN v THEN`, and
because `ELSE` reads as a default branch and never was one.

`SPLIT TYPE` works in nanoflows on the same terms.

## Unsupported Control Flow

The following constructs are **not** supported in MDL and will cause parse errors:

| Unsupported | Use Instead |
|-------------|-------------|
| `CASE ... WHEN 'String' ... ELSE ...` | Bare enum values and a branch per value — see [CASE (Enum Split)](#case-enum-split); `CASE` itself is supported |
| `TRY ... CATCH ... END TRY` | `ON ERROR { ... }` blocks on individual activities |

## Complete Example

```sql
CREATE MICROFLOW Sales.ACT_ProcessBatch
FOLDER 'Batch'
BEGIN
  DECLARE $Orders List of Sales.Order = empty;
  DECLARE $SuccessCount Integer = 0;
  DECLARE $ErrorCount Integer = 0;

  RETRIEVE $Orders FROM Sales.Order
    WHERE Status = 'Pending';

  LOOP $Order IN $Orders
  BEGIN
    IF $Order/TotalAmount <= 0 THEN
      LOG WARNING 'Skipping order with zero amount: ' + $Order/OrderNumber;
      CONTINUE;
    END IF;

    @caption 'Process order'
    CALL MICROFLOW Sales.SUB_ProcessSingleOrder (
      Order = $Order
    ) ON ERROR {
      LOG ERROR 'Failed to process order: ' + $Order/OrderNumber;
      SET $ErrorCount = $ErrorCount + 1;
      CONTINUE;
    };

    SET $SuccessCount = $SuccessCount + 1;
  END LOOP;

  LOG INFO 'Batch complete: ' + toString($SuccessCount) + ' processed, '
    + toString($ErrorCount) + ' errors';
END;
```
