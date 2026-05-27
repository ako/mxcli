---
title: Generate Database Connector from External Schema
status: done
---

# Proposal: Generate Database Connector from External Schema

**Status**: Draft
**Date**: 2026-03-05
**Branch**: `sql`
**Depends on**: Phase 1-3 (SQL CONNECT, schema discovery, IMPORT) — all implemented

## Motivation

When using Mendix as a **frontend for existing applications**, developers need to:

1. Connect to the existing database
2. Create non-persistent entities matching the database tables/views
3. Set up constants for JDBC connection credentials
4. Create a Database Connection document with query definitions
5. Build pages to display the data

Steps 2–4 are tedious and mechanical: discover columns, map SQL types to Mendix types, generate boilerplate MDL. With `sql connect` and `describe` already implemented, mxcli can automate the entire process with a single command.

## Use Case

```
-- Connect to the external database
sql connect postgres 'postgres://user:pass@host:5432/legacydb' as source;

-- Discover the schema
sql source show tables;
sql source describe employees;

-- Generate everything: constants + non-persistent entities + database connection + queries
sql source generate connector into HRModule;

-- Or generate for specific tables only
sql source generate connector into HRModule tables (employees, departments);

-- Or generate for views
sql source generate connector into HRModule views (active_employees_v);
```

Output: a complete MDL script that can be piped to `mxcli exec`, printed for review, or executed directly.

## Generated MDL

For a PostgreSQL database with table `employees(id serial, name varchar(100), email varchar(200), salary numeric(10,2), active boolean, hired_at timestamp)`, the command generates:

```sql
-- Constants (names must follow Database Connector convention: {ConnectionName}_DB{Suffix})
create constant HRModule.SourceDatabase_DBSource type string
  default 'jdbc:postgresql://host:5432/legacydb'
  comment 'JDBC connection string for SourceDatabase';

create constant HRModule.SourceDatabase_DBUsername type string
  default 'user'
  comment 'Database username for SourceDatabase';

create constant HRModule.SourceDatabase_DBPassword type string
  default ''
  comment 'Database password for SourceDatabase';

-- Non-persistent entity for employees table
create non-persistent entity HRModule.Employee (
  EmployeeId: integer,
  Name: string(100),
  Email: string(200),
  Salary: decimal,
  Active: boolean,
  HiredAt: datetime
);

-- Database Connection with queries
create database connection HRModule.SourceDatabase
type 'PostgreSQL'
connection string @HRModule.SourceDatabase_DBSource
username @HRModule.SourceDatabase_DBUsername
password @HRModule.SourceDatabase_DBPassword
begin
  query GetAllEmployees
    sql 'SELECT id, name, email, salary, active, hired_at FROM employees'
    returns HRModule.Employee
    map (
      id as EmployeeId,
      name as Name,
      email as Email,
      salary as Salary,
      active as Active,
      hired_at as HiredAt
    );
end;
```

## Design Decisions

### SQL Type → Mendix Type Mapping

| SQL Type (PostgreSQL/Oracle/MSSQL) | Mendix Type | Notes |
|---|---|---|
| `integer`, `int`, `int4`, `NUMBER(n,0)` n≤10 | `integer` | |
| `bigint`, `int8`, `NUMBER(n,0)` n>10 | `long` | |
| `smallint`, `int2`, `tinyint` | `integer` | |
| `serial`, `bigserial`, `identity` | `autonumber` | Primary key columns only; `integer` otherwise |
| `numeric`, `decimal`, `NUMBER(p,s)` s>0, `money` | `decimal` | |
| `real`, `float4`, `float`, `double`, `BINARY_FLOAT` | `decimal` | Mendix has no float type |
| `varchar(n)`, `character varying(n)`, `VARCHAR2(n)`, `nvarchar(n)` | `string(n)` | Length preserved |
| `text`, `clob`, `ntext`, `nvarchar(max)` | `string(unlimited)` | No length limit |
| `char(n)`, `nchar(n)` | `string(n)` | |
| `boolean`, `bit` (MSSQL) | `boolean` | |
| `date` | `datetime` | Mendix has no date-only type |
| `timestamp`, `datetime`, `datetime2`, `date` (Oracle) | `datetime` | |
| `timestamp with time zone`, `datetimeoffset` | `datetime` | |
| `bytea`, `blob`, `varbinary`, `RAW` | `— skipped —` | Binary not mappable to Mendix type |
| `uuid`, `uniqueidentifier` | `string(36)` | |
| `json`, `jsonb`, `xml` | `string(unlimited)` | |

Unmappable types are skipped with a warning comment in the output.

### Naming Conventions

- **Entity name**: Table name → PascalCase singular. `employees` → `Employee`, `order_items` → `OrderItem`
- **Attribute name**: Column name → PascalCase. `first_name` → `FirstName`, `employee_id` → `EmployeeId`
- **Constants**: `{module}.{ConnectionName}_DBSource`, `{module}.{ConnectionName}_DBUsername`, `{module}.{ConnectionName}_DBPassword` (matches Mendix Database Connector convention)
- **Connection name**: `{module}.{Alias}database` (e.g. alias `f1` → `F1Database`)
- **Query name**: `GetAll{EntityName}` (one per table/view)

