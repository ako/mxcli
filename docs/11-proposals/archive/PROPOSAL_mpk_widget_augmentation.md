---
title: Augment Widget Templates from .mpk at Runtime
status: done
---

# Proposal: Augment Widget Templates from .mpk at Runtime

## Problem Statement

When mxcli creates pluggable widgets (ComboBox, DataGrid2, etc.), it uses **static JSON templates** extracted from a Mendix 11.6.0 project. However, the target project may have **newer widget versions** installed in its `widgets/` folder. For example, the ComboBox template has 52 regular properties, but the installed `.mpk` (v2.5.0) defines 55 — it added `staticDataSourceCaption`, `staticDataSourceCustomContent`, and `staticDataSourceValue`. This mismatch causes **CE0463** ("widget definition has changed") in Studio Pro.

**Scope**: This proposal addresses only widget version mismatches (problem #2 in the BSON Schema Registry proposal). It does not address platform version differences or extension documents.

## Current Architecture

```
Embedded json templates (Mendix 11.6)
  combobox.json (52 properties)
  datagrid.json
  gallery.json
  ...
         │
         ▼
  GetTemplateFullBSON()
  (loads template, remaps IDs, converts to BSON)
         │
         ▼
  widget BSON in page document
  (PropertyTypes in type + properties in object)
```

**The problem**: If the project has ComboBox v2.5.0 (55 properties) but our template has v2.3.0 (52 properties), Studio Pro sees 3 missing PropertyTypes and raises CE0463. The same happens in reverse if the template has properties the installed widget doesn't know about.

## Proposed Solution

Read the `.mpk` file from the project's `widgets/` folder at runtime and **augment the static template** — adding missing properties and removing stale ones — before BSON conversion. The augmentation happens at the JSON `map[string]interface{}` level, keeping the existing BSON conversion pipeline untouched.

```
Static template (json)
         │
         ▼
  Load & deep-clone template
         │
         ▼
  Augment from .mpk xml    ◄── Parse widget xml from ZIP
  (add missing, remove stale)
         │
         ▼
  ID remapping (collectIDs)
         │
         ▼
  BSON conversion (jsonToBSON...)
```

## Detailed Design

### 1. New Package: `sdk/widgets/mpk/` — Parse .mpk Widget Packages

`.mpk` files are ZIP archives. Inside each one:
- `package.xml` — manifest with widget file path and version
- `{WidgetName}.xml` — widget definition with property schemas

```go
package mpk

type PropertyDef struct {
    key          string   // e.g. "staticDataSourceCaption"
    type         string   // xml type: "attribute", "expression", "textTemplate", "widgets", etc.
    caption      string
    description  string
    Category     string   // from enclosing propertyGroup captions, joined with "::"
    required     bool
    DefaultValue string   // for enumeration types
    IsList       bool
    IsSystem     bool     // true for <systemProperty> elements
}

type WidgetDefinition struct {
    ID         string          // e.g. "com.mendix.widget.web.combobox.Combobox"
    Name       string
    version    string
    properties []PropertyDef   // regular <property> elements
    SystemProps []PropertyDef  // <systemProperty> elements
}

// ParseMPK opens an .mpk ZIP archive, finds the widget xml, and parses it.
func ParseMPK(mpkPath string) (*WidgetDefinition, error)

// FindMPK looks in the project's widgets/ directory for an .mpk matching the widgetID.
func FindMPK(projectDir string, widgetID string) (string, error)
```

**MPK filename mapping**: The widgetID is `com.mendix.widget.web.combobox.Combobox` but the `.mpk` filename is `com.mendix.widget.web.Combobox.mpk`. Strategy:
1. List all `.mpk` files in `widgets/` directory
2. For each `.mpk`, read `package.xml` to get the widget XML filename
3. Read the widget XML and check if `<widget id="...">` matches the widgetID
4. Cache the mapping (widgetID -> mpkPath) per project directory

**Caching**: Both the directory scan and parsed definitions are cached in-memory with `sync.RWMutex` protection, keyed by project directory and widget ID.

### 2. New File: `sdk/widgets/augment.go` — Template Augmentation

```go
// AugmentTemplate modifies a template's Type and Object in-place to match an .mpk definition.
// - Adds PropertyTypes (in type) and properties (in object) for keys in .mpk but missing from template
// - Removes PropertyTypes and properties for keys in template but missing from .mpk
// - Only processes regular properties (not system properties)
func AugmentTemplate(tmpl *WidgetTemplate, def *mpk.WidgetDefinition) error
```

**Adding a missing property** requires creating both:

1. **A PropertyType** in `Type.ObjectType.PropertyTypes[]` — uses a type-dependent factory based on the XML `type=` attribute. Each XML type maps to a specific BSON `ValueType` structure:

| XML `type=`   | BSON `ValueType.Type` | Default Object Value                        |
|---------------|----------------------|---------------------------------------------|
| `attribute`   | `"attribute"`        | `AttributeRef: null`                        |
| `expression`  | `"expression"`       | `expression: ""`                            |
| `textTemplate`| `"TextTemplate"`     | `TextTemplate: {Forms$ClientTemplate...}`   |
| `widgets`     | `"widgets"`          | `widgets: [2]` (empty array marker)         |
| `enumeration` | `"enumeration"`      | `PrimitiveValue: "<defaultValue>"`          |
| `boolean`     | `"boolean"`          | `PrimitiveValue: "true"/"false"`            |
| `integer`     | `"integer"`          | `PrimitiveValue: "0"`                       |
| `datasource`  | `"datasource"`       | `datasource: null`                          |
| `action`      | `"action"`           | `action: {Forms$NoAction}`                  |
| `selection`   | `"selection"`        | `selection: "none"`                         |
| `association`  | `"association"`     | `AttributeRef: null`                        |
| `object`      | `"object"`           | `objects: [2]`                              |
| `string`      | `"string"`           | `PrimitiveValue: ""`                        |
| `decimal`     | `"decimal"`          | `PrimitiveValue: ""`                        |

2. **A Property** in `Object.Properties[]` — a `CustomWidgets$WidgetProperty` with matching `TypePointer` and a `CustomWidgets$WidgetValue` containing the default value for the type.

**Cloning strategy**: For each XML property type, find an existing PropertyType/Property pair of the same type in the template, deep-copy it, and update IDs and keys. This ensures BSON field structure is correct without hardcoding every field combination.

**ID generation**: Use placeholder hex strings (e.g., sequential `"aa00000000000000000000000000xxxx"`) since `GetTemplateFullBSON()` remaps all IDs anyway via `collectIDs()`.

**Removing a stale property**:
1. Remove PropertyType from `Type.ObjectType.PropertyTypes[]` by matching `PropertyKey`
2. Remove corresponding Property from `Object.Properties[]` by matching `TypePointer`

### 3. Modify `sdk/widgets/loader.go` — Wire Augmentation Into Pipeline

Add `projectPath` parameter to `GetTemplateFullBSON`:

```go
// before:
func GetTemplateFullBSON(widgetID string, idGenerator func() string) (...)

// after:
func GetTemplateFullBSON(widgetID string, idGenerator func() string, projectPath string) (...)
```

Inside the function, after loading the template but before ID collection:

```go
tmpl, err := GetTemplate(widgetID)
// ...

// Deep-clone so augmentation doesn't modify cached original
tmplClone := deepCloneTemplate(tmpl)

// Augment from .mpk if project path available
if projectPath != "" {
    projectDir := filepath.Dir(projectPath)
    mpkPath, err := mpk.FindMPK(projectDir, widgetID)
    if err == nil && mpkPath != "" {
        def, err := mpk.ParseMPK(mpkPath)
        if err == nil {
            AugmentTemplate(tmplClone, def)
        }
    }
}

// continue with tmplClone...
```

### 4. Update Call Sites

All callers of `GetTemplateFullBSON` pass `pb.reader.Path()`:

| File | Functions |
|------|-----------|
| `cmd_pages_builder_v3_pluggable.go` | `buildComboBoxV3`, `buildGalleryV3`, `buildTextFilterV3`, `buildNumberFilterV3`, `buildDropdownFilterV3`, `buildDateFilterV3` |
| `cmd_pages_builder_input_filters.go` | `buildFilterWidgetBSON` |
| `cmd_pages_builder_v3_widgets.go` | DataGrid2 template loading |

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `sdk/widgets/mpk/mpk.go` | **Create** | Parse .mpk ZIP, extract widget XML |
| `sdk/widgets/mpk/mpk_test.go` | **Create** | Unit tests |
| `sdk/widgets/augment.go` | **Create** | AugmentTemplate function |
| `sdk/widgets/augment_test.go` | **Create** | Unit tests for augmentation |
| `sdk/widgets/loader.go` | **Modify** | Add projectPath param, deep-clone, call augment |
| `cmd_pages_builder_v3_pluggable.go` | **Modify** | Pass project path |
| `cmd_pages_builder_input_filters.go` | **Modify** | Pass project path |
| `cmd_pages_builder_v3_widgets.go` | **Modify** | Pass project path |

## Graceful Degradation

The augmentation is entirely **best-effort**:
- If `.mpk` not found: use template as-is (current behavior)
- If XML parsing fails: use template as-is
- If a property type is unknown: skip that property
- If no matching template exists for cloning: skip that property

This means the feature can never make things worse — it only improves compatibility when it can.

## Verification

```bash
# build
make build

# Unit tests
make test

# end-to-end: combobox with newer widget version
./bin/mxcli exec test.mdl -p /path/to/project.mpr
reference/mxbuild/modeler/mx check /path/to/project.mpr
# Should have NO CE0463 errors
```

## Comparison with BSON Schema Registry Proposal

This proposal implements a **tactical subset** of Phase 3 (Widget Schema Resolution) from the BSON Schema Registry proposal, scoped to solve the immediate CE0463 problem.

| Aspect | Schema Registry (full) | This Proposal (tactical) |
|--------|----------------------|--------------------------|
| **Scope** | Platform types + widgets + extensions + migrations | Widgets only |
| **Data source** | Reflection data JSON + .mpk XML + extension manifests | .mpk XML only |
| **Schema format** | Unified `TypeSchema`/`WidgetSchema` Go structs | Lightweight `PropertyDef` from XML |
| **Integration point** | New `schema/` package wired into reader + writer | Augmentation hook in existing `GetTemplateFullBSON` |
| **Widget resolution** | 3-tier registry (project -> embedded -> passthrough) | 2-step: augment embedded template from .mpk |
| **Complexity** | 6 phases, new package hierarchy | 2 new files + 1 modified function + call site updates |
| **Risk** | Large refactor touching reader/writer core | Zero-risk: graceful fallback to current behavior |
| **Timeline** | Multi-week effort across 6 phases | Single implementation session |

### Key Differences

1. **No registry abstraction**: This proposal doesn't create a `WidgetRegistry` or `WidgetSchema` type. It operates on the existing `WidgetTemplate` (`map[string]interface{}`) directly. The schema registry would formalize this into typed Go structs.

2. **Template augmentation vs. schema-driven generation**: The schema registry would eventually generate widget BSON from the schema alone. This proposal takes the existing template as ground truth and patches it — simpler but less flexible.

3. **No validation framework**: The schema registry includes a `Validator` that checks BSON output against schemas. This proposal relies on `mx check` for validation.

4. **Forward-compatible**: This proposal's `mpk.ParseMPK()` and `mpk.FindMPK()` functions can be reused directly as the Tier 1 loader in the schema registry's `WidgetRegistry`. The augmentation logic would be replaced by schema-driven generation in Phase 3.

### Recommendation

Implement this proposal now to fix CE0463 immediately, then evolve toward the full schema registry over time. The `.mpk` parsing code created here becomes the foundation for the registry's widget schema loader.
