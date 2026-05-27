---
title: High-Level API for modelsdk-go
status: done
---

# Proposal: High-Level API for modelsdk-go

**Status: IMPLEMENTED** (January 2026)

The core API has been implemented in the `api/` package with:
- `DomainModelsAPI` - EntityBuilder, AssociationBuilder, AttributeBuilder
- `EnumerationsAPI` - EnumerationBuilder, EnumValueBuilder
- `MicroflowsAPI` - MicroflowBuilder with parameters and return types
- `PagesAPI` - PageBuilder, widget builders (TextBox, CheckBox, DropDown, etc.)
- `ModulesAPI` - Module retrieval

See `api/api_integration_test.go` for working examples.

---

## Overview

This proposal outlines a simplified, high-level API layer for modelsdk-go, inspired by the [Mendix Web Extensibility Model API](https://docs.mendix.com/apidocs-mxsdk/apidocs/web-extensibility-api-11/model-api/). The goal is to provide a more intuitive, fluent interface that abstracts away low-level details like BSON serialization, UUID generation, and TypeName constants.

## Motivation

The current modelsdk-go API requires explicit handling of:
- UUID generation for every element
- TypeName strings for BSON serialization
- Complex nested struct initialization
- ContainerID management for parent-child relationships

Compare the current low-level approach (see `examples/create_page/main.go`):

```go
// Current: 40+ lines to create a simple page with one text box
page := &pages.Page{
    BaseElement: model.BaseElement{
        ID:       model.ID(mpr.GenerateID()),
        TypeName: "pages$page",
    },
    Name:        "Customer_Edit",
    ContainerID: targetModule.ID,
    title: &model.Text{
        BaseElement: model.BaseElement{
            ID:       model.ID(mpr.GenerateID()),
            TypeName: "Texts$text",
        },
        Translations: map[string]string{"en_US": "Edit Customer"},
    },
}
// ... plus 30+ more lines for layout, widgets, etc.
```

With a high-level API:

```go
// Proposed: ~10 lines for the same result
api := modelsdk.GetModelAPI(writer)

page := api.Pages.CreatePage("Customer_Edit").
    InModule(targetModule).
    WithTitle("Edit Customer").
    WithLayout("Atlas_Default").
    build()

dataview := api.Pages.CreateDataView().
    WithEntity("Customer").
    FromParameter("Customer").
    AddTo(page, "Main")

api.Pages.CreateTextBox("Name").
    WithLabel("Name").
    WithAttribute("Customer.Name").
    AddTo(dataview)
```

## Design Principles

1. **Fluent Builder Pattern**: Chain method calls for readable, discoverable APIs
2. **Sensible Defaults**: Auto-generate IDs, infer TypeNames, set common defaults
3. **Namespace Organization**: Group related operations (Pages, DomainModels, Microflows)
4. **Type Safety**: Use Go's type system to prevent invalid configurations
5. **Incremental Adoption**: High-level API wraps low-level types; users can mix both

## Proposed API Structure

### Entry Point

```go
package modelsdk

// GetModelAPI returns the high-level api for a writable project
func GetModelAPI(writer *mpr.Writer) *ModelAPI

type ModelAPI struct {
    pages        *PagesAPI
    DomainModels *DomainModelsAPI
    microflows   *MicroflowsAPI
    enumerations *EnumerationsAPI
    modules      *ModulesAPI
}
```

### Pages API

```go
package modelsdk

type PagesAPI struct {
    writer *mpr.Writer
}

// page Builder
type PageBuilder struct {
    api  *PagesAPI
    page *pages.Page
}

func (api *PagesAPI) CreatePage(name string) *PageBuilder
func (b *PageBuilder) InModule(module *model.Module) *PageBuilder
func (b *PageBuilder) WithTitle(title string) *PageBuilder
func (b *PageBuilder) WithTitleTranslations(translations map[string]string) *PageBuilder
func (b *PageBuilder) WithLayout(layoutName string) *PageBuilder
func (b *PageBuilder) WithURL(url string) *PageBuilder
func (b *PageBuilder) WithParameter(name string, entityName string) *PageBuilder
func (b *PageBuilder) build() (*pages.Page, error)

// dataview Builder
type DataViewBuilder struct {
    api      *PagesAPI
    dataview *pages.DataView
}

func (api *PagesAPI) CreateDataView() *DataViewBuilder
func (b *DataViewBuilder) WithName(name string) *DataViewBuilder
func (b *DataViewBuilder) WithEntity(entityName string) *DataViewBuilder
func (b *DataViewBuilder) FromParameter(paramName string) *DataViewBuilder
func (b *DataViewBuilder) FromMicroflow(microflowName string) *DataViewBuilder
func (b *DataViewBuilder) AddTo(parent interface{}, placeholderName string) *DataViewBuilder
func (b *DataViewBuilder) build() *pages.DataView

// widget Builders
func (api *PagesAPI) CreateTextBox(name string) *TextBoxBuilder
func (api *PagesAPI) CreateTextArea(name string) *TextAreaBuilder
func (api *PagesAPI) CreateDatePicker(name string) *DatePickerBuilder
func (api *PagesAPI) CreateCheckBox(name string) *CheckBoxBuilder
func (api *PagesAPI) CreateDropDown(name string) *DropDownBuilder
func (api *PagesAPI) CreateComboBox(name string) *ComboBoxBuilder
func (api *PagesAPI) CreateButton(caption string) *ButtonBuilder
func (api *PagesAPI) CreateLayoutGrid() *LayoutGridBuilder
func (api *PagesAPI) CreateContainer() *ContainerBuilder
func (api *PagesAPI) CreateListView() *ListViewBuilder
func (api *PagesAPI) CreateDataGrid2() *DataGrid2Builder

// Common widget builder interface
type WidgetBuilder interface {
    WithLabel(label string) WidgetBuilder
    WithAttribute(attributePath string) WidgetBuilder
    AddTo(parent interface{}) WidgetBuilder
    build() pages.Widget
}
```

### Domain Models API

```go
type DomainModelsAPI struct {
    writer *mpr.Writer
}

// entity Builder
type EntityBuilder struct {
    api    *DomainModelsAPI
    entity *domainmodel.Entity
}

func (api *DomainModelsAPI) CreateEntity(name string) *EntityBuilder
func (b *EntityBuilder) InModule(module *model.Module) *EntityBuilder
func (b *EntityBuilder) persistent() *EntityBuilder
func (b *EntityBuilder) NonPersistent() *EntityBuilder
func (b *EntityBuilder) WithGeneralization(entityName string) *EntityBuilder
func (b *EntityBuilder) WithAttribute(name string, dataType string) *EntityBuilder
func (b *EntityBuilder) WithStringAttribute(name string, length int) *EntityBuilder
func (b *EntityBuilder) WithIntegerAttribute(name string) *EntityBuilder
func (b *EntityBuilder) WithDecimalAttribute(name string) *EntityBuilder
func (b *EntityBuilder) WithBooleanAttribute(name string) *EntityBuilder
func (b *EntityBuilder) WithDateTimeAttribute(name string) *EntityBuilder
func (b *EntityBuilder) WithEnumerationAttribute(name string, enumName string) *EntityBuilder
func (b *EntityBuilder) build() (*domainmodel.Entity, error)

// association Builder
type AssociationBuilder struct {
    api         *DomainModelsAPI
    association *domainmodel.Association
}

func (api *DomainModelsAPI) CreateAssociation(name string) *AssociationBuilder
func (b *AssociationBuilder) from(entityName string) *AssociationBuilder
func (b *AssociationBuilder) to(entityName string) *AssociationBuilder
func (b *AssociationBuilder) OneToMany() *AssociationBuilder
func (b *AssociationBuilder) ManyToMany() *AssociationBuilder
func (b *AssociationBuilder) OneToOne() *AssociationBuilder
func (b *AssociationBuilder) WithDeleteBehavior(parent, child string) *AssociationBuilder
func (b *AssociationBuilder) build() (*domainmodel.Association, error)
```

### Microflows API

```go
type MicroflowsAPI struct {
    writer *mpr.Writer
}

// microflow Builder
type MicroflowBuilder struct {
    api       *MicroflowsAPI
    microflow *microflows.Microflow
    current   microflows.MicroflowObject // Current position for chaining
}

func (api *MicroflowsAPI) CreateMicroflow(name string) *MicroflowBuilder
func (b *MicroflowBuilder) InModule(module *model.Module) *MicroflowBuilder
func (b *MicroflowBuilder) WithParameter(name string, entityName string) *MicroflowBuilder
func (b *MicroflowBuilder) WithReturnType(typeName string) *MicroflowBuilder
func (b *MicroflowBuilder) ReturnsBoolean() *MicroflowBuilder
func (b *MicroflowBuilder) ReturnsString() *MicroflowBuilder
func (b *MicroflowBuilder) ReturnsList(entityName string) *MicroflowBuilder
func (b *MicroflowBuilder) ReturnsObject(entityName string) *MicroflowBuilder

// activity builders (chainable)
func (b *MicroflowBuilder) CreateObject(entityName string) *CreateObjectBuilder
func (b *MicroflowBuilder) ChangeObject(variableName string) *ChangeObjectBuilder
func (b *MicroflowBuilder) RetrieveByAssociation(variableName, associationName string) *RetrieveBuilder
func (b *MicroflowBuilder) RetrieveFromDatabase(entityName string) *RetrieveBuilder
func (b *MicroflowBuilder) DeleteObject(variableName string) *MicroflowBuilder
func (b *MicroflowBuilder) CommitObject(variableName string) *MicroflowBuilder
func (b *MicroflowBuilder) CallMicroflow(microflowName string) *CallMicroflowBuilder
func (b *MicroflowBuilder) ShowPage(pageName string) *ShowPageBuilder
func (b *MicroflowBuilder) ShowMessage(message string) *ShowMessageBuilder
func (b *MicroflowBuilder) log(message string) *LogBuilder
func (b *MicroflowBuilder) decision(expression string) *DecisionBuilder
func (b *MicroflowBuilder) loop(variableName string) *LoopBuilder

func (b *MicroflowBuilder) end() *MicroflowBuilder
func (b *MicroflowBuilder) EndWithReturn(expression string) *MicroflowBuilder
func (b *MicroflowBuilder) build() (*microflows.Microflow, error)

// Example usage:
// api.Microflows.CreateMicroflow("ACT_Customer_Create").
//     WithParameter("Customer", "MyModule.Customer").
//     CreateObject("MyModule.Order").OutputAs("$NewOrder").
//     ChangeObject("$NewOrder").Set("Customer", "$Customer").
//     CommitObject("$NewOrder").
//     ShowPage("MyModule.Order_Edit").WithObject("$NewOrder").
//     build()
```

### Enumerations API

```go
type EnumerationsAPI struct {
    writer *mpr.Writer
}

type EnumerationBuilder struct {
    api  *EnumerationsAPI
    enum *domainmodel.Enumeration
}

func (api *EnumerationsAPI) CreateEnumeration(name string) *EnumerationBuilder
func (b *EnumerationBuilder) InModule(module *model.Module) *EnumerationBuilder
func (b *EnumerationBuilder) WithValue(name string, caption string) *EnumerationBuilder
func (b *EnumerationBuilder) WithValues(values map[string]string) *EnumerationBuilder
func (b *EnumerationBuilder) build() (*domainmodel.Enumeration, error)
```

## Modifying Existing Documents

The API supports not just creation but also retrieval, modification, and removal of existing elements.

### Retrieving Existing Elements

```go
api := modelsdk.GetModelAPI(writer)

// get existing page by qualified name
page, err := api.Pages.GetPage("MyModule.Customer_Edit")

// get existing entity
entity, err := api.DomainModels.GetEntity("MyModule.Customer")

// get existing microflow
microflow, err := api.Microflows.GetMicroflow("MyModule.ACT_Customer_Save")

// get existing enumeration
enum, err := api.Enumerations.GetEnumeration("MyModule.OrderStatus")
```

### Adding Elements to Existing Documents

```go
// add widget to existing page
page, _ := api.Pages.GetPage("MyModule.Customer_Edit")

// find the dataview by name and add a new widget
dataview := page.FindWidget("customerDataView")

api.Pages.CreateTextBox("phoneTextBox").
    WithLabel("Phone").
    WithAttribute("MyModule.Customer.Phone").
    AddTo(dataview)

// add attribute to existing entity
entity, _ := api.DomainModels.GetEntity("MyModule.Customer")

api.DomainModels.AddAttribute(entity).
    Name("Phone").
    string(20).
    build()

// add activity to existing microflow
microflow, _ := api.Microflows.GetMicroflow("MyModule.ACT_Customer_Save")

api.Microflows.InsertActivity(microflow).
    after("createObject1").  // insert after specific activity
    log("Customer saved: " + "$Customer/Name").
    build()

// add value to existing enumeration
enum, _ := api.Enumerations.GetEnumeration("MyModule.OrderStatus")

api.Enumerations.AddValue(enum).
    Name("Cancelled").
    caption("Cancelled").
    build()
```

### Removing Elements

```go
// remove widget from page
page, _ := api.Pages.GetPage("MyModule.Customer_Edit")
err := api.Pages.RemoveWidget(page, "phoneTextBox")

// remove attribute from entity
entity, _ := api.DomainModels.GetEntity("MyModule.Customer")
err := api.DomainModels.RemoveAttribute(entity, "Phone")

// remove activity from microflow
microflow, _ := api.Microflows.GetMicroflow("MyModule.ACT_Customer_Save")
err := api.Microflows.RemoveActivity(microflow, "logMessage1")

// remove value from enumeration
enum, _ := api.Enumerations.GetEnumeration("MyModule.OrderStatus")
err := api.Enumerations.RemoveValue(enum, "Cancelled")
```

### Modifying Existing Elements

```go
// modify widget properties
page, _ := api.Pages.GetPage("MyModule.Customer_Edit")
textbox := page.FindWidget("nameTextBox").(*pages.TextBox)

api.Pages.ModifyWidget(textbox).
    WithLabel("full Name").        // change label
    WithPlaceholder("Enter name"). // add placeholder
    apply()

// modify entity attribute
entity, _ := api.DomainModels.GetEntity("MyModule.Customer")
attr := entity.FindAttribute("Name")

api.DomainModels.ModifyAttribute(attr).
    WithLength(200).              // change from 100 to 200
    required().                   // add not null
    apply()

// modify microflow activity
microflow, _ := api.Microflows.GetMicroflow("MyModule.ACT_Customer_Save")
activity := microflow.FindActivity("showMessage1")

api.Microflows.ModifyActivity(activity).
    WithMessage("Customer updated successfully").
    apply()
```

### Navigation and Search Helpers

```go
// find widgets by type
page, _ := api.Pages.GetPage("MyModule.Customer_Edit")

allTextBoxes := page.FindWidgetsByType("textbox")
allButtons := page.FindWidgetsByType("actionbutton")

// find by attribute path
widgetsForEmail := page.FindWidgetsByAttribute("MyModule.Customer.Email")

// find container hierarchy
dataview := page.FindWidget("customerDataView")
parent := dataView.Parent()           // returns containing widget
children := dataView.Children()       // returns child widgets
footer := dataView.FooterWidgets()    // dataview-specific

// find pages that use a specific entity
pagesWithCustomer := api.Pages.FindPagesWithEntity("MyModule.Customer")

// find microflows that call another microflow
callers := api.Microflows.FindCallers("MyModule.SUB_ValidateCustomer")
```

### Bulk Operations

```go
// add multiple attributes at once
entity, _ := api.DomainModels.GetEntity("MyModule.Customer")

api.DomainModels.BatchModify(entity).
    AddStringAttribute("Phone", 20).
    AddStringAttribute("Address", 500).
    RemoveAttribute("OldField").
    ModifyAttribute("Email").WithLength(300).
    apply()

// Reorder widgets in a container
page, _ := api.Pages.GetPage("MyModule.Customer_Edit")
dataview := page.FindWidget("customerDataView")

api.Pages.ReorderWidgets(dataview, []string{
    "nameTextBox",
    "emailTextBox",
    "phoneTextBox",    // Moved up
    "birthDatePicker",
    "activeCheckBox",
})

// move widget to different container
api.Pages.MoveWidget(phoneTextBox, newContainer)
```

### Extended API Interfaces

The complete API interfaces including modification support:

```go
type PagesAPI struct {
    writer *mpr.Writer
}

// Creation
func (api *PagesAPI) CreatePage(name string) *PageBuilder
func (api *PagesAPI) CreateDataView() *DataViewBuilder
func (api *PagesAPI) CreateTextBox(name string) *TextBoxBuilder
// ... other create methods ...

// Retrieval
func (api *PagesAPI) GetPage(qualifiedName string) (*pages.Page, error)
func (api *PagesAPI) GetLayout(qualifiedName string) (*pages.Layout, error)
func (api *PagesAPI) GetSnippet(qualifiedName string) (*pages.Snippet, error)
func (api *PagesAPI) FindPagesWithEntity(entityName string) []*pages.Page
func (api *PagesAPI) FindPagesWithMicroflow(microflowName string) []*pages.Page

// Modification
func (api *PagesAPI) ModifyWidget(widget pages.Widget) *WidgetModifier
func (api *PagesAPI) RemoveWidget(parent interface{}, widgetName string) error
func (api *PagesAPI) ReorderWidgets(container interface{}, order []string) error
func (api *PagesAPI) MoveWidget(widget pages.Widget, newParent interface{}) error

type DomainModelsAPI struct {
    writer *mpr.Writer
}

// Creation
func (api *DomainModelsAPI) CreateEntity(name string) *EntityBuilder
func (api *DomainModelsAPI) CreateAssociation(name string) *AssociationBuilder

// Retrieval
func (api *DomainModelsAPI) GetEntity(qualifiedName string) (*domainmodel.Entity, error)
func (api *DomainModelsAPI) GetAssociation(qualifiedName string) (*domainmodel.Association, error)
func (api *DomainModelsAPI) FindEntitiesWithAttribute(attrName string) []*domainmodel.Entity

// Modification
func (api *DomainModelsAPI) AddAttribute(entity *domainmodel.Entity) *AttributeBuilder
func (api *DomainModelsAPI) ModifyAttribute(attr *domainmodel.Attribute) *AttributeModifier
func (api *DomainModelsAPI) RemoveAttribute(entity *domainmodel.Entity, attrName string) error
func (api *DomainModelsAPI) BatchModify(entity *domainmodel.Entity) *EntityBatchModifier

type MicroflowsAPI struct {
    writer *mpr.Writer
}

// Creation
func (api *MicroflowsAPI) CreateMicroflow(name string) *MicroflowBuilder

// Retrieval
func (api *MicroflowsAPI) GetMicroflow(qualifiedName string) (*microflows.Microflow, error)
func (api *MicroflowsAPI) FindCallers(microflowName string) []*microflows.Microflow
func (api *MicroflowsAPI) FindMicroflowsWithEntity(entityName string) []*microflows.Microflow

// Modification
func (api *MicroflowsAPI) InsertActivity(mf *microflows.Microflow) *ActivityInserter
func (api *MicroflowsAPI) ModifyActivity(activity microflows.MicroflowObject) *ActivityModifier
func (api *MicroflowsAPI) RemoveActivity(mf *microflows.Microflow, activityName string) error
func (api *MicroflowsAPI) ReconnectFlow(from, to string) error

type EnumerationsAPI struct {
    writer *mpr.Writer
}

// Creation
func (api *EnumerationsAPI) CreateEnumeration(name string) *EnumerationBuilder

// Retrieval
func (api *EnumerationsAPI) GetEnumeration(qualifiedName string) (*domainmodel.Enumeration, error)

// Modification
func (api *EnumerationsAPI) AddValue(enum *domainmodel.Enumeration) *EnumValueBuilder
func (api *EnumerationsAPI) ModifyValue(value *domainmodel.EnumerationValue) *EnumValueModifier
func (api *EnumerationsAPI) RemoveValue(enum *domainmodel.Enumeration, valueName string) error
func (api *EnumerationsAPI) ReorderValues(enum *domainmodel.Enumeration, order []string) error
```

## Example: Modifying an Existing Page

```go
package main

import (
    "fmt"
    "github.com/mendixlabs/mxcli"
    "github.com/mendixlabs/mxcli/sdk/mpr"
)

func main() {
    writer, _ := mpr.NewWriter("app.mpr")
    defer writer.Close()

    api := modelsdk.GetModelAPI(writer)

    // 1. add a new attribute to an existing entity
    entity, _ := api.DomainModels.GetEntity("MyModule.Customer")

    api.DomainModels.AddAttribute(entity).
        Name("Phone").
        string(20).
        build()

    // 2. find all pages that display Customer and add the new field
    pages := api.Pages.FindPagesWithEntity("MyModule.Customer")

    for _, page := range pages {
        // find DataViews bound to Customer
        dataViews := page.FindWidgetsByType("dataview")

        for _, dv := range dataViews {
            dataview := dv.(*pages.DataView)
            if dataView.DataSource.EntityName == "MyModule.Customer" {
                // add Phone field after Email
                api.Pages.CreateTextBox("phoneTextBox").
                    WithLabel("Phone").
                    WithAttribute("MyModule.Customer.Phone").
                    InsertAfter(dataview, "emailTextBox")
            }
        }
    }

    // 3. update a microflow to validate the new field
    microflow, _ := api.Microflows.GetMicroflow("MyModule.ACT_Customer_Save")

    // insert validation before the commit
    api.Microflows.InsertActivity(microflow).
        before("commitObject1").
        decision("$Customer/Phone != empty").
            OnTrue().Continue().
            OnFalse().ShowValidationMessage("Phone is required").End().
        build()

    fmt.Println("Updated entity, pages, and microflow")
}
```

## Implementation Strategy

### Phase 1: Core Infrastructure
1. Create `api/` package for high-level API
2. Implement `ModelAPI` factory function
3. Implement ID generation and TypeName resolution helpers
4. Add module resolution and qualified name helpers
5. Implement element lookup/caching for retrieval operations

### Phase 2: Domain Model API
1. `EntityBuilder` with all attribute types
2. `AssociationBuilder` with relationship types
3. `EnumerationBuilder`
4. Integration with existing `writer.CreateEntity()`, `writer.CreateAssociation()`
5. `GetEntity()`, `GetAssociation()` retrieval methods
6. `AddAttribute()`, `ModifyAttribute()`, `RemoveAttribute()` modification methods

### Phase 3: Pages API
1. `PageBuilder` with layout integration
2. Basic widget builders (TextBox, Button, Container)
3. DataView and ListView builders
4. Advanced widget builders (DataGrid2, etc.)
5. `GetPage()` retrieval and widget navigation (`FindWidget`, `FindWidgetsByType`)
6. `ModifyWidget()`, `RemoveWidget()`, `ReorderWidgets()`, `MoveWidget()` modification methods

### Phase 4: Microflows API
1. `MicroflowBuilder` with parameter/return type
2. Basic activity builders (Create, Change, Delete, Commit)
3. Control flow builders (Decision, Loop, Merge)
4. Advanced activities (CallMicroflow, ShowPage, etc.)
5. `GetMicroflow()` retrieval and activity navigation
6. `InsertActivity()`, `ModifyActivity()`, `RemoveActivity()` modification methods

### Phase 5: Search and Cross-Reference APIs
1. `FindPagesWithEntity()`, `FindPagesWithMicroflow()`
2. `FindCallers()`, `FindMicroflowsWithEntity()`
3. `FindEntitiesWithAttribute()`
4. Index building for efficient cross-reference queries

## Package Structure

```
modelsdk-go/
├── api/                          # New high-level api package
│   ├── api.go                    # ModelAPI entry point
│   ├── pages.go                  # PagesAPI and builders
│   ├── pages_widgets.go          # widget builders
│   ├── domainmodels.go           # DomainModelsAPI and builders
│   ├── microflows.go             # MicroflowsAPI and builders
│   ├── microflows_activities.go  # activity builders
│   ├── enumerations.go           # EnumerationsAPI
│   └── helpers.go                # ID generation, name resolution
│
├── sdk/                          # Existing low-level types (unchanged)
│   ├── pages/
│   ├── domainmodel/
│   ├── microflows/
│   └── mpr/
```

## Example: Complete Page Creation

```go
package main

import (
    "github.com/mendixlabs/mxcli"
    "github.com/mendixlabs/mxcli/sdk/mpr"
)

func main() {
    writer, _ := mpr.NewWriter("app.mpr")
    defer writer.Close()

    api := modelsdk.GetModelAPI(writer)
    modules, _ := writer.Reader().ListModules()
    module := modules[0]

    // create entity
    entity, _ := api.DomainModels.CreateEntity("Customer").
        InModule(module).
        persistent().
        WithStringAttribute("Name", 100).
        WithStringAttribute("Email", 200).
        WithDateTimeAttribute("BirthDate").
        WithBooleanAttribute("IsActive").
        build()

    // create page
    page, _ := api.Pages.CreatePage("Customer_Edit").
        InModule(module).
        WithTitle("Edit Customer").
        WithLayout("Atlas_Default").
        WithParameter("Customer", module.Name+".Customer").
        build()

    // add dataview
    dataview := api.Pages.CreateDataView().
        WithEntity(module.Name + ".Customer").
        FromParameter("Customer").
        AddTo(page, "Main").
        build()

    // add widgets
    api.Pages.CreateTextBox("nameTextBox").
        WithLabel("Name").
        WithAttribute(module.Name + ".Customer.Name").
        AddTo(dataview)

    api.Pages.CreateTextBox("emailTextBox").
        WithLabel("Email").
        WithAttribute(module.Name + ".Customer.Email").
        AddTo(dataview)

    api.Pages.CreateDatePicker("birthDatePicker").
        WithLabel("Birth date").
        WithAttribute(module.Name + ".Customer.BirthDate").
        AddTo(dataview)

    api.Pages.CreateCheckBox("activeCheckBox").
        WithLabel("Is Active").
        WithAttribute(module.Name + ".Customer.IsActive").
        AddTo(dataview)

    // add buttons to footer
    api.Pages.CreateButton("Save").
        WithStyle("primary").
        WithSaveAction().ClosePage().
        AddToFooter(dataview)

    api.Pages.CreateButton("cancel").
        WithCancelAction().ClosePage().
        AddToFooter(dataview)
}
```

## Comparison with Mendix Extensibility API

| Mendix Extensibility API | Proposed Go API |
|-------------------------|-----------------|
| `getStudioProApi()` | `modelsdk.GetModelAPI(writer)` |
| `api.pages.createPage()` | `api.Pages.CreatePage()` |
| `api.domainModels.createEntity()` | `api.DomainModels.CreateEntity()` |
| `api.microflows.createMicroflow()` | `api.Microflows.CreateMicroflow()` |
| `IPageApi` interface | `*PagesAPI` struct |
| `createElement<T>(type)` | Type-specific `create*()` methods |
| Promise-based async | Synchronous with error returns |
| TypeScript generics | Go type-specific builders |

## Benefits

1. **Reduced Boilerplate**: 70-80% less code for common operations
2. **Discoverability**: IDE autocomplete guides valid options
3. **Type Safety**: Compile-time checking of builder chains
4. **Readability**: Self-documenting fluent interface
5. **Consistency**: Uniform patterns across all domains
6. **Backwards Compatible**: Low-level API remains available

## Open Questions

1. **Error Handling**: Should builders panic on invalid state, or collect errors?
   - Recommendation: Collect errors and return on `build()` or `apply()`

2. **Immutability**: Should builders be immutable (return new builder) or mutable?
   - Recommendation: Mutable for simplicity, but document non-thread-safety

3. **Validation**: How much validation in builders vs. at write time?
   - Recommendation: Basic validation in builders, full validation on write

4. **Module Context**: Should API track "current module" to avoid repetition?
   - Recommendation: Yes, with `api.SetModule(module)` and per-builder override

5. **Dirty Tracking**: Should the API track which elements have been modified?
   - Recommendation: Yes, track modified elements internally and only write changed documents
   - This improves performance and enables `api.HasChanges()` and `api.GetModifiedElements()`

6. **Automatic Persistence**: Should modifications be persisted immediately or batched?
   - Recommendation: Batch changes, require explicit `api.Save()` or use auto-save on `writer.Close()`
   - Alternative: Immediate persistence with each `build()`/`apply()` call

7. **Undo/Redo**: Should the API support undo/redo for modifications?
   - Recommendation: Not in initial version; consider for future enhancement
   - Could implement via command pattern if needed

8. **Concurrent Modifications**: How to handle concurrent access to the same document?
   - Recommendation: Document as single-threaded; no concurrent modification support
   - Use mutex internally if needed for safety

9. **Reference Integrity**: Should removing an entity also remove/update referencing widgets/microflows?
   - Recommendation: No cascading deletes by default; provide `RemoveWithReferences()` variant
   - Warn user about broken references via validation

10. **Widget Positioning**: How to specify exact widget positions in modification operations?
    - Recommendation: Use `InsertAfter()`, `InsertBefore()`, `InsertAt(index)` methods
    - Default `AddTo()` appends to end of container

## Next Steps

1. Review and approve this proposal
2. Create `api/` package with core infrastructure
3. Implement DomainModelsAPI (simplest domain)
4. Implement PagesAPI (most requested)
5. Implement MicroflowsAPI (most complex)
6. Update examples to use high-level API
7. Document API in README.md
