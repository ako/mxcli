---
name: json-structures-and-mappings
description: "Create and manage JSON structures, import mappings and export mappings in MDL, plus the domain-model shapes they map onto. Use when turning a JSON payload into entities, mapping a REST or queue response, or exporting objects as JSON."
---

# JSON Structures, Import Mappings & Export Mappings

This skill covers creating and managing JSON structures, import mappings, and export mappings in Mendix using MDL.

## Key Concepts

### JSON Structures
A JSON structure defines the schema of a JSON payload. It stores a JSON snippet and auto-derives an element tree with paths, types, and custom names.

### Import Mappings
An import mapping converts a JSON string into Mendix entity objects. It maps JSON fields to entity attributes.

#### Two names per member: the raw key and the exposed name

Every JSON structure element stores **both**, and for any lowercase-initial key
they differ:

| | Example | Used for |
|---|---|---|
| **Path** (raw JSON key) | `(Object)\|uuid` | what the **runtime** resolves by |
| **ExposedName** (derived) | `Uuid` | what **Studio Pro displays** |

Mendix derives the exposed name by capitalising the initial, and for an array's
item object by suffixing `Item` — so `total` → `Total`, `camelCase` → `CamelCase`,
`__Value` (array) → `__ValueItem` (its item). Keys already starting with an
underscore are left alone: `__returnedCount` stays `__returnedCount`.

This is **Mendix's own convention, not something mxcli does**. A blank app's
Studio-Pro-authored `FeedbackModule.JSON_AppInsightsResponse` stores
`ExposedName: "Uuid"` against `Path: "(Object)|uuid"`, and its `IMM_PostResponse`
binds `JsonPath: "(Object)|uuid"`.

Consequences worth knowing:

- **Either spelling works in MDL.** `Total = total` and `Total = Total` produce the
  same stored mapping. Write whichever you have.
- **`DESCRIBE` emits the raw JSON key**, so its output reproduces the script that
  produced the mapping — `Total = total` comes back as `Total = total`, and an
  array binding as `= item` rather than `= ItemItem`. It also emits
  `create or modify`, so the output re-runs against the project it was read from.
  (Until #915 it printed the exposed name and a bare `create`: the text differed
  from the input, making every script-vs-describe diff noise, and re-running it
  failed with "import mapping already exists". The stored mapping was correct
  either way.)
- **A member matching neither spelling is refused**, listing what would have
  worked. It is never written with a guessed path: such a mapping passed
  `mxcli check` and failed later in mxbuild (CE5015) or at runtime.

#### Inherited attributes

Mendix inheritance is multi-table: all of a parent's attributes are members of the
child, so an entity created with `extends` can map them. Name an inherited
attribute exactly like one of the entity's own — mxcli resolves each to the entity
that **declares** it, which is the reference Studio Pro needs to show the field
mapped.

```sql
create persistent entity Docs.DocumentBase (
  DocName: String(200),
  Confidential: Boolean
);

create persistent entity Docs.Contract extends Docs.DocumentBase (
  ContractNumber: String(50)
);

create import mapping Docs.IMM_Contract
  with json structure Docs.JSON_Contract
{
  create Docs.Contract {
    ContractNumber = contractNumber,   -- own
    DocName        = docName,          -- inherited
    Confidential   = confidential      -- inherited
  }
};
```

Qualifying an inherited attribute against the entity being mapped instead of its
declaring entity is Mendix **CE1613** "The selected attribute ... no longer
exists", and the field shows unmapped in Studio Pro.

## Export Mappings
An export mapping converts Mendix entity objects into a JSON string. It maps entity attributes to JSON fields.

### Critical: Import and Export Need Different Domain Models

**Import and export mappings for the same JSON structure typically require different entity structures.**

- **Import**: The child entity owns the FK to the parent (`from Child to Parent`). Arrays map directly to the item entity — no intermediate container entity needed.
- **Export**: The domain model mirrors the JSON structure. Arrays need an intermediate container entity (e.g., `Items`) plus an item entity (e.g., `ItemsItem`). The container links to the parent, the item links to the container.

---

## JSON Structures

