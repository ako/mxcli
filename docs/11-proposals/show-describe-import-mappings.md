---
title: SHOW/DESCRIBE Import Mappings
status: proposed
---

# Proposal: SHOW/DESCRIBE Import Mappings

## Overview

**Document type:** `ImportMappings$ImportMapping`
**Prevalence:** 83 across test projects (15 Enquiries, 35 Evora, 33 Lato)
**Priority:** High — heavily used for REST/JSON integrations

Import Mappings define how incoming JSON/XML data is mapped to Mendix entities. They reference a schema source (JSON Structure, XML Schema, or Message Definition) and specify how each field maps to entity attributes.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | Only in generated metamodel |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 2957 |

## BSON Structure (from test projects)

```
ImportMappings$ImportMapping:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  JsonStructure: string (qualified name reference)
  XmlSchema: string (qualified name reference)
  MessageDefinition: string (qualified name reference)
  WsdlFile: string (web service reference)
  ServiceName: string
  OperationName: string
  PublicName: string
  XsdRootElementName: string
  ParameterType: DataTypes$* (polymorphic)
  UseSubtransactionsForMicroflows: bool
  MappingSourceReference: nullable
  Elements: []*ImportMappings$ObjectMappingElement
    - entity: string (qualified entity name)
    - ExposedName: string
    - JsonPath: string
    - XmlPath: string
    - ObjectHandling: string ("create", "find", "Custom", "CallAMicroflow")
    - association: string
    - Children: [] (recursive, mix of object and value elements)
      - ImportMappings$ValueMappingElement:
        - attribute: string (qualified name)
        - ExposedName: string
        - JsonPath: string
        - type: DataTypes$* (polymorphic)
        - IsKey: bool
```

## Proposed MDL Syntax

### SHOW IMPORT MAPPINGS

```
show import mappings [in module]
```

| Qualified Name | Module | Name | Schema Source | Root Entity | Elements |
|----------------|--------|------|--------------|-------------|----------|

Where "Schema Source" shows the referenced JSON Structure, XML Schema, or Message Definition.

### DESCRIBE IMPORT MAPPING

```
describe import mapping Module.Name
```

Output format:

```
/**
 * Maps customer API response to Customer entity
 */
import mapping MyModule.ImportCustomer
  from json structure MyModule.CustomerResponse
{
  root -> MyModule.Customer (create)
    id -> Id (integer, key)
    name -> Name (string)
    email -> Email (string)
    addresses -> MyModule.Address (create) via MyModule.Customer_Address
      street -> Street (string)
      city -> City (string)
};
/
```

For XML/WSDL-based mappings:

```
import mapping MyModule.ImportOrder
  from xml schema MyModule.OrderSchema
  ROOT ELEMENT 'Order'
{
  Order -> MyModule.Order (find)
    OrderId -> OrderId (integer, key)
    LineItems -> MyModule.LineItem (create) via MyModule.Order_LineItem
      Product -> ProductName (string)
      Quantity -> Quantity (integer)
};
/
```

## Implementation Steps

### 1. Add Model Types (model/types.go)

```go
type ImportMapping struct {
    ContainerID   model.ID
    Name          string
    documentation string
    Excluded      bool
    ExportLevel   string
    // schema source (one of these is set)
    JsonStructure     string // qualified name
    XmlSchema         string
    MessageDefinition string
    WsdlFile          string
    // mapping tree
    Elements []ImportMappingElement
}

type ImportMappingElement struct {
    Kind          string // "object" or "value"
    entity        string // qualified name (for objects)
    attribute     string // qualified name (for values)
    ExposedName   string
    JsonPath      string
    ObjectHandling string // "create", "find", "Custom"
    association   string
    IsKey         bool
    DataType      string
    Children      []ImportMappingElement
}
```

### 2. Add Parser (sdk/mpr/parser_import_mapping.go)

Parse `ImportMappings$ImportMapping` BSON. Recursively parse element tree (mix of `ObjectMappingElement` and `ValueMappingElement`).

### 3. Add Reader

```go
func (r *Reader) ListImportMappings() ([]*model.ImportMapping, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Standard pattern. Grammar tokens: `import` (already exists), `mapping`, `mappings`.

### 5. Add Autocomplete

```go
func (e *Executor) GetImportMappingNames(moduleFilter string) []string
```

## Dependencies

- Depends on JSON Structures proposal (for resolving schema references in DESCRIBE output)
- Can be implemented independently (just show qualified name references)

## Testing

- Create `mdl-examples/doctype-tests/18-mapping-examples.mdl`
- Verify against all 3 test projects
