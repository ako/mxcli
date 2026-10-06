# REST Integration

Calling external APIs from microflows -- GET, POST, authentication, and error handling.

## Simple GET

```sql
CREATE MICROFLOW Integration.FetchWebpage ()
RETURNS String AS $Content
BEGIN
  $Content = CALL REST SERVICE GET 'https://example.com/api/status' (
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Content;
END;

```

## GET with URL Parameters

```sql
CREATE MICROFLOW Integration.SearchProducts (
  $Query: String,
  $Page: Integer
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE GET 'https://api.example.com/search?q={1}&page={2}' WITH (
    {1} = urlEncode($Query),
    {2} = toString($Page)
  ) (
    Headers: ('Accept': 'application/json'),
    Timeout: 60,
  ) RETURNS String;
  RETURN $Response;
END;

```

## POST with JSON Body

```sql
CREATE MICROFLOW Integration.CreateCustomer (
  $Name: String,
  $Email: String
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE POST 'https://api.example.com/customers' (
    Headers: ('Content-Type': 'application/json'),
    Body: TEMPLATE '{{"name": "{1}", "email": "{2}"}' WITH (
      {1} = $Name,
      {2} = $Email
    ),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Response;
END;

```

## POST a Binary Body (File Upload)

Mendix models a binary request body on the **REST CALL activity**, not on a
consumed REST client document. The expression is the file document's `Contents`
member, and the content type goes on a header.

```sql
CREATE MICROFLOW Integration.UploadDocument (
  $Doc: Integration.UploadedFile
)
RETURNS Boolean AS $Ok
BEGIN
  DECLARE $Ok Boolean = false;
  $Response = CALL REST SERVICE POST 'https://api.example.com/documents' (
    Headers: ('ContentType': 'application/pdf'),
    Body: BINARY $Doc/Contents,
    Timeout: 300,
  ) RETURNS response;
  SET $Ok = $Response/StatusCode = 200;
  RETURN $Ok;
END;

```

`RETURNS response` binds a `System.HttpResponse`, so the microflow's own return
type must match what you do with it — returning `$Response` from a
`RETURNS String` microflow is `CE0117` at the end event.

`$Doc` must be a specialization of `System.FileDocument`.

A consumed REST **client document** has no binary body — its three body types
(`Rest$JsonBody`, `Rest$StringBody`, `Rest$ImplicitMappingBody`) are all textual
— so `Body: file from $Doc` in a `CREATE CONSUMED REST SERVICE` operation is refused as
**MDL-REST02**. Downloading is unaffected: `Response: file as $Doc` works.

## Basic Authentication

```sql
CREATE MICROFLOW Integration.FetchSecureData (
  $Username: String,
  $Password: String
)
RETURNS String AS $Response
BEGIN
  $Response = CALL REST SERVICE GET 'https://api.example.com/secure/data' (
    Headers: ('Accept': 'application/json'),
    Authentication: BASIC (Username: $Username, Password: $Password),
    Timeout: 30,
  ) RETURNS String;
  RETURN $Response;
END;

```

## Error Handling

Use `ON ERROR WITHOUT ROLLBACK` to catch failures and return a fallback instead of rolling back the transaction:

```sql
CREATE MICROFLOW Integration.SafeAPICall (
  $Url: String
)
RETURNS Boolean AS $Success
BEGIN
  DECLARE $Success Boolean = false;

  $Response = CALL REST SERVICE GET $Url (
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
  ) RETURNS String
    ON ERROR WITHOUT ROLLBACK BEGIN
      LOG ERROR NODE 'Integration' 'API call failed: ' + $Url;
      RETURN false;
    END ERROR;

  SET $Success = true;
  RETURN $Success;
END;

```

## JSON Structure + Import Mapping

Map a JSON response to Mendix entities using a JSON structure and import mapping.

```sql
mdl 1;
-- Step 1: Define the JSON structure
CREATE JSON STRUCTURE Integration.JSON_Pet
  SAMPLE '{"id": 1, "name": "Fido", "status": "available"}';

-- Step 2: Create a non-persistent entity to hold the data
CREATE NON-PERSISTENT ENTITY Integration.PetResponse (
  PetId: Integer,
  Name: String,
  Status: String
);

-- Step 3: Create the import mapping
CREATE IMPORT MAPPING Integration.IMM_Pet
  WITH JSON STRUCTURE Integration.JSON_Pet
{
  CREATE Integration.PetResponse {
    PetId = id,
    Name = name,
    Status = status
  }
};

-- Step 4: Use in a microflow with IMPORT FROM MAPPING
-- $PetResponse = IMPORT FROM MAPPING Integration.IMM_Pet($JsonContent);
```

