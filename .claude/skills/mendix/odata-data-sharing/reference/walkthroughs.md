# Step-by-step build walkthroughs

Supporting reference for [odata-data-sharing](../SKILL.md).

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
