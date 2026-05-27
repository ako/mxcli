---
title: Page Variables Support
status: done
---

# Proposal: Page Variables Support

## Summary

Add support for **page variables** (`Forms$LocalVariable`) — local variables defined at the page level that can be referenced in expressions throughout the page. This enables column visibility expressions, conditional formatting, and other dynamic behavior without requiring `$currentObject`.

## Background

### What are page variables?

Page variables are a Mendix feature (available in Mendix 10+) that allows declaring typed variables at the page level with default value expressions. They are stored in the page's `variables` array in BSON.

### BSON Structure

```json
{
  "variables": [
    3,
    {
      "$ID": "...",
      "$type": "Forms$LocalVariable",
      "DefaultValue": "if ( 3 < 4 ) then true else false",
      "Name": "showStockColumn",
      "VariableType": {
        "$ID": "...",
        "$type": "DataTypes$BooleanType"
      }
    }
  ]
}
```

### Why needed?

- **Column Visible**: DataGrid2 column `visible` property hides/shows the *entire column* for all rows. Using `$currentObject/attr` here doesn't make sense — it needs a page-level boolean. Studio Pro shows an error if `$currentObject` is used.
- **Conditional logic**: Page variables allow toggling sections, panels, or modes based on user interaction or computed state.
- **Expression syntax**: Mendix expressions use `if (...) then ... else ...` syntax, NOT `if(..., ..., ...)` function-call style.

### Current state (broken)

The MDL example `P033b_DataGrid_ColumnProperties` had two issues:
1. `visible: '$currentObject/IsActive'` — invalid, columns need page variables for visibility
2. `DynamicCellClass: 'if($currentObject/Stock < 10, ''text-danger'', '''')'` — wrong if-syntax

## Proposed MDL Syntax

### Page variable declaration

Add a `variables` block to page/snippet headers:

```sql
create page MyModule.ProductOverview (
  title: 'Products',
  layout: Atlas_Core.Atlas_Default,
  variables: {
    $showStockColumn: boolean = 'if (3 < 4) then true else false',
    $filterMode: string = '''active'''
  }
) {
  datagrid dgProducts (datasource: database MyModule.Product) {
    column colName (attribute: Name, caption: 'Name')
    column colStock (
      attribute: Stock, caption: 'Stock',
      visible: '$showStockColumn'
    )
  }
}
```

### Syntax details

```
variables: {
  $varName: DataType = 'defaultValueExpression',
  ...
}
```

- **DataType**: `boolean`, `string`, `integer`, `decimal`, `datetime`, or entity type
- **Default value**: Mendix expression string (required, in single quotes)
- Variable names prefixed with `$` (consistent with parameters)

### DESCRIBE output

```sql
create page MyModule.ProductOverview (
  title: 'Products',
  layout: Atlas_Core.Atlas_Default,
  variables: { $showStockColumn: boolean = 'true' }
) { ... }
```

## Implementation Plan

### Phase 1: DESCRIBE support (read-only)

1. **Read Variables from BSON** in `cmd_pages_describe.go`:
   - Extract `variables` array from raw page data
   - Parse each `Forms$LocalVariable`: Name, VariableType, DefaultValue
   - Resolve `VariableType.$type` to MDL type name (e.g., `DataTypes$BooleanType` → `boolean`)

2. **Output in page header**: Add `variables: { ... }` to props list in `describePage()`

3. **Same for snippets**: Snippets also support local variables

**Files**: `cmd_pages_describe.go`

### Phase 2: CREATE support (write)

1. **Grammar**: Add `variablesBlock` rule to `MDLParser.g4`:
   ```antlr
   pageProperty
       : ...
       | variables COLON LBRACE variableDecl (COMMA variableDecl)* RBRACE
       ;

   variableDecl
       : DOLLAR_IDENT COLON dataType ASSIGN STRING_LITERAL
       ;
   ```

2. **AST**: Add `variables` field to page AST node (list of `{Name, type, DefaultValue}`)

3. **Visitor**: Build variable list from parse tree

4. **Builder**: In page creation, serialize `variables` array with `Forms$LocalVariable` entries:
   - Generate `$ID`
   - Set `$type` to `Forms$LocalVariable`
   - Set `Name`, `DefaultValue`, and `VariableType` (DataType BSON)

**Files**: `MDLParser.g4`, `ast/ast_page_v3.go`, `visitor/visitor_page_v3.go`, `cmd_pages_builder.go`

### Phase 3: ALTER PAGE support

Add ability to add/modify/remove page variables via ALTER PAGE:

```sql
alter page MyModule.ProductOverview
  add VARIABLE $showDetails: boolean = 'true';

alter page MyModule.ProductOverview
  drop VARIABLE $showDetails;
```

**Files**: `MDLParser.g4`, `ast/ast_alter.go`, `executor/cmd_alter_page.go`

## Effort Estimate

| Phase | Scope | Complexity |
|-------|-------|------------|
| Phase 1 | DESCRIBE read | Low — parse BSON Variables array, format output |
| Phase 2 | CREATE write | Medium — grammar, AST, visitor, BSON serialization |
| Phase 3 | ALTER PAGE | Medium — add/drop variable operations |

## Risks

- **DataType mapping**: Need to map all `DataTypes$*` BSON types to MDL type names. Most are straightforward but entity types need qualified name resolution.
- **Expression validation**: Page variable default expressions should be valid Mendix expressions. We can't fully validate these but can pass them through as strings.
- **Version compatibility**: Page variables may not exist in older Mendix versions. Need to check if `variables` array is always present or version-dependent.

## Related fixes in this changeset

- Removed `visible: '$currentObject/...'` from DataGrid column example (columns hide entire column, not per-row)
- Fixed `DynamicCellClass` expression syntax: `if(...) then ... else ...` (not function-call style)
- Updated docs and skills with correct expression syntax
