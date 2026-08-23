# CREATE MICROFLOW

## Synopsis

```sql
CREATE [ OR REPLACE ] MICROFLOW module.Name
    [ ( DECLARE $param : type [, ...] ) ]
    [ RETURN type ]
    [ FOLDER 'path' ]
BEGIN
    statements
END
```

## Description

Creates a new microflow in the specified module. Microflows are the server-side logic building blocks in Mendix -- they execute on the application server and have access to the full set of activities including database operations, external service calls, and security-sensitive actions.

If `OR REPLACE` is specified and a microflow with the same qualified name already exists, it is replaced. Otherwise, creating a microflow with an existing name is an error.

The microflow body consists of a sequence of statements enclosed in `BEGIN ... END`. Statements are executed in order, with control flow managed by `IF`, `LOOP`, `WHILE`, and `RETURN` constructs.

### Microflow Parameters

Microflow parameters are declared in parentheses after the name. Each parameter has a name (prefixed with `$`), a colon, and a type. Parameters become variables available throughout the microflow body.

### Return Type

If the microflow returns a value, specify `RETURN type` after the parameter list. The type can be any primitive type, an entity type, or a list type. Every execution path must end with a `RETURN` statement when a return type is declared.

### Folder Placement

The optional `FOLDER` clause places the microflow in a subfolder within the module. Nested folders use `/` as separator. Missing folders are created automatically.

### Activities

The following activities are available inside the microflow body.

**Variable Declaration and Assignment**

```sql
DECLARE $Var Type = value;
DECLARE $Entity Module.Entity;
DECLARE $List List of Module.Entity = empty;
SET $Var = expression;
```

Primitive types: `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`. Entity declarations do not use `= empty`. List declarations require `= empty` to initialize an empty list.

**Object Operations**

```sql
$Var = CREATE Module.Entity ( Attr1 = value1, Attr2 = value2 );
CHANGE $Entity ( Attr = value );
COMMIT $Entity [ WITH EVENTS ] [ REFRESH ];
DELETE $Entity;
ROLLBACK $Entity [ REFRESH ];
```

`CREATE` instantiates a new object with initial attribute values. `CHANGE` modifies attributes on an existing object. `COMMIT` persists changes to the database -- `WITH EVENTS` triggers before/after commit event handlers, `REFRESH` updates client-side state. `ROLLBACK` reverts uncommitted changes to an object.

**Retrieval**

```sql
-- Database retrieve with optional XPath constraint
RETRIEVE $Var FROM Module.Entity [ WHERE condition ] [ LIMIT n ];

-- Retrieve by association
RETRIEVE $List FROM $Parent/Module.AssocName;
```

`RETRIEVE ... LIMIT 1` returns a single entity. Without `LIMIT` or with `LIMIT` greater than 1, it returns a list. Retrieve by association traverses an association from a known object.

**Calls**

```sql
$Result = CALL MICROFLOW Module.Name ( Param = $value );
$Result = CALL NANOFLOW Module.Name ( Param = $value );
$Result = CALL JAVA ACTION Module.Name ( Param = value );
```

Call another microflow, nanoflow, or Java action. Parameters are passed by name. The result can be assigned to a variable when the callee has a return type. If no return value is needed, omit the `$Result =` prefix.

**UI Actions**

```sql
SHOW PAGE Module.PageName ( $Param = $value );
CLOSE PAGE;
```

`SHOW PAGE` opens a page, passing parameters by name. `CLOSE PAGE` closes the current page.

**Validation and Logging**

```sql
VALIDATION FEEDBACK $Entity/Attribute MESSAGE 'message';
LOG INFO | WARNING | ERROR [ NODE 'name' ] 'message';
```

`VALIDATION FEEDBACK` adds a validation error to a specific attribute on an object. `LOG` writes to the application log at the specified level, optionally tagged with a log node name.

**Execute Database Query**

```sql
$Result = EXECUTE DATABASE QUERY Module.Connector.QueryName;
```

Executes a Database Connector query. The three-part name identifies the connector module, connection, and query. Supports `DYNAMIC`, parameters, and `CONNECTION` override.

**Control Flow**

```sql
IF condition THEN
    statements
[ ELSE
    statements ]
END IF;

LOOP $Item IN $List BEGIN
    statements
END LOOP;

WHILE condition BEGIN
    statements
END WHILE;

RETURN $value;
```

`IF` branches on a boolean expression. `LOOP` iterates over each item in a list. `WHILE` loops while a condition holds true. `RETURN` ends execution and returns a value (required when the microflow declares a return type).

**Error Handling**

```sql
-- Suffix on any activity (except EXECUTE DATABASE QUERY)
activity ON ERROR CONTINUE;
activity ON ERROR ROLLBACK;
activity ON ERROR {
    handler_statements
};
```

Error handling is attached as a suffix to an individual activity. `ON ERROR CONTINUE` suppresses the error and continues. `ON ERROR ROLLBACK` rolls back the current transaction. `ON ERROR { ... }` executes custom error-handling logic.

