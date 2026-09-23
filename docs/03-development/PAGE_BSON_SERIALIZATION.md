# Page BSON Serialization

This document describes the BSON serialization format for Mendix pages, including widget type mappings, required default properties, and common pitfalls.

## Source of Truth

The authoritative reference for BSON serialization is the **reflection-data** at:
```
reference/mendixmodellib/reflection-data/{version}-structures.json
```

Each structure entry contains:
- `qualifiedName`: The API name (e.g., `pages$DivContainer`)
- `storageName`: The BSON `$type` value (e.g., `Forms$DivContainer`)
- `defaultSettings`: Required default property values
- `properties`: Property definitions with types and requirements

## Type Name Mapping

Mendix uses different prefixes for API names vs storage names:

| API Prefix | Storage Prefix | Domain |
|------------|----------------|--------|
| `pages$` | `Forms$` | Page widgets |
| `microflows$` | `microflows$` | Microflow elements |
| `DomainModels$` | `DomainModels$` | Domain model elements |
| `Texts$` | `Texts$` | Text/translation elements |
| `DataTypes$` | `DataTypes$` | Data type definitions |
| `CustomWidgets$` | `CustomWidgets$` | Pluggable widgets |

### Common Type Name Mistakes

| Incorrect (will fail) | Correct Storage Name |
|-----------------------|---------------------|
| `Forms$NoClientAction` | `Forms$NoAction` |
| `Forms$PageClientAction` | `Forms$FormAction` |
| `Forms$MicroflowClientAction` | `Forms$MicroflowAction` |
| `pages$DivContainer` | `Forms$DivContainer` |
| `pages$actionbutton` | `Forms$actionbutton` |

## Widget Default Properties

Each widget type requires specific default properties to be serialized. Studio Pro will fail to load the project if required properties are missing.

### DivContainer (Container)

```json
{
  "$type": "Forms$DivContainer",
  "Appearance": { ... },
  "ConditionalVisibilitySettings": null,
  "Name": "",
  "NativeAccessibilitySettings": null,
  "OnClickAction": { "$type": "Forms$NoAction", ... },
  "rendermode": "div",
  "ScreenReaderHidden": false,
  "tabindex": 0,
  "widgets": [3]
}
```

### LayoutGrid

```json
{
  "$type": "Forms$layoutgrid",
  "Appearance": { ... },
  "ConditionalVisibilitySettings": null,
  "Name": "",
  "Rows": [3],
  "tabindex": 0,
  "width": "FullWidth"
}
```

### LayoutGridRow

```json
{
  "$type": "Forms$LayoutGridRow",
  "Appearance": { ... },
  "columns": [3],
  "ConditionalVisibilitySettings": null,
  "HorizontalAlignment": "none",
  "SpacingBetweenColumns": true,
  "VerticalAlignment": "none"
}
```

### LayoutGridColumn

```json
{
  "$type": "Forms$LayoutGridColumn",
  "Appearance": { ... },
  "PhoneWeight": -1,
  "PreviewWidth": -1,
  "TabletWeight": -1,
  "VerticalAlignment": "none",
  "Weight": -1,
  "widgets": [3]
}
```

### ActionButton

```json
{
  "$type": "Forms$actionbutton",
  "action": { ... },
  "Appearance": { ... },
  "AriaRole": "button",
  "buttonstyle": "default",
  "CaptionTemplate": { ... },
  "ConditionalVisibilitySettings": null,
  "icon": null,
  "Name": "",
  "NativeAccessibilitySettings": null,
  "RenderType": "button",
  "tabindex": 0,
  "tooltip": { ... }
}
```

**Note:** Use `RenderType` (not `rendermode`) for ActionButton.

### DynamicText

```json
{
  "$type": "Forms$dynamictext",
  "Appearance": { ... },
  "ConditionalVisibilitySettings": null,
  "content": { ... },
  "Name": "",
  "NativeAccessibilitySettings": null,
  "NativeTextStyle": "text",
  "rendermode": "text",
  "tabindex": 0
}
```

### Text (Static)

```json
{
  "$type": "Forms$text",
  "Appearance": { ... },
  "caption": { ... },
  "ConditionalVisibilitySettings": null,
  "Name": "",
  "NativeAccessibilitySettings": null,
  "NativeTextStyle": "text",
  "rendermode": "text",
  "tabindex": 0
}
```

