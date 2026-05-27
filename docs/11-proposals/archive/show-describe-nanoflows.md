---
title: DESCRIBE Nanoflow (Enhancement)
status: superseded
---

# Proposal: DESCRIBE Nanoflow (Enhancement)

> **Status: SUPERSEDED** — Full nanoflow support (SHOW, DESCRIBE, CREATE, DROP, CALL, GRANT/REVOKE, SHOW ACCESS, MOVE, MERMAID) is implemented in PRs #287–#297 and documented in [`PROPOSAL_nanoflow_support.md`](./PROPOSAL_nanoflow_support.md). This proposal is retained for historical context only.

## Overview

**Document type:** `microflows$nanoflow`
**Prevalence:** 227 across test projects (79 Enquiries, 97 Evora, 51 Lato)
**Priority:** High — nanoflows are heavily used, SHOW works but DESCRIBE is missing

Nanoflows execute client-side in the browser or native app. They share the same BSON structure as microflows (`MicroflowObjectCollection`, `MicroflowObject`, `SequenceFlow`) but run on the client. Currently `show nanoflows` works but `describe nanoflow` does not.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Yes | `sdk/microflows/microflows.go` — `nanoflow` struct (has `ObjectCollection` field) |
| **Parser** | **Partial** | `sdk/mpr/parser_nanoflow.go` — parses metadata and parameters only, **does NOT parse ObjectCollection** |
| **Reader** | Yes | `ListNanoflows()`, `GetNanoflow()` |
| **SHOW** | Yes | `showNanoflows()` in executor (activity count is always 0) |
| **DESCRIBE** | **No** | No `DescribeNanoflow` AST type or handler |
| **DROP** | **No** | No `DropNanoflowStmt` |

## Critical Gap: Nanoflow Parser Ignores Activities

The current `parseNanoflow()` in `sdk/mpr/parser_nanoflow.go` only parses:
- Name, Documentation, MarkAsUsed, Excluded (basic metadata)
- Parameters

It does **NOT** call `parseMicroflowObjectCollection()`, so the `ObjectCollection` field is always nil. This means:
- `show nanoflows` always shows 0 activities
- Any DESCRIBE handler would output an empty body
- The nanoflow serializer similarly ignores activities

The microflow parser (`parser_microflow.go`) **does** parse `ObjectCollection` via `parseMicroflowObjectCollection()`, which handles all flow objects and action types.

## Nanoflow vs Microflow Activities

Nanoflows use the same `MicroflowObject` types as microflows, but Mendix restricts which activities are available client-side:

### Activities available in nanoflows

| Activity | MDL Keyword | Notes |
|----------|-------------|-------|
| CreateVariable | `create VARIABLE` | Same as microflow |
| ChangeVariable | `change VARIABLE` | Same as microflow |
| CreateObject | `create` | Same as microflow |
| ChangeObject | `change` | Same as microflow |
| RetrieveAction | `retrieve` | From database or by association |
| CreateList | `create list` | Same as microflow |
| ChangeList | `change list` | Same as microflow |
| ListOperation | `list operation` | Same as microflow |
| AggregateList | `AGGREGATE list` | Same as microflow |
| ShowPage | `show page` | Same as microflow |
| ClosePage | `close page` | Same as microflow |
| ShowMessage | `show message` | Same as microflow |
| ValidationFeedback | `validation feedback` | Same as microflow |
| CallNanoflow | `call nanoflow` | Uses MicroflowCallAction with nanoflow target |
| CallJavaScriptAction | `call javascript action` | Via JavaActionCallAction with JS action target |
| ExclusiveSplit | `if ... then` | Same as microflow |
| ExclusiveMerge | (merge point) | Same as microflow |
| LoopedActivity | `loop` | Same as microflow |
| StartEvent | (implicit) | Same as microflow |
| EndEvent | `return` | Same as microflow |
| ErrorEvent | (error handler) | Same as microflow |
| ShowHomePage | `show home page` | Same as microflow |

### Activities NOT available in nanoflows (microflow-only)

| Activity | MDL Keyword | Reason |
|----------|-------------|--------|
| CommitAction | `commit` | Server-side database operation |
| DeleteAction | `delete` | Server-side database operation |
| RollbackAction | `rollback` | Server-side transaction |
| JavaActionCallAction | `call java action` | Server-side JVM execution |
| RestCallAction | `call rest` | Server-side HTTP (nanoflows use JS actions for HTTP) |
| LogMessageAction | `log message` | Server-side logging |
| DownloadFileAction | `DOWNLOAD file` | Server-side file streaming |
| CastAction | `cast` | Server-side type casting |
| ExecuteDatabaseQueryAction | `execute database query` | Server-side SQL |
| CallExternalAction | `call external` | Server-side external calls |

### Nanoflow-specific considerations

