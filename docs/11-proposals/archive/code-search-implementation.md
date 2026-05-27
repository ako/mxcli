---
title: mxcli Code Search Extension - Implementation Proposal
status: done
---

# mxcli Code Search Extension - Implementation Proposal

## Executive Summary

This proposal extends mxcli with code search capabilities to help Claude Code understand and navigate Mendix projects. Based on analysis of the existing codebase, this can be implemented by:

1. Adding a `catalog.references` table to track cross-element relationships
2. Extending the existing catalog builder to extract references during indexing
3. Adding new MDL statements for search queries
4. Adding CLI subcommands for direct search access
5. Implementing context assembly for LLM consumption

## Current State Analysis

### Existing Infrastructure (What We Already Have)

**Catalog System** (`mdl/catalog/`):
- In-memory SQLite database with 13 tables
- Fast mode (metadata only) and full mode (activities/widgets)
- SQL query interface via `select ... from CATALOG.xxx`
- Snapshot system for point-in-time comparisons

**Current Reference Tracking**:
- `activities.MicroflowId` - links activities to their containing microflow
- `activities.EntityRef` - entity referenced by CreateObjectAction
- `widgets.ContainerId` - links widgets to pages/snippets
- `widgets.EntityRef`, `widgets.AttributeRef` - data binding references
- `entities.Generalization` - parent entity reference

**What's Missing**:
- Microflow-to-microflow call references
- Page-to-layout references
- Entity event handlers → microflow references
- Association → target entity references
- Widget action → page/microflow references
- Unified reference query interface

### Key Files

| Component | Location | Purpose |
|-----------|----------|---------|
| Catalog schema | `mdl/catalog/tables.go` | Table definitions |
| Catalog builder | `mdl/catalog/builder*.go` | Populates tables |
| MDL grammar | `mdl/grammar/MDLParser.g4`, `MDLLexer.g4` | Syntax definition |
| AST nodes | `mdl/ast/ast_query.go` | Statement types |
| Executor | `mdl/executor/executor.go` | Statement dispatch |
| CLI | `cmd/mxcli/main.go` | Cobra commands |
| Microflow actions | `sdk/microflows/microflows_actions.go` | Action types with references |
| Page widgets | `sdk/pages/pages_widgets*.go` | Widget types with references |

---

## Proposed Implementation

### 1. Schema Changes

#### New References Table

Add to `mdl/catalog/tables.go`:

```sql
create table if not exists references (
    Id text primary key,
    SourceType text not null,       -- 'microflow', 'page', 'entity', 'widget', etc.
    SourceId text not null,         -- UUID of the source element
    SourceName text not null,       -- Qualified name (e.g., 'Module.Microflow')
    SourceLocation text,            -- Optional: activity/widget ID within document
    TargetType text not null,       -- 'microflow', 'entity', 'page', 'attribute', etc.
    TargetId text,                  -- UUID of target (if available)
    TargetName text not null,       -- Qualified name of referenced thing
    RefKind text not null,          -- 'call', 'retrieve', 'change', 'create', etc.
    -- Snapshot metadata (same as other tables)
    ProjectId text,
    ProjectName text,
    SnapshotId text,
    SnapshotDate text,
    SnapshotSource text,
    SourceId text,
    SourceBranch text,
    SourceRevision text
);

create index if not exists idx_refs_target on references(TargetType, TargetName);
create index if not exists idx_refs_source on references(SourceType, SourceName);
create index if not exists idx_refs_kind on references(RefKind);
create index if not exists idx_refs_source_id on references(SourceId);
create index if not exists idx_refs_target_id on references(TargetId);
```

#### Convenience Views

```sql
-- Microflow call graph
create view if not exists call_graph as
select
    SourceName as Caller,
    TargetName as Callee
from references
where SourceType = 'microflow'
  and TargetType = 'microflow'
  and RefKind = 'call';

-- Entity usage view
create view if not exists entity_usage as
select
    TargetName as entity,
    SourceType,
    SourceName,
    RefKind
from references
where TargetType = 'entity';

-- Page references view
create view if not exists page_refs as
select
    SourceName as source,
    TargetName as page,
    RefKind
from references
where TargetType = 'page';
```