### Title

```json
{
  "$type": "Forms$title",
  "Appearance": { ... },
  "caption": { ... },
  "ConditionalVisibilitySettings": null,
  "Name": "",
  "NativeAccessibilitySettings": null,
  "tabindex": 0
}
```

### DataView

```json
{
  "$type": "Forms$dataview",
  "Appearance": { ... },
  "ConditionalEditabilitySettings": null,
  "ConditionalVisibilitySettings": null,
  "datasource": { ... },
  "Editability": "Always",
  "FooterWidgets": [3],
  "LabelWidth": 3,
  "Name": "",
  "NoEntityMessage": { ... },
  "ReadOnlyStyle": "Control",
  "ShowFooter": true,
  "tabindex": 0,
  "widgets": [3]
}
```

### Input Widgets (TextBox, TextArea, DatePicker, etc.)

Input widgets require several non-null properties for proper serialization:

```json
{
  "$type": "Forms$textbox",
  "Appearance": { ... },
  "AriaRequired": false,
  "AttributeRef": {
    "$ID": "<uuid>",
    "$type": "DomainModels$AttributeRef",
    "attribute": "Module.Entity.AttributeName",
    "EntityRef": null
  },
  "AutoFocus": false,
  "Autocomplete": true,
  "AutocompletePurpose": "on",
  "ConditionalEditabilitySettings": null,
  "ConditionalVisibilitySettings": null,
  "editable": "Always",
  "FormattingInfo": { ... },
  "InputMask": "",
  "IsPasswordBox": false,
  "KeyboardType": "default",
  "LabelTemplate": { ... },
  "MaxLengthCode": -1,
  "Name": "textBox1",
  "NativeAccessibilitySettings": null,
  "OnChangeAction": { "$type": "Forms$NoAction", ... },
  "OnEnterAction": { "$type": "Forms$NoAction", ... },
  "OnEnterKeyPressAction": { "$type": "Forms$NoAction", ... },
  "OnLeaveAction": { "$type": "Forms$NoAction", ... },
  "PlaceholderTemplate": { ... },
  "ReadOnlyStyle": "Inherit",
  "ScreenReaderLabel": null,
  "SourceVariable": null,
  "SubmitBehaviour": "OnEndEditing",
  "SubmitOnInputDelay": 300,
  "tabindex": 0,
  "validation": { ... }
}
```

**Required nested objects:**
- `AttributeRef` - Must have `attribute` as fully qualified path (e.g., `Module.Entity.AttributeName`)
- `FormattingInfo` - Required for TextBox and DatePicker
- `PlaceholderTemplate` - Required `Forms$ClientTemplate` object
- `validation` - Required `Forms$WidgetValidation` object

### Attribute Path Resolution

The `AttributeRef.Attribute` field requires a **fully qualified path** in the format `Module.Entity.AttributeName`.

When using short attribute names in MDL (e.g., `attribute 'Name'`), the SDK automatically resolves them to fully qualified paths using the DataView's entity context:

```
Short:     Name
Resolved:  PgTest.Customer.Name
```

This resolution happens in `cmd_pages_builder_input.go:resolveAttributePath()` using the entity context set by the containing DataView.

## Client Action Types

| Action Type | Storage Name |
|-------------|--------------|
| No Action | `Forms$NoAction` |
| Save Changes | `Forms$SaveChangesClientAction` |
| Cancel Changes | `Forms$CancelChangesClientAction` |
| Close Page | `Forms$ClosePageClientAction` |
| Delete | `Forms$DeleteClientAction` |
| Show Page | `Forms$FormAction` |
| Call Microflow | `Forms$MicroflowAction` |
| Call Nanoflow | `Forms$CallNanoflowClientAction` |

## Array Version Markers

Mendix BSON uses version markers for arrays:

| Marker | Meaning |
|--------|---------|
| `[3]` | Empty array |
| `[2, item1, item2, ...]` | Non-empty array with items |
| `[3, item1, item2, ...]` | Non-empty array (text items) |

Example:
```json
"widgets": [3]           // empty widgets array
"widgets": [2, {...}]    // One widget
"Items": [3, {...}]      // One text translation item
```

