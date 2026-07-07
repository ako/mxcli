# CREATE PAGE

## Synopsis

```sql
CREATE [ OR REPLACE ] PAGE module.Name
(
    [ Params: { $param : Module.Entity | Type [, ...] }, ]
    Title: 'title',
    Layout: Module.LayoutName
    [, Folder: 'path' ]
    [, Variables: { $name : type = 'expression' [, ...] } ]
)
{
    widget_tree
}
```

## Description

Creates a new page in the specified module. A page defines a user interface screen with a title, a layout, optional parameters, and a hierarchical widget tree.

If `OR REPLACE` is specified and a page with the same qualified name already exists, it is replaced. Otherwise, creating a page with an existing name is an error.

### Page Properties

The page declaration block `( ... )` contains comma-separated key-value properties:

- **Params** -- Page parameters. Each parameter has a `$`-prefixed name and a type — either an entity (`Module.Entity`) or a primitive (`String`, `Integer`, `Decimal`, `Boolean`, `DateTime`). Parameters are passed when the page is opened (e.g., from a microflow `SHOW PAGE` or from a data view). Primitive parameters require Mendix 11.6+.
- **Title** -- The page title displayed in the browser tab or page header.
- **Layout** -- The layout the page is based on. Must be a qualified name referring to an existing layout (e.g., `Atlas_Core.Atlas_Default`, `Atlas_Core.PopupLayout`).
- **Folder** -- Optional folder path within the module. Nested folders use `/` separator.
- **Variables** -- Optional page-level variables with default expressions. Variables can be used for conditional visibility and dynamic behavior.

### Widget Tree

The widget tree inside `{ ... }` defines the page content. Widgets are nested hierarchically. Each widget has:

- A **type keyword** (e.g., `DATAVIEW`, `TEXTBOX`, `ACTIONBUTTON`)
- A **name** (unique within the page, used for ALTER PAGE references)
- **Properties** in parentheses `(Key: value, ...)`
- Optional **children** in braces `{ ... }`

### Widget Types

**Layout Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `LAYOUTGRID` | Responsive grid container | `Class`, `Style` |
| `ROW` | Grid row | `Class` |
| `COLUMN` | Grid column | `Class` |
| `CONTAINER` | Generic div container | `Class`, `Style`, `DesignProperties` |
| `CUSTOMCONTAINER` | Custom container widget | `Class`, `Style` |

**Data Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `DATAVIEW` | Displays/edits a single object | `DataSource` |
| `LISTVIEW` | Renders a list of objects | `DataSource`, `PageSize` |
| `DATAGRID` | Tabular data display with columns | `DataSource`, `PageSize`, `Pagination` |
| `GALLERY` | Card-based list display | `DataSource`, `DesktopColumns`, `TabletColumns`, `PhoneColumns` |

**Input Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `TEXTBOX` | Single-line text input | `Label`, `Attribute` |
| `TEXTAREA` | Multi-line text input | `Label`, `Attribute` |
| `CHECKBOX` | Boolean toggle | `Label`, `Attribute` |
| `RADIOBUTTONS` | Radio button group | `Label`, `Attribute` |
| `DATEPICKER` | Date/time selector | `Label`, `Attribute` |
| `COMBOBOX` | Dropdown selector | `Label`, `Attribute` |

**Display Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `DYNAMICTEXT` | Display-only text bound to an attribute | `Attribute` |
| `IMAGE` | Generic image | `Width`, `Height` |
| `STATICIMAGE` | Fixed image from project resources | `Width`, `Height` |
| `DYNAMICIMAGE` | Image from an entity attribute | `Width`, `Height` |

**Action Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `ACTIONBUTTON` | Button that triggers an action | `Caption`, `Action`, `ButtonStyle` |
| `LINKBUTTON` | Hyperlink-styled action | `Caption`, `Action` |

**Structure Widgets**

| Widget | Description | Key Properties |
|--------|-------------|----------------|
| `HEADER` | Page header area | -- |
| `FOOTER` | Page footer area (typically for buttons) | -- |
| `CONTROLBAR` | Control bar for data grids | -- |
| `SNIPPETCALL` | Embeds a snippet | `Snippet: Module.SnippetName` |
| `NAVIGATIONLIST` | Navigation sidebar list | -- |

