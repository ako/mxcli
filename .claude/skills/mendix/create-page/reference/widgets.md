# Widget catalogue

Supporting reference for [create-page](../SKILL.md).

## Supported Widgets

### DYNAMICTEXT Widget

Display dynamic or static text:

```sql
-- Simple text
dynamictext heading (content: 'Heading Text', rendermode: H2)

-- Text bound to page parameter attribute (use $ParamName.Attribute)
-- This preserves the parameter reference for pages with multiple parameters of the same type
dynamictext productName (content: '$Product.Name', rendermode: H3)

-- Explicit template with page parameter binding
dynamictext greeting (content: 'Welcome, {1}!', contentparams: [{1} = $Customer.Name])

-- Template with attribute from current DataView context (simple attribute name)
dynamictext email (content: 'Email: {1}', contentparams: [{1} = Email])

-- Bind directly to an attribute of the surrounding DataView/ListView/Gallery
-- entity. `Attribute: X` is shorthand for `content: '{1}', contentparams: [{1} = X]`.
dynamictext title (Attribute: Title)
```

**ContentParams Reference Types:**
| Syntax | Context | Example |
|--------|---------|---------|
| `$ParamName.Attr` | Page parameter attribute | `$Product.Name` |
| `AttrName` | Current DataView/Gallery entity | `Name`, `Email` |
| `'literal'` | String literal expression | `'Hello'` |

**Formatting a parameter (Decimal / DateTime / Enum):** append a `format (…)`
block to a content parameter. Without it, a Decimal renders with the platform
default (e.g. `5068.38000000`).

```sql
-- Decimal: 2 decimals + thousands separator  ->  "5,068.38"
dynamictext amt (content: '{1}', contentparams: [{1} = Amount format (decimalPrecision: 2, groupDigits: true)])

-- DateTime: date + time, or a custom pattern
dynamictext due  (content: '{1}', contentparams: [{1} = DueOn format (dateFormat: DateTime)])
dynamictext day  (content: '{1}', contentparams: [{1} = DueOn format (dateFormat: Custom, customDateFormat: 'dd-MM-yyyy')])
```

| Format key | Applies to | Values |
|------------|-----------|--------|
| `decimalPrecision` | Decimal / Float | a non-negative integer |
| `groupDigits` | Decimal / Float | `true` \| `false` |
| `dateFormat` | DateTime | `Date` \| `DateTime` \| `Time` \| `Custom` |
| `customDateFormat` | DateTime | a pattern string, requires `dateFormat: Custom` |
| `enumFormat` | Enumeration | `Text` \| `Image` |

> The **`format` keyword is required** — a bare `(…)` after the value is
> ambiguous with a function call. Putting a format key at the **widget** level
> (e.g. `dynamictext x (…, decimalPrecision: 2)`) is an error (MDL-WIDGET18):
> formatting is per-parameter, so it must go inside the `contentparams` block.

> **Never leave a `{N}` placeholder unbound.** `content: '{1}'` with no
> `Attribute:`/`ContentParams:` is an orphaned template — `mxcli check` rejects it
> (MDL-WIDGET04), MxBuild fails with CE0720, and Studio Pro throws a
> NullReferenceException when the widget is opened. Bind every placeholder, or use
> a plain static `content: 'text'`.

### ACTIONBUTTON Widget

Create a button with action binding:

```sql
actionbutton widgetName (caption: 'Caption', action: ACTION_TYPE [, buttonstyle: style] [, icon: 'Module.IconCollection.IconName'])
```

`icon:` names an icon inside an icon collection — `Module.Collection.IconName`,
e.g. `'Atlas_Core.Atlas_Filled.pencil'`. Browse what a project has with
`show icon collection` and `describe icon collection Atlas_Core.Atlas_Filled`.

A wrong icon name is a **build error** (CE1613, *"The selected custom icon … no
longer exists"*), so check it before building:

```bash
mxcli check script.mdl -p app.mpr --references
```

That resolves every icon reference against the project's collections and
suggests near matches for a typo. It needs `-p` — the collections are documents
in the project, so a plain `mxcli check` cannot see them.

Use `linkbutton` instead of `actionbutton` for a button rendered as a link (same
properties). Both accept an `icon:` — an **icon-collection** reference, e.g.
`icon: 'Atlas_Core.Atlas_Filled.pencil'` (the modern Atlas icon set). The name
must exist in the icon collection or MxBuild rejects it (CE1613).

**Find valid icon names** — don't guess (icons have non-obvious names: it's
`add`, not `plus`). List them:

