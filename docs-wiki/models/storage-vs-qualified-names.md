---
title: Storage Names vs Qualified Names
category: mental-model
last-synced: 4e185f73
sources:
  - CLAUDE.md
  - sdk/mpr/parser_microflow.go
---

> **Do not duplicate**: the full storage-name mapping table (CLAUDE.md is canonical), the TypeCacheUnknownTypeException fix recipe (symptom table), or reflection-data structure (read the JSON).

## What this is

Mendix elements have two parallel names. The **qualified name** is what the TypeScript SDK and Mendix docs show (e.g. `CreateObjectAction`, `ShowPageAction`). The **storage name** is what actually goes in the BSON `$Type` field (e.g. `CreateChangeAction`, `ShowFormAction`). They are frequently the same, but not always — and where they differ, only the storage name is correct on disk.

## How it fits

The divergence is an accident of history. Mendix renamed concepts in its product over the years but kept the original identifiers in the persisted format for backward compatibility. "Form" was the original word for "Page," so `ShowPageAction` persists as `ShowFormAction`; a create-object action persists as `CreateChangeAction` because it was once modeled as a kind of change action. The SDK presents the modern, friendly name; the file keeps the legacy one.

This matters because the writer must emit the storage name. Emit a qualified name into `$Type` and the file looks plausible but Studio Pro throws `TypeCacheUnknownTypeException` when it tries to deserialize — it has no type registered under that string. The failure is deferred: mxcli writes happily, and you only discover the mistake when the project is opened.

The reader is deliberately more forgiving. The parser registers **both** names for the same handler (see the `microflowActionParsers` map), so it can read files regardless of which variant a given Mendix version wrote. Writing is strict, reading is lenient.

When adding a new type, never assume the SDK name is the storage name. Verify against a known-good MPR — open a Studio-Pro-created example and inspect the `$Type` string directly.

## See also

- [../../CLAUDE.md](../../CLAUDE.md) — canonical qualified-name → storage-name table ("BSON Storage Names vs Qualified Names")
- [../../sdk/mpr/parser_microflow.go](../../sdk/mpr/parser_microflow.go) — `microflowActionParsers` registers both names per handler
- [[models/association-pointers]] — another counter-intuitive BSON naming invariant
- [[bug-patterns/widget-type-object-drift]] — a related "looks valid, fails on open" failure mode
