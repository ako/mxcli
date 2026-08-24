# Control flow, error handling and annotations

Supporting reference for [write-microflows](../SKILL.md).

## Control Flow

### IF Statements

```mdl
-- Simple IF
if $value > 10 then
  set $message = 'Greater than 10';
end if;

-- IF/ELSE
if $value > 100 then
  set $Category = 'High';
else
  set $Category = 'Low';
end if;

-- Nested IF
if $Score >= 90 then
  set $Grade = 'A';
else
  if $Score >= 80 then
    set $Grade = 'B';
  else
    set $Grade = 'C';
  end if;
end if;
```

**Important**: Always close with `end if` (not just `end`).

### Enumeration Comparisons

**CRITICAL**: When comparing enumeration values, use the fully qualified enumeration value, NOT a string literal.

```mdl
-- CORRECT: Use fully qualified enumeration value
if $task/status = Module.TaskStatus.Completed then
  set $IsComplete = true;
end if;

if $Order/OrderStatus != Module.OrderStatus.Cancelled then
  -- Process the order
end if;

-- WRONG: Do NOT use string literals
-- IF $Task/Status = 'Completed' THEN  -- INCORRECT!
```

Putting an enumeration **into** a string is the same mistake — concatenating it
directly is rejected, so render it first with `getCaption()` (the caption) or
`toString()` (the value name):

```mdl
-- CORRECT
log warning 'Unexpected status: ' + getCaption($Order/Status);

-- WRONG
log warning 'Unexpected status: ' + $Order/Status;
```

**Where the string form is and is not accepted** (verified against mxbuild 11.13.0 —
one microflow per row, `mx check` read per construct):

| Context | `'Draft'` | Note |
|---------|-----------|------|
| Comparison in a decision — `if $O/Status = 'Draft'` | ❌ **CE0117** | The one that bites |
| Concatenation — `'x' + $O/Status` | ❌ **CE0117** | Use `getCaption()` / `toString()` |
| `change $O (Status = 'Draft')` | ✅ accepted | Slot is already enum-typed |
| `create M.E (Status = 'Draft')` | ✅ accepted | Same |
| Attribute `DEFAULT 'Draft'` | ✅ accepted | Documented as the legacy form |
| XPath constraint `[Status = 'Draft']` | ✅ accepted | Enums are strings at DB level |

`mxcli check` does **not** flag the two failing rows (it does not type expressions —
see `docs/11-proposals/PROPOSAL_expression_type_checking.md`), so a script can pass
`check` and fail the build. The qualified form is valid in every row above: use it
everywhere and the distinction never has to be remembered.

**Checking for empty enumeration:**
```mdl
if $entity/status = empty then
  -- Enumeration is not set
end if;
```

### CASE Statements (Enum Split)

Use `case` when a microflow branches on an enumeration value.

```mdl
case $Status
  when Open, Pending then
    return true;
  when Closed then
    return false;
  when (empty) then
    return false;
end case;
```

`(empty)` represents an unset enumeration value. Multiple values can share one `when` branch by separating them with commas. Case values are bare identifiers — do **not** quote them.

> **Every value needs a branch, including `(empty)` — and there is no `else`.**
> A Mendix enum split is an exclusive split with one outgoing flow per condition
> value, so an uncovered value fails the build with **CE0079** *"The 'X' condition
> value should be configured in properties for an outgoing flow."* `mxcli check`
> reports a missing `(empty)` branch as **MDL056**, and an `else` branch as
> **MDL008** (an `else` does not stand in for the missing flows: mxbuild reports
> CE0079 for each uncovered value *and* CE0773 on the else flow itself).
>
> The `(empty)` branch is required **even when the attribute is `not null`** —
> verified on Mendix 11.6.6. If several values share a path, put them in one
> branch (`when Open, Pending then`) rather than reaching for `else`.

### Type Split And Cast Statements

Use `split type` when a microflow branches on an object's runtime specialization.
Use `cast` inside a type branch to create the specialized variable used by the branch body.

Branches are `when <Entity> then`, the same as an enumeration split — one
statement, two subjects.

```mdl
declare $IsSpecialized boolean = false;
split type $Input
  when Sample.SpecializedInput then
    cast $SpecificInput;
    set $IsSpecialized = true;
  when Sample.BaseInput then
  when (empty) then
end split;
return $IsSpecialized;
```

Branch values are qualified entity names.

