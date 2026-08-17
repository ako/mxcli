# OData Data Sharing Between Mendix Apps

This skill covers how to use OData services to share data between Mendix applications, with emphasis on using view entities as an abstraction layer to decouple the API contract from the internal domain model.

## When to Use This Skill

- User asks to expose data from one Mendix app to another
- User wants to set up inter-app communication via OData
- User needs to create an API layer that abstracts internal entities
- User asks about external entities, consumed/published OData services
- User wants to decouple modules or apps for independent deployment
- User asks about the view entity pattern for OData services
- User asks about local metadata files or offline OData development

## MetadataUrl Formats

`CREATE ODATA CLIENT` supports three formats for the `MetadataUrl` parameter:

| Format | Example | Stored In Model |
|--------|---------|-----------------|
| **HTTP(S) URL** | `https://api.example.com/odata/v4/$metadata` | Unchanged |
| **Absolute file:// URI** | `file:///Users/team/contracts/service.xml` | Unchanged |
| **Relative path** | `./metadata/service.xml` or `metadata/service.xml` | **Normalized to absolute `file://`** |

**Path Normalization:**
- Relative paths (with or without `./`) are **automatically converted** to absolute `file://` URLs in the Mendix model
- This ensures Studio Pro can properly detect local file vs HTTP metadata sources (radio button in UI)
- Example: `./metadata/service.xml` → `file:///absolute/path/to/project/metadata/service.xml`

**Path Resolution (before normalization):**
- With project loaded (`-p` flag or REPL): relative paths are resolved against the `.mpr` file's directory
- Without project: relative paths are resolved against the current working directory

**Use Cases for Local Metadata:**
- **Offline development** — no network access required
- **Testing and CI/CD** — reproducible builds with metadata snapshots
- **Version control** — commit metadata files alongside code
- **Pre-production** — test against upcoming API changes before deployment
- **Firewall-friendly** — works in locked-down corporate environments

## ServiceUrl Must Be a Constant

**IMPORTANT:** The `ServiceUrl` parameter **must always be a constant reference** (prefixed with `@`). Direct URLs are not allowed.

**Correct:**
```sql
CREATE CONSTANT ProductClient.ProductDataApiLocation
  TYPE String
  DEFAULT 'http://localhost:8080/odata/productdataapi/v1/';

CREATE ODATA CLIENT ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/$metadata',
  ServiceUrl: '@ProductClient.ProductDataApiLocation'  -- ✅ Constant reference
);
```

**Incorrect:**
```sql
CREATE ODATA CLIENT ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/$metadata',
  ServiceUrl: 'https://api.example.com/odata'  -- ❌ Direct URL not allowed
);
```

This enforces Mendix best practice of externalizing configuration values for different environments.

## Architecture Overview

OData data sharing follows a **producer/consumer** pattern with three layers:

```
┌─────────────────────────────────────────────┐
│  PRODUCER APP                               │
│                                             │
│  persistent entities  ──▶  view entities    │
│  (Shop.Customer,          (Api.CustomerVE)  │
│   Shop.Address)                             │
│                          ▼                  │
│                    odata service             │
│                   (Api.CustomerApi)          │
└──────────────────────┬──────────────────────┘
                       │ HTTP/OData4
┌──────────────────────▼──────────────────────┐
│  CONSUMER APP                               │
│                                             │
│                    odata client             │
│                  (Client.CustomerApiClient)  │
│                          ▼                  │
│                  external entities           │
│                 (Client.CustomersEE)         │
│                          ▼                  │
│                  pages & microflows          │
└─────────────────────────────────────────────┘
```

### Why View Entities?

Publishing persistent entities directly exposes your internal schema. When you change a column name, add a table, or restructure associations, every consumer app breaks. **View entities** solve this:

1. **Stable API contract** -- the view's shape stays the same even when the underlying tables change
2. **Flattened data** -- joins across multiple tables into a single flat resource (e.g., Customer + BillingAddress + DeliveryAddress into one `CustomerAddressVE`)
3. **Computed fields** -- add calculated columns like `FullAddress` or `ActivePrice` using OQL expressions
4. **Filtered datasets** -- restrict what's visible (e.g., only active products, cheap products)
5. **Aggregations** -- expose pre-aggregated metrics (e.g., orders per day, sum of line items)

## Step-by-Step: Read-Only API with View Abstraction

### Step 1: Create the Producer Module and Role

```sql
create module ProductApi;

create module role ProductApi.ApiUser
  description 'Role for OData API access';
```

### Step 2: Create View Entities as the API Layer

Instead of publishing `Shop.Product` and `Shop.Price` directly, create a view that joins and flattens them:

```sql
/**
 * Flattened product with current active price.
 * Joins Product with the most recent Price entry.
 */
create view entity ProductApi.ProductWithPriceVE (
  ProductId: integer,
  Name: string,
  description: string,
  PriceInEuro: decimal
) as (
  select p.ID         as ProdId
  ,      p.ProductId  as ProductId
  ,      p.Name       as Name
  ,      p.Description as description
  ,      ( select pr.PriceInEuro
           from   Shop.Price as pr
           where  pr.StartDate <= '[%BeginOfTomorrow%]'
           and    pr/Shop.Price_Product = p.ID
           order  by pr.StartDate desc
           limit  1
         ) as PriceInEuro
  from   Shop.Product as p
  where  p.IsActive
);

grant ProductApi.ApiUser on ProductApi.ProductWithPriceVE
  (read *, write *);
```

For aggregated data:

```sql
/**
 * Daily sales totals for cheap products.
 */
create view entity ProductApi.CheapProductSalesVE (
  OrderDate: datetime,
  TotalItems: long
) as (
  select o.OrderDate     as OrderDate
  ,      sum(ol.Amount)  as TotalItems
  from   Shop.OrderLine as ol
    left join Shop.OrderLine_Order/Shop."Order" as o
  where  ol/Shop.OrderLine_Product/Shop.Product.PriceInEuro < 100
  group by o.OrderDate
  order by o.OrderDate desc
  limit 1000
);

grant ProductApi.ApiUser on ProductApi.CheapProductSalesVE
  (read *, write *);
```

