# Creating a Page

Pages are the user interface of a Mendix application. In this page you'll create an overview page that lists products in a data grid, and then an edit page with a form for modifying a single product.

## Prerequisites

Before creating a page, you need two things:

1. **An entity** -- the page needs data to display. We'll use the `MyModule.Product` entity from the previous steps.
2. **A layout** -- every page is placed inside a layout that provides the overall page structure (header, sidebar, content area). The layout must already exist in your project.

To see what layouts are available:

```bash
mxcli -p app.mpr -c "SHOW PAGES IN Atlas_Core"
```

Most Mendix projects based on Atlas UI have layouts like `Atlas_Core.Atlas_Default` (full page), `Atlas_Core.PopupLayout` (dialog), and others.

## Create an overview page

An overview page typically shows a data grid that lists all objects of an entity:

```sql
CREATE PAGE MyModule.ProductOverview
(
    Title: 'Products',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAGRID dgProducts (DataSource: DATABASE MyModule.Product) {
        COLUMN colName (Attribute: Name, Caption: 'Name')
        COLUMN colPrice (Attribute: Price, Caption: 'Price')
        COLUMN colActive (Attribute: IsActive, Caption: 'Active')
    }
}
```

Let's break this down:

| Part | Meaning |
|------|---------|
| `MyModule.ProductOverview` | Fully qualified page name |
| `LAYOUT Atlas_Core.Atlas_Default` | The layout this page uses -- must exist in the project |
| `TITLE 'Products'` | The page title shown in the browser tab and header |
| `DATAGRID SOURCE DATABASE MyModule.Product` | A data grid that loads `Product` objects from the database |
| `COLUMN Name` | A column showing the `Name` attribute |

Execute it:

```bash
mxcli -p app.mpr -c "CREATE PAGE MyModule.ProductOverview (Title: 'Products', Layout: Atlas_Core.Atlas_Default) { DATAGRID dgProducts (DataSource: DATABASE MyModule.Product) { COLUMN colName (Attribute: Name, Caption: 'Name') COLUMN colPrice (Attribute: Price, Caption: 'Price') COLUMN colActive (Attribute: IsActive, Caption: 'Active') } }"
```

Or save to a file and run:

```bash
mxcli check create-pages.mdl
mxcli exec create-pages.mdl -p app.mpr
```

## Verify the page

```bash
mxcli -p app.mpr -c "DESCRIBE PAGE MyModule.ProductOverview"
```

This prints the full page definition with all widgets and their properties.

## Create an edit page

Edit pages use a **DataView** to display and edit a single object. The object is passed as a page parameter:

```sql
CREATE PAGE MyModule.Product_Edit
(
    Params: { $Product: MyModule.Product },
    Title: 'Edit Product',
    Layout: Atlas_Core.PopupLayout
)
{
    DATAVIEW dvProduct (DataSource: $Product) {
        TEXTBOX txtName (Label: 'Name', Attribute: Name)
        TEXTBOX txtPrice (Label: 'Price', Attribute: Price)
        CHECKBOX cbActive (Label: 'Active', Attribute: IsActive)

        FOOTER footer1 {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
        }
    }
};
```

Key differences from the overview page:

| Part | Meaning |
|------|---------|
| `Params: { $Product: MyModule.Product }` | The page expects a `Product` object to be passed when opened |
| `Layout: Atlas_Core.PopupLayout` | Uses a popup/dialog layout instead of a full page |
| `DATAVIEW dvProduct (DataSource: $Product)` | Binds to the page parameter |
| `TEXTBOX`, `CHECKBOX` | Input widgets bound to entity attributes |
| `FOOTER` | A section at the bottom of the DataView for action buttons |
| `Action: SAVE_CHANGES` | Built-in action that commits the object and closes the page |
| `Action: CANCEL_CHANGES` | Built-in action that rolls back changes and closes the page |

Notice the two different page syntaxes: the overview page uses the **compact syntax** (`LAYOUT` and `TITLE` as keywords before parentheses), while the edit page uses the **property syntax** (properties inside a `(Key: value)` block followed by a `{ widget tree }` block). Both are valid -- use whichever fits better.

## Widget reference

Here are the most commonly used widgets:

### Input widgets

```sql
TEXTBOX txtName (Label: 'Name', Attribute: Name)
TEXTAREA txtDesc (Label: 'Description', Attribute: Description)
CHECKBOX cbActive (Label: 'Active', Attribute: IsActive)
DATEPICKER dpCreated (Label: 'Created', Attribute: CreatedDate)
COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
RADIOBUTTONS rbType (Label: 'Type', Attribute: ProductType)
```

### Display widgets

```sql
DYNAMICTEXT dynName (Attribute: Name)
```

### Action buttons

```sql
ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
ACTIONBUTTON btnProcess (Caption: 'Process', Action: MICROFLOW MyModule.ACT_ProcessProduct(Product: $Product))
```

### Layout widgets

```sql
CONTAINER cntWrapper (Class: 'card') {
    -- child widgets go here
}

LAYOUTGRID lgMain {
    ROW r1 {
        COLUMN c1 (Weight: 6) { ... }
        COLUMN c2 (Weight: 6) { ... }
    }
}
```

## Embedding a snippet

To reuse a page fragment, call a snippet:

```sql
SNIPPETCALL scProductCard (Snippet: MyModule.ProductCard)
```

The snippet must already exist in the project.

## Common mistakes

**Layout must exist in the project.** If you specify a layout that doesn't exist, the page will fail validation. Check available layouts with `SHOW PAGES` and look for documents of type `Layout`.

**Widget names must be unique within a page.** Every widget needs a name (e.g., `txtName`, `btnSave`), and these must not collide within the same page.

**DataView needs a data source.** A DataView must know where its data comes from -- either a page parameter (`DataSource: $Product`), a microflow, or a nested context.
