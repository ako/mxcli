# Page Structure

Every MDL page has three main parts: page properties (title, layout, parameters), a layout reference that defines the page skeleton, and a widget tree that fills the content area.

## CREATE PAGE Syntax

```sql
CREATE [OR REPLACE] PAGE <Module>.<Name> [FOLDER '<path>']
(
  [Params: ( $Param: Module.Entity | Type [, ...] ),]
  Title: '<title>',
  Layout: <Module.LayoutName>
  [, Variables: ( $name: Type = <expression> [, ...] )]
)
{
  <widget-tree>
}
```

## Layout Reference

The `Layout` property selects the page layout, which determines the overall structure (navigation sidebar, top bar, popup frame, etc.). The widget tree you define fills the layout's content placeholder.

```sql
CREATE PAGE MyModule.CustomerList
(
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
)
{
  -- Widgets here fill the main content area of Atlas_Default
  DATAGRID dgCustomers (DataSource: DATABASE MyModule.Customer) {
    COLUMN (Attribute: Name, Caption: 'Name')
    COLUMN (Attribute: Email, Caption: 'Email')
  }
};
```

Popup layouts are typically used for edit and detail pages:

```sql
CREATE PAGE MyModule.Customer_Edit
(
  Params: ( $Customer: MyModule.Customer ),
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    FOOTER {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
};
```

## Page Parameters

Page parameters define values that must be passed when the page is opened. Parameters use the `$` prefix and can be entity types or primitive types (String, Integer, Decimal, Boolean, DateTime):

```sql
(
  Params: ( $Customer: MyModule.Customer ),
  ...
)
```

Multiple parameters are comma-separated, and can mix entity and primitive types:

```sql
(
  Params: ( $Order: Sales.Order, $Quantity: Integer, $IsNew: Boolean ),
  ...
)
```

Inside the widget tree, a `DATAVIEW` binds to a parameter using `DataSource: $ParamName`.

## Page Variables

Page variables store local state (booleans, strings, etc.) that can control widget visibility or other conditional logic:

```sql
(
  Title: 'Product Detail',
  Layout: Atlas_Core.Atlas_Default,
  Variables: ( $showDetails: Boolean = true )
)
```

## Data Sources

Data sources tell a container widget (DataView, DataGrid, ListView, Gallery) where to get its data.

### Page Parameter Source

Binds to a page parameter. Used by DataView widgets for edit/detail pages:

```sql
DATAVIEW dvCustomer (DataSource: $Customer) {
  -- widgets bound to Customer attributes
}
```

### Database Source

Retrieves entities directly from the database. Optionally includes an XPath constraint:

```sql
DATAGRID dgOrders (DataSource: DATABASE Sales.Order) {
  COLUMN (Attribute: OrderId, Caption: 'Order #')
  COLUMN (Attribute: OrderDate, Caption: 'Date')
}
```

### Microflow Source

Calls a microflow that returns a list or single object:

```sql
DATAVIEW dvDashboard (DataSource: MICROFLOW MyModule.DS_GetDashboardData) {
  -- widgets bound to the returned object's attributes
}
```

### Nanoflow Source

Same as microflow source but calls a nanoflow (runs on the client):

```sql
LISTVIEW lvRecent (DataSource: NANOFLOW MyModule.DS_GetRecentItems) {
  -- widgets for each item
}
```

### Association Source

Follows an association from a parent DataView's object:

```sql
DATAVIEW dvCustomer (DataSource: $Customer) {
  LISTVIEW lvOrders (DataSource: ASSOCIATION Customer_Order) {
    -- widgets for each Order
  }
}
```

### Selection Source

Binds to the currently selected item in another list widget:

```sql
DATAGRID dgProducts (DataSource: DATABASE MyModule.Product) {
  COLUMN (Attribute: Name)
}

DATAVIEW dvDetail (DataSource: SELECTION dgProducts) {
  TEXTBOX txtDescription (Label: 'Description', Attribute: Description)
}
```

## Widget Tree Structure

The widget tree is a nested hierarchy. Container widgets hold child widgets within `{ }` braces. Every widget requires a unique name — except the elements Mendix stores no name for, which take none: a layout grid's rows and columns, a data grid's columns and control bar, and a gallery's template and filter. A name written there anyway is ignored and reported as MDL-DEPR005.

```sql
{
  LAYOUTGRID grid1 {
    ROW {
      COLUMN {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
      }
      COLUMN {
        TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
      }
    }
  }
}
```

## Folder Organization

Use the `FOLDER` clause after the name to organize pages into folders within a module:

```sql
CREATE PAGE MyModule.Customer_Edit FOLDER 'Customers'
(
  Params: ( $Customer: MyModule.Customer ),
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  ...
}
```

Nested folders use `/` separators: `FOLDER 'Pages/Customers/Detail'`. Missing folders are auto-created. The `Folder: '…'` property is a deprecated alias of the clause (`MDL-DEPR105`).

## See Also

- [Pages](./pages.md) -- page overview and CREATE PAGE basics
- [Widget Types](./widget-types.md) -- full catalog of widgets
- [Data Binding](./data-binding.md) -- attribute binding with the Attribute property
- [Common Patterns](./page-patterns.md) -- list page, edit page, master-detail examples
