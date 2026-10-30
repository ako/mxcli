# CREATE JAVA ACTION

## Synopsis

```sql
CREATE [ OR MODIFY ] JAVA ACTION module.Name [ < T [, ...] > ] [ FOLDER 'folder_path' ] ( parameters )
    RETURNS type [ AS $return_name ]
    [ EXPOSED AS 'caption' IN 'category' ]
    AS $$ java_code $$
    [ EXTRA CODE $$ helper_code $$ ]

DROP JAVA ACTION module.Name;
```

## Description

Creates a Java action with inline Java code. Java actions allow custom server-side logic written in Java to be called from microflows.

The action parameters, return type, and Java body are all specified inline. When the action is created, the corresponding Java source file is generated with the parameter boilerplate and the provided code body.

> **The `AS $$ ... $$` body is mandatory.** Unlike microflows, Java actions require a code body in every `CREATE` statement. If the body is omitted the parser reports `no viable alternative at input '...'`. Use a minimal stub body if the implementation is not yet written: `AS $$ return false; $$`.

If `OR MODIFY` is specified and the Java action already exists, its parameters, return type, exposed-as settings, and Java body are updated in place. The UUID and any existing Java source file path are preserved.

### Type Parameters

Java actions support generic entity handling through type parameters. A parameter declared as `ENTITY <pEntity>` creates a type parameter selector -- the caller chooses which entity type to use. Other parameters declared as bare `pEntity` (without `ENTITY`) receive instances of the selected entity type.

A type parameter that no `ENTITY <T>` parameter names -- one bound by the object the caller passes in -- is declared in the header after the action name: `module.Name<T>`. The header also fixes the order of the type parameters when it differs from the order the `ENTITY <T>` parameters would give. An unqualified type that is neither a declared type parameter nor `Microflow`/`Nanoflow` is refused (`MDL-JA01`): it would be written as an entity with no module, which Mendix reports as CE1613.

### Parameter Descriptions and the Return Value Name

A parameter's description is a `/** ... */` doc comment written before it -- the
same form an entity attribute's documentation takes. `RETURNS type AS $Name`
names the return value: the default name of the variable a microflow call stores
the result in, as `RETURNS type AS $Name` does on a microflow. Both are what Studio
Pro shows when a modeler drags the action into a microflow, and what `DESCRIBE JAVA
ACTION` prints.

On `CREATE OR MODIFY`, leaving either out keeps what is stored; an empty `/** */`
clears a description. A new action gets no descriptions and the return value name
`ReturnValueName`, as in Studio Pro.

### The Java Source File

The statement writes `javasource/<module>/actions/<Name>.java` the way mxbuild
generates it, byte for byte, so the next build leaves it unchanged.

The body is only in this file, not in the model, so commit `javasource/`
together with the `.mpr` and `mprcontents/`. A commit of the model alone gives a
clone an action with no logic. `mxcli exec` names each file it writes there on
its own line, for example
`Wrote javasource/sales/actions/JA_Ping.java (Java action body; commit it with the model)`,
and ends the run with `Files changed outside the model: N (…)`. A rewrite that
leaves the file's content as it was is not reported. `RENAME JAVA ACTION` moves
the file and `DROP JAVA ACTION` removes it; both are reported the same way. The action's
documentation -- the `/** ... */` before `CREATE` -- is the class javadoc above
`public class`; a rewrite without one keeps the stored documentation, in the model
and in the file. (Before ako/mxcli#1700 the file had no javadoc, so every rewrite
removed the one a build had written and the next build put it back.)

