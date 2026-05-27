---
title: Page Syntax V3 - Implementation Reference
status: done
---

# Page Syntax V3 - Implementation Reference

## Status: Implemented ✅

> **Note:** This is now the recommended syntax for creating pages in MDL.
> V1 (BEGIN/END) and V2 (->) syntaxes are still supported for backward compatibility.

---

**Refactor MDL Page Grammar for Agent-Friendly Syntax**

Refactor the MDL page grammar to use a more consistent, explicit syntax that is optimized for agentic coding (LLM code generation). The goal is to eliminate syntax variations and make the language predictable with a single pattern for all constructs.

## Core Principle

All widgets follow this pattern:
```
widgettype name (Prop: value, Prop: value) { children }
```

## Changes to Implement

### 1. Unified Page Header Block

All page metadata goes in a single `()` block after the qualified name:

**Before:**
```
create page PgTest.Example (
  $Order: PgTest.Order
)
  title 'Example'
  layout Atlas_Core.Atlas_Default
  url 'example'
{ ... }
```

**After:**
```
create page PgTest.Example
(
  params: {
    $Order: PgTest.Order,
    $Customer: PgTest.Customer
  },
  title: 'Example',
  layout: Atlas_Core.Atlas_Default,
  url: 'example'
)
{ ... }
```

- `params:` block is optional, only present when page has parameters
- All metadata properties use consistent `key: value` syntax

### 2. DataSource Property

Replace `->` and `-> database` with explicit `datasource:` property. Always include the source type keyword.

**Before:**
```
dataview OrderForm -> $Order { ... }
datagrid ProductGrid -> database PgTest.Product { ... }
gallery productGallery -> database PgTest.Product { ... }
```

**After:**
```
dataview OrderForm (datasource: $Order) { ... }
datagrid ProductGrid (datasource: database PgTest.Product) { ... }
gallery productGallery (datasource: database PgTest.Product) { ... }
```

DataSource type keywords:
- `database <entity>` — database source
- `microflow <QualifiedName>` — microflow source
- `nanoflow <QualifiedName>` — nanoflow source
- `association <path>` — association source
- `$ParamName` — parameter reference (no keyword needed, already typed)

### 3. Attribute Property for Attribute Bindings

Replace `->` with explicit `attribute:` property for input widgets.

**Before:**
```
textbox txtName 'Product Name' -> Name
checkbox cbIsActive 'Is Active' -> IsActive
datepicker dpDate 'Created On' -> CreateDate
```

**After:**
```
textbox txtName (label: 'Product Name', attribute: Name)
checkbox cbIsActive (label: 'Is Active', attribute: IsActive)
datepicker dpDate (label: 'Created On', attribute: CreateDate)
```

### 4. Action Property for Buttons and Navigation

Replace `->` with explicit `action:` property. Same syntax for ACTIONBUTTON and navigation ITEM.

**Before:**
```
actionbutton btnSave 'Save' -> save_changes (buttonstyle: success)
actionbutton btnRun 'Run' -> microflow PgTest.DoSomething
item -> show_page 'PageTemplates.Customer_Overview' { ... }
```

**After:**
```
actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
actionbutton btnRun (caption: 'Run', action: microflow PgTest.DoSomething)
item item1 (action: show_page PageTemplates.Customer_Overview) { ... }
```

Action keywords:
- `show_page <PageRef>`
- `microflow <QualifiedName>`
- `nanoflow <QualifiedName>`
- `open_link <url>`
- `save_changes`
- `cancel_changes`
- `close_page`
- `delete_object`
- `create_object`
- `sign_out`
- `CALL_WORKFLOW`
- `SYNCHRONIZE`
- (and others as defined in Mendix)

### 5. Consistent Property Syntax

All properties use `(Prop: value, Prop: value)` syntax. No bare strings or keyword-value pairs outside parentheses.

**Before:**
```
dynamictext text1 'Hello' (rendermode: H2)
title 'Page Title'
layout Atlas_Core.Atlas_Default
```

**After:**
```
dynamictext text1 (content: 'Hello', rendermode: H2)
title: 'Page Title'   // inside header block
layout: Atlas_Core.Atlas_Default   // inside header block
```

### 6. DATAGRID Column Syntax

Columns use `attribute:` for attribute binding, `caption:` for header, and support capability properties and nested children (filters, custom content).

