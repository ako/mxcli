---
title: Microflow Inheritance Split And Cast Statements
status: done
date: 2026-08-06
related:
  - PROPOSAL_split_statement_syntax_alignment.md
---

# Proposal: Microflow Inheritance Split And Cast Statements

Status: Done

## Summary

Add round-trip MDL support for type-based microflow decisions and cast actions:

```mdl
split type $Input
  when Sample.SpecializedInput then
    cast $SpecificInput;
  when Sample.BaseInput then
    return false;
  when (empty) then
    return false;
end split;
```

## Motivation

Studio Pro represents specialization/type decisions as `InheritanceSplit` objects and stores downcasts as `CastAction` activities. Without first-class MDL statements, `describe` can only emit unsupported comments or incomplete split output, and `exec` cannot rebuild the same graph.

## Semantics

`split type $Var` evaluates the runtime specialization of an object variable. Each `when Module.Entity then` branch corresponds to an outgoing sequence flow with an `InheritanceCase`.

The `when (empty) then` branch maps to the outgoing flow with **no** inheritance case — which on a Mendix object-type decision is the `(empty)` flow, taken when the object is **null**. It is **not** a default for unmatched types: mxbuild still requires a flow for every subtype and for the base entity (CE0090), and omitting the empty branch is CE0089. It was originally spelled `else`, which read as a default and never was one; that spelling still parses and warns MDL065 (mxcli #913).

`cast $Output` emits a `CastAction` that produces the downcast variable. `$Output = cast $Input` is accepted for source-preserving authoring, but current Mendix BSON stores the generated cast variable as the primary persisted field.

## Tests And Examples

`mdl-examples/bug-tests/365-microflow-inheritance-split.mdl` demonstrates the syntax. Go regression tests cover parser construction, builder output, describer output, validation recursion, and BSON writer support for inheritance case values and cast actions.

## Open Questions

- Should `exec` validate `case Module.Entity` against the project's specialization hierarchy when connected?
- Should the source-preserving `$Output = cast $Input` form round-trip both variable names once the underlying BSON fields are confirmed for all supported Mendix versions?
