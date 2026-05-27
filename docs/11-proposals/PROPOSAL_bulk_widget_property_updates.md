---
title: Bulk Change Custom Widget Properties
status: draft
date: 2026-01-29
---

# Proposal: Bulk Change Custom Widget Properties

**Status:** Draft
**Author:** Claude
**Date:** 2026-01-29

## Problem Statement

Custom widgets (pluggable widgets) in Mendix have complex nested property structures. There's currently no way to:

1. Find all instances of a specific widget type across pages/snippets
2. Modify property values of existing widgets
3. Bulk update properties across multiple widget instances

## Use Cases

| Use Case | Description |
|----------|-------------|
| Update datasource references | Change the entity/attribute used by all ComboBox widgets |
| Change styling properties | Update color, size, or CSS class across all DataGrids |
| Rename microflow/nanoflow references | After renaming an action, update all widgets that call it |
| Update label text | Bulk change captions/labels across widget instances |
| Refactor attribute paths | When entity attributes are renamed |

## Background: Custom Widget Structure

Custom widgets have a complex nested structure:

```
customwidget
├── widgettype (schema definition)
│   ├── WidgetID: "com.mendix.widget.web.combobox.Combobox"
│   ├── Name: "Combo box"
│   └── ObjectType
│       └── PropertyTypes[]
│           ├── key: "showLabel"
│           ├── ValueType: "boolean"
│           └── ID: <uuid>
│
└── WidgetObject (property values)
    └── properties[]
        ├── TypePointer: <references PropertyType.ID>
        └── value
            ├── PrimitiveValue: "true"
            ├── AttributeRef: "Module.Entity.Attribute"
            ├── microflow: "Module.MicroflowName"
            └── ... (other value types)
```

Each property value has a `TypePointer` that references the corresponding `PropertyType.ID` in the schema. This relationship must be preserved during updates.

## Proposed MDL Syntax

### Option A: UPDATE WIDGETS Statement (Recommended)

SQL-like syntax that's familiar and consistent with catalog queries:

```sql
-- Update all widgets of a specific type across all pages
update widgets
set 'propertyPath' = 'newValue'
where widgettype = 'com.mendix.widget.web.combobox.Combobox';

-- Update with conditions
update widgets
set 'labelCaption' = 'Select Customer'
where widgettype = 'com.mendix.widget.web.combobox.Combobox'
  and 'attributeEnumeration' like '%Customer%';

-- Update in specific pages only
update widgets in MyModule.CustomerPage, MyModule.OrderPage
set 'backgroundColor' = '#f5f5f5'
where widgettype = 'com.mendix.widget.web.datagrid.Datagrid';

-- Update in module
update widgets in module MyModule
set 'showLabel' = false
where widgettype = 'com.mendix.widget.web.combobox.Combobox';
```

### Option B: ALTER PAGE ... WIDGETS Statement

```sql
-- Modify widgets within a specific page
alter page MyModule.CustomerOverview
modify widgets where widgettype = 'Combobox'
set 'source.type' = 'Database';

-- Modify all pages in module
alter pages in MyModule
modify widgets where widgettype = 'DataGrid2'
set 'pagination.pageSize' = 25;
```

### Option C: CHANGE WIDGET Command (Action-Oriented)

```sql
-- Simple property change
change widget PROPERTY 'showLabel' to false
for widget type 'Combobox';

-- Multiple properties
change widget properties (
  'showLabel' = false,
  'readOnly' = true
)
for widget type 'Combobox'
in module MyModule;
```

### Recommendation

**Option A (UPDATE WIDGETS)** is recommended because:
- SQL-like syntax is familiar and consistent with catalog queries
- Flexible WHERE clause for filtering
- Supports scope limitation (pages, modules)
- Natural for bulk operations

## Property Path Syntax

Custom widgets have nested properties. Proposed path notation:

```sql
-- Simple property
'showLabel'                           -- Top-level property

-- Nested property (dot notation)
'dataSource.type'                     -- Property within dataSource object
'pagination.pageSize'                 -- Property within pagination object

-- Array item (bracket notation)
'columns[0].caption'                  -- First column's caption
'columns[*].width'                    -- All columns' width (wildcard)

-- Deep nesting
'dataSource.constraints[0].attribute' -- First constraint's attribute
```

## Implementation Approach

### Phase 1: Query/Discovery (Read-Only)

