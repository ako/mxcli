# Widget Template System

Pluggable widgets (DataGrid2, ComboBox, Gallery, etc.) require embedded template definitions for correct BSON serialization. This page explains how the template system works, from extraction through runtime loading and property mapping.

> For the user-facing view — how mxcli keeps widget definitions version-correct across Mendix releases, and the `mxcli widget describe` command for inspecting a widget's discovered properties and dynamic rules — see [Pluggable Widgets Across Versions](../guides/pluggable-widgets.md).

## Why Templates Are Needed

Pluggable widgets in Mendix are defined by two BSON components stored inside each widget instance:

1. **Type** (`CustomWidgets$CustomWidgetType`) -- defines the widget's PropertyTypes schema (what properties exist, their value types)
2. **Object** (`CustomWidgets$WidgetObject`) -- provides a valid instance with default values for all properties

Both must be present. The Object's `TypePointer` references the Type's `$ID`, and each `WidgetProperty.TypePointer` in the Object references the corresponding `WidgetPropertyType.$ID` in the Type. If these cross-references are broken or any property is missing, Studio Pro reports **CE0463 "widget definition changed"**.

Building these structures programmatically is error-prone (50+ PropertyTypes, nested ValueTypes, TextTemplate structures, etc.), so mxcli clones them from known-good templates extracted from Studio Pro.

## Template Location

### Embedded Templates (built into binary)

```
sdk/widgets/
├── loader.go                      # Template loading with go:embed
├── mpk/mpk.go                     # .mpk ZIP parsing for augmentation
├── definitions/                   # Widget definition files (.def.json)
│   ├── combobox.def.json
│   └── gallery.def.json
└── templates/
    └── mendix-11.6/               # Templates by Mendix version
        ├── combobox.json
        ├── datagrid.json
        ├── gallery.json
        └── datagrid-*-filter.json
```

### User Templates (3-tier priority)

| Priority | Location | Scope |
|----------|----------|-------|
| 1 (highest) | `<project>/.mxcli/widgets/*.json` | Project-specific |
| 2 | `~/.mxcli/widgets/*.json` | Global (all projects) |
| 3 (lowest) | `sdk/widgets/templates/` (embedded) | Built-in |

## Template JSON Structure

Each template file contains both the Type and Object structures converted from BSON to JSON:

```json
{
  "widgetId": "com.mendix.widget.web.combobox.Combobox",
  "name": "Combo box",
  "version": "11.6.0",
  "extractedFrom": "PageTemplates.Customer_NewEdit",
  "type": {
    "$ID": "aa000000000000000000000000000001",
    "$Type": "CustomWidgets$CustomWidgetType",
    "WidgetId": "com.mendix.widget.web.combobox.Combobox",
    "PropertyTypes": [
      {
        "$ID": "aa000000000000000000000000000010",
        "$Type": "CustomWidgets$WidgetPropertyType",
        "PropertyKey": "attributeEnumeration",
        "ValueType": {
          "$ID": "aa000000000000000000000000000011",
          "Type": "Attribute",
          "DefaultValue": ""
        }
      }
    ]
  },
  "object": {
    "$ID": "aa000000000000000000000000000100",
    "$Type": "CustomWidgets$WidgetObject",
    "TypePointer": "aa000000000000000000000000000001",
    "Properties": [
      2,
      {
        "$ID": "aa000000000000000000000000000110",
        "$Type": "CustomWidgets$WidgetProperty",
        "TypePointer": "aa000000000000000000000000000010",
        "Value": {
          "$ID": "aa000000000000000000000000000111",
          "$Type": "CustomWidgets$WidgetValue",
          "Action": { "$Type": "Forms$NoAction", "DisabledDuringExecution": true },
          "AttributeRef": null,
          "DataSource": null,
          "EntityRef": null,
          "Expression": "",
          "PrimitiveValue": "",
          "Selection": "None",
          "TextTemplate": null,
          "Widgets": [2],
          "XPathConstraint": ""
        }
      }
    ]
  }
}
```

### Key Cross-References

```
Type.PropertyTypes[].$ID  <--  Object.Properties[].TypePointer
    (WidgetPropertyType)       (WidgetProperty points to its type)

Type.$ID  <--  Object.TypePointer
    (CustomWidgetType)     (WidgetObject points to its type)

Type.PropertyTypes[].ValueType.$ID  <--  Object.Properties[].Value.TypePointer
    (ValueType definition)              (WidgetValue points to its value type)
```

