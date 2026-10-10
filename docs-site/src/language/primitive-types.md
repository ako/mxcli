# Primitive Types

This page documents each primitive type available for entity attributes in MDL.

## String

Variable-length text data.

```sql
String              -- Default length (200)
String(n)           -- Specific length (1 to unlimited)
String(unlimited)   -- No length limit
```

**Examples:**

```sql
Name: String(200)
Description: String(unlimited)
Code: String(10)
```

**Default value format:**

```sql
Name: String(200) DEFAULT 'Unknown'
Code: String(10) DEFAULT ''
```

## Integer

32-bit signed integer.

```sql
Integer
```

**Range:** -2,147,483,648 to 2,147,483,647

```sql
Quantity: Integer
Age: Integer DEFAULT 0
Priority: Integer NOT NULL DEFAULT 1
```

## Long

64-bit signed integer.

```sql
Long
```

**Range:** -9,223,372,036,854,775,808 to 9,223,372,036,854,775,807

```sql
FileSize: Long
TotalCount: Long DEFAULT 0
```

## Decimal

High-precision decimal number with up to 20 digits total.

```sql
Decimal
```

```sql
Price: Decimal
Amount: Decimal DEFAULT 0
TaxRate: Decimal DEFAULT 0.21
```

## Boolean

True/false value. Boolean attributes **must** have a `DEFAULT` value (enforced by Mendix Studio Pro).

```sql
Boolean DEFAULT TRUE
Boolean DEFAULT FALSE
```

```sql
IsActive: Boolean DEFAULT TRUE
Enabled: Boolean DEFAULT TRUE
Deleted: Boolean DEFAULT FALSE
```

## DateTime

Date and time stored as a UTC timestamp.

```sql
DateTime
```

```sql
CreatedAt: DateTime
ModifiedAt: DateTime
ScheduledFor: DateTime
```

A DateTime is localized (shown in the user's time zone) by default. For a
calendar date that must not shift with the time zone, such as a birth date, add
`NOT LOCALIZED` — see [Constraints](./constraints.md#localized--not-localized-datetime-only):

```text
BirthDate: DateTime NOT LOCALIZED
```

DateTime values include both date and time components. To show only the date, format it on the widget; there is no `Date` type.

## No `Date`, `Float` or `Currency`

Mendix has no date-only attribute type: a date is a `DateTime`, and showing only
its date part is a formatting choice on the widget. `Float` and `Currency` were
removed in Mendix 7 in favour of `Decimal`. MDL refuses `float` and `currency` with
the type to write instead — earlier versions accepted them and silently stored a
`String`.

`date` was always stored as a `DateTime`, so in a script without the `mdl 1;`
header it is a deprecated spelling of `DateTime`: it builds a `DateTime` and warns
`MDL-DEPR160`, and `mxcli fmt --upgrade` rewrites it to `DateTime`. Under `mdl 1;`
it is an error.

## AutoNumber

Auto-incrementing integer, typically used for human-readable identifiers. The `DEFAULT` value specifies the starting number.

```sql
AutoNumber
```

```sql
OrderId: AutoNumber NOT NULL UNIQUE DEFAULT 1
CustomerId: AutoNumber
```

AutoNumber attributes automatically receive the next value on object creation. They are typically combined with `NOT NULL` and `UNIQUE` [constraints](./constraints.md).

## Binary

Binary data for files, images, and other non-text content.

```sql
Binary
```

```sql
ProfileImage: Binary
Document: Binary
Thumbnail: Binary
```

File metadata (name, size, MIME type) is stored separately by Mendix. Maximum size is configurable per attribute.

## HashedString

Securely hashed string, used for password storage. Values are one-way hashed and cannot be retrieved -- comparison is done by hashing the input and comparing hashes.

```sql
HashedString
```

```sql
Password: HashedString
```

## Enumeration Reference

References an enumeration type defined with `CREATE ENUMERATION`. See [Enumerations](./enumerations.md) for details.

```sql
Enumeration(<Module.EnumerationName>)
```

```sql
Status: Enumeration(Sales.OrderStatus)
Priority: Enumeration(Core.Priority) DEFAULT Core.Priority.Normal
Type: Enumeration(MyModule.ItemType) NOT NULL
```

**Default value format:**

```sql
-- Fully qualified (preferred)
DEFAULT Module.EnumName.ValueName

-- Legacy string literal form
DEFAULT 'ValueName'
```

## See Also

- [Data Types](./data-types.md) -- type system overview
- [Constraints](./constraints.md) -- NOT NULL, UNIQUE, DEFAULT
- [Type Mapping](./type-mapping.md) -- MDL to Mendix to database mapping
