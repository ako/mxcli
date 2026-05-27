---
title: Microflow CHANGE Refresh Modifier
status: draft
---

# Proposal: Microflow CHANGE Refresh Modifier

Status: Draft

## Summary

Allow `change` microflow statements to explicitly preserve the Mendix `RefreshInClient` flag:

```mdl
change $Customer (Name = 'Jane') refresh;
change $Customer refresh;
```

## Motivation

Mendix change-object actions can refresh the changed object in the client independently from committing the object. MDL previously had no syntax for that flag, so a describe/exec round-trip could rewrite `RefreshInClient = true` to `false` for change actions with member assignments.

## Semantics

The `refresh` modifier maps directly to `ChangeObjectAction.RefreshInClient`. Omitting it preserves the existing default behavior. The modifier is accepted both with member assignments and on a memberless change action.

## Tests And Examples

`mdl-examples/doctype-tests/change_refresh_modifier.mdl` demonstrates both forms. Go tests cover formatter output, parser behavior, and builder serialization.

## Open Questions

- Should a memberless `change $Object;` continue to infer refresh in separate validity-focused fixes, or should the explicit `refresh` modifier be the only authoring form?
