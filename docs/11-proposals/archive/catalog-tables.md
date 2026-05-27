---
title: Catalog Tables for Go mxcli
status: done
---

# Proposal: Catalog Tables for Go mxcli

## Overview

Add support for catalog tables that enable flexible SQL querying of Mendix project metadata. This feature provides an in-memory SQLite database populated from MPR data, allowing users to run arbitrary SQL queries for analysis, searching, and agentic workflows.

## Motivation

Current MDL commands like `show entities`, `show microflows`, etc. provide fixed-format output. Catalog tables enable:

1. **Flexible querying** - Filter, join, aggregate data using standard SQL
2. **Cross-referencing** - Find relationships between objects (e.g., which microflows use entity X)
3. **Agentic search** - AI agents can explore project structure dynamically
4. **Impact analysis** - Track dependencies and usages across the model
5. **Comparison** - Snapshot support for comparing project states over time

## User Experience

### Building the Catalog

```
mdl> show catalog tables;
Building catalog...
✓ modules: 12
✓ entities: 45
✓ microflows: 120
✓ nanoflows: 15
✓ pages: 65
✓ snippets: 8
✓ enumerations: 12
✓ Activities: 450
✓ widgets: 890
✓ xpath Expressions: 78
✓ catalog ready (1.8s)

found 13 catalog table(s)
| table                     |
|---------------------------|
| CATALOG.MODULES           |
| CATALOG.ENTITIES          |
| CATALOG.MICROFLOWS        |
| CATALOG.NANOFLOWS         |
| CATALOG.PAGES             |
| CATALOG.SNIPPETS          |
| CATALOG.ENUMERATIONS      |
| CATALOG.ACTIVITIES        |
| CATALOG.WIDGETS           |
| CATALOG.XPATH_EXPRESSIONS |
| CATALOG.PROJECTS          |
| CATALOG.SNAPSHOTS         |
| CATALOG.OBJECTS           |
```

### Querying Catalog Tables

```sql
-- Find all entities in a specific module
select Name, QualifiedName, EntityType
from CATALOG.ENTITIES
where ModuleName = 'MyModule';

-- Find microflows that contain Java actions
select distinct m.QualifiedName, m.Description
from CATALOG.MICROFLOWS m
join CATALOG.ACTIVITIES a on m.Id = a.MicroflowId
where a.ActivityType = 'JavaActionCallAction';

-- Find pages using a specific entity
select p.QualifiedName, w.WidgetType
from CATALOG.PAGES p
join CATALOG.WIDGETS w on p.Id = w.ContainerId
where w.EntityRef like '%Customer%';

-- Count activities by type across project
select ActivityType, count(*) as count
from CATALOG.ACTIVITIES
GROUP by ActivityType
ORDER by count desc;

-- Find complex XPath expressions
select DocumentQualifiedName, XPathExpression
from CATALOG.XPATH_EXPRESSIONS
where length(XPathExpression) > 50;
```

## Catalog Table Schema

### CATALOG.MODULES

| Column           | Type    | Description                        |
|------------------|---------|------------------------------------|
| Id               | TEXT    | Module UUID                        |
| Name             | TEXT    | Module name                        |
| QualifiedName    | TEXT    | Same as Name for modules           |
| Description      | TEXT    | Module documentation               |
| IsSystemModule   | INTEGER | 1 if system/marketplace module     |
| AppStoreVersion  | TEXT    | Marketplace version if applicable  |
| AppStoreGuid     | TEXT    | Marketplace GUID                   |
| ProjectId        | TEXT    | Parent project ID                  |
| SnapshotId       | TEXT    | Snapshot this data belongs to      |
| SnapshotDate     | TEXT    | ISO timestamp of snapshot          |

### CATALOG.ENTITIES