```
show icon collections                              -- the project's icon sets
describe icon collection Atlas_Core.Atlas_Filled   -- every icon + its reference form
```

**Action Bindings:**
- `action: save_changes` - Save changes to object
- `action: save_changes close_page` - Save and close page
- `action: cancel_changes` - Cancel changes
- `action: close_page` - Close the page
- `action: delete` - Delete object
- `action: microflow Module.MicroflowName` - Call microflow
- `action: microflow Module.MicroflowName(Param: $value)` - Call microflow with parameters
- `action: nanoflow Module.NanoflowName` - Call nanoflow (client-side)
- `action: nanoflow Module.NanoflowName(Param: $value)` - Call nanoflow with parameters
- `action: show_page Module.PageName` - Navigate to page
- `action: show_page Module.PageName(Param: $value)` - Navigate with parameters
- `action: show_page Module.PageName($Param = $value)` - Also accepted (microflow-style)
- `action: create_object Module.Entity then show_page Module.PageName` - Create and navigate
- **A `show_page` argument must be the context object.** Mendix takes the page
  argument from the enclosing data widget, so the only spellings that mean
  anything are `$currentObject` or the name of the variable that widget is bound
  to (`datasource: $Customer` → `(Customer: $Customer)` is fine). Naming any other
  variable is refused as **MDL-PAGEARG01** — it used to be accepted and silently
  opened the page with the context object anyway. To open a page with something
  else, call a microflow that shows it.

**Button Styles:** `default`, `primary`, `success`, `info`, `warning`, `danger`, `inverse`
- Case-insensitive (`primary` and `Primary` both work).
- These are the only values Mendix recognizes — anything else (a typo, or `secondary`/`link`, which Mendix has no button style for) is rejected by `mxcli check` (MDL-WIDGET02). Previously an unknown value was silently rendered as `btn-default`.

**Examples:**
```sql
-- Save with style
actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)

-- Navigate with parameter (inside DATAVIEW)
actionbutton btnEdit (caption: 'Edit', action: show_page Module.EditPage(Product: $Product))

-- Navigate with $currentObject (inside DATAGRID column)
actionbutton btnEdit (caption: 'Edit', action: show_page Module.EditPage(Product: $currentObject))

-- Call microflow with page/dataview parameter
actionbutton btnProcess (caption: 'Process', action: microflow Module.ACT_Process(Order: $Order), buttonstyle: success)

-- Call microflow with $currentObject (inside DATAGRID/LISTVIEW column)
actionbutton btnDelete (caption: 'Delete', action: microflow Module.ACT_Delete(Target: $currentObject), buttonstyle: danger)

-- Create object and show page
actionbutton btnNew (caption: 'New', action: create_object Module.Product then show_page Module.Product_Edit, buttonstyle: primary)
```

**Using `$currentObject`:**
Use `$currentObject` inside DATAGRID, LISTVIEW, or GALLERY columns to reference the current row's object. This is typically used in columns with `ShowContentAs: customContent` for action buttons.

### LISTVIEW Specialization Templates

A List View over a **generalization** can render a different body per
specialization. The template is identified by the entity it renders — it has no
name, which is why the keyword takes `for` and a qualified entity:

```sql
listview vehicleListView (datasource: database from Pages.Vehicle) {
  -- the default body: used for an object no template matches
  dynamictext defaultVehicle (content: '{1} {2}', contentparams: [{1} = Brand, {2} = Model])

  template for Pages.Bus {
    dynamictext busLabel (content: 'Bus, capacity {1}', contentparams: [{1} = PassengerCapacity])
  }
  template for Pages.Truck {
    dynamictext truckLabel (content: 'Truck, max load {1} kg', contentparams: [{1} = MaxLoadKg])
  }
}
```

Rules:

- The entity must be the list view's entity **or a specialization of it**. A
  template for an unrelated entity can never match, so it is refused.
- **At most one template per entity.**
- Templates keep their **source order** — Mendix stores and matches in that
  order, so it is authored, not derived, and DESCRIBE emits them as stored.
- Inside a template the context object **is the specialization**, so an
  attribute only that specialization has resolves there (`PassengerCapacity` on
  `Pages.Bus` above, which `Pages.Vehicle` does not have).

Do not confuse this with a **Gallery's** `template <name>`, which is a named
content slot, not a per-specialization body.

### LINKBUTTON Widget

Similar to ActionButton but rendered as link:

```sql
linkbutton linkName (caption: 'Caption', action: ACTION_TYPE)
```

### LAYOUTGRID Widget

Create responsive grid layout:

```sql
layoutgrid gridName {
  row rowName {
    column colName (desktopwidth: 8) {
      -- Nested widgets
    }
    column col2 (desktopwidth: 4) {
      -- Nested widgets
    }
  }
}
```

**Column Width Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `desktopwidth` | 1-12 or `autofill` | `autofill` | Desktop column width |
| `tabletwidth` | 1-12 or `autofill` | auto | Tablet column width |
| `phonewidth` | 1-12 or `autofill` | auto | Phone column width |

```sql
column col1 (desktopwidth: 8, tabletwidth: 6, phonewidth: 12) { ... }
```

Example:
```sql
layoutgrid mainGrid {
  row row1 {
    column colMain (desktopwidth: 8) {
      dynamictext heading (content: 'Main Content', rendermode: H3)
    }
    column colSide (desktopwidth: 4) {
      dynamictext sideHeading (content: 'Sidebar', rendermode: H3)
    }
  }
}
```

### DATAGRID Widget

Display list of objects using DataGrid widget:

```sql
datagrid gridName (
  datasource: database from Module.Entity where [condition] sort by attributename asc|desc,
  selection: Multi
) {
  column colName (attribute: attributename, caption: 'Label')
}
```

> **Reserved keyword column names:** If the attribute name is an MDL reserved keyword (e.g. `Status`, `Type`), you must quote the attribute value and use a distinct widget name for the column:
> ```sql
> column colStatus (attribute: "Status", caption: 'Status')
> column colType   (attribute: "Type",   caption: 'Type')
> ```
> Writing `COLUMN Status (attribute: Status)` fails silently — `Status` and `Type` are parsed as keywords. Always use a `col`-prefixed widget name when the attribute name is reserved.

**Column Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `attribute` | attribute name | (required) | Attribute binding |
| `caption` | string | attribute name | Column header text |
| `Alignment` | `left`, `center`, `right` | `left` | Text alignment |
| `WrapText` | `true`, `false` | `false` | Wrap text in cell |
| `Sortable` | `true`, `false` | `true` (if attribute), `false` (if not) | Can sort column |
| `Resizable` | `true`, `false` | `true` | Can resize column |
| `Draggable` | `true`, `false` | `true` | Can reorder column |
| `Hidable` | `yes`, `hidden`, `no` | `yes` | Can hide column |
| `ColumnWidth` | `autofill`, `autoFit`, `manual` | `autofill` | Column width mode |
| `Size` | integer (px) | `1` | Width in pixels (when `ColumnWidth: manual`) |
| `visible` | expression string | `true` | Column-level visibility — hides/shows the whole column, so use page variables, NOT `$currentObject` (per-object widget visibility is different — see "Conditional Visibility and Editability") |
| `DynamicCellClass` | expression string | (empty) | Dynamic CSS class per cell |
| `tooltip` | text string | (empty) | Cell tooltip text |

Only non-default column properties appear in `describe page` output.

**Dynamic-text columns (`ShowContentAs: dynamicText`):** a column can render its cell as a formatted text template instead of a bare attribute — the same `Content` / `ContentParams` / `format (...)` syntax as a `dynamictext` widget (see below). The column needs no `Attribute`; give it a `Caption` for the header.