### 2. Reference Extraction

#### Reference Kinds to Extract

| Source Type | Reference Kind | Target Type | Example |
|-------------|----------------|-------------|---------|
| microflow | call | microflow | MicroflowCallAction |
| microflow | create | entity | CreateObjectAction |
| microflow | change | entity | ChangeObjectAction |
| microflow | retrieve | entity | RetrieveAction |
| microflow | delete | entity | DeleteObjectAction |
| microflow | show_page | page | ShowPageAction |
| microflow | close_page | page | ClosePageAction |
| microflow | parameter_type | entity | Parameter with Object/List type |
| microflow | return_type | entity | Return type Object/List |
| page | layout | layout | LayoutCall |
| page | data_source | entity | DataView/ListView entity |
| page | data_source | microflow | MicroflowSource |
| page | action | microflow | MicroflowClientAction |
| page | action | page | PageClientAction |
| widget | attribute | attribute | TextBox attribute binding |
| widget | action | microflow | Button on-click action |
| entity | generalization | entity | Parent entity |
| entity | event_handler | microflow | EventHandler |
| entity | association | entity | Association target |
| enumeration | attribute_type | entity | EnumerationAttributeType |

#### Builder Extensions

**New file: `mdl/catalog/builder_references.go`**

```go
package catalog

import (
    "database/sql"
    "github.com/mendixlabs/mxcli/model"
    "github.com/mendixlabs/mxcli/sdk/microflows"
    "github.com/mendixlabs/mxcli/sdk/pages"
)

// addReference adds a reference to the database
func (b *Builder) addReference(stmt *sql.Stmt,
    sourceType, sourceID, sourceName, sourceLocation,
    targetType, targetID, targetName, refKind string) error {

    projectID, projectName, snapshotID, snapshotDate, snapshotSource,
        srcID, srcBranch, srcRevision := b.snapshotMeta()

    _, err := stmt.Exec(
        model.GenerateID(), // unique ID for reference record
        sourceType, sourceID, sourceName, sourceLocation,
        targetType, targetID, targetName, refKind,
        projectID, projectName, snapshotID, snapshotDate, snapshotSource,
        srcID, srcBranch, srcRevision,
    )
    return err
}

// extractMicroflowReferences extracts all references from a microflow
func (b *Builder) extractMicroflowReferences(stmt *sql.Stmt,
    mf *microflows.Microflow, qualifiedName string) error {

    sourceID := string(mf.ID)

    // parameter types
    for _, param := range mf.Parameters {
        if entityName := getEntityFromDataType(param.ParameterType); entityName != "" {
            b.addReference(stmt, "microflow", sourceID, qualifiedName, "",
                "entity", "", entityName, "parameter_type")
        }
    }

    // return type
    if entityName := getEntityFromDataType(mf.ReturnType); entityName != "" {
        b.addReference(stmt, "microflow", sourceID, qualifiedName, "",
            "entity", "", entityName, "return_type")
    }

    // actions
    if mf.ObjectCollection != nil {
        for _, obj := range mf.ObjectCollection.Objects {
            b.extractActionReferences(stmt, obj, sourceID, qualifiedName)
        }
    }

    return nil
}

// extractActionReferences extracts references from microflow actions
func (b *Builder) extractActionReferences(stmt *sql.Stmt,
    obj microflows.MicroflowObject, sourceID, sourceName string) {

    activityID := string(obj.GetID())

    if act, ok := obj.(*microflows.ActionActivity); ok && act.Action != nil {
        switch a := act.Action.(type) {
        case *microflows.MicroflowCallAction:
            // microflow call reference
            if a.MicroflowQualifiedName != "" {
                b.addReference(stmt, "microflow", sourceID, sourceName, activityID,
                    "microflow", string(a.MicroflowID), a.MicroflowQualifiedName, "call")
            }

        case *microflows.CreateObjectAction:
            if a.EntityQualifiedName != "" {
                b.addReference(stmt, "microflow", sourceID, sourceName, activityID,
                    "entity", string(a.EntityID), a.EntityQualifiedName, "create")
            }

        case *microflows.ChangeObjectAction:
            if a.EntityQualifiedName != "" {
                b.addReference(stmt, "microflow", sourceID, sourceName, activityID,
                    "entity", "", a.EntityQualifiedName, "change")
            }

        case *microflows.RetrieveAction:
            if src, ok := a.RetrieveSource.(*microflows.DatabaseRetrieveSource); ok {
                if src.EntityQualifiedName != "" {
                    b.addReference(stmt, "microflow", sourceID, sourceName, activityID,
                        "entity", string(src.EntityID), src.EntityQualifiedName, "retrieve")
                }
            }

        case *microflows.ShowPageAction:
            if a.PageQualifiedName != "" {
                b.addReference(stmt, "microflow", sourceID, sourceName, activityID,
                    "page", string(a.PageID), a.PageQualifiedName, "show_page")
            }

        // add more action types as needed...
        }
    }
}

// getEntityFromDataType extracts entity name from a data type
func getEntityFromDataType(dt microflows.DataType) string {
    switch t := dt.(type) {
    case *microflows.ObjectType:
        return t.EntityQualifiedName
    case *microflows.ListType:
        return t.EntityQualifiedName
    }
    return ""
}
```

