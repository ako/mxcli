---
title: Page Syntax V2 - Implementation Reference
status: superseded
---

# Page Syntax V2 - Implementation Reference

## Status: Superseded by V3 ⚠️

> **Note:** V3 syntax is now the recommended approach. See `proposal_pages_v3.md`.
> V2 syntax is still supported for backward compatibility.

This document describes the Page Syntax V2 that has been implemented in MDL.

## Overview

Page Syntax V2 provides a more consistent, readable, and flexible syntax for creating pages and widgets in MDL:

- **`{ }` blocks** instead of `begin/end` for cleaner nesting
- **`->` binding operator** for attributes, variables, actions, and datasources
- **`(Name: value)` property syntax** matching Studio Pro property names
- **Consistent widget pattern**: `widget [id] ['label'] [-> binding] [(properties)] [{ children }]`

## Syntax Pattern

```
widgettype [id] ['label'] [-> binding] [(properties)] [{ children }]
```

| Part | Required | Description |
|------|----------|-------------|
| `widgettype` | Yes | Widget type keyword (TEXTBOX, DATAVIEW, etc.) |
| `id` | No | Widget identifier for referencing |
| `'label'` | No | Display label (positional string) |
| `-> binding` | No | Binding target (attribute, variable, action, datasource) |
| `(properties)` | No | Additional properties in parentheses |
| `{ children }` | No | Nested widgets in braces |

## Binding Operator `->`

The `->` operator provides a clear, unified way to express bindings:

```mdl
-- Attribute binding (form widgets)
textbox 'Name' -> Name
checkbox 'Active' -> IsActive

-- Variable binding (containers)
dataview dvProduct -> $Product { ... }

-- Action binding (buttons)
actionbutton 'Save' -> save_changes
actionbutton 'Process' -> microflow MyModule.ProcessOrder

-- Database source with query
datagrid -> database MyModule.Product
  where [IsActive = true]
  ORDER by Name asc

-- Selection binding (master-detail)
dataview -> selection galleryName
```

## Complete Example

```mdl
create page MyModule.ProductDetail (
  $Product: MyModule.Product
)
  title 'Product Details'
  layout Atlas_Core.Atlas_Default
{
  layoutgrid {
    row {
      column (desktopwidth: 12) {
        dynamictext 'Product: {1}' with ({1} = $Product/Name) (rendermode: H3)
      }
    }

    row {
      column (desktopwidth: 6) {
        dataview dvProduct -> $Product {
          textbox 'Name' -> Name
          textbox 'Code' -> Code
          textarea 'Description' -> description
          datepicker 'Created' -> CreatedDate
          checkbox 'Active' -> IsActive
          dropdown 'Status' -> status

          footer {
            actionbutton 'Save' -> save_changes (buttonstyle: primary)
            actionbutton 'Cancel' -> close_page
            actionbutton 'Process' -> microflow MyModule.ACT_ProcessProduct (
              Product: $Product
            ) (buttonstyle: success)
          }
        }
      }

      column (desktopwidth: 6) {
        datagrid dgRelated -> database MyModule.RelatedItem
          where [MyModule.RelatedItem_Product = $Product]
          ORDER by Name asc
        {
          controlbar: {
            actionbutton 'New' -> create_object MyModule.RelatedItem
              then show_page MyModule.RelatedItem_Edit
              (buttonstyle: primary)
          }
          columns: {
            column 'Item Name' -> Name
            column 'Category' -> Category
            column 'Price' -> Price
          }
        }
      }
    }
  }
}
```

## Property Syntax

Properties use `Name: value` syntax with colons:

```mdl
(
  PropertyName: value,
  PropertyName: 'string value',
  PropertyName: 123,
  PropertyName: true
)
```

### Body Properties

Complex widgets can have property groups in their body:

```mdl
datagrid dgProducts -> database MyModule.Product {
  controlbar: {
    actionbutton 'New' -> create_object MyModule.Product
      then show_page MyModule.Product_Edit
      (buttonstyle: primary)
  }
  columns: {
    column 'Name' -> Name
    column 'Price' -> Price
    column 'Actions' {
      actionbutton 'Edit' -> show_page MyModule.Product_Edit
        (Product: $currentObject) (buttonstyle: default)
    }
  }
}
```

