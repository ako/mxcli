---
title: Integration Pane — Unified View of External Service Assets
status: draft
---

# Proposal: Integration Pane — Unified View of External Service Assets

## Overview

Mendix Studio Pro has an **Integration Pane** that shows all connected services and lists the available assets from their contracts (OData `$metadata`, OpenAPI specs, AsyncAPI definitions, external database schemas). We need an equivalent in mxcli — both as **MDL commands** and **catalog tables** — to give users a unified way to discover what integrations exist and what assets they expose.

### Service Types

| Service Type | Contract Format | Assets |
|---|---|---|
| **OData Client** | `$metadata` (EDMX/CSDL) | External entities, external actions/functions |
| **REST Client** | OpenAPI / manual operations | REST operations (resources, methods, parameters) |
| **Published OData** | Self-defined | Published entity sets, exposed members |
| **Published REST** | Self-defined | Resources, operations |
| **Business Events** | AsyncAPI | Channels, messages, attributes |
| **Database Connection** | DB schema | Queries, table mappings, parameters |
| **SOAP Client** (consumed) | WSDL | Imported operations, parameters |
| **SOAP Service** (published) | WSDL | Published operations, parameters, microflow handlers |

### Contract Storage in MPR (Key Finding)

**Contracts ARE stored in the MPR file.** Studio Pro caches the full contract document in BSON fields so the Integration Pane can show available assets without network access. We were not parsing these fields.

| Service Type | BSON Field | Format | Example |
|---|---|---|---|
| **OData Client** | `Metadata` | OData `$metadata` XML (EDMX/CSDL) | Full entity types, properties, associations, entity sets, actions |
| **Business Event Client** | `Document` | AsyncAPI 2.2.0 YAML | Channels, messages, schemas, traits |

#### OData `Metadata` Field

The `rest$ConsumedODataService` BSON contains a `Metadata` string field with the complete `$metadata` XML. Also stores:
- `MetadataHash` — SHA-256 hash for change detection
- `MetadataReferences` — array (entity/type references)
- `ValidatedEntities` — array of validated entity references
- `ApplicationId`, `EndpointId`, `CatalogUrl` — Mendix Catalog integration
- `EnvironmentType` — "Production", etc.
- `icon` — base64-encoded PNG

Verified in test projects:
- `EnquiriesManagement/LatoIntegrations.SAP` — OData3, 9 entity types (PurchaseOrder, Product, Customer, Employee, etc.) with full property definitions
- `EnquiriesManagement/LatoIntegrations.Product_Inventory` — OData4, 7,072 chars of metadata

#### Business Events `Document` Field

The `BusinessEvents$BusinessEventService` BSON uses two patterns:
- **Publisher**: Uses structured `Definition` object (channels/messages/attributes) — `Document` is empty
- **Consumer/Client**: Stores full **AsyncAPI YAML** in `Document` field — `Definition` is null

Verified in `QueryDemoApp/ShopViewsClient.ShopEventsClient` — full AsyncAPI 2.2.0 contract with channels, messages, schemas, CloudEvents headers.

### Implementation Approach

Since contracts are stored locally, we can parse them without network access:

1. **Phase 1** (current) — list MPR-stored configuration (service metadata, imported entities, operations)
2. **Phase 2** — parse the cached `Metadata` XML and `Document` YAML to list ALL available entities, properties, actions, channels, messages from the contracts

---

## Current State

### SHOW/DESCRIBE Commands

| Service Type | SHOW | DESCRIBE | Catalog Table |
|---|---|---|---|
| OData Client | `show odata clients` | `describe odata client` | `odata_clients` |
| OData Service (published) | `show odata services` | `describe odata service` | `odata_services` |
| External Entity | `show external entities` | `describe external entity` | — (in `entities` table) |
| External Action | — | — | — |
| SOAP Client (consumed) | — | — | — |
| SOAP Service (published) | — | — | — |
| REST Client | `show rest clients` | `describe rest client` | — |
| Published REST | — | — | — |
| Business Event Service | `show business event services` | `describe business event service` | `business_event_services` |
| Business Event Messages | `show business events` | — | — |
| Business Event Client | `show business event clients` (stub) | — | — |
| Database Connection | `show database connections` | `describe database connection` | `database_connections` |