**Extend `builder_microflows.go`**:
- Add reference extraction after processing each microflow
- Only in full mode (references require deep parsing)

**New file: `mdl/catalog/builder_pages_refs.go`**:
- Extract page → layout references
- Extract widget → entity/microflow references
- Extract action → page/microflow references

**Extend `builder_modules.go`**:
- Extract entity → generalization references
- Extract entity → event handler microflow references
- Extract association → target entity references

### 3. MDL Statement Extensions

#### New SEARCH Statement

**Grammar additions** (`MDLParser.g4`):

```antlr
searchStatement
    : search (searchType)? searchPattern (searchOptions)*
    ;

searchType
    : entity | microflow | page | references | callers | callees
    ;

searchPattern
    : STRING_LITERAL
    | qualifiedName
    ;

searchOptions
    : in qualifiedName
    | where expression
    | transitive
    | limit INTEGER_LITERAL
    ;

// Keywords
search: S E A R C H;
callers: C A L L E R S;
callees: C A L L E E S;
references: R E F E R E N C E S;
transitive: T R A N S I T I V E;
```

**AST types** (`mdl/ast/ast_search.go`):

```go
package ast

type SearchType int

const (
    SearchAll SearchType = iota
    SearchEntity
    SearchMicroflow
    SearchPage
    SearchReferences
    SearchCallers
    SearchCallees
)

type SearchStmt struct {
    SearchType   SearchType
    pattern      string
    InModule     string
    WhereClause  string
    transitive   bool
    limit        int
}

func (s *SearchStmt) isStatement() {}
```

**Executor** (`mdl/executor/cmd_search.go`):