### DSN → JDBC URL Conversion

The mxcli connection uses Go driver DSNs (`postgres://...`), but the Mendix Database Connector needs JDBC URLs. Convert automatically:

| Go DSN | JDBC URL |
|--------|----------|
| `postgres://user:pass@host:5432/db` | `jdbc:postgresql://host:5432/db` |
| `oracle://user:pass@host:1521/service` | `jdbc:oracle:thin:@//host:1521/service` |
| `sqlserver://user:pass@host:1433?database=db` | `jdbc:sqlserver://host:1433;databaseName=db` |

### Output Modes

| Flag | Behavior |
|------|----------|
| (default) | Print generated MDL to stdout |
| `--exec` / `exec` | Execute the generated MDL against the open project |
| `--file <path>` | Write MDL to file |

In REPL mode (MDL syntax), use suffix keyword:

```sql
sql source generate connector into HRModule;           -- print to stdout
sql source generate connector into HRModule exec;      -- execute immediately
```

## MDL Syntax

```
sql <alias> generate connector into <module>
  [tables (<table1>, <table2>, ...)]
  [views (<view1>, <view2>, ...)]
  [exec];
```

- Without `tables`/`views`: generates for **all** user tables (excluding system tables)
- With `tables`: only the listed tables
- With `views`: only the listed views
- Can combine `tables` and `views`
- `exec`: execute the generated MDL immediately (requires open project)

## Implementation Plan

### Step 1: SQL Type Mapping (`sql/typemap.go` — new)

```go
type MendixType struct {
    TypeName string // "string", "integer", "decimal", "boolean", "datetime", "long", "autonumber"
    length   int    // for string(n); 0 = unlimited
}

// MapSQLType maps a sql data_type string to a Mendix type.
func MapSQLType(driver DriverName, sqlType string, length int, isPK bool) *MendixType

// GoDriverDSNToJDBC converts a Go driver DSN to a JDBC URL.
func GoDriverDSNToJDBC(driver DriverName, dsn string) (string, error)
```

### Step 2: Name Conventions (`sql/naming.go` — new)

```go
// TableToEntityName converts "order_items" → "OrderItem" (PascalCase, singular)
func TableToEntityName(tableName string) string

// ColumnToAttributeName converts "first_name" → "FirstName" (PascalCase)
func ColumnToAttributeName(colName string) string

// TableToQueryName returns "GetAll{EntityName}" for a table
func TableToQueryName(tableName string) string
```

Singularization: simple rules only (strip trailing `s`, `es`, `ies` → `y`). No NLP dependency.

### Step 3: Schema Reader (`sql/schema.go` — new)

```go
type TableSchema struct {
    schema  string
    Name    string
    columns []ColumnSchema
    IsView  bool
}

type ColumnSchema struct {
    Name       string
    DataType   string
    Nullable   bool
    length     int    // from character_maximum_length
    IsPK       bool
}

// ReadTableSchema reads column metadata with primary key detection.
func ReadTableSchema(ctx context.Context, conn *connection, tableName string) (*TableSchema, error)

// ReadAllTableSchemas reads schemas for all user tables.
func ReadAllTableSchemas(ctx context.Context, conn *connection) ([]*TableSchema, error)
```

This extends `meta.go`'s `DescribeTable` with PK detection and parsed length values (the current `DescribeTable` returns raw `information_schema` rows).

### Step 4: MDL Generator (`sql/generate.go` — new)

```go
type GenerateConfig struct {
    Conn       *connection
    module     string        // target Mendix module
    Alias      string        // connection alias (for naming)
    tables     []string      // nil = all tables
    views      []string      // nil = no views; empty = all views
    DSN        string        // original DSN for JDBC conversion
}

type GenerateResult struct {
    MDL           string       // complete MDL script
    TablesCount   int
    ViewsCount    int
    SkippedCols   []string     // columns with unmappable types
}

// GenerateConnector discovers schema and generates complete MDL.
func GenerateConnector(ctx context.Context, cfg *GenerateConfig) (*GenerateResult, error)
```

### Step 5: Grammar — add to `sqlPassthroughStatement`

The `sql <alias> generate connector into <module>` can be parsed within the existing `sqlPassthroughStatement` rule (the freeform SQL portion). The executor detects `generate connector` as the query prefix and dispatches to the generator instead of executing SQL.

Alternatively, add a dedicated grammar rule:

```antlr
sqlGenerateStatement
    : sql identifierOrKeyword generate connector into identifierOrKeyword
      (tables LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN)?
      (views LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN)?
      (exec)?
    ;
```

Add `generate` and `connector` tokens to `MDLLexer.g4`.

**Recommendation**: Dedicated grammar rule — gives better error messages and cleaner AST.

### Step 6: AST Node (`mdl/ast/ast_sql.go`)