> **`when (empty) then` is the null-object branch, not a default.** It is
> Mendix's `(empty)` outgoing flow, taken when the split variable is empty. It
> does **not** cover types you did not name — see the CE0090 note below.
>
> **The older spelling still works.** `case Sample.SpecializedInput` (no `then`)
> and `else` for the empty branch parse and build the identical flow, but warn
> **MDL065**. `case` introduced a *branch* here while introducing the *subject*
> in `case $x when V then` and in expressions, so the word meant two things
> (mxcli #913); `else` read as a default and never was one.

> **Every type needs a branch — including the base entity.** An object-type
> decision gets one outgoing flow per listed type, and a type with no flow fails
> the build with **CE0090** *"The 'X' value should be configured for an outgoing
> flow."* The base entity (the split variable's own type) counts: `case
> Sample.BaseInput` above is what covers "it is not any of the specializations".
>
> **The `(empty)` branch does not stand in for the base-type case.** It is
> accepted — it serializes as `Microflows$NoCase` — but it does not satisfy
> coverage, so one named type plus `when (empty) then` still fails CE0090.
> Measured on 11.13.0: `when Zoo.Dog then` + an empty branch gives CE0090 for
> `Zoo.Cat` **and** `Zoo.Animal`; branch on every type, keep the empty branch,
> and it is 0 errors. Verified on Mendix 11.6.6 and 11.13.0.
>
> You cannot drop the empty branch either — that is **CE0089**. mxcli emits the
> flow unconditionally for that reason.
>
> **The split needs somewhere to go afterwards.** Branch bodies converge on a
> merge that continues to the microflow's end event, so a non-void microflow
> needs a `return` after `end split;` — otherwise `mxcli check` reports MDL003
> and the build fails **CE0067** *"The 'Return value' property is required."*
> Doing the per-branch work into a variable and returning it once (above) is the
> clearest shape; returning inside every branch also works, but still needs the
> trailing `return`.

**`cast` only stores the output variable.** Studio Pro persists Microflows$CastAction with a single `VariableName` field — the source variable is implicit (the type-split's input). Use `cast $SpecificName;` to give the specialized variable its name. The two-variable form `$Output = cast $Source;` parses but `$Source` is dropped on roundtrip; prefer the single-variable form.

### LOOP Statements

```mdl
-- Basic loop
loop $Product in $ProductList
begin
  set $count = $count + 1;
end loop;

-- Loop with object modification
loop $Product in $ProductList
begin
  change $Product (IsActive = true);
  commit $Product;
end loop;

-- Loop with conditional logic
loop $Product in $ProductList
begin
  if $Product/IsActive then
    set $ActiveCount = $ActiveCount + 1;
  end if;
end loop;

-- Label a loop with a note (loops have no caption in Mendix)
@annotation 'Process each product'
loop $Product in $ProductList
begin
  set $count = $count + 1;
end loop;
```

> **`@caption` does nothing on a loop.** Mendix for-loops have no caption
> property, so `@caption` on a `loop` is silently dropped (`mxcli check` flags
> it as **MDL042**). To label a loop, use `@annotation 'text'` — it attaches a
> note, exactly like drawing one onto the loop in Studio Pro.

**Note**:
- Loop variable (`$Product`) is scoped to the loop body
- The loop variable type is **automatically derived** from the list type (e.g., `list of Test.Product` → `Test.Product`)
- CHANGE statements inside loops use the derived type to resolve attribute names

> **Nothing a loop defines survives past `end loop;`.** The iterator *and*
> anything the body creates (a `retrieve`, a `$X = create …`, a call output) are
> visible only inside the body; using one afterwards is
> `CE0108 "Variable 'X' is defined but not in scope at this location."`
> (`mxcli check` flags it as **MDL053**).
>
> ```mdl
> -- WRONG: $Last is created inside the loop, read outside it
> loop $Item in $Items
> begin
>   $Last = create Test.Product (Name = $Item/Name);
> end loop;
> commit $Last;                        -- MDL053 / CE0108
>
> -- RIGHT: declare before the loop, assign inside, read after
> declare $LastName string = '';
> loop $Item in $Items
> begin
>   set $LastName = $Item/Name;
> end loop;
> log info node 'Test' $LastName;
> ```
>
> Visibility and *naming* are separate rules: names must also be unique across
> the **whole** microflow, so two loops cannot share an iterator name either
> (`CE0111`, flagged as **MDL052**).

### Performance: Batch Commit After Loop

**CRITICAL**: Do NOT commit inside a loop. Each `commit` inside a loop issues a separate database transaction, which causes N round-trips for N records and degrades performance significantly.

❌ **INCORRECT — commit inside loop (N transactions):**
```mdl
loop $Binding in $BindingsList
begin
  $NewBatch = create BatteryOntology.MaterialBatch (BatchNo = $BatchNoObj/Value);
  commit $NewBatch;  -- ❌ one DB transaction per record
end loop;
```

✅ **CORRECT — create list before loop, commit once after:**
```mdl
$BatchList = create list of BatteryOntology.MaterialBatch;
loop $Binding in $BindingsList
begin
  $NewBatch = create BatteryOntology.MaterialBatch (BatchNo = $BatchNoObj/Value);
  add $NewBatch to $BatchList;   -- accumulate in memory
end loop;
commit $BatchList on error rollback;  -- ✅ single transaction
```

**Pattern:**
1. Before the loop: `$XxxList = create list of Module.Entity;`
2. Inside the loop: `add $NewXxx to $XxxList;` (replaces `commit`)
3. After the loop: `commit $XxxList on error rollback;`

This applies whenever the loop **creates** new objects. For loops that only **change** existing objects, the same pattern applies — accumulate changed objects in a list, commit the list once outside the loop.
## Activity Annotations

Annotations use `@` prefix syntax placed before the activity they apply to:

```mdl
-- Canvas position (always shown in DESCRIBE output)
@position(200, 200)
commit $Order;

-- Custom caption (overrides auto-generated caption)
@caption 'Save the order'
commit $Order;

-- Background color (Blue, Green, Red, Yellow, Purple, Gray)
@color Green
log info node 'App' 'Success';

-- Visual note attached to the next activity (creates AnnotationFlow)
@annotation 'Validate the order before processing'
commit $Order;

-- Multiple annotations stacked on a single activity
@position(400, 200)
@caption 'Persist product'
@color Blue
@annotation 'Step 2: Save to database'
commit $Product;
```

**Rules:**
- `@annotation` before an activity attaches the note to that activity
- `@annotation` before activity-binding metadata such as `@position`, `@caption`, `@color`, `@excluded`, or `@anchor` stays free-floating when later metadata binds the following activity
- `@annotation` at the end (no following activity) creates a free-floating note
- Escape single quotes by doubling: `@annotation 'Don''t forget'`
- `@position` always appears in DESCRIBE output; `@caption` only when custom; `@color` only when not Default
- DESCRIBE MICROFLOW shows `@` annotations before their activities
- `@start(x, y)` positions the **start event** and goes on the first statement, because the start has no statement of its own. Omit it and the start is derived — one spacing unit (160) left of the first activity, on its centre line — and a rewrite re-derives it so the start follows the activities when they move. A start that is not at the derived spot was placed by hand (in Studio Pro or with `@start`): it survives a rewrite that does not mention it, and DESCRIBE emits `@start` for it. An explicit `@start` overrides both (#951)
## Error Handling

MDL supports error handling for activities that may fail (microflow calls, commits, external service calls, etc.).

### Error Handling Types

```mdl
-- ON ERROR CONTINUE: Ignore error and continue execution
call microflow Module.RiskyOperation() on error continue;

-- ON ERROR ROLLBACK: Rollback transaction and propagate error
commit $Order on error rollback;

-- ON ERROR { ... }: Custom error handler with rollback
$Result = call microflow Module.ExternalService(data = $data) on error {
  log error node 'ServiceError' 'External service failed';
  return $DefaultResult;
};

-- ON ERROR WITHOUT ROLLBACK { ... }: Custom handler, keep changes
commit $Order on error without rollback {
  log warning node 'CommitError' 'Commit failed, using fallback';
  change $Order (status = 'PENDING');
};
```

### Error Handling Semantics

| Syntax | Behavior |
|--------|----------|
| `on error continue` | Catch error silently, continue normal flow |
| `on error rollback` | Rollback database changes, propagate error |
| `on error { ... }` | Execute handler block, then continue (with rollback) |
| `on error without rollback { ... }` | Execute handler block, keep database changes |

### When to Use Each Type

- **CONTINUE**: Non-critical operations where failure is acceptable
- **ROLLBACK**: Critical operations where data integrity must be preserved
- **Custom handlers**: When you need to log errors, set fallback values, or notify users

### Example: Robust External Call

```mdl
/**
 * Calls external service with error handling
 */
create microflow Module.SafeExternalCall (
  $RequestData: string
)
returns Module.Response as $response
begin
  -- The call output establishes $response — objects are never declared
  $response = call microflow Module.CallExternalAPI(data = $RequestData)
    on error without rollback {
      log error node 'ExternalAPI' 'API call failed for: ' + $RequestData;
      -- Create error response
      $response = create Module.Response (
        success = false,
        message = 'External service unavailable');
    };

  return $response;
end;
/
```