```go
package executor

import (
    "fmt"
    "strings"
    "github.com/mendixlabs/mxcli/mdl/ast"
)

func (e *Executor) execSearch(s *ast.SearchStmt) error {
    switch s.SearchType {
    case ast.SearchCallers:
        return e.searchCallers(s.Pattern, s.Transitive, s.Limit)
    case ast.SearchCallees:
        return e.searchCallees(s.Pattern, s.Transitive, s.Limit)
    case ast.SearchReferences:
        return e.searchReferences(s.Pattern, s.InModule, s.Limit)
    default:
        return e.searchSymbols(s)
    }
}

func (e *Executor) searchCallers(target string, transitive bool, limit int) error {
    if transitive {
        // Recursive CTE for transitive callers
        query := `
            with RECURSIVE callers_cte as (
                select Caller, 1 as depth
                from call_graph
                where Callee = ?
                union all
                select cg.Caller, c.Depth + 1
                from call_graph cg
                join callers_cte c on cg.Callee = c.Caller
                where c.Depth < 10
            )
            select distinct Caller, min(depth) as depth
            from callers_cte
            GROUP by Caller
            ORDER by depth, Caller
        `
        if limit > 0 {
            query += fmt.Sprintf(" limit %d", limit)
        }
        return e.execCatalogQuery(query, target)
    }

    // Direct callers only
    query := `select Caller from call_graph where Callee = ?`
    if limit > 0 {
        query += fmt.Sprintf(" limit %d", limit)
    }
    return e.execCatalogQuery(query, target)
}

func (e *Executor) searchCallees(source string, transitive bool, limit int) error {
    // Similar to searchCallers but reversed
    // ...
}

func (e *Executor) searchReferences(target, module string, limit int) error {
    query := `
        select SourceType, SourceName, RefKind
        from references
        where TargetName = ? or TargetName like ?
    `
    args := []interface{}{target, "%" + target}

    if module != "" {
        query += " and SourceName like ?"
        args = append(args, module+".%")
    }

    query += " ORDER by RefKind, SourceType, SourceName"
    if limit > 0 {
        query += fmt.Sprintf(" limit %d", limit)
    }

    return e.execCatalogQueryWithArgs(query, args...)
}
```

#### MDL Function Syntax (Alternative)

For REPL usage, add function-style queries:

```ruby
# find callers
show callers of Module.MicroflowName
show callers of Module.MicroflowName transitive

# find callees
show callees of Module.MicroflowName

# find references
show references to Module.EntityName
show references to Module.EntityName.AttributeName

# impact analysis
show impact of Module.EntityName
```

### 4. CLI Commands

**Add to `cmd/mxcli/main.go`**:

```go
var searchCmd = &cobra.Command{
    use:   "search <pattern>",
    Short: "search for elements in the project",
    long: `search for modules, entities, microflows, pages, etc.

Examples:
  mxcli search -p app.mpr Customer
  mxcli search -p app.mpr --type entity Order
  mxcli search -p app.mpr --callers Module.ValidateOrder
  mxcli search -p app.mpr --callees Module.ProcessOrder --transitive
  mxcli search -p app.mpr --refs Module.Customer
`,
    Args: cobra.MinimumNArgs(1),
    run: func(cmd *cobra.Command, args []string) {
        // Implementation
    },
}

func init() {
    searchCmd.Flags().StringP("type", "t", "", "Element type: entity, microflow, page")
    searchCmd.Flags().Bool("callers", false, "find callers of a microflow")
    searchCmd.Flags().Bool("callees", false, "find callees of a microflow")
    searchCmd.Flags().Bool("refs", false, "find references to an element")
    searchCmd.Flags().Bool("transitive", false, "Include transitive references")
    searchCmd.Flags().IntP("limit", "l", 50, "maximum results")

    rootCmd.AddCommand(searchCmd)
}
```

**Additional search subcommands**:

```go
// mxcli callers <microflow> [--transitive]
var callersCmd = &cobra.Command{...}

// mxcli callees <microflow> [--transitive]
var calleesCmd = &cobra.Command{...}

// mxcli refs <element> [--type entity|microflow|page]
var refsCmd = &cobra.Command{...}

// mxcli impact <element>
var impactCmd = &cobra.Command{...}

// mxcli context <element> [--depth N] [--max-tokens N]
var contextCmd = &cobra.Command{...}
```

### 5. Context Assembly Feature

The `context` command assembles relevant information for LLM consumption.

**New file: `mdl/executor/cmd_context.go`**:

