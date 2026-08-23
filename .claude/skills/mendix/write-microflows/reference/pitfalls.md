# Pitfalls, unsupported syntax and build errors

Supporting reference for [write-microflows](../SKILL.md).

## Common Pitfalls

### 1. Object (Entity) Variables Cannot Be Declared

**Error**: CE0053 - "Selected type is not allowed" (+ CE0038, CE7247) — passes
`mxcli check` only until MDL043 was added; previously surfaced at mxbuild.

❌ **INCORRECT:**
```mdl
declare $Product Test.Product;             -- object Create Variable — rejected
declare $Product Test.Product = $In;       -- aliasing a parameter — rejected
declare $Product as Test.Product;          -- AS keyword also not supported
```

✅ **CORRECT** — get the object from a source that produces one:
```mdl
-- as a microflow parameter
create microflow Test.Save ($Product: Test.Product) returns boolean as $ok ...

-- from a retrieve (single object)
retrieve $Product from Test.Product where Code = $Code limit 1;

-- from a create object
$Product = create Test.Product (Name = $Name);

-- from a loop iterator
loop $Product in $Products
begin
  change $Product (Processed = true);
end
```

**Explanation**: `declare` becomes a *Create Variable* activity, which in Mendix
only holds primitive types. Objects (and lists) have no Create Variable form — use
the parameter/retrieve/create/loop sources above. To "keep a reference" to an
existing object, just use the variable you already have (`$In`, the loop variable,
the retrieve/create output); Mendix has no aliasing activity.

### 2. XPath Association Navigation

**Error**: CE0117 - "Error in expression"

❌ **INCORRECT:**
```mdl
-- Using simple association name
declare $CustomerName string = $Order/Customer/Name;
set $Name = $Product/Category/Name;
```

✅ **CORRECT:**
```mdl
-- Use fully qualified association name: Module.AssociationName
declare $CustomerName string = $Order/Shop.Order_Customer/Name;
set $Name = $Product/Shop.Product_Category/Name;
```

**Explanation**: XPath navigation requires the full qualified association name in the format `Module.AssociationName`.

### 3. Missing Attributes

**Error**: Attribute references must exist in entity definition

❌ **INCORRECT:**
```mdl
-- Referencing Status when it doesn't exist in Order entity
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

✅ **CORRECT:**
```mdl
-- First, ensure entity has the attributes
create persistent entity Shop.Order (
  OrderNumber: string(50),
  status: string(50),          -- ← Must be defined
  ProcessedDate: datetime       -- ← Must be defined
);

-- Then reference them
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

### 4. Flow Must End with RETURN

**Error**: CE0105 - "Activity cannot be the last object of a flow"

❌ **INCORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  -- Missing RETURN!
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  return $success;  -- ← Always required
end;
```

### 5. Unreachable Code After RETURN

**Error**: CE0104 - "Action activity is unreachable"

❌ **INCORRECT:**
```mdl
if $value < 0 then
  return false;
  log info 'This will never execute';  -- ← Unreachable!
end if;
```

✅ **CORRECT:**
```mdl
if $value < 0 then
  log info 'Value is negative';
  return false;
end if;
```

### 6. Unused Variables

**Warning**: CW0094 - "Variable 'X' is never used"

```mdl
-- Studio Pro will warn if parameters/variables are declared but never used
create microflow Test.Example (
  $ProductCode: string  -- ← Warning if never referenced
)
returns boolean as $success
begin
  set $success = true;  -- ProductCode never used
  return $success;
end;
```

### 7. Using SET on Undeclared Variables

**Error**: MDL executor validates that all variables used with `set` are declared first.

❌ **INCORRECT:**
```mdl
begin
  if $value > 10 then
    set $message = 'High';  -- ERROR: $Message not declared!
  end if;
  return true;
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $message string = '';  -- Declare first
  if $value > 10 then
    set $message = 'High';  -- Now SET works
  end if;
  return true;
end;
```

**Note**: Parameters are automatically declared by the parameter list. The `returns type as $Var` syntax names the return variable but does NOT declare it - you must still use `declare $Var type = value;` if you want to use SET on it.

### 8. RETURN Inside a Loop

**Error**: CE0068 - "End events cannot be placed inside a loop." (MDL062)

A `return` builds an End event, and Mendix does not allow one inside a loop —
whether the return sits in the loop body directly or inside a branch within it.

❌ **INCORRECT:**
```mdl
loop $Part in $PartList
begin
  if $Part/IsMatch then
    return true;   -- End event inside the loop
  end if;
end loop;
```

✅ **CORRECT** — leave the loop with `break`, and return once after it:
```mdl
declare $Found boolean = false;
loop $Part in $PartList
begin
  if $Part/IsMatch then
    set $Found = true;
    break;
  end if;
