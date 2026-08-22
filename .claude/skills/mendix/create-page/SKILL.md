---
name: create-page
description: "CREATE PAGE syntax reference — parameters, variables, layouts, and the full widget vocabulary. Use before writing any CREATE PAGE statement, or when a widget's property spelling needs checking."
---

# CREATE PAGE - MDL Syntax Guide

## Reference files

`SKILL.md` covers page structure, the syntax, and what does not work. The bulk is
next door:

- [`reference/widgets.md`](reference/widgets.md) — the widget catalogue: every
  supported widget with its properties and the exact MDL spelling. **Look a widget
  up here before writing it**; guessing a property name is the most common way a
  page fails to build.
- [`reference/examples.md`](reference/examples.md) — complete pages, end to end,
  to copy and adapt rather than assemble from parts.

For the widgets *this project actually has* — including any marketplace or custom
ones — read the generated `widgets` skill instead
(`.ai-context/skills/widgets/SKILL.md`).

## Overview
Guide for writing CREATE PAGE statements in Mendix Definition Language (MDL).

## Syntax

```sql
create [or replace] page Module.PageName
(
  [params: { $ParamName: Module.EntityType | PrimitiveType, ... },]
  [variables: { $varName: DataType = 'defaultExpression', ... },]
  title: 'Page Title',
  layout: Module.LayoutName,
  [url: 'page-url',]
  [folder: 'FolderPath',]
  [PopupWidth: 800, PopupHeight: 480, PopupResizable: true,]
  [Class: 'css-class', Style: 'css: rule']
)
{
  -- Widget definitions using explicit properties
}
```

**Pop-up dimensions** (`PopupWidth` / `PopupHeight` / `PopupResizable`) apply when the
page is opened in a pop-up. They are optional — omitting them uses the Mendix defaults
(600 × 600, not resizable). Unlike the other header keywords, these property names are
**case-sensitive** and must be written exactly as shown. They can also be changed later
with `alter page … { set PopupWidth = …; }` (see the alter-page skill).

**Page CSS class / style** (`Class` / `Style`) set the page's Appearance — a CSS class
and inline style applied to the whole page (e.g. `Class: 'container-fluid bg-light'`).
Both are optional and can be changed later with `alter page … { set Class = '…'; }`.

**Page Variables**: Local variables at the page level for use in expressions (e.g., column visibility).
- DataType: `boolean`, `string`, `integer`, `decimal`, `datetime`
- Default value: Mendix expression in single quotes
- Referenced in expressions as `$varName`
- Use for DataGrid2 column `visible:` (which hides/shows entire column, NOT per-row)

### Key Syntax Elements

| Element | Syntax | Example |
|---------|--------|---------|
| Properties | `(key: value, ...)` | `(title: 'Edit', layout: Atlas_Core.Atlas_Default)` |
| Widget name | Required after type | `textbox txtName (...)` |
| Attribute binding | `attribute: AttrName` | `textbox txt (label: 'Name', attribute: Name)` |
| Variable binding | `datasource: $Var` | `dataview dv (datasource: $Product) { ... }` |
| Action binding | `action: type` | `actionbutton btn (caption: 'Save', action: save_changes)` |
| Database source | `datasource: database entity` | `datagrid dg (datasource: database Module.Entity)` |
| Selection binding | `datasource: selection widget` | `dataview dv (datasource: selection galleryList)` |
| CSS class | `class: 'classes'` | `container c (class: 'card mx-spacing-top-large')` |
| Inline style | `style: 'css'` | `container c (style: 'padding: 16px;')` |
| Design properties | `designproperties: [...]` | `container c (designproperties: ['Spacing top': 'Large', 'full width': on])` |

### FOLDER Option

Place pages in folders for better organization:

```sql
create page MyModule.CustomerEdit
(
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout,
  folder: 'Customers'
)
{
  -- widgets
}

-- Nested folders (created automatically if they don't exist)
create page MyModule.OrderDetail
(
  title: 'Order Details',
  layout: Atlas_Core.Atlas_Default,
  folder: 'Orders/Details'
)
{
  -- widgets
}
```

### Styling: Class, Style, and DesignProperties

Three styling mechanisms can be applied to any widget:

**CSS Class** — Atlas UI utility classes or custom CSS classes:
```sql
container c (class: 'card mx-spacing-top-large') { ... }
actionbutton btn (caption: 'Save', class: 'btn-lg')
```

**Inline Style** — One-off CSS styles (use sparingly):
```sql
container c (style: 'background-color: #f8f9fa; padding: 16px;') { ... }
```

> **Warning:** Do NOT use `style` directly on DYNAMICTEXT widgets — it crashes MxBuild with a NullReferenceException. Wrap the DYNAMICTEXT in a styled CONTAINER instead.

