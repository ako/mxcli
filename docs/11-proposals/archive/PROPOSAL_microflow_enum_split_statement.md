---
title: Microflow ENUM SPLIT Statement
status: done
---

# Proposal: Microflow ENUM SPLIT Statement

Status: Implemented

## Summary

Round-trip MDL support for enumeration decisions using SQL/OQL-style `CASE WHEN THEN` syntax:

```mdl
case $Status
  when Open, Pending then
    return true;
  when Closed then
    return false;
  when (empty) then
    return false;
end case;
```

> The original version of this example carried an `else` branch. **MDL008 rejects
> that** — a Mendix enum split is exclusive, with one outgoing flow per value and
> no default. Cover every value explicitly; `(empty)` may share a branch with a
> real value. Corrected while investigating mxcli #913, where the stale example
> was one of the surfaces contradicting the shipped behaviour.

## Motivation

Studio Pro represents enumeration decisions as exclusive splits whose outgoing sequence flows carry enumeration case values. Without a first-class MDL statement, describe/exec round-trips collapse those structures into boolean-looking decisions or unsupported comments.

## Semantics

`case` evaluates an enumeration variable or attribute path. Each `when` lists one or more enumeration values (bare identifiers, consistent with all other enum value references in MDL) that enter the same branch. `(empty)` represents the Mendix empty enumeration case, and a branch for it is **required** (MDL056; mxbuild reports CE0079 without one). There is **no** `else`: MDL008 rejects it, because a Mendix enum split is exclusive with one outgoing flow per value and no default. The enum type is inferred from the variable's declared type — no explicit type annotation is needed at the call site. Maximum 16 cases are supported; a clear error is raised if exceeded.

## Tests And Examples

`mdl-examples/doctype-tests/enum_split_statement.mdl` demonstrates parser syntax. Go regression tests cover AST parsing, builder generation of enumeration case flows, and describer output for existing split graphs.

## Open Questions

- Should the builder validate case values against the referenced enumeration when backend metadata is available?
- Should enum value names be emitted fully qualified in ambiguous cross-module cases?