### DataSource Types

The `DataSource` property determines how a data widget obtains its data:

| DataSource | Syntax | Description |
|------------|--------|-------------|
| Parameter | `DataSource: $ParamName` | Binds to a page parameter or enclosing data view |
| Database | `DataSource: DATABASE Module.Entity` | Retrieves from database |
| Selection | `DataSource: SELECTION widgetName` | Listens to selection on another widget |
| Microflow | `DataSource: MICROFLOW Module.MFName` | Calls a microflow for data |

### Action Types

The `Action` property on buttons determines what happens when clicked:

| Action | Syntax | Description |
|--------|--------|-------------|
| Save | `Action: SAVE_CHANGES` | Commits and closes |
| Cancel | `Action: CANCEL_CHANGES` | Rolls back and closes |
| Microflow | `Action: MICROFLOW Module.Name(Param: val)` | Calls a microflow |
| Nanoflow | `Action: NANOFLOW Module.Name(Param: val)` | Calls a nanoflow |
| Page | `Action: PAGE Module.PageName` | Opens a page |
| Close | `Action: CLOSE_PAGE` | Closes the current page |
| Delete | `Action: DELETE` | Deletes the context object |

### ButtonStyle Values

`Primary`, `Default`, `Success`, `Danger`, `Warning`, `Info`.

### DataGrid Columns

Inside a `DATAGRID`, child widgets define columns. Each column widget specifies:

| Property | Values | Description |
|----------|--------|-------------|
| `Attribute` | attribute name | The entity attribute to display |
| `Caption` | string | Column header (defaults to attribute name) |
| `Alignment` | `left`, `center`, `right` | Text alignment |
| `WrapText` | `true`, `false` | Whether text wraps |
| `Sortable` | `true`, `false` | Whether column is sortable |
| `Resizable` | `true`, `false` | Whether column is resizable |
| `Draggable` | `true`, `false` | Whether column is draggable |
| `Hidable` | `yes`, `hidden`, `no` | Column visibility control |
| `ColumnWidth` | `autoFill`, `autoFit`, `manual` | Width mode |
| `Size` | integer (px) | Manual width in pixels |
| `Visible` | expression string | Conditional visibility |
| `Tooltip` | text string | Column tooltip |

### Common Widget Properties

These properties are available on most widget types:

| Property | Description | Example |
|----------|-------------|---------|
| `Class` | CSS class names | `Class: 'card mx-spacing-top-large'` |
| `Style` | Inline CSS | `Style: 'padding: 16px;'` |
| `Editable` | Edit control | `Editable: NEVER` or `Editable: ALWAYS` |
| `Visible` | Visibility expression | `Visible: '$showField'` |
| `DesignProperties` | Atlas design properties | `DesignProperties: ['Spacing top': 'Large']` |

## Parameters

`module.Name`
:   The qualified name of the page (`Module.PageName`). The module must already exist.

`Params: { ... }`
:   Optional page parameters. Each parameter has a `$`-prefixed name and an entity type.

`Title: 'title'`
:   The display title of the page. Required.

`Layout: Module.LayoutName`
:   The page layout. Must reference an existing layout. Required.

`Folder: 'path'`
:   Optional folder within the module. Nested folders use `/`.

`Variables: { ... }`
:   Optional page variables with type and default expression.

## Examples

Edit page with a data view and form fields:

```sql
CREATE PAGE MyModule.Customer_Edit
(
    Params: { $Customer: MyModule.Customer },
    Title: 'Edit Customer',
    Layout: Atlas_Core.PopupLayout
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
        COMBOBOX cbStatus (Label: 'Status', Attribute: Status)

        FOOTER footer1 {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
        }
    }
};
```

Overview page with a data grid:

