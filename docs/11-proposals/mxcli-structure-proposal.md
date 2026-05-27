---
title: Implement `mxcli structure` Command
status: partial
---

# Implement `mxcli structure` Command

## Context

mxcli is a Go CLI tool for working with Mendix projects. It reads `.mpr` files (BSON format), has an internal SQLite database with catalog tables (catalog.entities, catalog.microflows, catalog.pages, catalog.attributes, etc.), and provides a REPL and CLI interface.

### Existing commands and overlap

Several existing commands already provide pieces of what `structure` aims to deliver:

| Command | Scope | Format | Token cost |
|---------|-------|--------|------------|
| `show entities in module` | One type, one module | Markdown table | Medium |
| `describe entity Module.X` | One element, full detail | Round-trippable MDL | High |
| `show context of Module.X` | One element + dependencies | Markdown sections | Medium-high |
| `select ... from CATALOG.*` | Arbitrary SQL queries | Tabular rows | Varies |

**The gap**: None of these give you "the whole project at a glance." You'd need ~7 SHOW commands per module to approximate depth 2, and the output is markdown tables or verbose MDL — neither is token-efficient for LLM consumption.

### Catalog data availability

| Data needed | Catalog table | Available columns | Gap |
|-------------|---------------|-------------------|-----|
| Module list | `modules` | Name, IsSystemModule, AppStoreGuid | None |
| Entity names + attributes | `entities` + `attributes` | Entity: Name, AttributeCount; Attr: Name, DataType, Length | None |
| Association info | — | — | **No associations table** — only SHOW ASSOCIATIONS via reader |
| Microflow signatures | `microflows` | Name, ReturnType, ParameterCount | **No parameter types** — only count |
| Page widget types | `widgets` | WidgetType, EntityRef | Full mode only |
| Enumeration values | `enumerations` | Name, ValueCount | **No values** — only count |
| Nanoflow signatures | `nanoflows` (view) | Same as microflows | Same gap |
| Java actions | `java_actions` | Name, ReturnType, ParameterCount | **No parameter types** |
| Snippets | `snippets` | Name, QualifiedName, WidgetCount | None |
| OData clients | `odata_clients` | Name, QualifiedName, MetadataUrl, ODataVersion | None |
| OData services | `odata_services` | Name, QualifiedName, Path, EntitySetCount | None |
| Constants | — | — | **No catalog table** — reader `ListConstants()` available |
| Scheduled events | — | — | **No catalog table** — reader `ListScheduledEvents()` available |

## Goal

Add a `structure` subcommand that outputs a compact, token-efficient overview of a Mendix project. This is designed to be consumed by LLMs (Claude Code, Cline, Cursor) as a first step before they ask for details on specific elements. Think of it as a "repo-map" — just enough to understand the project shape, not the full content.

## Command Interface

```bash
# full project structure (default depth 2)
mxcli structure -p app.mpr

# module-level summary only
mxcli structure -p app.mpr --depth 1

# Specific module
mxcli structure -p app.mpr --module CRM

# Deep: include attribute types and microflow parameters
mxcli structure -p app.mpr --depth 3

# Include system/marketplace modules
mxcli structure -p app.mpr --all
```

Also available in the REPL when a project is connected:
```sql
show structure;
show structure depth 1;
show structure in CRM;
show structure depth 3;
```

## Output Format

The output is plain text, indented with 2 spaces per level, designed for minimal token usage while maximizing information density. No markdown, no tables, no decorations.

### Depth 1 — Module Summary

```
CRM           5 entities, 4 microflows, 1 nanoflow, 3 pages, 1 enum, 2 java actions
auth          1 entity, 2 microflows, 1 nanoflow, 1 page, 1 enum, 1 snippet
Reporting     1 entity, 2 microflows, 1 page, 1 odata service
```

Rules:
- One line per module
- Skip system/marketplace modules unless `--all` flag (see filtering below)
- Show counts per document type
- Sort modules alphabetically
- Only show document types that have count > 0

**Data source**: All from catalog — `select ModuleName, count(*) from entities/microflows/pages/enumerations/java_actions/snippets/odata_clients/odata_services GROUP by ModuleName`. Constants and scheduled events require reader calls (no catalog table yet).

### Depth 2 — Elements with Signatures (Default)

