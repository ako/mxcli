---
title: Consumed REST Services (SHOW / DESCRIBE / CREATE)
status: proposed
---

# Proposal: Consumed REST Services (SHOW / DESCRIBE / CREATE)

## Overview

**Document type:** `rest$ConsumedRestService`
**Prevalence:** 2 in Evora project (not found in Enquiries or Lato)
**Priority:** Medium — newer Mendix feature (10.1.0+), growing in adoption
**Reference:** `mdl-examples/doctype-tests/06-rest-client-examples.mdl` (21 examples)

Consumed REST Services define external REST API connections. Each service has a base URL, authentication scheme, and one or more operations with HTTP methods, paths, headers, parameters, and response handling.

This is different from "Consumed OData Services" (which already have full SHOW/DESCRIBE/CREATE support). Consumed REST Services are a Mendix 10+ feature for calling arbitrary REST APIs.

### Version Requirements

| Feature | Minimum Version |
|---------|----------------|
| ConsumedRestService | Mendix 10.1.0 |
| Method property | Mendix 10.4.0 |
| Query parameters | Mendix 11.0.0 |

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Generated metamodel** | Yes | `generated/metamodel/types.go` — full type hierarchy |
| **Grammar rules** | Partial | `MDLParser.g4:1997-2048` — basic structure, needs revision |
| **Go model types** | No | — |
| **MPR parser** | No | — |
| **MPR reader** | No | — |
| **MPR writer** | No | — |
| **AST nodes** | No | — |
| **Visitor** | No | — |
| **Executor** | No | — |

## BSON Type Hierarchy

From `generated/metamodel/types.go` and reflection data:

```
rest$ConsumedRestService
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: RestExportLevel ("api" | "Hidden")
  BaseUrl: rest$ValueTemplate { value: string }
  BaseUrlParameter: rest$RestParameter? { Name, DataType, TestValue }
  AuthenticationScheme: polymorphic? (see below)
  OpenApiFile: rest$OpenApiFile? { content: string }
  Operations: []*rest$RestOperation

rest$RestOperation
  Name: string
  method: polymorphic (see below)
  path: rest$ValueTemplate { value: string }
  headers: []*rest$HeaderWithValueTemplate { Name, value: rest$ValueTemplate }
  QueryParameters: []*rest$QueryParameter { Name, ParameterUsage, TestValue }
  parameters: []*rest$RestOperationParameter { Name, DataType, TestValue }
  ResponseHandling: polymorphic (see below)
  Tags: []string
  timeout: int (seconds, 0 = default 300s)
```

### Polymorphic Types

**Method** (`rest$RestOperationMethod`):
- `rest$RestOperationMethodWithBody` — POST, PUT, PATCH (has `body` field)
  - `body`: `rest$ImplicitMappingBody` | `rest$JsonBody` | `rest$StringBody`
- `rest$RestOperationMethodWithoutBody` — GET, DELETE, HEAD, OPTIONS

**Authentication** (`rest$AuthenticationScheme`):
- `rest$BasicAuthenticationScheme` — `username: rest$value`, `password: rest$value`
  - Values are `rest$ConstantValue` (references a constant) or `rest$StringValue` (literal)
- `null` — no authentication

**Response Handling** (`rest$RestOperationResponseHandling`):
- `rest$ImplicitMappingResponseHandling` — `ContentType`, `RootMappingElement`, `StatusCode`
- `rest$NoResponseHandling` — `ContentType`, `StatusCode`

**Body** (`rest$body`):
- `rest$ImplicitMappingBody` — export mapping with `RootMappingElement`
- `rest$JsonBody` — raw JSON string in `value`
- `rest$StringBody` — template string in `ValueTemplate`

**Query Parameter Usage** (`rest$QueryParameterUsage`):
- `rest$RequiredQueryParameterUsage`
- `rest$OptionalQueryParameterUsage` — has `Included` bool flag

## Proposed MDL Syntax

### Design Principles

1. **Roundtrip**: DESCRIBE output must be valid CREATE input
2. **Consistency**: Follow existing CREATE REST CLIENT grammar structure (BEGIN...END blocks)
3. **Alignment**: Match the syntax in `06-rest-client-examples.mdl`
4. **Simplicity**: Use MDL-native types ($variables, data types) rather than exposing BSON internals

### SHOW REST CLIENTS

```sql
show rest clients [in module]
```

| Module | Name | Base URL | Auth | Operations |
|--------|------|----------|------|------------|
| RestTest | RC001_SimpleAPI | https://reqbin.com | NONE | 1 |
| RestTest | RC018_PetStoreAPI | https://petstore.swagger.io/v2 | NONE | 6 |