| Column        | Type    | Description                              |
|---------------|---------|------------------------------------------|
| Id            | TEXT    | Entity UUID                              |
| Name          | TEXT    | Entity name                              |
| QualifiedName | TEXT    | Module.Entity                            |
| ModuleName    | TEXT    | Parent module name                       |
| Folder        | TEXT    | Folder path within module                |
| EntityType    | TEXT    | PERSISTENT, NON_PERSISTENT, EXTERNAL     |
| Description   | TEXT    | Entity documentation                     |
| Generalization| TEXT    | Parent entity qualified name             |
| AttributeCount| INTEGER | Number of attributes                     |
| AssociationCount| INTEGER | Number of associations                 |
| ProjectId     | TEXT    | Parent project ID                        |
| SnapshotId    | TEXT    | Snapshot ID                              |
| SnapshotDate  | TEXT    | Snapshot timestamp                       |

### CATALOG.MICROFLOWS

| Column         | Type    | Description                             |
|----------------|---------|-----------------------------------------|
| Id             | TEXT    | Microflow UUID                          |
| Name           | TEXT    | Microflow name                          |
| QualifiedName  | TEXT    | Module.Microflow                        |
| ModuleName     | TEXT    | Parent module name                      |
| Folder         | TEXT    | Folder path within module               |
| MicroflowType  | TEXT    | MICROFLOW or NANOFLOW                   |
| Description    | TEXT    | Documentation                           |
| ReturnType     | TEXT    | Return type (Entity, Boolean, etc.)     |
| ParameterCount | INTEGER | Number of parameters                    |
| ActivityCount  | INTEGER | Number of activities                    |
| ProjectId      | TEXT    | Parent project ID                       |
| SnapshotId     | TEXT    | Snapshot ID                             |
| SnapshotDate   | TEXT    | Snapshot timestamp                      |

### CATALOG.NANOFLOWS

Same schema as MICROFLOWS, filtered for nanoflows only.

### CATALOG.PAGES

| Column        | Type    | Description                              |
|---------------|---------|------------------------------------------|
| Id            | TEXT    | Page UUID                                |
| Name          | TEXT    | Page name                                |
| QualifiedName | TEXT    | Module.Page                              |
| ModuleName    | TEXT    | Parent module name                       |
| Folder        | TEXT    | Folder path within module                |
| Title         | TEXT    | Page title                               |
| URL           | TEXT    | Page URL                                 |
| LayoutRef     | TEXT    | Layout qualified name                    |
| Description   | TEXT    | Page documentation                       |
| ParameterCount| INTEGER | Number of page parameters                |
| WidgetCount   | INTEGER | Number of widgets                        |
| ProjectId     | TEXT    | Parent project ID                        |
| SnapshotId    | TEXT    | Snapshot ID                              |
| SnapshotDate  | TEXT    | Snapshot timestamp                       |

### CATALOG.SNIPPETS

| Column        | Type    | Description                              |
|---------------|---------|------------------------------------------|
| Id            | TEXT    | Snippet UUID                             |
| Name          | TEXT    | Snippet name                             |
| QualifiedName | TEXT    | Module.Snippet                           |
| ModuleName    | TEXT    | Parent module name                       |
| Folder        | TEXT    | Folder path within module                |
| Description   | TEXT    | Snippet documentation                    |
| ParameterCount| INTEGER | Number of snippet parameters             |
| WidgetCount   | INTEGER | Number of widgets                        |
| ProjectId     | TEXT    | Parent project ID                        |
| SnapshotId    | TEXT    | Snapshot ID                              |
| SnapshotDate  | TEXT    | Snapshot timestamp                       |

### CATALOG.ENUMERATIONS

| Column        | Type    | Description                              |
|---------------|---------|------------------------------------------|
| Id            | TEXT    | Enumeration UUID                         |
| Name          | TEXT    | Enumeration name                         |
| QualifiedName | TEXT    | Module.Enumeration                       |
| ModuleName    | TEXT    | Parent module name                       |
| Folder        | TEXT    | Folder path within module                |
| Description   | TEXT    | Enumeration documentation                |
| ValueCount    | INTEGER | Number of enumeration values             |
| ProjectId     | TEXT    | Parent project ID                        |
| SnapshotId    | TEXT    | Snapshot ID                              |
| SnapshotDate  | TEXT    | Snapshot timestamp                       |