## Appearance Object

Standard appearance object for widgets:

```json
{
  "$ID": "<uuid>",
  "$type": "Forms$Appearance",
  "class": "",
  "designproperties": [3],
  "DynamicClasses": "",
  "style": ""
}
```

## Page Structure

A page document has this top-level structure:

```json
{
  "$ID": "<uuid>",
  "$type": "Forms$page",
  "AllowedModuleRoles": [1],
  "Appearance": { ... },
  "Autofocus": "DesktopOnly",
  "CanvasHeight": 600,
  "CanvasWidth": 1200,
  "documentation": "",
  "Excluded": false,
  "ExportLevel": "Hidden",
  "FormCall": { ... },
  "Name": "PageName",
  "parameters": [3, ...],
  "PopupCloseAction": "",
  "title": { ... },
  "url": "page_url",
  "variables": [3]
}
```

## Pluggable Widgets (CustomWidget)

Pluggable widgets like ComboBox use the `CustomWidgets$customwidget` type with a complex structure:

```json
{
  "$type": "CustomWidgets$customwidget",
  "Appearance": { ... },
  "ConditionalEditabilitySettings": null,
  "ConditionalVisibilitySettings": null,
  "editable": "Always",
  "LabelTemplate": null,
  "Name": "comboBox1",
  "object": {
    "$type": "CustomWidgets$WidgetObject",
    "properties": [2, { ... }],
    "TypePointer": "<binary ID referencing ObjectType>"
  },
  "tabindex": 0,
  "type": {
    "$type": "CustomWidgets$CustomWidgetType",
    "HelpUrl": "...",
    "ObjectType": {
      "$type": "CustomWidgets$WidgetObjectType",
      "PropertyTypes": [2, { ... }]
    },
    "OfflineCapable": false,
    "PluginWidget": false,
    "WidgetId": "com.mendix.widget.web.combobox.Combobox"
  }
}
```

### CustomWidget Size

Each pluggable widget instance contains a **full copy** of both Type and Object:

| Component | BSON Size | Description |
|-----------|-----------|-------------|
| Type (CustomWidgetType) | ~54 KB | Widget definition with all PropertyTypes |
| Object (WidgetObject) | ~34 KB | Property values for all PropertyTypes |
| **Total per widget** | **~88 KB** | |

For a page with 4 ComboBox widgets: **~352 KB** just for the widgets.

This is exactly how Mendix Studio Pro stores pluggable widgets - there is no deduplication within a page.

### TypePointer References

**Critical:** There are three levels of TypePointer references:

1. **WidgetObject.TypePointer** → References `ObjectType.$ID` (the WidgetObjectType)
2. **WidgetProperty.TypePointer** → References `PropertyType.$ID` (the WidgetPropertyType)
3. **WidgetValue.TypePointer** → References `ValueType.$ID` (the WidgetValueType)

```
CustomWidgetType
└── ObjectType (WidgetObjectType)           ← WidgetObject.TypePointer
    └── PropertyTypes[]
        └── PropertyType (WidgetPropertyType) ← WidgetProperty.TypePointer
            └── ValueType (WidgetValueType)   ← WidgetValue.TypePointer
```

### Common Mistake: Missing TypePointers

If any TypePointer is missing or references an invalid ID, you'll get errors like:
- `NullReferenceException in GenerateDefaultProperties(WidgetObject widgetObject)` - Missing WidgetObject.TypePointer
- `The given key 'abc123...' was not present in the dictionary` - Invalid PropertyType reference
- `Could not find widget property value for property X` - Missing WidgetProperty for a PropertyType

### Widget Templates

To create pluggable widgets correctly, we use **embedded templates** extracted from working widgets:

```
sdk/widgets/
├── loader.go                           # template loading and cloning
└── templates/
    └── mendix-11.6/
        ├── combobox.json              # full combobox template (~5400 lines json)
        ├── datagrid.json              # datagrid template
        └── ...
```

Each template contains:
- `type`: The full CustomWidgetType definition
- `object`: The full WidgetObject with all property values

When creating a widget:
1. Load the template from embedded JSON
2. Clone both Type and Object with regenerated IDs
3. Update the ID mapping so TypePointers reference the new IDs
4. Modify specific property values (e.g., `attributeEnumeration`)