### Catalog Gaps

| Gap | Details |
|---|---|
| No `rest_clients` table | Consumed REST services not queryable via SQL |
| No `rest_operations` table | Individual operations not queryable |
| No `published_rest_services` table | Published REST services not queryable |
| No `published_rest_operations` table | Published REST operations not queryable |
| No `external_entities` table | External entities mixed into `entities` table with no easy filter |
| No `external_actions` table | No way to discover OData action usage |
| No `business_events` table | Individual messages not queryable |
| `CallExternalAction` not in activity type switch | Catalog builder doesn't recognize this activity type |
| `activities` table missing service/action columns | No OData service or action name stored per activity |
| No SOAP parsing at all | `WebServices$ImportedWebService` and `WebServices$PublishedWebService` BSON types exist in metamodel but no parser, reader, model types, or commands |
| No `soap_clients` / `soap_services` tables | SOAP services not queryable |

---

## Phase 1: MPR-Stored Assets

### 1.1 New MDL Commands

#### OData External Actions (from microflow usage)
```sql
show external actions;                    -- All external actions used in microflows
show external actions in MyModule;        -- Filter by module
```

Output columns: `service`, `action`, `parameters`, `UsedBy` (microflow names)

> **Note:** Phase 1 discovers actions from microflow usage only. Phase 2 will parse the cached `$metadata` XML to list ALL available actions from the contract, including those not yet used. See also [mendixlabs/mxcli#44](https://github.com/mendixlabs/mxcli/issues/44).

#### Published REST Services
```sql
show published rest services;             -- All published REST services
show published rest services in MyModule; -- Filter by module
describe published rest service MyModule.CustomerAPI;
```

Output columns: `module`, `QualifiedName`, `path`, `version`, `Resources`, `Operations`

### 1.2 New Catalog Tables

#### `rest_clients` — Consumed REST Services
```sql
create table if not exists rest_clients (
    Id text primary key,
    Name text,
    QualifiedName text,
    ModuleName text,
    folder text,
    BaseUrl text,
    AuthScheme text,           -- "Basic", "None"
    OperationCount integer,
    documentation text,
    ProjectId text,
    ProjectName text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `rest_operations` — REST Client Operations (detail table)
```sql
create table if not exists rest_operations (
    Id text primary key,
    ServiceId text,             -- FK to rest_clients.Id
    ServiceQualifiedName text,
    Name text,
    HttpMethod text,            -- GET, POST, PUT, PATCH, DELETE
    path text,
    ParameterCount integer,
    HasBody integer,
    ResponseType text,          -- "ImplicitMapping", "NoResponse"
    timeout integer,
    ModuleName text,
    ProjectId text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `published_rest_services` — Published REST Services
```sql
create table if not exists published_rest_services (
    Id text primary key,
    Name text,
    QualifiedName text,
    ModuleName text,
    folder text,
    path text,
    version text,
    ServiceName text,
    ResourceCount integer,
    OperationCount integer,
    documentation text,
    ProjectId text,
    ProjectName text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `published_rest_operations` — Published REST Operations (detail table)
```sql
create table if not exists published_rest_operations (
    Id text primary key,
    ServiceId text,             -- FK to published_rest_services.Id
    ServiceQualifiedName text,
    ResourceName text,
    HttpMethod text,
    path text,
    Summary text,
    microflow text,             -- Implementation microflow (BY_NAME)
    deprecated integer,
    ModuleName text,
    ProjectId text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `external_entities` — OData External Entities (view on entities)
```sql
create table if not exists external_entities (
    Id text primary key,
    Name text,
    QualifiedName text,
    ModuleName text,
    ServiceName text,           -- Consumed OData service qualified name
    EntitySet text,             -- Remote entity set name
    RemoteName text,            -- Remote entity name
    Countable integer,
    Creatable integer,
    Deletable integer,
    Updatable integer,
    AttributeCount integer,
    ProjectId text,
    ProjectName text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `external_actions` — OData External Actions (from microflow usage)
```sql
create table if not exists external_actions (
    Id text primary key,        -- Synthetic: hash of service+action
    ServiceName text,           -- Consumed OData service qualified name
    ActionName text,
    ModuleName text,            -- Module where the calling microflow lives
    UsageCount integer,         -- Number of microflow activities calling this
    CallerNames text,           -- Comma-separated list of calling microflows
    ParameterNames text,        -- Comma-separated parameter names
    ProjectId text,
    ProjectName text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

#### `business_events` — Individual Business Event Messages (detail table)
```sql
create table if not exists business_events (
    Id text primary key,
    ServiceId text,             -- FK to business_event_services.Id
    ServiceQualifiedName text,
    ChannelName text,
    MessageName text,
    CanPublish integer,
    CanSubscribe integer,
    AttributeCount integer,
    entity text,                -- Associated entity (BY_NAME)
    PublishMicroflow text,      -- Publisher microflow (BY_NAME)
    SubscribeMicroflow text,    -- Subscriber microflow (BY_NAME)
    ModuleName text,
    ProjectId text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text
);
```

### 1.3 Fix Existing Catalog Gaps

1. **Add `CallExternalAction` to `getMicroflowActionType()`** in `mdl/catalog/builder_microflows.go`
2. **Add service/action columns to `activities` table** — `ServiceRef text`, `ActionRef text` for CallExternalAction, CallRestAction, etc.
3. **Populate `external_entities` table** during catalog refresh from domain models with `source = "rest$ODataRemoteEntitySource"`
4. **Add `IsExternal` column to `entities` table** for easy filtering

### 1.4 OBJECTS View Integration

Add all new tables to the `CATALOG.OBJECTS` UNION ALL view so `show catalog tables` and `select * from CATALOG.OBJECTS` include them:

```sql
union all select Id, 'RestClient' as ObjectType, Name, QualifiedName, ModuleName, ... from rest_clients
union all select Id, 'RestOperation' as ObjectType, Name, ... from rest_operations
union all select Id, 'PublishedRestService' as ObjectType, Name, ... from published_rest_services
union all select Id, 'ExternalEntity' as ObjectType, Name, ... from external_entities
union all select Id, 'ExternalAction' as ObjectType, ActionName as Name, ... from external_actions
union all select Id, 'BusinessEvent' as ObjectType, MessageName as Name, ... from business_events
```

---

## Phase 1 Implementation Plan

### Step 1: Catalog tables and builders (foundation)

- [ ] Add table DDL for `rest_clients`, `rest_operations`, `published_rest_services`, `published_rest_operations`, `external_entities`, `external_actions`, `business_events` to `mdl/catalog/tables.go`
- [ ] Add `IsExternal` column to existing `entities` table
- [ ] Add `ServiceRef`, `ActionRef` columns to existing `activities` table
- [ ] Add `CallExternalAction` case to `getMicroflowActionType()` in `builder_microflows.go`
- [ ] Create `builder_rest.go` — populate `rest_clients` and `rest_operations`
- [ ] Create `builder_published_rest.go` — populate `published_rest_services` and `published_rest_operations`
- [ ] Create `builder_external_entities.go` — populate `external_entities` from domain models
- [ ] Create `builder_external_actions.go` — populate `external_actions` by scanning microflows
- [ ] Extend `builder_businessevents.go` — populate `business_events` detail table
- [ ] Update OBJECTS view with new table UNIONs
- [ ] Wire builders into `builder.go` refresh pipeline

### Step 2: SHOW EXTERNAL ACTIONS command

- [ ] Add `ShowExternalActions` to AST `ShowObjectType` enum
- [ ] Add grammar rule `show external actions [in module]` to `MDLParser.g4`
- [ ] Add visitor handler in `visitor_query.go`
- [ ] Implement `showExternalActions()` in `cmd_odata.go` (scan microflows for CallExternalAction)
- [ ] Add executor dispatch in `executor.go`
- [ ] Regenerate ANTLR parser

### Step 3: SHOW/DESCRIBE PUBLISHED REST SERVICES

- [ ] Add `ShowPublishedRestServices` to AST `ShowObjectType` enum
- [ ] Add `DescribePublishedRestService` to AST `DescribeObjectType` enum
- [ ] Add grammar rules to `MDLParser.g4`
- [ ] Add visitor handlers in `visitor_query.go`
- [ ] Implement `showPublishedRestServices()` in new `cmd_published_rest.go`
- [ ] Implement `describePublishedRestService()` with MDL output format
- [ ] Add executor dispatch in `executor.go`
- [ ] Regenerate ANTLR parser

### Step 4: Help text and examples

- [ ] Update `cmd/mxcli/help_topics/odata.txt` with SHOW EXTERNAL ACTIONS
- [ ] Create `cmd/mxcli/help_topics/rest.txt` or update existing with published REST
- [ ] Add examples to `mdl-examples/doctype-tests/`
- [ ] Update CLAUDE.md Current Implementation Status section

### Step 5: Tests

- [ ] Add catalog builder tests for new tables
- [ ] Add visitor parsing tests for new grammar rules
- [ ] Verify `make test` passes

---

## Phase 2: Parse Cached Contracts

Phase 2 parses the contract documents already stored in the MPR (no network access needed) to list all available assets from each service.

### OData `$metadata` Parsing

Parse the `Metadata` XML field on `rest$ConsumedODataService` to extract:
- Entity types with properties (name, Edm type, nullable, max length)
- Navigation properties (associations between entity types)
- Entity sets (with entity type mapping)
- Function imports / Actions (OData4)
- Complex types and enum types

```sql
-- Show all entities available in the OData contract (including not-yet-imported)
show contract entities from MyModule.SalesforceAPI;
show contract actions from MyModule.SalesforceAPI;
describe contract entity MyModule.SalesforceAPI.PurchaseOrder;
```

Catalog tables:
- `contract_entities` — entity types from cached `$metadata` (name, properties, key, service ref)
- `contract_actions` — function imports / actions from cached `$metadata` (name, parameters, return type)

### Generating CREATE EXTERNAL ENTITY from Contracts ([mendixlabs/mxcli#44](https://github.com/mendixlabs/mxcli/issues/44))

For **entities**, there's no new command needed — `create external entity` already exists. The contract parsing enables a workflow where the user browses available entities and the tool generates the correct `create external entity` with attributes mapped from Edm types:

```sql
-- 1. Browse what's available in the contract
show contract entities from MyModule.SalesforceAPI;

-- 2. Inspect a specific entity's properties
describe contract entity MyModule.SalesforceAPI.PurchaseOrder;
-- Output:
--   PurchaseOrder (Key: ID)
--     ID           Edm.Int64        NOT NULL
--     Number       Edm.Int64
--     Status       Edm.String
--     SupplierName Edm.String(200)
--     GrossAmount  Edm.Decimal
--     DeliveryDate Edm.DateTimeOffset
--     → PurchaseOrderItems  (Navigation: PurchaseOrderItem *)
--     → Customer            (Navigation: Customer 0..1)

-- 3. Generate a CREATE EXTERNAL ENTITY from the contract (all attributes)
describe contract entity MyModule.SalesforceAPI.PurchaseOrder format mdl;
-- Output: ready-to-execute CREATE EXTERNAL ENTITY statement

-- 4. Or create with a subset of attributes
create external entity MyModule.PurchaseOrder
from odata client MyModule.SalesforceAPI (
    EntitySet: 'PurchaseOrders',
    RemoteName: 'PurchaseOrder',
    Countable: Yes
)
(
    Number: long,
    status: string(200),
    SupplierName: string(200),
    GrossAmount: decimal
);
```

For **actions**, there IS new functionality needed — action definitions and their request/response NPEs (non-persistent entities) don't have a `create` equivalent today:

```sql
-- Browse available actions
show contract actions from MyModule.SalesforceAPI;

-- Inspect an action's signature (parameters, return type)
describe contract action MyModule.SalesforceAPI.CreateOrder;
-- Output:
--   CreateOrder
--     Parameters:
--       OrderData  ComplexType:OrderInput (OrderId: Edm.Int64, Items: Collection(OrderItem))
--     Returns:
--       OrderResult  ComplexType:OrderConfirmation (ConfirmationId: Edm.String, Status: Edm.String)

-- Generate MDL to create the NPEs and wire the action
describe contract action MyModule.SalesforceAPI.CreateOrder format mdl;
-- Output: CREATE ENTITY statements for NPEs + documentation for CALL EXTERNAL ACTION usage
```

`describe contract action ... format mdl` should generate:
1. `create entity` (non-persistent) for complex type parameters
2. `create entity` (non-persistent) for complex type return values
3. Edm → Mendix type mapping (Edm.String → String, Edm.Int64 → Long, Edm.Decimal → Decimal, etc.)
4. A comment showing the `call external action` syntax with the correct parameter names

This addresses the core request in issue #44: users want to browse available actions and generate the domain model entities needed to call them.

### AsyncAPI Document Parsing

Parse the `Document` YAML field on `BusinessEvents$BusinessEventService` (consumer pattern) to extract:
- Channels with operation types (publish/subscribe)
- Messages with payload schemas
- Schema properties with types

```sql
-- Show all channels/messages from the AsyncAPI contract
show contract channels from MyModule.EventClient;
show contract messages from MyModule.EventClient;
```

### OpenAPI / REST Contract Parsing

The `rest$ConsumedRestService` may contain an `OpenApiFile` field (not yet verified in test projects). If present, parse it for:
- Paths and operations
- Request/response schemas
- Parameters

### Database Schema Discovery
```sql
-- Already partially implemented via SQL CONNECT + SQL <alias> SHOW TABLES
sql mydb show tables;
sql mydb describe table orders;
```

### Contract Generation for Published Services

Studio Pro can generate/download contracts for published services (OpenAPI for REST, `$metadata` for OData, GraphQL schema for OData, AsyncAPI for business events). We need the same in MDL so users can export a current contract for services defined in their project.

#### Published REST → OpenAPI
```sql
-- Generate OpenAPI 3.0 JSON from a published REST service definition
export contract from MyModule.CustomerAPI format openapi;
export contract from MyModule.CustomerAPI format openapi to '/path/to/openapi.json';
```

Generate by mapping published REST resources and operations to OpenAPI paths, methods, and parameters. Include microflow return types as response schemas where inferrable.

#### Published OData → `$metadata` / GraphQL
```sql
-- Generate OData $metadata XML from a published OData service definition
export contract from MyModule.ProductAPI format odata;
export contract from MyModule.ProductAPI format odata to '/path/to/metadata.xml';

-- Generate GraphQL schema from a published OData service (optional, OData4 only)
export contract from MyModule.ProductAPI format graphql;
```

Generate by mapping published entity types, entity sets, exposed members, and CRUD modes to EDMX/CSDL. The GraphQL variant maps entity sets to queries and CUD modes to mutations.

#### Business Event Service → AsyncAPI
```sql
-- Generate AsyncAPI YAML from a business event service definition
export contract from MyModule.OrderEvents format asyncapi;
export contract from MyModule.OrderEvents format asyncapi to '/path/to/asyncapi.yaml';
```

Generate by mapping channels, messages, and attributes from the structured `Definition` to AsyncAPI 2.x format with CloudEvents headers.

#### Default Behavior
```sql
-- Without FORMAT, auto-detect based on service type
export contract from MyModule.CustomerAPI;          -- REST → openapi
export contract from MyModule.ProductAPI;           -- OData → odata
export contract from MyModule.OrderEvents;          -- Business Event → asyncapi
```

When no `to` path is given, output the contract to stdout (useful for piping or inspection).

---

## Phase 3: SOAP Web Services (Future)

Mendix supports both consuming and publishing SOAP web services. The BSON types are fully defined in the metamodel (`WebServices$ImportedWebService`, `WebServices$PublishedWebService`) but **no parser, reader, model types, or commands exist yet**. This requires building the full stack from scratch.

### BSON Types Available

| Type | Description |
|---|---|
| `WebServices$ImportedWebService` | Consumed SOAP service — holds WSDL, operations, port/binding info |
| `WebServices$PublishedWebService` | Published SOAP service — versioned services with published operations |
| `WebServices$WsdlDescription` | WSDL metadata (schema entries, target namespace) |
| `WebServices$ServiceInfo` | SOAP service definition (location, port, SOAP version, operations) |
| `WebServices$OperationInfo` | Individual SOAP operation (request/response body, SOAP action) |
| `WebServices$PublishedOperation` | Published operation (microflow handler, parameters, return type) |
| `WebServices$VersionedService` | Versioned published service (caption, authentication, validation) |
| `microflows$WebServiceCallAction` | Microflow activity that calls a consumed SOAP operation |

### Implementation Steps

1. **Model types** — Add `ImportedWebService`, `PublishedWebService` to `model/types.go`
2. **Parser** — Create `sdk/mpr/parser_webservices.go` for BSON deserialization
3. **Reader** — Add `ListImportedWebServices()`, `ListPublishedWebServices()` to reader
4. **SHOW/DESCRIBE** — `show SOAP clients`, `show SOAP services`, `describe SOAP client`, `describe SOAP service`
5. **Catalog tables** — `soap_clients` (with operation count, WSDL URL, SOAP version), `soap_services` (with versioned service info), `soap_operations` (detail table)
6. **Microflow integration** — Add `WebServiceCallAction` to `getMicroflowActionType()` and populate `ServiceRef`/`ActionRef`

### Target MDL Commands
```sql
show SOAP clients;                           -- List consumed SOAP services
show SOAP clients in MyModule;
show SOAP services;                          -- List published SOAP services
describe SOAP client MyModule.WeatherService;
describe SOAP service MyModule.OrderService;

-- Catalog queries
select * from CATALOG.SOAP_CLIENTS;
select * from CATALOG.SOAP_SERVICES;
select * from CATALOG.SOAP_OPERATIONS;
```

---

## Example Queries After Phase 1

```sql
-- Integration overview: all external services
select ObjectType, QualifiedName, ModuleName
from CATALOG.OBJECTS
where ObjectType in ('ODataClient', 'RestClient', 'PublishedODataService',
                     'PublishedRestService', 'BusinessEventService', 'DatabaseConnection',
                     'SoapClient', 'SoapService');  -- Phase 3

-- All external entities and their source services
select QualifiedName, ServiceName, EntitySet, RemoteName
from CATALOG.EXTERNAL_ENTITIES;

-- All external actions and where they're called from
select ServiceName, ActionName, UsageCount, CallerNames
from CATALOG.EXTERNAL_ACTIONS;

-- All REST operations across all consumed services
select ServiceQualifiedName, HttpMethod, path, Name
from CATALOG.REST_OPERATIONS
ORDER by ServiceQualifiedName, path;

-- Published API surface area
select ServiceQualifiedName, HttpMethod, path, microflow
from CATALOG.PUBLISHED_REST_OPERATIONS;

-- Business event messages with their handlers
select ServiceQualifiedName, MessageName, CanPublish, CanSubscribe, entity
from CATALOG.BUSINESS_EVENTS;

-- Cross-cutting: find all integration touchpoints in a module
select 'OData Client' as type, QualifiedName from CATALOG.ODATA_CLIENTS where ModuleName = 'Integration'
union all
select 'REST Client', QualifiedName from CATALOG.REST_CLIENTS where ModuleName = 'Integration'
union all
select 'External Entity', QualifiedName from CATALOG.EXTERNAL_ENTITIES where ModuleName = 'Integration'
union all
select 'Business Event', ServiceQualifiedName || '.' || MessageName from CATALOG.BUSINESS_EVENTS where ModuleName = 'Integration';
```