## Implemented Binding Types

| Binding | Syntax | Use Case |
|---------|--------|----------|
| Attribute | `-> Name` | Form inputs bound to entity attribute |
| Variable | `-> $Product` | DataView/Container bound to page parameter |
| Save | `-> save_changes [close_page]` | Save button action |
| Cancel | `-> cancel_changes [close_page]` | Cancel button action |
| Close | `-> close_page` | Close page action |
| Delete | `-> delete` | Delete object action |
| Show Page | `-> show_page Module.Page (params)` | Navigate to page |
| Microflow | `-> microflow Module.MF (params)` | Call microflow |
| Create+Show | `-> create_object entity then show_page page` | Create and navigate |
| Database | `-> database entity where [...] ORDER by` | Grid/Gallery data source |
| Selection | `-> selection widgetName` | Master-detail binding |

## Implemented Widgets

### Container Widgets
- `layoutgrid` with `row` and `column`
- `container`
- `navigationlist` with `item`
- `footer`

### Data Widgets
- `dataview` - Single object form
- `datagrid` - Data table (DataGrid2 widget)
- `gallery` - Card gallery with selection
- `listview` - Simple list

### Input Widgets
- `textbox` - Single-line text input
- `textarea` - Multi-line text input
- `checkbox` - Boolean checkbox
- `radiobuttons` - Radio button group
- `datepicker` - Date/time picker
- `dropdown` - Dropdown (deprecated, use COMBOBOX)
- `combobox` - Combo box (pluggable widget)

### Display Widgets
- `dynamictext` - Dynamic text with optional template
- `title` - Page heading
- `text` - Static text

### Action Widgets
- `actionbutton` - Button with action
- `linkbutton` - Link-styled button

### Special Widgets
- `snippetcall` - Embed snippet
- `template` - Template for Gallery/ListView items
- `filter` with `textfilter` - Gallery filter

## Property Name Mapping

Property names match Studio Pro for familiarity:

| MDL Property | Widget Types |
|--------------|--------------|
| `content` | Text widgets |
| `rendermode` | Text widgets (H1-H6, Paragraph, Text) |
| `buttonstyle` | Buttons (Default, Primary, Success, etc.) |
| `desktopwidth` | Columns (1-12, AutoFill, AutoFit) |
| `selection` | Gallery (Single, Multiple, None) |
| `class` | All widgets |

## Implementation Files

| File | Purpose |
|------|---------|
| `mdl/grammar/MDLLexer.g4` | Tokens for `{`, `}`, `->`, `:`, keywords |
| `mdl/grammar/MDLParser.g4` | Widget and binding grammar rules |
| `mdl/ast/ast_page_v2.go` | AST types for V2 widgets |
| `mdl/visitor/visitor_page_v2.go` | Parser to AST conversion |
| `mdl/executor/cmd_pages_builder.go` | Page building with V2 support |
| `mdl/executor/cmd_pages_builder_widgets_v2.go` | V2 widget builders |

## Backward Compatibility

Both syntaxes are supported:
- **V1 (BEGIN/END)**: Still works, primarily used by DESCRIBE output
- **V2 ({ } and ->)**: New recommended syntax for writing pages

The parser accepts both syntaxes, and the executor handles both AST formats.

## Example Files

See `mdl-examples/doctype-tests/03-page-examples-v2.mdl` for comprehensive examples including:
- Empty pages
- Layout grids with dynamic text
- DataViews with form inputs
- DataGrids with control bars and columns
- Galleries with filters and templates
- Master-detail patterns
- Snippets with parameters
- String templates with WITH syntax

## Verification

```bash
# Parse V2 syntax
./bin/mxcli check mdl-examples/doctype-tests/03-page-examples-v2.mdl

# execute against project
./bin/mxcli -p app.mpr -c "execute script 'mdl-examples/doctype-tests/03-page-examples-v2.mdl'"

# Verify in Studio Pro
reference/mxbuild/modeler/mx check app.mpr
```
