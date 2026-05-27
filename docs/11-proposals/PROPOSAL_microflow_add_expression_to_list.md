---
title: Microflow ADD Expression To List
status: draft
---

# Proposal: Microflow ADD Expression To List

Status: Draft

## Summary

Allow `add` microflow statements to use any expression as the value being added to a list:

```mdl
add if $UseFirst then $FirstItem else $SecondItem to $TargetList;
```

The existing variable-only form remains valid:

```mdl
add $Item to $TargetList;
```

## Motivation

Studio Pro stores the value of a list-add action as an expression string. Existing models can therefore contain a list-add value that is not a bare variable. MDL previously parsed only `add $Item to $List`, so describe/exec round-trips could not preserve expression-valued list additions.

## Semantics

The parser stores the add value as an expression. For compatibility, a bare variable expression also populates the legacy `Item` field in the AST. The builder writes the expression source to the Mendix `ChangeListAction.Value` field and falls back to the legacy item variable only when no expression is present.

## Tests And Examples

`mdl-examples/doctype-tests/add_expression_to_list.mdl` demonstrates adding an object-valued conditional expression to another list. Go regression tests cover parser behavior and builder output for both expression and simple-variable forms.

## Open Questions

- Should validation infer the list element type and reject expressions that cannot produce an object compatible with the target list?
