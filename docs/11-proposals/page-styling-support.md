---
title: Add Styling Support to MDL Pages
status: partial
---

# Proposal: Add Styling Support to MDL Pages

## Current State

Mendix has four styling mechanisms on every widget, all stored in a BSON `Forms$Appearance` object:

| Mechanism | BSON Field | Type | Example |
|---|---|---|---|
| CSS classes | `class` | string | `"btn-lg mx-spacing-top-large"` |
| Inline CSS | `style` | string | `"color: red; margin: 10px;"` |
| Dynamic classes | `DynamicClasses` | string (XPath) | `"if $currentObject/IsActive then 'highlight' else ''"` |
| Design properties | `designproperties` | array of typed tokens | Atlas UI spacing, colors, toggles |

Additionally, some widgets have their own style enums (e.g. `buttonstyle: primary` on buttons — already supported as a separate keyword).

**What exists today:** The serializer (`serializeAppearance()` in `writer_widgets.go`) writes the `Forms$Appearance` BSON structure but always with empty values. `BaseWidget` has `class` and `style` fields in the Go struct but they're never populated from MDL. DESCRIBE doesn't output any styling. The `buttonstyle:` keyword was renamed from `style:` to free up `style:` for CSS inline styling.

## Proposal: Three Phases

### Phase 1 — `class` and `style` properties (highest value) ✅ DONE

`class` and `style` are standard widget properties in the V3 syntax:

```sql
textbox txtName (label: 'Name', attribute: Name, class: 'form-control-lg mx-spacing-top-large')
container ctn1 (class: 'card', style: 'padding: 16px; border-radius: 8px;') {
  dynamictext txt1 (content: '{1}', params: [FullName], class: 'text-primary h3')
}
actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary, class: 'btn-block')
```

This is the most common way developers style Mendix apps and maps directly to existing fields on `BaseWidget`. Implementation:

- **Grammar**: `class` and `style` keywords in `widgetPropertyV3` rule (MDLParser.g4)
- **AST**: `GetClass()` and `GetStyle()` helpers on `WidgetV3` (ast_page_v3.go)
- **Visitor**: Extracts Class/Style from parsed string literals (visitor_page_v3.go)
- **Builder**: `applyWidgetAppearance()` sets Class/Style on any widget via `SetAppearance()` (cmd_pages_builder_v3.go)
- **Serializer**: `serializeAppearance(class, style)` passes values through to BSON (writer_widgets.go + all callers)
- **Describe Parse**: Extracts Class/Style from `Appearance` BSON (cmd_pages_describe_parse.go)
- **Describe Output**: Emits `class:` and `style:` when non-empty via `appendAppearanceProps()` (cmd_pages_describe_output.go)
- **Wireframe**: Class/Style fields on `wireframeNode` (cmd_page_wireframe.go)

### Phase 2 — `DynamicClasses` (conditional styling)

```sql
container ctn1 (
  class: 'card',
  DynamicClasses: "if $currentObject/status = 'Error' then 'alert-danger' else 'alert-info'"
) {
  ...
}
```

This is an XPath expression string. Implementation is similar to Phase 1 — just another string property — but it's less commonly used and the XPath expressions can be complex.

### Phase 3 — Design Properties (Atlas UI tokens)

Design properties are structured arrays of typed key-value pairs set via Atlas UI's design system.

#### Where Design Property Definitions Live

Design property **definitions** (what properties are available per widget type) are NOT in the MPR. They live in the project's `themesource` folder:

```
<project-root>/themesource/<module>/<platform>/design-properties.json
```

The primary file for most Mendix apps:
```
themesource/atlas_core/web/design-properties.json      -- Web platform
themesource/atlas_core/native/design-properties.json    -- Native mobile
```

Any module can add its own design properties at `themesource/<YourModule>/web/design-properties.json`. Multiple modules' properties are merged at load time.

#### Design Property Definition Format

The `design-properties.json` is a JSON object where keys are widget type names and values are arrays of property definitions:

```json
{
    "widget": [
        { "name": "Spacing top", "type": "dropdown", "description": "...",
          "options": [
            { "name": "none", "class": "spacing-outer-top-none" },
            { "name": "Small", "class": "spacing-outer-top" },
            { "name": "Large", "class": "spacing-outer-top-large" }
          ]
        },
        { "name": "Hide on phone", "type": "Toggle", "description": "...",
          "class": "hide-phone" }
    ],
    "DivContainer": [
        { "name": "Align content", "type": "dropdown", "description": "...",
          "options": [
            { "name": "left align as a row", "class": "row-left" },
            { "name": "Center align as a row", "class": "row-center" }
          ]
        },
        { "name": "background color", "type": "dropdown", "description": "...",
          "options": [
            { "name": "Brand primary", "class": "background-primary" },
            { "name": "Brand Inverse", "class": "background-inverse" }
          ]
        }
    ],
    "button": [
        { "name": "Size", "type": "dropdown", ... },
        { "name": "full width", "type": "Toggle", "class": "btn-block", ... },
        { "name": "Border", "type": "Toggle", "class": "btn-bordered", ... }
    ],
    "com.mendix.widget.web.accordion.Accordion": [ ... ]
}
```

**Widget type keys**: Built-in widgets use Model SDK class names (`DivContainer`, `button`, `datagrid`, `listview`, `dynamictext`). Pluggable widgets use their widget ID (`com.mendix.widget.web.accordion.Accordion`). The `widget` key defines properties that apply to ALL widgets (inherited by all subtypes).

**Five property types**:

| Type | Description | Value stored in MPR |
|------|-------------|---------------------|
| `Toggle` | On/off CSS class | `ToggleDesignPropertyValue` (presence = on) |
| `dropdown` | Single-select option list | `OptionDesignPropertyValue` (option name string) |
| `ColorPicker` | Dropdown with color preview | `OptionDesignPropertyValue` (same storage as Dropdown) |
| `ToggleButtonGroup` | Related options as buttons | `OptionDesignPropertyValue` (single) or `CompoundDesignPropertyValue` (multi) |
| `Spacing` | Margin/padding in 4 directions | `CompoundDesignPropertyValue` (nested properties) |

#### BSON Storage Format

Design property **values** are stored per widget in `Appearance.DesignProperties` as an array of `Forms$DesignPropertyValue` objects:

```bson
"Appearance": {
  "$type": "Forms$Appearance",
  "class": "",
  "style": "",
  "DynamicClasses": "",
  "designproperties": [
    2,
    {
      "$type": "Forms$DesignPropertyValue",
      "key": "Spacing top",
      "value": {
        "$type": "Forms$OptionDesignPropertyValue",
        "Option": "Large"
      }
    },
    {
      "$type": "Forms$DesignPropertyValue",
      "key": "full width",
      "value": {
        "$type": "Forms$ToggleDesignPropertyValue"
      }
    },
    {
      "$type": "Forms$DesignPropertyValue",
      "key": "Spacing",
      "value": {
        "$type": "Forms$CompoundDesignPropertyValue",
        "properties": [
          2,
          {
            "$type": "Forms$DesignPropertyValue",
            "key": "Top",
            "value": {
              "$type": "Forms$OptionDesignPropertyValue",
              "Option": "M"
            }
          }
        ]
      }
    }
  ]
}
```

**Value type hierarchy**:
- `Forms$ToggleDesignPropertyValue` — empty struct, presence means enabled
- `Forms$OptionDesignPropertyValue` — has `Option` string (the selected option name)
- `Forms$CustomDesignPropertyValue` — has `value` string (arbitrary text)
- `Forms$CompoundDesignPropertyValue` — has nested `properties` array of `DesignPropertyValue`

#### Inline Syntax in CREATE PAGE

Design properties can be included in `create page` using an explicit `designproperties:` array on any widget. This keeps them clearly separated from built-in widget properties (Class, Style, Label, Binds, etc.):

```sql
create page MyModule.Customer_Edit
(
  params: { $Customer: MyModule.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvCustomer (datasource: $Customer) {
    container ctn1 (
      class: 'card',
      style: 'padding: 16px;',
      designproperties: [
        'Spacing top': 'Large',
        'Background color': 'Brand Primary',
        'Hide on phone': on
      ]
    ) {
      textbox txtName (label: 'Name', attribute: Name)
      textbox txtEmail (label: 'Email', attribute: Email)
    }

    footer footer1 {
      actionbutton btnSave (
        caption: 'Save',
        action: save_changes,
        buttonstyle: primary,
        designproperties: ['Full width': on, 'Size': 'Large']
      )
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}
```