For flattening across associations:

```sql
/**
 * Customer with billing and delivery address flattened into one resource.
 */
create view entity ProductApi.CustomerAddressVE (
  CustomerId: long,
  CustomerName: string,
  Email: string,
  BillingStreet: string,
  BillingCity: string,
  BillingCountry: string,
  DeliveryStreet: string,
  DeliveryCity: string,
  DeliveryCountry: string
) as (
  select c.ID                              as CustomerID
  ,      c.CustomerId                      as CustomerId
  ,      c.FirstName + ' ' + c.LastName    as CustomerName
  ,      c.EmailAddress                    as Email
  ,      ba.Streetname                     as BillingStreet
  ,      ba.City                           as BillingCity
  ,      ba.Country                        as BillingCountry
  ,      da.Streetname                     as DeliveryStreet
  ,      da.City                           as DeliveryCity
  ,      da.Country                        as DeliveryCountry
  from   Shop.Customer as c
    left outer join c/Shop.BillingAddress_Customer/Shop.Address as ba
    left outer join c/Shop.DeliveryAddress_Customer/Shop.Address as da
);

grant ProductApi.ApiUser on ProductApi.CustomerAddressVE
  (read *, write *);
```

### Step 3: Publish the OData Service

```sql
/**
 * Product and customer data API.
 * Exposes flattened views for external consumers.
 */
create odata service ProductApi.ProductDataApi (
  path: 'odata/productdataapi/v1/',
  version: '1.0.0',
  ODataVersion: OData4,
  namespace: 'DefaultNamespace',
  ServiceName: 'ProductDataApi',
  Summary: 'Product and customer data API'
  -- PublishAssociations is left at its default (Yes = associations as links).
  -- Setting it to No means "associations as an associated object id", which
  -- Mendix only allows when the system ID is published as the key — publishing
  -- an ordinary attribute as the key then fails the build with CE7375, even
  -- when no associations are exposed at all.
)
authentication basic
{
  publish entity ProductApi.ProductWithPriceVE as 'Product' (
    ReadMode: ReadFromDatabase,
    InsertMode: NotSupported,
    UpdateMode: NotSupported,
    DeleteMode: NotSupported
  )
  expose (
    ProductId as 'ProductId' (Filterable, Sortable, key),
    Name as 'Name' (Filterable, Sortable),
    description as 'Description' (Filterable, Sortable),
    PriceInEuro as 'PriceInEuro' (Filterable, Sortable)
  );

  publish entity ProductApi.CustomerAddressVE as 'CustomerAddress' (
    ReadMode: ReadFromDatabase,
    InsertMode: NotSupported,
    UpdateMode: NotSupported,
    DeleteMode: NotSupported
  )
  expose (
    CustomerId as 'CustomerId' (Filterable, Sortable, key),
    CustomerName as 'CustomerName' (Filterable, Sortable),
    Email as 'Email' (Filterable, Sortable),
    BillingStreet as 'BillingStreet' (Filterable, Sortable),
    BillingCity as 'BillingCity' (Filterable, Sortable),
    BillingCountry as 'BillingCountry' (Filterable, Sortable),
    DeliveryStreet as 'DeliveryStreet' (Filterable, Sortable),
    DeliveryCity as 'DeliveryCity' (Filterable, Sortable),
    DeliveryCountry as 'DeliveryCountry' (Filterable, Sortable)
  );
};

grant access on odata service ProductApi.ProductDataApi
  to ProductApi.ApiUser;
```

### Step 4: Set Up the Consumer App

In the consuming application, create an OData client and external entities:

```sql
create module ProductClient;

create module role ProductClient.User;

-- Location constant (configure per environment)
create constant ProductClient.ProductDataApiLocation
  type string
  default 'http://localhost:8080/odata/productdataapi/v1/';

-- OData client connection
create odata client ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'http://localhost:8080/odata/productdataapi/v1/$metadata',
  timeout: 300,
  ServiceUrl: '@ProductClient.ProductDataApiLocation',
  UseAuthentication: Yes,
  HttpUsername: 'MxAdmin',
  HttpPassword: '1'
);

-- OData client with local file - relative path (offline development)
-- Resolved relative to .mpr directory when project is loaded
CREATE ODATA CLIENT ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: './metadata/productdataapi.xml',
  Timeout: 300,
  ServiceUrl: '@ProductClient.ProductDataApiLocation',
  UseAuthentication: Yes,
  HttpUsername: 'MxAdmin',
  HttpPassword: '1'
);

-- OData client with local file - relative path without ./
CREATE ODATA CLIENT ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'metadata/productdataapi.xml',
  Timeout: 300,
  ServiceUrl: '@ProductClient.ProductDataApiLocation',
  UseAuthentication: Yes,
  HttpUsername: 'MxAdmin',
  HttpPassword: '1'
);

-- OData client with local file - absolute file:// URI
CREATE ODATA CLIENT ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'file:///Users/team/contracts/productdataapi.xml',
  Timeout: 300,
  ServiceUrl: '@ProductClient.ProductDataApiLocation',
  UseAuthentication: Yes,
  HttpUsername: 'MxAdmin',
  HttpPassword: '1'
);

-- External entities (mapped from published service)
create external entity ProductClient.ProductsEE
from odata client ProductClient.ProductDataApiClient
(
  EntitySet: 'Product',
  RemoteName: 'Product',
  Countable: Yes
)
(
  ProductId: long,
  Name: string,
  description: string,
  PriceInEuro: decimal
);

grant ProductClient.User on ProductClient.ProductsEE (read *);

create external entity ProductClient.CustomerAddressesEE
from odata client ProductClient.ProductDataApiClient
(
  EntitySet: 'CustomerAddress',
  RemoteName: 'CustomerAddress',
  Countable: Yes
)
(
  CustomerId: long,
  CustomerName: string,
  Email: string,
  BillingStreet: string,
  BillingCity: string,
  BillingCountry: string,
  DeliveryStreet: string,
  DeliveryCity: string,
  DeliveryCountry: string
);

grant ProductClient.User on ProductClient.CustomerAddressesEE (read *);
```