**Design Properties** — Atlas UI structured properties (spacing, colors, toggles):
```sql
-- Option property: 'Key': 'Value'
container c (designproperties: ['Spacing top': 'Large', 'Background color': 'Brand Primary']) { ... }

-- Toggle property: 'Key': ON (enabled) or OFF (disabled/omitted)
container c (designproperties: ['Full width': on]) { ... }

-- Multiple types combined
actionbutton btn (caption: 'Save', designproperties: ['Size': 'Large', 'Full width': on])
```

**Dynamic Classes** — a Mendix expression evaluated at runtime that returns a
class list (applied on top of the static `class`). Root attributes in
`$currentObject` and escape single quotes by doubling them (`''`):
```sql
dynamictext ovChip (
  content: 'chip',
  class: 'ss-chip',
  dynamicclasses: 'if $currentObject/VesselClass = Mod.BoatClass.Astute then ''ss-chip--astute'' else '''''
)
```

**All can be combined on a single widget:**
```sql
container ctnHero (
  class: 'card',
  style: 'border-left: 4px solid #264AE5;',
  dynamicclasses: 'if $currentObject/Featured then ''is-featured'' else ''''',
  designproperties: ['Spacing top': 'Large', 'Full width': on]
) {
  dynamictext txtTitle (content: 'Styled Container', rendermode: H3)
}
```