```sql
CREATE PAGE Sales.Order_Overview
(
    Title: 'Orders',
    Layout: Atlas_Core.Atlas_Default,
    Folder: 'Orders'
)
{
    DATAGRID dgOrders (DataSource: DATABASE Sales.Order, PageSize: 20) {
        COLUMN colId (Attribute: OrderId, Caption: 'Order #')
        COLUMN colDate (Attribute: OrderDate, Caption: 'Date')
        COLUMN colStatus (Attribute: Status, Caption: 'Status')
        COLUMN colAmount (Attribute: TotalAmount, Caption: 'Amount', Alignment: right)
        CONTROLBAR cb1 {
            ACTIONBUTTON btnNew (Caption: 'New Order', Action: SHOW_PAGE Sales.Order_Edit, ButtonStyle: Primary)
        }
    }
};
```

Page with layout grid and containers:

```sql
CREATE PAGE MyModule.Dashboard
(
    Title: 'Dashboard',
    Layout: Atlas_Core.Atlas_Default
)
{
    LAYOUTGRID lgMain {
        ROW row1 {
            COLUMN col1 (Class: 'col-md-8') {
                CONTAINER cntRecent (Class: 'card') {
                    LISTVIEW lvRecent (DataSource: DATABASE MyModule.RecentActivity, PageSize: 5) {
                        DYNAMICTEXT txtDesc (Attribute: Description)
                    }
                }
            }
            COLUMN col2 (Class: 'col-md-4') {
                CONTAINER cntStats (Class: 'card') {
                    DYNAMICTEXT txtCount (Attribute: TotalCount)
                }
            }
        }
    }
};
```

Page with snippet call:

```sql
CREATE PAGE MyModule.Customer_Detail
(
    Params: { $Customer: MyModule.Customer },
    Title: 'Customer Detail',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        SNIPPETCALL snpHeader (Snippet: MyModule.CustomerHeader)
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    }
};
```

Page with a gallery and selection-driven detail:

```sql
CREATE PAGE Sales.Product_Gallery
(
    Title: 'Products',
    Layout: Atlas_Core.Atlas_Default
)
{
    LAYOUTGRID lgMain {
        ROW row1 {
            COLUMN col1 (Class: 'col-md-6') {
                GALLERY galProducts (DataSource: DATABASE Sales.Product, PageSize: 12) {
                    DYNAMICTEXT txtName (Attribute: Name)
                    DYNAMICTEXT txtPrice (Attribute: Price)
                }
            }
            COLUMN col2 (Class: 'col-md-6') {
                DATAVIEW dvDetail (DataSource: SELECTION galProducts) {
                    TEXTBOX txtName (Label: 'Product', Attribute: Name)
                    TEXTBOX txtDesc (Label: 'Description', Attribute: Description)
                }
            }
        }
    }
};
```

Page with page variables and conditional visibility:

```sql
CREATE PAGE MyModule.AdvancedForm
(
    Params: { $Item: MyModule.Item },
    Title: 'Advanced Form',
    Layout: Atlas_Core.Atlas_Default,
    Variables: { $showAdvanced: Boolean = 'false' }
)
{
    DATAVIEW dvItem (DataSource: $Item) {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        ACTIONBUTTON btnToggle (Caption: 'Show Advanced', Action: NANOFLOW MyModule.NAV_Toggle)
        CONTAINER cntAdvanced (Visible: '$showAdvanced') {
            TEXTAREA taNotes (Label: 'Notes', Attribute: Notes)
        }
        FOOTER footer1 {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
        }
    }
};
```

Page with microflow data source:

```sql
CREATE PAGE Reports.MonthlySummary
(
    Title: 'Monthly Summary',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAVIEW dvReport (DataSource: MICROFLOW Reports.DS_GetMonthlySummary) {
        DYNAMICTEXT txtPeriod (Attribute: Period)
        DYNAMICTEXT txtRevenue (Attribute: TotalRevenue)
        DYNAMICTEXT txtOrders (Attribute: OrderCount)
    }
};
```

## See Also

[CREATE SNIPPET](create-snippet.md), [ALTER PAGE](alter-page.md), [DROP PAGE](drop-page.md), [DESCRIBE PAGE](/reference/query/describe-page.md), [GRANT VIEW ON PAGE](/reference/security/grant.md)