### Create

```sql
create json structure Module.JSON_Pet
  snippet '{"id": 1, "name": "Fido", "status": "available"}';
```

For multi-line JSON, use dollar-quoting:
```sql
create json structure Module.JSON_Order
  snippet $${
  "orderId": 100,
  "customer": {"name": "Alice", "email": "alice@example.com"},
  "items": [{"sku": "A1", "quantity": 2, "price": 9.99}]
}$$;
```

Custom name mapping (rename JSON fields):
```sql
create json structure Module.JSON_Pet
  snippet '{"id": 1, "name": "Fido"}'
  CUSTOM NAME map ('id' as '_id');
```

### Browse

```sql
show json structures;
show json structures in module;
describe json structure Module.JSON_Pet;
drop json structure Module.JSON_Pet;
```

---

## Import Mappings

### Domain Model for Import

For import mappings, associations point FROM the child entity TO the parent:

```sql
create non-persistent entity Module.OrderResponse (
  OrderId: integer
);
/

create non-persistent entity Module.CustomerInfo (
  Name: string,
  Email: string
);
/

create non-persistent entity Module.OrderItem (
  Sku: string,
  Quantity: integer,
  Price: decimal
);
/

-- Child entity owns the FK (FROM child TO parent)
create association Module.CustomerInfo_OrderResponse
  from Module.CustomerInfo
  to Module.OrderResponse;
/

create association Module.OrderItem_OrderResponse
  from Module.OrderItem
  to Module.OrderResponse;
/
```

### Simple Import Mapping (flat JSON)

```sql
create import mapping Module.IMM_Pet
  with json structure Module.JSON_Pet
{
  create Module.PetResponse {
    PetId = id,
    Name = name,
    status = status
  }
};
```

### Nested Import Mapping (objects and arrays)

Arrays map directly to the item entity — no intermediate container needed:

```sql
create import mapping Module.IMM_Order
  with json structure Module.JSON_Order
{
  create Module.OrderResponse {
    OrderId = orderId,
    create Module.CustomerInfo_OrderResponse/Module.CustomerInfo = customer {
      Name = name,
      Email = email
    },
    create Module.OrderItem_OrderResponse/Module.OrderItem = items {
      Sku = sku,
      Quantity = quantity,
      Price = price
    }
  }
};
```

### Object Handling

| Syntax | Meaning |
|--------|---------|
| `create Module.Entity` | Always create a new object (default) |
| `find Module.Entity` | Find by KEY attributes, ignore if not found |
| `find or create Module.Entity` | Find by KEY, create if not found |

```sql
create import mapping Module.IMM_UpsertPet
  with json structure Module.JSON_Pet
{
  find or create Module.PetResponse {
    PetId = id key,
    Name = name,
    status = status
  }
};
```

**Note**: `key` is only valid with `find` or `find or create`, not with `create`.

---

## Export Mappings

### Domain Model for Export

Export mappings require entities that **mirror the JSON structure**. Arrays need an intermediate container entity:

```sql
-- Root entity (matches top-level JSON object)
create non-persistent entity Module.ExRoot (
  OrderId: integer
);
/

-- Nested object entity (1-1 relationship, use OWNER Both)
create non-persistent entity Module.ExCustomer (
  Name: string,
  Email: string
);
/

-- Array CONTAINER entity (no attributes, just links parent to items)
create non-persistent entity Module.ExItems;
/

-- Array ITEM entity (attributes for each array element)
create non-persistent entity Module.ExItemsItem (
  Sku: string,
  Quantity: integer,
  Price: decimal
);
/

-- Associations: child FROM, parent TO
create association Module.ExCustomer_ExRoot
  from Module.ExCustomer
  to Module.ExRoot
  owner both;   -- 1-1 for nested objects
/

create association Module.ExItems_ExRoot
  from Module.ExItems
  to Module.ExRoot;   -- 1-* for arrays
/

create association Module.ExItemsItem_ExItems
  from Module.ExItemsItem
  to Module.ExItems;   -- 1-* for array items
/
```

### Simple Export Mapping (flat JSON)