### CATALOG.ACTIVITIES

| Column                 | Type    | Description                        |
|------------------------|---------|------------------------------------|
| Id                     | TEXT    | Activity UUID                      |
| Name                   | TEXT    | Activity type name                 |
| Caption                | TEXT    | Display caption                    |
| ActivityType           | TEXT    | TypeName (e.g., ActionActivity)    |
| MicroflowId            | TEXT    | Parent microflow UUID              |
| MicroflowQualifiedName | TEXT    | Parent microflow qualified name    |
| ModuleName             | TEXT    | Module name                        |
| Folder                 | TEXT    | Folder path                        |
| EntityRef              | TEXT    | Referenced entity (if applicable)  |
| ActionType             | TEXT    | Specific action (Create, Retrieve) |
| Description            | TEXT    | Activity documentation             |
| ProjectId              | TEXT    | Parent project ID                  |
| SnapshotId             | TEXT    | Snapshot ID                        |
| SnapshotDate           | TEXT    | Snapshot timestamp                 |

### CATALOG.WIDGETS

| Column                 | Type    | Description                        |
|------------------------|---------|------------------------------------|
| Id                     | TEXT    | Widget UUID                        |
| Name                   | TEXT    | Widget name                        |
| WidgetType             | TEXT    | Widget type (DataGrid, TextBox)    |
| ContainerId            | TEXT    | Parent page/snippet UUID           |
| ContainerQualifiedName | TEXT    | Parent qualified name              |
| ContainerType          | TEXT    | PAGE or SNIPPET                    |
| ModuleName             | TEXT    | Module name                        |
| Folder                 | TEXT    | Folder path                        |
| EntityRef              | TEXT    | Data source entity                 |
| AttributeRef           | TEXT    | Bound attribute                    |
| Description            | TEXT    | Widget documentation               |
| ProjectId              | TEXT    | Parent project ID                  |
| SnapshotId             | TEXT    | Snapshot ID                        |
| SnapshotDate           | TEXT    | Snapshot timestamp                 |

### CATALOG.XPATH_EXPRESSIONS

| Column                 | Type    | Description                        |
|------------------------|---------|------------------------------------|
| Id                     | TEXT    | Generated ID                       |
| DocumentType           | TEXT    | MICROFLOW, PAGE, etc.              |
| DocumentId             | TEXT    | Parent document UUID               |
| DocumentQualifiedName  | TEXT    | Parent document qualified name     |
| ComponentType          | TEXT    | RETRIEVE_ACTION, DATASOURCE, etc.  |
| ComponentId            | TEXT    | Component UUID                     |
| ComponentName          | TEXT    | Component name                     |
| XPathExpression        | TEXT    | Raw XPath string                   |
| XPathAST               | TEXT    | Parsed XPath as JSON               |
| TargetEntity           | TEXT    | Entity being queried               |
| ReferencedEntities     | TEXT    | JSON array of referenced entities  |
| IsParameterized        | INTEGER | 1 if contains parameters           |
| UsageType              | TEXT    | RETRIEVE, CONSTRAINT, FILTER       |
| ModuleName             | TEXT    | Module name                        |
| Folder                 | TEXT    | Folder path                        |
| ProjectId              | TEXT    | Parent project ID                  |
| SnapshotId             | TEXT    | Snapshot ID                        |
| SnapshotDate           | TEXT    | Snapshot timestamp                 |

### CATALOG.PROJECTS

