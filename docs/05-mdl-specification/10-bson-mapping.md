# MDL to BSON Mapping

This document describes how MDL constructs map to BSON structures in Mendix MPR files.

## Table of Contents

1. [MPR File Format Overview](#mpr-file-format-overview)
2. [BSON Structure Conventions](#bson-structure-conventions)
3. [Entity Mapping](#entity-mapping)
4. [Attribute Mapping](#attribute-mapping)
5. [Validation Rule Mapping](#validation-rule-mapping)
6. [Index Mapping](#index-mapping)
7. [Association Mapping](#association-mapping)
8. [Enumeration Mapping](#enumeration-mapping)
9. [Text and Localization](#text-and-localization)
10. [ID Generation](#id-generation)
11. [Complete Example](#complete-example)
12. [Implementing New Document Types](#implementing-new-document-types)
13. [Page Widget Mapping](#page-widget-mapping)

---

## MPR File Format Overview

Mendix projects are stored in `.mpr` files which contain:

### MPR v1 (Mendix < 10.18)
Single SQLite database file with:
- `Unit` table: Document metadata
- `UnitContents` table: BSON document contents

### MPR v2 (Mendix >= 10.18)
SQLite metadata file + separate content files:
- `.mpr` file: SQLite with `Unit` table (metadata only)
- `mprcontents/` folder: Individual `.mxunit` files containing BSON

### Unit Types

| UnitType | Document Type |
|----------|---------------|
| `DomainModels$DomainModel` | Domain model (entities, associations) |
| `DomainModels$ViewEntitySourceDocument` | OQL query for VIEW entities |
| `microflows$microflow` | Microflow definition |
| `microflows$nanoflow` | Nanoflow definition |
| `pages$page` | Page definition |
| `pages$layout` | Layout definition |
| `pages$snippet` | Snippet definition |
| `pages$BuildingBlock` | Building block definition |
| `enumerations$enumeration` | Enumeration definition |
| `JavaActions$JavaAction` | Java action definition |
| `security$ProjectSecurity` | Project security settings |
| `security$ModuleSecurity` | Module security settings |
| `navigation$NavigationDocument` | Navigation profile |
| `settings$ProjectSettings` | Project settings |
| `BusinessEvents$BusinessEventService` | Business event service |
| `CustomWidgets$customwidget` | Custom widget definition |

---

## BSON Structure Conventions

### Standard Fields

Every BSON document contains:

| Field | Type | Description |
|-------|------|-------------|
| `$ID` | Binary (UUID) | Unique identifier |
| `$type` | String | Fully qualified type name |

### ID Format

IDs are stored as BSON Binary subtype 0 (generic) containing UUID bytes:
```json
{
  "$ID": {
    "Subtype": 0,
    "data": "base64-encoded-uuid"
  }
}
```

UUID string format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`

### Array Format

**CRITICAL**: Arrays in Mendix BSON have the **count of elements** as the first element:
```json
{
  "Items": [
    2,           // count of items (2 items follow)
    { ... },     // First element
    { ... }      // Second element
  ]
}
```

**Important notes:**
- The count is an `int32` value representing the number of actual items that follow
- When writing arrays, you MUST include this count prefix
- When parsing arrays, skip the first element (the count) when iterating
- Missing or incorrect counts will cause Studio Pro to misinterpret the data

Example in Go:
```go
// Writing an array with count prefix
items := bson.A{int32(len(changes))} // Start with count
for _, change := range changes {
    items = append(items, serializeItem(change))
}
```

> **JSON Templates vs BSON**: This array format with count prefixes applies to **BSON serialization in Go code**. When editing JSON template files (like `sdk/widgets/templates/*.json`), use truly empty arrays `[]` - the version markers are added automatically during BSON serialization. Writing `[2]` in JSON creates an array containing the integer 2, not an empty array.

### Reference Types

The Mendix metamodel defines two types of references:

| Reference Type | Storage Format | Example Use |
|---------------|----------------|-------------|
| `BY_ID_REFERENCE` | Binary UUID | Index `AttributePointer` |
| `BY_NAME_REFERENCE` | Qualified name string | ValidationRule `attribute` |

**BY_ID_REFERENCE** - Stored as BSON Binary containing UUID bytes:
```json
{
  "AttributePointer": {
    "Subtype": 0,
    "data": "base64-uuid"
  }
}
```

**BY_NAME_REFERENCE** - Stored as qualified name string:
```json
{
  "attribute": "MyModule.MyEntity.MyAttribute"
}
```

> **Critical**: Using the wrong reference format will cause Studio Pro to fail loading the model. The metamodel reflection data specifies which format each property uses via the `kind` field in `typeInfo`.

### Type Names: qualifiedName vs storageName

The metamodel defines two type identifiers for each element type:

| Field | Usage | Example |
|-------|-------|---------|
| `qualifiedName` | TypeScript SDK API, internal naming | `DomainModels$index` |
| `storageName` | BSON `$type` field value | `DomainModels$EntityIndex` |

**Critical**: The `$type` field in BSON must use the `storageName`, not the `qualifiedName`. These are often identical, but not always:

```json
// from metamodel reflection data
"DomainModels$index" : {
  "qualifiedName" : "DomainModels$index",
  "storageName" : "DomainModels$EntityIndex",  // ← use this for $type!
  ...
}
```

Using the wrong type name causes Studio Pro to fail with:
```
TypeCacheUnknownTypeException: The type cache does not contain a type with qualified name DomainModels$index
```

**Known differences** (Mendix 11.6):

| qualifiedName | storageName (use this) |
|---------------|------------------------|
| `DomainModels$index` | `DomainModels$EntityIndex` |
| `DomainModels$entity` | `DomainModels$EntityImpl` |

When adding support for new document types, always check the metamodel reflection data in `reference/mendixmodellib/reflection-data/<version>-structures.json` to find the correct `storageName`.

### Metamodel Reference Definition

From the metamodel reflection data (`*-structures.json`):

```json
{
  "DomainModels$ValidationRule": {
    "properties": {
      "attribute": {
        "storageName": "attribute",
        "typeInfo": {
          "type": "ELEMENT",
          "elementType": "DomainModels$attribute",
          "kind": "BY_NAME_REFERENCE"
        }
      }
    }
  },
  "DomainModels$IndexedAttribute": {
    "properties": {
      "attribute": {
        "storageName": "AttributePointer",
        "typeInfo": {
          "type": "ELEMENT",
          "elementType": "DomainModels$attribute",
          "kind": "BY_ID_REFERENCE"
        }
      }
    }
  }
}
```

---

## Entity Mapping

### MDL Entity
```sql
/** Documentation text */
@position(100, 200)
create persistent entity Module.EntityName (
  AttrName: string(200) not null
)
index (AttrName);
/
```

### BSON Structure
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$EntityImpl",
  "Name": "EntityName",
  "documentation": "documentation text",
  "Location": "100;200",
  "MaybeGeneralization": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$NoGeneralization",
    "Persistable": true
  },
  "attributes": [
    3,
    { /* attribute BSON */ }
  ],
  "ValidationRules": [
    3,
    { /* validation rule BSON */ }
  ],
  "Indexes": [
    3,
    { /* index BSON */ }
  ],
  "AccessRules": [3],
  "events": [3]
}
```

### Entity Type Mapping

| MDL | BSON Persistable | BSON Source |
|-----|------------------|-------------|
| `persistent` | `true` | `null` |
| `non-persistent` | `false` | `null` |
| `view` | `false` | `OqlViewEntitySource` |
| `external` | `false` | `ODataRemoteEntitySource` |

### VIEW Entity Structure

VIEW entities require two separate documents:

1. **ViewEntitySourceDocument** - Contains the OQL query (MODEL_UNIT)
2. **Entity with OqlViewEntitySource** - References the source document

#### ViewEntitySourceDocument BSON

This is a separate document (unit) that stores the OQL query:

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$ViewEntitySourceDocument",
  "Name": "ActiveProducts",
  "documentation": "Products that are currently active",
  "Excluded": false,
  "ExportLevel": "Hidden",
  "Oql": "select p.Name as Name, p.Price as Price from Module.Product p where p.IsActive = true"
}
```

#### Entity with OqlViewEntitySource

The entity's `source` field references the ViewEntitySourceDocument by qualified name:

```json
{
  "$type": "DomainModels$EntityImpl",
  "Name": "ActiveProducts",
  "MaybeGeneralization": {
    "$type": "DomainModels$NoGeneralization",
    "Persistable": false
  },
  "source": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$OqlViewEntitySource",
    "SourceDocument": "Module.ActiveProducts"
  },
  "attributes": [...]
}
```

#### OqlViewValue for View Attributes

View entity attributes use `OqlViewValue` instead of `StoredValue`. The `reference` field contains the OQL column alias:

```json
{
  "$type": "DomainModels$attribute",
  "Name": "Name",
  "NewType": {
    "$type": "DomainModels$StringAttributeType",
    "length": 0
  },
  "value": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$OqlViewValue",
    "reference": "Name"
  }
}
```

The `reference` value must match the OQL column alias (e.g., `as Name` in the SELECT clause).

### Location Format

Position is stored as semicolon-separated string:
```
"Location": "100;200"
```

Parsed from MDL:
```sql
@position(100, 200)
```

---

## Attribute Mapping

### MDL Attribute
```sql
/** Attribute documentation */
AttrName: string(200) not null error 'Required' unique error 'Must be unique' default 'value'
```

### BSON Structure
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$attribute",
  "Name": "AttrName",
  "documentation": "attribute documentation",
  "NewType": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$StringAttributeType",
    "length": 200
  },
  "value": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$StoredValue",
    "DefaultValue": "value"
  }
}
```

### Attribute Type Mapping

| MDL Type | BSON $Type | Additional Fields |
|----------|------------|-------------------|
| `string` | `DomainModels$StringAttributeType` | `length: 200` (default) |
| `string(n)` | `DomainModels$StringAttributeType` | `length: n` |
| `integer` | `DomainModels$IntegerAttributeType` | - |
| `long` | `DomainModels$LongAttributeType` | - |
| `decimal` | `DomainModels$DecimalAttributeType` | - |
| `boolean` | `DomainModels$BooleanAttributeType` | - |
| `datetime` | `DomainModels$DateTimeAttributeType` | `LocalizeDate: false` |
| `autonumber` | `DomainModels$AutoNumberAttributeType` | - |
| `binary` | `DomainModels$BinaryAttributeType` | - |
| `enumeration(M.E)` | `DomainModels$EnumerationAttributeType` | `enumeration: "Module.EnumName"` |
| `hashedstring` | `DomainModels$HashedStringAttributeType` | - |

### Enumeration Attribute Type

The `enumeration` field in `EnumerationAttributeType` uses a **BY_NAME_REFERENCE** (qualified name string), not a binary UUID:

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$EnumerationAttributeType",
  "enumeration": "MyModule.Status"
}
```

The qualified name references the enumeration document by its `Module.EnumName` path.

### Default Value Mapping

| MDL Default | BSON Structure |
|-------------|----------------|
| `default 'text'` | `{$type: "DomainModels$StoredValue", DefaultValue: "text"}` |
| `default 123` | `{$type: "DomainModels$StoredValue", DefaultValue: "123"}` |
| `default true` | `{$type: "DomainModels$StoredValue", DefaultValue: "true"}` |
| `default false` | `{$type: "DomainModels$StoredValue", DefaultValue: "false"}` |
| `default 'EnumValue'` | `{$type: "DomainModels$StoredValue", DefaultValue: "EnumValue"}` |
| (no default) | `value` field absent or null |

---

## Validation Rule Mapping

Validation rules are stored separately from attributes in the entity's `ValidationRules` array.

### MDL Validation
```sql
AttrName: string not null error 'Field is required' unique error 'Must be unique'
```

### BSON Structure (Required Rule)

> **Important**: Field order matters. Studio Pro expects: `$ID`, `$type`, `attribute`, `message`, `RuleInfo`

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$ValidationRule",
  "attribute": "Module.Entity.AttrName",
  "message": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "Texts$text",
    "Items": [
      3,
      {
        "$ID": {"Subtype": 0, "data": "<uuid>"},
        "$type": "Texts$Translation",
        "LanguageCode": "en_US",
        "text": "Field is required"
      }
    ]
  },
  "RuleInfo": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$RequiredRuleInfo"
  }
}
```

### BSON Structure (Unique Rule)
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$ValidationRule",
  "attribute": "Module.Entity.AttrName",
  "message": { /* same structure */ },
  "RuleInfo": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$UniqueRuleInfo"
  }
}
```

### Validation Rule Type Mapping

| MDL Constraint | BSON RuleInfo.$Type |
|----------------|---------------------|
| `not null` | `DomainModels$RequiredRuleInfo` |
| `unique` | `DomainModels$UniqueRuleInfo` |
| (future) `range` | `DomainModels$RangeRuleInfo` |
| (future) `regex` | `DomainModels$RegexRuleInfo` |

### Attribute Reference (BY_NAME_REFERENCE)

The `attribute` field in ValidationRule uses **BY_NAME_REFERENCE** and MUST be a qualified name string:

```json
{
  "attribute": "Module.Entity.Attribute"
}
```

> **Critical**: Do NOT use binary UUID for this field. The metamodel specifies `"kind": "BY_NAME_REFERENCE"` for ValidationRule.attribute, which requires a qualified name string. Using binary UUID will cause `System.ArgumentNullException` in Studio Pro when it tries to resolve the attribute.

This is different from Index's `AttributePointer` which uses **BY_ID_REFERENCE** (binary UUID).

**Implementation Note**: When reading validation rules from BSON, the qualified name string is stored in the Go struct's `AttributeID` field. When re-serializing (e.g., when adding another entity to the domain model), the code must detect whether `AttributeID` contains a UUID (for newly created entities) or a qualified name string (for entities read from disk). If it contains dots, it's already a qualified name and can be used directly.

---

## Index Mapping

### MDL Index
```sql
index (AttrName1, AttrName2 desc)
```

### BSON Structure

> **Note**: Index uses `AttributePointer` with **BY_ID_REFERENCE** (binary UUID), unlike ValidationRule which uses qualified name strings.

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$EntityIndex",
  "attributes": [
    2,
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "DomainModels$IndexedAttribute",
      "AttributePointer": {"Subtype": 0, "data": "<attr1-uuid>"},
      "Ascending": true,
      "type": "Normal"
    },
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "DomainModels$IndexedAttribute",
      "AttributePointer": {"Subtype": 0, "data": "<attr2-uuid>"},
      "Ascending": false,
      "type": "Normal"
    }
  ]
}
```

### Attribute Reference (BY_ID_REFERENCE)

The `AttributePointer` field in IndexedAttribute uses **BY_ID_REFERENCE** and MUST be a binary UUID:

```json
{
  "AttributePointer": {"Subtype": 0, "data": "<attribute-uuid>"}
}
```

This is different from ValidationRule's `attribute` which uses **BY_NAME_REFERENCE** (qualified name string).

### Sort Order Mapping

| MDL | BSON Ascending |
|-----|----------------|
| `AttrName` (default) | `true` |
| `AttrName asc` | `true` |
| `AttrName desc` | `false` |

---

## Association Mapping

### MDL Association
```sql
-- Many Orders can reference one Customer (1-to-many from Customer perspective)
create association Module.Order_Customer
  from Module.Order       -- Entity holding the FK reference
  to Module.Customer      -- Entity being referenced
  type reference
  owner default           -- Creates 1-to-many cardinality
  delete_behavior DELETE_BUT_KEEP_REFERENCES;
```

### BSON Structure
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "DomainModels$association",
  "Name": "Order_Customer",
  "documentation": "",
  "ExportLevel": "Hidden",
  "GUID": {"Subtype": 0, "data": "<uuid>"},
  "ParentPointer": {"Subtype": 0, "data": "<order-entity-uuid>"},
  "ChildPointer": {"Subtype": 0, "data": "<customer-entity-uuid>"},
  "type": "reference",
  "owner": "default",
  "ParentConnection": "0;50",
  "ChildConnection": "100;50",
  "StorageFormat": "table",
  "source": null,
  "DeleteBehavior": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "DomainModels$DeleteBehavior",
    "ChildDeleteBehavior": "DeleteMeButKeepReferences",
    "ChildErrorMessage": null,
    "ParentDeleteBehavior": "DeleteMeButKeepReferences",
    "ParentErrorMessage": null
  }
}
```

### Field Mapping

| MDL | BSON Field | Description |
|-----|------------|-------------|
| `from entity` | `ParentPointer` | Entity holding the foreign key reference (BY_ID_REFERENCE) |
| `to entity` | `ChildPointer` | Entity being referenced (BY_ID_REFERENCE) |
| `delete_behavior` | `DeleteBehavior.ChildDeleteBehavior` | Behavior when child (TO) entity is deleted |

### Association Type Mapping

| MDL Type | BSON Type |
|----------|-----------|
| `reference` | `"reference"` |
| `ReferenceSet` | `"ReferenceSet"` |

### Owner Mapping and Cardinality

The `owner` setting determines relationship cardinality in Studio Pro:

| MDL Owner | BSON Owner | Cardinality | Use Case |
|-----------|------------|-------------|----------|
| `default` | `"default"` | **1-to-many** | Many Orders → One Customer |
| `both` | `"both"` | **1-to-1** | One Order ↔ One Customer |

**Important**: For many-to-one relationships, use `owner default`. Using `owner both` creates a one-to-one relationship.

```sql
-- Many-to-one (many orders to one customer)
create association Module.Order_Customer
from Module.Order to Module.Customer
type reference
owner default;  -- Creates 1-to-many: Customer has many Orders

-- One-to-one (bidirectional)
create association Module.Order_Customer
from Module.Order to Module.Customer
type reference
owner both;     -- Creates 1-to-1: Customer has one Order
```

### Delete Behavior Mapping

| MDL Behavior | BSON DeleteBehavior.Type |
|--------------|--------------------------|
| `DELETE_BUT_KEEP_REFERENCES` (and the default, when the clause is omitted) | `"DeleteMeButKeepReferences"` |
| `DELETE_AND_REFERENCES`, `CASCADE` | `"DeleteMeAndReferences"` |
| `DELETE_IF_NO_REFERENCES`, `PREVENT` | `"DeleteMeIfNoReferences"` |

---

## Enumeration Mapping

### MDL Enumeration
```sql
/** Status values */
create enumeration Module.Status (
  Active 'Active',
  Inactive 'Inactive',
  Pending 'Pending Review'
);
```

### BSON Structure
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "enumerations$enumeration",
  "Name": "status",
  "documentation": "status values",
  "values": [
    3,
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "enumerations$EnumerationValue",
      "Name": "Active",
      "caption": {
        "$type": "Texts$text",
        "Items": [
          3,
          {
            "$type": "Texts$Translation",
            "LanguageCode": "en_US",
            "text": "Active"
          }
        ]
      }
    },
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "enumerations$EnumerationValue",
      "Name": "Inactive",
      "caption": { /* ... */ }
    },
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "enumerations$EnumerationValue",
      "Name": "Pending",
      "caption": {
        "$type": "Texts$text",
        "Items": [
          3,
          {
            "$type": "Texts$Translation",
            "LanguageCode": "en_US",
            "text": "Pending Review"
          }
        ]
      }
    }
  ]
}
```

---

## Text and Localization

### Text Structure

All user-visible text uses the `Texts$text` structure with translations:

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "Texts$text",
  "Items": [
    3,
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "Texts$Translation",
      "LanguageCode": "en_US",
      "text": "English text"
    },
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "Texts$Translation",
      "LanguageCode": "nl_NL",
      "text": "Dutch text"
    }
  ]
}
```

### Language Codes

Common language codes:
- `en_US` - English (United States)
- `en_GB` - English (United Kingdom)
- `nl_NL` - Dutch (Netherlands)
- `de_DE` - German (Germany)
- `fr_FR` - French (France)

### MDL Text Handling

Currently MDL uses the first available translation or `en_US` if available:

```sql
-- Error message uses en_US translation
AttrName: string not null error 'This field is required'
```

Multi-language support is planned for future versions.

---

## ID Generation

When creating new elements, UUIDs must be generated for all `$ID` fields.

### UUID Format

Version 4 (random) UUIDs in standard format:
```
xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
```

Where:
- `x` is any hexadecimal digit
- `4` indicates version 4
- `y` is one of: 8, 9, a, b

### Example

```go
// Go implementation
import "github.com/google/uuid"

id := uuid.New().String()
// "f47ac10b-58cc-4372-a567-0e02b2c3d479"
```

---

## Complete Example

### MDL Input
```sql
/** Customer entity */
@position(100, 200)
create persistent entity Sales.Customer (
  /** Unique identifier */
  CustomerId: autonumber not null unique default 1,
  /** Customer name */
  Name: string(200) not null error 'Name is required',
  Email: string(200) unique error 'Email must be unique'
)
index (Name);
/
```

### BSON Output
```json
{
  "$ID": {"Subtype": 0, "data": "..."},
  "$type": "DomainModels$EntityImpl",
  "Name": "Customer",
  "documentation": "Customer entity",
  "Location": "100;200",
  "MaybeGeneralization": {
    "$type": "DomainModels$NoGeneralization",
    "Persistable": true
  },
  "attributes": [
    3,
    {
      "$type": "DomainModels$attribute",
      "Name": "CustomerId",
      "documentation": "unique identifier",
      "NewType": {"$type": "DomainModels$AutoNumberAttributeType"},
      "value": {"$type": "DomainModels$StoredValue", "DefaultValue": "1"}
    },
    {
      "$type": "DomainModels$attribute",
      "Name": "Name",
      "documentation": "Customer name",
      "NewType": {"$type": "DomainModels$StringAttributeType", "length": 200}
    },
    {
      "$type": "DomainModels$attribute",
      "Name": "Email",
      "NewType": {"$type": "DomainModels$StringAttributeType", "length": 200}
    }
  ],
  "ValidationRules": [
    3,
    {
      "$ID": {"Subtype": 0, "data": "..."},
      "$type": "DomainModels$ValidationRule",
      "attribute": "Sales.Customer.CustomerId",
      "RuleInfo": {"$ID": {...}, "$type": "DomainModels$RequiredRuleInfo"}
    },
    {
      "$ID": {"Subtype": 0, "data": "..."},
      "$type": "DomainModels$ValidationRule",
      "attribute": "Sales.Customer.CustomerId",
      "RuleInfo": {"$ID": {...}, "$type": "DomainModels$UniqueRuleInfo"}
    },
    {
      "$ID": {"Subtype": 0, "data": "..."},
      "$type": "DomainModels$ValidationRule",
      "attribute": "Sales.Customer.Name",
      "message": {"$ID": {...}, "$type": "Texts$text", "Items": [3, {...}]},
      "RuleInfo": {"$ID": {...}, "$type": "DomainModels$RequiredRuleInfo"}
    },
    {
      "$ID": {"Subtype": 0, "data": "..."},
      "$type": "DomainModels$ValidationRule",
      "attribute": "Sales.Customer.Email",
      "message": {"$ID": {...}, "$type": "Texts$text", "Items": [3, {...}]},
      "RuleInfo": {"$ID": {...}, "$type": "DomainModels$UniqueRuleInfo"}
    }
  ],
  "Indexes": [
    3,
    {
      "$type": "DomainModels$EntityIndex",
      "attributes": [
        2,
        {"AttributePointer": "<name-attr-id>", "Ascending": true}
      ]
    }
  ],
  "AccessRules": [3],
  "events": [3]
}
```

---

## Implementing New Document Types

When adding support for new Mendix metamodel types (microflows, pages, workflows, etc.), follow this checklist:

### 1. Find the Metamodel Definition

Locate the type in `reference/mendixmodellib/reflection-data/<version>-structures.json`:

```bash
# search for a type
grep -A 20 '"Microflows\$Microflow"' reference/mendixmodellib/reflection-data/11.6.0-structures.json
```

### 2. Check storageName vs qualifiedName

**Always use `storageName` for the `$type` field**:

```json
"microflows$microflow" : {
  "qualifiedName" : "microflows$microflow",
  "storageName" : "microflows$microflow",  // ← use this
  ...
}
```

### 3. Check Property Reference Types

For each property that references another element, check the `kind` field:

```json
"properties": {
  "objectCollection": {
    "storageName": "ObjectCollection",
    "typeInfo": {
      "type": "ELEMENT",
      "elementType": "microflows$MicroflowObjectCollection",
      "kind": "PART"  // Embedded object, serialize inline
    }
  },
  "returnType": {
    "storageName": "MicroflowReturnType",
    "typeInfo": {
      "type": "ELEMENT",
      "elementType": "DataTypes$DataType",
      "kind": "PART"
    }
  }
}
```

Reference kind mappings:

| Kind | Storage Format | Description |
|------|----------------|-------------|
| `PART` | Embedded BSON object | Child object serialized inline |
| `BY_ID_REFERENCE` | Binary UUID | Reference by ID (BSON Binary) |
| `BY_NAME_REFERENCE` | Qualified name string | Reference by name (e.g., "Module.Entity") |
| `LOOKUP` | Usually string | Named lookup in parent context |

### 4. Check Array Prefixes

Arrays have a type prefix as the first element. Common values:

| Prefix | Meaning |
|--------|---------|
| `2` | List/array type |
| `3` | Another array type (most common) |

Check existing BSON files to determine the correct prefix for each array property.

### 5. Check Default Values

The `defaultSettings` in the metamodel shows what fields can be omitted:

```json
"defaultSettings": {
  "documentation": "",
  "allowedModuleRoles": [],
  "markAsUsed": false
}
```

Fields matching their default value can often be omitted from BSON.

### 6. Test with Studio Pro

After implementing serialization:

1. Create a test MPR with Studio Pro
2. Use the SDK to modify it
3. Reopen in Studio Pro to verify no errors
4. Compare BSON output with original using debug tools

### Common Pitfalls

1. **Wrong $Type**: Using `qualifiedName` instead of `storageName`
2. **Wrong reference format**: Using UUID for BY_NAME_REFERENCE or vice versa
3. **Missing array prefix**: Forgetting the integer prefix in arrays
4. **Field ordering**: Some types require specific field order (use `bson.D` not `bson.M`)
5. **Missing $ID**: Every element needs a unique UUID in its `$ID` field

---

## Page Widget Mapping

### LayoutGridColumn

Layout grid columns have weight properties for responsive design:

| Field | Description | Values |
|-------|-------------|--------|
| `Weight` | Desktop column width | 1-12 for explicit, -1 for auto-fill |
| `PhoneWeight` | Phone column width | Usually -1 (auto) |
| `TabletWeight` | Tablet column width | Usually -1 (auto) |

**BSON Structure:**
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "Forms$LayoutGridColumn",
  "Appearance": {...},
  "PhoneWeight": -1,
  "TabletWeight": -1,
  "Weight": 6,
  "widgets": [3, ...]
}
```

> **Critical**: Use `Weight` (not `DesktopWeight`) for desktop column width. Using the wrong field name causes columns to display "Manual - 1" in Studio Pro.

### ActionButton with CaptionTemplate

ActionButton widgets use `CaptionTemplate` (not `caption`) for parameterized button text with placeholders like `{1}`.

**BSON Structure:**
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "Forms$actionbutton",
  "action": {...},
  "Appearance": {...},
  "buttonstyle": "primary",
  "CaptionTemplate": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "Forms$ClientTemplate",
    "Fallback": {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "Texts$text",
      "Items": [3]
    },
    "parameters": [
      2,
      {
        "$ID": {"Subtype": 0, "data": "<uuid>"},
        "$type": "Forms$ClientTemplateParameter",
        "AttributeRef": null,
        "expression": "'Hello'",
        "FormattingInfo": {...},
        "SourceVariable": null
      }
    ],
    "template": {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "Texts$text",
      "Items": [
        3,
        {
          "$ID": {"Subtype": 0, "data": "<uuid>"},
          "$type": "Texts$Translation",
          "LanguageCode": "en_US",
          "text": "Save {1}"
        }
      ]
    }
  },
  "Name": "btnSave1",
  "rendermode": "button"
}
```

> **Critical Field Names and Structures:**
> 1. **`CaptionTemplate`** - Must be `CaptionTemplate`, NOT `caption`. Using `caption` causes the button text to not display in Studio Pro.
> 2. **`Fallback`** - Must be a `Texts$text` object, NOT a string field like `FallbackValue`. Using `FallbackValue: ""` causes the template to fail.
> 3. **Array version markers differ by context:**
>    - `parameters`: Use `[2, items...]` for non-empty, `[3]` for empty
>    - `Template.Items`: Use `[3, items...]` for non-empty, `[3]` for empty
>
> These differences were discovered by comparing SDK-generated BSON with Studio Pro-generated BSON. When in doubt, create a reference structure in Studio Pro and compare.

### ClientTemplate (DynamicText)

DynamicText widgets use `ClientTemplate` for parameterized content with placeholders like `{1}`.

**BSON Structure:**
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "Forms$dynamictext",
  "content": {
    "$ID": {"Subtype": 0, "data": "<uuid>"},
    "$type": "Forms$ClientTemplate",
    "Fallback": {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "Texts$text",
      "Items": [3]
    },
    "parameters": [
      2,
      {
        "$ID": {"Subtype": 0, "data": "<uuid>"},
        "$type": "Forms$ClientTemplateParameter",
        "expression": "'Hello World'"
      }
    ],
    "template": {
      "$type": "Texts$text",
      "Items": [3, {"LanguageCode": "en_US", "text": "{1}"}]
    }
  }
}
```