**Bulk alternative:** Instead of creating external entities one by one, import all (or a subset) from the contract:

```sql
-- All entities from the service
create external entities from ProductClient.ProductDataApiClient;

-- Or specific ones only
create external entities from ProductClient.ProductDataApiClient
  entities (Product, CustomerAddress);

-- Idempotent re-import
create or modify external entities from ProductClient.ProductDataApiClient;
```

### AllowCreateChangeLocally — Read-Only API, Editable in the App

Use `AllowCreateChangeLocally: Yes` when the remote OData API only supports GET (read-only), but the Mendix app needs to let users edit the data locally before passing it to another API call — for example, an external action or a REST microflow that POSTs the change to a different endpoint.

Without this flag, external entities are completely non-editable in the client: form widgets are read-only and no in-memory change can be committed. With the flag, Mendix allows the object to be created and changed locally (in memory or in the Mendix database), without trying to write back through the OData client.

**Typical pattern:**
1. Retrieve data via the OData client into the external entity (GET only).
2. User edits the record in a page — possible because `AllowCreateChangeLocally` is set.
3. A microflow reads the changed object and calls an external action or REST operation (POST/PUT) to submit the change to the remote system.

```sql
-- API is read-only (no insert/update/delete on the OData endpoint).
-- AllowCreateChangeLocally lets users edit the object in the app
-- and submit changes via a separate external action.
create or modify external entity ShopClient.Product
from odata client ShopClient.ShopApiClient
(
  EntitySet: 'Products',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No,
  AllowCreateChangeLocally: Yes
)
(
  ProductId: long,
  Name: string,
  Price: decimal
);

-- Toggle the flag without recreating the entity.
alter entity ShopClient.Product set allow_create_change_locally = true;
alter entity ShopClient.Product set allow_create_change_locally = false;
```

## Publishing a Non-Persistable Entity (no copy of the data)

A published entity does **not** have to be persistable. Back it with a read
microflow and the rows are produced per request — nothing is stored, and there
is no refresh job to keep a copy in step with the source. This is the shape to
use when the data lives outside Mendix (an external database, a CSV, an API).

```sql
create non-persistent entity Api.Lap (
  LapKey:  string(60),
  Driver:  string(120),
  LapTime: decimal
);

-- While Countable is Yes (the default), the read microflow MUST take a
-- $Response: System.ODataResponse parameter — Mendix asks it for the count.
CREATE MICROFLOW Api.Read_Laps ($Response: System.ODataResponse)
  RETURNS List of Api.Lap AS $Laps
BEGIN
  -- retrieve from wherever the data actually lives, e.g. EXECUTE DATABASE QUERY
  $Laps = CREATE LIST OF Api.Lap;
  RETURN $Laps;
END;

create odata service Api.LapApi (
  path: 'odata/laps/',
  version: '1.0.0',
  ODataVersion: OData4,
  namespace: 'Api.Laps'
)
authentication basic
{
  publish entity Api.Lap as 'Laps' (
    ReadMode: microflow Api.Read_Laps,
    InsertMode: not_supported,
    UpdateMode: not_supported,
    DeleteMode: not_supported
  )
  expose (
    LapKey as 'lapKey' (KEY, Filterable, Sortable),
    Driver (Filterable, Sortable),
    LapTime (Sortable)
  );
};
```

Two things worth knowing before you write this:

- **`ReadMode: microflow Module.MF`** is the whole feature. `InsertMode`,
  `UpdateMode` and `DeleteMode` take the same form for a read-write resource.
- **Counting is not free.** If the count means a full scan of the underlying
  source, set `Countable: No` on the published entity — the read microflow then
  takes no parameters at all. `SkipSupported: No` and `TopSupported: No` turn
  off `$skip` and `$top` the same way. All three default to Yes.

`PublishAssociations` must stay at its default (Yes) — and not only here.

**It is not a yes/no, it is a two-value representation.** Studio Pro's own labels
for it are "As a link (recommended)" (Yes) and "As an associated object id" (No).
So `PublishAssociations: No` does not mean "this service publishes no
associations"; it selects the legacy representation, which requires the system
`ID` attribute published as the key. MDL cannot publish the system ID (CE1613),
so `No` cannot build from a script.

That holds even when the service publishes no associations at all, and even for
a persistable entity with a perfectly good key of its own. Measured on Mendix
11.13, both arms of the same service:

| | `mx check` |
|---|---|
| `PublishAssociations: No` | **CE7375** "Attribute ID … must be published and be the key when associations are exposed as an associated object id" |
| `PublishAssociations: Yes` | 0 errors |

The error names a concept the script never mentions, which is why this costs
hours rather than minutes. `mxcli check` now warns (MDL-ODATA06).

**Do not take CE7375's advice literally.** It says to publish the `ID` and make
it the key, and that is the wrong direction for anything you share outside the
app. The two representations exist for a reason: OData v3 had no link support,
so a foreign key had to be an exposed object id; v4 added links largely so
internal ids no longer had to leave the app. Going back to ids gives up that.

**A published key should be a business key.** Mendix object ids are
autogenerated and are not stable across an app landscape — the same record has
different ids in test, acceptance and production — so an id baked into an
external contract breaks the moment a consumer moves between environments, or
compares data from two of them. Pick something the business already guarantees:
an invoice number, an ISIN, an employee number. Mendix requires a key to be
unique, required and stable (the last is the point here), and the unique
validation rule it makes you add is checking exactly that.

That is also why the key needs `unique error '…'` on the attribute — see the
CE6624 note below. Both halves of the same idea: the value identifies one row,
and keeps identifying it.

### A view entity read from the database gets the query options for free

**This decides whether you need any pushdown machinery at all**, so check it
before reaching for Java.

A published resource has an `Action`: *Read from database*, or a read microflow.
The difference is not a detail:

| Action | `$filter` `$orderby` `$top` `$skip` `$count` |
|---|---|
| **Read from database** (a view entity, or a persistable one) | **Mendix applies them** — they reach the database |
| Read microflow | Mendix applies **none** of them; whatever the microflow returns is what the client gets |