```go
// get template and clone with new IDs
embeddedType, embeddedObject, propertyIDs, objectTypeID, err := widgets.GetTemplateFullBSON(
    pages.WidgetIDComboBox,
    mpr.GenerateID,
)

// update specific property value
updatedObject := updateWidgetPropertyValue(embeddedObject, propertyIDs, "attributeEnumeration", ...)
```

### ComboBox Widget ID

```go
const WidgetIDComboBox = "com.mendix.widget.web.combobox.Combobox"
```

### Extracting New Widget Templates

To extract a template from an existing widget in a Mendix project:

```go
reader, _ := modelsdk.Open("project.mpr")
rawWidget, _ := reader.FindCustomWidgetType(pages.WidgetIDComboBox)

// rawWidget.RawType contains the CustomWidgetType
// rawWidget.RawObject contains the WidgetObject with all property values
```

Convert to JSON and save to `sdk/widgets/templates/mendix-{version}/`.

## Common Errors

### "The type cache does not contain a type with qualified name X"

This error means the `$type` value is incorrect. Check the reflection-data for the correct storage name.

**Examples:**
- `Forms$NoClientAction` → Use `Forms$NoAction`
- `Forms$PageClientAction` → Use `Forms$FormAction`
- `pages$DivContainer` → Use `Forms$DivContainer`

### "No entity configured for the data source"

The DataView's `datasource` property is missing or incorrectly configured. A DataView using a page parameter needs a `Forms$DataViewSource` with proper `EntityRef` and `SourceVariable`:

```json
{
  "$type": "Forms$DataViewSource",
  "EntityRef": {
    "$type": "DomainModels$DirectEntityRef",
    "entity": "Module.EntityName"
  },
  "ForceFullObjects": false,
  "SourceVariable": {
    "$type": "Forms$PageVariable",
    "LocalVariable": "",
    "PageParameter": "ParameterName",
    "SnippetParameter": "",
    "SubKey": "",
    "UseAllPages": false,
    "widget": ""
  }
}
```

**Common mistakes:**
- Using `EntityPathSource` instead of `DataViewSource` for page parameters
- Missing `EntityRef` or `SourceVariable` properties
- `PageParameter` should be the parameter name without the `$` prefix

### "Project uses features that are no longer supported"

Widget properties are missing or have incorrect values. Check that all required default properties from the reflection-data are included.

## Querying Reflection Data

Use this Python snippet to check widget default settings:

```python
import json

with open('reference/mendixmodellib/reflection-data/11.0.0-structures.json') as f:
    data = json.load(f)

# find widget by api name
widget = data.get('Pages$DivContainer', {})
print('Storage name:', widget.get('storageName'))
print('Defaults:', json.dumps(widget.get('defaultSettings', {}), indent=2))

# search by storage name
for key, val in data.items():
    if val.get('storageName') == 'Forms$NoAction':
        print(f'{key}: {val.get("defaultSettings")}')
```

## Files Reference

| File | Purpose |
|------|---------|
| `mdl/backend/modelsdk/widget_write.go` | Widget serialization to BSON |
| `mdl/backend/modelsdk/page_write.go` | Page serialization |
| `mdl/backend/modelsdk/widget_pluggable_write.go` | Pluggable widget templates |
| `mdl/backend/modelsdk/page.go` | Page deserialization |
| `modelsdk/codec/encoder.go` | Document → BSON |
| `modelsdk/codec/decoder.go` | BSON → document |
| `sdk/widgets/loader.go` | Embedded template loading |
| `sdk/widgets/templates/mendix-11.6/*.json` | Embedded widget templates |
| `sdk/pages/pages_widgets_advanced.go` | CustomWidget Go types |
| `mdl/executor/cmd_pages_builder_input.go` | Widget creation from MDL |
| `reference/mendixmodellib/reflection-data/*.json` | Type definitions |

## Pluggable Widgets (CustomWidgets)

Pluggable widgets like DataGrid2, ComboBox, and Gallery use a fundamentally different structure than built-in widgets.

### Structure