end loop;
return $Found;
```

### 9. Two Activities Creating the Same Variable

**Error**: CE0111 - "Duplicate variable name 'X'." (MDL063)

A microflow's variable names are unique **flow-wide**. Branches and loop bodies
do not open a scope, and parameters and loop iterators share the same namespace.
The trap is that every activity with an output **creates** its variable — there
is no form in which a call, a retrieve, an aggregate or an import mapping writes
into one that already exists.

❌ **INCORRECT:**
```mdl
declare $Session string = '';
$Session = call microflow Mod.Login();   -- the call creates $Session too
```

✅ **CORRECT** — let the activity create it:
```mdl
$Session = call microflow Mod.Login();
```

Assigning to an existing variable is fine, because `set` is a *Change Variable*
activity and creates nothing:

```mdl
declare $Session string = '';
set $Session = 'anonymous';   -- valid, any number of times
```

### 10. Calling a Rule or Microflow Inside an Expression

**Error**: CE0117 - "Error(s) in expression." (MDL066)

A Mendix **expression** has no user-callable functions. Its library is built-in
and unqualified (`length`, `toString`, `contains`, ...); microflows, rules and
Java actions are called by **activities**. So a qualified call in a value
position is not an expression at all — mxbuild rejects it whichever document it
names.

❌ **INCORRECT:**
```mdl
declare $Active Boolean = Sample.Rule_IsActive(IsActive = $IsActive);
declare $Next Integer = Sample.MF_Increment(N = $N);   -- a microflow is no better
```

✅ **CORRECT** — a microflow or Java action is an activity:
```mdl
$Next = call microflow Sample.MF_Increment(N = $N);
```

A **rule** has no call activity at all: Mendix can only evaluate one as a
decision's condition, and that is the single position where a bare qualified
call is valid MDL.

```mdl
if Sample.Rule_IsActive(IsActive = $IsActive) then
  ...
end if;
```

The name in that position must resolve to a real **rule** — a microflow there is
the same CE0117, and mxcli refuses the statement rather than writing it.
## Implicit Variable Creation (CE0111 Duplicate Variable)

These statements **implicitly create a new variable** with the name on the left side:

- `$Var = call microflow ...`
- `$Var = call java action ...`
- `$Var = call nanoflow ...`
- `$Var = create Module.Entity (...)`
- `retrieve $Var from Module.Entity ...`

**Do NOT use `declare` before these** — it creates a duplicate variable (CE0111):

```mdl
-- WRONG: Duplicate variable — DECLARE + CALL both create $Result
declare $Result boolean = false;
$Result = call java action Module.DoSomething();  -- CE0111!

-- CORRECT: Let CALL create the variable, use a different name if you need a default
declare $success boolean = false;
$CallResult = call java action Module.DoSomething();
set $success = $CallResult;

-- CORRECT: Simple pass-through (no default needed)
$Result = call java action Module.DoSomething();
return $Result;
```

The same applies to RETRIEVE:

```mdl
-- WRONG: declaring a list at all (CE0053/CE0038, MDL040) — and duplicate variable
declare $Items list of Module.Entity = empty;
retrieve $Items from Module.Entity where Active = true;  -- CE0111!

-- CORRECT: Let RETRIEVE create the variable
retrieve $Items from Module.Entity where Active = true;
```

**Note**: `returns type as $Var` in the microflow signature does NOT create an activity variable — it only names the return value. So `$Var = call java action ...` after `returns as $Var` is fine (one creation).

**Variables are scoped to the branch that creates them.** A variable first created
inside an `if`/`else` arm (including by `$Var = call ...`) is not visible outside
that arm. To use one value after a conditional, `declare` it *before* the
conditional and assign in every branch:

```mdl
-- WRONG: $GTotalText is created only in the `then` arm → not declared in `else`
if $HasVariance then
  $GTotalText = call microflow Module.FMT_Variance($v);   -- created here only
else
  set $GTotalText = 'n/a';                                 -- error: not declared
end if;

-- CORRECT: declare before, then set in each branch (call into a temp, then set)
declare $GTotalText string = '';
if $HasVariance then
  $Tmp = call microflow Module.FMT_Variance($v);
  set $GTotalText = $Tmp;
else
  set $GTotalText = 'n/a';
end if;
```

**Fallback chains reuse the same name = CE0111.** Because each `$Var = call microflow …` (or retrieve/create) is a *fresh* variable creation, the natural "try A, else try B" shape is invalid — even when the retry is inside an `if`:

```mdl
-- WRONG: the second call re-creates $Summary — CE0111
$Summary = call microflow M.Inner(Tag = 'description');
if trim($Summary) = '' then
  $Summary = call microflow M.Inner(Tag = 'summary');   -- CE0111!
end if;

-- CORRECT: one variable per call, then a plain `set` picks the winner
$Summary = call microflow M.Inner(Tag = 'description');
if trim($Summary) = '' then
  $SummaryFallback = call microflow M.Inner(Tag = 'summary');
  set $Summary = $SummaryFallback;
