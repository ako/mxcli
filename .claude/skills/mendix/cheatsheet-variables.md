# MDL Variable Cheatsheet

Quick reference for variable declarations in MDL microflows.

## Declaration Syntax

| Type | Syntax | Example |
|------|--------|---------|
| String | `declare $name string = 'value';` | `declare $message string = '';` |
| Integer | `declare $name integer = 0;` | `declare $count integer = 0;` |
| Boolean | `declare $name boolean = true;` | `declare $IsValid boolean = true;` |
| Decimal | `declare $name decimal = 0.0;` | `declare $Amount decimal = 0;` |
| DateTime | `declare $name datetime = [%CurrentDateTime%];` | `declare $Now datetime = [%CurrentDateTime%];` |
| Entity | `declare $name as Module.Entity;` | `declare $Customer as Sales.Customer;` |
| List | **Do not declare** — use a parameter, `retrieve`, or `create list` | `$Orders = create list of Sales.Order;` |

## Key Rules

1. **Primitives**: Use `declare $var type = value;` (initialization required)
2. **Entities**: Use `declare $var as Module.Entity;` (use AS keyword, no initialization)
3. **Lists**: Never `declare` a list — Mendix forbids the Create Variable activity from producing a list (CE0053/CE0038, flagged as MDL040 by `mxcli check`). Get the list from a microflow parameter, a `retrieve`, or `$var = create list of Module.Entity;`
4. **SET requires DECLARE**: Always declare variables before using SET
5. **Parameters are pre-declared**: Microflow parameters don't need DECLARE

## Common Mistakes

### Entity Declaration

```mdl
-- WRONG: Missing AS keyword
declare $Product Module.Product = empty;

-- CORRECT: Use AS for entity types
declare $Product as Module.Product;
```

### SET Without DECLARE

```mdl
-- WRONG: Variable not declared
if $value > 10 then
  set $message = 'High';  -- ERROR!
end if;

-- CORRECT: Declare first
declare $message string = '';
if $value > 10 then
  set $message = 'High';
end if;
```

### Lists (never declared)

```mdl
-- WRONG: declaring a list — Mendix rejects it (CE0053/CE0038, MDL040)
declare $Items list of Module.Item = empty;

-- CORRECT: build the list without declare
$Items = create list of Module.Item;          -- empty list to accumulate into
retrieve $Items from Module.Item where ...;    -- or populate from the database
-- or accept it as a parameter: create microflow M.Process ($Items: list of Module.Item) ...
```

## Special Values

| Value | Usage |
|-------|-------|
| `empty` | Null/empty value for any type |
| `[%CurrentDateTime%]` | Current date and time |
| `[%CurrentUser%]` | Currently logged in user object |
| `true` / `false` | Boolean literals |

## Parameter vs Variable

```mdl
create microflow Module.Example (
  $Input: string,              -- Parameter: auto-declared
  $entity: Module.Customer     -- Parameter: auto-declared
)
returns boolean
begin
  -- Parameters $Input and $Entity are already available

  declare $Result boolean = true;  -- Local variable: must declare
  declare $Temp as Module.Order;   -- Local entity: must declare

  return $Result;
end;
/
```

## Variable Scope

- Parameters: Available throughout the microflow
- DECLARE variables: Available from declaration point forward
- Loop variables: Only available inside the loop body

```mdl
loop $item in $ItemList
begin
  -- $Item is available here (derived from list type)
  set $count = $count + 1;
end loop;
-- $Item is NOT available here
```