## Export Mapping (Entity → JSON)

Serialize entities back to JSON using an export mapping.

```sql
CREATE EXPORT MAPPING Integration.EMM_Pet
  WITH JSON STRUCTURE Integration.JSON_Pet
{
  Integration.PetResponse {
    id = PetId,
    name = Name,
    status = Status
  }
};

-- Use in a microflow with EXPORT TO MAPPING
-- $JsonOutput = EXPORT TO MAPPING Integration.EMM_Pet($PetResponse);
```

## Nested Mappings with Associations

Map nested JSON objects to multiple entities linked by associations.

```sql
CREATE IMPORT MAPPING Integration.IMM_Order
  WITH JSON STRUCTURE Integration.JSON_Order
{
  CREATE Integration.OrderResponse {
    OrderId = orderId KEY,
    CREATE Integration.OrderResponse_CustomerInfo/Integration.CustomerInfo = customer {
      Email = email,
      Name = name
    },
    CREATE Integration.OrderResponse_OrderItem/Integration.OrderItem = items {
      Sku = sku,
      Quantity = quantity,
      Price = price
    }
  }
};
```

## Consuming a REST API

### From an OpenAPI Spec (Recommended)

If the API has an OpenAPI 3.0 spec (JSON or YAML), generate the REST client in one command:

```sql
mdl 1;
-- From a local file (relative to the .mpr file)
CREATE OR MODIFY CONSUMED REST SERVICE CapitalModule.CapitalAPI (
  OpenAPI: 'specs/capital.json'
);

-- From a URL
CREATE OR MODIFY CONSUMED REST SERVICE PetStoreModule.PetStoreAPI (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json'
);
```

Operations, path/query parameters, request bodies, response types, resource groups (from `tags`), and Basic auth are all derived automatically from the spec.

**Preview without writing:**
```sql
DESCRIBE CONTRACT OPERATION FROM OPENAPI 'specs/capital.json';
```

### Manual Definition

Define a reusable REST client with typed operations using `CREATE CONSUMED REST SERVICE`. Each operation declares its method, path, optional parameters, headers, body, and response mapping.

```sql
-- Define a client for the orders API
CREATE CONSUMED REST SERVICE Integration.OrdersApi (
  BaseUrl: 'https://api.example.com/v1',
  Authentication: NONE
)
{
  OPERATION GetOrder (
    Method: GET,
    Path: '/orders/{id}',
    Parameters: ($id: String),
    Headers: ('Accept': 'application/json'),
    Timeout: 30,
    Response: JSON AS $Result
  )

  OPERATION CreateOrder (
    Method: POST,
    Path: '/orders',
    Headers: ('Content-Type': 'application/json'),
    Body: MAPPING Integration.OrderRequest {
      customerId = CustomerId,
      totalAmount = TotalAmount,
      notes = Notes,
    },
    Response: MAPPING Integration.OrderResponse {
      Id = id,
      Status = status,
      CreatedAt = createdAt,
    }
  )
};
```

A header value is a template, like the path: `{Token}` is the operation parameter `Token`, which the operation must declare (`Parameters: ($Token: String)`, else CE7056). An authorization header is `Headers: ('Authorization': 'Bearer {Token}')`. The expression spelling `'Bearer ' + $Token` is deprecated (MDL-DEPR711) and `mxcli fmt --upgrade` rewrites it; it used to store only `Bearer `.

Use `CREATE OR MODIFY CONSUMED REST SERVICE` to update an existing client without dropping it first:

```sql
CREATE OR MODIFY CONSUMED REST SERVICE Integration.OrdersApi (
  BaseUrl: 'https://api.example.com/v2',
  Authentication: BASIC (Username: 'apiuser', Password: 'secret')
)
{
  OPERATION GetOrder (
    Method: GET,
    Path: '/orders/{id}',
    Parameters: ($id: String),
    Headers: ('Accept': 'application/json'),
    Timeout: 60,
    Response: JSON AS $Result
  )
};
```

**Body types:** `JSON FROM $var`, `TEMPLATE '...'`, `MAPPING Entity { jsonField = Attr, ... }`
**Response types:** `JSON AS $var`, `STRING AS $var`, `FILE AS $var`, `STATUS AS $var`, `NONE`, `MAPPING Entity { Attr = jsonField, ... }`
**Authentication:** `NONE`, `BASIC (Username: '...', Password: '...')`

