# Go Library API Reference

The `modelsdk-go` library provides programmatic access to Mendix projects from Go code. This is the underlying library that powers `mxcli`.

## Installation

```bash
go get github.com/mendixlabs/mxcli
```

## Quick Start

### Reading a Project

```go
package main

import (
    "fmt"
    "github.com/mendixlabs/mxcli"
)

func main() {
    // open a Mendix project
    reader, err := modelsdk.Open("/path/to/MyApp.mpr")
    if err != nil {
        panic(err)
    }
    defer reader.Close()

    // list all modules
    modules, _ := reader.ListModules()
    for _, m := range modules {
        fmt.Printf("module: %s\n", m.Name)
    }

    // get domain model for a module
    dm, _ := reader.GetDomainModel(modules[0].ID)
    for _, entity := range dm.Entities {
        fmt.Printf("  entity: %s\n", entity.Name)
        for _, attr := range entity.Attributes {
            fmt.Printf("    - %s: %s\n", attr.Name, attr.Type.GetTypeName())
        }
    }

    // list microflows
    microflows, _ := reader.ListMicroflows()
    fmt.Printf("Total microflows: %d\n", len(microflows))

    // list pages
    pages, _ := reader.ListPages()
    fmt.Printf("Total pages: %d\n", len(pages))
}
```

### Modifying a Project

```go
package main

import (
    "github.com/mendixlabs/mxcli"
)

func main() {
    // open for writing
    writer, err := modelsdk.OpenForWriting("/path/to/MyApp.mpr")
    if err != nil {
        panic(err)
    }
    defer writer.Close()

    reader := writer.Reader()
    modules, _ := reader.ListModules()
    dm, _ := reader.GetDomainModel(modules[0].ID)

    // create a new entity
    customer := modelsdk.NewEntity("Customer")
    writer.CreateEntity(dm.ID, customer)

    // add attributes
    writer.AddAttribute(dm.ID, customer.ID, modelsdk.NewStringAttribute("Name", 200))
    writer.AddAttribute(dm.ID, customer.ID, modelsdk.NewStringAttribute("Email", 254))
    writer.AddAttribute(dm.ID, customer.ID, modelsdk.NewBooleanAttribute("IsActive"))
    writer.AddAttribute(dm.ID, customer.ID, modelsdk.NewDateTimeAttribute("CreatedDate", true))

    // create another entity
    order := modelsdk.NewEntity("Order")
    writer.CreateEntity(dm.ID, order)

    // create an association
    assoc := modelsdk.NewAssociation("Customer_Order", customer.ID, order.ID)
    writer.CreateAssociation(dm.ID, assoc)
}
```

### High-Level Fluent API

The `api/` package provides a simplified, fluent API inspired by the Mendix Web Extensibility Model API:

```go
package main

import (
    "github.com/mendixlabs/mxcli/api"
)

func main() {
    // api.Open opens the project for reading and writing and owns the
    // connection; api.New(b) wraps a backend the caller already owns.
    modelAPI, err := api.Open("/path/to/MyApp.mpr")
    if err != nil {
        panic(err)
    }
    defer modelAPI.Close()

    // set the current module context
    module, _ := modelAPI.Modules.Get("MyModule")
    modelAPI.SetModule(module)

    // create entity with fluent builder
    customer, _ := modelAPI.DomainModels.CreateEntity("Customer").
        persistent().
        WithStringAttribute("Name", 100).
        WithStringAttribute("Email", 254).
        WithIntegerAttribute("Age").
        WithBooleanAttribute("IsActive").
        WithDateTimeAttribute("CreatedDate").
        build()

    // create another entity
    order, _ := modelAPI.DomainModels.CreateEntity("Order").
        persistent().
        WithDecimalAttribute("TotalAmount").
        WithDateTimeAttribute("OrderDate").
        build()

    // create association between entities
    _, _ = modelAPI.DomainModels.CreateAssociation("Customer_Orders").
        from("Customer").
        to("Order").
        OneToMany().
        build()

    // create enumeration
    _, _ = modelAPI.Enumerations.CreateEnumeration("OrderStatus").
        WithValue("Pending", "Pending").
        WithValue("Processing", "Processing").
        WithValue("Completed", "Completed").
        WithValue("Cancelled", "Cancelled").
        build()

    // create microflow
    _, _ = modelAPI.Microflows.CreateMicroflow("ACT_ProcessOrder").
        WithParameter("Order", "MyModule.Order").
        WithStringParameter("message").
        ReturnsBoolean().
        build()
}
```

