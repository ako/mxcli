---
title: mxcli Code Search Extension - Design Input
status: done
---

# mxcli Code Search Extension - Design Input

## Context

mxcli is a Go CLI tool that provides a REPL for working with Mendix projects using MDL (Mendix Definition Language). It can also be used by Claude Code to execute single MDL commands from the command line and to diff MDL scripts against project state.

The tool already has:
- An internal SQLite database
- Catalog tables indexing Mendix project documents (catalog.microflows, catalog.pages, catalog.entities, etc.)
- SQL query capability against this catalog
- MDL parser and evaluator
- REPL interface
- CLI interface for single commands

## Goals

Extend mxcli to be an LLM-friendly codebase search tool that helps Claude Code understand and navigate Mendix projects. The tool should:

1. Provide symbol lookup, reference finding, and call graph analysis
2. Expose search via CLI commands, MDL language extensions, and optionally LSP
3. Reuse the existing SQLite catalog infrastructure
4. Support "context assembly" - gathering related code for LLM consumption

## Proposed Architecture

### Single Binary, Multiple Interfaces

```
┌─────────────────────────────────────────────────────┐
│                     mxcli                           │
├─────────────────────────────────────────────────────┤
│  Interfaces                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐            │
│  │   CLI    │ │   REPL   │ │   LSP    │            │
│  │(existing)│ │(existing)│ │  (new)   │            │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘            │
│       │            │            │                   │
├───────┴────────────┴────────────┴───────────────────┤
│  Core services                                      │
│  ┌─────────────┐ ┌─────────────┐ ┌───────────────┐ │
│  │ MDL Parser  │ │ project     │ │ search service│ │
│  │ & Evaluator │ │ model       │ │ (new/extend)  │ │
│  │ (existing)  │ │ (existing)  │ │               │ │
│  └─────────────┘ └─────────────┘ └───────────────┘ │
│  ┌─────────────┐ ┌─────────────┐                   │
│  │ Diff Engine │ │ SQLite      │                   │
│  │ (existing)  │ │ catalog     │                   │
│  │             │ │ (existing)  │                   │
│  └─────────────┘ └─────────────┘                   │
└─────────────────────────────────────────────────────┘
```

### Extending the SQLite Schema

Add reference tracking to enable "find usages" and call graph queries:

```sql
-- New table for reference tracking
create table catalog.references (
    id integer primary key,
    source_type text,      -- 'microflow', 'page', 'entity', ...
    source_name text,      -- qualified name of the referring document
    source_location text,  -- optional: position within document
    target_type text,      -- what kind of thing is referenced
    target_name text,      -- qualified name of referenced thing
    ref_kind text          -- 'call', 'attribute_use', 'entity_use', 'parameter_type', ...
);

create index idx_refs_target on catalog.references(target_type, target_name);
create index idx_refs_source on catalog.references(source_type, source_name);

-- Convenience view for microflow call graph
create view catalog.call_graph as
select 
    source_name as caller,
    target_name as callee
from catalog.references 
where source_type = 'microflow' 
  and target_type = 'microflow'
  and ref_kind = 'call';
```

## Proposed CLI Commands

```bash
# Symbol search
mxcli search symbol <name> [--type=microflow|entity|page|...]
mxcli search entities [pattern]
mxcli search microflows [pattern]

# reference search  
mxcli search refs <name> [--type=...] [--ref-kind=call|use|...]

# call graph
mxcli search callers <microflow> [--transitive]
mxcli search callees <microflow> [--transitive]

# impact analysis
mxcli search impact <name> [--type=...]

# context assembly (for LLM consumption)
mxcli search context <name> [--depth=N] [--max-tokens=N]

# Direct sql (already exists?)
mxcli sql "<query>"
```

## Proposed MDL Language Extensions

Add search functions to MDL that compile to SQL queries:

```ruby
# find symbols by type and filters
find(entity)                                    # all entities
find(entity, name: /^Customer/)                 # regex/pattern match
find(microflow, module: "OrderManagement")      # filter by module

# reference queries
refs(entity: "Customer")                        # What references Customer?
refs(microflow: "ACT_CreateOrder")              # What calls this microflow?
refs("Customer.Name")                           # references to specific attribute

# call graph queries
callers("SUB_ValidateOrder")                    # Direct callers
callers("SUB_ValidateOrder", transitive: true)  # transitive callers
callees("ACT_ProcessOrder")                     # What does this call?

# Dependency and impact analysis
deps("OrderManagement")                         # module dependencies
impact("Customer.Email")                        # What would changing this affect?

# context assembly for LLM
context("ACT_CreateOrder", depth: 2)            # microflow + deps + entities used
```

## Reference Extraction During Indexing

During the existing project indexing pass, extract references from each document type:

### Microflows
- Microflow call actions → reference to called microflow (ref_kind: 'call')
- Retrieve actions → reference to entity (ref_kind: 'retrieve')
- Change/create actions → reference to entity (ref_kind: 'change')
- Attribute access → reference to attribute (ref_kind: 'attribute_read' or 'attribute_write')
- Parameter types → reference to entity (ref_kind: 'parameter_type')

### Pages
- Data views → reference to entity or microflow data source
- List views → reference to entity
- Microflow buttons → reference to microflow
- Widget attribute bindings → reference to attribute

### Entities
- Associations → reference to target entity
- Generalizations → reference to parent entity
- Event handlers → reference to microflow

## Context Assembly Feature

The `context` command/function assembles relevant information for LLM consumption:

```
context("ACT_CreateOrder", depth: 2) returns:

1. The target microflow definition/structure
2. all entities it uses (retrieves, creates, changes)
3. all microflows it calls (depth 1)
4. all microflows those call (depth 2)  
5. Direct callers of the target (limited)
6. parameter and return types

Formatted as a single text block with clear sections,
trimmed to fit within max_tokens.
```

## Optional: LSP Server Mode

Add `mxcli lsp` command that starts an LSP server, enabling VS Code integration:

```bash
mxcli lsp              # stdio mode
mxcli lsp --tcp :9257  # TCP mode
```

Supported LSP features (all backed by SQLite queries):
- textDocument/definition - go to definition
- textDocument/references - find all references  
- textDocument/hover - show symbol info
- workspace/symbol - search symbols

Go LSP library options:
- go.lsp.dev/protocol
- github.com/sourcegraph/go-lsp
- Direct jsonrpc2 implementation

## Questions for Codebase Analysis

Please investigate the existing codebase to answer:

1. **SQLite schema**: What tables currently exist in the catalog? What columns do they have?

2. **Indexing code**: Where is the code that populates the catalog tables? Can we extend it to extract references?

3. **MDL evaluator**: How are builtin functions registered? What's the pattern for adding new functions like `find()`, `refs()`, `callers()`?

4. **CLI structure**: What CLI framework is used (cobra?)? How are subcommands organized?

5. **Project model**: What data structures represent Mendix documents (microflows, entities, pages)? Do they already track references internally?

6. **SQL execution**: How is SQL currently exposed? Is there already a `mxcli sql` command or similar?

## Deliverable

Based on this input and the codebase analysis, produce a design proposal that:

1. Maps the proposed features to specific files/packages to modify
2. Proposes concrete schema changes with migrations
3. Shows where reference extraction should be added in the indexing code
4. Defines the Go interfaces for the SearchService
5. Shows how MDL functions would be implemented
6. Estimates complexity and suggests implementation order
