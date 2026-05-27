---
title: SHOW/DESCRIBE Regular Expressions
status: proposed
---

# Proposal: SHOW/DESCRIBE Regular Expressions

## Overview

**Document type:** `RegularExpressions$RegularExpression`
**Prevalence:** 13 across test projects (5 Enquiries, 4 Evora, 4 Lato)
**Priority:** Low — small count but simple to implement, useful for understanding validation rules

Regular Expressions are reusable named patterns used in entity attribute validation constraints. They are referenced by name from entity validation rules.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Partial | `model/types.go` line 227 — `RegularExpression{ContainerID, Name, documentation, expression}`, missing `Excluded`, `ExportLevel` |
| **Parser** | No | No `parseRegularExpression()` function |
| **Reader** | No | No `ListRegularExpressions()` method |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 7769 |

## BSON Structure (from test projects)

```
RegularExpressions$RegularExpression:
  Name: string
  documentation: string
  expression: string (the regex pattern)
  Excluded: bool
  ExportLevel: string ("api", "Hidden")
```

This is one of the simplest document types — just 5 fields.

## Proposed MDL Syntax

### SHOW REGULAR EXPRESSIONS

```
show REGULAR EXPRESSIONS [in module]
```

| Qualified Name | Module | Name | Expression |
|----------------|--------|------|------------|

Where "Expression" is truncated to ~40 chars for display.

### DESCRIBE REGULAR EXPRESSION

```
describe REGULAR expression Module.Name
```

Output format:

```
/**
 * Validates email addresses
 */
REGULAR expression MyModule.EmailRegex
  expression '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$';
/
```

## Implementation Steps

### 1. Add Parser (sdk/mpr/parser_misc.go)

Simple flat BSON parsing — extract Name, Documentation, Expression, Excluded, ExportLevel.

### 2. Add Reader

```go
func (r *Reader) ListRegularExpressions() ([]*model.RegularExpression, error)
```

### 3. Add AST, Grammar, Visitor, Executor

Grammar tokens: `REGULAR`, `expression`, `EXPRESSIONS`.

This is one of the simplest implementations — no nested structures, no recursive parsing.

### 4. Add Autocomplete

```go
func (e *Executor) GetRegularExpressionNames(moduleFilter string) []string
```

## Complexity

**Very low** — flat structure, 5 fields, no nested objects. Good candidate for a first implementation to establish the pattern.

## Testing

- Verify against all 3 test projects