| Column           | Type    | Description                        |
|------------------|---------|------------------------------------|
| ProjectId        | TEXT    | Project identifier                 |
| ProjectName      | TEXT    | Project name                       |
| MendixVersion    | TEXT    | Mendix version                     |
| CreatedDate      | TEXT    | Catalog creation timestamp         |
| LastSnapshotDate | TEXT    | Most recent snapshot               |
| SnapshotCount    | INTEGER | Number of snapshots                |

### CATALOG.SNAPSHOTS

| Column          | Type    | Description                         |
|-----------------|---------|-------------------------------------|
| SnapshotId      | TEXT    | Snapshot identifier                 |
| SnapshotName    | TEXT    | Optional name                       |
| ProjectId       | TEXT    | Parent project ID                   |
| ProjectName     | TEXT    | Project name                        |
| SnapshotDate    | TEXT    | Snapshot timestamp                  |
| SnapshotSource  | TEXT    | LIVE, GIT, IMPORT                   |
| SourceId        | TEXT    | Git commit or import ID             |
| SourceBranch    | TEXT    | Git branch name                     |
| SourceRevision  | TEXT    | Git revision                        |
| ObjectCount     | INTEGER | Total objects in snapshot           |
| IsActive        | INTEGER | 1 if current active snapshot        |

### CATALOG.OBJECTS

Union view of all object types for generic querying:

| Column        | Type    | Description                              |
|---------------|---------|------------------------------------------|
| Id            | TEXT    | Object UUID                              |
| ObjectType    | TEXT    | MODULE, ENTITY, MICROFLOW, PAGE, etc.    |
| Name          | TEXT    | Object name                              |
| QualifiedName | TEXT    | Full qualified name                      |
| ModuleName    | TEXT    | Parent module (empty for modules)        |
| Folder        | TEXT    | Folder path                              |
| Description   | TEXT    | Documentation                            |
| ProjectId     | TEXT    | Parent project ID                        |
| SnapshotId    | TEXT    | Snapshot ID                              |
| SnapshotDate  | TEXT    | Snapshot timestamp                       |

## Architecture

### Component Structure

```
mdl/
├── catalog/
│   ├── catalog.go           # Main catalog struct and interface
│   ├── builder.go           # Builds catalog from MPR data
│   ├── tables.go            # table definitions and schemas
│   ├── query.go             # sql query execution
│   └── snapshot.go          # Snapshot management
│
├── executor/
│   ├── cmd_catalog.go       # show catalog tables, select handlers
│   └── ...
│
└── grammar/
    └── MDLParser.g4         # add select statement grammar
```

### Catalog Interface

```go
package catalog

import (
    "database/sql"
    "time"
)

// catalog provides sql querying over Mendix project metadata.
type catalog struct {
    db          *sql.DB        // in-memory SQLite
    projectID   string
    projectName string
    snapshots   map[string]*Snapshot
    activeSnap  string
}

// Snapshot represents a point-in-time view of project data.
type Snapshot struct {
    ID         string
    Name       string
    date       time.Time
    source     SnapshotSource
    SourceID   string
    branch     string
    Revision   string
    ObjectCount int
}

type SnapshotSource string

const (
    SnapshotSourceLive    SnapshotSource = "LIVE"
    SnapshotSourceGit     SnapshotSource = "GIT"
    SnapshotSourceImport  SnapshotSource = "import"
)

// New creates a new catalog with an in-memory SQLite database.
func New() (*catalog, error)

// build populates the catalog from MPR reader data.
func (c *catalog) build(reader *mpr.Reader, progress func(table string, count int)) error

// query executes a sql query and returns results.
func (c *catalog) query(sql string) (*QueryResult, error)

// tables returns the list of available catalog tables.
func (c *catalog) tables() []string

// CreateSnapshot creates a named snapshot of current state.
func (c *catalog) CreateSnapshot(name string, source SnapshotSource) (*Snapshot, error)

// close releases catalog resources.
func (c *catalog) close() error

// QueryResult holds query results.
type QueryResult struct {
    columns []string
    Rows    [][]interface{}
    count   int
}
```