## API Reference

### Core Types

| Type | Description |
|------|-------------|
| `modelsdk.ID` | Unique identifier for model elements (UUID) |
| `modelsdk.Module` | Represents a Mendix module |
| `modelsdk.Project` | Represents a Mendix project |
| `modelsdk.DomainModel` | Contains entities and associations |
| `modelsdk.Entity` | An entity in the domain model |
| `modelsdk.Attribute` | An attribute of an entity |
| `modelsdk.Association` | A relationship between entities |
| `modelsdk.Microflow` | A microflow (server-side logic) |
| `modelsdk.Nanoflow` | A nanoflow (client-side logic) |
| `modelsdk.Page` | A page in the UI |
| `modelsdk.Layout` | A page layout template |

### Reader Methods

```go
// open a project
reader, _ := modelsdk.Open("path/to/project.mpr")
defer reader.Close()

// Metadata
reader.Path()                    // get file path
reader.Version()                 // get MPR version (1 or 2)
reader.GetMendixVersion()        // get Mendix Studio Pro version

// modules
reader.ListModules()             // list all modules
reader.GetModule(id)             // get module by ID
reader.GetModuleByName(name)     // get module by name

// Domain models
reader.ListDomainModels()        // list all domain models
reader.GetDomainModel(moduleID)  // get domain model for module

// microflows & nanoflows
reader.ListMicroflows()          // list all microflows
reader.GetMicroflow(id)          // get microflow by ID
reader.ListNanoflows()           // list all nanoflows
reader.GetNanoflow(id)           // get nanoflow by ID

// pages & layouts
reader.ListPages()               // list all pages
reader.GetPage(id)               // get page by ID
reader.ListLayouts()             // list all layouts
reader.GetLayout(id)             // get layout by ID

// Other
reader.ListEnumerations()        // list all enumerations
reader.ListConstants()           // list all constants
reader.ListScheduledEvents()     // list all scheduled events
reader.ExportJSON()              // export entire model as json
```

### Writer Methods

```go
// open for writing
writer, _ := modelsdk.OpenForWriting("path/to/project.mpr")
defer writer.Close()

// access the reader
reader := writer.Reader()

// modules
writer.CreateModule(module)
writer.UpdateModule(module)
writer.DeleteModule(id)

// entities
writer.CreateEntity(domainModelID, entity)
writer.UpdateEntity(domainModelID, entity)
writer.DeleteEntity(domainModelID, entityID)

// attributes
writer.AddAttribute(domainModelID, entityID, attribute)

// associations
writer.CreateAssociation(domainModelID, association)
writer.DeleteAssociation(domainModelID, associationID)

// microflows & nanoflows
writer.CreateMicroflow(microflow)
writer.UpdateMicroflow(microflow)
writer.DeleteMicroflow(id)
writer.CreateNanoflow(nanoflow)
writer.UpdateNanoflow(nanoflow)
writer.DeleteNanoflow(id)

// pages & layouts
writer.CreatePage(page)
writer.UpdatePage(page)
writer.DeletePage(id)
writer.CreateLayout(layout)
writer.UpdateLayout(layout)
writer.DeleteLayout(id)

// Other
writer.CreateEnumeration(enumeration)
writer.CreateConstant(constant)
```

### Helper Functions

```go
// create attributes
modelsdk.NewStringAttribute(name, length)
modelsdk.NewIntegerAttribute(name)
modelsdk.NewDecimalAttribute(name)
modelsdk.NewBooleanAttribute(name)
modelsdk.NewDateTimeAttribute(name, localize)
modelsdk.NewEnumerationAttribute(name, enumID)

// create entities
modelsdk.NewEntity(name)                 // Persistable entity
modelsdk.NewNonPersistableEntity(name)   // non-persistable entity

// create associations
modelsdk.NewAssociation(name, parentID, childID)      // reference (1:N)
modelsdk.NewReferenceSetAssociation(name, p, c)       // reference set (M:N)

// create flows
modelsdk.NewMicroflow(name)
modelsdk.NewNanoflow(name)

// create pages
modelsdk.NewPage(name)

// generate IDs
modelsdk.GenerateID()
```