```sql
create export mapping Module.EMM_Pet
  with json structure Module.JSON_Pet
{
  Module.PetResponse {
    id = PetId,
    name = Name,
    status = status
  }
};
```

### Nested Export Mapping (objects and arrays)

Arrays have TWO levels: container entity + item entity:

```sql
create export mapping Module.EMM_Order
  with json structure Module.JSON_Order
{
  Module.ExRoot {
    orderId = OrderId,
    Module.ExCustomer_ExRoot/Module.ExCustomer as customer {
      name = Name,
      email = Email
    },
    Module.ExItems_ExRoot/Module.ExItems as items {
      Module.ExItemsItem_ExItems/Module.ExItemsItem as ItemsItem {
        sku = Sku,
        quantity = Quantity,
        price = Price
      }
    }
  }
};
```

### NULL VALUES option

```sql
create export mapping Module.EMM_Pet
  with json structure Module.JSON_Pet
  null values SendAsNil     -- or LeaveOutElement (default)
{
  ...
};
```

---

## Starting a Mapping Below the Payload Root

A mapping does not have to start at the top of the JSON. `root a/b/c` on the
source clause selects the element it starts at — the same choice Studio Pro
offers when you pick a node deeper in the payload. Useful when the interesting
object is buried under an envelope you do not want entities for.

The path is written in **member names**, and it may pass **through arrays**: the
mapping is then rooted at the array's item. (A value reference cannot do that —
many items cannot collapse into one value, and mxbuild reports CE0256.)

```sql
create json structure RootDemo.JSON_Completion
  snippet $${
    "requestId": "r-1",
    "response": {
      "model": "gpt-x",
      "choices": [
        {
          "index": 0,
          "message": {
            "role": "assistant",
            "content": "hello",
            "citations": [ { "title": "t", "url": "u" } ]
          }
        }
      ]
    }
  }$$;
```

**Through an array, to an object several levels down.** Everything inside the
statement is relative to the selected root, associations included:

```sql
create import mapping RootDemo.IMM_Answer
  with json structure RootDemo.JSON_Completion root response/choices/message
{
  create RootDemo.Answer {
    Role = role,
    Content = content,
    create RootDemo.Citation_Answer/RootDemo.Citation = citations {
      Title = title,
      Url = url
    }
  }
};
```

stored as one root element at `(Object)|response|choices|(Object)|message`, with
`citations` nesting under it as usual.

**Landing on an array.** A root that ends on an array roots the mapping at its
**item**, so `Index` below is a member of one choice, not of the list:

```sql
create import mapping RootDemo.IMM_Choice
  with json structure RootDemo.JSON_Completion root response/choices
{
  create RootDemo.Choice {
    Index = index
  }
};
```

stored at `(Object)|response|choices|(Object)`.

**Export takes the same clause**, and produces the envelope down to the selected
element:

```sql
create export mapping RootDemo.EXM_Answer
  with json structure RootDemo.JSON_Completion root response/choices/message
{
  RootDemo.Answer {
    role = Role,
    content = Content,
    RootDemo.Citation_Answer/RootDemo.Citation as citations {
      title = Title,
      url = Url
    }
  }
};
```

### Notes

- **Omit the clause** and the mapping starts at the structure's own root — an
  array-rooted structure included, which needs no syntax of its own.
- **DESCRIBE emits the clause** for any mapping stored below the root, including
  ones authored in Studio Pro, so `describe` → `exec` round-trips. Re-running a
  described mapping reports `Unchanged import mapping …`: the rebuild is
  semantically equal and the write is elided.
- **A path that does not resolve is refused**, and the error names what would
  have worked:

  ```
  Error: import mapping RootDemo.IMM_Bad: root "response/choise": "choise" is not
  a member of the schema at (Object)|response; available: model (or Model),
  choices (or Choices)
  ```

- The selected root does **not** have to be an object. Any element the structure
  contains can be picked; a value root would leave nothing to map, so in practice
  it is an object or an array.

## Microflow Actions

### Import from Mapping (JSON → entities)