```
CustomWidgets$customwidget
├── type (CustomWidgets$CustomWidgetType)
│   ├── WidgetId: "com.mendix.widget.web.datagrid.Datagrid"
│   ├── ObjectType (CustomWidgets$WidgetObjectType)
│   │   └── PropertyTypes[] (CustomWidgets$WidgetPropertyType)
│   │       ├── $ID: "<property-type-id>"
│   │       ├── PropertyKey: "datasource"
│   │       └── ValueType (CustomWidgets$WidgetValueType)
│   └── ...
└── object (CustomWidgets$WidgetObject)
    └── properties[] (CustomWidgets$WidgetProperty)
        ├── TypePointer: "<property-type-id>"  // references PropertyTypes.$ID
        └── value (CustomWidgets$WidgetValue)
            ├── TypePointer: "<value-type-id>"
            ├── datasource, AttributeRef, PrimitiveValue, TextTemplate, etc.
            └── objects[] (for nested object lists like columns)
```

### Critical Requirements

1. **Type-Object ID Consistency**: `Object.Properties[].TypePointer` MUST reference valid `Type.ObjectType.PropertyTypes[].$ID` values. When regenerating IDs, both must use the same ID mapping.

2. **All Properties Required**: Every PropertyType in the Type must have a corresponding WidgetProperty in the Object. Missing properties cause "widget definition has changed" errors.

3. **TextTemplate Properties**: Properties with `ValueType.Type = "TextTemplate"` need proper `Forms$ClientTemplate` structures, not null:
   ```json
   "TextTemplate": {
     "$ID": "<uuid>",
     "$type": "Forms$ClientTemplate",
     "Fallback": { "$ID": "<uuid>", "$type": "Texts$text", "Items": [] },
     "parameters": [],
     "template": { "$ID": "<uuid>", "$type": "Texts$text", "Items": [] }
   }
   ```

   **CRITICAL**: Empty arrays must be `[]`, NOT `[2]`. In JSON, `[2]` is an array containing the integer 2, not an empty array with a version marker. The version markers only exist in BSON format, not in the JSON templates.

4. **Default Values from Template**: Use embedded templates from `sdk/widgets/templates/` which include both Type AND Object with correct default values.

### Implementation Pattern

```go
// Load template with both type and object
embeddedType, embeddedObject, propertyTypeIDs, objectTypeID, err :=
    widgets.GetTemplateFullBSON(widgetID, mpr.GenerateID)

// update the template object with specific values (datasource, columns)
rawObject := updateTemplateObject(embeddedObject, propertyTypeIDs, datasource, columns)

// create widget with cloned type and updated object
widget := &pages.CustomWidget{
    RawType:   embeddedType,
    RawObject: rawObject,
    ...
}
```

### Nested WidgetObjects (Columns, etc.)

**Critical Insight**: Pluggable widgets often contain nested `WidgetObject` instances (e.g., DataGrid2 columns). These nested objects must follow the **same completeness rule** as the parent widget:

> **ALL properties defined in the nested `ObjectType.PropertyTypes` must be created with default values, not just the ones with explicit values.**

#### Example: DataGrid2 Columns

The DataGrid2 `columns` property has an `ObjectType` with 21 `PropertyTypes`:

```
showContentAs, attribute, content, dynamictext, exportValue, header, tooltip,
filter, visible, sortable, resizable, draggable, hidable, allowEventPropagation,
width, minWidth, minWidthLimit, size, alignment, columnClass, wrapText
```

**Wrong approach** (creates incomplete columns):
```go
// Only creates 5 properties - columns won't appear in Page Explorer
columnProperties := bson.A{int32(2),
    buildProperty("showContentAs", "attribute"),
    buildProperty("attribute", attrPath),
    buildProperty("header", headerText),
    buildProperty("content", filterWidget),
    buildProperty("filter", filterWidget),
}
```

**Correct approach** (creates all 21 properties):
```go
// create all properties from the template's ObjectType.PropertyTypes
for _, propType := range columnObjectType.PropertyTypes {
    if propType.PropertyKey == "attribute" {
        // use explicit value
        columnProperties = append(columnProperties, buildProperty(propType, attrPath))
    } else {
        // use default value from template
        columnProperties = append(columnProperties, buildDefaultProperty(propType))
    }
}
```

#### Symptoms of Incomplete Nested Objects