### Fluent API Namespaces

| Namespace | Description |
|-----------|-------------|
| `modelAPI.DomainModels` | Create/modify entities, attributes, associations |
| `modelAPI.Enumerations` | Create/modify enumerations and values |
| `modelAPI.Microflows` | Create microflows with parameters and return types |
| `modelAPI.Pages` | Create pages with widgets (DataView, TextBox, etc.) |
| `modelAPI.Modules` | List and retrieve modules |

## Package Structure

```
github.com/mendixlabs/mxcli/
├── modelsdk.go          # Main package with convenience functions
├── model/               # Core model types (ID, module, project, etc.)
├── api/                 # High-level fluent api (builders)
│   ├── api.go           # ModelAPI entry point
│   ├── domainmodels.go  # EntityBuilder, AssociationBuilder
│   ├── enumerations.go  # EnumerationBuilder
│   ├── microflows.go    # MicroflowBuilder
│   ├── pages.go         # PageBuilder, widget builders
│   └── modules.go       # ModulesAPI
├── sdk/
│   ├── domainmodel/     # Domain model types (entity, attribute, association)
│   ├── microflows/      # microflow and nanoflow types
│   ├── pages/           # page, layout, and widget types
│   └── mpr/             # MPR file reader and writer
└── examples/            # Example applications
```

## MPR File Format

Mendix projects are stored in `.mpr` files which are SQLite databases containing BSON-encoded model elements.

### MPR v1 (Mendix < 10.18)
- Single `.mpr` file containing all model data
- Documents stored as BSON blobs in SQLite

### MPR v2 (Mendix >= 10.18)
- `.mpr` file contains references and metadata
- `mprcontents/` folder contains individual document files
- Better for Git versioning and large projects

The library automatically detects and handles both formats.

## Model Structure

```
project
├── modules
│   ├── Domain model
│   │   ├── entities
│   │   │   ├── attributes
│   │   │   ├── Indexes
│   │   │   ├── access rules
│   │   │   ├── validation rules
│   │   │   └── event Handlers
│   │   ├── associations
│   │   └── Annotations
│   ├── microflows
│   │   ├── parameters
│   │   └── Activities & Flows
│   ├── nanoflows
│   ├── pages
│   │   ├── widgets
│   │   └── data Sources
│   ├── layouts
│   ├── snippets
│   ├── enumerations
│   ├── constants
│   ├── Scheduled events
│   └── java actions
└── project Documents
```

## Examples

### Read Project Information

```bash
cd examples/read_project
go run main.go /path/to/MyApp.mpr
```

### Modify Project

```bash
cd examples/modify_project
go run main.go /path/to/MyApp.mpr
```

**Warning**: Always backup your `.mpr` file before modifying it!

## Comparison with Official SDK

| Feature | Mendix Model SDK (TypeScript) | modelsdk-go |
|---------|-------------------------------|-------------|
| Language | TypeScript/JavaScript | Go |
| Runtime | Node.js | Native binary |
| Cloud Required | Yes (Platform API) | No |
| Local Files | No | Yes |
| Real-time Collaboration | Yes | No |
| Read Operations | Yes | Yes |
| Write Operations | Yes | Yes |
| Type Safety | Yes (TypeScript) | Yes (Go) |
| CLI Tool | No | Yes (mxcli) |
| SQL-like DSL | No | Yes (MDL) |

## Resources

- [Mendix Model SDK Documentation](https://docs.mendix.com/apidocs-mxsdk/mxsdk/)
- [Mendix Metamodel Documentation](https://docs.mendix.com/apidocs-mxsdk/mxsdk/mendix-metamodel/)
- [MPR File Format Discussion](https://community.mendix.com/link/space/studio-pro/questions/86892)
