---
title: Direct SQL Query Execution in mxcli
status: done
---

# Proposal: Direct SQL Query Execution in mxcli

**Status**: Draft
**Date**: 2026-03-03
**Branch**: `sql`

## Motivation

When building Mendix applications that integrate with existing databases (via the Database Connector), developers and LLM-assisted workflows face three recurring needs:

1. **Discovery** — Explore schemas and sample data in existing databases to understand what's available before configuring the Mendix Database Connector.
2. **Test data import** — Populate the running Mendix application database with representative data from the source database for development and testing.
3. **Data migration** — Move production data from a legacy database into the Mendix application database as part of a migration project.

Today these tasks require separate database client tools (psql, sqlcmd, SQL*Plus) installed on the developer machine, and they expose database credentials to the LLM session. By integrating SQL execution directly into mxcli we gain:

- **Zero external dependencies** — No need to install database-specific client tools. Pure Go drivers compile into the mxcli binary.
- **Credential isolation** — mxcli can read connection details from environment variables or configuration files and never expose them to the LLM. The LLM sends SQL queries; mxcli handles authentication transparently.
- **Unified workflow** — Discover → Design domain model → Import data → Test, all within a single mxcli session.

## Use Cases

### Use Case 1: Schema Discovery

```
mxcli> sql connect postgres as source
mxcli> sql source show tables
mxcli> sql source describe employees
mxcli> sql source select * from employees limit 10
```

The LLM uses this to understand the source database structure, then generates MDL to create matching Mendix entities, database connection configurations, and query definitions.

### Use Case 2: Test Data Import

```
mxcli> sql connect postgres as source
mxcli> sql source select id, name, email, department from employees
-- LLM sees the data shape, then generates:
mxcli> OQL insert into HRModule.Employee (Name, Email, Department) values ('Alice', 'alice@example.com', 'Engineering')
-- Or bulk import via a new IMPORT command:
mxcli> import from source query 'SELECT name, email, department FROM employees' into HRModule.Employee map (name as Name, email as Email, department as Department)
```

### Use Case 3: Data Migration

Same as test data import but at scale, potentially with transformation logic:

```
mxcli> import from source query 'SELECT ...' into Module.Entity map (...) batch 1000
```

## Proposed Architecture

### Connection Management

Connections are named and stored in the mxcli session. Credentials come from one of three sources (checked in order):

1. **Environment variables** — e.g., `MXCLI_SQL_POSTGRES_HOST`, `MXCLI_SQL_POSTGRES_PASSWORD`
2. **Configuration file** — `.mxcli/connections.yaml` (gitignored) in project root
3. **Interactive prompt** — mxcli prompts the user (not the LLM) for missing credentials

```yaml
# .mxcli/connections.yaml
connections:
  source:
    driver: postgres
    host: 10.211.55.2
    port: 5432
    database: legacy_hr
    username: readonly_user
    # password: omitted → read from MXCLI_SQL_SOURCE_PASSWORD env var
    # or prompt interactively
  warehouse:
    driver: sqlserver
    host: sql.corp.local
    port: 1433
    database: DataWarehouse
    username: etl_user
```

### Credential Isolation

The key security property: **the LLM never sees credentials**. The flow is:

```
LLM → "sql connect postgres as source" → mxcli resolves credentials internally
LLM → "sql source select * from employees limit 5" → mxcli executes, returns results
```

mxcli outputs query results (tabular data) but never echoes back connection strings, passwords, or authentication tokens. If a connection requires credentials not available from env/config, mxcli prompts the human user directly via stderr (which is not captured by the LLM in MCP mode).

### MDL Syntax

New statements to add to the MDL grammar:

```sql
-- Connection management
sql connect <driver> as <alias> [OPTIONS ...]
sql disconnect <alias>
sql connections                          -- list active connections

-- Schema discovery
sql <alias> show tables [like 'pattern']
sql <alias> show views [like 'pattern']
sql <alias> describe <table>             -- columns, types, keys, indexes

-- Query execution
sql <alias> <any-sql-statement>          -- freeform SQL passthrough

-- Import (from external DB → running Mendix app via OQL/M2EE)
import from <alias>
  query '<sql>'
  into <Module.Entity>
  map (<sql_col> as <MxAttribute>, ...)
  [batch <size>]
  [limit <count>]
  [where <filter>]
```