| Symptom | Cause |
|---------|-------|
| Columns not visible in Page Explorer | Column objects missing properties |
| "widget definition has changed" error | Property count mismatch |
| Widget shows in editor but not explorer | Partial object recognition |

#### Template Structure for Columns

The embedded templates at `sdk/widgets/templates/mendix-11.6/datagrid.json` contain the full column `ObjectType` definition:

```json
{
  "PropertyKey": "columns",
  "ValueType": {
    "ObjectType": {
      "PropertyTypes": [
        { "PropertyKey": "showContentAs", "ValueType": { "DefaultValue": "attribute" } },
        { "PropertyKey": "attribute", ... },
        { "PropertyKey": "content", ... },
        // ... all 21 properties with their default values
      ]
    }
  }
}
```

When creating columns, iterate through ALL `PropertyTypes` and create a `WidgetProperty` for each one.

### Expression-Type Properties

**Critical Insight**: Properties with `ValueType.Type = "expression"` (like `visible`, `editable`, etc.) require a non-empty `expression` value. Template widgets often have empty/placeholder Expression values that will cause validation errors if cloned directly.

#### Example: DataGrid2 Column "visible" Property

The `visible` property on DataGrid2 columns controls whether the column is displayed. It uses the Expression type:

```json
{
  "$type": "CustomWidgets$WidgetProperty",
  "TypePointer": "<visible-property-type-id>",
  "value": {
    "$type": "CustomWidgets$WidgetValue",
    "expression": "true",        // required: non-empty expression
    "TypePointer": "<visible-value-type-id>",
    ...
  }
}
```

**Template pitfall**: The template's `visible` property may have `"expression": ""` (empty). When cloning column properties, you must:

1. **Check if Expression is empty**: If the template has an empty Expression
2. **Rebuild the property**: Create a new property with `expression: "true"` instead of cloning

```go
// in cloneAndUpdateColumnProperties
if propKey == "visible" {
    var hasExpression bool
    for _, pe := range propMap {
        if pe.Key == "value" {
            if valDoc, ok := pe.Value.(bson.D); ok {
                for _, ve := range valDoc {
                    if ve.Key == "expression" && ve.Value != "" {
                        hasExpression = true
                    }
                }
            }
        }
    }
    if !hasExpression {
        // Rebuild with expression: "true" instead of cloning empty value
        result = append(result, pb.buildColumnExpressionProperty(visibleEntry, "true"))
    } else {
        result = append(result, pb.clonePropertyWithNewIDs(propMap))
    }
}
```

#### Symptoms of Empty Expression Values

| Error | Property | Solution |
|-------|----------|----------|
| CE0642 "Property 'Visible' is required" | Column `visible` | Rebuild with `expression: "true"` |
| CE0642 "Property 'Editable' is required" | Column `editable` | Rebuild with `expression: "true"` |
| Column always hidden | Column `visible` | Check Expression isn't empty |

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| CE0463 "widget definition has changed" | Object properties don't match Type PropertyTypes | Use template Object as base, only modify needed properties |
| CE0642 "Property 'X' is required" | Expression-type property has empty Expression value | Rebuild property with non-empty Expression (e.g., "true") |
| "Sequence contains no matching element" | Missing properties in Object | Ensure all PropertyTypes have corresponding WidgetProperties |
| Column captions empty | TextTemplate not properly structured | Use `Forms$ClientTemplate` with Fallback and Template |
| Columns not in Page Explorer | Nested WidgetObjects incomplete | Create ALL properties from ObjectType.PropertyTypes |

### Cloning Strategy Summary

When cloning pluggable widget properties from a template:

1. **Clone everything with new IDs** - All `$ID` values must be regenerated
2. **Keep TypePointers consistent** - Don't regenerate TypePointer values (they reference the Type)
3. **Handle empty Expression values** - Rebuild Expression-type properties if the template has empty values
4. **Add missing required properties** - If the template is sparse, add required properties that are missing

## CE0463: Widget Definition Changed — Root Cause Analysis

The CE0463 error ("The definition of this widget has changed") is one of the most subtle
errors when building pluggable widgets programmatically. This section documents the findings
from systematic debugging of DataGrid2 custom content columns.

### CE0463 Is Mode-Dependent, Not Just Type-Dependent

