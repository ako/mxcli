---
title: Association Mapping in IMPORT
status: draft
---

# Proposal: Association Mapping in IMPORT

**Status**: Draft
**Date**: 2026-03-04
**Parent**: [PROPOSAL_mxcli_sql.md](PROPOSAL_mxcli_sql.md) (Phase 3 extension)

## Problem

The current `import from` command maps source columns to entity attributes only. Associations — the foreign key relationships between Mendix entities — must be set up manually with raw SQL after import. This is error-prone: the user must know the storage mode (column vs junction table), the exact database column names, and the Mendix ID of each referenced object.

A typical workaround today:

```sql
-- Step 1: Import employees without department link
import from source query 'SELECT name, email FROM employees'
  into HR.Employee map (name as Name, email as Email);

-- Step 2: Manually figure out FK column name and run raw SQL
sql _mendix update "hr$employee" e
  set "hr$employee_department" = d.id
  from "hr$department" d
  where d.name = e.department_name;
-- ^^^ Requires knowing column name, storage mode, table names
```

This should be a single command.

## Proposed Syntax

Extend `import from` with an optional `link` clause that maps source columns to associations:

```sql
-- Lookup by natural key on the child entity
import from source query 'SELECT name, email, dept_name FROM employees'
  into HR.Employee
  map (name as Name, email as Email)
  link (dept_name to Employee_Department on Name)
  batch 500;

-- Multiple associations
import from source query $$
    select e.name, e.email, d.name as dept, m.email as mgr_email
    from employees e
    join departments d on e.dept_id = d.id
    left join employees m on e.manager_id = m.id
$$
  into HR.Employee
  map (name as Name, email as Email)
  link (dept to Employee_Department on Name,
        mgr_email to Employee_Manager on Email);

-- Direct ID mapping (source already has Mendix IDs — rare)
import from source query 'SELECT name, dept_mx_id FROM migrated_employees'
  into HR.Employee
  map (name as Name)
  link (dept_mx_id to Employee_Department);
```

### LINK Clause Grammar

```
link ( linkMapping (, linkMapping)* )

linkMapping:
    sourceColumn to associationName on childAttribute   -- lookup mode
  | sourceColumn to associationName                     -- direct ID mode
```

- **sourceColumn**: Column name from the source query result set
- **associationName**: Unqualified association name (module inferred from target entity)
- **ON childAttribute**: Attribute on the *child* entity to match against the source value
- Without `on`: the source value is treated as a raw Mendix object ID

## Design

### Algorithm

**Pre-import setup** (once, before streaming rows):

1. For each LINK mapping, resolve association metadata:
   - Query `mendixsystem$association` in the Mendix app DB:
     ```sql
     select association_name, table_name, child_column_name, storage_format
     from mendixsystem$association
     where association_name = $1
     ```
   - Determines: column storage (FK inline) vs table storage (junction table)
   - Gets exact database column/table names (no guessing conventions)

2. For each LINK with `on` clause, build a lookup cache:
   - Identify child entity from the association (MPR reader or system tables)
   - Query child entity table:
     ```sql
     select id, <lookup_column> from <child_table>
     ```
   - Build `map[any]int64` (lookup value → Mendix object ID)
   - For large tables (>100K rows), fall back to per-row queries with a bounded LRU cache

**During import** (per row):

3. For each LINK mapping, resolve the source value to a Mendix ID:
   - `on` mode: look up in cache → get child entity's `id`
   - Direct mode: use source value as-is (cast to int64)
   - NULL source value → NULL FK (no association)

4. Insert the resolved FK based on storage mode:
   - **Column storage**: Add FK column + value to the same INSERT as the entity row
   - **Table storage**: Collect `(parent_id, child_id)` pairs for batch insert after the entity INSERT

**After each batch INSERT**:

5. For table-storage associations, batch-insert junction table rows:
   ```sql
   insert into "module$assoc_name" ("module$parentid", "module$childid")
   values ($1, $2), ($3, $4), ...
   ```

### How Association Metadata Is Resolved

Two sources are available; we use both for robustness:

| Source | What It Provides | When Available |
|--------|-----------------|----------------|
| **MPR (reader)** | Association name, parent/child entity, Type, Owner, StorageFormat | Always (project must be connected) |
| **`mendixsystem$association`** | Exact DB column names, table names | After app has been started once |

