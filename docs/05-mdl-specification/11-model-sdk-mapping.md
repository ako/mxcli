# MDL to Model SDK Mapping

This document describes how MDL constructs map to the modelsdk-go library types and API.

## Table of Contents

1. [Package Structure](#package-structure)
2. [Entity Mapping](#entity-mapping)
3. [Attribute Mapping](#attribute-mapping)
4. [Validation Rule Mapping](#validation-rule-mapping)
5. [Index Mapping](#index-mapping)
6. [Association Mapping](#association-mapping)
7. [Enumeration Mapping](#enumeration-mapping)
8. [API Usage Examples](#api-usage-examples)

---

## Package Structure

The modelsdk-go library is organized into packages:

| Package | Description |
|---------|-------------|
| `modelsdk` | Main API: `open()`, `OpenForWriting()`, helpers |
| `model` | Core types: `ID`, `module`, `Element`, `Point` |
| `api` | High-level fluent API: `ModelAPI`, builders for entities, microflows, pages |
| `sdk/domainmodel` | Domain model types: `entity`, `attribute`, `association` |
| `sdk/microflows` | Microflow types (60+ activity types) |
| `sdk/pages` | Page and widget types (50+ widgets) |
| `sdk/widgets` | Embedded widget templates for pluggable widgets |
| `sdk/mpr` | MPR file reading/writing, BSON parsing |
| `sql` | External database connectivity (PostgreSQL, Oracle, SQL Server) |
| `mdl/executor` | MDL statement execution engine |
| `mdl/catalog` | SQLite-based catalog for cross-reference queries |
| `mdl/linter` | Linting framework with built-in and Starlark rules |

---

## Entity Mapping

### MDL Entity
```sql
/** Customer entity */
@position(100, 200)
create persistent entity Sales.Customer (
  Name: string(200) not null
);
```

### Go SDK Types

```go
import (
    "github.com/mendixlabs/mxcli/sdk/domainmodel"
    "github.com/mendixlabs/mxcli/model"
)

entity := &domainmodel.Entity{
    BaseElement: model.BaseElement{
        ID:       model.ID("generated-uuid"),
        TypeName: "DomainModels$EntityImpl",
    },
    Name:          "Customer",
    documentation: "Customer entity",
    Location:      model.Point{X: 100, Y: 200},
    Persistable:   true,
    attributes:    []*domainmodel.Attribute{...},
}
```

### Entity Type Mapping

| MDL | Go Field | Value |
|-----|----------|-------|
| `persistent` | `Persistable` | `true` |
| `non-persistent` | `Persistable` | `false` |
| `view` | `Persistable`, `source` | `false`, `"OqlView"` |
| `@position(x,y)` | `Location` | `model.Point{X: x, Y: y}` |
| `/** doc */` | `documentation` | `"doc"` |

### Entity Struct Definition

```go
// domainmodel/domainmodel.go
type entity struct {
    model.BaseElement
    ContainerID      model.ID    // Parent domain model ID
    Name             string
    documentation    string
    Location         model.Point
    Persistable      bool

    // entity members
    attributes       []*attribute
    Indexes          []*index
    ValidationRules  []*ValidationRule
    AccessRules      []*AccessRule
    EventHandlers    []*EventHandler

    // generalization
    generalization    generalization
    GeneralizationID  model.ID
    GeneralizationRef string      // e.g., "System.User"

    // external/view entity
    source            string      // "OqlViewEntitySource", etc.
    RemoteSource      string
    OqlQuery          string      // for view entities
}
```

---

## Attribute Mapping

### MDL Attribute
```sql
/** Customer name */
Name: string(200) not null error 'Required' default 'Unknown'
```

### Go SDK Types

```go
attr := &domainmodel.Attribute{
    BaseElement: model.BaseElement{
        ID:       model.ID("generated-uuid"),
        TypeName: "DomainModels$attribute",
    },
    Name:          "Name",
    documentation: "Customer name",
    type: &domainmodel.StringAttributeType{
        length: 200,
    },
    value: &domainmodel.AttributeValue{
        type:         "StoredValue",
        DefaultValue: "Unknown",
    },
}
```

### Attribute Type Mapping

| MDL Type | Go Type |
|----------|---------|
| `string` | `*StringAttributeType{length: 200}` |
| `string(n)` | `*StringAttributeType{length: n}` |
| `integer` | `*IntegerAttributeType{}` |
| `long` | `*LongAttributeType{}` |
| `decimal` | `*DecimalAttributeType{}` |
| `boolean` | `*BooleanAttributeType{}` |
| `datetime` | `*DateTimeAttributeType{}` |
| `autonumber` | `*AutoNumberAttributeType{}` |
| `binary` | `*BinaryAttributeType{}` |
| `enumeration(M.E)` | `*EnumerationAttributeType{EnumerationID: id}` |

### Attribute Type Interface

```go
// domainmodel/domainmodel.go
type AttributeType interface {
    GetTypeName() string
}

type StringAttributeType struct {
    model.BaseElement
    length int
}

func (t *StringAttributeType) GetTypeName() string {
    return "string"
}

type IntegerAttributeType struct {
    model.BaseElement
}

func (t *IntegerAttributeType) GetTypeName() string {
    return "integer"
}

// ... similar for other types
```

### Attribute Value

```go
type AttributeValue struct {
    model.BaseElement
    type         string    // "StoredValue" or "CalculatedValue"
    DefaultValue string    // string representation of default
    MicroflowID  model.ID  // for calculated values
}
```

---

## Validation Rule Mapping

### MDL Validation
```sql
Name: string not null error 'Name is required' unique error 'Name must be unique'
```

### Go SDK Types

```go
// required validation
requiredRule := &domainmodel.ValidationRule{
    BaseElement: model.BaseElement{
        ID: model.ID("generated-uuid"),
    },
    AttributeID: attrID,  // or qualified name like "Module.Entity.Attr"
    type:        "required",
    ErrorMessage: &model.Text{
        Translations: map[string]string{
            "en_US": "Name is required",
        },
    },
}

// unique validation
uniqueRule := &domainmodel.ValidationRule{
    BaseElement: model.BaseElement{
        ID: model.ID("generated-uuid"),
    },
    AttributeID:  attrID,
    type:         "unique",
    ErrorMessage: &model.Text{
        Translations: map[string]string{
            "en_US": "Name must be unique",
        },
    },
}
```

### Validation Rule Type Mapping

| MDL Constraint | Go Type Field |
|----------------|---------------|
| `not null` | `type: "required"` |
| `unique` | `type: "unique"` |
| `not null error 'msg'` | `type: "required"`, `ErrorMessage: {...}` |
| `unique error 'msg'` | `type: "unique"`, `ErrorMessage: {...}` |

### ValidationRule Struct

```go
type ValidationRule struct {
    model.BaseElement
    ContainerID  model.ID     // Parent entity ID
    AttributeID  model.ID     // Can be UUID or qualified name
    type         string       // "required", "unique", "range", "regex"
    ErrorMessage *model.Text  // Localized error message
    rule         ValidationRuleInfo  // Additional rule details
}
```

---

## Index Mapping

### MDL Index
```sql
index (Name, CreatedAt desc)
```

### Go SDK Types

```go
index := &domainmodel.Index{
    BaseElement: model.BaseElement{
        ID: model.ID("generated-uuid"),
    },
    attributes: []*domainmodel.IndexAttribute{
        {
            AttributeID: nameAttrID,
            Ascending:   true,
        },
        {
            AttributeID: createdAtAttrID,
            Ascending:   false,
        },
    },
}
```

### Index Struct

```go
type index struct {
    model.BaseElement
    ContainerID  model.ID           // Parent entity ID
    Name         string             // Optional index name
    attributes   []*IndexAttribute  // Indexed columns
    AttributeIDs []model.ID         // Alternative: just IDs
}

type IndexAttribute struct {
    model.BaseElement
    AttributeID model.ID
    Ascending   bool
}
```

### Sort Order Mapping

| MDL | Go Ascending |
|-----|--------------|
| `AttrName` | `true` |
| `AttrName asc` | `true` |
| `AttrName desc` | `false` |

---

## Association Mapping

### MDL Association
```sql
create association Sales.Order_Customer
  from Sales.Customer
  to Sales.Order
  type reference
  owner default
  delete_behavior DELETE_BUT_KEEP_REFERENCES;
```

### Go SDK Types

```go
assoc := &domainmodel.Association{
    BaseElement: model.BaseElement{
        ID:       model.ID("generated-uuid"),
        TypeName: "DomainModels$association",
    },
    Name:     "Order_Customer",
    ParentID: customerEntityID,
    ChildID:  orderEntityID,
    type:     domainmodel.AssociationTypeReference,
    owner:    domainmodel.AssociationOwnerDefault,
    ParentDeleteBehavior: &domainmodel.DeleteBehavior{
        type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences,
    },
}
```

### Association Type Mapping

| MDL | Go Constant |
|-----|-------------|
| `reference` | `AssociationTypeReference` |
| `ReferenceSet` | `AssociationTypeReferenceSet` |

### Owner Mapping

| MDL | Go Constant |
|-----|-------------|
| `default` | `AssociationOwnerDefault` |
| `both` | `AssociationOwnerBoth` |
| `Parent` | (not yet defined) |
| `Child` | (not yet defined) |

### Delete Behavior Mapping

| MDL | Go Constant |
|-----|-------------|
| `DELETE_BUT_KEEP_REFERENCES` | `DeleteBehaviorTypeDeleteMeButKeepReferences` |
| `DELETE_AND_REFERENCES`, `CASCADE` | `DeleteBehaviorTypeDeleteMeAndReferences` |
| `DELETE_IF_NO_REFERENCES`, `PREVENT` | `DeleteBehaviorTypeDeleteMeIfNoReferences` |

### Association Struct

```go
type association struct {
    model.BaseElement
    ContainerID          model.ID
    Name                 string
    documentation        string
    ParentID             model.ID
    ChildID              model.ID
    type                 AssociationType
    owner                AssociationOwner
    ParentConnection     model.Point
    ChildConnection      model.Point
    ParentDeleteBehavior *DeleteBehavior
    ChildDeleteBehavior  *DeleteBehavior
}

type AssociationType string

const (
    AssociationTypeReference    AssociationType = "reference"
    AssociationTypeReferenceSet AssociationType = "ReferenceSet"
)

type AssociationOwner string

const (
    AssociationOwnerDefault AssociationOwner = "default"
    AssociationOwnerBoth    AssociationOwner = "both"
)
```

---

## Enumeration Mapping

### MDL Enumeration
```sql
create enumeration Sales.OrderStatus (
  Draft 'Draft Order',
  Pending 'Pending Approval',
  Approved 'Approved'
);
```

### Go SDK Types

```go
enum := &model.Enumeration{
    BaseElement: model.BaseElement{
        ID:       model.ID("generated-uuid"),
        TypeName: "enumerations$enumeration",
    },
    Name: "OrderStatus",
    values: []*model.EnumerationValue{
        {
            Name: "Draft",
            caption: &model.Text{
                Translations: map[string]string{
                    "en_US": "Draft Order",
                },
            },
        },
        {
            Name: "Pending",
            caption: &model.Text{
                Translations: map[string]string{
                    "en_US": "Pending Approval",
                },
            },
        },
        {
            Name: "Approved",
            caption: &model.Text{
                Translations: map[string]string{
                    "en_US": "Approved",
                },
            },
        },
    },
}
```

---

## API Usage Examples

### Reading Entities

```go
import (
    modelsdk "github.com/mendixlabs/mxcli"
)

// open project read-only
reader, err := modelsdk.Open("/path/to/project.mpr")
if err != nil {
    log.Fatal(err)
}
defer reader.Close()

// list modules
modules, err := reader.ListModules()
for _, m := range modules {
    fmt.Printf("module: %s\n", m.Name)
}

// get domain model
dm, err := reader.GetDomainModel(moduleID)
for _, entity := range dm.Entities {
    fmt.Printf("entity: %s (persistable: %v)\n",
        entity.Name, entity.Persistable)

    for _, attr := range entity.Attributes {
        fmt.Printf("  - %s: %s\n",
            attr.Name, attr.Type.GetTypeName())
    }
}
```

### Creating Entities

```go
// open project for writing
writer, err := modelsdk.OpenForWriting("/path/to/project.mpr")
if err != nil {
    log.Fatal(err)
}
defer writer.Close()

// create entity using helper
entity := modelsdk.NewEntity("Customer")
entity.Documentation = "Customer master data"
entity.Location = model.Point{X: 100, Y: 200}

// add attributes using helpers
entity.Attributes = append(entity.Attributes,
    modelsdk.NewAutoNumberAttribute("CustomerId"),
    modelsdk.NewStringAttribute("Name", 200),
    modelsdk.NewStringAttribute("Email", 200),
)

// create in domain model
err = writer.CreateEntity(domainModelID, entity)
if err != nil {
    log.Fatal(err)
}
```

### Helper Functions

```go
// modelsdk.go - Public helper functions

// NewEntity creates a new persistent entity
func NewEntity(name string) *domainmodel.Entity {
    return &domainmodel.Entity{
        BaseElement: model.BaseElement{
            ID: model.ID(GenerateID()),
        },
        Name:        name,
        Persistable: true,
    }
}

// NewNonPersistableEntity creates a non-persistent entity
func NewNonPersistableEntity(name string) *domainmodel.Entity {
    entity := NewEntity(name)
    entity.Persistable = false
    return entity
}

// NewStringAttribute creates a string attribute
func NewStringAttribute(name string, length int) *domainmodel.Attribute {
    return &domainmodel.Attribute{
        BaseElement: model.BaseElement{
            ID: model.ID(GenerateID()),
        },
        Name: name,
        type: &domainmodel.StringAttributeType{length: length},
    }
}

// NewIntegerAttribute creates an integer attribute
func NewIntegerAttribute(name string) *domainmodel.Attribute {
    return &domainmodel.Attribute{
        BaseElement: model.BaseElement{
            ID: model.ID(GenerateID()),
        },
        Name: name,
        type: &domainmodel.IntegerAttributeType{},
    }
}

// ... similar helpers for other types
```

### MDL Executor Integration

The MDL executor (`mdl/executor` package) translates MDL AST to SDK calls:

```go
// executor/cmd_entities.go
func (e *Executor) execCreateEntity(s *ast.CreateEntityStmt) error {
    // find module
    module, err := e.findModule(s.Name.Module)
    if err != nil {
        return err
    }

    // get domain model
    dm, err := e.reader.GetDomainModel(module.ID)
    if err != nil {
        return err
    }

    // create entity
    entity := &domainmodel.Entity{
        Name:        s.Name.Name,
        documentation: s.Documentation,
        Location:    model.Point{X: s.Position.X, Y: s.Position.Y},
        Persistable: s.Kind != ast.EntityNonPersistent,
    }

    // Convert attributes
    for _, a := range s.Attributes {
        attr := &domainmodel.Attribute{
            Name:          a.Name,
            documentation: a.Documentation,
            type:          convertDataType(a.Type),
        }
        if a.HasDefault {
            attr.Value = &domainmodel.AttributeValue{
                DefaultValue: fmt.Sprintf("%v", a.DefaultValue),
            }
        }
        entity.Attributes = append(entity.Attributes, attr)
    }

    // write to project
    return e.writer.CreateEntity(dm.ID, entity)
}
```

---

## Type Conversion Functions

### MDL AST to SDK Types

```go
// executor/executor.go
func convertDataType(dt ast.DataType) domainmodel.AttributeType {
    switch dt.Kind {
    case ast.TypeString:
        return &domainmodel.StringAttributeType{length: dt.Length}
    case ast.TypeInteger:
        return &domainmodel.IntegerAttributeType{}
    case ast.TypeLong:
        return &domainmodel.LongAttributeType{}
    case ast.TypeDecimal:
        return &domainmodel.DecimalAttributeType{}
    case ast.TypeBoolean:
        return &domainmodel.BooleanAttributeType{}
    case ast.TypeDateTime:
        return &domainmodel.DateTimeAttributeType{}
    case ast.TypeAutoNumber:
        return &domainmodel.AutoNumberAttributeType{}
    case ast.TypeBinary:
        return &domainmodel.BinaryAttributeType{}
    case ast.TypeEnumeration:
        return &domainmodel.EnumerationAttributeType{
            // EnumerationID resolved from dt.EnumRef
        }
    default:
        return &domainmodel.StringAttributeType{length: 200}
    }
}
```

### SDK Types to MDL Output

```go
// executor/executor.go
func getAttributeTypeName(at domainmodel.AttributeType) string {
    switch t := at.(type) {
    case *domainmodel.StringAttributeType:
        if t.Length > 0 {
            return fmt.Sprintf("string(%d)", t.Length)
        }
        return "string"
    case *domainmodel.IntegerAttributeType:
        return "integer"
    case *domainmodel.LongAttributeType:
        return "long"
    case *domainmodel.DecimalAttributeType:
        return "decimal"
    case *domainmodel.BooleanAttributeType:
        return "boolean"
    case *domainmodel.DateTimeAttributeType:
        return "datetime"
    case *domainmodel.AutoNumberAttributeType:
        return "autonumber"
    case *domainmodel.BinaryAttributeType:
        return "binary"
    case *domainmodel.EnumerationAttributeType:
        if t.EnumerationID != "" {
            return fmt.Sprintf("enumeration(%s)", t.EnumerationID)
        }
        return "enumeration"
    default:
        return "Unknown"
    }
}
```

---

## High-Level Fluent API

The `api/` package provides a simplified builder API as an alternative to direct SDK type construction.

### Entity Builder

```go
import "github.com/mendixlabs/mxcli/api"

modelAPI := api.New(writer)
module, _ := modelAPI.Modules.GetModule("Sales")
modelAPI.SetModule(module)

entity, _ := modelAPI.DomainModels.CreateEntity("Customer").
    persistent().
    WithStringAttribute("Name", 200).
    WithIntegerAttribute("Age").
    WithEnumerationAttribute("status", "Sales.CustomerStatus").
    build()
```

### Microflow Builder

```go
mf, _ := modelAPI.Microflows.CreateMicroflow("ACT_ProcessOrder").
    WithParameter("Order", "Sales.Order").
    WithStringParameter("Note").
    ReturnsBoolean().
    build()
```

### Enumeration Builder

```go
enum, _ := modelAPI.Enumerations.CreateEnumeration("OrderStatus").
    WithValue("Draft", "Draft").
    WithValue("Active", "Active").
    WithValue("Closed", "Closed").
    build()
```

### MDL to API Mapping

| MDL Statement | Fluent API Method |
|---------------|-------------------|
| `create persistent entity` | `DomainModels.CreateEntity().Persistent().Build()` |
| `create non-persistent entity` | `DomainModels.CreateEntity().NonPersistent().Build()` |
| `create association` | `DomainModels.CreateAssociation().Build()` |
| `create enumeration` | `Enumerations.CreateEnumeration().Build()` |
| `create microflow` | `Microflows.CreateMicroflow().Build()` |
| `create page` | `Pages.CreatePage().Build()` |