First implement discovery commands to find widgets and inspect properties:

```sql
-- Find all widgets of a type
show widgets where widgettype = 'Combobox';

-- Show available properties for a widget type
show widget properties for 'com.mendix.widget.web.combobox.Combobox';

-- Query current property values
select PageName, WidgetName, Property('showLabel')
from CATALOG.WIDGETS
where widgettype like '%combobox%';
```

### Phase 2: Single Page Update

```sql
-- Update widgets in one page
update widgets in MyModule.CustomerPage
set 'showLabel' = false
where widgettype = 'Combobox';
```

### Phase 3: Bulk Update

```sql
-- Update across all pages
update widgets
set 'showLabel' = false
where widgettype = 'Combobox';
```

## Technical Implementation

### New Catalog Table: CATALOG.WIDGETS_DETAILS

Extend the existing WIDGETS catalog table with property information:

```sql
create table widgets_details (
  PageID text,
  PageName text,
  WidgetID text,
  WidgetName text,
  widgettype text,           -- e.g., 'com.mendix.widget.web.combobox.Combobox'
  PropertyKey text,          -- e.g., 'showLabel'
  PropertyPath text,         -- e.g., 'dataSource.type'
  PropertyValue text,        -- JSON-encoded value
  PropertyValueType text,    -- 'Primitive', 'Attribute', 'Microflow', etc.
  PropertyTypeID text,       -- For serialization reference
  ModuleName text
);
```

### New Files

| File | Purpose |
|------|---------|
| `mdl/ast/ast_widget_update.go` | AST types for UPDATE WIDGETS statement |
| `mdl/visitor/visitor_widget_update.go` | Parse UPDATE WIDGETS grammar |
| `mdl/executor/cmd_widget_update.go` | Execute widget property updates |
| `mdl/catalog/builder_widgets_details.go` | Build detailed widget property catalog |

### Core Functions

```go
// WidgetFilter defines criteria for finding widgets
type WidgetFilter struct {
    widgettype     string            // full or partial widget type ID
    PropertyFilter map[string]string // Property conditions
    pages          []string          // limit to specific pages
    modules        []string          // limit to specific modules
}

// WidgetMatch represents a found widget
type WidgetMatch struct {
    PageID     model.ID
    PageName   string
    WidgetID   model.ID
    WidgetName string
    widgettype string
    widget     *pages.CustomWidget
}

// find all widgets matching criteria
func (e *Executor) findWidgets(filter WidgetFilter) ([]*WidgetMatch, error)

// update property value in widget
func updateWidgetProperty(widget *pages.CustomWidget, path string, value interface{}) error

// Navigate nested property path
func getPropertyByPath(obj *pages.WidgetObject, path string) (*pages.WidgetProperty, error)

// set property value with type conversion
func setPropertyValue(prop *pages.WidgetProperty, value interface{}) error
```

### Property Value Types

```go
type PropertyValueType int

const (
    PropertyValuePrimitive PropertyValueType = iota  // string, integer, boolean, decimal
    PropertyValueExpression                          // Mendix expression
    PropertyValueAttribute                           // Entity.Attribute reference
    PropertyValueMicroflow                           // microflow reference
    PropertyValueNanoflow                            // nanoflow reference
    PropertyValuePage                                // page reference
    PropertyValueDataSource                          // datasource configuration
    PropertyValueAction                              // ClientAction
    PropertyValueObject                              // Nested WidgetObject
    PropertyValueObjectList                          // list of WidgetObjects
)
```

## Challenges

### 1. Property Type IDs

Each property has a `TypePointer` that must match the `WidgetPropertyType.ID`. When modifying values, these references must be preserved.

**Solution:** Read existing TypePointer, only modify the Value portion.

### 2. Nested Objects

Some properties contain nested WidgetObjects with their own properties. Path navigation must handle these recursively.

**Solution:** Implement recursive path resolver that can navigate `WidgetObject.Properties[].Value.Objects[]`.

### 3. Validation

Some properties have constraints (required, allowed values). Need to validate before writing.

**Solution:**
- Phase 1: No validation (user responsibility)
- Phase 2: Add optional `--validate` flag
- Phase 3: Full validation based on PropertyType metadata

### 4. Serialization

After modification, the page must be correctly re-serialized to BSON format with all IDs intact.

**Solution:** Use existing `UpdatePage()` function which handles serialization correctly.