**Before:**
```
datagrid ProductGrid -> database PgTest.Product
  where [Stock < 10]
  ORDER by Stock asc
{
  columns: {
    column -> Name
    column -> Stock
  }
}
```

**After:**
```
datagrid ProductGrid (
  datasource: database from PgTest.Product where [Stock < 10] sort by Stock asc
) {
  column colName (attribute: Name, caption: 'Name')
  column colStock (attribute: Stock, caption: 'Stock', CanSort: true) {
    numberfilter filterStock (attribute: Stock)
  }
  column colActions (caption: 'Actions') {
    actionbutton btnEdit (caption: 'Edit', action: show_page PgTest.Product_Edit)
  }
}
```

- `where` and `sort by` are inline in the `datasource:` expression, matching RETRIEVE syntax
- Columns are direct children, no `columns: { }` wrapper
- Columns can have nested filter widgets or custom content

### 7. Filter Bindings

Filters use `attribute:` property.

**Before:**
```
textfilter searchName -> Name
```

**After:**
```
textfilter searchName (attribute: Name)
numberfilter filterStock (attribute: Stock)
datefilter filterDate (attribute: OrderDate)
```

### 8. GALLERY Syntax

**After:**
```
gallery productGallery (datasource: database PgTest.Product, selection: single) {
  filter {
    textfilter searchName (attribute: Name)
  }
  template {
    dynamictext prodName (content: '{Name}', rendermode: H4)
    dynamictext prodCode (content: 'SKU: {Code}')
    dynamictext prodPrice (content: 'Price: {Price}')
  }
}
```

### 9. Widget Names Always Required

Every widget must have a name identifier. Do not make names optional, as widgets may be referenced from microflows, nanoflows, or other external locations.

## Complete Example

**Before:**
```
create page PgTest.P015_Product_EditFull (
  $Product: PgTest.Product
)
  title 'Edit Product Details'
  layout Atlas_Core.PopupLayout
{
  dataview ProductForm -> $Product {
    textbox txtName 'Product Name' -> Name
    textbox txtPrice 'Unit Price' -> Price
    checkbox cbIsActive 'Product is Active' -> IsActive
    datepicker dpCreatedDate 'Created On' -> CreateDate
    footer {
      actionbutton btnSave 'Save' -> save_changes (buttonstyle: success)
      actionbutton btnCancel 'Cancel' -> cancel_changes (buttonstyle: default)
    }
  }
}
```

**After:**
```
create page PgTest.P015_Product_EditFull
(
  params: {
    $Product: PgTest.Product
  },
  title: 'Edit Product Details',
  layout: Atlas_Core.PopupLayout
)
{
  dataview ProductForm (datasource: $Product) {
    textbox txtName (label: 'Product Name', attribute: Name)
    textbox txtPrice (label: 'Unit Price', attribute: Price)
    checkbox cbIsActive (label: 'Product is Active', attribute: IsActive)
    datepicker dpCreatedDate (label: 'Created On', attribute: CreateDate)
    footer {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: success)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}
```

## Implementation Notes

- Update the grammar/parser to support the new syntax ✅
- Update any code generation or serialization logic ✅
- Ensure the linter validates the new syntax ✅

## Implementation Files

| File | Purpose |
|------|---------|
| `mdl/grammar/MDLLexer.g4` | Tokens for V3 keywords |
| `mdl/grammar/MDLParser.g4` | V3 widget and page grammar rules |
| `mdl/ast/ast_page_v3.go` | AST types for V3 pages and widgets |
| `mdl/visitor/visitor_page_v3.go` | Parser to AST conversion for V3 |
| `mdl/executor/cmd_pages_builder_v3.go` | V3 widget builders |
| `mdl/executor/cmd_pages_describe.go` | DESCRIBE output in V3 format |

## Verification

```bash
# Parse V3 syntax
./bin/mxcli check mdl-examples/doctype-tests/04-page-examples-v3.mdl

# execute against project
./bin/mxcli -p app.mpr -c "execute script 'mdl-examples/doctype-tests/04-page-examples-v3.mdl'"

# Verify in Studio Pro
reference/mxbuild/modeler/mx check app.mpr
```

## Example Files

See `mdl-examples/doctype-tests/04-page-examples-v3.mdl` for comprehensive examples.

## Skill Documentation

See `.claude/skills/mendix/`:
- `create-page.md` - V3 syntax reference
- `overview-pages.md` - CRUD page patterns
- `master-detail-pages.md` - Master-detail patterns