## Publishing a REST API

Create a published REST service with CRUD operations backed by microflows.

```sql
CREATE PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API'
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetAllOrders;
    GET '{id}' MICROFLOW Module.PRS_GetOrderById;
    POST '' MICROFLOW Module.PRS_CreateOrder;
    PUT '{id}' MICROFLOW Module.PRS_UpdateOrder;
    DELETE '{id}' MICROFLOW Module.PRS_DeleteOrder;
  }
};
```

**Operation paths:** Use empty string `''` for the root, `'{paramName}'` for path parameters. Do NOT start or end with `/`. Path parameters must match a microflow parameter name exactly (case-sensitive) — `'{id}'` binds the microflow's `$id`, whatever its type.

**Operation parameters** come from the microflow, the way Studio Pro derives them. Create the microflow before the service:

| Microflow parameter | Operation parameter |
|---|---|
| named in the path (`'{id}'`) | a path parameter |
| an object or a list | the body |
| `System.HttpRequest`, `System.HttpResponse` | none — they are the request and the response |
| anything else | a query parameter |

Each gets the microflow parameter's type. A header parameter, a renamed parameter or a description set in Studio Pro has no MDL spelling: `describe` notes it in a comment, and `CREATE OR MODIFY` / `ALTER` on that project keep it.

**Mappings and commit:** a body that is not a file document needs an import mapping, and a microflow returning an object or a list needs an export mapping:

```sql
mdl 1;
CREATE OR MODIFY PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Order API'
)
{
  RESOURCE 'orders' {
    -- COMMIT: Yes (the default without the clause) | YesWithoutEvents | No
    POST '' MICROFLOW Module.PRS_CreateOrder
      IMPORT MAPPING Module.IMM_Order EXPORT MAPPING Module.EMM_Order
      COMMIT YesWithoutEvents;
  }
};
```

### Multiple Resources

```sql
CREATE PUBLISHED REST SERVICE Module.CrmAPI (
  Path: 'rest/crm/v1',
  Version: '1.0.0',
  ServiceName: 'CRM API'
)
{
  RESOURCE 'orders' {
    GET '' MICROFLOW Module.PRS_GetOrders;
  }
  RESOURCE 'customers' {
    GET '' MICROFLOW Module.PRS_GetCustomers;
  }
  RESOURCE 'orders/{orderId}/items' {
    GET '' MICROFLOW Module.PRS_GetOrderItems;
  }
};
```

### Update or Remove

```sql
-- Replace with new version
CREATE OR REPLACE PUBLISHED REST SERVICE Module.OrderAPI (
  Path: 'rest/orders/v2',
  Version: '2.0.0',
  ServiceName: 'Order API v2'
) { ... };

-- Remove entirely
DROP PUBLISHED REST SERVICE Module.OrderAPI;
```

## Data Transformers (JSLT)

Data Transformers apply transformation steps (JSLT or XSLT) to JSON or XML payloads. Useful for reshaping API responses before import mapping, or normalising data from third-party sources. Requires Mendix 11.9+.

```sql
mdl 1;
-- Create a transformer that extracts key fields from a weather API response.
-- The SOURCE JSON defines a sample payload used for schema inference and testing.
CREATE DATA TRANSFORMER Integration.WeatherTransform
SOURCE JSON '{
  "latitude": 51.9,
  "longitude": 4.5,
  "timezone": "Europe/Amsterdam",
  "current": {
    "time": "2024-01-15T14:00",
    "temperature_2m": 12.8,
    "wind_speed_10m": 18.3,
    "weather_code": 3
  }
}'
{
  JSLT $$
{
  "lat":        .latitude,
  "lon":        .longitude,
  "timezone":   .timezone,
  "temp":       .current.temperature_2m,
  "wind_speed": .current.wind_speed_10m,
  "code":       .current.weather_code
}
  $$;
};

-- List all data transformers in the Integration module
LIST DATA TRANSFORMERS IN Integration;

-- Inspect a transformer (outputs a re-executable CREATE statement)
DESCRIBE DATA TRANSFORMER Integration.WeatherTransform;

-- Remove a transformer
DROP DATA TRANSFORMER Integration.WeatherTransform;
```

**Notes:**
- Steps execute in order; the output of each step feeds the next.
- `JSLT '...'` for short single-line expressions; `JSLT $$ ... $$` for multi-line.
- `XSLT $$ ... $$` is also supported for XML-to-XML transformations.
- Requires Mendix 11.9+. Use `LIST FEATURES` to confirm support before using.
