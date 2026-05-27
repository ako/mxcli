---
title: SHOW/DESCRIBE Message Definitions
status: proposed
---

# Proposal: SHOW/DESCRIBE Message Definitions

## Overview

**Document type:** `MessageDefinitions$MessageDefinitionCollection`
**Prevalence:** 28 across test projects (7 Enquiries, 11 Evora, 10 Lato)
**Priority:** Medium — used for service contracts, business events, and mappings

Message Definition Collections define entity-based message schemas for integrations. Each collection contains one or more Message Definitions, each exposing an entity's attributes and associations as a structured message contract. They are referenced by Published/Consumed REST services, OData services, and Business Events.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | Only in generated metamodel |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 3457 |

## BSON Structure (from test projects)

```
MessageDefinitions$MessageDefinitionCollection:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  MessageDefinitions: []*MessageDefinitions$EntityMessageDefinition
    - Name: string
    - documentation: string
    - ExposedEntity: MessageDefinitions$ExposedEntity
      - entity: string (qualified entity name)
      - ExposedName: string
      - Children: [] (recursive)
        - MessageDefinitions$ExposedAttribute:
          - attribute: string (qualified name)
          - ExposedName: string
          - PrimitiveType: string
        - MessageDefinitions$ExposedAssociation:
          - association: string (qualified name)
          - entity: string (qualified entity name)
          - ExposedName: string
          - Children: [] (recursive — nested entity exposure)
```

## Proposed MDL Syntax

### SHOW MESSAGE DEFINITIONS

```
show message DEFINITIONS [in module]
```

| Qualified Name | Module | Name | Messages | Entities |
|----------------|--------|------|----------|----------|

Where "Messages" is the count of message definitions in the collection, and "Entities" lists the exposed entity names.

### DESCRIBE MESSAGE DEFINITION

```
describe message DEFINITION Module.Name
```

Output format:

```
message DEFINITION collection MyModule.CustomerMessages
{
  message CustomerMessage
    entity MyModule.Customer as 'Customer'
    {
      Id as 'id': integer
      Name as 'name': string
      Email as 'email': string
      association MyModule.Customer_Address as 'addresses'
        entity MyModule.Address as 'Address'
        {
          Street as 'street': string
          City as 'city': string
        }
    };

  message OrderMessage
    entity MyModule.Order as 'Order'
    {
      OrderNumber as 'orderNumber': integer
      TotalAmount as 'totalAmount': decimal
    };
};
/
```

## Implementation Steps

### 1. Add Model Types (model/types.go)

```go
type MessageDefinitionCollection struct {
    ContainerID model.ID
    Name        string
    documentation string
    Excluded    bool
    ExportLevel string
    Definitions []*MessageDefinition
}

type MessageDefinition struct {
    Name          string
    documentation string
    entity        string // qualified entity name
    ExposedName   string
    attributes    []*ExposedAttribute
    associations  []*ExposedAssociation
}

type ExposedAttribute struct {
    attribute   string // qualified name
    ExposedName string
    type        string
}

type ExposedAssociation struct {
    association string
    entity      string
    ExposedName string
    attributes  []*ExposedAttribute
    associations []*ExposedAssociation // recursive
}
```

### 2. Add Parser (sdk/mpr/parser_message_definitions.go)

Parse `MessageDefinitions$MessageDefinitionCollection` BSON. Recursively parse the exposed entity tree with its attributes and associations.

### 3. Add Reader

```go
func (r *Reader) ListMessageDefinitions() ([]*model.MessageDefinitionCollection, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Grammar tokens: `message` (may already exist for business events), `DEFINITION`, `DEFINITIONS`.

### 5. Add Autocomplete

```go
func (e *Executor) GetMessageDefinitionNames(moduleFilter string) []string
```

## Testing

- Verify against all 3 test projects
