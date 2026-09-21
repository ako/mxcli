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
## Disabling activities in a flow that already exists

`@disabled` states the flag while **authoring** a flow. The everyday case is the
other one — turning a step off in a microflow that is already there, and usually
several at once:

```mdl
-- turn off the debug logging across a module before a release
alter microflows in Sales disable activities
  where action = log and level in (debug, trace);

-- and back on
alter microflows in Sales enable activities
  where action = log and level in (debug, trace);

-- one step in one flow
alter nanoflow Sales.ACT_Save disable activities
  where action = 'call javascript action';

-- everything anyone turned off, whatever it was
alter microflows enable activities where disabled = true;
```

`microflow`, `nanoflow` and `rule` take the singular form; `microflows`,
`nanoflows` and `rules` take the bulk form, where **omitting `IN <module>` means
the whole project** — the same rule `alter pages … set layout` follows.

**This is not `create or modify microflow` with one line changed, and the
difference is the point.** A rewrite rebuilds the whole document from MDL, so it
is only as faithful as what MDL can spell — and the flows worth reaching into are
exactly the ones holding constructs it cannot. This statement sets one boolean on
the **stored** document and leaves every other byte alone, so nothing has to be
reproducible for it to be safe. It goes through the same write choke point as
everything else, so a re-run that changes nothing writes nothing.

### The filter

Conditions are joined by `and`. There is no `or`: `in (a, b)` covers what an `or`
would be written for, and a boolean expression tree would need precedence rules
no other MDL clause has.

| Column | Values | Notes |
|--------|--------|-------|
| `action` | `log`, `commit`, `retrieve`, `'call javascript action'`, … | The MDL keyword for the activity; **quoted** when it contains a space. The Mendix storage name works too — the value `select ActionType from CATALOG.ACTIVITIES` prints — so an activity you cannot name can be found by querying for it |
| `level` | `critical`, `error`, `warning`, `info`, `debug`, `trace` | A **log** activity's level, and a property of nothing else. Pairing it with a different `action` matches nothing, and is refused rather than run |
| `caption` | `'Save the order'`, or `caption like '%TODO%'` | The caption DESCRIBE shows. `like` takes `%` and `_` |
| `disabled` | `true` / `false` | The activity's **current** state |

Each takes `=`, `!=`, `in (…)` and `not in (…)`; only `caption` takes `like`.

### Why a filter that selects nothing is an error

The statement reports by counting, so "your filter is wrong" and "the model had
nothing to change" both write nothing and exit 0. The ones that can be recognised
as impossible — an action word that names no Mendix action, `level` beside a
non-log `action`, `like` on an enumeration — are **MDL088**, refused by `mxcli
check` (with no project needed) and by `exec`, through the same function.

What remains is told apart in the report rather than merged:

```
Disabled 2 activities in 1 microflow: Sales.ACT_Do (2)
Unchanged: all 2 matching activities already disabled.
No activity matched in Sales (14 microflows) — nothing to disable.
```

The flag needs **Mendix 9.12+** (`Microflows$ActionActivity.disabled`). A matching
activity in an older document is **named, never patched** — adding a property the
type does not have is what makes a project Studio Pro cannot open.

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

-- Disabled: Studio Pro's right-click "Disable". The step stays in the flow,
-- drawn greyed out, and is skipped at runtime.
@disabled
call javascript action NanoflowCommons.RefreshEntity (
    EntityToRefresh = $Item
);

-- Multiple annotations stacked on a single activity
@position(400, 200)
@caption 'Persist product'
@color Blue
@annotation 'Step 2: Save to database'
commit $Product;
```

**Rules:**
- `@annotation` before an activity attaches the note to that activity
- `@annotation` before activity-binding metadata such as `@position`, `@caption`, `@color`, `@disabled`, or `@anchor` stays free-floating when later metadata binds the following activity
- `@annotation` at the end (no following activity) creates a free-floating note
- Escape single quotes by doubling: `@annotation 'Don''t forget'`
- `@position` always appears in DESCRIBE output; `@caption` only when custom; `@color` only when not Default
- `@disabled` marks **one action activity** inert — it is Studio Pro's right-click
  "Disable", and the generated step is greyed out and skipped at runtime. Its use is
  generating a step that is not yet right (a parameter mxcli cannot express) OFF rather
  than live, for a developer to fix and re-enable without losing the configuration.
  Mendix stores the flag on `Microflows$ActionActivity` and on nothing else, so it is
  refused (**MDL087**) on an `if`, `case`, `split type`, `loop`, `while`, `merge`,
  `join`, `return`, `raise error`, `break` and `continue` rather than being dropped.
  `@excluded` before a **statement** is the older spelling of the same flag and still
  works; before a `create microflow` the same word means "Exclude from project", which
  is a different setting (mendixlabs/mxcli#1139)
- DESCRIBE MICROFLOW shows `@` annotations before their activities
- `@start(x, y)` positions the **start event** and goes on the first statement, because the start has no statement of its own. Omit it and the start is derived — one spacing unit (160) left of the first activity, on its centre line — and a rewrite re-derives it so the start follows the activities when they move. A start that is not at the derived spot was placed by hand (in Studio Pro or with `@start`): it survives a rewrite that does not mention it, and DESCRIBE emits `@start` for it. An explicit `@start` overrides both (#951)
- `@position(x, y)` on a **parameter** goes inside the parameter list, ahead of the parameter it places — a parameter is a stored node with its own coordinates, and this is the only annotation it takes. Omit it and the parameters form a row along the top of the canvas (200;53, 300;53, …). The `@start` rule above applies unchanged: a parameter on that derived row is re-derived on a rewrite, one anywhere else was placed by hand, survives, and is emitted by DESCRIBE (#993). Before this, a hand-aligned parameter block was moved back onto the row by any rewrite — including a describe → exec of mxcli's own output:

  ```
  create or modify nanoflow MyModule.ACT_Clear (
    @position(-77, 0)
    $Feedback: MyModule.Feedback
  )
  ```
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

### RAISE ERROR is handler-only

`raise error;` builds Mendix's **error event**, which *re-raises the error
currently being handled*. Mendix therefore allows one only where an error is in
scope — that is, inside an `on error { ... }` block. Studio Pro will not even
let you draw the connection from the normal flow to an error event.

```mdl
-- ✅ inside a handler: an error IS in scope
call microflow Module.RiskyOperation()
on error {
  log error node 'Module' 'failed, re-raising';
  raise error;
};