**Syntax rules**:
- `designproperties:` is followed by `[` ... `]` (array brackets, same pattern as `params:`)
- Each entry is `STRING_LITERAL COLON (STRING_LITERAL | on | off)` — quoted key, colon, value
- Toggle properties use `on` / `off` keywords
- Dropdown/ColorPicker/ToggleButtonGroup properties use quoted option name strings
- Entries are comma-separated; single-line for few properties, multi-line for many
- The array is optional — omitting `designproperties:` means no design properties (same as today)

**Why explicit `designproperties:` wrapper** (not bare quoted keys in the property block):
1. **No ambiguity** — a design property named `style` or `label` won't collide with built-in keywords
2. **Cleaner grammar** — one rule for `designproperties COLON LBRACKET ... RBRACKET` instead of a catch-all
3. **Explicit intent** — reading the MDL you immediately know these are theme-driven, not built-in settings
4. **Simpler builder logic** — everything inside `designproperties:` needs theme registry lookup; everything outside doesn't

**Serialization dependency**: The builder must read the project's `themesource/*/web/design-properties.json` to determine the BSON value type for each property. For example, `'full width': on` serializes as `Forms$ToggleDesignPropertyValue` (empty struct), while `'Size': 'Large'` serializes as `Forms$OptionDesignPropertyValue` with `Option: "Large"`. Without the theme registry, the builder cannot distinguish between property types.

**DESCRIBE roundtrip**: `describe page` outputs `designproperties: [...]` on any widget that has non-empty design properties, making the output re-executable.

#### Proposed Commands — Styling Fragments

In addition to inline syntax in `create page`, design properties can be managed through **fragment-style commands** that operate on individual widgets within existing pages. This is useful for modifying styling without rewriting entire pages.

##### 1. Discover Available Design Properties

Read the `design-properties.json` from the project's themesource and show what properties are available for a given widget type:

```sql
-- Show all design properties available for Container widgets
show design properties for container;

-- Output:
-- From: Widget (inherited)
--   Spacing top          Dropdown    [None, Small, Medium, Large]
--   Spacing bottom       Dropdown    [None, Small, Medium, Large]
--   Hide on phone        Toggle      class: hide-phone
--   Hide on tablet       Toggle      class: hide-tablet
-- From: DivContainer
--   Align content        Dropdown    [Left align as a row, Center align as a row, ...]
--   Background color     Dropdown    [Brand Default, Brand Primary, Brand Inverse, ...]

-- Show available properties for a pluggable widget
show design properties for DATAGRID2;

-- Show available properties for all widget types
show design properties;
```

This requires reading `themesource/*/web/design-properties.json` from the project directory and mapping widget type keys to MDL widget types.

##### 2. Describe Current Styling on a Widget

Show all styling (Class, Style, DynamicClasses, and DesignProperties) for a specific widget on a page:

```sql
-- Show styling for a specific widget on a page
describe styling on page MyModule.CustomerEdit widget btnSave;

-- Output:
-- WIDGET btnSave (ActionButton)
--   Class: 'btn-block'
--   Style: 'margin-top: 8px;'
--   Design Properties:
--     Spacing top: Large
--     Full width: ON

-- Show styling for ALL widgets on a page
describe styling on page MyModule.CustomerEdit;

-- Output (one section per styled widget):
-- WIDGET ctn1 (Container)
--   Class: 'card'
--   Design Properties:
--     Background color: Brand Primary
-- WIDGET btnSave (ActionButton)
--   Class: 'btn-block'
--   Design Properties:
--     Spacing top: Large
```

##### 3. Set Styling on a Single Widget (Fragment Update)

Change styling properties on a single widget without rewriting the entire page:

```sql
-- Set Class and Style on a widget
alter styling on page MyModule.CustomerEdit widget btnSave
  set class = 'btn-block btn-lg',
      style = 'margin-top: 16px;';

-- Set a design property (dropdown/colorpicker selection)
alter styling on page MyModule.CustomerEdit widget ctn1
  set 'Spacing top' = 'Large',
      'Background color' = 'Brand Primary';

-- Toggle a design property on
alter styling on page MyModule.CustomerEdit widget btnSave
  set 'Full width' = on;

-- Toggle a design property off
alter styling on page MyModule.CustomerEdit widget btnSave
  set 'Full width' = off;

-- Clear all design properties on a widget
alter styling on page MyModule.CustomerEdit widget btnSave
  clear design properties;

-- Mixed: set Class, Style, and design properties together
alter styling on page MyModule.CustomerEdit widget ctn1
  set class = 'card custom-card',
      style = 'border-radius: 12px;',
      'Spacing top' = 'Large',
      'Background color' = 'Brand Primary';
```