### DESCRIBE REST CLIENT

```sql
describe rest client Module.Name
```

Outputs a valid `create rest client` statement:

```sql
/**
 * Swagger Pet Store API
 * A complete REST client for the classic Pet Store demo API.
 */
create rest client RestTest.PetStoreAPI
base url 'https://petstore.swagger.io/v2'
authentication none
begin
  /** List all pets with optional filtering */
  operation ListPets
    method get
    path '/pet/findByStatus'
    query $status: string
    header 'Accept' = 'application/json'
    timeout 30
    response json as $PetList;

  /** Get a single pet by ID */
  operation GetPet
    method get
    path '/pet/{petId}'
    parameter $petId: integer
    header 'Accept' = 'application/json'
    response json as $Pet;

  /** Create a new pet */
  operation AddPet
    method post
    path '/pet'
    header 'Content-Type' = 'application/json'
    header 'Accept' = 'application/json'
    body json from $NewPet
    response json as $CreatedPet;

  /** Delete a pet */
  operation RemovePet
    method delete
    path '/pet/{petId}'
    parameter $petId: integer
    header 'api_key' = $ApiKey
    response none;
end;
```

### CREATE REST CLIENT

Full syntax reference:

```sql
create rest client qualifiedName
base url 'url'
authentication authScheme
begin
  operationDef*
end;
```

**Authentication schemes:**

```sql
authentication none
authentication basic (username = 'literal', password = 'literal')
authentication basic (username = $Variable, password = $Variable)
```

**Operation definition:**

```sql
[docComment]
operation name
  method get|post|put|patch|delete
  path '/path/{param}'
  [parameter $name: type]*          -- path parameters (extracted from {param} in PATH)
  [query $name: type]*              -- query parameters
  [header 'name' = headerValue]*    -- static or dynamic headers
  [body bodySpec]                   -- request body (POST/PUT/PATCH only)
  [timeout seconds]                 -- override default 300s
  response responseSpec;            -- response handling
```

**Header values:**

```sql
header 'Accept' = 'application/json'           -- static literal
header 'X-Request-ID' = $RequestId             -- dynamic from parameter
header 'Authorization' = 'Bearer ' + $token    -- concatenation
```

**Body types:**

```sql
body json from $Variable       -- JSON body from variable (maps to ImplicitMappingBody)
body file from $FileDocument   -- binary file upload (maps to StringBody with file content)
```

**Response types:**

```sql
response json as $Variable     -- JSON response mapped to entity
response string as $Variable   -- raw string response
response file as $Variable     -- binary file download
response status as $Variable   -- HTTP status code only
response none                  -- no response expected
```

### DROP REST CLIENT

```sql
drop rest client Module.Name;
```

## BSON ↔ MDL Mapping

### Authentication

| MDL | BSON |
|-----|------|
| `authentication none` | `AuthenticationScheme: null` |
| `authentication basic (username = 'user', password = 'pass')` | `AuthenticationScheme: rest$BasicAuthenticationScheme { username: rest$StringValue, password: rest$StringValue }` |
| `authentication basic (username = $Var, password = $Var)` | `AuthenticationScheme: rest$BasicAuthenticationScheme { username: rest$ConstantValue, password: rest$ConstantValue }` |

### Operation Method

| MDL | BSON |
|-----|------|
| `method get` (no BODY) | `rest$RestOperationMethodWithoutBody { HttpMethod: "get" }` |
| `method delete` (no BODY) | `rest$RestOperationMethodWithoutBody { HttpMethod: "delete" }` |
| `method post` + `body ...` | `rest$RestOperationMethodWithBody { HttpMethod: "post", body: ... }` |
| `method put` + `body ...` | `rest$RestOperationMethodWithBody { HttpMethod: "put", body: ... }` |
| `method patch` + `body ...` | `rest$RestOperationMethodWithBody { HttpMethod: "patch", body: ... }` |

### Response Handling

| MDL | BSON |
|-----|------|
| `response none` | `rest$NoResponseHandling` |
| `response status as $Var` | `rest$NoResponseHandling { StatusCode: ... }` |
| `response json as $Var` | `rest$ImplicitMappingResponseHandling { ContentType: "application/json" }` |
| `response string as $Var` | `rest$NoResponseHandling` (with string result handling) |
| `response file as $Var` | `rest$NoResponseHandling` (with file result handling) |

### Headers

