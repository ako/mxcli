# OQL Queries

The `mxcli oql` command executes OQL (Object Query Language) queries against a running Mendix runtime via the M2EE admin API. OQL is Mendix's query language for retrieving data from the runtime, similar to SQL but operating on Mendix entities rather than database tables.

## Usage

```bash
mxcli oql -p app.mpr "SELECT * FROM Sales.Customer"
```

## Prerequisites

- A Mendix application must be running (via `mxcli docker run` or another deployment)
- The M2EE admin API must be accessible
- The project file is needed to resolve entity and attribute names

## Query Syntax

OQL uses Mendix entity names and attribute names, not database table names:

```bash
# Select all customers
mxcli oql -p app.mpr "SELECT * FROM Sales.Customer"

# Select specific attributes
mxcli oql -p app.mpr "SELECT Name, Email FROM Sales.Customer"

# Filter with WHERE
mxcli oql -p app.mpr "SELECT * FROM Sales.Order WHERE Status = 'Open'"

# Join entities via associations
mxcli oql -p app.mpr "SELECT o.OrderNumber, c.Name FROM Sales.Order o JOIN Sales.Customer c ON o.Sales.Order_Customer = c"

# Aggregation
mxcli oql -p app.mpr "SELECT COUNT(*) FROM Sales.Customer"
```

## `ORDER BY` and null values

`ORDER BY <attribute> DESC` on an attribute that some rows leave empty does **not**
return the newest rows first. Mendix emits the ordering to the database without a
null placement, so the database's default applies — and on PostgreSQL, `DESC`
means **NULLS FIRST**:

```sql
-- what Mendix sends for `ORDER BY DealtAt DESC LIMIT 3`
SELECT "sales$game"."label" AS "label"
FROM "sales$game"
ORDER BY "sales$game"."dealtat" DESC
LIMIT $1
```

With two null `DealtAt` rows in the table, that returns both nulls and then one
real row — so "give me the most recent 3" silently answers with rows that are not
recent, **stably** across runs. The stability is what makes it convincing and
wrong: a degenerate sort key is every bit as repeatable as a correct one.

Two things follow:

- **Check for nulls before concluding the ordering is broken.**
  `SELECT count(*) AS n FROM Sales.Game WHERE DealtAt = empty` settles it in one
  query. Measured on Mendix 11.13.0 against PostgreSQL, `ORDER BY` on a DateTime
  is passed through correctly and orders correctly when the values are present —
  it is not ignored.
- **Add `NULLS LAST` when the attribute is optional**, rather than falling back to
  ordering by `id`. Ordering by `id` answers a different question (insertion
  order), which happens to agree with recency only when nothing backdates a row.

Null placement is database-specific — SQL Server and Oracle differ from
PostgreSQL — so a query relying on the default behaves differently per
environment. Being explicit is the portable choice.

## Difference from Catalog Queries

| Feature | OQL (`mxcli oql`) | Catalog (`SELECT FROM CATALOG.*`) |
|---------|-------------------|----------------------------------|
| Data source | Running Mendix runtime | Project metadata (MPR file) |
| Queries | Instance data (rows) | Model structure (entities, microflows) |
| Requires | Running application | Only the MPR file |
| Language | OQL (Mendix-specific) | SQL (SQLite) |

- Use **OQL** to query actual data in a running application
- Use **catalog queries** to analyze the project structure and metadata

## Use Cases

### Verifying Imported Data

After running `IMPORT FROM`, verify the data was imported correctly:

```bash
mxcli oql -p app.mpr "SELECT COUNT(*) FROM HR.Employee"
mxcli oql -p app.mpr "SELECT Name, Email FROM HR.Employee LIMIT 10"
```

### Debugging Business Logic

Check data state while debugging microflows:

```bash
mxcli oql -p app.mpr "SELECT * FROM Sales.Order WHERE Status = 'Draft'"
```

### Data Exploration

Explore data patterns in a running application:

```bash
mxcli oql -p app.mpr "SELECT Status, COUNT(*) FROM Sales.Order GROUP BY Status"
```