### ClientTemplateParameter Expression Format

The `expression` field must contain a valid Mendix expression:

| Value Type | Expression Format | Example |
|------------|-------------------|---------|
| String literal | Single-quoted | `'Hello World'` |
| Variable | Dollar prefix | `$parameter/Name` |
| Number | Unquoted | `42` |
| Boolean | Unquoted | `true` or `false` |
| Attribute path | Dollar + path | `$currentObject/Name` |

> **Critical**: String literals MUST be wrapped in single quotes. Using bare strings like `Hello` causes CE0117 "Error(s) in expression" errors in Studio Pro.

---

## Microflow Action Mapping

### CreateChangeAction (CREATE Object)

The CreateObjectAction uses `microflows$CreateChangeAction` as its storageName.

**BSON Structure:**
```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "microflows$CreateChangeAction",
  "commit": "No",
  "entity": "Module.EntityName",
  "ErrorHandlingType": "rollback",
  "Items": [
    4,  // count of items!
    {
      "$ID": {"Subtype": 0, "data": "<uuid>"},
      "$type": "microflows$ChangeActionItem",
      "association": "",
      "attribute": "Module.Entity.AttributeName",
      "type": "set",
      "value": "$ParameterName"
    },
    // ... more items
  ],
  "RefreshInClient": false,
  "VariableName": "NewProduct"
}
```