**Resolution flow:**

1. Read association from MPR by name → get child entity qualified name, storage format
2. Query `mendixsystem$association` → get exact `child_column_name` and `table_name`
3. If system table unavailable (app never started), derive column names from conventions:
   - Column storage FK: `{module}${association_name_lower}`
   - Junction table: `{module}${association_name_lower}` with columns `{module}${parent_entity_lower}id` and `{module}${child_entity_lower}id`

### Scope Constraints (MVP)

| Feature | Supported | Notes |
|---------|-----------|-------|
| Reference (1:N) associations | Yes | Column or table storage |
| ReferenceSet (N:M) associations | No | Junction table with multiple rows per entity — complex source data format needed |
| Cross-module associations | Yes | Child entity in different module |
| Self-referencing associations | Yes | e.g., Employee → Employee for manager |
| NULL source values | Yes | No association link created |
| Multiple LINK mappings | Yes | Different associations on same entity |
| Lookup cache | Yes | Pre-built for tables <100K rows |

### Error Handling

| Condition | Behavior |
|-----------|----------|
| Association not found in MPR | Error before import starts |
| Association is ReferenceSet | Error: "ReferenceSet associations not supported in IMPORT; use manual SQL" |
| Lookup value not found in child table | Warning + NULL FK (row still imported, no link) |
| Duplicate lookup values in child table | Error before import: "ambiguous lookup — N rows match value X for attribute Y" |
| Child entity table empty | Warning: "child table is empty; all associations will be NULL" |
| `mendixsystem$association` unavailable | Fall back to convention-based column names |

## Implementation Steps

### Step 1: Extend Grammar

**Modify** `mdl/grammar/MDLParser.g4`:

```antlr
importStatement
    : import from identifierOrKeyword query (STRING_LITERAL | DOLLAR_STRING)
      into qualifiedName
      map LPAREN importMapping (COMMA importMapping)* RPAREN
      (link LPAREN linkMapping (COMMA linkMapping)* RPAREN)?    // ← NEW
      (batch NUMBER_LITERAL)?
      (limit NUMBER_LITERAL)?                                    # importFromQuery
    ;

linkMapping
    : identifierOrKeyword to identifierOrKeyword on identifierOrKeyword   # linkLookup
    | identifierOrKeyword to identifierOrKeyword                          # linkDirect
    ;
```

**Modify** `mdl/grammar/MDLLexer.g4`:
- Add `link: L I N K;` token (if not already present)
- Add `link` to `keyword` rule in parser

### Step 2: Extend AST

**Modify** `mdl/ast/ast_sql.go`:

```go
type LinkMapping struct {
    SourceColumn    string
    AssociationName string
    LookupAttr      string // empty = direct ID mode
}

type ImportStmt struct {
    SourceAlias  string
    query        string
    TargetEntity string
    mappings     []ImportMapping
    Links        []LinkMapping     // ← NEW
    BatchSize    int
    limit        int
}
```

### Step 3: Extend Visitor

**Modify** `mdl/visitor/visitor_sql.go` — in `ExitImportFromQuery`:
- Parse `AllLinkMapping()` contexts
- Build `[]ast.LinkMapping` from `linkLookup` and `linkDirect` alternatives

### Step 4: Create `sql/import_assoc.go` (new)

Association resolution and lookup logic:

```go
// AssocInfo holds resolved association metadata for import.
type AssocInfo struct {
    AssociationName string
    ChildEntity     string  // qualified name
    StorageFormat   string  // "column" or "table"
    FKColumnName    string  // for column storage: column in parent table
    JunctionTable   string  // for table storage: junction table name
    ParentColName   string  // for table storage: parent ID column in junction
    ChildColName    string  // for table storage: child ID column in junction
    LookupAttr     string  // attribute on child entity for lookup (empty = direct)
    LookupCache    map[any]int64 // value → Mendix ID
}

// ResolveAssociations looks up association metadata and builds lookup caches.
func ResolveAssociations(ctx context.Context, mendixConn *connection,
    reader AssociationReader, entityName string, links []LinkMapping) ([]*AssocInfo, error)

// LookupAssociation resolves a single source value to a Mendix ID.
func (a *AssocInfo) Lookup(value any) (int64, bool)
```

### Step 5: Extend `sql/import.go`

