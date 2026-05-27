---
title: SHOW/DESCRIBE Custom Icon Collections
status: proposed
---

# Proposal: SHOW/DESCRIBE Custom Icon Collections

## Overview

**Document type:** `CustomIcons$CustomIconCollection`
**Prevalence:** 8 across test projects (3 Enquiries, 2 Evora, 3 Lato)
**Priority:** Low — small count, used for custom icon fonts in UI

Custom Icon Collections define icon fonts with named glyphs. Each collection has a CSS class prefix and contains icons mapped to character codes. The font data itself is stored as binary.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 1329 |

## BSON Structure (from test projects)

```
CustomIcons$CustomIconCollection:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  CollectionClass: string (e.g., "mx-icon-lined")
  Prefix: string (e.g., "mx-icon")
  FontData: binary (embedded font file)
  Icons: []*CustomIcons$CustomIcon
    - Name: string (e.g., "arrow-down")
    - CharacterCode: int32 (e.g., 59648)
    - Tags: string (comma-separated search tags)
```

## Proposed MDL Syntax

### SHOW ICON COLLECTIONS

```
show icon COLLECTIONS [in module]
```

| Qualified Name | Module | Name | Class | Prefix | Icons |
|----------------|--------|------|-------|--------|-------|

### DESCRIBE ICON COLLECTION

```
describe icon collection Module.Name
```

Output format:

```
icon collection MyModule.CustomIcons
  class 'mx-icon-lined'
  PREFIX 'mx-icon'
{
  arrow-down (U+E900)
  arrow-up (U+E901)
  check (U+E902)
  close (U+E903)
  search (U+E904)
};

-- (5 icons, font data: 24.5 KB)
/
```

## Implementation Steps

### 1. Add Model Type (model/types.go)

```go
type CustomIconCollection struct {
    ContainerID     model.ID
    Name            string
    documentation   string
    Excluded        bool
    ExportLevel     string
    CollectionClass string
    Prefix          string
    FontDataSize    int // size in bytes (don't store actual font data)
    Icons           []*CustomIcon
}

type CustomIcon struct {
    Name          string
    CharacterCode int
    Tags          string
}
```

### 2. Add Parser, Reader, AST, Grammar, Executor

Standard pattern. Grammar tokens: `icon`, `ICONS`, `collection`, `COLLECTIONS` (COLLECTION may be shared with IMAGE COLLECTION).

### 3. Add Autocomplete

```go
func (e *Executor) GetIconCollectionNames(moduleFilter string) []string
```

## Complexity

**Low** — flat structure with a simple list of icons.

## Testing

- Verify against all 3 test projects