```go
package executor

import (
    "fmt"
    "strings"
)

type ContextAssembler struct {
    executor  *Executor
    maxTokens int
    depth     int
    visited   map[string]bool
    output    strings.Builder
}

func (e *Executor) assembleContext(target string, depth, maxTokens int) (string, error) {
    ca := &ContextAssembler{
        executor:  e,
        maxTokens: maxTokens,
        depth:     depth,
        visited:   make(map[string]bool),
    }

    return ca.assemble(target)
}

func (ca *ContextAssembler) assemble(target string) (string, error) {
    // 1. Determine target type (microflow, entity, page)
    targetType, err := ca.detectType(target)
    if err != nil {
        return "", err
    }

    // 2. get the target element definition
    ca.addSection("Target: " + target)
    ca.addDefinition(target, targetType)

    // 3. get related elements based on type
    switch targetType {
    case "microflow":
        ca.assembleMicroflowContext(target)
    case "entity":
        ca.assembleEntityContext(target)
    case "page":
        ca.assemblePageContext(target)
    }

    return ca.output.String(), nil
}

func (ca *ContextAssembler) assembleMicroflowContext(name string) {
    // add entities used (create, change, retrieve)
    ca.addSection("entities Used")
    ca.addQueryResults(`
        select distinct TargetName, RefKind
        from references
        where SourceName = ? and TargetType = 'entity'
    `, name)

    // add called microflows (up to depth)
    ca.addSection("Called microflows")
    ca.addCallees(name, ca.depth)

    // add direct callers (limited)
    ca.addSection("Direct callers (sample)")
    ca.addQueryResults(`
        select SourceName from references
        where TargetName = ? and RefKind = 'call'
        limit 5
    `, name)

    // add parameter/return types
    ca.addSection("Signature")
    ca.addMicroflowSignature(name)
}

func (ca *ContextAssembler) assembleEntityContext(name string) {
    // add entity definition with attributes
    ca.addSection("entity Definition")
    ca.addEntityDefinition(name)

    // add microflows that use this entity
    ca.addSection("microflows Using This entity")
    ca.addQueryResults(`
        select distinct SourceName, RefKind
        from references
        where TargetName = ? and SourceType = 'microflow'
        ORDER by RefKind, SourceName
        limit 20
    `, name)

    // add pages that display this entity
    ca.addSection("pages Displaying This entity")
    ca.addQueryResults(`
        select distinct SourceName
        from references
        where TargetName = ? and SourceType = 'page'
        limit 10
    `, name)
}
```

**CLI command**:

```bash
# Assemble context for a microflow
mxcli context -p app.mpr Module.ProcessOrder --depth 2 --max-tokens 4000

# Output format (markdown):
# ## Target: Module.ProcessOrder
#
# ### Definition
# microflow Module.ProcessOrder(Order: MyModule.Order) returns boolean
#   ...microflow definition...
#
# ### entities Used
# | entity | Usage |
# |--------|-------|
# | MyModule.Order | retrieve, change |
# | MyModule.OrderLine | create |
#
# ### Called microflows
# - Module.ValidateOrder (depth 1)
# - Module.CalculateTotal (depth 1)
# - Module.SendNotification (depth 2)
#
# ### Direct callers
# - Module.ACT_SubmitOrder
# - Module.ACT_ReprocessOrder
```

### 6. Optional LSP Server (Phase 2)

For IDE integration, add an LSP server mode:

**New package: `mdl/lsp/`**

```go
package lsp

import (
    "github.com/sourcegraph/go-lsp"
    "github.com/sourcegraph/jsonrpc2"
)

type Server struct {
    catalog *catalog.Catalog
}

// Supported LSP methods:
// - textDocument/definition
// - textDocument/references
// - textDocument/hover
// - workspace/symbol
```

**CLI command**:

```bash
mxcli lsp              # stdio mode
mxcli lsp --tcp :9257  # TCP mode
```

This is deferred to Phase 2 as it requires additional complexity and VS Code extension development.

---

## Implementation Plan

### Phase 1: Core Reference Tracking (Estimated: Medium Complexity)

1. **Schema changes** - Add references table and indexes
   - File: `mdl/catalog/tables.go`
   - Add table creation SQL
   - Add view definitions

2. **Reference extraction for microflows**
   - File: `mdl/catalog/builder_references.go` (new)
   - File: `mdl/catalog/builder_microflows.go` (extend)
   - Extract: call, create, change, retrieve, show_page references

