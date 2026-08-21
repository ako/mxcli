# Creating an Entity

Entities are the foundation of any Mendix application -- they define your data model. In this page you'll create a `Product` entity with several attributes, verify it, and then link it to another entity with an association.

## Create a simple entity

The `CREATE PERSISTENT ENTITY` statement defines an entity that is stored in the database. Attributes go inside parentheses, separated by commas:

```sql
CREATE PERSISTENT ENTITY MyModule.Product (
    Name: String(200) NOT NULL,
    Price: Decimal,
    IsActive: Boolean DEFAULT true
);
```

Let's break this down:

| Part | Meaning |
|------|---------|
| `PERSISTENT` | The entity is stored in the database (as opposed to `NON-PERSISTENT`, which exists only in memory) |
| `MyModule.Product` | Fully qualified name: module dot entity name |
| `String(200)` | A string attribute with a maximum length of 200 characters |
| `NOT NULL` | The attribute is required -- it cannot be left empty |
| `Decimal` | A decimal number (no precision/scale needed for basic use) |
| `Boolean DEFAULT true` | A boolean that defaults to `true` when a new object is created |

Run it from the command line:

```bash
mxcli -p app.mpr -c "CREATE PERSISTENT ENTITY MyModule.Product (Name: String(200) NOT NULL, Price: Decimal, IsActive: Boolean DEFAULT true);"
```

Or save it to a file and execute:

```bash
mxcli exec create-product.mdl -p app.mpr
```

## Verify the entity

Use `DESCRIBE ENTITY` to confirm your entity was created correctly:

```bash
mxcli -p app.mpr -c "DESCRIBE ENTITY MyModule.Product"
```

This prints the full MDL definition of the entity, including all attributes and their types. You should see `Name`, `Price`, and `IsActive` listed.

## Add an association

Associations link entities together. To connect `Product` to an existing `Order` entity, create a reference association:

```sql
CREATE ASSOCIATION MyModule.Order_Product
    FROM MyModule.Order TO MyModule.Product
    TYPE Reference;
```

This creates a many-to-one relationship: each `Order` can reference one `Product`. Use `ReferenceSet` instead of `Reference` for many-to-many.

The naming convention `Order_Product` follows Mendix best practices -- the "from" entity name comes first, then the "to" entity.

### Association types

| Type | Meaning | Example |
|------|---------|---------|
| `Reference` | Many-to-one (or one-to-one) | An Order references one Product |
| `ReferenceSet` | Many-to-many | An Order can have multiple Products |

### Delete behavior

You can specify what happens when the "to" entity is deleted:

```sql
CREATE ASSOCIATION MyModule.Order_Product
    FROM MyModule.Order TO MyModule.Product
    TYPE Reference
    DELETE_BEHAVIOR PREVENT;
```

Options: `PREVENT` (block deletion if referenced), `CASCADE` (delete the associated objects too), or leave it out for the default behavior — delete the object and null out the references.

## Using OR MODIFY for idempotent scripts

If you want to run the same script repeatedly without errors, use `CREATE OR MODIFY`:

```sql
CREATE OR MODIFY PERSISTENT ENTITY MyModule.Product (
    Name: String(200) NOT NULL,
    Price: Decimal,
    IsActive: Boolean DEFAULT true,
    Description: String(unlimited)
);
```

If the entity already exists, this updates it to match the new definition. If it doesn't exist, it creates it. This is especially useful during iterative development.

## More attribute types

Here's a fuller example showing the attribute types you'll use most often:

```sql
CREATE PERSISTENT ENTITY MyModule.Product (
    Name: String(200) NOT NULL,
    Description: String(unlimited),
    Price: Decimal,
    Quantity: Integer,
    Weight: Long,
    IsActive: Boolean DEFAULT true,
    CreatedDate: DateTime,
    Status: MyModule.ProductStatus
);
```

The last attribute references an enumeration (`MyModule.ProductStatus`). You'd create that separately:

```sql
CREATE ENUMERATION MyModule.ProductStatus (
    Active 'Active',
    Discontinued 'Discontinued',
    OutOfStock 'Out of Stock'
);
```

## Extending a system entity

To create an entity that inherits from a system entity (like `System.Image` for file storage), use `EXTENDS`. Note that `EXTENDS` must come **before** the opening parenthesis:

```sql
-- Correct: EXTENDS before (
CREATE PERSISTENT ENTITY MyModule.ProductPhoto EXTENDS System.Image (
    PhotoCaption: String(200)
);
```

```sql
-- Wrong: EXTENDS after ( -- this will cause a parse error
CREATE PERSISTENT ENTITY MyModule.ProductPhoto (
    PhotoCaption: String(200)
) EXTENDS System.Image;
```

## Common mistakes

**String needs an explicit length.** `String` alone is not valid -- you must specify the maximum length:

```sql
-- Wrong
Name: String

-- Correct
Name: String(200)

-- For unlimited length
Description: String(unlimited)
```

**EXTENDS must come before the parenthesis.** See the example above. This is a common source of parse errors.

**Module must exist.** The module in the qualified name (e.g., `MyModule` in `MyModule.Product`) must already exist in the project. Check with `SHOW MODULES`.
