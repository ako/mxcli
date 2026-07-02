# CREATE CONSTANT

## Synopsis

    CREATE [ OR MODIFY ] CONSTANT module.name TYPE data_type [ DEFAULT value ]

## Description

`CREATE CONSTANT` defines a named constant in a module. Constants hold configuration values (API URLs, feature flags, limits) that can differ between environments. In Mendix, constant values can be overridden per runtime configuration without changing the model.

The `TYPE` clause specifies the constant's data type. Supported types include `String`, `Integer`, `Long`, `Decimal`, `Boolean`, and `DateTime`.

The optional `DEFAULT` clause sets the constant's default value. This is the value used at runtime unless overridden by environment configuration.

If `OR MODIFY` is specified, the statement is idempotent. If the constant already exists, its type and default value are updated.

Constant values can also be overridden per deployment configuration using `ALTER SETTINGS CONSTANT`.

## Parameters

**OR MODIFY**
: Makes the statement idempotent. If the constant already exists, its definition is updated. Without this clause, creating a duplicate constant is an error.

**module.name**
: The qualified name of the constant in the form `Module.ConstantName`. The module must already exist.

**data_type**
: The constant's data type. One of: `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`.

**DEFAULT value**
: The default value for the constant. String values are single-quoted. Numeric values are bare. Boolean values are `true` or `false`.

## Examples

### String constant for API URL

```sql
CREATE CONSTANT MyModule.ApiBaseUrl TYPE String DEFAULT 'https://api.example.com';
```

### Integer constant for configuration

```sql
CREATE CONSTANT MyModule.MaxRetries TYPE Integer DEFAULT 3;
```

### Boolean feature flag

```sql
CREATE CONSTANT MyModule.EnableLogging TYPE Boolean DEFAULT true;
```

### Idempotent with OR MODIFY

```sql
CREATE OR MODIFY CONSTANT MyModule.ApiBaseUrl TYPE String DEFAULT 'https://api.example.com/v2';
```

### Constant with an empty default

A `DEFAULT` is always required. For secrets, use an empty default and set the
real value per configuration (see below):

```sql
CREATE CONSTANT MyModule.DatabasePassword TYPE String DEFAULT '';
```

### Override constant per configuration

```sql
-- Create the constant
CREATE CONSTANT MyModule.ApiBaseUrl TYPE String DEFAULT 'https://api.example.com';

-- Override in a specific runtime configuration
ALTER SETTINGS CONSTANT 'MyModule.ApiBaseUrl' VALUE 'https://staging.example.com' IN CONFIGURATION 'Staging';
```

## See Also

[CREATE ENTITY](create-entity.md), [CREATE ENUMERATION](create-enumeration.md)