```sql
-- With result variable (non-persistent entities)
$PetResponse = import from mapping Module.IMM_Pet($JsonContent);

-- Without result variable (persistent entities, stores to DB)
import from mapping Module.IMM_Pet($JsonContent);
```

#### Range — how much of the result to bind

Optional trailing clause, matching Studio Pro's **All / First / Custom** setting
on the activity. Omit it and mxcli infers from the mapping's own root shape, as
it always has.

```sql
$Pets = import from mapping Module.IMM_Pets($Json) all;            -- All (the default)
$Pet  = import from mapping Module.IMM_Pets($Json) first;          -- First: ONE object
$Page = import from mapping Module.IMM_Pets($Json) limit 10;       -- Custom
$Page = import from mapping Module.IMM_Pets($Json) limit 10 offset 5;
```

`first` is a separate word from `limit 1` on purpose: `limit 1` is a *list* of
one, `first` binds a single *object*, so the result variable's type differs.

Two things the range does **not** do:

- **It does not change what the mapping returns.** An object-rooted mapping
  binds an object under every range — `all` on one is Studio Pro's own default,
  and the blank app ships one (`FeedbackModule.SUB_Feedback_PostToAppInsights`).
  Only `first` narrows a list mapping to a single object.
- **`offset` is not accepted everywhere.** Mendix rejects it with
  **CE6100** ("This entity does not support offset") unless the mapping's root
  is a list; `limit` alone is fine either way. Verified on mxbuild 11.6.6.

### Export to Mapping (entity → JSON)

```sql
$JsonOutput = export to mapping Module.EMM_Pet($PetResponse);
```

### Complete Pipeline

```sql
create microflow Module.ProcessData ()
begin
  declare $json string = $latestHttpResponse/content;
  $PetResponse = import from mapping Module.IMM_Pet($json);
  -- Process...
  $Output = export to mapping Module.EMM_Pet($PetResponse);
  log info node 'Integration' 'Result: ' + $Output;
end;
/
```

---

## Browse

```sql
show import mappings [in module];
show export mappings [in module];
describe import mapping Module.Name;
describe export mapping Module.Name;
drop import mapping Module.Name;
drop export mapping Module.Name;
```

---

## Export Workflow: PE → NPE → JSON

Export mappings work on non-persistent entity (NPE) structures that mirror the target JSON. When the source data is in persistent entities (PE) in the database, the typical workflow is:

1. **Retrieve** persistent data from the database
2. **Build NPE tree** in a microflow: create NPE objects, set attributes, link via associations to match the JSON structure
3. **Export to mapping** to serialize the NPE tree to JSON

```sql
-- Example: build NPE tree from persistent Order data, then export
create microflow Module.ExportOrder ($Order: Module.Order)
returns string as $json
begin
  -- Build the NPE tree matching the JSON structure
  $Root = create Module.ExRoot (OrderId = $Order/OrderId);

  retrieve $Customer from $Order/Module.Order_Customer;
  $ExCust = create Module.ExCustomer (Name = $Customer/Name, Email = $Customer/Email);
  -- Link customer to root...

  -- Export
  $json = export to mapping Module.EMM_Order($Root);
  return $json;
end;
/
```

### Shortcut with View Entities

View Entities (OQL-backed) can retrieve data directly into the export-ready structure, skipping the manual NPE assembly:

```sql
create view entity Module.ExOrderView (
  OrderId: integer,
  CustomerName: string,
  CustomerEmail: string
) as select o.OrderId, c.Name, c.Email
   from Module.Order o
   join Module.Order_Customer/Module.Customer c;
```

This can reduce the microflow to a single retrieve + export step.

---

## Realistic Example: Countries REST API

A complete example consuming a Countries REST API, importing the response, and
exporting country data back to JSON.

### Step 1: JSON Structures

```sql
-- Single country (flat object)
create json structure Integration.JSON_Country
  snippet '{"name": "Netherlands", "officialName": "Kingdom of the Netherlands", "capital": "Amsterdam", "region": "Europe", "population": 18100436, "flagUrl": "https://flagcdn.com/w320/nl.png"}';

-- List of countries (array of objects)
create json structure Integration.JSON_CountryList
  snippet '[{"name": "Netherlands", "capital": "Amsterdam", "region": "Europe", "population": 18100436}]';
```