end if;
```

`mxcli check --references` catches this (CE0111) before the build.
## UNSUPPORTED Syntax (Will Cause Parse Errors)

**CRITICAL**: The following syntax is NOT implemented and will cause parse errors. Do NOT use these patterns:

### ROLLBACK Statement (Supported!)

```mdl
-- CORRECT: ROLLBACK is now supported
rollback $Order;

-- With REFRESH to update client UI
rollback $Order refresh;
```

**Use Case**: Revert uncommitted changes to an object. Useful when validation fails and you want to restore the object to its database state.

### RETRIEVE with LIMIT (Supported!)

```mdl
-- CORRECT: LIMIT is supported
retrieve $Product from Module.Product where IsActive = true limit 1;

-- LIMIT 1 returns a single entity (not a list)
-- Without LIMIT, returns a list
retrieve $ProductList from Module.Product where IsActive = true;
```

### WHILE Loop

```mdl
-- WHILE loops iterate while a condition is true
while $Counter < 10
begin
  set $Counter = $Counter + 1;
end while;

-- FOR EACH loops iterate over a list
loop $item in $ItemList
begin
  -- Process each item
end loop;
```

### CASE with string values, `else`, or an alias

`case … end case` **is supported** — see [CASE Statements (Enum Split)](#case-statements-enum-split)
above for the correct form. What is not supported is the SQL-flavoured spelling of
it: quoted values, an `else` fallback, and an `AS` alias all fail.

```mdl
-- WRONG: case values are not string literals (parse error)
case $Order/Status
  when 'Active' then set $Result = 1;
end case;

-- WRONG: case values are not qualified (parse error)
case $Order/Status
  when MyModule.Status.Active then set $Result = 1;
end case;

-- WRONG: no AS alias (parse error: mismatched input 'as' expecting WHEN)
case $Order/Status as s
  when Active then set $Result = 1;
end case;

-- WRONG: no else branch (MDL008 → mxbuild CE0079 + CE0773)
case $Order/Status
  when Active then set $Result = 1;
  else set $Result = 0;
end case;

-- CORRECT: bare enum values, one branch per value, including (empty)
case $Order/Status
  when Active then set $Result = 1;
  when Inactive then set $Result = 2;
  when (empty) then set $Result = 0;
end case;
```

An enum split is the *only* thing `case` does — it branches on an enumeration, not
on arbitrary expressions. For anything else (a string comparison, a numeric range),
use nested `if … else … end if`.

### TRY/CATCH Block

```mdl
-- WRONG: TRY/CATCH not supported
TRY
  commit $Order;
CATCH
  log error 'Commit failed';
end TRY;

-- CORRECT: Use ON ERROR on specific activities
commit $Order on error {
  log error 'Commit failed';
};
```

### BREAK/CONTINUE in Loops

```mdl
-- WRONG: BREAK/CONTINUE not supported
loop $item in $ItemList
begin
  if $item/Skip = true then
    continue;  -- NOT SUPPORTED
  end if;
  if $item/Stop = true then
    break;     -- NOT SUPPORTED
  end if;
end loop;

-- CORRECT: Use conditional logic
loop $item in $ItemList
begin
  if $item/Skip = false and $item/Stop = false then
    -- Process item
  end if;
end loop;
```

### Reserved Words as Identifiers

**Best practice: Always quote all identifiers** (attribute names, parameter names, entity names) with double quotes. This eliminates all reserved keyword conflicts and is always safe — quotes are stripped automatically by the parser.

> **Exception — never quote `$`-prefixed variable/parameter references.** The quote
> rule is for *bare* names (entities, attributes, associations, declared parameter
> names). Variable/parameter **references** in expressions stay **unquoted**:
> `$Customer/Name`, `$currentObject`, `retrieve … from $List`. Quoting the `$` token
> (`"$Customer"`) breaks resolution.

```mdl
create persistent entity Module."item" (
  "check": boolean default false,
  "text": string(500),
  "format": string(50),
  "value": decimal,
  "create": datetime,
  "delete": datetime
);
```

Quoted identifiers also work for microflow parameter names:
```mdl
create microflow Module."Process" ("select": string, "type": integer)
begin
  log info 'Processing';
  return;
end;
```
## Common Studio Pro Errors

| Error Code | Message | Fix |
|------------|---------|-----|
| CE0053 | Selected type is not allowed | Don't `declare` an object/list — get it from a parameter, retrieve, create, or loop (MDL043/MDL040) |
| CE0117 | Error in expression | Use qualified association names; use `not(expr)` not bare `not expr` |
| CE0104 | Action activity is unreachable | Remove code after RETURN |
| CE0105 | Must end with end event | Add RETURN statement |
| CE0008 | No action defined | Define action for activity |
| CW0094 | Variable never used | Remove unused variables or use them |
| MDL | Variable not declared | Use `declare $var type = value;` before SET |
