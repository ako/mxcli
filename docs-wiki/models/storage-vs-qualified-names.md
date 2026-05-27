---
title: Storage Names vs Qualified Names
category: mental-model
last-synced: fda04711
sources:
  - CLAUDE.md
  - sdk/mpr/parser_microflow.go
  - sdk/mpr/writer_odata.go
  - sdk/mpr/parser_odata.go
---

> **Do not duplicate**: the full `$Type` storage-name mapping table (CLAUDE.md is canonical), the per-instance fix recipes (symptom table), or reflection-data structure (read the JSON).

## What this is

What the BSON file calls a thing is often not what the TypeScript SDK or Studio Pro UI calls it. The divergence shows up at two levels — `$Type` strings AND nested field keys inside a document — and the only authoritative reference is what Studio Pro actually wrote to disk.

## How it fits

The `$Type` divergence is the one most documented. Mendix renamed concepts in its product over the years but kept the original identifiers in the persisted format for backward compatibility — "Form" was the original word for "Page," so `ShowPageAction` persists as `ShowFormAction`; a create-object action persists as `CreateChangeAction` because it was once modeled as a change action. The SDK presents the modern, friendly name; the file keeps the legacy one. Writing the SDK name into `$Type` produces a file that looks plausible but raises `TypeCacheUnknownTypeException` when Studio Pro opens it. The failure is deferred: mxcli writes happily, the bug surfaces on open.

The same divergence applies one level down, to keys *within* a typed document. Three flavours recur:

- **Qualified-name field values.** Some string-valued fields look like bare names but Studio Pro expects qualified ones — `ODataPublish$PublishedAttribute.Attribute` is `"Module.Entity.AttrName"`, not `"AttrName"`. Bare names silently fail to link, often visible only when a second entity in the same document mysteriously stops resolving its key.
- **Renamed-across-versions field keys.** A storage key valid in one Mendix minor may have been renamed in another. A wrong key is written into the BSON happily, Studio Pro silently ignores it, and the property reads as its default in the UI — no warning, no error, the dropdown just sits on "Constants only" forever.
- **Single field with a sub-document discriminator.** What looks like several mutually-exclusive UI options can map to ONE BSON field, with Studio Pro picking the label from the SHAPE of the referenced sub-document rather than from which field carries the value. The OData consumed service's "Configuration microflow" vs "Headers microflow" dropdown is one field (`ConfigurationMicroflow`) whose label is decided by the referenced microflow's return type.

The reader is deliberately more forgiving than the writer. The parser registers BOTH names for the same handler ([`microflowActionParsers`](../../sdk/mpr/parser_microflow.go)), so it can read files regardless of which variant a given Mendix version wrote. Writing is strict, reading is lenient.

The escape hatch for any uncertainty: never assume the SDK name (or a previous mxcli guess) is the storage shape. Have the user create or duplicate the object in Studio Pro, save, then dump that file's BSON. Re-dump after every Studio Pro change — cached dumps go stale as soon as the user touches the project (see [[../bug-patterns/visitor-wiring-gaps]] for a related "looks valid, fails silently" failure mode).

## See also

- [../../CLAUDE.md](../../CLAUDE.md) — canonical qualified-name → storage-name table ("BSON Storage Names vs Qualified Names")
- [../../sdk/mpr/parser_microflow.go](../../sdk/mpr/parser_microflow.go) — `microflowActionParsers` registers both names per handler (strict-write/lenient-read)
- [../../sdk/mpr/writer_odata.go](../../sdk/mpr/writer_odata.go) — `qualifyMemberName` / `qualifyAssociationName` enforce qualified field-key values; comment block records the verified `ConfigurationMicroflow` key after multiple wrong guesses
- [../../sdk/mpr/parser_odata.go](../../sdk/mpr/parser_odata.go) — round-trip read of the single-field discriminator design
- [[association-pointers]] — another counter-intuitive BSON naming invariant
- [[../bug-patterns/widget-type-object-drift]] — a related "looks valid, fails on open" failure mode
- [fix-issue symptom table](../../.claude/skills/fix-issue.md) — per-instance recipes for "Studio Pro shows default value despite explicit MDL"
