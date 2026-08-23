# REST, SOAP, Java actions and files

Supporting reference for [write-microflows](../SKILL.md).

## Legacy SOAP Web Service Calls

`call web service` preserves legacy Mendix SOAP activities. Prefer REST clients
for new integrations; this syntax exists mainly so existing projects can
round-trip without dropping SOAP actions.

```mdl
-- Structured form. Resolved SOAP references use normal qualified names.
$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
send mapping SampleSOAP.OrderRequest
receive mapping SampleSOAP.OrderResponse
timeout 30
on error rollback;

-- Quoted raw IDs are accepted when old project references are dangling or unavailable.
$Root = call web service 'sample-service-id'
operation FetchSampleItems
send mapping 'sample-send-mapping-id'
receive mapping 'sample-receive-mapping-id';

-- Raw escape hatch emitted for unsupported SOAP fields.
$Root = call web service raw 'AQID';
```

**Design note:** the raw payload is base64-encoded BSON for the complete action
and is authoritative on re-exec. Treat this as round-trip support, not a
recommended authoring format for new integrations.
## REST Service Calls

MDL supports two patterns for calling REST APIs from microflows:

### SEND REST REQUEST — Consumed REST Service Operations

Calls an operation defined in a consumed REST service (created via `create rest client`). The URL, headers, authentication, and response mapping are configured in the REST client document — the microflow only references the operation.

```mdl
-- Fire and forget (RESPONSE NONE operation)
send rest request Module.ServiceName.OperationName;

-- With output variable (RESPONSE JSON operation — maps to entity)
$Result = send rest request Module.ServiceName.OperationName;

-- With request body (POST/PUT operations)
$Result = send rest request Module.ServiceName.CreateItem
    body $NewItem;
```

**CRITICAL: `$latestHttpResponse` system variable**

After every `send rest request`, Mendix automatically populates `$latestHttpResponse` (type `System.HttpResponse`). Use this to check call success — do **NOT** check the output variable directly:

```mdl
-- ✅ CORRECT: check $latestHttpResponse
$RootResult = send rest request Module.Service.GetData;
if $latestHttpResponse/Content != empty then
  -- Process $RootResult (the mapped entity)
end if;

-- ❌ WRONG: checking the output variable directly causes CE0117
if $RootResult != empty then  -- ERROR!
```

**Key attributes on `$latestHttpResponse`:**
- `Content` (String) — response body as string. **Capital `C`** — it is inherited from the parent entity `System.HttpMessage`, so `DESCRIBE ENTITY System.HttpResponse` does not list it. Lowercase `$latestHttpResponse/content` fails CE0117.
- `StatusCode` (Integer) — HTTP status code (200, 404, etc.)

**Restrictions:**
- `send rest request` does **NOT** support custom error handling (`on error continue/rollback` causes CE6035). Errors are always handled by aborting.
- The operation must be defined via `create rest client` with a three-part qualified name: `Module.ServiceDocument.OperationName`.

### REST CALL — Inline HTTP Calls

Direct HTTP call with URL, headers, auth, body, and response handling specified inline. Useful for one-off calls or when no REST client document exists.

```mdl
-- Simple GET returning string
$response = rest call get 'https://api.example.com/data'
    header Accept = 'application/json'
    timeout 30
    returns string;

-- POST with JSON body
$response = rest call post 'https://api.example.com/items'
    header 'Content-Type' = 'application/json'
    header Accept = 'application/json'
    body '{{"name": "{1}", "value": {2}}' with (
        {1} = $ItemName,
        {2} = toString($ItemValue)
    )
    timeout 30
    returns string
    on error continue;

-- POST a BINARY body (upload a file document's contents)
-- The expression is the FileDocument's Contents MEMBER, not the document
-- itself, and the content type goes on a header. A consumed REST CLIENT
-- document has no binary body — `Body: file from $Doc` there is refused as
-- MDL-REST02 — so binary uploads belong here.
$response = rest call post 'https://api.example.com/upload'
    header 'ContentType' = 'application/pdf'
    body binary $Doc/Contents
    timeout 300
    returns response;

-- GET with URL template parameters
$response = rest call get 'https://api.example.com/users/{1}' with (
    {1} = toString($UserId)
)
    header Accept = 'application/json'
    returns string;

-- With basic authentication
$response = rest call get 'https://api.example.com/secure'
    header Accept = 'application/json'
    auth basic $username password $password
    timeout 30
    returns string;

-- DELETE (no response)
rest call delete 'https://api.example.com/items/{1}' with (
    {1} = $ItemId
)
    returns nothing
    on error continue;
```