### Builder Implementation

```go
package catalog

// Builder populates catalog tables from MPR data.
type Builder struct {
    catalog *catalog
    reader  *mpr.Reader
    snapshot *Snapshot
}

func (b *Builder) build(progress func(table string, count int)) error {
    // build tables in dependency order
    if err := b.buildModules(progress); err != nil {
        return err
    }
    if err := b.buildEntities(progress); err != nil {
        return err
    }
    if err := b.buildMicroflows(progress); err != nil {
        return err
    }
    if err := b.buildNanoflows(progress); err != nil {
        return err
    }
    if err := b.buildPages(progress); err != nil {
        return err
    }
    if err := b.buildSnippets(progress); err != nil {
        return err
    }
    if err := b.buildEnumerations(progress); err != nil {
        return err
    }
    if err := b.buildActivities(progress); err != nil {
        return err
    }
    if err := b.buildWidgets(progress); err != nil {
        return err
    }
    if err := b.buildXPathExpressions(progress); err != nil {
        return err
    }
    if err := b.buildObjectsView(progress); err != nil {
        return err
    }
    return nil
}

func (b *Builder) buildModules(progress func(string, int)) error {
    modules, err := b.reader.ListModules()
    if err != nil {
        return err
    }

    stmt, err := b.catalog.db.Prepare(`
        insert into modules (Id, Name, QualifiedName, description,
            IsSystemModule, AppStoreVersion, AppStoreGuid,
            ProjectId, SnapshotId, SnapshotDate)
        values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, m := range modules {
        isSystem := 0
        if m.FromAppStore {
            isSystem = 1
        }
        _, err := stmt.Exec(
            m.ID, m.Name, m.Name, m.Documentation,
            isSystem, m.AppStoreVersion, m.AppStoreGuid,
            b.snapshot.ProjectID, b.snapshot.ID, b.snapshot.Date,
        )
        if err != nil {
            return err
        }
    }

    progress("modules", len(modules))
    return nil
}

// Similar methods for other tables...
```

### Grammar Extensions

Add to MDLParser.g4:

```antlr
// catalog statements
catalogStatement
    : show catalog tables
    | selectStatement
    ;

selectStatement
    : select selectColumns from catalogTable
      (where whereClause)?
      (GROUP by groupByClause)?
      (ORDER by orderByClause)?
      (limit limitClause)?
    ;

catalogTable
    : catalog DOT IDENTIFIER
    ;

selectColumns
    : STAR
    | selectColumn (COMMA selectColumn)*
    ;

selectColumn
    : expression (as? IDENTIFIER)?
    ;

// sql expressions (subset)
expression
    : IDENTIFIER
    | IDENTIFIER DOT IDENTIFIER
    | STRING_LITERAL
    | NUMBER
    | expression (PLUS | MINUS | STAR | SLASH) expression
    | expression (EQ | NE | LT | GT | LE | GE) expression
    | expression (and | or) expression
    | expression like expression
    | expression in LPAREN expressionList RPAREN
    | functionCall
    | LPAREN expression RPAREN
    ;

functionCall
    : IDENTIFIER LPAREN expressionList? RPAREN
    ;
```

### Executor Integration

```go
// cmd_catalog.go
package executor

func (e *Executor) execShowCatalogTables() error {
    // build catalog if not already built
    if e.catalog == nil {
        cat, err := catalog.New()
        if err != nil {
            return err
        }
        e.catalog = cat

        fmt.Fprintln(e.output, "Building catalog...")
        err = e.catalog.Build(e.reader, func(table string, count int) {
            fmt.Fprintf(e.output, "✓ %s: %d\n", table, count)
        })
        if err != nil {
            return err
        }
    }

    tables := e.catalog.Tables()
    fmt.Fprintf(e.output, "\nFound %d catalog table(s)\n", len(tables))

    // Output table list
    e.outputTable([]string{"table"}, tables)
    return nil
}