| MDL | BSON |
|-----|------|
| `header 'Accept' = 'application/json'` | `rest$HeaderWithValueTemplate { Name: "Accept", value: rest$ValueTemplate { value: "application/json" } }` |
| `header 'auth' = 'Bearer ' + $token` | `rest$HeaderWithValueTemplate { Name: "auth", value: rest$ValueTemplate { value: "Bearer {1}" } }` + parameter reference |

### Parameters

| MDL | BSON |
|-----|------|
| `parameter $userId: integer` | `rest$RestOperationParameter { Name: "userId", DataType: integer }` (path parameter, matches `{userId}` in PATH) |
| `query $search: string` | `rest$QueryParameter { Name: "search", ParameterUsage: rest$RequiredQueryParameterUsage }` |

## Implementation Plan

### Phase 1: Read Support (SHOW / DESCRIBE)

#### 1.1 Add Model Types (`model/types.go`)

```go
// ConsumedRestService represents a consumed rest service document.
type ConsumedRestService struct {
    ContainerID    ID
    Name           string
    documentation  string
    Excluded       bool
    BaseUrl        string
    authentication *RestAuthentication // nil = none
    Operations     []*RestClientOperation
}

// RestAuthentication represents authentication configuration.
type RestAuthentication struct {
    Scheme   string // "basic"
    username string // literal value or constant reference
    password string // literal value or constant reference
}

// RestClientOperation represents a single rest operation.
type RestClientOperation struct {
    Name            string
    documentation   string
    HttpMethod      string // "get", "post", etc.
    path            string
    parameters      []*RestClientParameter // path parameters
    QueryParameters []*RestClientParameter // query parameters
    headers         []*RestClientHeader
    BodyType        string // "json", "file", "" (none)
    BodyVariable    string // variable name for body
    ResponseType    string // "json", "string", "file", "status", "none"
    ResponseVariable string // variable name for response
    timeout         int    // 0 = default
}

// RestClientParameter represents a path or query parameter.
type RestClientParameter struct {
    Name     string
    DataType string // "string", "integer", "boolean", "decimal"
}

// RestClientHeader represents an HTTP header.
type RestClientHeader struct {
    Name  string
    value string // literal value or expression with parameters
}
```

#### 1.2 Add MPR Parser (`sdk/mpr/parser_rest.go`)

Extend existing file with:

```go
func (r *Reader) parseConsumedRestService(doc bson.Raw) *model.ConsumedRestService
```

Handles the polymorphic types: `RestOperationMethodWithBody` vs `WithoutBody`, `BasicAuthenticationScheme` vs null, `ImplicitMappingResponseHandling` vs `NoResponseHandling`.

#### 1.3 Add Reader (`sdk/mpr/reader_documents.go`)

```go
func (r *Reader) ListConsumedRestServices() []*model.ConsumedRestService
```

Pattern: follow `ListPublishedRestServices()` — query documents by `$type = "rest$ConsumedRestService"`.

#### 1.4 Add AST Types (`mdl/ast/ast_query.go`)

Add to existing enums:

```go
// in ShowObjectType
ShowRestClients

// in DescribeObjectType
DescribeRestClient
```

#### 1.5 Add Visitor (`mdl/visitor/visitor_query.go`)

Add cases in `exitShowStatement()` and `exitDescribeStatement()`:

```go
// show rest clients [in module]
if ctx.REST() != nil && ctx.CLIENTS() != nil { ... }

// describe rest client qualifiedName
if ctx.REST() != nil && ctx.CLIENT() != nil { ... }
```

#### 1.6 Add Executor (`mdl/executor/cmd_rest_clients.go`)

New file, following `cmd_odata.go` pattern:

```go
func (e *Executor) showRestClients(moduleName string) error
func (e *Executor) describeRestClient(name ast.QualifiedName) error
func (e *Executor) outputConsumedRestServiceMDL(svc *model.ConsumedRestService) string
```

The `outputConsumedRestServiceMDL()` function must produce valid CREATE REST CLIENT syntax that roundtrips.

#### 1.7 Add Autocomplete

```go
func (e *Executor) GetRestClientNames(moduleFilter string) []string
```

### Phase 2: Write Support (CREATE / DROP)

#### 2.1 Add AST Types (`mdl/ast/ast_rest.go`)

New file:

```go
type CreateRestClientStmt struct {
    Name           QualifiedName
    BaseUrl        string
    authentication *RestAuthDef // nil = none
    Operations     []*RestOperationDef
    documentation  string
    CreateOrModify bool
}

type RestAuthDef struct {
    Scheme   string // "basic"
    username string // literal or $variable
    password string // literal or $variable
}

type RestOperationDef struct {
    Name            string
    documentation   string
    method          string // "get", "post", etc.
    path            string
    parameters      []RestParamDef // path parameters
    QueryParameters []RestParamDef // query parameters
    headers         []RestHeaderDef
    BodyType        string // "json", "file", ""
    BodyVariable    string
    ResponseType    string // "json", "string", "file", "status", "none"
    ResponseVariable string
    timeout         int
}

type RestParamDef struct {
    Name     string // includes $ prefix
    DataType string
}

type RestHeaderDef struct {
    Name  string
    value string // can be literal, $variable, or 'prefix' + $variable
}

type DropRestClientStmt struct {
    Name QualifiedName
}
```

#### 2.2 Update Grammar (`MDLParser.g4`)

The existing grammar rules (lines 1997-2048) need significant revision to match the MDL syntax:

```antlr
createRestClientStatement
    : rest client qualifiedName
      restClientBaseUrl
      restClientAuthentication
      begin restOperationDef* end
    ;

restClientBaseUrl
    : base url STRING_LITERAL
    ;

restClientAuthentication
    : authentication none
    | authentication basic LPAREN
        username ASSIGN restAuthValue COMMA
        password ASSIGN restAuthValue
      RPAREN
    ;

restAuthValue
    : STRING_LITERAL          // literal: 'api_user'
    | VARIABLE               // parameter: $ApiUsername
    ;

restOperationDef
    : documentationComment?
      operation (IDENTIFIER | STRING_LITERAL)
        method restHttpMethod
        path STRING_LITERAL
        restOperationClause*
        response restResponseSpec SEMICOLON
    ;

restHttpMethod
    : get | post | put | patch | delete
    ;

restOperationClause
    : parameter VARIABLE COLON dataType                         // path param
    | query VARIABLE COLON dataType                             // query param
    | header STRING_LITERAL ASSIGN restHeaderValue              // header
    | body (json | file) from VARIABLE                          // request body
    | timeout NUMBER_LITERAL                                    // timeout override
    ;

restHeaderValue
    : STRING_LITERAL                                            // 'application/json'
    | VARIABLE                                                  // $RequestId
    | STRING_LITERAL PLUS VARIABLE                              // 'Bearer ' + $token
    ;

restResponseSpec
    : json as VARIABLE         // json response
    | string as VARIABLE       // string response
    | file as VARIABLE         // file download
    | status as VARIABLE       // status code only
    | none                     // no response
    ;
```

**New tokens needed:** `file` (if not already defined), `string` (keyword, not `STRING_LITERAL`).

#### 2.3 Add Visitor (`mdl/visitor/visitor_rest.go`)

New file to convert grammar parse tree → AST:

```go
func (b *ASTBuilder) exitCreateRestClientStatement(ctx *parser.CreateRestClientStatementContext)
```

#### 2.4 Add Writer (`sdk/mpr/writer_rest.go`)

New file for BSON serialization:

```go
func (w *Writer) WriteConsumedRestService(svc *model.ConsumedRestService) error
```

Must correctly produce:
- Polymorphic method types (`RestOperationMethodWithBody` vs `WithoutBody`)
- Authentication scheme or null
- ValueTemplate structures for URLs, paths, headers
- Response handling types
- Query parameter usage types

#### 2.5 Add Executor Create/Drop Handlers

In `mdl/executor/cmd_rest_clients.go`:

```go
func (e *Executor) createRestClient(s *ast.CreateRestClientStmt) error
func (e *Executor) dropRestClient(s *ast.DropRestClientStmt) error
```

### Phase 3: Test Enablement

Remove the `exit;` guard from `06-rest-client-examples.mdl` (line 34) and verify all 21 examples parse and execute correctly.

## Complexity

**Medium-High** — Multiple polymorphic BSON types (method, body, response handling, authentication, parameter values), value templates with parameter interpolation, and header expression parsing. The OData implementation provides a proven template but REST has more polymorphic variance.

## Testing

- Parse all 21 examples in `06-rest-client-examples.mdl` with `mxcli check`
- Roundtrip test: CREATE → DESCRIBE → re-parse must produce identical AST
- Verify BSON output against Evora project's existing consumed REST services
- **Important**: Before writing BSON, create a reference REST client in Studio Pro and compare the generated BSON structure field-by-field

## Related

- `docs/03-development/proposals/show-describe-published-rest-services.md` — Published REST services (opposite direction: exposing endpoints)
- `mdl/executor/cmd_odata.go` — OData client implementation (template for this work)
- `mdl/ast/ast_odata.go` — OData AST types (template for REST AST)
- `mdl/executor/cmd_microflows_builder_calls.go:576` — Existing REST CALL microflow action (different feature: inline REST calls in microflows)