### Step 2: Import — Single Country

```sql
create non-persistent entity Integration.Country (
  Name: string,
  OfficialName: string,
  Capital: string,
  Region: string,
  Population: integer,
  FlagUrl: string
);
/

create import mapping Integration.IMM_Country
  with json structure Integration.JSON_Country
{
  create Integration.Country {
    Name = name,
    OfficialName = officialName,
    Capital = capital,
    Region = region,
    Population = population,
    FlagUrl = flagUrl
  }
};
```

### Step 3: Import — List of Countries

For a list response, the import mapping maps the array item directly (no container):

```sql
create non-persistent entity Integration.CountryListItem (
  Name: string,
  Capital: string,
  Region: string,
  Population: integer
);
/

create import mapping Integration.IMM_CountryList
  with json structure Integration.JSON_CountryList
{
  create Integration.CountryListItem {
    Name = name,
    Capital = capital,
    Region = region,
    Population = population
  }
};
```

### Step 4: Export — Serialize Country to JSON

For the flat country, the same entity works for both import and export:

```sql
create export mapping Integration.EMM_Country
  with json structure Integration.JSON_Country
{
  Integration.Country {
    name = Name,
    officialName = OfficialName,
    capital = Capital,
    region = Region,
    population = Population,
    flagUrl = FlagUrl
  }
};
```

### Step 5: Export — List of Countries

For exporting a list, the export domain model needs a root container + item entities:

```sql
-- Container entity wrapping the array
create non-persistent entity Integration.ExCountryList;
/

-- Item entity for each country in the array
create non-persistent entity Integration.ExCountryItem (
  Name: string,
  Capital: string,
  Region: string,
  Population: integer
);
/

create association Integration.ExCountryItem_ExCountryList
  from Integration.ExCountryItem
  to Integration.ExCountryList;
/

create export mapping Integration.EMM_CountryList
  with json structure Integration.JSON_CountryList
{
  Integration.ExCountryList {
    Integration.ExCountryItem_ExCountryList/Integration.ExCountryItem as Root {
      name = Name,
      capital = Capital,
      region = Region,
      population = Population
    }
  }
};
```

### Step 6: Microflow — Fetch, Import, Process, Export

```sql
create microflow Integration.GetCountryInfo ()
returns string as $json
begin
  -- Fetch country data from REST API
  $response = rest call get 'https://restcountries.com/v3.1/name/netherlands'
    header Accept = 'application/json'
    timeout 30
    returns string
    on error continue;

  -- Import JSON into entity
  $Country = import from mapping Integration.IMM_Country($response);

  -- Export back to our own JSON format
  $json = export to mapping Integration.EMM_Country($Country);
  log info node 'Integration' 'Country: ' + $json;

  return $json;
end;
/
```

---

## Placing Documents in Folders

Every one of these documents takes a `folder` clause on `create`, straight after
the qualified name. Missing folders in the path are created:

```mdl
create json structure Sales.JSON_Order folder 'Private/JSON structures'
  snippet '{"id": 1, "total": 9.99}';

create import mapping Sales.IMM_Order folder 'Private/Import mappings'
  with json structure Sales.JSON_Order
{
  create Sales.Order { OrderId = id, Total = total }
};
```

On `create or modify` the clause **moves** an existing document. Omitting it
leaves placement alone — it never returns a document to the module root — so
adding a folder to an existing script is safe and removing one is a no-op.
`describe` emits the clause, so a description replays into the same folder.

See `organize-project` for `move` and the full folder story.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Reusing import domain model for export | Export needs separate entities mirroring JSON structure |
| Association direction wrong | Always FROM child TO parent (child owns FK) |
| Using `owner default` for 1-1 nested objects in export | Use `owner both` for 1-1 relationships |
| Missing array container entity in export | Arrays need Container + Item entities |
| Using `key` with `create` handling | `key` only valid with `find` or `find or create` |
| Arrays in import with container entity | Import arrays map directly to item entity, no container |