func (e *Executor) execSelect(stmt *ast.SelectStmt) error {
    // Ensure catalog is built
    if e.catalog == nil {
        return fmt.Errorf("catalog not built - run show catalog tables first")
    }

    // Convert AST to sql string
    sql := e.selectToSQL(stmt)

    // execute query
    result, err := e.catalog.Query(sql)
    if err != nil {
        return fmt.Errorf("failed to execute catalog query: %w", err)
    }

    // Output results
    fmt.Fprintf(e.output, "found %d result(s)\n", result.Count)
    if result.Count == 0 {
        fmt.Fprintln(e.output, "(no results)")
        return nil
    }

    e.outputQueryResults(result)
    return nil
}
```

## Implementation Plan

### Phase 1: Core Infrastructure
- Create `mdl/catalog/` package structure
- Implement in-memory SQLite setup with table schemas
- Add basic `show catalog tables` command

### Phase 2: Table Builders
- Implement builders for each table type
- Add progress reporting during build
- Handle cross-references (activities→microflows, widgets→pages)

### Phase 3: Query Support
- Add SELECT statement to grammar
- Implement SQL generation from AST
- Add query execution and result formatting

### Phase 4: Advanced Features
- Snapshot support for versioning
- CATALOG.OBJECTS union view
- XPath expression parsing and AST storage

### Phase 5: Optimizations
- Lazy loading of detailed data
- Caching of frequently accessed data
- Index optimization for common queries

## Use Cases

### 1. Find All Usages of an Entity

```sql
-- Find all microflows that use Customer entity
select distinct m.QualifiedName, a.ActivityType, a.Caption
from CATALOG.MICROFLOWS m
join CATALOG.ACTIVITIES a on m.Id = a.MicroflowId
where a.EntityRef like '%Customer%'
ORDER by m.QualifiedName;

-- Find all pages with Customer data
select p.QualifiedName, w.WidgetType, w.Name
from CATALOG.PAGES p
join CATALOG.WIDGETS w on p.Id = w.ContainerId
where w.EntityRef like '%Customer%';
```

### 2. Code Quality Analysis

```sql
-- Find microflows with many activities (complexity)
select QualifiedName, ActivityCount
from CATALOG.MICROFLOWS
where ActivityCount > 20
ORDER by ActivityCount desc;

-- Find pages with many widgets
select QualifiedName, WidgetCount
from CATALOG.PAGES
where WidgetCount > 50
ORDER by WidgetCount desc;
```

### 3. Documentation Coverage

```sql
-- Find undocumented microflows
select QualifiedName
from CATALOG.MICROFLOWS
where description IS null or description = '';

-- Find documented entities
select QualifiedName, description
from CATALOG.ENTITIES
where description IS not null and description != '';
```

### 4. Agentic Exploration

AI agents can use catalog queries to understand project structure:

```sql
-- Get project overview
select ObjectType, count(*) as count
from CATALOG.OBJECTS
GROUP by ObjectType;

-- Find entry points (pages without parameters)
select QualifiedName, title, url
from CATALOG.PAGES
where ParameterCount = 0 and url IS not null;

-- Find external integrations
select QualifiedName, ActivityType
from CATALOG.ACTIVITIES
where ActivityType in ('RestCallAction', 'WebServiceCallAction', 'JavaActionCallAction');
```

## Dependencies

- `github.com/mattn/go-sqlite3` - Already a project dependency (used for MPR v1)
- No additional dependencies required

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Memory usage for large projects | Lazy loading, streaming results |
| Build time for 1000+ objects | Progress indicators, parallel building |
| SQL injection in queries | Parameterized queries where possible |
| Schema changes breaking queries | Version catalog schema, migration support |

## Success Criteria

1. All 13 catalog tables populated correctly
2. Standard SQL queries execute successfully
3. Build time < 5 seconds for typical projects
4. Memory usage < 100MB for large projects
5. Query results match expected TypeScript output
