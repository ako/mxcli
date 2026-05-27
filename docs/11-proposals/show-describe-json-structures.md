---
title: SHOW/DESCRIBE JSON Structures
status: proposed
---

# Proposal: SHOW/DESCRIBE JSON Structures

## Overview

**Document type:** `JsonStructures$JsonStructure`
**Prevalence:** 96 across test projects (16 Enquiries, 42 Evora, 38 Lato)
**Priority:** High — used for REST integrations, import/export mappings

JSON Structures define the schema for JSON data used in integrations. They contain a JSON snippet and a parsed element tree that maps JSON fields to types. They are referenced by Import Mappings, Export Mappings, and REST operations.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | Only in generated metamodel |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 3172 |

## BSON Structure (from test projects)

```
JsonStructures$JsonStructure:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  JsonSnippet: string (raw json example)
  Elements: []*JsonStructures$JsonElement
    - path: string (e.g., "root", "root/name", "root/items")
    - ExposedName: string
    - ExposedItemName: string
    - PrimitiveType: string ("string", "integer", "boolean", "decimal", "datetime", "Unknown")
    - ElementType: string ("object", "Array", "value", "Choice")
    - MinOccurs: int
    - MaxOccurs: int
    - Nillable: bool
    - IsDefaultType: bool
    - Children: []*JsonElement (recursive)
    - MaxLength: int
    - FractionDigits: int
    - TotalDigits: int
```

## Proposed MDL Syntax

### SHOW JSON STRUCTURES

```
show json structures [in module]
```

| Qualified Name | Module | Name | Elements | Source |
|----------------|--------|------|----------|--------|

Where "Elements" is the count of top-level elements, and "Source" indicates if it was derived from a JSON snippet.

### DESCRIBE JSON STRUCTURE

```
describe json structure Module.Name
```

Output format:

```
/**
 * Customer API response schema
 */
json structure MyModule.CustomerResponse
{
  root: object
    id: integer
    name: string
    email: string
    addresses: Array
      street: string
      city: string
      zipCode: string
    active: boolean
};

-- JSON Snippet:
-- {
--   "id": 1,
--   "name": "John",
--   "email": "john@example.com",
--   "addresses": [{"street": "...", "city": "...", "zipCode": "..."}],
--   "active": true
-- }
/
```

The element tree is rendered with indentation to show nesting. The original JSON snippet is shown as a comment block.

## Implementation Steps

### 1. Add Model Type (model/types.go)

```go
type JsonStructure struct {
    ContainerID model.ID
    Name        string
    documentation string
    JsonSnippet string
    Elements    []*JsonElement
    Excluded    bool
    ExportLevel string
}

type JsonElement struct {
    path          string
    ExposedName   string
    PrimitiveType string
    ElementType   string // "object", "Array", "value"
    MinOccurs     int
    MaxOccurs     int
    Children      []*JsonElement
}
```

### 2. Add Parser (sdk/mpr/parser_misc.go or new file)

Parse `JsonStructures$JsonStructure` BSON into the model type. Recursively parse `Elements` and their `Children`.

### 3. Add Reader (sdk/mpr/reader_documents.go)

```go
func (r *Reader) ListJsonStructures() ([]*model.JsonStructure, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Standard pattern: `ShowJsonStructures`, `DescribeJsonStructure`.

Grammar tokens: `json`, `structure`, `structures`.

### 5. Add Autocomplete

```go
func (e *Executor) GetJsonStructureNames(moduleFilter string) []string
```

## Testing

- Create `mdl-examples/doctype-tests/18-json-structure-examples.mdl`
- Verify against all 3 test projects