**Required fields for CreateChangeAction:**
| Field | Description | Required |
|-------|-------------|----------|
| `commit` | "No", "Yes", or "YesWithoutEvents" | Yes |
| `entity` | Qualified entity name (BY_NAME_REFERENCE) | Yes |
| `ErrorHandlingType` | "Rollback" or "Abort" | Yes |
| `Items` | Array of ChangeActionItem with count prefix | Yes |
| `RefreshInClient` | Boolean | Yes |
| `VariableName` | Output variable name (without $) | Yes |

### ChangeActionItem

Each item in the `Items` array represents an attribute assignment.

**Required fields:**
| Field | Description | Required |
|-------|-------------|----------|
| `$ID` | Unique UUID | Yes |
| `$type` | `"microflows$ChangeActionItem"` | Yes |
| `association` | Empty string for attribute changes | Yes |
| `attribute` | Qualified attribute name (BY_NAME_REFERENCE) | For attribute changes |
| `type` | "Set", "Add", or "Remove" | Yes |
| `value` | Expression string | Yes |

> **Critical**: The `association` field MUST be present even when empty. Missing this field causes Studio Pro to fail silently, showing fewer items than expected.

### ChangeAction (CHANGE Object)

Similar to CreateChangeAction but uses `microflows$ChangeAction`:

```json
{
  "$ID": {"Subtype": 0, "data": "<uuid>"},
  "$type": "microflows$ChangeAction",
  "ChangeVariableName": "Product",
  "commit": "No",
  "Items": [
    2,  // count of items
    { /* ChangeActionItem */ },
    { /* ChangeActionItem */ }
  ],
  "RefreshInClient": false
}
```

---

## Nanoflow Mapping

Nanoflows share the `Microflows` BSON domain — all action types, parameters, and flow structures are identical to microflows.

### BSON Type

| Unit Type | BSON `$Type` | Go Struct |
|-----------|-------------|-----------|
| Nanoflow | `Microflows$Nanoflow` | `microflows.Nanoflow` |

### Key Differences from Microflow

| Field | Microflow | Nanoflow |
|-------|-----------|----------|
| `$Type` | `Microflows$Microflow` | `Microflows$Nanoflow` |
| `AllowConcurrentExecution` | Present | Absent |
| `ConcurrencyErrorMicroflow` | Present | Absent |
| `ConcurrenyErrorMessage` | Present | Absent |
| `ApplyEntityAccess` | Present | Absent |

All other fields — `ObjectCollection`, `Parameters`, `ReturnType`, `AllowedModuleRoles`, `Documentation`, `Excluded`, `MarkAsUsed` — are identical.

