---
title: Empty Java Action Argument
status: done
---

# Empty Java Action Argument

Status: Implemented

## Summary

Use the existing MDL `empty` literal to represent an intentionally unbound Java
action argument in microflow call statements.

```mdl
$Total = call java action SampleModule.Recalculate(
  CompanyId       = empty,
  RecalculateAll  = true,
  ItemList        = empty
);
```

In this Java-action argument context, `empty` produces a parameter binding with
an empty `Argument` string in the serialized BSON
(`Microflows$BasicCodeActionParameterValue.Argument = ""`). Re-executing the
script reproduces the same empty binding, so `describe -> exec -> describe`
stays symmetric for existing Studio Pro projects that have unbound code-action
parameters.

## Motivation

Studio Pro's Java-action call dialog allows a developer to leave individual
parameters empty. The on-disk representation is a
`Microflows$JavaActionParameterMapping` whose value is a
`BasicCodeActionParameterValue` with `Argument: ""`.

Emitting `''` would create a literal empty string expression, not an unbound
parameter. Dropping the parameter would lose the original mapping. The existing
`empty` literal is already valid MDL expression syntax and is clearer than
introducing a new placeholder token for this one case.

## Semantics

- In Java-action call arguments, `empty` maps to an empty
  `BasicCodeActionParameterValue.Argument`.
- If the Java action parameter type is a microflow callback, `empty` maps to a
  `Microflows$MicroflowParameterValue` with an empty `Microflow` reference.
- Outside Java-action call arguments, `empty` keeps its normal MDL literal
  meaning.

## Examples

```mdl
-- Java action call with two unbound and one bound argument.
$Total = call java action SampleModule.Recalculate(
  CompanyId       = empty,
  RecalculateAll  = true,
  ItemList        = empty
);
```

The Mendix BSON for the unbound arguments is:

```text
JavaActionParameterMapping {
  Parameter: 'SampleModule.Recalculate.CompanyId',
  Value: BasicCodeActionParameterValue { Argument: '' }
}
```

## Tests And Examples

- Builder coverage: `TestBuildJavaAction_EmptyArgumentPreservesEmptyBasicValue`
  and `TestBuildJavaAction_EmptyMicroflowArgumentUsesMicroflowParameterValue`
  in `mdl/executor/cmd_microflows_builder_java_action_test.go`.
- Example script:
  `mdl-examples/doctype-tests/empty_java_action_argument.mdl`.