### Pure Go Database Drivers

All drivers are pure Go (no CGO, no external libraries):

| Database    | Go Driver                          | Pure Go | Notes                              |
|-------------|------------------------------------|---------|------------------------------------|
| PostgreSQL  | `github.com/jackc/pgx/v5`         | Yes     | De facto standard, high performance |
| SQL Server  | `github.com/microsoft/go-mssqldb` | Yes     | Official Microsoft driver           |
| Oracle      | `github.com/sijms/go-ora/v2`      | Yes     | Community driver, TNS & OCI support |
| MySQL       | `github.com/go-sql-driver/mysql`  | Yes     | Standard MySQL/MariaDB driver       |

All four implement Go's `database/sql` interface, so the mxcli implementation can use a single abstraction layer.

### Implementation Packages

```
cmd/mxcli/
├── cmd_sql.go              # Cobra command: mxcli sql
├── cmd_import.go           # Cobra command: mxcli import (or import statement)

mdl/
├── ast/
│   ├── sql_statements.go   # AST nodes for sql connect, sql query, import
├── visitor/
│   ├── visitor_sql.go      # ANTLR visitor → AST for sql statements
├── executor/
│   ├── cmd_sql.go          # sql statement execution
│   ├── cmd_import.go       # import statement execution

sql/                         # New package: database abstraction
├── driver.go               # Driver registry and connection pool
├── connection.go           # Named connection management
├── config.go               # Credential resolution (env, file, prompt)
├── discovery.go            # show tables, describe, schema introspection
├── result.go               # query result formatting (table, json, CSV)
├── import.go               # Bulk import: external DB → Mendix runtime
```

### Result Formatting

Query results are formatted as aligned tables by default (matching existing `mxcli oql` output), with optional `--format json` and `--format csv` flags for programmatic consumption:

```
mxcli> sql source select id, name, department from employees limit 5
+----+-----------+-------------+
| id | name      | department  |
+----+-----------+-------------+
|  1 | Alice     | Engineering |
|  2 | Bob       | Sales       |
|  3 | Charlie   | Engineering |
|  4 | Diana     | Marketing   |
|  5 | Eve       | Sales       |
+----+-----------+-------------+
5 rows (12ms)
```

## Phased Implementation

### Phase 1: Connection & Query (MVP) ✅ Implemented

- `sql/` package with driver registry and connection pool
- Credential resolution from env vars and config file
- `sql connect`, `sql disconnect`, `sql connections` statements
- Freeform SQL passthrough with tabular result output
- Support for PostgreSQL only (simplest to test)
- Cobra subcommand: `mxcli sql -c <alias> "<query>"`
- REPL integration: `sql <alias> <query>`

### Phase 2: Schema Discovery ✅ Implemented

- `sql <alias> show tables`, `show views`
- `sql <alias> describe <table>` — columns, types, nullability, keys, indexes
- Cross-database schema introspection abstraction (each driver implements a `SchemaProvider` interface)
- Add SQL Server and Oracle driver support

### Phase 3: Import & Migration ✅ Implemented

- `import from <alias> query '...' into Module.Entity map (...) [batch n] [limit n]`
- Batched insert directly into Mendix app PostgreSQL database (M2EE `preview_execute_oql` is read-only)
- Automatic Mendix ID generation (`(short_id << 48) | sequence`) and sequence counter updates
- Auto-connects to Mendix app DB using project settings (`_mendix` alias)
- `MXCLI_DB_HOST` env var for host override (devcontainers)
- Progress reporting for long-running imports
- Per-batch transactions (INSERT + sequence update atomic)
- Auto-splits batches if PostgreSQL 65535 parameter limit would be exceeded