```sql
datagrid gridName (datasource: database from Module.Entity) {
  -- Decimal with 2 decimals + thousands separator: renders e.g. "Amt: -1,234.50"
  column amount (
    Caption: 'Amount',
    ShowContentAs: dynamicText,
    Content: 'Amt: {1}',
    ContentParams: [{1} = Amount format (decimalPrecision: 2, groupDigits: true)]
  )
  column due (attribute: DueOn, caption: 'Due')
}
```

The `format (...)` block accepts `decimalPrecision`, `groupDigits`, `dateFormat` (`Date` / `DateTime` / `Time` / `Custom`), `customDateFormat`, and `enumFormat` (`Text` / `Image`). Formatting is applied by Mendix only to **attribute-bound** parameters — bind the bare attribute (`Amount`), not `toString(...)`.

```sql
column colPrice (
  attribute: Price, caption: 'Unit Price',
  Alignment: right, WrapText: true,
  Sortable: false, Resizable: false,
  Hidable: hidden,
  ColumnWidth: manual, Size: 150,
  DynamicCellClass: 'if($currentObject/Price > 100) then ''highlight'' else '''' ',
  tooltip: 'Price in USD'
)
```

**Associated-attribute columns:**

A column can bind an attribute *over an association*, not just an own-entity
attribute — use a bare association path `Assoc/Attr`:

```sql
datagrid dgOrders (datasource: database from Sales.Order) {
  column colNumber   (attribute: Number, caption: 'Order #')
  column colCustomer (attribute: Order_Customer/Name, caption: 'Customer')  -- associated attr
}
```