> `mxcli check` **warns** (MDL-WIDGET07) when a built-in widget carries an
> unrecognized property (a typo, or a property mxcli doesn't persist) — it would
> otherwise be silently dropped on write. It is a warning, not an error, so the
> check still passes; fix the spelling or remove the property.

## Basic Examples

### Simple Page with Title

```sql
create page MyModule.HomePage
(
  title: 'Home Page',
  layout: Atlas_Core.Atlas_Default
)
{
  dynamictext welcomeText (content: 'Welcome to My App', rendermode: H1)
}
```

### Page with Multiple Widgets

```sql
create page MyModule.CustomerPage
(
  title: 'Customer Details',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext heading (content: 'Customer Information', rendermode: H2)
      }
    }
    row row2 {
      column col2a (desktopwidth: 6) {
        actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      }
      column col2b (desktopwidth: 6) {
        actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
      }
    }
  }
}
```

### Layout Placeholders (multiple content areas)

By default all top-level widgets bind to the layout's **Main** placeholder. When a
layout has more than one placeholder (e.g. Main + a sidebar/topbar), use a
`placeholder <Name> { … }` block to assign widgets to a specific placeholder. Bare
widgets still bind to Main.

```sql
-- Atlas_Core.Atlas_SideBar has two placeholders: Main and Topbar
create page MyModule.Dashboard (title: 'Dashboard', layout: Atlas_Core.Atlas_SideBar)
{
  placeholder Main {
    dynamictext lblMain (content: 'Main content area')
  }
  placeholder Topbar {
    dynamictext lblTop (content: 'Top bar content')
  }
}
```

Notes:
- The placeholder name must match a placeholder defined in the layout (e.g. `Main`,
  `Right`, `Topbar`, `Content` — depends on the layout). An unknown name fails `mx check`.
- Keyword-like names (`Right`, `Left`, `Content`) are accepted.
- `describe page` emits `placeholder` blocks for multi-placeholder pages so they round-trip.

## Modifying Existing Pages

To make targeted changes to an existing page (change a label, add a field, remove a widget), use `alter page` instead of `create or replace page`. ALTER PAGE modifies the widget tree in-place, preserving properties that MDL doesn't model.

```sql
-- Change a button caption and add a field
alter page Module.Customer_Edit {
  set caption = 'Save & Close' on btnSave;
  insert after txtEmail {
    textbox txtPhone (label: 'Phone', attribute: Phone)
  }
};
```

See the dedicated skill file: [ALTER PAGE/SNIPPET](../alter-page/SKILL.md)

## Conditional Visibility and Editability

Any widget (including CONTAINER) can have conditional visibility. Input widgets can also have conditional editability. Use bracket syntax `[expression]`:

```sql
-- Conditionally visible widget (boolean attribute)
textbox txtName (label: 'Name', attribute: Name, visible: [IsActive])

-- Conditionally visible container
container ctnDetails (visible: [Name != '']) { dynamictext t (content: '...') }

-- Conditionally editable input (boolean)
textbox txtStatus (label: 'Status', attribute: status, editable: [CanEdit])

-- Enum comparison: use the QUALIFIED enum value. Attributes are rooted for you,
-- but a bare enum VALUE would be treated as an attribute — always qualify it.
textbox txtNotes (label: 'Notes', attribute: Notes,
  visible: [Status = MES.EquipmentStatus.Running])

-- Combined
textbox txtEmail (label: 'Email', attribute: Email,
  visible: [ShowEmail],
  editable: [CanEdit])

-- Static values still work
textbox txtReadOnly (label: 'Read Only', attribute: Name, editable: Never)
textbox txtHidden (label: 'Hidden', attribute: Name, visible: false)

-- A quoted-string expression is also accepted (CREATE and ALTER). Unlike the
-- bracket form, it is NOT auto-rooted — write $currentObject/ yourself.
dynamictext ovChip (content: 'chip', visible: '$currentObject/Name != empty')

-- Function calls work in the bracket form, including functions whose name is
-- also an MDL keyword (trim, length, find). Arguments are rooted like any other
-- reference.
dynamictext tTrim (content: 'x', visible: [trim($currentObject/Slug) != ''])
textbox txtSlug (label: 'Slug', attribute: Slug, editable: [length(Slug) > 0])
```

> **`visible:`/`editable:` is a Mendix *expression*, not XPath** — a different
> function set from a datasource `where` clause, even though both use `[ ... ]`:
>
> | | `visible:` / `editable:` (client expression) | `where [ … ]` (XPath) |
> |---|---|---|
> | String tests | `trim()`, `length()`, `toUpperCase()`, `find()`, `contains()` | `contains()`, `starts-with()`, `ends-with()`, `string-length()` |
> | `length()` | character count | number of elements in a list |
> | Emptiness | `$currentObject/X != ''` / `!= empty` | `[X = empty]` or `[X = NULL]` — a **keyword**, never `empty(…)` |
> | Aggregates | not available | `count()`/`avg()`/`min()`/`max()`/`sum()` are Java-API-only |
>
> mxcli's grammar accepts any function name in both and lets MxBuild adjudicate,
> so a wrong-context call surfaces as **CE0117** "Error(s) in expression" at
> build rather than as a parse error. See the Mendix reference guide:
> [XPath constraint functions](https://docs.mendix.com/refguide/xpath-constraint-functions/),
> [XPath keywords](https://docs.mendix.com/refguide/xpath-keywords-and-system-variables/).

> **An unparseable conditional is an error, not a silent drop.** If the
> expression inside `visible: [ ... ]` / `editable: [ ... ]` can't be parsed, the
> property has nowhere to go and would vanish on write — leaving the widget
> unconditionally visible/editable, which looks identical to a specificity bug in
> the running app. `mxcli check` reports this as **MDL-WIDGET19** and fails the
> command instead. Until v0.16.x, `trim(…)` and `length(…)` hit exactly this path
> and disappeared without a word (issue #852).

> **Attribute rooting is automatic** — a bare attribute in a widget
> visibility/editability expression (`[Name != '']`, `[IsActive]`) is rooted in the
> widget data context as `$currentObject/Name != ''` for you, so it no longer
> triggers CE0117. Paths you write with an explicit `$currentObject/…` or `$Param/…`
> prefix pass through unchanged.
>
> **Enum comparison differs by context:**
> - **Widget visibility/editability expression** (per-object): qualified enum value —
>   `[Status = MES.EquipmentStatus.Running]` (the `Status` attribute is rooted for you;
>   the *value* must stay qualified, or it would be mistaken for an attribute).
> - **XPath datasource constraint** (`where […]`): the string key works —
>   `where [Status = 'Running']` (see [xpath-constraints](../xpath-constraints/SKILL.md)).
> - **Microflow expression**: qualified value —
>   `$obj/Status = MES.EquipmentStatus.Running` (see [write-microflows](../write-microflows/SKILL.md)).
>
> Widget-level visibility does **not** apply to **DataGrid2 column** `visible:` (next
> section), which hides/shows the whole column and must use page variables.

## Known Limitations

The following features are NOT implemented in mxcli and require manual configuration in Studio Pro:

| Feature | Workaround |
|---------|------------|
| Nested dataviews filtering by parent | Use microflow datasource or configure in Studio Pro |
| Complex conditional visibility | Configure visibility rules in Studio Pro |
| Widget-level security | Configure access rules in Studio Pro |

### Runtime Pitfalls

> **Empty CONTAINER crashes at runtime.** A CONTAINER with no child widgets compiles and builds successfully but crashes when the page loads with "Did not expect an argument to be undefined". Always include at least one child widget:
> ```sql
> -- Wrong: crashes at runtime
> CONTAINER spacer1 (Style: 'height: 6px;')
>
> -- Correct: include a child (even a space)
> CONTAINER spacer1 (Style: 'height: 6px;') {
>   DYNAMICTEXT spacerText (Content: ' ', RenderMode: Paragraph)
> }
> ```

> **`content: ''` (empty string) fails MxBuild.** An empty Content on DYNAMICTEXT causes a misleading error: "Place holder index 1 is greater than 0, the number of parameter(s)." Use a single space instead:
> ```sql
> -- Wrong: MxBuild error
> DYNAMICTEXT spacer (Content: '')
>
> -- Correct: use a space
> DYNAMICTEXT spacer (Content: ' ')
> ```

### Binding across modules and to audit members

An attribute path may cross module boundaries, including into the platform's
`System` module — the association does not need to live in the same module as
the entity it targets:

```sql
create association IT.Issue_Assignee from IT.Issue to System.User;

DATAVIEW dv (DataSource: $Issue) {
  DYNAMICTEXT txtAssignee (Attribute: Issue_Assignee/Name)   -- into System
  DYNAMICTEXT txtApprover (Attribute: Issue_Approver/Name)   -- into another module
}
```

A bare association name is qualified with the module of the entity the widget
sits on. On a ComboBox that matters: its `DataSource:` is the *option list*, but
`Association:` names a reference on the containing entity, so
`Association: Issue_Assignee` resolves against the dataview's entity, not the
option list's module.

Audit members declared with the `Auto*` pseudo-types bind under the name you
declared:

```sql
create or modify persistent entity IT.Issue ( CreatedDate: AutoCreatedDate );
DYNAMICTEXT txtCreated (Attribute: CreatedDate)   -- also accepts createdDate
```

**Script Execution Note:** Script execution stops on the first error. If a page fails to create (e.g., invalid widget syntax), earlier statements in the script will have already been committed. Plan scripts with uncertain syntax in phases.

## Tips

1. **OR REPLACE**: Use to recreate existing pages
2. **Widget Names**: Required - use descriptive camelCase names
3. **Layout Requirement**: Layout must exist in the project
4. **Nesting**: Use `{ }` blocks for all widget children
5. **Properties**: Use `(key: value)` syntax for all widget properties
6. **Bindings**: Use `attribute:` for attributes, `datasource:` for data, `action:` for buttons

## Related Commands

- `alter page Module.PageName { ... }` - Modify page widgets in-place (SET, INSERT, DROP, REPLACE)
- `alter snippet Module.SnippetName { ... }` - Modify snippet widgets in-place
- `describe page Module.PageName` - View page source in MDL format (shows Class, Style, DesignProperties)
- `describe snippet Module.SnippetName` - View snippet source in MDL format
- `show pages [in module]` - List all pages
- `show widgets [where ...] [in module]` - Discover widgets across pages/snippets
- `update widgets set ... where ... [dry run]` - Bulk update widget properties (see below)
- `drop page Module.PageName` - Delete a page

### Bulk Widget Updates

Use `update widgets` to change properties across many widgets at once:

```sql
-- Preview changes first (always use DRY RUN)
update widgets set 'Class' = 'card' where widgettype like '%Container%' in MyModule dry run;

-- Apply changes
update widgets set 'showLabel' = false where widgettype like '%combobox%';

-- Multiple properties
update widgets set 'Class' = 'btn-lg', 'Style' = 'margin-top: 8px;' where widgettype like '%ActionButton%';
```

## PLUGGABLEWIDGET Escape Hatch

All shorthand widgets (IMAGE, COMBOBOX, GALLERY, DATAGRID, etc.) are pluggable widgets under the hood. When the shorthand doesn't expose a property you need, use `pluggablewidget 'widget.id' name (properties)` for full access to all widget properties.

```sql
-- Shorthand (common properties only)
image imgLogo (width: 48, height: 48)

-- Full PLUGGABLEWIDGET syntax (all properties available)
pluggablewidget 'com.mendix.widget.web.image.Image' imgLogo (
  datasource: imageUrl, imageUrl: 'img/logo.svg',
  widthUnit: pixels, width: 48, heightUnit: pixels, height: 48
)
```

The project's own widgets are documented as a skill: read
`.ai-context/skills/widgets/SKILL.md` (also at `.claude/skills/widgets/SKILL.md`)
for the index, then the per-widget file for the one you are placing — it carries
the full property table with enumeration values, nested object properties, child
slots and object lists.

`mxcli widget docs -p app.mpr` regenerates it (so does `refresh catalog`), and
`mxcli widget describe <name> -p app.mpr` reads the same data live from the
`.mpk` when a widget has been upgraded since.

## See Also

- [Overview Pages](../overview-pages/SKILL.md) - CRUD page patterns
- [Master-Detail Pages](../master-detail-pages/SKILL.md) - Selection binding pattern