The `alter styling` command:
1. Opens the page from the MPR
2. Finds the widget by name (searching the widget tree)
3. Reads the current `Appearance` BSON
4. Modifies only the specified properties (preserving others)
5. Writes the modified `Appearance` back

This is a **surgical update** — it doesn't require parsing or rewriting the full page MDL, just modifying one widget's Appearance blob.

##### 4. Bulk Styling Updates (Extension of UPDATE WIDGETS)

The existing `update widgets` command could be extended for styling:

```sql
-- Set a design property on all buttons across a module
update widgets set 'Full width' = on
  where widgettype like '%Button%' in MyModule dry run;

-- Set Class on all containers
update widgets set class = 'card'
  where widgettype = 'DivContainer' in MyModule;
```

#### Widget Type Mapping

The `design-properties.json` keys must be mapped to BSON `$type` values used in the MPR:

| design-properties.json key | BSON $Type | MDL keyword |
|---------------------------|------------|-------------|
| `widget` | *(all)* | *(all)* |
| `DivContainer` | `Forms$DivContainer` | `container` |
| `button` / `actionbutton` | `Forms$actionbutton` | `actionbutton` |
| `datagrid` | `Forms$datagrid` | `datagrid` |
| `listview` | `Forms$listview` | `listview` |
| `dynamictext` | `Forms$dynamictext` | `dynamictext` |
| `StaticImageViewer` | `Forms$StaticImageViewer` | `staticimage` |
| `label` | `Forms$label` | `statictext` |
| `groupbox` | `Forms$groupbox` | `groupbox` |
| `tabcontainer` | `Forms$tabcontainer` | `tabcontainer` |
| Pluggable widget ID | `CustomWidgets$customwidget` | Widget-specific |

#### Implementation Approach

**Step 1: Theme reader** — Parse `themesource/*/web/design-properties.json` files, merge by widget type, and build an in-memory registry of available properties per widget type. This is a prerequisite for both inline syntax and fragment commands.

**Step 2: Grammar + AST + Visitor** — Add `designproperties COLON LBRACKET designPropertyEntry (COMMA designPropertyEntry)* RBRACKET` rule to `widgetPropertyV3` in MDLParser.g4. AST stores design properties as `map[string]string` on `WidgetV3` (key = property name, value = option name or "ON"/"OFF"). Visitor extracts from parse tree.

**Step 3: Builder + Serializer** — In `buildWidgetV3`, read design properties from AST, look up each in the theme registry to determine the BSON value type, and pass to a new `serializeDesignProperties()` function that builds the `Forms$DesignPropertyValue` array inside `Appearance`.

**Step 4: DESCRIBE Parse + Output** — Parse `designproperties` array from Appearance BSON, store on `rawWidget`, and emit `designproperties: [...]` in MDL output when non-empty.

**Step 5: DESCRIBE STYLING** — Dedicated command to show all styling on a widget or page, with human-readable output cross-referencing the theme registry.

**Step 6: ALTER STYLING** — Locate widget in page BSON by name, modify Appearance in place, write back to MPR. Validates property names and values against theme registry.

**Step 7: SHOW DESIGN PROPERTIES** — Query the theme registry and format as a table, showing available properties per widget type with their allowed values.

#### Open Questions

1. **Snippet styling** — Should `alter styling` also work on snippets (`alter styling on snippet Module.Name widget ...`)?
2. **Compound properties** — Spacing has nested structure (margin-top, margin-bottom, etc.). Should we flatten to `'Spacing.Margin.Top' = 'M'` or use a nested syntax?
3. **Validation** — Should `alter styling` reject unknown property names / invalid option values, or allow them (for forward compatibility with newer themes)?
4. **Theme discovery** — The MPR path gives us the project root, but we need to verify `themesource/` exists and handle projects without Atlas Core.

## Recommendation

Phase 1 alone covers ~90% of real-world styling needs. It's a small change (the plumbing already exists in `BaseWidget` and `serializeAppearance`) and fits naturally into the existing property syntax. Phase 2 is a straightforward extension. Phase 3 provides two complementary interfaces: inline `designproperties: [...]` in `create page` for new pages, and fragment commands (`describe styling`, `alter styling`, `show design properties`) for surgical updates to existing pages. Both depend on a theme reader that parses `design-properties.json` from the project's `themesource` folder.
