---
title: SHOW/DESCRIBE Building Blocks
status: proposed
---

# Proposal: SHOW/DESCRIBE Building Blocks

## Overview

**Document type:** `Forms$BuildingBlock`
**Prevalence:** 233 across test projects (83 Enquiries, 73 Evora, 77 Lato)
**Priority:** High — present in every project, reusable UI components

Building Blocks are reusable widget compositions that can be dragged onto pages in Studio Pro. They are structurally similar to Snippets but serve as templates rather than runtime components.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Yes | `sdk/pages/pages.go` — `BuildingBlock{Name, documentation, widget, TemplateID}` |
| **Parser** | Minimal | `sdk/mpr/parser_misc.go` line 165 — Name + Documentation only, no widgets |
| **Reader** | Yes | `ListBuildingBlocks()` in `sdk/mpr/reader_types.go` |
| **Generated metamodel** | Yes | Full struct in `generated/metamodel/types.go` |
| **AST** | No | — |
| **Executor** | No | — |
| **Grammar** | No | — |

## BSON Structure (from test projects)

```
Forms$BuildingBlock:
  Name: string
  documentation: string
  DisplayName: string
  Excluded: bool
  ExportLevel: string
  Platform: string ("Web" | "Native")
  TemplateCategory: string
  TemplateCategoryWeight: int32
  CanvasWidth: int32
  CanvasHeight: int32
  DocumentationUrl: string
  ImageData: binary (preview thumbnail)
  widgets: []*widget (same widget tree as pages)
```

## Proposed MDL Syntax

### SHOW BUILDING BLOCKS

```
show BUILDING BLOCKS [in module]
```

Output table columns:

| Qualified Name | Module | Name | Display Name | Platform | Category | Widgets |
|----------------|--------|------|--------------|----------|----------|---------|

### DESCRIBE BUILDING BLOCK

```
describe BUILDING BLOCK Module.Name
```

Output format (similar to DESCRIBE SNIPPET):

```
/**
 * A reusable card component
 */
-- Building Block: MyModule.CustomerCard
-- Display Name: Customer Card
-- Platform: Web
-- Category: Cards
BUILDING BLOCK MyModule.CustomerCard
{
  container
  {
    textbox $Name;
    textbox $Email;
  };
};
/
```

For the initial implementation, widget tree output can be simplified to show structure without full property details (same approach as DESCRIBE SNIPPET).

## Implementation Steps

### 1. Enhance Parser (sdk/mpr/parser_misc.go)

Extend `parseBuildingBlock()` to capture:
- `DisplayName`, `Platform`, `TemplateCategory`, `Excluded`, `ExportLevel`
- Widget tree parsing (reuse `parseWidgets()` from `parser_page.go`)

Update `BuildingBlock` struct in `sdk/pages/pages.go` to add `DisplayName`, `Platform`, `TemplateCategory`.

### 2. Add AST Types (mdl/ast/ast_query.go)

```go
ShowBuildingBlocks    // in ShowObjectType enum
DescribeBuildingBlock // in DescribeObjectType enum
```

### 3. Add Grammar Rules

```antlr
BUILDING: 'BUILDING';
BLOCK: 'BLOCK';
BLOCKS: 'BLOCKS';

// show BUILDING BLOCKS [in module]
// describe BUILDING BLOCK qualifiedName
```

### 4. Add Executor (mdl/executor/cmd_building_blocks.go)

- `showBuildingBlocks(moduleName string)` — table listing
- `describeBuildingBlock(name QualifiedName)` — MDL output with widget tree

The DESCRIBE handler can reuse the widget tree formatter from `cmd_pages_describe.go`.

### 5. Add Autocomplete

```go
func (e *Executor) GetBuildingBlockNames(moduleFilter string) []string
```

## Testing

- Create `mdl-examples/doctype-tests/17-building-block-examples.mdl`
- Verify against all 3 test projects
