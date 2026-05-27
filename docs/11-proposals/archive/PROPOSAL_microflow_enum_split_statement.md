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
  when (empty) then
    return false;
  else
    return false;
end case;
```

## Motivation

Studio Pro represents enumeration decisions as exclusive splits whose outgoing sequence flows carry enumeration case values. Without a first-class MDL statement, describe/exec round-trips collapse those structures into boolean-looking decisions or unsupported comments.

## Semantics

`case` evaluates an enumeration variable or attribute path. Each `when` lists one or more enumeration values (bare identifiers, consistent with all other enum value references in MDL) that enter the same branch. `(empty)` represents the Mendix empty enumeration case. `else` is optional and maps to the outgoing flow without an explicit case value. The enum type is inferred from the variable's declared type — no explicit type annotation is needed at the call site. Maximum 16 cases are supported; a clear error is raised if exceeded.

## Tests And Examples

`mdl-examples/doctype-tests/enum_split_statement.mdl` demonstrates parser syntax. Go regression tests cover AST parsing, builder generation of enumeration case flows, and describer output for existing split graphs.

## Open Questions

- Should the builder validate case values against the referenced enumeration when backend metadata is available?
- Should enum value names be emitted fully qualified in ambiguous cross-module cases?