Measured on 11.13 against a running app — an OQL view over four rows aggregating
to three, published with `Action: Read from database` and no Java anywhere:

```
$top=1                       -> 1 row, not 3
$count=true&$top=1           -> "@odata.count": 3, one row returned
$filter=Category eq 'Rent'   -> only the Rent row
$orderby=Total desc&$skip=1  -> [400, 250]   (1500 correctly skipped)
```

So for a **view entity**, aggregation happens in the database and paging and
filtering push down to it — a chart or grid can page a large resource with
nothing hand-written. That is the whole capability the `mendix-odata-pushdown`
pack exists to recreate.

**Which options a read microflow actually has to implement — measured, and not
all-or-nothing.** The same view served both ways on 11.13, three rows behind
each:

| option | database read | read microflow |
|---|---|---|
| `$select` | applied | **applied** — Mendix projects the response either way |
| `$filter` | applied | **200, unfiltered** |
| `$orderby` | applied | **200, unsorted** |
| `$top` / `$skip` | applied | **200, full set** |
| `$count` | applied | needs `System.ODataResponse` (CE6962) |

Two things follow that "Mendix applies none of them" gets wrong:

- **`$select` is not the microflow's correctness problem.** The client already
  receives only the fields it asked for. And the consumer drives it: removing
  attributes from an external entity narrows the `$select` it sends, because the
  external entity has nowhere to put what it dropped. So pushing `$select` into
  the source query is a *cost* optimisation — fewer columns read at the source —
  never a fix for wrong output.
- **Declaring the capability is what turns a safe refusal into a silent lie.**
  With `Filterable`/`Sortable` *not* declared, Mendix rejects the request:
  `400 "Property 'Category' is non-filterable."` Declare them — which you must,
  or no client can filter at all — and the identical request becomes 200 with
  every row. The declaration is a promise Mendix enforces at the boundary and
  does not keep for you.

That second one is the sharpest statement of why this work exists: the failure
is *created by* promising the capability, and the microflow is the only place
left to keep the promise.

The pack is for the case a view cannot cover: **the data is not in this app's
database at all**, so there is no table for a view to select from and a read
microflow is the only way to produce the rows. Mendix then applies nothing to
them — a `?$top=5` that quietly returns all 917 rows.

Its motivating shape is two apps, and the topology is what makes the pushdown
load-bearing rather than an optimisation:

```
frontend app  --- external entities / OData --->  backend app
(grid, chart)                                     (no data of its own)
                                                        |
                                          external database connector
                                                        |
                                                  DuckDB over CSV
```

The frontend's grid pages and filters by generating `$top` / `$skip` /
`$filter` — it has no other vocabulary, because external entities *are* OData.
The backend's read microflow has to translate those options into the SQL it
sends through the connector. Without that translation the frontend's paging
still looks correct while every page drags the whole file across, and nothing
in either app reports a problem.

Two consequences worth holding on to:

- **A view entity is not an option here**, so "prefer the view" is not advice
  that applies. The question is only whether *this app* owns the data.
- **The consumer's capability flags must match the service.** An external
  entity generated with `TopSupported`/`SkipSupported` that the service does
  not honour is CE6630 in the consuming app — the two ends of this contract
  are checked against each other.

If the resource *is* backed by this app's own tables, prefer a view entity and
skip the machinery entirely.

### An aggregate view's key is its grain

A summary resource — an OQL view entity, or a non-persistable row filled by a
read microflow — has no business key to reach for. Monthly totals per category
are not an invoice; nothing in the domain issues them a number.

**The key is the grain: the columns the aggregate groups by.** For monthly
totals per category that is `(Period, Category)` — together they identify
exactly one row, they are stable because they are the definition of the row, and
they mean the same thing in every environment.

What not to do is cast the internal id into a column (`cast(c.id as string) as
RowId`) and publish that. It satisfies "a key" and it is the id problem again,
one level down: autogenerated, environment-specific, and now stable only as long
as nobody rebuilds the view.

Measured on Mendix 11.13, each row a separate build:

| shape | result |
|---|---|
| single key attribute, persistable, no `unique` rule | **CE6624** — add one |
| single key attribute, persistable, `unique error '…'` | 0 errors |
| **single key attribute, VIEW entity, no `unique` rule** | **0 errors** |
| **composite key, `OData3`** | **CE7238** "You can only have more than one key attribute when the OData version is 4" |
| composite key, `OData4`, persistable, no `unique` rules | 0 errors |
| **composite key, `OData4`, non-persistable, no `unique` rules** | **0 errors** |
| any validation rule on a non-persistable entity | **CE0070** — not allowed |

Two consequences worth holding on to:

- **A grain key needs `ODataVersion: OData4`.** More than one key attribute is a
  v4 feature; on v3 the same model is CE7238.
- **A composite key needs no `unique` validation rule**, and a non-persistable
  entity could not carry one anyway (CE0070). The rule is only demanded for a
  *single*-attribute key, where one attribute has to be unique by itself —
  which is exactly the case a grain is not. So the CE6624 hurdle disappears
  the moment the key is honest about being multi-column.
