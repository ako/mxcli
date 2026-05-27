---
title: SHOW/DESCRIBE Export Mappings
status: proposed
---

# Proposal: SHOW/DESCRIBE Export Mappings

## Overview

**Document type:** `ExportMappings$ExportMapping`
**Prevalence:** 67 across test projects (12 Enquiries, 31 Evora, 24 Lato)
**Priority:** High — complement to Import Mappings, used for REST response serialization

Export Mappings define how Mendix entities are serialized to JSON/XML for outgoing data. They are the mirror of Import Mappings and share much of the same structure.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | Only in generated metamodel |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 2854 |

## BSON Structure (from test projects)

```
ExportMappings$ExportMapping:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  JsonStructure: string (qualified name)
  XmlSchema: string
  MessageDefinition: string
  WsdlFile: string
  ServiceName: string
  OperationName: string
  PublicName: string
  XsdRootElementName: string
  NullValueOption: string ("LeaveOutElement", "SendAsNil")
  IsHeaderParameter: bool
  ParameterName: string
  Elements: []*ExportMappings$ObjectMappingElement
    Same tree structure as ImportMapping but with ExportMappings-prefixed types
```

## Proposed MDL Syntax

### SHOW EXPORT MAPPINGS

```
show export mappings [in module]
```

| Qualified Name | Module | Name | Schema Source | Root Entity | Elements |
|----------------|--------|------|--------------|-------------|----------|

### DESCRIBE EXPORT MAPPING

```
describe export mapping Module.Name
```

Output format:

```
/**
 * Serializes Customer entity to JSON
 */
export mapping MyModule.ExportCustomer
  to json structure MyModule.CustomerResponse
  null values LeaveOutElement
{
  MyModule.Customer -> root
    Id -> id (integer)
    Name -> name (string)
    Email -> email (string)
    MyModule.Address via MyModule.Customer_Address -> addresses
      Street -> street (string)
      City -> city (string)
};
/
```

## Implementation Steps

### 1. Add Model Types (model/types.go)

```go
type ExportMapping struct {
    ContainerID     model.ID
    Name            string
    documentation   string
    Excluded        bool
    ExportLevel     string
    JsonStructure   string
    XmlSchema       string
    MessageDefinition string
    NullValueOption string
    Elements        []ExportMappingElement
}

type ExportMappingElement struct {
    Kind        string // "object" or "value"
    entity      string
    attribute   string
    ExposedName string
    JsonPath    string
    association string
    DataType    string
    Children    []ExportMappingElement
}
```

### 2. Implementation

Same pattern as Import Mappings — share the element tree parsing logic where possible. Consider a shared `parseMappingElements()` helper.

Grammar tokens: `export` (already exists), `mapping`, `mappings` (shared with Import).

## Dependencies

- Can share implementation with Import Mappings proposal
- Same schema source resolution

## Testing

- Combine with Import Mapping examples in `mdl-examples/doctype-tests/18-mapping-examples.mdl`
