# CREATE JSON STRUCTURE

## Synopsis

```sql
CREATE [ OR MODIFY ] JSON STRUCTURE module.Name
    [ FOLDER 'folder/path' ]
    [ COMMENT 'description' ]
    SNIPPET 'json_sample'
    [ CUSTOM_NAME_MAP ( 'jsonKey' AS 'AttributeName' [, ...] ) ]
```

## Description

Creates a JSON structure document from a representative JSON sample. The sample is stored verbatim and Mendix uses its shape to define attribute types and nesting when you create import or export mappings against it.

The structure represents one concrete JSON document shape. Arrays within the sample define the repeating item structure. Nested objects define sub-entity structures.

If `OR MODIFY` is specified and the JSON structure already exists, it is updated in-place preserving its UUID so that import/export mappings that reference it remain valid. `OR REPLACE` is also accepted as a synonym.

The optional `CUSTOM_NAME_MAP` clause overrides the attribute names generated from JSON keys. This is useful when JSON keys contain characters that are not valid Mendix attribute names, or when you want a more descriptive name.

## Parameters

`module.Name`
:   The qualified name of the JSON structure.

`FOLDER 'folder/path'`
:   Optional. Places the document in the specified Studio Pro folder (forward-slash separated), creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`COMMENT 'description'`
:   Optional. A description for the JSON structure document.

`SNIPPET 'json_sample'`
:   A representative JSON document. Must be a valid JSON string. The sample defines field names and types. Multi-line snippets can use `$$...$$` quoting.

`CUSTOM_NAME_MAP ( 'jsonKey' AS 'AttributeName' )`
:   Optional. Renames individual JSON keys to the specified attribute names. Useful for keys that are reserved words or contain special characters.

## Examples

### Flat JSON object

```sql
CREATE JSON STRUCTURE MyModule.JSON_Pet
    SNIPPET '{"id": 1, "name": "Fido", "status": "available"}';
```

### Nested JSON with array

```sql
CREATE JSON STRUCTURE MyModule.JSON_Order
    SNIPPET '{
        "orderId": 100,
        "customer": {"name": "Alice", "email": "alice@example.com"},
        "items": [{"sku": "A1", "quantity": 2, "price": 9.99}]
    }';
```

### With custom name mapping

```sql
CREATE JSON STRUCTURE MyModule.JSON_WeatherResponse
    SNIPPET '{"current_temperature": 12.8, "wind_speed_10m": 18.3}'
    CUSTOM_NAME_MAP (
        'current_temperature' AS 'Temperature',
        'wind_speed_10m'      AS 'WindSpeed'
    );
```

### Naming an array's item element

An array's item is the anonymous `[…]` entry, so it has no JSON key and the plain
form cannot reach it. `ITEM OF` addresses it by the array's key:

```sql
CREATE JSON STRUCTURE MyModule.JSON_Invoice
    SNIPPET '{"lines": [{"sku": "A1"}], "tags": ["urgent"]}'
    CUSTOM NAME MAP (
        'lines'         AS 'OrderLines',
        ITEM OF 'lines' AS 'OrderLine',
        ITEM OF 'tags'  AS 'Tag'
    );
```

The two clauses are independent: naming an item does not require renaming its
array. `ITEM OF` also names a primitive array's wrapper element, which is that
array's item.

A **root-level** array has no key either — `ITEM OF 'Root'` names its item:

```sql
CREATE JSON STRUCTURE MyModule.JSON_Feed
    SNIPPET '[{"id": 1}]'
    CUSTOM NAME MAP (ITEM OF 'Root' AS 'Entry');
```

Left unnamed, an item keeps its generated name (`<Array>Item`, the singular for a
primitive array's wrapper, `JsonObject` at the root).

An entry naming a key the snippet does not contain is an error (`MDL-JSON01`),
as is `ITEM OF` on a key that is not an array (`MDL-JSON02`) — previously such an
entry applied to nothing and reported nothing.

### Idempotent replacement

```sql
CREATE OR REPLACE JSON STRUCTURE MyModule.JSON_Pet
    SNIPPET '{"id": 1, "name": "Fido", "status": "available", "tags": []}';
```

## See Also

[CREATE IMPORT MAPPING](create-import-mapping.md), [CREATE EXPORT MAPPING](create-export-mapping.md)