### Allowed Actions

Nanoflows run client-side. The following action types are **disallowed** in nanoflows:

- `Microflows$DownloadFileAction` — requires server response
- `Microflows$ImportMappingCallAction` — requires server context
- `Microflows$ExportMappingCallAction` — requires server context
- `Microflows$RestCallAction` — use JavaScript actions for HTTP from client
- `Microflows$WebServiceCallAction` — server-side only

The following are **nanoflow-specific** (allowed only in nanoflows):

- `Microflows$SynchronizeAction` — triggers offline data sync

### JavaScript Action Calls

JavaScript actions use `Microflows$JavaScriptActionCallAction` (not `JavaActionCallAction`):

| Field | JavaAction | JavaScriptAction |
|-------|-----------|-----------------|
| `$Type` | `Microflows$JavaActionCallAction` | `Microflows$JavaScriptActionCallAction` |
| Action reference | `JavaAction` | `JavaScriptAction` |
| Result variable | `ResultVariableName` | `OutputVariableName` |
| Parameter mapping type | `Microflows$JavaActionParameterMapping` | `Microflows$JavaScriptActionParameterMapping` |
| Parameter value key | `Value` | `ParameterValue` |

---

## Debugging BSON Issues

When Studio Pro doesn't display data correctly (e.g., missing attributes, incorrect values), follow this debugging approach:

### 1. Create a Reference Copy

1. Create the expected structure manually in Studio Pro
2. Save and close Studio Pro
3. Use the SDK to extract and examine the BSON

### 2. Compare BSON Structures

Use this pattern to compare your generated BSON with Mendix-generated BSON:

```go
// in sdk/mpr/reader_units.go there's GetRawMicroflowByName for debugging
raw1, _ := reader.GetRawMicroflowByName("Module.BrokenMicroflow")
raw2, _ := reader.GetRawMicroflowByName("Module.WorkingMicroflow")

// Unmarshal and compare
var map1, map2 map[string]interface{}
bson.Unmarshal(raw1, &map1)
bson.Unmarshal(raw2, &map2)
// Compare field by field
```

### 3. Common BSON Issues

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| Items missing in Studio Pro | Missing array count prefix | Add `int32(count)` as first element |
| First item missing | Count is 0 or missing | Verify count matches actual items |
| Fields showing default values | Missing required fields | Check metamodel for required fields |
| "Unknown type" error | Wrong $Type value | Use `storageName`, not `qualifiedName` |
| Silent failures | Missing optional-but-expected fields | Compare with Mendix-generated BSON |
| "(Empty caption)" on buttons | Wrong field name or structure | Use `CaptionTemplate`, not `caption` |
| Template not displaying | Wrong Fallback structure | Use `Fallback: {Texts$text}`, not `FallbackValue: ""` |

### 4. Key Patterns Discovered

1. **Array version markers vary by type**:
   - `parameters` arrays use `[2, items...]` for non-empty
   - `Texts$Text.Items` arrays use `[3, items...]` for non-empty
   - Empty arrays typically use `[3]` alone
2. **Field names may differ from SDK types**: e.g., SDK uses `caption` but BSON needs `CaptionTemplate`
3. **Object vs string fields**: e.g., `Fallback` must be a `Texts$text` object, not a string
4. **Empty string vs null**: Some fields require empty string `""`, not null/omitted
5. **Required "optional" fields**: Some fields marked optional in metamodel are required by Studio Pro
6. **Field order**: Some elements require specific field ordering (use `bson.D`, not `bson.M`)

### 5. Debugging Checklist

- [ ] Array has count prefix as first element
- [ ] Count matches actual number of items
- [ ] All required fields are present (check metamodel)
- [ ] $Type uses `storageName` from reflection data
- [ ] References use correct format (BY_ID vs BY_NAME)
- [ ] Empty strings used where null might cause issues
- [ ] Field order matches Mendix-generated examples