Studio Pro keeps three parts of the file when it regenerates it, and the statement
states all three: the code between `BEGIN USER CODE` and `END USER CODE` is the
`AS $$ ... $$` body; an `import` line in the body goes to the file's import list;
and `EXTRA CODE $$ ... $$` is the section between `BEGIN EXTRA CODE` and
`END EXTRA CODE` -- helper methods, constants and nested classes the body calls.
On `CREATE OR MODIFY` an omitted `EXTRA CODE` keeps the stored section, as an
omitted `EXPOSED AS` keeps the toolbox entry; `EXTRA CODE $$ $$` clears it.
`DESCRIBE JAVA ACTION` prints the imports a developer added and the section, so
executing its output on another project rebuilds the same file (ako/mxcli#1702).

### Exposed Actions

The optional `EXPOSED AS` clause makes the action visible in the Studio Pro toolbox under the specified category, allowing modelers to use it directly in microflows.

## Parameters

`module.Name`
:   The qualified name of the Java action.

`< T [, ...] >`
:   Optional. Declares type parameters, in order. Needed only for a type parameter that no `ENTITY <T>` parameter declares; a name that is a keyword or a primitive is quoted (`<"Entity">`).

`FOLDER 'folder_path'`
:   Optional. Places the document in the named module folder, creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`parameters`
:   Comma-separated parameter declarations. Each parameter has an optional `/** description */`, a name, colon, and type. Supported types:
    - Primitives: `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`
    - Entity: `Module.EntityName`
    - List: `List of Module.EntityName`
    - Enumeration: `ENUM Module.EnumName` or `Enumeration(Module.EnumName)`
    - String template: `StringTemplate(Sql)`, `StringTemplate(Oql)`
    - Type parameter declaration: `ENTITY <pEntity>` (declares the type parameter)
    - Type parameter reference: bare `pEntity` (uses the declared type parameter)
    - Add `NOT NULL` after the type to mark a parameter as required.

`RETURNS type`
:   The return type. Same type options as parameters.

`AS $return_name`
:   Optional. The return value's name. Omitted, the stored name is kept (`ReturnValueName` on a new action).

`EXPOSED AS 'caption' IN 'category'`
:   Optional. Makes the action visible in the toolbox with the given caption and category.

`AS $$ java_code $$`
:   The Java code body, enclosed in `$$` delimiters. **Mandatory** — the body cannot be omitted. An `import` line in it goes to the file's import list.

`EXTRA CODE $$ helper_code $$`
:   Optional. The file's EXTRA CODE section, beside `executeAction()`. Omitted, the stored section is kept; empty, it is cleared.

`OR MODIFY`
:   Makes the statement idempotent. If the Java action already exists, updates its parameters, return type, exposed-as settings, and body. Without this clause, creating a duplicate Java action is an error.

## Examples

Simple Java action returning a string:

```sql
CREATE JAVA ACTION MyModule.JA_FormatCurrency (
    Amount: Decimal NOT NULL,
    CurrencyCode: String NOT NULL
) RETURNS String
AS $$
    java.text.NumberFormat formatter = java.text.NumberFormat.getCurrencyInstance();
    formatter.setCurrency(java.util.Currency.getInstance(CurrencyCode));
    return formatter.format(Amount);
$$;
```

Java action with parameter descriptions and a named return value:

```sql
/** Converts text to upper case. */
CREATE JAVA ACTION MyModule.JA_ToUpper (
    /** The text to convert. */
    Input: String NOT NULL
) RETURNS String AS $UpperText
AS $$
    return Input.toUpperCase();
$$;
```

Java action with a helper in the EXTRA CODE section, and the import it needs:

```sql
CREATE JAVA ACTION MyModule.JA_Twice (
    N: Integer NOT NULL
) RETURNS Integer
AS $$
    import java.util.Objects;

    return twice(Objects.requireNonNull(N));
$$
EXTRA CODE $$
private static long twice(long n) {
	return n * 2L;
}
$$;
```

Java action with type parameters for generic entity handling:

```sql
CREATE JAVA ACTION MyModule.JA_Validate (
    EntityType: ENTITY <pEntity> NOT NULL,
    InputObject: pEntity NOT NULL
) RETURNS Boolean
EXPOSED AS 'Validate Entity' IN 'Validation'
AS $$
    return InputObject != null;
$$;
```

Java action with a type parameter that no entity selector declares -- the caller's object binds it:

```sql
CREATE JAVA ACTION MyModule.JA_Describe<pEntity> (
    InputObject: pEntity NOT NULL
) RETURNS String
AS $$
    return InputObject.toString();
$$;
```

Java action with a list parameter:

```sql
CREATE JAVA ACTION MyModule.JA_ExportToCsv (
    Records: List of MyModule.Customer NOT NULL,
    FilePath: String NOT NULL
) RETURNS Boolean
AS $$
    // Export logic here
    return true;
$$;
```

Drop a Java action:

```sql
DROP JAVA ACTION MyModule.JA_FormatCurrency;
```

### Idempotent upsert with OR MODIFY

```sql
mdl 1;
-- Initial creation
CREATE JAVA ACTION MyModule.JA_FormatCurrency (
    Amount: Decimal NOT NULL,
    CurrencyCode: String NOT NULL
) RETURNS String
AS $$
    return Amount.toString();
$$;

-- Later: update to add proper formatting (UUID preserved)
CREATE OR MODIFY JAVA ACTION MyModule.JA_FormatCurrency (
    Amount: Decimal NOT NULL,
    CurrencyCode: String NOT NULL
) RETURNS String
AS $$
    java.text.NumberFormat formatter = java.text.NumberFormat.getCurrencyInstance();
    formatter.setCurrency(java.util.Currency.getInstance(CurrencyCode));
    return formatter.format(Amount);
$$;
```

### Minimal stub body

When a Java action needs to exist in the project but the implementation is not yet written:

```sql
CREATE JAVA ACTION MyModule.JA_Placeholder (
    Input: String NOT NULL
) RETURNS Boolean
AS $$ return false; $$;
```

## See Also

[CREATE MICROFLOW](create-microflow.md), [LIST JAVA ACTIONS](/reference/query/list-microflows.md)