### Annotations

Annotations are placed before an activity to control visual appearance in the microflow editor:

```sql
@position(x, y)          -- Canvas position
@start(x, y)             -- Canvas position of the start event (first statement only)
@caption 'text'          -- Custom caption
@color Green             -- Background color
@annotation 'text'       -- Visual note attached to next activity
```

`@start` positions the start event, which has no statement of its own, so it is
written on the first statement — the one the start flows into. It is optional:
omit it and the start is placed one spacing unit left of the first activity, on
that activity's centre line, and a later rewrite re-derives it so it follows the
activities when they move.

A start that is *not* at that derived spot — one dragged somewhere in Studio Pro,
or written with `@start` — is treated as placed on purpose. It survives a rewrite
that does not mention it, and `DESCRIBE` emits an `@start` line for it so the
description reproduces the flow exactly. An explicit `@start` overrides both.

## Parameters

`module.Name`
:   The qualified name of the microflow (`Module.MicroflowName`). The module must already exist.

`$param : type`
:   A parameter declaration. The name must start with `$`. The type can be:
    - Primitive: `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`
    - Entity: `Module.EntityName`
    - List: `List of Module.EntityName`
    - Enumeration: `Enumeration(Module.EnumName)`

`RETURN type`
:   The return type of the microflow. Same type options as parameters.

`FOLDER 'path'`
:   Optional folder path within the module. Nested folders use `/` separator (e.g., `'Orders/Processing'`).

## Examples

Simple microflow that creates and commits an object:

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

Microflow with parameters and conditional logic:

```sql
CREATE MICROFLOW Sales.ACT_ApproveOrder
    (DECLARE $Order: Sales.Order)
    RETURN Boolean
BEGIN
    IF $Order/Status = Sales.OrderStatus.Pending THEN
        CHANGE $Order (Status = Sales.OrderStatus.Approved);
        COMMIT $Order WITH EVENTS;
        LOG INFO NODE 'OrderProcessing' 'Order approved';
        RETURN true;
    ELSE
        VALIDATION FEEDBACK $Order/Status MESSAGE 'Only pending orders can be approved';
        RETURN false;
    END IF;
END;
```

Microflow with a loop over a retrieved list:

```sql
CREATE MICROFLOW Sales.ACT_DeactivateExpiredCustomers
    RETURN Integer
FOLDER 'Scheduled'
BEGIN
    DECLARE $Count Integer = 0;
    RETRIEVE $Customers FROM Sales.Customer
        WHERE [IsActive = true AND ExpiryDate < [%CurrentDateTime%]];
    LOOP $Customer IN $Customers BEGIN
        CHANGE $Customer (IsActive = false);
        COMMIT $Customer;
        SET $Count = $Count + 1;
    END LOOP;
    LOG INFO NODE 'Maintenance' 'Deactivated expired customers';
    RETURN $Count;
END;
```

Microflow with error handling:

```sql
CREATE MICROFLOW Integration.ACT_SyncData
BEGIN
    DECLARE $Result Integration.SyncResult;
    $Result = CREATE Integration.SyncResult (
        StartTime = [%CurrentDateTime%],
        Status = 'Running'
    );
    COMMIT $Result;

    $Result = CALL MICROFLOW Integration.SUB_FetchExternalData (
        SyncResult = $Result
    ) ON ERROR {
        CHANGE $Result (Status = 'Failed');
        COMMIT $Result;
        LOG ERROR NODE 'Integration' 'Sync failed';
    };

    CHANGE $Result (
        Status = 'Completed',
        EndTime = [%CurrentDateTime%]
    );
    COMMIT $Result;
END;
```

Microflow calling a Java action:

```sql
CREATE MICROFLOW MyModule.ACT_GenerateReport
    (DECLARE $StartDate: DateTime, DECLARE $EndDate: DateTime)
    RETURN String
BEGIN
    DECLARE $Report String = '';
    $Report = CALL JAVA ACTION MyModule.JA_GenerateReport (
        StartDate = $StartDate,
        EndDate = $EndDate
    );
    RETURN $Report;
END;
```

Using `OR REPLACE` to update an existing microflow:

```sql
CREATE OR REPLACE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
    DECLARE $Order Sales.Order;
    $Order = CREATE Sales.Order (
        OrderDate = [%CurrentDateTime%],
        Status = 'Draft',
        CreatedBy = '[%CurrentUser%]'
    );
    COMMIT $Order WITH EVENTS;
    RETURN $Order;
END;
```

## See Also

[CREATE NANOFLOW](create-nanoflow.md), [DROP MICROFLOW](drop-microflow.md), [CREATE JAVA ACTION](create-java-action.md), [DESCRIBE MICROFLOW](/reference/query/describe-microflow.md), [GRANT EXECUTE ON MICROFLOW](/reference/security/grant.md)
