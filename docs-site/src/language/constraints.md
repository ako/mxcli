# Constraints

Constraints restrict the values an attribute can hold. They are specified after the type in an attribute definition.

## Syntax

```sql
<name>: <type> [NOT NULL [ERROR '<message>']] [UNIQUE [ERROR '<message>']] [DEFAULT <value>]
```

Constraints must appear in this order:

1. `NOT NULL` (optionally with `ERROR`)
2. `UNIQUE` (optionally with `ERROR`)
3. `DEFAULT`

## NOT NULL

Marks the attribute as required. The value cannot be empty.

```sql
Name: String(200) NOT NULL
```

With a custom error message displayed to the user:

```sql
Name: String(200) NOT NULL ERROR MESSAGE 'Name is required'
```

## UNIQUE

Enforces that the value must be unique across all objects of the entity.

```sql
Email: String(200) UNIQUE
```

With a custom error message:

```sql
Email: String(200) UNIQUE ERROR MESSAGE 'Email already exists'
```

## DEFAULT

Sets the initial value when a new object is created.

```sql
Count: Integer DEFAULT 0
IsActive: Boolean DEFAULT TRUE
Status: Enumeration(Sales.OrderStatus) DEFAULT 'Draft'
Name: String(200) DEFAULT 'Unknown'
```

Default value syntax varies by type:

| Type | Default Syntax | Examples |
|------|----------------|----------|
| String | `DEFAULT 'value'` | `DEFAULT ''`, `DEFAULT 'Unknown'` |
| Integer | `DEFAULT n` | `DEFAULT 0`, `DEFAULT -1` |
| Long | `DEFAULT n` | `DEFAULT 0` |
| Decimal | `DEFAULT n.n` | `DEFAULT 0`, `DEFAULT 0.00`, `DEFAULT 99.99` |
| Boolean | `DEFAULT TRUE/FALSE` | `DEFAULT TRUE`, `DEFAULT FALSE` |
| AutoNumber | `DEFAULT n` | `DEFAULT 1` (starting value) |
| Enumeration | `DEFAULT Module.Enum.Value` | `DEFAULT Shop.Status.Active`, `DEFAULT 'Pending'` |

Omitting the `DEFAULT` clause means no default value is set:

```sql
OptionalField: String(200)
```

## LOCALIZED / NOT LOCALIZED (DateTime only)

Studio Pro's **Localize** setting on a DateTime attribute (`LocalizeDate` in the
model). A localized DateTime is converted to the user's time zone when it is
shown or entered; a non-localized one is not, which is what a calendar date such
as a birth date needs — otherwise it can show as the day before for a user west
of UTC.

```text
OrderDate: DateTime,                -- localized (Mendix's default)
BirthDate: DateTime NOT LOCALIZED,  -- not localized
```

- `LOCALIZED` states the default explicitly. It is mostly useful on `ALTER ENTITY
  … MODIFY ATTRIBUTE` to switch a non-localized attribute back, the way
  `NULLABLE` clears `NOT NULL`.
- On a **new** attribute, no clause means localized.
- On a **rewrite** — `CREATE OR MODIFY ENTITY` or `MODIFY ATTRIBUTE` — no clause
  keeps the value already stored (a Studio Pro-authored non-localized attribute
  stays non-localized); a stated clause wins.
- On a **view entity** the value follows the OQL source attribute and needs no
  clause; a stated one overrides it, and a mismatch with the source is CE6770.
- On any type other than DateTime the clause is an error: there is nowhere to
  store it.
- `DESCRIBE ENTITY` prints ` not localized` (right after the type) when the
  attribute is not localized, and nothing otherwise, so its output round-trips.

The deprecated `Date` type (MDL-DEPR160) is a localized DateTime. For a
date-only value write `DateTime NOT LOCALIZED`.

## Combining Constraints

All three constraints can be used together:

```sql
Email: String(200) NOT NULL ERROR MESSAGE 'Email is required'
                   UNIQUE ERROR MESSAGE 'Email already exists'
                   DEFAULT ''
```

More examples:

```sql
CREATE PERSISTENT ENTITY Sales.Product (
  -- Required only
  Name: String(200) NOT NULL,

  -- Required with custom error
  SKU: String(50) NOT NULL ERROR MESSAGE 'SKU is required for all products',

  -- Unique only
  Barcode: String(50) UNIQUE,

  -- Required and unique with custom errors
  ProductCode: String(20) NOT NULL ERROR MESSAGE 'Product code required'
                          UNIQUE ERROR MESSAGE 'Product code must be unique',

  -- Default only
  Quantity: Integer DEFAULT 0,

  -- No constraints
  Description: String(unlimited)
);
```

## Validation Rule Mapping

Under the hood, constraints map to Mendix validation rules:

| MDL Constraint | Mendix Validation Rule |
|----------------|----------------------|
| `NOT NULL` | `DomainModels$RequiredRuleInfo` |
| `UNIQUE` | `DomainModels$UniqueRuleInfo` |

`LOCALIZED` / `NOT LOCALIZED` is not a validation rule: it is the
`LocalizeDate` property of the attribute's `DomainModels$DateTimeAttributeType`.

## See Also

- [Primitive Types](./primitive-types.md) -- type reference
- [Attributes](./attributes.md) -- full attribute definition syntax
- [ALTER ENTITY](./alter-entity.md) -- modifying constraints on existing entities