CE0463 is **not** simply about the Type section being outdated. It triggers when the
**Object property values are inconsistent with the current mode** of the widget.

Pluggable widgets have **mode-switching properties** (like DataGrid2's `showContentAs`)
that change which other properties are visible vs hidden. The widget's `editorConfig.js`
(inside the `.mpk` package) defines these visibility rules:

```javascript
// from Datagrid.editorConfig.js (deminified):
// when not customContent mode: HIDE content, allowEventPropagation, exportValue
"customContent" !== col.showContentAs &&
    hideNestedPropertiesIn(properties, values, "columns", idx,
        ["content", "allowEventPropagation", "exportValue"]);

// when in customContent mode: HIDE tooltip
"customContent" === col.showContentAs &&
    hidePropertyIn(properties, values, "columns", idx, "tooltip");
```

### The Property State Matrix

When `showContentAs` changes, certain properties must have mode-appropriate values:

| Property              | attribute mode       | customContent mode     |
|-----------------------|---------------------|------------------------|
| `showContentAs`       | PV="attribute"      | PV="customContent"    |
| `attribute`           | AttrRef=present     | AttrRef=null          |
| `content`             | HIDDEN (W=0)        | VISIBLE (W=n widgets) |
| `tooltip`             | VISIBLE (TT=present)| HIDDEN (TT=null)      |
| `exportValue`         | HIDDEN (no TT)      | VISIBLE (TT=present)  |
| `allowEventPropagation`| HIDDEN (PV=true)   | VISIBLE (PV=true, required) |
| `dynamictext`         | VISIBLE             | HIDDEN                |

**Key insight**: Simply cloning a template column (which has attribute-mode defaults) and
changing only `showContentAs` to `customContent` triggers CE0463 because the hidden/visible
property states are still in attribute mode.

### Evidence: Mutation Test

Taking a working widget (P012 ProductGrid with attribute-mode columns, passes CE0463 after
`mx update-widgets`) and changing **only** the PrimitiveValue of showContentAs from
"attribute" to "customContent" immediately triggers CE0463 — proving the issue is about
Object property state consistency, not Type section correctness.

### Widget Package (.mpk) as Source of Truth

The project's `widgets/` folder contains `.mpk` files (ZIP archives) for each pluggable
widget. The canonical widget definition is in `{WidgetName}.xml` inside the mpk:

```
widgets/com.mendix.widget.web.Datagrid.mpk
├── Datagrid.xml              ← widget schema: properties, types, defaults, enums
├── Datagrid.editorConfig.js  ← Property visibility rules (mode-dependent hiding)
├── package.xml               ← Package metadata and version
└── com/mendix/.../Datagrid.js  ← runtime widget code
```

The `Datagrid.xml` defines all 21 column properties with their types, defaults, and
constraints. The `editorConfig.js` defines which properties are visible/hidden based on
the current values of other properties. Together they form the complete specification
that `mx update-widgets` uses to normalize widget Objects.

### Workaround: Post-Processing with `mx update-widgets`

Running `mx update-widgets` after creating pages normalizes all widget Objects to match
the mpk definition. This eliminates CE0463 regardless of what property states the
programmatic builder set:

```bash
# after creating pages with mxcli:
reference/mxbuild/modeler/mx update-widgets /path/to/app.mpr
```

This is safe: it only updates the Object section (not the Type), and only changes
properties that are in an inconsistent state.

### Proper Fix: Mode-Aware Column Building

To avoid CE0463 without post-processing, the column builder must adjust properties
based on the showContentAs mode. When building a customContent column:

1. Set `showContentAs` PV to "customContent"
2. Set `content` Widgets to actual content widgets
3. Clear `tooltip` TextTemplate (hidden in customContent mode)
4. Ensure `exportValue` has a TextTemplate (visible in customContent mode)
5. Keep `allowEventPropagation` as-is from template (visible in customContent mode; clearing it triggers CE0642 "Property 'Allow row events' is required")
6. Clear `attribute` AttrRef (no attribute in customContent mode)

## Related Documentation

- [MDL Parser Architecture](./MDL_PARSER_ARCHITECTURE.md)
- [String Template Syntax](./STRING_TEMPLATE_SYNTAX.md)