- **CE6624 does not apply to a view entity at all.** A view can carry a
  *single*-attribute key with no validation rule and build cleanly — confirmed
  against a Studio Pro service publishing a view keyed on one column. So if the
  view already has a naturally unique column (an id carried through from the
  source data, not the platform's object id), key on that and skip the grain.
  Reach for the grain when no single column identifies a row — which is the
  normal case for an aggregate.

```sql
create non-persistent entity Fin.VMonthCategory (
  Period:   string(7),      -- 2026-08
  Category: string(60),
  Total:    decimal
);

create odata service Fin.ChartApi (
  path: 'odata/charts/', ServiceName: 'ChartApi', Namespace: 'Fin.Charts',
  version: '1.0.0', ODataVersion: OData4   -- required: the key is composite
) {
  publish entity Fin.VMonthCategory (
    ReadMode: microflow Fin.Read_MonthCategory,
    Countable: No                          -- else CE6962 wants System.ODataResponse
  )
  expose ( Period (KEY), Category (KEY), Total )
}
```

(Measured on 11.13: `PublishAssociations: Yes` builds under both `OData3` and
`OData4`, so choosing links is not a v4-only option in current Mendix.)

**`Path` has two rules and one trap.** No leading slash (CE6550), and it must end
with a single slash (CE6552). A path with **no slash at all** is the trap: mxbuild
throws `System.ArgumentOutOfRangeException` out of its own validator, with no
error code, no element name and no line, which reads as a corrupt project. Use
`'odata/thing/'`. `mxcli check` catches all three (MDL-ODATA05).

## Also Publishing as GraphQL

`SupportsGraphQL: Yes` makes the same service answer GraphQL as well as OData.
One boolean; the OData surface is untouched.

```sql
create odata service Fin.ChartApi (
  path: 'odata/charts/', ServiceName: 'ChartApi', Namespace: 'Fin.Charts',
  version: '1.0.0', ODataVersion: OData4,
  SupportsGraphQL: Yes
) { ... }
```

**The GraphQL endpoint is the service location itself** — there is no `/graphql`
path. Clients `POST` a query to the same URL that serves OData:

```
GET  /odata/charts/$metadata     -> the OData contract
POST /odata/charts/              -> {"query":"{ monthCategories { period } }"}
```

Verified against a running Mendix 11.13 app:

| request | response |
|---|---|
| `POST` `{ __schema { queryType { name } } }` | `{"data":{"__schema":{"queryType":{"name":"Query"}}}}` |
| `POST` `{ monthCategories { period category total } }` | `{"data":{"monthCategories":[]}}` |

Three things that only bite once GraphQL is on:

- **Query field names are camelCased.** `Period` in the model is `period` in a
  query; asking for `Total` returns 400
  `{"errors":[{"message":"Field 'Total' not found"}]}`. The OData names are
  unchanged, so the two surfaces spell the same attribute differently.
- **Exposed names must be unique beyond case (CE2881).** Publishing an entity
  without `as '...'` gives the entity type and the entity set the same name,
  which OData accepts and GraphQL rejects. A service that built yesterday can
  fail on the day it is enabled. Give the set its own name:
  `publish entity Fin.VMonthCategory as 'MonthCategories'`.
- **`PublishAssociations` must be Yes.** GraphQL has no representation for an
  associated object id, so Mendix refuses the pair: CE8055 "A service that
  supports GraphQL must publish associations as a link." mxcli refuses it before
  writing, since no other change can make it build.
- **Mendix 10.14+**, where it arrived as an experimental feature. mxcli refuses
  the statement on an older project rather than writing a property that version's
  metamodel does not have — an unknown property is not a build error, it is a
  document Studio Pro will not open.

### What GraphQL actually covers, measured

The GraphQL surface is narrower than the OData one, and the gaps are not
documented next to the checkbox. Introspected and exercised on 11.13, on a
resource published over both at once:

| OData | GraphQL |
|---|---|
| `$select` | **inherent** — you name the fields, that *is* the projection |
| `$top` | `first: Int` |
| `$skip` | `offset: Int` |
| `$orderby` | `orderBy: [{field: ASC\|DESC}]` |
| key lookup `Set(K='v')` | a singular field: `vMonthCategory(period: "…", category: "…")` |
| **`$filter`** | **absent** |
| **`$count`** | **absent** |
| `$expand` | not measured here (the probe has no associations) |

The whole schema for a one-entity service is nine types — `Query`,
`SortOrder`, the entity, its order input, and the scalars. There is no filter
type, no where type and no count type in it.

Two traps, both measured:

- **`orderBy` must be a LIST.** `orderBy: {total: DESC}` fails with
  `Incorrect value for orderBy`, while `orderBy: [{total: DESC}]` works — and
  introspection advertises the argument as a bare input object
  (`VMonthCategoryOrderInput`), not a list, so the schema and the parser
  disagree. The error does not mention it.
- **An unknown argument is silently ignored.** `monthCategories(where: {…})`
  and even `monthCategories(bogusArgument: 42)` both return **200 with the
  full result set** rather than an error. A client that assumes a filter
  argument exists gets every row and no warning — the same "200 with the wrong
  rows" failure the pushdown pack was written about, in a different surface.

So: **paging and sorting are safe over GraphQL; filtering is not there.** A
widget that needs server-side filtering has to use the OData surface, and a
resource where the client filters is a reason to keep OData even when GraphQL
is enabled.

GraphQL here is not as complete as the OData surface — it is a second way to read
the same published resources, which some widgets and clients prefer.

## HTTP Status Codes and Errors: What Each Capability Can Do

**The read path and the write path have different powers, and the difference is
the single most expensive thing to get wrong here.** Read this before designing
any microflow-backed resource.

| Capability | Can set the HTTP status code? | How |
|---|---|---|
| OData **action** (published microflow) | **Yes** | add a `System.HttpResponse` parameter |
| Entity **Insertable / Updatable / Deletable** microflow | **Yes** | add a `System.HttpResponse` parameter |
| Entity **Readable** microflow | **No** | not offered — the read capability has no documented `HttpResponse` parameter |

Sources: [published-odata-microflow §4](https://docs.mendix.com/refguide/published-odata-microflow/#4-customizing-the-outgoing-http-response),
[published-odata-entity, custom HTTP response](https://docs.mendix.com/refguide/published-odata-entity/#custom-http-response).
The custom-response section names Insertable, Updatable and Deletable; the
Readable section never references it.

### Writing a status code (action / insert / update / delete)

```sql
create microflow Api.InsertRow (
  $Row: Api.Row,
  $HttpResponse: System.HttpResponse
)
begin
  if $Row/RowKey = empty then
    change $HttpResponse (StatusCode = 400, Content = '{"error":"rowKey is required"}');
    return;
  end if;
  ...
end;
```

Three rules the platform imposes:

- **`ReasonPhrase` is ignored.** Setting it is dead code; put the explanation in
  `Content`.
- **`204` always produces an empty body.** Setting `Content` alongside it is
  discarded.
- **Changing status or content makes the whole response come from
  `HttpResponse`** — headers included. Changing *only* headers merges them with
  the defaults instead.
- `Transfer-Encoding` and `Date` cannot be changed.

### The read path cannot refuse, so it must not over-promise

A read microflow has no way to answer `400`. Its only exits are to throw (a
blunt `500`) or to return data. That has two consequences, and both are design
obligations rather than nice-to-haves:

**1. Declare capabilities you do not implement as `No`.** Mendix applies *no*
query options to a read-microflow resource — it hands over the request and
returns whatever comes back — so `TopSupported` / `SkipSupported` / `Countable`
are claims about your microflow, not about the platform. A resource that
advertises `TopSupported: Yes` and ignores `$top` returns the entire collection
with a `200`, and the client believes it received a page.

```sql
publish entity Api.Row as 'Rows' (
  ReadMode: microflow Api.Read_Rows,
  -- Only claim what Read_Rows actually parses out of the URI:
  TopSupported: No,
  SkipSupported: No,
  Countable: No
)
```

Declaring `No` is the read path's substitute for the `400` it cannot send. It is
also safe to declare: mxcli reads these capabilities off the contract, so a
consuming app generated from this service says `No` too. (Until it learned to,
the generator always said `Yes`, and the consumer failed to build with **CE6630**
"'Rows' is marked supports $top=False in the OData service, but True in the app"
— if you hit that against an older mxcli, that is the cause.)

**The capabilities vocabulary has two annotation shapes**, and it is worth knowing
which you are looking at when reading a `$metadata` by hand:

| Capability | Shape |
|---|---|
| `TopSupported`, `SkipSupported` | standalone: `<Annotation Bool="false" Term="…TopSupported"/>` |
| `Insertable`, `Updatable`, `Deletable`, `Countable` | a `<Record>` with a `Bool` property value |
| `Filterable`, `Sortable` | **either** — a record listing `NonFilterableProperties` when *some* attributes are filterable, or a bare `Bool="false"` on the record when *none* are |

That last row is the trap. Mendix picks the shape by arithmetic (there is no list
to write when nothing is filterable), so both appear in one document on different
entity sets, and a service where an entity exposes only a KEY emits the whole-set
form.

`mxcli check` enforces this as **MDL-ODATA03**, and it looks for the option
*names* in the microflow body, not just for a `System.HttpRequest` parameter —
adding the parameter to answer the KEY (below) does not silence the paging
warning. A microflow that hands the request to a Java action, a JavaScript
action, or a microflow outside the script is left alone, since mxcli cannot read
what those do.

**2. Answer a lookup by your own KEY.** A client holding a row re-reads it by
key, unprompted, and Mendix's own OData client sends the `$filter` spelling:

```
?$filter=rowKey eq '1036-c'     ← what the runtime actually sends
/Rows('1036-c')                 ← bare path key
/Rows(rowKey='1036-c')          ← named path key
```

If the microflow parses only its collection filter, the key request falls through
to the collection default and the client adopts the **first row** as the identity
of the object it is displaying. There is no error: the request is well-formed,
the response is a valid collection, the count is right, the status is `200`. Two
different objects are then on screen at once, and nothing distinguishes them
until one travels to another page.

So: `expose ( … (KEY) )` is a promise the *service* makes on the *microflow's*
behalf. Branch key → id → filter → default.

**Not declaring the KEY is not a way out.** Mendix requires a published entity to
have one — `CE6585 "Published entity 'X' must have a key defined."` — so the only
correct resolution is to answer the lookup. (Query *options* you may decline;
the key you may not.)

The request itself always arrives on `System.HttpRequest`:

```sql
create microflow Api.Read_Rows (
  $Request:  System.HttpRequest,
  $Response: System.ODataResponse    -- required while Countable is Yes
)
returns List of Api.Row
begin
  log info 'URI=' + $Request/Uri;    -- the whole query string, URL-encoded
  ...
end;
```

To watch what clients actually send, raise the **`OData Publish`** log node (note
the space; it exists only when the project publishes a service):

```bash
mxcli log set "OData Publish" TRACE
```
```
TRACE - OData Publish: Incoming request from 127.0.0.1: GET .../Rows?$top=5&$filter=rowKey eq 'abc'
DEBUG - OData Publish: Responding to client with status code 400.
```

That is the fastest way to see a client re-reading a row by key, and it needs no
change to the model. `ODataConsume` is the client side, a different node.

Mendix validates field names before the microflow runs (`$filter=secretColumn eq 'x'`
is a `400` from the platform), and it enforces `Filterable`: filtering on a property
you did not declare filterable is rejected with `400 "Property 'x' is
non-filterable"` before the microflow runs. So the microflow only ever sees names
that exist in the published metadata. That is defence in depth, not a substitute for a
whitelist — it constrains the *name*, not what you do with it.

## OData Actions: Publishing a Microflow

An entity set is a *read* surface. To let a client **invoke** something —
`POST /odata/x/RecordNote` with arguments in the body — publish a microflow.
Mendix exposes it in `$metadata` as an `ActionImport`.

```sql
create odata service ProductApi.Actions ( ... )
authentication basic
{
  publish microflow ProductApi.RecordNote as 'RecordNote'
    expose ( Note as 'note', Amount as 'amount' (CanBeEmpty) );
};
```

**Parameter data types and the return type are not written in MDL.** They are
read off the microflow, which already declares them — the same thing Studio Pro
does, and the only arrangement in which the two cannot drift. Omitting the
`expose` clause publishes every parameter under its own name.

### Why this matters more than it looks

Mendix validates `$filter` against the published metadata **before** the read
microflow runs. So a parameterised *entity set* cannot take its arguments as
filters: `?$filter=driverId eq 'x'` against a resource whose entity has no
`driverId` attribute is answered

```
400  Could not map 'driverId' to attribute or association.
```

and the microflow never sees it. The workaround is to carry every parameter as
an attribute and echo it back on each row — `SELECT f.*, {driverId} AS driver_id
…`. An action takes them as parameters and needs none of that.

### Two things Mendix will not do for you

- **A returning stored procedure cannot be called directly.** The External
  Database Connector dispatches a `CALL` as an update, and PgJDBC refuses
  because a `CALL` with INOUT parameters answers with a row: *"A result was
  returned when none was expected"*. There is no `execute database statement`
  activity — only `execute database query`, which wants a `SELECT`. Wrap the
  procedure in a one-line function that `CALL`s it and returns its row.
- **The JDBC driver must be declared and shipped even for PostgreSQL**, the
  database Mendix itself runs on. Without a module jar dependency the build
  fails CE5278; declared but `included = false`, the build is green and the
  first request dies with *"No JDBC driver found in app for URL"* — the
  Connector resolves drivers from the app's classpath, not the runtime's. Use
  `included = true` and `mxcli sync-java-deps`.

## Authentication Methods, and the Cost of Basic Auth

A published service names one or more methods in the `authentication` clause:

```sql
authentication basic, session
authentication microflow ProductApi.Authenticate
```

| Method | How the caller proves itself | Cost per request |
|---|---|---|
| `basic` | `Authorization: Basic …` on every request | **a full password hash** |
| `session` | an existing session + `X-Csrf-Token` | none, but only reachable from same-origin JavaScript in the same app |
| `guest` | anonymous | none |
| `microflow` | your microflow decides | whatever the microflow does |

**Basic auth hashes on every call, including failures.** A consumed OData
service holds no session, so every request is a fresh login. Measured on a real
app, that was 60–80% of a page turn — and a *wrong* password costs the same,
because the hash runs either way:

```
GET /odata/f1/Drivers?$top=20
  basic auth                 200  ~360 ms
  wrong password             401  ~346 ms   <- hashes either way
  session cookie             200   ~19 ms
  no credentials             401   ~42 ms
```

Do not read the 19 ms as the fix: `session` is only open to same-origin
JavaScript, and a Mendix OData *client* sends basic auth and keeps no cookie.

### Custom authentication

A microflow taking `List of System.HttpHeader` and returning a `System.User`.
Return empty to deny. No password hash anywhere, so the ~42 ms floor.

```sql
CREATE MICROFLOW ProductApi.Authenticate ($Headers: List of System.HttpHeader)
  RETURNS System.User AS $User
BEGIN
  -- e.g. compare a shared secret from $Headers, then retrieve the service account
  retrieve $Users from System.User;
  $User = head($Users);
  RETURN $User;
END;

create odata service ProductApi.Api ( ... )
authentication microflow ProductApi.Authenticate
{ ... };
```

**The client needs no change.** The microflow can read the `Authorization:
Basic …` header itself and compare it against a constant, so the consumer keeps
its existing username/password configuration and the server simply stops
calling BCrypt.

Two build rules to know before you reach for it:

- **The microflow is mandatory.** `authentication microflow` with no name parses
  but fails the build with **CE0333** "Please select a microflow to use for
  authentication". `mxcli check` flags this as `MDL-ODATA04`.
- **App security must be on.** With security off, Mendix reports **CE6600**
  "App security is off, but custom authentication is enabled for this service".
  Set it with `alter project security level prototype` (or `production`).

If custom authentication is more than you need, `ALTER SETTINGS MODEL
BcryptCost = 8` shrinks the hash instead of removing it. Each step down halves
the cost; the default is 12. That is a judgement about the accounts in *that*
app — appropriate for machine accounts with generated passwords, not for a
database of human passwords.

## Step-by-Step: Read-Write API with Microflow Handlers

For write operations (insert, update, delete), the OData service delegates to microflows that map between the view entity and the underlying persistent entities.

### Step 1: Create CUD Microflows on the Producer

Each microflow receives the view entity and an `$HttpRequest` parameter:

```sql
/**
 * Handles INSERT on ProductWithPriceVE.
 * Creates a new Product and initial Price entry.
 */
create microflow ProductApi.InsertProductWithPriceVE (
  $ProductWithPriceVE: ProductApi.ProductWithPriceVE,
  $HttpRequest: System.HttpRequest
)
begin
  -- Map view fields to persistent entities
  $Product = create Shop.Product (
    Name = $ProductWithPriceVE/Name,
    description = $ProductWithPriceVE/description,
    IsActive = true
  );
  commit $Product;

  $Price = create Shop.Price (
    PriceInEuro = $ProductWithPriceVE/PriceInEuro,
    StartDate = '[%CurrentDateTime%]'
  );
  change $Price (Shop.Price_Product = $Product);
  commit $Price;
end;

grant execute on microflow ProductApi.InsertProductWithPriceVE
  to ProductApi.ApiUser;

/**
 * Handles UPDATE on ProductWithPriceVE.
 * Updates the Product name/description and creates a new Price entry.
 */
create microflow ProductApi.UpdateProductWithPriceVE (
  $ProductWithPriceVE: ProductApi.ProductWithPriceVE,
  $HttpRequest: System.HttpRequest
)
begin
  retrieve $Product from Shop.Product
    where ProductId = $ProductWithPriceVE/ProductId
    limit 1;

  change $Product (
    Name = $ProductWithPriceVE/Name,
    description = $ProductWithPriceVE/description
  );
  commit $Product;
end;

grant execute on microflow ProductApi.UpdateProductWithPriceVE
  to ProductApi.ApiUser;

/**
 * Handles DELETE on ProductWithPriceVE.
 * Soft-deletes the product by setting IsActive = false.
 */
create microflow ProductApi.DeleteProductWithPriceVE (
  $ProductWithPriceVE: ProductApi.ProductWithPriceVE,
  $HttpRequest: System.HttpRequest
)
begin
  retrieve $Product from Shop.Product
    where ProductId = $ProductWithPriceVE/ProductId
    limit 1;

  change $Product (IsActive = false);
  commit $Product;
end;

grant execute on microflow ProductApi.DeleteProductWithPriceVE
  to ProductApi.ApiUser;
```

### Step 2: Wire Microflows to Published Entity

Set `InsertMode`, `UpdateMode`, `DeleteMode` to `CallMicroflow`:

```sql
  publish entity ProductApi.ProductWithPriceVE as 'Product' (
    ReadMode: ReadFromDatabase,
    InsertMode: microflow ProductApi.InsertProductWithPriceVE,
    UpdateMode: microflow ProductApi.UpdateProductWithPriceVE,
    DeleteMode: microflow ProductApi.DeleteProductWithPriceVE
  )
  expose (...);
```

### Step 3: Grant Write Access on External Entity

On the consumer side, grant CREATE, WRITE, and DELETE rights:

```sql
grant ProductClient.User on ProductClient.ProductsEE
  (create, delete, read *, write *);
```

The consumer can now create, update, and delete products through the OData API, and the producer's microflows handle the mapping to persistent entities.

## Advanced: Configuration Microflow for Custom Headers

When the consumer needs to pass custom headers (e.g., for audit trails or user context), use a configuration microflow:

```sql
/**
 * Adds current user name as custom header for audit logging.
 */
create microflow ProductClient.SetClientHeaders (
  $httpResponse: System.HttpResponse
)
returns list of System.HttpHeader as $HttpHeaderList
begin
  $HttpHeaderList = create list of System.HttpHeader;
  $NewHttpHeader = create System.HttpHeader (
    key = 'X-Audit-User',
    value = $currentUser/Name
  );
  add $NewHttpHeader to $HttpHeaderList;
  return $HttpHeaderList;
end;
```

Reference it in the client:

```sql
create odata client ProductClient.ProductDataApiClient (
  ...
  ConfigurationMicroflow: microflow ProductClient.SetClientHeaders
);
```

## API Versioning

When your API contract changes, create a new version rather than breaking existing consumers:

```sql
-- v1: Original API (keep running for existing consumers)
create odata service ProductApi.ProductDataApi (
  path: 'odata/productdataapi/v1/',
  version: '1.0.0',
  ...
);

-- v2: New version with additional fields
create odata service ProductApi.ProductDataApi_v2 (
  path: 'odata/productdataapi/v2/',
  version: '2.0.0',
  ODataVersion: OData4,
  ServiceName: 'ProductDataApi',
  Summary: 'Product API v2 - includes weight and tags',
  ...
)
authentication basic
{
  publish entity ProductApi.ProductWithPriceAndTagsVE as 'Product' (
    ReadMode: ReadFromDatabase,
    InsertMode: microflow ProductApi.InsertProductV2,
    UpdateMode: microflow ProductApi.UpdateProductV2,
    DeleteMode: microflow ProductApi.DeleteProductV2
  )
  expose (...);
};
```

## Folder Organization

Use the `Folder` property to organize OData documents within modules.

**MetadataUrl accepts three formats:**
1. **HTTP(S) URL** — fetches from remote service (production)
2. **file:///absolute/path** — reads from local absolute path
3. **./path or path/file.xml** — reads from local relative path (resolved against .mpr directory)

```sql
-- Format 1: HTTP(S) URL
create odata client ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'https://api.example.com/odata/v4/$metadata',
  Folder: 'Integration/ProductAPI'
);

-- Format 2: Absolute file:// URI
create odata client ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'file:///Users/team/contracts/productdataapi.xml',
  Folder: 'Integration/ProductAPI'
);

-- Format 3a: Relative path with ./
create odata client ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: './metadata/productdataapi.xml',
  Folder: 'Integration/ProductAPI'
);

-- Format 3b: Relative path without ./
create odata client ProductClient.ProductDataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'metadata/productdataapi.xml',
  Folder: 'Integration/ProductAPI'
);

create odata service ProductApi.ProductDataApi (
  path: 'odata/productdataapi/v1/',
  version: '1.0.0',
  ODataVersion: OData4,
  folder: 'Integration/APIs'
)
authentication basic
{ ... };
```

Folders are created automatically if they don't exist. Use `/` for nested folders.

## Module Organization Conventions

Follow this naming convention for clean separation:

| Module | Purpose | Contains |
|--------|---------|----------|
| `Shop` | Core domain | Persistent entities, business logic |
| `ShopApi` or `ShopViews` | API layer (producer) | View entities, OData service, CUD microflows |
| `ShopClient` or `ShopViewsClient` | API consumer | OData client, external entities, client constants |

This keeps the API contract separate from the domain logic, and the consumer separate from the producer.

## Checklist

Before publishing:
- [ ] View entities expose only the fields consumers need (no internal IDs unless needed for writes)
- [ ] View entity has at least one `key` field for OData identity
- [ ] Module role created and granted on view entities (READ, optionally WRITE)
- [ ] OData service has AUTHENTICATION set (Basic, Session, or Microflow)
- [ ] GRANT ACCESS ON ODATA SERVICE to the API module role
- [ ] CUD microflows (if writable) accept `($ViewEntity, $HttpRequest)` parameters
- [ ] CUD microflows granted EXECUTE to the API module role

Before consuming:
- [ ] Location constant created for environment-specific URLs
- [ ] OData client `MetadataUrl` points to either:
  - HTTP(S) URL: `https://api.example.com/$metadata`
  - Local file (absolute): `file:///path/to/metadata.xml`
  - Local file (relative): `./metadata/service.xml` (resolved against `.mpr` directory)
- [ ] OData client uses `ServiceUrl: '@Module.Constant'` for runtime endpoint
- [ ] External entities match the published exposed names and types
- [ ] Module role created and granted on external entities (READ, optionally CREATE/WRITE/DELETE)

## Exploration Commands

Use these commands to inspect existing OData setup in a project:

```sql
-- List all published and consumed services
show odata services;
show odata clients;

-- Inspect a specific service
describe odata service ShopViews.ShopViewsApi;
describe odata client ShopViewsClient.ShopViewsApiClient;

-- See external entities and view entities
show entities in ShopViewsClient;
show external entities;
show external actions;

-- Browse available assets from cached $metadata contract
show contract entities from ShopViewsClient.ShopViewsApiClient;
show contract actions from ShopViewsClient.ShopViewsApiClient;
describe contract entity ShopViewsClient.ShopViewsApiClient.Product;
describe contract entity ShopViewsClient.ShopViewsApiClient.Product format mdl;

-- Check security setup
show access on odata service ShopViews.ShopViewsApi;
show module roles in ShopViews;
```