### 5. Undo/Preview

Should show what will change before applying.

**Solution:** Implement `dry run` mode that reports changes without applying them.

## Example Workflow

```sql
-- 1. Discover: Find all ComboBox widgets
show widgets where widgettype like '%combobox%';

-- Result:
-- | Page                    | Widget    | WidgetType                               |
-- |-------------------------|-----------|------------------------------------------|
-- | MyModule.CustomerPage   | cmbStatus | com.mendix.widget.web.combobox.Combobox  |
-- | MyModule.OrderPage      | cmbCountry| com.mendix.widget.web.combobox.Combobox  |

-- 2. Check current values
select PageName, WidgetName, Property('showLabel')
from CATALOG.WIDGETS_DETAILS
where widgettype like '%combobox%';

-- 3. Preview changes (dry-run)
update widgets dry run
set 'showLabel' = false
where widgettype like '%combobox%';

-- Result:
-- Will update 2 widget(s):
--   MyModule.CustomerPage.cmbStatus: showLabel: true -> false
--   MyModule.OrderPage.cmbCountry: showLabel: true -> false

-- 4. Apply changes
update widgets
set 'showLabel' = false
where widgettype like '%combobox%';

-- Result:
-- Updated 2 widget(s) in 2 page(s)
```

## Grammar Changes

### Lexer (MDLLexer.g4)

```antlr
// Already exists
widgets: W I D G E T S;

// May need to add
PROPERTY: P R O P E R T Y;
dry: D R Y;
run: R U N;
```

### Parser (MDLParser.g4)

```antlr
updateWidgetsStatement
    : update widgets (dry run)?
      (in (qualifiedNameList | module IDENTIFIER))?
      set widgetPropertyAssignmentList
      (where widgetFilterExpression)?
    ;

widgetPropertyAssignmentList
    : widgetPropertyAssignment (COMMA widgetPropertyAssignment)*
    ;

widgetPropertyAssignment
    : STRING_LITERAL equals expression
    ;

widgetFilterExpression
    : WIDGET_TYPE equals STRING_LITERAL
    | WIDGET_TYPE like STRING_LITERAL
    | STRING_LITERAL (equals | like) expression
    | widgetFilterExpression and widgetFilterExpression
    | widgetFilterExpression or widgetFilterExpression
    | LPAREN widgetFilterExpression RPAREN
    ;
```

## Phased Implementation Plan

| Phase | Feature | Effort | Priority |
|-------|---------|--------|----------|
| 1 | `show widgets` query (discovery) | Low | High |
| 2 | `CATALOG.WIDGETS_DETAILS` table with properties | Medium | High |
| 3 | `update widgets` single page | Medium | High |
| 4 | `update widgets` bulk (all pages) | Medium | Medium |
| 5 | Nested property path support | High | Medium |
| 6 | `dry run` preview mode | Low | Medium |
| 7 | Property validation | Medium | Low |

## Alternative Approaches

### Direct Page Modification

For simpler use cases, could provide direct widget property access:

```sql
-- Update specific widget in specific page
alter page MyModule.CustomerPage
set widget 'cmbStatus' PROPERTY 'showLabel' = false;
```

### Scripted Approach

For complex transformations, expose widget data as JSON for external processing:

```sql
-- Export widget data
export widgets to 'widgets.json'
where widgettype like '%combobox%';

-- After external modification, import back
import widgets from 'widgets_modified.json';
```

## Success Criteria

1. Users can discover all instances of a custom widget type
2. Users can view current property values
3. Users can update property values in a single page
4. Users can bulk update properties across multiple pages
5. Changes are correctly persisted and readable by Studio Pro
6. Preview mode shows changes before applying

## Related Work

- MOVE command (relocating documents between folders/modules)
- SHOW IMPACT (analyzing references before changes)
- Catalog queries (SQL-like querying of project metadata)

## Open Questions

1. Should we support updating properties of built-in widgets (TextBox, Button, etc.) or only custom/pluggable widgets?
2. How to handle widgets with the same name in different pages?
3. Should property paths be validated against the widget schema?
4. How to handle rollback if some updates fail mid-batch?

## References

- Custom widget structure: `sdk/pages/pages_widgets_advanced.go`
- Widget serialization: `sdk/mpr/writer_widgets.go`
- Widget templates: `sdk/widgets/templates/`
- Existing catalog: `mdl/catalog/`
