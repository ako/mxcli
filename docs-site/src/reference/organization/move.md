# MOVE

## Synopsis

    MOVE document_type qualified_name TO module_name
    MOVE document_type qualified_name TO FOLDER 'folder_path'
    MOVE document_type qualified_name TO FOLDER 'folder_path' IN module_name

## Description

Moves a document to a different folder or module. Every top-level document type is supported, spelled the way `DESCRIBE` spells it. When moving to a folder, missing intermediate folders are created automatically. Cross-module moves change the qualified name of the document, which may break by-name references elsewhere in the project.

Entity moves only support moving to a module (not to a folder), because entities are embedded in domain model documents.

A `FOLDER` clause on `CREATE OR MODIFY` moves an existing document as well, so a
script that already creates a document does not need a separate `MOVE` to place
it.

## Parameters

**document_type**
: The type of document to move:

| Group | Types |
|-------|-------|
| Pages | `PAGE`, `SNIPPET`, `BUILDING BLOCK`, `LAYOUT`, `MENU` |
| Logic | `MICROFLOW`, `NANOFLOW`, `WORKFLOW`, `QUEUE`, `SCHEDULED EVENT` |
| Domain | `ENUMERATION`, `CONSTANT`, `REGULAR EXPRESSION`, `ENTITY` |
| Mappings | `JSON STRUCTURE`, `IMPORT MAPPING`, `EXPORT MAPPING` |
| Code | `JAVA ACTION`, `JAVASCRIPT ACTION`, `DATABASE CONNECTION`, `DATA TRANSFORMER` |
| Resources | `IMAGE COLLECTION`, `ICON COLLECTION` |
| Integration | `REST CLIENT`, `PUBLISHED REST SERVICE`, `ODATA CLIENT`, `ODATA SERVICE`, `BUSINESS EVENT SERVICE` |
| AI | `MODEL`, `AGENT`, `KNOWLEDGE BASE`, `CONSUMED MCP SERVICE` |

`FOLDER` moves a folder rather than a document — see the example below.

If the named document turns out to be a different type, the statement is refused
and the error names what it actually is.

**qualified_name**
: The current `Module.Name` of the document to move.

**module_name**
: The target module. When specified without `FOLDER`, moves the document to the module root.

**folder_path**
: The target folder path within the module. Use `/` for nested paths (e.g., `'Orders/Processing'`). Enclosed in single quotes.

## Examples

### Move a page to a folder in the same module

```sql
MOVE PAGE MyModule.CustomerEdit TO FOLDER 'Customers';
```

### Move a microflow to a nested folder

```sql
MOVE MICROFLOW MyModule.ACT_ProcessOrder TO FOLDER 'Orders/Processing';
```

### Move a snippet to a different module

```sql
MOVE SNIPPET OldModule.NavigationMenu TO Common;
```

### Move an entity to a different module

```sql
MOVE ENTITY OldModule.Customer TO NewModule;
```

### Move a page to a folder in a different module

```sql
MOVE PAGE OldModule.CustomerPage TO FOLDER 'Screens' IN NewModule;
```

### Move a mapping or JSON structure

```sql
MOVE IMPORT MAPPING MyModule.IMM_Order TO FOLDER 'Private/Import mappings';
MOVE EXPORT MAPPING MyModule.EXM_Order TO FOLDER 'Private/Export mappings';
MOVE JSON STRUCTURE MyModule.JSON_Order TO FOLDER 'Private/JSON structures';
```

### Place a document while creating it

Every document type takes a `FOLDER` clause on `CREATE`, so a document can be
placed by the statement that creates it. On pages and snippets it is a property
(`Folder: 'path'`); on microflows and nanoflows a keyword before `BEGIN`; on
everything else a keyword straight after the qualified name:

```sql
CREATE OR MODIFY JSON STRUCTURE MyModule.JSON_Order
  FOLDER 'Private/JSON structures'
  SNIPPET '{"id": 1}';

CREATE QUEUE MyModule.Q_Orders FOLDER 'Private/Queues' ( Parallelism: 3 );

CREATE JAVA ACTION MyModule.JA_Sync FOLDER 'Private/Java' ()
  RETURNS String AS $$return null;$$;
```

The clause applies to an existing document too, so re-running a script that
gained one files the document rather than leaving it where it was. Omitting the
clause leaves the document where it is; it never returns it to the module root.

`DESCRIBE` emits the clause, so a description replays into the same folder.

### Move a folder

```sql
MOVE FOLDER MyModule.OldName TO FOLDER 'Archive';
```

### Check impact before a cross-module move

```sql
SHOW IMPACT OF OldModule.Customer;
MOVE ENTITY OldModule.Customer TO NewModule;
```

## See Also

[CREATE MODULE](create-module.md), [CREATE FOLDER](create-folder.md)