```
CRM
  entity Customer [name, email, phone, status]
    → Order (*), → Address (1)
  entity Order [orderNumber, amount, orderDate, status]
    → Customer (1), → OrderLine (*)
  entity OrderLine [quantity, unitPrice, lineTotal]
    → Order (1), → Product (1)
  microflow CreateOrder(Customer) → Order
  microflow ProcessOrder(Order) → boolean
  microflow ValidateCustomer(Customer) → boolean
  microflow SendOrderConfirmation(Order)
  nanoflow GetCurrentCustomer() → Customer
  page CustomerOverview [datagrid<Customer>]
  page CustomerEdit [dataview<Customer>]
  page OrderDetail [dataview<Order>, datagrid<OrderLine>]
  enumeration OrderStatus [Draft, Pending, Paid, Shipped, Cancelled]
  JavaAction SendEmail(string, string) → boolean
  snippet CustomerCard

auth
  entity user [username, passwordHash, role, lastLogin]
    → Customer (1)
  microflow AuthenticateUser(string, string) → user
  ...
```

Rules for each element type:

**Entities:**
- Format: `entity Name [attr1, attr2, attr3, ...]`
- List attribute names only (no types) in brackets
- On next line(s), indented: associations as `→ TargetEntity (cardinality)`
- Cardinality: `(1)` for reference, `(*)` for reference set
- Only show associations where this entity is the owner/parent
- **Data source**: `attributes` table for names; associations need reader calls (see prerequisites)

**Microflows:**
- Format: `microflow Name(ParamType1, ParamType2) → ReturnType`
- Use simple type names: `string`, `integer`, `boolean`, `datetime`, entity name for objects
- For list parameters: `list<Order>`
- Omit return type arrow if return type is void/Nothing
- Don't show parameter names, just types
- **Data source**: `microflows` table has `ReturnType`; parameter types need reader calls or new catalog table (see prerequisites)

**Nanoflows:**
- Same format as microflows: `nanoflow Name(ParamType1) → ReturnType`

**Pages:**
- Format: `page Name [TopWidget1<entity>, TopWidget2<entity>]`
- Show the top-level data widgets only: DataView, DataGrid, ListView, TemplateGrid, Gallery
- Include the entity they're bound to in angle brackets
- If the page has no data widgets, just show `page Name`
- Don't dig into nested widgets
- **Data source**: `widgets` table (requires full mode catalog). Falls back to `page Name` if fast-mode only.

**Enumerations:**
- Format: `enumeration Name [Value1, Value2, Value3, ...]`
- Show all values inline
- **Data source**: Needs new `enumeration_values` table or reader calls (see prerequisites)

**Java Actions:**
- Format: `JavaAction Name(ParamType1) → ReturnType`
- Same rules as microflows
- **Data source**: `java_actions` table has `ReturnType`; parameter types need reader calls

**Snippets:**
- Format: `snippet Name`
- Just the name — snippets are reusable page fragments without signatures
- **Data source**: `snippets` table (always available)

**OData Clients (consumed):**
- Format: `ODataClient Name (version)`
- Show OData version if available
- **Data source**: `odata_clients` table has `Name`, `ODataVersion`, `MetadataUrl`

**OData Services (published):**
- Format: `ODataService Name /path (n entities)`
- Show the service path and entity set count
- **Data source**: `odata_services` table has `Name`, `path`, `EntitySetCount`

**Constants:**
- Format: `constant Name: type = DefaultValue`
- Show type and default value if available
- **Data source**: No catalog table — requires reader calls via `ListConstants()`

**Scheduled Events:**
- Format: `ScheduledEvent Name → microflow`
- Show the target microflow
- **Data source**: No catalog table — requires reader calls via `ListScheduledEvents()`

**Document ordering within a module:**
1. Entities (alphabetical)
2. Enumerations (alphabetical)
3. Microflows (alphabetical)
4. Nanoflows (alphabetical)
5. Pages (alphabetical)
6. Snippets (alphabetical)
7. Java Actions (alphabetical)
8. Constants (alphabetical)
9. Scheduled Events (alphabetical)
10. OData Clients (alphabetical)
11. OData Services (alphabetical)

### Depth 3 — Include Types and Details

Same as depth 2 but:
- Entity attributes show types: `entity Customer [name: string(100), email: string(255), status: OrderStatus]`
- Microflow parameters show names: `microflow ProcessOrder(order: Order) → boolean`
- Associations show delete behavior if non-default: `→ OrderLine (*) cascade`

**Data source**: `attributes` table already has `DataType` and `length` columns. Parameter names need reader calls.

## Module Filtering

Skip these modules unless `--all` is provided:
- Modules where `IsSystemModule = 1` (has AppStoreGuid set)
- Modules where `AppStoreGuid != ''` (marketplace modules)
- Any module that starts with `_` (internal convention)

This replaces a hardcoded blocklist with a catalog-driven heuristic. The `modules` table already tracks `IsSystemModule` and `AppStoreGuid`, which is sufficient to distinguish user modules from system/marketplace modules.

