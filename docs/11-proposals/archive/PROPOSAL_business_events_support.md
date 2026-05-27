---
title: Business Events Support in mxcli
status: done
---

# Proposal: Business Events Support in mxcli

## Motivation

Business Events is a Mendix feature for asynchronous event-driven integration, allowing applications to publish and subscribe to business events via channels and messages. The feature was introduced in Mendix 9.8 and underwent a major redesign in 9.24.

**Important**: None of our three test projects (EnquiriesManagement, Evora-FactoryManagement, LatoProductInventory) contain any Business Event documents. This proposal is based entirely on the Mendix metamodel reflection data and generated types.

## Current State

- mxcli counts Business Events per module in `show modules` (checks `BusinessEvents$` prefix)
- No BSON parser, reader methods, catalog table, or MDL syntax exists
- Generated metamodel types exist in `generated/metamodel/types.go` (auto-generated from reflection data)

## Business Events Metamodel (from reflection data 11.6.0)

### Document Hierarchy

```
BusinessEvents$BusinessEventService (MODEL_UNIT - top-level document)
├── Name: string
├── documentation: string
├── ExportLevel: enum (api | Hidden)
├── Excluded: boolean
├── Document: string
├── Definition: BusinessEvents$BusinessEventDefinition?
│   ├── ServiceName: string
│   ├── EventNamePrefix: string
│   ├── description: string
│   ├── Summary: string
│   └── channels: BusinessEvents$Channel[]
│       ├── ChannelName: string
│       ├── description: string
│       └── messages: BusinessEvents$message[]
│           ├── MessageName: string
│           ├── description: string
│           ├── CanPublish: boolean
│           ├── CanSubscribe: boolean
│           └── attributes: BusinessEvents$MessageAttribute[]
│               ├── attributename: string
│               ├── AttributeType: DomainModels$AttributeType (required)
│               ├── description: string
│               └── EnumerationDefinition: BusinessEvents$AttributeEnumeration?
│                   └── Items: BusinessEvents$AttributeEnumerationItem[]
│                       └── value: string
└── OperationImplementations: BusinessEvents$ServiceOperation[]
    ├── MessageName: string
    ├── operation: string
    ├── entity: DomainModels$entity (BY_NAME, required)
    └── microflow: microflows$microflow (BY_NAME, optional)
```

### Cross-References to Other Domains

| Reference | Target | Type |
|-----------|--------|------|
| ServiceOperation.Entity | DomainModels$Entity | BY_NAME (required) |
| ServiceOperation.Microflow | Microflows$Microflow | BY_NAME (optional) |
| MessageAttribute.AttributeType | DomainModels$AttributeType | PART |

### Version History

| Version | Changes |
|---------|---------|
| 9.8.0 | Introduced `ConsumedBusinessEventService` and `PublishedBusinessEventService` |
| 9.11.0 | Added `ConsumedBusinessEvent`, `PublishedMessage`, `PublishedMessageAttribute` |
| 9.24.0 | **Major redesign**: Replaced published/consumed model with unified `BusinessEventService`, `BusinessEventDefinition`, `Channel`, `message` |
| 10.0.0 | Added `AttributeEnumeration` and `AttributeEnumerationItem` |
| 10.21.0 | Added `SourceApi` property |

**Note**: All Business Events types are marked as **experimental** in the TypeScript SDK.

### BSON Storage Names

All storage names match their qualified names (no aliasing):
- `BusinessEvents$BusinessEventService`
- `BusinessEvents$BusinessEventDefinition`
- `BusinessEvents$Channel`
- `BusinessEvents$message`
- `BusinessEvents$MessageAttribute`
- `BusinessEvents$AttributeEnumeration`
- `BusinessEvents$AttributeEnumerationItem`
- `BusinessEvents$ServiceOperation`

## Implementation Plan

### Phase 1: Read-Only Support

Given that we have no real test data, this should be a lightweight implementation.

#### 1a. SDK Types (`sdk/businessevents/`)

Simple Go types mirroring the metamodel:

