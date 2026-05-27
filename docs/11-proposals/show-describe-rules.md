---
title: SHOW/DESCRIBE Rules
status: proposed
---

# Proposal: SHOW/DESCRIBE Rules

## Overview

**Document type:** `microflows$rule`
**Prevalence:** 49 across test projects (9 Enquiries, 28 Evora, 12 Lato)
**Priority:** Medium — decision logic used in microflow split conditions

Rules are structurally identical to Microflows but must return Boolean. They are used in split conditions (exclusive splits) as an alternative to expressions. Rules have parameters, activities, and flows — the same internal structure as microflows.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **Parser** | No | Rules share BSON structure with Microflows but no parser exists |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 4791 |

## BSON Structure (from test projects)

```
microflows$rule:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  ApplyEntityAccess: bool
  MarkAsUsed: bool
  MicroflowReturnType: DataTypes$BooleanType (always boolean)
  ReturnVariableName: string
  ObjectCollection: microflows$MicroflowObjectCollection
    objects: []*MicroflowObject (activities, parameters, etc.)
  Flows: []*microflows$SequenceFlow
```

The internal structure (ObjectCollection, Flows) is identical to `microflows$microflow`.

## Proposed MDL Syntax

### SHOW RULES

```
show rules [in module]
```

| Qualified Name | Module | Name | Parameters | Activities |
|----------------|--------|------|------------|------------|

### DESCRIBE RULE

```
describe rule Module.Name
```

Output format (mirrors DESCRIBE MICROFLOW but with RULE keyword):

```
/**
 * Checks if the customer is eligible for a discount
 */
rule MyModule.IsEligibleForDiscount
  parameter $Customer: MyModule.Customer
  returns boolean
begin
  retrieve $orders from database
    where MyModule.Customer_Order/MyModule.Order/status = 'Completed';
  if $orders/length > 5 then
    return true;
  else
    return false;
  end if;
end;
/
```

## Implementation Steps

### 1. Add Model Type and Parser

Since Rules share the same BSON structure as Microflows, the existing microflow parser (`parser_microflow.go`) can be reused with minimal changes:

```go
type rule struct {
    // Same fields as microflow
    microflows.Microflow // embed or duplicate
}
```

Alternatively, parse Rules as `microflow` instances with a `Kind: "rule"` marker.

### 2. Add Reader

```go
func (r *Reader) ListRules() ([]*microflows.Microflow, error) {
    return r.listUnitsByType("microflows$rule", parseMicroflow)
}
```

### 3. Add AST, Grammar, Visitor, Executor

Grammar tokens: `rule` (may already exist), `rules`.

The DESCRIBE handler can delegate to `describeMicroflow()` internally, with the output keyword changed from `microflow` to `rule`.

### 4. Add Autocomplete

```go
func (e *Executor) GetRuleNames(moduleFilter string) []string
```

## Design Decision

**Option A: Reuse Microflow infrastructure** — Parse rules as microflows with a `Kind` field. DESCRIBE outputs `rule` keyword but reuses the microflow formatter. This is simpler but less explicit.

**Option B: Separate type** — Dedicated `rule` type and handlers. More code but cleaner separation.

**Recommendation:** Option A — rules ARE microflows with a Boolean return constraint. Reusing the infrastructure minimizes code.

## Testing

- Verify against Evora project (28 rules — most comprehensive)