Modify `ExecuteImport` and `insertBatch`:

- Accept `[]*AssocInfo` in `ImportConfig`
- In `insertBatch`:
  - For column-storage associations: add FK columns to the INSERT column list
  - For table-storage associations: collect `(parentID, childID)` pairs
- After entity INSERT, batch-insert junction table rows within same transaction

### Step 6: Extend Executor

**Modify** `mdl/executor/cmd_import.go`:
- After getting source/target connections, call `ResolveAssociations()` for each LINK mapping
- Pass resolved `[]*AssocInfo` into `ImportConfig`
- Use MPR reader to find associations by name and get child entity info

### Step 7: Update Docs

- `cmd/mxcli/help_topics/sql.txt` — add LINK clause syntax and examples
- `mdl/executor/cmd_misc.go` — update HELP text
- `.claude/skills/mendix/demo-data.md` — update IMPORT section with LINK examples
- `docs/01-project/MDL_QUICK_REFERENCE.md` — update IMPORT row

## Files Summary

| Action | File |
|--------|------|
| MODIFY | `mdl/grammar/MDLLexer.g4` (add `link` token if needed) |
| MODIFY | `mdl/grammar/MDLParser.g4` (add `linkMapping` rule, extend `importStatement`) |
| MODIFY | `mdl/ast/ast_sql.go` (add `LinkMapping`, extend `ImportStmt`) |
| MODIFY | `mdl/visitor/visitor_sql.go` (parse LINK clause) |
| CREATE | `sql/import_assoc.go` (association resolution + lookup cache) |
| MODIFY | `sql/import.go` (FK columns in INSERT, junction table inserts) |
| MODIFY | `mdl/executor/cmd_import.go` (resolve associations, pass to import) |
| MODIFY | `mdl/executor/stmt_summary.go` (include link count in summary) |
| MODIFY | Help text and docs (4 files) |

## Examples

### Basic: Import employees with department association

```sql
sql connect postgres 'postgres://...' as legacy;

-- Check what's available
sql legacy describe employees;
-- id | name | email | department_name

-- Import with department lookup
import from legacy query 'SELECT name, email, department_name FROM employees'
  into HR.Employee
  map (name as Name, email as Email)
  link (department_name to Employee_Department on Name)
  batch 500;

-- Output:
-- Resolving associations...
--   Employee_Department: Column storage, lookup by HR.Department.Name (45 distinct values cached)
-- Importing...
--   batch 1: 500 rows imported
--   batch 2: 500 rows imported
--   batch 3: 234 rows imported
-- Imported 1234 rows into HR.Employee (3 batches, 1.2s)
--   Associations: Employee_Department linked 1198/1234 rows (36 NULL — lookup value not found)
```

### Multiple associations with mixed storage modes

```sql
import from legacy query $$
    select e.name, d.name as dept, m.email as mgr
    from employees e
    join departments d on e.dept_id = d.id
    left join employees m on e.manager_id = m.id
$$
  into HR.Employee
  map (name as Name)
  link (dept to Employee_Department on Name,
        mgr to Employee_Manager on Email);
```

### Direct ID mapping (migration from another Mendix app)

```sql
import from oldapp query 'SELECT name, email, department_id FROM hr$employee'
  into HR.Employee
  map (name as Name, email as Email)
  link (department_id to Employee_Department);
```

## Future Extensions (not in scope)

- **ReferenceSet support**: Source format TBD — could be comma-separated IDs or a separate mapping query
- **Create-or-lookup**: If lookup value not found, auto-create the child entity (e.g., create department on-the-fly)
- **Cascading import**: Import parent and child entities in one command with auto-linking
- **EXPORT**: Reverse direction — export Mendix data with associations resolved to human-readable values

## Key Files to Reference

- `sql/import.go` — current import pipeline (extend this)
- `sql/mendix.go` — Mendix DSN builder, table/column name helpers
- `sdk/domainmodel/domainmodel.go:275-330` — Association, CrossModuleAssociation types
- `mdl/ast/ast_association.go` — AST association types (StorageType, OwnerType)
- `mdl/executor/cmd_associations.go` — how associations are resolved from the MPR
- `sdk/mpr/reader_documents.go:144-174` — `GetDomainModel()` for reading associations
- `.claude/skills/mendix/demo-data.md` — Mendix DB storage conventions