- **CallNanoflow**: Uses the same `MicroflowCallAction` BSON type but targets a nanoflow. The formatter should output `call nanoflow` instead of `call microflow` when the target is a nanoflow.
- **CallJavaScriptAction**: Uses `JavaActionCallAction` BSON type but targets a JS action. The formatter should output `call javascript action` instead of `call java action`.
- **Offline retrieval**: Nanoflows retrieve from the client-side database, which may have different behavior than server-side retrieves.

## Proposed MDL Syntax

### DESCRIBE NANOFLOW

```
describe nanoflow Module.Name
```

Output format (same as DESCRIBE MICROFLOW but with NANOFLOW keyword):

```
/**
 * Validates the customer form before saving
 */
create nanoflow MyModule.ValidateCustomerForm (
  $Customer: MyModule.Customer
)
returns boolean
begin
  if $Customer/Name = '' then
    validation feedback $Customer attribute Name message 'Name is required';
    return false;
  end if;
  return true;
end;
/
```

This matches the existing microflow DESCRIBE format exactly — parameters are inline in parentheses with `$` prefix, comma-separated, one per line.

### DROP NANOFLOW (optional, lower priority)

```
drop nanoflow Module.Name;
```

## Implementation Steps

### 1. Enhance Nanoflow Parser (sdk/mpr/parser_nanoflow.go) — **Critical**

Add `ObjectCollection` parsing to `parseNanoflow()`:

```go
func parseNanoflow(data map[string]any) (*microflows.Nanoflow, error) {
    nf := &microflows.Nanoflow{...}
    // ... existing metadata parsing ...

    // add: Parse ObjectCollection (reuse microflow parsing)
    if objColl, ok := data["ObjectCollection"]; ok {
        nf.ObjectCollection = parseMicroflowObjectCollection(objColl)
    }

    // add: Parse Flows
    if flows, ok := data["Flows"]; ok {
        nf.Flows = parseMicroflowFlows(flows)
    }

    // add: Parse ReturnType
    if rt, ok := data["MicroflowReturnType"]; ok {
        nf.ReturnType = parseMicroflowReturnType(rt)
    }

    return nf, nil
}
```

This reuses the existing `parseMicroflowObjectCollection()` and related functions from `parser_microflow.go` — no new parsing code is needed since nanoflows use the same BSON structure.

### 2. Add AST Types (mdl/ast/ast_query.go)

```go
// in DescribeObjectType enum:
DescribeNanoflow

// add string() case:
case DescribeNanoflow:
    return "nanoflow"
```

For DROP (optional):
```go
type DropNanoflowStmt struct {
    Name QualifiedName
}
```

### 3. Add Grammar Rules (MDLParser.g4)

The grammar likely already has `describe nanoflow` syntax — the visitor just needs to wire it to a new AST type instead of silently ignoring it.

### 4. Add Visitor Mapping

Map `describe nanoflow qualifiedName` to `DescribeStmt{ObjectType: DescribeNanoflow}`.

### 5. Add Executor Handler (mdl/executor/cmd_microflows_show.go)

```go
func (e *Executor) describeNanoflow(name ast.QualifiedName) error {
    // Look up nanoflow
    nanoflows, err := e.reader.ListNanoflows()
    // ... find by qualified name ...

    // Convert to microflow (they share the same structure) and
    // delegate to describeMicroflowToString with "nanoflow" keyword
}
```

The existing `formatActivity()` and `formatAction()` in `cmd_microflows_format_action.go` handle all `MicroflowObject` types and work unchanged for nanoflows since they share the same types.

Consideration: when formatting a `MicroflowCallAction` that targets a nanoflow, output `call nanoflow` instead of `call microflow`. This requires checking whether the call target is a nanoflow (by looking it up in the nanoflow list) or adding a helper to distinguish.

### 6. Wire into Executor Dispatcher

```go
case ast.DescribeNanoflow:
    return e.describeNanoflow(s.Name)
```

### 7. Fix SHOW NANOFLOWS activity count

With the parser enhancement, `show nanoflows` will correctly report activity counts instead of always showing 0.

## Complexity

**Medium** — The main work is enhancing the nanoflow parser to parse ObjectCollection:
- Parser enhancement: ~20 lines (calls to existing functions)
- AST + Grammar + Visitor wiring: ~15 lines
- Executor handler: ~40 lines (find nanoflow, delegate to formatter)
- CALL NANOFLOW vs CALL MICROFLOW distinction: ~15 lines
- Total: ~90 lines of code

The formatter code in `cmd_microflows_format_action.go` requires **no changes** since nanoflows use the same `MicroflowObject` types.

## Testing

- Add `describe nanoflow` examples to existing test files
- Verify against all 3 test projects (especially Evora with 97 nanoflows)
- Test activities: CreateObject, ChangeObject, Retrieve, ShowPage, ClosePage, ValidationFeedback, CallNanoflow, CallJavaScriptAction, ExclusiveSplit, Loop
- Verify that microflow-only activities (if somehow present) don't cause errors
