---
title: SHOW/DESCRIBE Queues (Task Queues)
status: proposed
---

# Proposal: SHOW/DESCRIBE Queues (Task Queues)

## Overview

**Document type:** `Queues$Queue`
**Prevalence:** 5 across test projects (2 Enquiries, 2 Evora, 1 Lato)
**Priority:** Low — small count but important for async processing configuration

Task Queues define asynchronous processing configurations for microflows. They control parallelism and cluster-wide behavior for background task execution.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **Parser** | No | — |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 7727 |

## BSON Structure (from test projects)

```
Queues$Queue:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  Config: Queues$BasicQueueConfig
    ClusterWide: bool
    ParallelismExpression: string (e.g., "3")
```

## Proposed MDL Syntax

### SHOW QUEUES

```
show QUEUES [in module]
```

| Qualified Name | Module | Name | Cluster Wide | Parallelism |
|----------------|--------|------|-------------|-------------|

### DESCRIBE QUEUE

```
describe QUEUE Module.Name
```

Output format:

```
/**
 * Background order processing queue
 */
QUEUE MyModule.OrderProcessing
  PARALLELISM 3
  CLUSTER WIDE;
/
```

For a simple local queue:

```
QUEUE MyModule.EmailSending
  PARALLELISM 1;
/
```

## Implementation Steps

### 1. Add Model Type (model/types.go)

```go
type Queue struct {
    ContainerID  model.ID
    Name         string
    documentation string
    Excluded     bool
    ExportLevel  string
    ClusterWide  bool
    Parallelism  string // expression, usually a number
}
```

### 2. Add Parser (sdk/mpr/parser_misc.go)

Simple flat BSON parsing. Parse the nested `Config` object to extract `ClusterWide` and `ParallelismExpression`.

### 3. Add Reader

```go
func (r *Reader) ListQueues() ([]*model.Queue, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Grammar tokens: `QUEUE`, `QUEUES`.

### 5. Add Autocomplete

```go
func (e *Executor) GetQueueNames(moduleFilter string) []string
```

## Complexity

**Low** — simple structure with one nested config object.

## Testing

- Verify against all 3 test projects