```go
type SQLGenerateConnectorStmt struct {
    Alias      string
    module     string
    tables     []string // nil = all
    views      []string // nil = none
    exec       bool     // execute immediately
}
func (s *SQLGenerateConnectorStmt) isStatement() {}
```

### Step 7: Visitor (`mdl/visitor/visitor_sql.go`)

Add `ExitSqlGenerateStatement` to extract alias, module, table/view lists, exec flag.

### Step 8: Executor (`mdl/executor/cmd_sql.go`)

Add `execSQLGenerateConnector(s *ast.SQLGenerateConnectorStmt)`:
1. Get connection from manager
2. Call `sqllib.GenerateConnector(ctx, cfg)`
3. If `exec` and project is open: parse + execute the generated MDL
4. Otherwise: print to stdout

### Step 9: CLI Flag (`cmd/mxcli/cmd_sql.go`)

Add `--generate` flag to `mxcli sql` subcommand:
```bash
mxcli sql --alias source --generate --module HRModule
mxcli sql --alias source --generate --module HRModule --tables employees,departments
```

### Step 10: Update DATABASE CONNECTION Grammar

The current grammar only supports `host`/`port`/`database` options. Update to also support:
- `connection string` (string literal or `@Module.Constant` reference)
- `begin ... end` block with `query` definitions
- `query` with `sql`, `parameter`, `returns`, `map` clauses

This is needed so the generated MDL can be round-tripped (parsed back and executed).

```antlr
createDatabaseConnectionStatement
    : database connection qualifiedName
      databaseConnectionOptions
      (begin databaseQuery* end)?
    ;

databaseConnectionOption
    : type STRING_LITERAL
    | connection string (STRING_LITERAL | AT qualifiedName)
    | host STRING_LITERAL
    | port NUMBER_LITERAL
    | database STRING_LITERAL
    | username (STRING_LITERAL | AT qualifiedName)
    | password (STRING_LITERAL | AT qualifiedName)
    ;

databaseQuery
    : query IDENTIFIER
      sql (STRING_LITERAL | DOLLAR_STRING)
      (parameter IDENTIFIER COLON dataType)*
      returns qualifiedName
      (map LPAREN queryColumnMapping (COMMA queryColumnMapping)* RPAREN)?
      SEMICOLON
    ;

queryColumnMapping
    : identifierOrKeyword as identifierOrKeyword
    ;
```

## Files Summary

| Action | File | Description |
|--------|------|-------------|
| CREATE | `sql/typemap.go` | SQL → Mendix type mapping, DSN → JDBC conversion |
| CREATE | `sql/typemap_test.go` | Type mapping tests |
| CREATE | `sql/naming.go` | Table/column → entity/attribute naming conventions |
| CREATE | `sql/naming_test.go` | Naming convention tests |
| CREATE | `sql/schema.go` | Structured schema reader (extends meta.go) |
| CREATE | `sql/generate.go` | MDL generation from schema |
| CREATE | `sql/generate_test.go` | Generator tests |
| MODIFY | `mdl/grammar/MDLLexer.g4` | Add `generate`, `connector` tokens |
| MODIFY | `mdl/grammar/MDLParser.g4` | Add `sqlGenerateStatement` rule; extend `createDatabaseConnectionStatement` with `connection string`, `begin/end`, `query` |
| MODIFY | `mdl/ast/ast_sql.go` | Add `SQLGenerateConnectorStmt` |
| MODIFY | `mdl/visitor/visitor_sql.go` | Parse generate statement |
| MODIFY | `mdl/executor/cmd_sql.go` | Execute generate (print or exec) |
| MODIFY | `mdl/executor/executor.go` | Add dispatch case |
| MODIFY | `mdl/executor/stmt_summary.go` | Add summary case |
| MODIFY | `cmd/mxcli/cmd_sql.go` | Add `--generate` and `--module` flags |
| MODIFY | `cmd/mxcli/help_topics/sql.txt` | Document GENERATE CONNECTOR |
| MODIFY | `docs/01-project/MDL_QUICK_REFERENCE.md` | Add syntax row |

## Key Design Notes

1. **No new dependencies** — all type mapping and naming logic is pure Go string manipulation
2. **Roundtrip-safe** — generated MDL can be parsed back by extending the DATABASE CONNECTION grammar
3. **Incremental** — running GENERATE again for the same module is safe (CREATE statements use create-or-update semantics in the executor)
4. **Constants use constant references** (`@Module.Constant`) — credentials are not embedded in the connection definition
5. **One query per table/view** — generates `GetAll{entity}` as the baseline; users add parameterized queries manually

## Future Extensions (Not in This Phase)

- **Parameterized queries**: Generate `GetByPK` queries using primary key columns as parameters
- **Pages**: Generate overview pages (DataGrid2) for each entity after generating the connector
- **Sync mode**: Generate persistent entities + scheduled microflow for periodic data sync instead of non-persistent entities + Database Connector
- **OData mode**: Generate an OData service exposing the external data via External Entities
- **Diff/update**: Compare existing connector against current schema and generate ALTER statements