3. **Reference extraction for pages**
   - File: `mdl/catalog/builder_pages_refs.go` (new)
   - File: `mdl/catalog/builder_pages.go` (extend)
   - Extract: layout, data_source, action references

4. **Reference extraction for entities**
   - File: `mdl/catalog/builder_modules.go` (extend)
   - Extract: generalization, event_handler, association references

5. **Basic query commands**
   - File: `mdl/executor/cmd_search.go` (new)
   - Add: `show callers`, `show callees`, `show references`

### Phase 2: Search Commands & CLI (Estimated: Low-Medium Complexity)

6. **Grammar extensions**
   - Files: `mdl/grammar/MDLParser.g4`, `MDLLexer.g4`
   - Add: SEARCH statement, CALLERS/CALLEES keywords

7. **AST and visitor**
   - Files: `mdl/ast/ast_search.go` (new), `mdl/visitor/visitor_search.go` (new)

8. **CLI commands**
   - File: `cmd/mxcli/main.go`
   - Add: search, callers, callees, refs subcommands

### Phase 3: Context Assembly (Estimated: Medium Complexity)

9. **Context assembler**
   - File: `mdl/executor/cmd_context.go` (new)
   - Implement context assembly logic

10. **CLI integration**
    - Add: context command with depth/token options

### Phase 4: LSP Server (Estimated: High Complexity, Optional)

11. **LSP implementation**
    - Package: `mdl/lsp/` (new)
    - Implement: definition, references, hover, symbol

12. **CLI integration**
    - Add: lsp command

---

## Testing Strategy

1. **Unit tests** for reference extraction
   - Test each action type extracts correct references
   - Test edge cases (nil values, missing qualified names)

2. **Integration tests** for search queries
   - Test transitive caller/callee queries
   - Test reference filtering by type/module

3. **End-to-end tests** with sample projects
   - Verify expected references are found
   - Verify context assembly produces useful output

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance with large projects | High | Use indexes, limit results, lazy loading |
| Missing action type handlers | Medium | Implement common types first, log unknowns |
| Qualified name resolution | Medium | Use existing hierarchy system |
| Full mode requirement | Low | Document that references need REFRESH CATALOG FULL |

---

## Open Questions

1. **Should references be cached or computed on-demand?**
   - Current proposal: Part of catalog build (cached)
   - Alternative: Compute on query (slower but always fresh)

2. **How to handle cross-module references?**
   - Qualified names handle this naturally
   - Consider: Module filtering options

3. **What's the priority for LSP support?**
   - Could be deferred if CLI/REPL is sufficient for Claude Code

4. **Should we add a `show impact` command?**
   - Would show everything affected by changing an element
   - Could be expensive for highly-connected elements

---

## Appendix: Current Catalog Schema Reference

```sql
-- Existing tables (from mdl/catalog/tables.go)
modules (Id, Name, QualifiedName, ModuleName, folder, description, ...)
entities (Id, Name, QualifiedName, ModuleName, folder, EntityType, generalization, ...)
microflows (Id, Name, QualifiedName, ModuleName, folder, MicroflowType, ReturnType, ...)
nanoflows -- View: filtered microflows
pages (Id, Name, QualifiedName, ModuleName, folder, title, url, LayoutRef, ...)
snippets (Id, Name, QualifiedName, ModuleName, folder, ...)
enumerations (Id, Name, QualifiedName, ModuleName, folder, ValueCount, ...)
activities (Id, Name, caption, ActivityType, MicroflowId, EntityRef, ActionType, ...)
widgets (Id, Name, widgettype, ContainerId, EntityRef, AttributeRef, ...)
xpath_expressions (...)  -- Schema exists but not populated
projects (...)
snapshots (...)
objects -- View: union of all element types

-- Proposed new table
references (Id, SourceType, SourceId, SourceName, SourceLocation,
            TargetType, TargetId, TargetName, RefKind, ...)

-- Proposed new views
call_graph (Caller, Callee)
entity_usage (entity, SourceType, SourceName, RefKind)
page_refs (source, page, RefKind)
```