**REST CALL response types:**
- `returns string` — response body as string variable
- `returns nothing` / `returns none` — ignore response
- `returns response` — returns `System.HttpResponse` object
- `returns mapping Module.ImportMapping as Module.Entity` — single object result
- `returns mapping Module.ImportMapping as list of Module.Entity` — list result
- `returns Module.MyFile` — store the body in a **file document**

**The file document form takes a specialization, never `System.FileDocument`
itself.** Mendix rejects the base type as a return type with `CE0362`, and
MDL064 reports it before the write. Create one first:

```mdl
create persistent entity MyModule.MyFile extends System.FileDocument ();

create microflow MyModule.ACT_Download ($Location: String)
begin
  $file = rest call get '{1}' with ({1} = $Location)
    header 'Accept' = 'application/octet-stream'
    timeout 300
    returns MyModule.MyFile;
end;
```

There is **no** equivalent for an HttpResponse specialization: Mendix allows only
`User`, `FileDocument`, `Image` and `Paging` to be specialized (`CE1540`), so
`returns response` already names the only type that result can have.

**Pick `as` vs `as list of` based on the call site, not the mapping shape.** The same import mapping can yield either a single object or a list — Studio Pro stores the cardinality on the microflow's `ImportMappingCall` (`Range.SingleObject` + `ForceSingleOccurrence`). Use `as Module.Entity` when the response is a single object (the mapping may still be list-typed; Studio Pro binds the first item). Use `as list of Module.Entity` when the response should bind a list. Mismatching the cardinality with the surrounding code produces `mx check` `CE0117` at the End event or `CE0013` / `CE0100` on downstream loop / aggregate / list-operation activities.

**REST CALL supports full error handling** (`on error continue`, `on error rollback`, custom error handlers).
## File Downloads

Use `download file` to stream a `System.FileDocument` from a microflow. Add
`show in browser` when the action should open the file inline instead of forcing
a download.

```mdl
download file $GeneratedReport show in browser;
download file $GeneratedExport;
```
## Empty Java-Action Argument (`empty`)

When `describe` round-trips a Java-action call that has an unbound parameter
in Studio Pro, it emits `empty` as the argument value. In this Java-action
argument context, `empty` preserves the
underlying empty `BasicCodeActionParameterValue.Argument` so that the next
`describe → exec → describe` cycle stays symmetric.

```mdl
$Total = call java action SampleModule.Recalculate(
  CompanyId       = empty,
  RecalculateAll  = true,
  ItemList        = empty
);
```

New scripts should bind every parameter to a real expression. Use `empty`
for a Java-action argument only when regenerating MDL from an existing project
that already had an unbound parameter.
## Microflow-Typed Java-Action Parameters

Some Java actions take a **microflow** — a callback the action invokes later.
`MCPServer.AddTool` (`ExecutingMicroflow`) and `MCPServer.CreateMCPServer`
(`AuthenticationMicroflow`) are the ones you meet first. Pass the microflow's
qualified name as a quoted string; mxcli resolves the parameter's declared type
from the Java action and stores a microflow reference, not a string literal.

```mdl
$Tool = call java action MCPServer.AddTool(
  McpServer          = $Server,
  Name               = 'memory_add',
  Description        = 'Stores a memory',
  ExecutingMicroflow = 'MyModule.MF_MemoryAdd',
  Schema             = ''
);
```

`DESCRIBE JAVA ACTION` prints such a parameter's type as the bare word
`Microflow` (`Nanoflow` for JavaScript actions), and that spelling is what
`CREATE JAVA ACTION` accepts, so the round-trip is stable.