```go
type BusinessEventService struct {
    model.BaseElement
    Name                     string
    documentation            string
    ExportLevel              string
    Excluded                 bool
    Definition               *BusinessEventDefinition
    OperationImplementations []*ServiceOperation
}

type BusinessEventDefinition struct {
    model.BaseElement
    ServiceName     string
    EventNamePrefix string
    description     string
    Summary         string
    channels        []*Channel
}

type Channel struct {
    model.BaseElement
    ChannelName string
    description string
    messages    []*message
}

type message struct {
    model.BaseElement
    MessageName  string
    description  string
    CanPublish   bool
    CanSubscribe bool
    attributes   []*MessageAttribute
}

type MessageAttribute struct {
    model.BaseElement
    attributename string
    AttributeType string // simplified
    description   string
}

type ServiceOperation struct {
    model.BaseElement
    MessageName string
    operation   string
    entity      string // qualified name
    microflow   string // qualified name
}
```

#### 1b. BSON Parser (`sdk/mpr/parser_businessevents.go`)

Parse the nested structure following reflection data field names.

#### 1c. Reader Methods

```go
func (r *Reader) ListBusinessEventServices() ([]*businessevents.BusinessEventService, error)
func (r *Reader) GetBusinessEventService(id string) (*businessevents.BusinessEventService, error)
```

#### 1d. SHOW/DESCRIBE Commands

- `show business events [in module]`
- `describe business event Module.ServiceName`

Example output:
```sql
business event Module.OrderEvents
  export level: api
  DEFINITION
    service NAME: 'OrderService'
    event PREFIX: 'com.example.orders'
    channels
      CHANNEL 'orders'
        message 'OrderCreated' (publish, subscribe)
          attributes
            OrderId: integer
            CustomerName: string
            Amount: decimal
  OPERATIONS
    'OrderCreated' -> Module.Order (microflow Module.ACT_HandleOrderCreated)
end;
```

#### 1e. Catalog Table

Add `CATALOG.BUSINESS_EVENTS` table:

| Column | Type |
|--------|------|
| Id | TEXT |
| Name | TEXT |
| QualifiedName | TEXT |
| ModuleName | TEXT |
| Documentation | TEXT |
| ExportLevel | TEXT |
| ChannelCount | INTEGER |
| MessageCount | INTEGER |
| OperationCount | INTEGER |

#### 1f. Cross-References

- `entity` - ServiceOperation references an entity
- `call` - ServiceOperation references a microflow

## Priority Assessment

**Low priority.** Business Events:
- Are not present in any of our test projects
- Are marked as experimental in the Mendix SDK
- Have a relatively simple structure (no complex flow graphs like workflows)
- Are less commonly used than workflows in typical Mendix applications

Recommend implementing after workflow support is complete and only if user demand warrants it.

## Files to Create/Modify

### New Files
| File | Description |
|------|-------------|
| `sdk/businessevents/businessevents.go` | Type definitions |
| `sdk/mpr/parser_businessevents.go` | BSON parser |
| `mdl/catalog/builder_businessevents.go` | Catalog builder |

### Modified Files
| File | Changes |
|------|---------|
| `sdk/mpr/reader_documents.go` | Add List/Get methods |
| `mdl/catalog/tables.go` | Add table schema |
| `mdl/catalog/builder.go` | Add to build pipeline |
| `mdl/catalog/catalog.go` | Add to Tables() |
| `mdl/executor/cmd_show.go` | Add SHOW command |
| `mdl/executor/cmd_describe.go` | Add DESCRIBE command |

## Verification

Since we have no test projects with Business Events, verification would require:
1. Creating a test project with Business Events in Studio Pro
2. Or finding a Marketplace app that uses them

```bash
# when a test project with business events is available:
./bin/mxcli -p /path/to/app.mpr -c "show business events"
./bin/mxcli -p /path/to/app.mpr -c "describe business event Module.ServiceName"
./bin/mxcli -p /path/to/app.mpr -c "refresh catalog full force; select * from CATALOG.BUSINESS_EVENTS"
```