-- ❌ on the main flow: MDL084, and mxbuild rejects it with
--    CE0710 "The main flow cannot join an error flow or end in an error event."
create microflow Module.Fail ()
begin
  raise error;
end;
```

Nesting does not change this: a `raise error;` inside an `if` or a `loop` on the
main flow is still on the main flow, and one inside a branch of a handler body is
still on the error flow.

Mendix has **no main-flow "throw" activity**. To fail deliberately from the normal
path, call a Java action that throws:

```mdl
create java action Module.JA_RaiseTechnicalError(Message: string not null) returns boolean as
$$ throw new com.mendix.systemwideinterfaces.MendixRuntimeException(Message); $$;
```

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

### Where the Error Path Goes — `merge` / `join`

`on error` has four forms, and they differ in **where the error path goes**, not
just in what it does. The difference is invisible in the MDL, so it is worth
knowing which one you are writing.

| Form | Error path |
|------|-----------|
| `on error continue` | No error path at all |
| `on error [without rollback] { … return/throw }` | Its own path, its own terminator |
| `on error [without rollback] { }` | **Not a no-op** — falls through to whatever the *enclosing branch* does next |
| `on error [without rollback] { … join L; }` | Rejoins the normal path at the merge labelled `L` |

The empty form is the one that surprises people. It means "on error, do whatever
the enclosing branch's continuation does" — which in a branch that returns
something else is a value nowhere in the text. Prefer `join` when you mean it.

```mdl
create microflow Module.Post (Payload: String) returns String
begin
  declare $Status String = 'sent';
  $r = call microflow Module.Send(Payload = $Payload) on error without rollback {
    log warning node 'Module' 'send failed, degrading';
    set $Status = 'degraded';
    join recovered;
  };
  join recovered;

  merge recovered;
  return $Status;
end;
```

**`merge <label>` declares a join point; `join <label>` sends a path to it.** The
label exists only in MDL — a Mendix `ExclusiveMerge` stores no name — so it is
resolved when the microflow is built and never written to the model.

Forward and backward references both resolve, so declaration order is free. A
backward one is how a **retry loop** is written, with the merge before the
activity:

```mdl
merge attempt;
$r = call microflow Module.Send(Payload = $Payload) on error without rollback {
  log warning node 'Module' 'retrying';
  join attempt;
};
return $r;
```

They also cover **crossed branches** — an inner split's branch landing where an
outer split's branch lands, which no nesting of `if` reproduces:

```mdl
if $A then
  if $B then join m1; else join m2; end if;
else
  join m1;
end if;

merge m1;
log info node 'Module' 'shared by two branches';
join m2;

merge m2;
return true;
```

Rules, all reported by `mxcli check` before anything is written:

| Rule | Refusal |
|------|---------|
| MDL-FLOW02 | `join L` with no `merge L`, or a `merge L` nothing joins (Mendix rejects a merge with no inbound path) |
| MDL-FLOW03 | The same label declared twice |
| MDL-FLOW04 | `merge` / `join` inside a `loop` or `while` body — a `LoopedActivity` owns its own object collection and a sequence flow cannot leave it, so there is no graph this could build. Use `break` / `continue` and put the merge outside |

A path that has already ended (`return`, `throw`, `join`) does **not** fall
through into a following `merge`: the merge starts a new path.

`DESCRIBE MICROFLOW` emits `merge` / `join` for an error path that rejoins the
normal one, so those microflows round-trip. A graph with **crossed branches and
no error handler** still describes to flattened MDL with the MDL-FLOW01 warning —
that half is not done, and the warning says not to re-execute it.