### Phase 4: Advanced Features

- MySQL driver support
- `--format json|csv` output modes
- Schema diff: compare external DB schema against Mendix domain model
- Auto-generate MDL (`create entity`, `create database connection`) from discovered schema ✅ Implemented (`sql <alias> generate connector into <module>`)
- Connection testing: `sql <alias> PING`

## Credential Resolution Detail

Resolution order for a connection named `source` with driver `postgres`:

1. **Explicit in config file** — `.mxcli/connections.yaml` field values
2. **Alias-specific env vars** — `MXCLI_SQL_SOURCE_HOST`, `MXCLI_SQL_SOURCE_PASSWORD`, etc.
3. **Driver-specific env vars** — `MXCLI_SQL_POSTGRES_HOST`, `MXCLI_SQL_POSTGRES_PASSWORD`
4. **Standard database env vars** — `PGHOST`, `PGPASSWORD` (for postgres); `MSSQL_SA_PASSWORD` (for sqlserver)
5. **Interactive prompt** — Prompt on stderr (invisible to LLM in MCP/pipe mode)

Passwords are **never logged**, **never included in error messages**, and **never returned in query results**.

## Integration with Existing Features

### Database Connector Configuration

After discovering a schema via `sql <alias> describe`, the LLM can generate:

```sql
-- MDL to create Mendix Database Connector configuration
create constant HR.PgConnectionString type string default 'jdbc:postgresql://...';
create non-persistent entity HR.EmployeeRecord (Name: string(100), Department: string(50));
create database connection HR.LegacyDB
type 'PostgreSQL'
connection string @HR.PgConnectionString
username @HR.PgUser
password @HR.PgPassword
begin
  query GetEmployees sql 'SELECT name, department FROM employees' returns HR.EmployeeRecord;
end;
```

### OQL Integration

The existing `mxcli oql` command inserts data into the running Mendix runtime. The `import` command builds on this by automating the read-from-external-DB → insert-into-Mendix loop.

### Catalog Integration

Discovered schemas can be registered in the mxcli catalog for cross-referencing:

```sql
refresh catalog sql source   -- index external DB schema in catalog
select * from CATALOG.sql_tables where connection = 'source'
```

## Security Considerations

- **Read-only by default** — `sql connect` defaults to read-only mode. Write operations (`insert`, `update`, `delete`, `drop`) require explicit `sql connect ... MODE readwrite`.
- **Query allow/deny lists** — Optional config to restrict which SQL statements are allowed.
- **No credential leakage** — Credentials are resolved internally and never appear in output, logs, or error messages visible to the LLM.
- **Connection timeout** — Default 10s connect timeout, 30s query timeout. Configurable per connection.
- **Row limits** — Default `limit 1000` applied to `select` queries that don't specify a limit, to prevent accidental large result sets.

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Oracle driver (`go-ora`) less mature than pgx | Phase Oracle support — start with PostgreSQL, add Oracle in Phase 2 with thorough testing |
| Binary size increase from 4 database drivers | Drivers are relatively small pure Go; total ~2-3MB increase. Could use build tags to make drivers optional |
| SQL injection via LLM-generated queries | Read-only default; document that mxcli trusts the SQL it receives (same trust model as OQL) |
| Network access to databases from dev environment | Same requirements as any database client; no new attack surface |

## Open Questions

1. **IMPORT target** — Should IMPORT insert into the Mendix runtime (via OQL/M2EE, requires running app) or directly into the MPR database (offline, but limited)? Recommendation: Runtime via M2EE (consistent with existing `mxcli oql`).
2. **Connection persistence** — Should connections survive across mxcli sessions? Recommendation: No, session-scoped only. Config file provides convenience for reconnection.
3. **Build tags for drivers** — Should database drivers be behind build tags to keep the default binary small? Recommendation: Include all by default; pure Go drivers add minimal size.
4. **SSH tunneling** — Should mxcli support SSH tunnels for reaching databases behind firewalls? Recommendation: Defer to Phase 4+; users can set up tunnels externally.