These cross-references are maintained through ID remapping at load time (see below).

## Loading Templates at Runtime

### Entry Point

```go
bsonType, bsonObject, propertyTypeIDs, objectTypeID, err :=
    widgets.GetTemplateFullBSON(widgetID, mpr.GenerateID, projectPath)
```

### 3-Phase Pipeline (`loader.go`)

#### Phase 1: Collect IDs and Generate Mapping

`collectIDs()` recursively walks both `type` and `object` JSON, finds every `$ID` field, and creates a mapping from old template IDs to freshly generated UUIDs:

```
Template $ID (static)                    -> New UUID (runtime)
"aa000000000000000000000000000001"       -> "a1b2c3d4e5f6..."  (mpr.GenerateID())
"aa000000000000000000000000000010"       -> "f7e8d9c0b1a2..."
...
```

This ensures each widget instance gets unique IDs while preserving internal cross-references.

#### Phase 2: Convert Type JSON to BSON

`jsonToBSONWithMappingAndObjectType()` converts the Type JSON to `bson.D`, performing three tasks simultaneously:

1. **ID replacement**: Every `$ID` field is looked up in the mapping, converted to binary GUID format via `hexToIDBlob()` (with Microsoft GUID byte-swap for the first 3 segments)
2. **String ID references**: Any 32-char hex string value that appears in the mapping is also converted to binary (these are cross-references between elements)
3. **PropertyTypeIDMap extraction**: For each `CustomWidgets$WidgetPropertyType` node, records:

```go
PropertyTypeIDMap["attributeEnumeration"] = PropertyTypeIDEntry{
    PropertyTypeID: "f7e8d9c0b1a2...",   // new ID of the PropertyType
    ValueTypeID:    "c3d4e5f6a7b8...",   // new ID of the ValueType
    DefaultValue:   "",                   // from ValueType.DefaultValue
    ValueType:      "Attribute",          // from ValueType.Type
    ObjectTypeID:   "...",               // for nested object list properties
    NestedPropertyIDs: {...},            // property IDs within nested ObjectType
}
```

This map is the bridge between `.def.json` property keys and the BSON structure.

#### Phase 3: Convert Object JSON to BSON

`jsonToBSONObjectWithMapping()` converts the Object JSON using the **same** ID mapping. Special handling for `TypePointer` fields ensures they point to the correct new IDs in the Type.

#### Placeholder Leak Detection

After conversion, `containsPlaceholderID()` checks for any remaining `aa000000`-prefix IDs (binary or string). If found, the load fails immediately rather than producing a corrupt MPR.

### MPK Augmentation

Before the 3-phase pipeline, `augmentFromMPK()` checks if the project has a newer version of the widget:

```
1. FindMPK(projectDir, widgetID)
   -> Scan project/widgets/*.mpk, parse package.xml to match widget ID
2. ParseMPK(mpkPath)
   -> Extract XML property definitions from the .mpk ZIP
3. AugmentTemplate(clone, mpkDef)
   -> Deep-clone the cached template (never mutate cache)
   -> Add properties present in .mpk but missing from template
   -> Remove properties in template but absent from .mpk
```

This reduces CE0463 errors from widget version drift. The `.mpk` in the project's `widgets/` folder is the source of truth for which properties should exist.

### JSON to BSON Conversion Rules

| JSON Type | BSON Type |
|-----------|-----------|
| `"string"` | string |
| `float64` (whole number) | `int32` |
| `float64` (decimal) | `float64` |
| `true`/`false` | boolean |
| `null` | null |
| `[]` | empty `bson.A` |
| `[2, ...]` | `bson.A{int32(2), ...}` (array with version marker) |
| 32-char hex string in ID mapping | `[]byte` (binary GUID) |

**Important**: Empty arrays in template JSON are `[]`, not `[3]`. The BSON array version markers (`int32(2)` for non-empty, `int32(3)` for empty) are added during widget serialization, not during template loading.

## How Operations Modify Template BSON

After loading, the pluggable widget engine applies property mappings. Each operation locates the target WidgetProperty by matching `TypePointer`:

```
updateWidgetPropertyValue(obj, propTypeIDs, "datasource", updateFn)
  |
  +-- Look up propTypeIDs["datasource"].PropertyTypeID -> "f7e8d9c0b1a2..."
  |
  +-- Scan obj.Properties[] array
  |     For each WidgetProperty:
  |       matchesTypePointer(prop, "f7e8d9c0b1a2...")
  |       -> prop.TypePointer (binary GUID) -> BlobToUUID() -> compare
  |       -> Match found!
  |
  +-- Extract prop.Value (bson.D)
  +-- Apply updateFn to modify specific fields:
        opAttribute  -> sets Value.AttributeRef
        opAssociation -> sets Value.AttributeRef + Value.EntityRef
        opPrimitive  -> sets Value.PrimitiveValue
        opDatasource -> replaces Value.DataSource
        opSelection  -> sets Value.Selection
        opWidgets    -> replaces Value.Widgets array
```

All modifications produce new `bson.D` values (immutable style).

## Extracting New Templates

### Important: Use Studio Pro-Created Widgets

Always extract templates from widgets that have been **created or updated by Studio Pro**. Programmatically generated templates often have subtle differences in property ordering, default values, or nested structures (especially `TextTemplate` properties that require `Forms$ClientTemplate` structure instead of `null`).

### Extraction Process

1. **Create the widget in Studio Pro** -- add the widget to a page, configure with default settings
2. **If updating**: right-click and select "Update widget" if Studio Pro shows "widget definition has changed"
3. **Extract using mxcli**:

```bash
# Extract from MPR (manual method)
mxcli bson dump -p App.mpr --type page --object "Module.TestPage" --format json

# Extract skeleton .def.json from .mpk (automated)
mxcli widget extract --mpk widgets/MyWidget.mpk
# Output: .mxcli/widgets/mywidget.def.json
```

4. From the JSON dump, extract the `Type` and `Object` fields of the `CustomWidgets$CustomWidget` and save as the template JSON file

### Verifying Templates

```bash
# Create a test page with the widget
mxcli -p test.mpr -c "CREATE PAGE Test.TestPage ... MYWIDGET ..."

# Check for errors (should have no CE0463)
~/.mxcli/mxbuild/*/modeler/mx check test.mpr

# Compare BSON structure if issues persist
mxcli bson dump -p test.mpr --type page --object "Test.TestPage" --format ndsl
```

## Debugging CE0463 Errors

If a page fails with CE0463 after creation:

1. Create the same widget manually in Studio Pro
2. Extract its BSON with `mxcli bson dump --format ndsl`
3. Compare against the mxcli-generated widget's BSON
4. Look for:
   - Missing properties (PropertyType exists in Type but no corresponding WidgetProperty in Object)
   - Wrong default values (especially `TextTemplate` properties that should not be `null`)
   - Stale properties from an older widget version
5. Update the template JSON to match, or let MPK augmentation handle version drift

See `.claude/skills/debug-bson.md` for the detailed BSON debugging workflow.

## TextTemplate Property Requirements

Properties with `"Type": "TextTemplate"` require a proper `Forms$ClientTemplate` structure -- they **cannot** be `null`:

```json
"TextTemplate": {
  "$ID": "<guid>",
  "$Type": "Forms$ClientTemplate",
  "Fallback": { "$ID": "<guid>", "$Type": "Texts$Text", "Items": [] },
  "Parameters": [],
  "Template": { "$ID": "<guid>", "$Type": "Texts$Text", "Items": [] }
}
```

Empty arrays here must be `[]`, not `[2]`.

## Key Source Files

| File | Purpose |
|------|---------|
| `sdk/widgets/loader.go` | Template loading, 3-phase ID remapping, MPK augmentation |
| `sdk/widgets/mpk/mpk.go` | .mpk ZIP parsing, XML property extraction, FindMPK |
| `sdk/widgets/definitions/*.def.json` | Built-in widget definitions |
| `sdk/widgets/templates/mendix-11.6/*.json` | Embedded BSON templates |
| `mdl/executor/widget_engine.go` | PluggableWidgetEngine, 6 operations, Build() pipeline |
| `mdl/executor/widget_registry.go` | 3-tier registry, definition validation |
| `mdl/executor/cmd_pages_builder_input.go` | `updateWidgetPropertyValue()`, TypePointer matching |
| `cmd/mxcli/cmd_widget.go` | `mxcli widget extract/list` CLI commands |