The association name is bare (resolved against the grid's entity module);
multi-hop paths (`A/B/Attr`) are supported. Module-qualified associations
(`Module.Assoc/Attr`) are **not** accepted — use the bare name.

**Custom Content Columns:**

Columns can contain nested widgets instead of attribute bindings. These build
correctly on the default engine (mxbuild-verified, 0 errors) — an earlier CE0463
(column property ordering) was fixed:

```sql
column colActions (caption: 'Actions') {
  actionbutton btnView (caption: 'View', action: close_page)
}
```

**Supported Datasource Types:**

| Syntax | Description |
|--------|-------------|
| `datasource: database from Module.Entity` | Direct database query |
| `datasource: $Variable` | Variable bound (requires DATAVIEW parent with entity) |
| `datasource: microflow Module.GetData` | Microflow datasource — no `()`, the name alone |
| `datasource: nanoflow Module.GetData` | Nanoflow datasource (client-side, no server roundtrip) — no `()` |
| `datasource: selection widgetName` | Listen to selection from another widget |
| `datasource: association path` | Retrieve by association from context (ByAssociation) |
| `datasource: $currentObject/Module.Assoc` | Sugar for `association` — same semantics, reads more naturally |

**With WHERE and SORT BY (inline in DataSource):**
```sql
datagrid dgActive (
  datasource: database from Module.Product where [IsActive = true] sort by Name asc
) {
  column colName (attribute: Name, caption: 'Name')
  column colPrice (attribute: Price, caption: 'Price')
}
```

**Complex WHERE conditions:**
```sql
datagrid dgFiltered (
  datasource: database from Module.Product
    where [IsActive = true and contains(Code, 'a') and Price > 10] or [Stock < 2]
    sort by Name asc, Price desc
) {
  column colName (attribute: Name, caption: 'Name')
}
```

**Paging Properties:**

| Property | Values | Default | Description |
|----------|--------|---------|-------------|
| `PageSize` | Any positive integer | 20 | Number of rows per page |
| `Pagination` | `buttons`, `virtualScrolling`, `loadMore` | `buttons` | Paging mode |
| `PagingPosition` | `bottom`, `top`, `both` | `bottom` | Position of paging controls |
| `ShowPagingButtons` | `always`, `auto` | `always` | When to show paging buttons |

```sql
-- Paging buttons above and below, 25 rows per page
datagrid dgProducts (
  datasource: database Module.Product,
  PageSize: 25,
  PagingPosition: both
) {
  column colName (attribute: Name, caption: 'Name')
}

-- Virtual scrolling (no paging buttons)
datagrid dgLargeList (
  datasource: database Module.Product,
  PageSize: 50,
  Pagination: virtualScrolling
) {
  column colName (attribute: Name, caption: 'Name')
}
```

Only non-default paging properties appear in `describe page` output.

### DATAVIEW Widget

Display single object with nested input widgets:

```sql
dataview dvName (datasource: $VariableName) {
  -- Nested input widgets
  textbox txtName (label: 'Name', attribute: Name)
  textarea txtDescription (label: 'Description', attribute: description)

  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}
```

### Input Widgets

Input widgets must be inside a DATAVIEW context. Use `attribute:` to bind to attributes:

**TEXTBOX** - Single-line text input:
```sql
textbox txtName (label: 'Label', attribute: attributename)
```

**TEXTAREA** - Multi-line text input:
```sql
textarea txtDescription (label: 'Description', attribute: description)
```

**CHECKBOX** - Boolean checkbox:
```sql
checkbox cbActive (label: 'Active', attribute: IsActive)
```

**RADIOBUTTONS** - Boolean or enum selection:
```sql
radiobuttons rbStatus (label: 'Status', attribute: status)
```

**DATEPICKER** - Date/time selection:
```sql
datepicker dpCreated (label: 'Created Date', attribute: CreatedDate)
```

**COMBOBOX** - Combo box (pluggable widget):
```sql
-- Enumeration mode (attribute is an enum type):
combobox cbCountry (label: 'Country', attribute: Country)

-- Association mode: bind a reference. Requires the option DataSource (the target
-- entity whose objects fill the dropdown) AND a CaptionAttribute (display value).
-- The reference can be given as `Association:` or, equivalently, `attribute:`.
combobox cmbCustomer (label: 'Customer', Association: Order_Customer, datasource: database MyModule.Customer, CaptionAttribute: Name)
-- WRONG: `combobox (Association: X)` with no datasource — mxcli check errors
-- MDL-WIDGET16 (Mendix would otherwise drop the binding → CE0642).
```

### DataView with Form Layout

```sql
dataview dataView1 (datasource: $Customer) {
  textbox txtName (label: 'Name', attribute: Name)
  textbox txtEmail (label: 'Email', attribute: Email)
  textarea txtAddress (label: 'Address', attribute: Address)
  combobox cbStatus (label: 'Status', attribute: status)
  checkbox cbActive (label: 'Active', attribute: IsActive)
  datepicker dpCreated (label: 'Created', attribute: CreateDate)

  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}
```

**Form Orientation (label placement):** the DataView's Studio Pro "Form
Orientation" radio is stored as `LabelWidth` in BSON. Specify either form in MDL:

```sql
dataview dv (datasource: $Customer, FormOrientation: Vertical)    -- label above
dataview dv (datasource: $Customer, FormOrientation: Horizontal)  -- label beside (default, LabelWidth=3)
dataview dv (datasource: $Customer, LabelWidth: 4)                -- explicit, 0..12 columns of 12
```

`LabelWidth: 0` ⇔ `FormOrientation: Vertical`. If both are given, `LabelWidth` wins.

**Footer (`showFooter`):** a `footer { … }` block turns the footer on by itself, so
the property is only needed when the two would disagree:

```sql
dataview dv (datasource: $Customer, showFooter: true)              -- empty footer, shown
dataview dv (datasource: $Customer, showFooter: false) {           -- widgets declared, hidden
  footer f { dynamictext t (content: 'hidden') }
}
```

An explicit `showFooter` wins over the block in both directions, and hiding a footer
never discards its widgets.

### GALLERY Widget

Display items in card layout with selection and responsive columns:

```sql
gallery galleryName (
  datasource: database from Module.Entity sort by Name asc,
  selection: Single,
  DesktopColumns: 3,
  TabletColumns: 2,
  PhoneColumns: 1
) {
  template template1 {
    dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
    dynamictext email (content: '{1}', contentparams: [{1} = Email])
  }
}
```

**With Filter:**
```sql
gallery productGallery (datasource: database Module.Product, selection: single) {
  filter filter1 {
    textfilter searchName (attribute: Name)
  }
  template template1 {
    dynamictext prodName (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
    dynamictext prodCode (content: 'SKU: {1}', contentparams: [{1} = Code])
  }
}
```

### Filter Widgets

Filter widgets are used inside GALLERY FILTER containers to enable search/filtering:

**TEXTFILTER** - Text search filter:
```sql
-- Simple binding to single attribute
textfilter searchName (attribute: Name)

-- Multiple attributes with explicit list
textfilter textFilter1 (attributes: [Module.Entity.Name, Module.Entity.Code, Module.Entity.Description])

-- With filter type
textfilter textFilter1 (attributes: [Module.Entity.Description], filtertype: startsWith)
```

**FilterType Options:**
- `contains` (default) - Matches if attribute contains text
- `startsWith` - Matches if attribute starts with text
- `endsWith` - Matches if attribute ends with text
- `equal` - Exact match

**NUMBERFILTER** - Numeric range filter:
```sql
numberfilter priceFilter (attributes: [Module.Entity.Price])
```

**DATEFILTER** - Date range filter:
```sql
datefilter datefilter (attributes: [Module.Entity.CreateDate])
```

**DROPDOWNFILTER** - Dropdown selection filter:
```sql
dropdownfilter statusFilter (attributes: [Module.Entity.Status])
```

Filter by an **association** instead of an attribute — the options are the
associated objects. Giving the filter a `datasource:` (the OPTION list) selects
this mode; all three parts are required:

```sql
column colCustomer (attribute: Order_Customer/Name, caption: 'Customer') {
  dropdownfilter ddfCustomer (
    Association: Sales.Order_Customer,          -- the reference on the GRID entity
    datasource: database Sales.Customer,        -- the option list (associated entity)
    CaptionAttribute: Name                      -- what each option shows
  )
}
```

> **A column cannot bind the association itself.** `column c (attribute: Order_Customer)`
> is refused — Mendix has nowhere to store a reference in an attribute-typed widget
> property, and writing one anyway fails the build with CE1613 *"The selected attribute
> … no longer exists"*. To **show** a value from the associated object, traverse the
> reference (`attribute: Order_Customer/Name`); to **filter** by it, use the mode above.

### NAVIGATIONLIST Widget

Create a menu with action items:

```sql
navigationlist navName {
  item itemEdit (caption: 'Edit', action: show_page Module.EditPage(entity: $EntityParameter))
  item itemDelete (caption: 'Delete', action: delete)
  item itemBack (caption: 'Back', action: close_page)
}
```

### SNIPPETCALL Widget

Embed a reusable snippet:

```sql
-- Simple snippet call
snippetcall snippetName (snippet: Module.SnippetName)

-- With parameters
snippetcall actions (snippet: Module.EntityActions, params: {entity: $Param})
```

**A parameter satisfied by the enclosing data context takes NO mapping.** Mendix
has no variable meaning "the surrounding context" — the correct model is the
*absence* of a mapping, which is what Studio Pro writes. So inside a DataView
bound to the parameter's entity, just omit `Params:`:

```sql
dataview dvOrder (datasource: $Order) {
  snippetcall scActions (snippet: MyModule.OrderActions)   -- parameter comes from dvOrder
}
```

`params: {Order: $currentObject}` means the same thing and produces the same
(empty) mapping. Naming a real page parameter or variable produces a real
mapping, as expected:

```sql
snippetcall scActions (snippet: MyModule.OrderActions, params: {Order: $Order})
```

Omitting `Params:` where the context is a *different* entity is still an error —
nothing there can satisfy the parameter.

### IMAGE / STATICIMAGE / DYNAMICIMAGE Widgets

Display images on pages:

```sql
-- Image with dimensions (responsive by default)
image imgLogo (width: 200, height: 100)
staticimage imgBanner (width: 400, height: 120)

-- Dynamic image (from entity data source, e.g. inside a DataView)
dynamicimage imgProduct (width: 300, height: 200)

-- Image without explicit dimensions
image imgIcon
```

**Properties:** `width: integer`, `height: integer`, `AlternativeText: 'text'`, `WidthUnit: pixels | percentage | auto`, `HeightUnit: pixels | percentage | auto`, `Responsive: true | false`, `DisplayAs: fullImage | thumbnail | icon`, `class: 'css'`, `style: 'css'`

#### Setting Image Source (PLUGGABLEWIDGET syntax)

The IMAGE shorthand creates a pluggable Image widget. For advanced properties like image source, use PLUGGABLEWIDGET syntax:

| Mode | Property | Use Case |
|------|----------|----------|
| `datasource: image` | `imageObject` | Dynamic image from entity (default) |
| `datasource: imageUrl` | `imageUrl: 'path'` | Static image from URL or file path |
| `datasource: icon` | `imageIcon` | Icon-based image |

```sql
-- Static image from file (logos, branding)
pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
  datasource: imageUrl,
  imageUrl: 'img/logo.svg',
  widthUnit: pixels, width: 48,
  heightUnit: pixels, height: 48
)

-- Update existing IMAGE via ALTER PAGE
alter page Mod.Home {
  replace imgLogo with {
    pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
      datasource: imageUrl, imageUrl: 'img/logo_dark.svg',
      widthUnit: pixels, width: 48, heightUnit: pixels, height: 48
    )
  }
};
```

For theme images, use paths relative to `theme/web/` (e.g., `img/logo.svg` → `theme/web/img/logo.svg`).

**A per-row image URL comes from the entity, two ways.** `imageUrl` is a text
template, so it takes either spelling:

```sql
-- named placeholder: shortest form for a single attribute
pluggablewidget 'com.mendix.widget.web.image.Image' cardImage (
  datasource: imageUrl, imageUrl: '{PictureUrl}'
)

-- numbered placeholders + contentparams: needed for several values, or a format block
pluggablewidget 'com.mendix.widget.web.image.Image' cardImage (
  datasource: imageUrl,
  imageUrl: '{1}/{2}', contentparams: [{1} = BaseUrl, {2} = PictureUrl]
)
```

Every `{N}` must have a matching parameter — Mendix rejects a shortfall with
`CE0720` ("place holder index N is greater than …, the number of parameter(s)").
Parameters with no `{N}` to fill are reported by MDL-WIDGET21 rather than
dropped in silence.

### Buttons Have Visibility, Not Editability

`editable:` only exists on **input** widgets — Mendix gives exactly eleven page
widgets an editability setting (textbox, textarea, checkbox, datepicker,
dropdown, radiobuttons, referenceselector, inputreferencesetselector,
filemanager, imageuploader, and dataview). No button of any kind has one, so
`editable:` on a button is reported by MDL-WIDGET20 and does nothing.

To disable a button conditionally, hide it instead — buttons do support
conditional visibility — or put the condition in the microflow it calls:

```sql
actionbutton btnSubmit (
  caption: 'Submit', action: microflow Mod.ACT_Submit,
  visible: [$currentObject/Status = Mod.Status.Draft]
)
```

### CONTAINER / CUSTOMCONTAINER Widgets

Generic container for grouping widgets. `customcontainer` is an alias for `container` (both map to `Forms$DivContainer`):

```sql
-- Basic container with CSS class
container card1 (class: 'card', style: 'padding: 16px;') {
  dynamictext title (content: 'Card Title', rendermode: H4)
  dynamictext body (content: 'Card body content')
}

-- Container with design properties
container spaced1 (designproperties: ['Spacing top': 'Large', 'Full width': on]) {
  dynamictext text1 (content: 'Spaced full-width content')
}

-- Nested containers with combined styling
customcontainer outer1 (class: 'section') {
  container inner1 (class: 'card', designproperties: ['Spacing top': 'Medium']) {
    dynamictext text1 (content: 'Nested content')
  }
}
```

**Clickable container (On click action).** A container can trigger an action when
clicked — use `OnClick:` (or the equivalent `Action:` keyword) with any client
action (`microflow`, `nanoflow`, `show_page`, `save_changes`, …):

```sql
container card1 (OnClick: microflow MyModule.ACT_OpenDetails, class: 'clickable-card') {
  dynamictext title (content: 'Open details')
}
```

This maps to the Mendix Container's **On click** event (`Forms$DivContainer.OnClickAction`).
A container with no `OnClick:`/`Action:` is non-clickable (a no-op action), exactly as in Studio Pro.

**Prefer a clickable container over an `actionbutton` when the trigger needs child
widgets or a context argument.** An `actionbutton` can't contain child widgets and
maps only page-context object parameters — so a tile showing a digit over a label,
or a card that opens a specific object, is best built as a `container` with
`OnClick:`. The container's `OnClick:` takes the same `(Param: …)` argument syntax
as an `actionbutton`'s `action:`:

```sql
-- Rich, parameterised trigger: a card that opens the object it represents
container tileCard (OnClick: microflow MyModule.ACT_Open(Item: $currentObject), class: 'tile') {
  dynamictext tileValue (content: '4')
  dynamictext tileLabel (content: '4 LEFT', class: 'tile-label')
}
```

Note the Mendix limit this works around: a button (or `OnClick`) can map **object**
parameters from the page context but **cannot pass a literal argument**. To vary a
literal per trigger (e.g. a 1–9 number pad), make one real microflow and a thin
wrapper per value (`ACT_Set1`…`ACT_Set9`), each calling the shared implementation.

### FOOTER Widget

Container for form action buttons:

```sql
footer footerName {
  actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
  actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
}
```

### HEADER Widget

Container for header content:

```sql
header headerName {
  dynamictext title (content: 'Form Title', rendermode: H3)
}
```

### CONTROLBAR Widget

Control bar for data widgets:

```sql
controlbar controlBar1 {
  actionbutton btnNew (caption: 'New', action: create_object Module.Entity then show_page Module.EditPage, buttonstyle: primary)
}
```

### Charts (Charts.mpk — ColumnChart / BarChart / AreaChart / PieChart)

Charts are pluggable widgets whose data lives in one or more `series` object-list
items. Each series binds a datasource (an OQL **view entity** is the natural feed —
one row per category) and picks X/Y attributes on that datasource. Requires the
Charts widget installed (`widgets/Charts.mpk`); run `mxcli widget init -p app.mpr`.

```sql
-- Aggregated view entity = the chart's data source
create view entity Sales.ByRegion (Region: string(100), Total: decimal) as
  select s.Region as Region, sum(s.Amount) as Total
  from Sales.Sale as s group by s.Region;

create page Sales.Dashboard (Title: 'Revenue', Layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.columnchart.ColumnChart' revenueChart {
    series sRevenue (
      dataSet: static,
      DataSource: database from Sales.ByRegion,   -- or: staticDataSource: database from Sales.ByRegion
      StaticXAttribute: Region,
      StaticYAttribute: Total,
      StaticName: 'Revenue'
    )
  }
}
```

Notes:
- `DataSource:` inside a series is a friendly alias for `staticDataSource:` /
  `dynamicDataSource:` (chosen by `dataSet`); either form works.
- X/Y attributes resolve against the **series' own** datasource entity, not the page.
- Add multiple `series ( ... )` blocks for multi-series charts. BarChart/AreaChart/
  PieChart use the same `series` shape.
- **CE0463 at `mx check`**: charts can report "widget definition changed" from
  widget-version drift (embedded template vs the installed `Charts.mpk`), even for a
  chart with no series. Clear it with **`mxcli docker check`/`build`**, which normalize
  the widgets and preserve your storage format.
  **Do NOT run bare `mx update-widgets` on an MPRv2 project** (one with an
  `mprcontents/` folder — everything `mxcli new` creates). `mx update-widgets` rewrites
  the project into the single-file v1 format and **deletes `mprcontents/`**, which
  corrupts a git working tree, breaks a running `mxcli run --local` loop, and can leave
  Studio Pro unable to open the project. `mxcli docker check`/`build` snapshot and
  restore the v2 files around the normalization, so they are safe; raw
  `mx update-widgets` is only safe on a v1 project (or on a throwaway copy used purely
  for diagnosis).
- LineChart/BubbleChart/HeatMap/TimeSeries are **also MDL-authorable** (via the
  `line`/`scalecolor` object-lists) — see
  `mdl-examples/doctype-tests/34-chart-widget-examples.mdl` for working examples.
  (They author on the modelsdk engine.)
- **Slider / RangeSlider**: set `showTooltip: false`. The tooltip calls React's
  removed `findDOMNode` on Mendix 11, throwing "Could not render widget" on drag — a
  runtime crash `mx check` cannot catch (only a running build does). See
  `atlas-design` for the full runtime-verification rule.