## Implementation Plan

### Phase 1: Catalog prerequisites

Fill the catalog gaps that `structure` depth 2 depends on. Each is a small, independent change following the `java_actions` pattern.

**1a. Add `associations` catalog table**
- Columns: `Id`, `Name`, `QualifiedName`, `ModuleName`, `ParentEntity`, `ChildEntity`, `type` (Reference/ReferenceSet), `owner`, `DeleteBehavior`
- Builder: `buildAssociations()` using `reader.ListDomainModels()` (associations are part of domain models)
- This also benefits `show references to` and `show impact of` queries

**1b. Add `enumeration_values` catalog table**
- Columns: `Id`, `Name`, `caption`, `EnumerationId`, `EnumerationQualifiedName`, `ModuleName`, `Sequence`
- Builder: `buildEnumerationValues()` — extend `buildEnumerations()` to also insert values
- Alternative: store values as comma-separated in a `values` column on `enumerations` (simpler, sufficient for structure output)

**1c. Add microflow parameter info to catalog**
- Option A: New `microflow_parameters` table with `Name`, `type`, `MicroflowId`, `Sequence`
- Option B: Add `ParameterTypes` text column to `microflows` table (comma-separated, e.g., `"Customer, string"`)
- Option B is simpler and sufficient for depth 2 output; Option A is better for depth 3 (needs parameter names)

### Phase 2: Structure command (depth 1)

Implement the basic command with depth 1 output only. This requires no catalog changes — it's pure GROUP BY queries on existing tables.

- Add `cmd/mxcli/cmd_structure.go` with Cobra command definition
- Add `mdl/executor/cmd_structure.go` with `execShowStructure()` for REPL support
- Add grammar rule for `show structure [depth n] [in module]`
- Module filtering via `IsSystemModule` / `AppStoreGuid` columns

### Phase 3: Structure command (depth 2)

Add element-level output using catalog data from Phase 1.

- Entity attributes from `attributes` table
- Associations from `associations` table (Phase 1a)
- Microflow return types from `microflows` table
- Microflow parameter types from new catalog data (Phase 1c)
- Enumeration values from new catalog data (Phase 1b)
- Page widgets from `widgets` table (graceful fallback if fast-mode)
- Snippets from `snippets` table
- Java actions from `java_actions` table
- OData clients from `odata_clients` table
- OData services from `odata_services` table
- Constants via reader `ListConstants()` (no catalog table)
- Scheduled events via reader `ListScheduledEvents()` (no catalog table)

### Phase 4: Structure command (depth 3)

Add type details — mostly just formatting changes since the data is already in the catalog from Phase 1.

## Performance

- Depth 1: 1 query per table type × ~11 types = ~11 queries (+ reader calls for constants/scheduled events). Sub-100ms.
- Depth 2: 1 query per module per element type. For 10 user modules × 11 types = ~110 queries. Should be under 500ms with SQLite.
- Depth 3: Same query count as depth 2, just more columns. No additional cost.
- All data comes from the catalog — no BSON deserialization at query time.

## Output to Stdout

- Plain text, no color codes (so it works when piped or captured by LLMs)
- UTF-8 arrow character `→` for associations and return types
- If stdout is a terminal, optionally add color (but make it disableable with `--no-color` or `NO_COLOR` env var)

## Error Handling

- If no project is connected/found: clear error message with usage hint
- If catalog is empty/not indexed: auto-build catalog (fast mode for depth 1-2, suggest full mode for page widget info)

## Testing

Write tests that verify:
1. Depth 1 output format with counts
2. Depth 2 output format with element signatures
3. Depth 3 output format with types
4. Module filtering (--module flag)
5. System module exclusion (using IsSystemModule/AppStoreGuid)
6. Alphabetical ordering within categories
7. Empty modules are skipped
8. Correct cardinality display for associations
9. Graceful fallback when widgets table is empty (fast-mode catalog)

## Example Test Case

Given a project with:
- Module "Sales" containing Entity "Invoice" with attributes [number, date, total], associated to Entity "InvoiceLine" (1:*) and Entity "Customer" (*:1)
- Microflow "CreateInvoice" with parameter Customer, returns Invoice
- Page "InvoiceOverview" with a DataGrid bound to Invoice

Expected depth 2 output:
```
Sales
  entity Customer [...]
    → Invoice (*)
  entity Invoice [number, date, total]
    → InvoiceLine (*), → Customer (1)
  entity InvoiceLine [...]
    → Invoice (1)
  microflow CreateInvoice(Customer) → Invoice
  page InvoiceOverview [datagrid<Invoice>]
```

(Where `[...]` represents whatever attributes those entities have)
