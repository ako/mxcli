---
title: Association Parent/Child Pointer Inversion
category: mental-model
last-synced: 4e185f73
sources:
  - CLAUDE.md
  - mdl/backend/modelsdk/domainmodel_write.go
  - sdk/domainmodel/domainmodel.go
---

> **Do not duplicate**: the storage-name rule (CLAUDE.md is canonical), the CE0066 fix recipe (symptom table in `.claude/skills/fix-issue.md`), or association BSON layout (read source).

## What this is

A Mendix association's BSON pointers are named the opposite of what the words suggest. `ParentPointer` stores the **FROM** entity — the one that owns the foreign key — and `ChildPointer` stores the **TO** entity that is being referenced. The Go model mirrors this: `Association.ParentID` is the FROM entity, `ChildID` is the TO entity.

## How it fits

The naming is historical: in Mendix's data model the "parent" end is the object that *holds* the reference, not the conceptual owner you'd draw at the top of a diagram. So `create association Mod.Child_Parent from Mod.Child to Mod.Parent` writes `ParentPointer = Child.$ID` and `ChildPointer = Parent.$ID`. Once you internalize "Parent = FROM = FK owner," the rest of the model follows.

The consequence that bites people is **entity access**. An association is a member of exactly one entity — the FROM entity, the one in `ParentPointer`. When you grant access to that association, the `MemberAccess` entry must be added only to that entity's access rules. Add it to the TO entity instead and Studio Pro reports CE0066 "Entity access is out of date," because the access rule references a member the entity doesn't actually own. The same FROM-side rule governs cross-module associations (`CrossModuleAssociation.ParentID` is still the local FROM entity; the TO end becomes a by-name `Child` reference).

Getting the inversion wrong silently produces structurally valid BSON that fails validation only when Studio Pro opens it — the writer has no way to know your intent was flipped.

## See also

- [../../CLAUDE.md](../../CLAUDE.md) — canonical pointer/keyword mapping table ("Association Parent/Child Pointer Semantics")
- [../../mdl/backend/modelsdk/domainmodel_write.go](../../mdl/backend/modelsdk/domainmodel_write.go) — writes `ParentPointer`/`ChildPointer`
- [../../sdk/domainmodel/domainmodel.go](../../sdk/domainmodel/domainmodel.go) — `Association.ParentID`/`ChildID` and `MemberAccess`
- [[models/storage-vs-qualified-names]] — the other place BSON naming surprises you
