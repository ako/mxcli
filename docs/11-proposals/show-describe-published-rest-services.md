---
title: SHOW/DESCRIBE Published REST Services
status: proposed
---

# Proposal: SHOW/DESCRIBE Published REST Services

## Overview

**Document type:** `rest$PublishedRestService`
**Prevalence:** 16 across test projects (1 Enquiries, 7 Evora, 8 Lato)
**Priority:** High — REST APIs are critical for modern Mendix apps

Published REST Services expose HTTP endpoints backed by microflows. Each service has a base path, version, authentication configuration, and one or more resources with operations (GET, POST, PUT, DELETE, PATCH).

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Partial | `model/types.go` line 556 — Name, Path, Version, ServiceName, Excluded, Resources |
| **Parser** | Partial | `sdk/mpr/parser_rest.go` — parses basic fields + resources + operations |
| **Reader** | Yes | `ListPublishedRestServices()` in `sdk/mpr/reader_documents.go` |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 8177 |
| **AST** | No | — |
| **Executor** | No | — |

## BSON Structure (from test projects)

```
rest$PublishedRestService:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  path: string (e.g., "rest/customers/v1")
  version: string (e.g., "1.0.0")
  ServiceName: string
  AllowedRoles: [] (qualified role names)
  AuthenticationTypes: [] ("basic", "Custom", "none")
  AuthenticationMicroflow: string (qualified name)
  CorsConfiguration: nullable
  parameters: []*RestOperationParameter (service-level params)
  Resources: []*rest$PublishedRestServiceResource
    - Name: string (e.g., "Customers")
    - documentation: string
    - Operations: []*rest$PublishedRestServiceOperation
      - HttpMethod: string ("get", "post", "put", "delete", "patch")
      - path: string (e.g., "/{id}")
      - microflow: string (qualified name)
      - Summary: string
      - deprecated: bool
      - commit: string ("Yes", "No")
      - ImportMapping: string (qualified name)
      - ExportMapping: string (qualified name)
      - parameters: []*RestOperationParameter
```

## Proposed MDL Syntax

### SHOW REST SERVICES

```
show rest services [in module]
```

| Qualified Name | Module | Name | Path | Version | Resources | Operations |
|----------------|--------|------|------|---------|-----------|------------|

### DESCRIBE REST SERVICE

```
describe rest service Module.Name
```

Output format:

```
/**
 * Customer management API
 */
rest service MyModule.CustomerAPI
  path 'rest/customers/v1'
  version '1.0.0'
  authentication basic
{
  resource Customers
  {
    get /
      microflow MyModule.GetAllCustomers
      export mapping MyModule.ExportCustomerList
      SUMMARY 'List all customers';

    get /{id}
      microflow MyModule.GetCustomerById
      export mapping MyModule.ExportCustomer
      SUMMARY 'Get customer by ID';

    post /
      microflow MyModule.CreateCustomer
      import mapping MyModule.ImportCustomer
      commit Yes
      SUMMARY 'Create a new customer';

    put /{id}
      microflow MyModule.UpdateCustomer
      import mapping MyModule.ImportCustomer
      commit Yes
      SUMMARY 'Update an existing customer';

    delete /{id}
      microflow MyModule.DeleteCustomer
      SUMMARY 'Delete a customer';
  };
};
/
```

## Implementation Steps

### 1. Enhance Model Type (model/types.go)

The existing `PublishedRestService` struct needs:
- `documentation`, `AllowedRoles`, `AuthenticationTypes`, `AuthenticationMicroflow`

The existing `RestResource` and `RestOperation` structs need:
- `Summary`, `deprecated`, `commit`, `ImportMapping`, `ExportMapping`

### 2. Enhance Parser (sdk/mpr/parser_rest.go)

Extend existing parser to capture all fields listed above.

### 3. Add AST Types

```go
ShowRestServices    // in ShowObjectType enum
DescribeRestService // in DescribeObjectType enum
```

### 4. Add Grammar Rules

```antlr
rest: 'REST';
service: 'SERVICE';   // may already exist for odata
services: 'SERVICES'; // may already exist

// show rest services [in module]
// describe rest service qualifiedName
```

### 5. Add Executor (mdl/executor/cmd_rest_services.go)

- `showRestServices(moduleName string)` — table listing
- `describeRestService(name QualifiedName)` — MDL output with resources and operations

### 6. Add Autocomplete

```go
func (e *Executor) GetRestServiceNames(moduleFilter string) []string
```

## Testing

- Create `mdl-examples/doctype-tests/19-rest-service-examples.mdl`
- Verify against Lato project (8 REST services — most comprehensive)
