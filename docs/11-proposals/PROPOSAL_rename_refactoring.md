---
title: RENAME with Reference Refactoring
status: draft
date: 2026-04-08
---

# Proposal: RENAME with Reference Refactoring

**Status:** Draft
**Date:** 2026-04-08

## Motivation

Renaming entities, microflows, pages, and modules is one of the most common refactoring operations. Currently mxcli has `rename entity` and `rename module` in the grammar but neither is implemented. Renaming in Mendix is dangerous because cross-references are stored as qualified name strings (BY_NAME_REFERENCE) — renaming without updating references breaks the project.

A safe RENAME command that automatically updates all references would be a significant productivity gain, especially for AI-assisted refactoring workflows (e.g., monolith-to-multi-app decomposition).

## Current State

### What exists

| Operation | Status |
|-----------|--------|
| `alter entity ... rename attribute Old to New` | Works |
| `alter enumeration ... rename value Old to New` | Works |
| `rename entity Module.Old to New` | Grammar only, not implemented |
| `rename module Old to New` | Grammar only, not implemented |
| `rename microflow/page/nanoflow ...` | Not in grammar |

### Existing reference-update code

The codebase already has precedent for scanning and rewriting BY_NAME references:

- `UpdateEnumerationRefsInAllDomainModels()` in `writer_domainmodel.go` — scans all entities to update enumeration qualified names when an enum moves between modules
- `MoveEntity()` in `writer_domainmodel.go` — updates validation rule attribute references when an entity is moved

### How references are stored

Mendix MPR files use two reference types:

| Type | Storage | Example | Rename Impact |
|------|---------|---------|---------------|
| BY_ID_REFERENCE | Binary UUID | Association parent/child pointers, index attribute pointers | **Safe** — IDs survive rename |
| BY_NAME_REFERENCE | Qualified name string | `"Module.EntityName"`, `"Module.MicroflowName"` | **Breaks** — string must be updated |

All cross-document references use BY_NAME_REFERENCE. This is the fundamental challenge.

## Design

### Syntax

```sql
-- Rename entity (updates all references)
rename entity Module.OldName to NewName;

-- Rename microflow
rename microflow Module.OldName to NewName;

-- Rename nanoflow
rename nanoflow Module.OldName to NewName;

-- Rename page
rename page Module.OldName to NewName;

-- Rename module (updates ALL qualified names)
rename module OldName to NewName;

-- Dry run: show what would change without modifying anything
rename entity Module.OldName to NewName dry run;
```

The `to` target is always just a name (not qualified) — the document stays in the same module. Cross-module moves are handled by the existing `move` command.

### Reference Update Matrix

When renaming `Module.OldEntity` to `Module.NewEntity`, these BY_NAME references must be updated:

| Location | Reference Field | Example |
|----------|----------------|---------|
| **Microflow actions** | Entity in CREATE/RETRIEVE/DELETE/CHANGE/AGGREGATE/LIST | `"Module.OldEntity"` → `"Module.NewEntity"` |
| **Microflow parameters** | Entity type reference | `$Param: Module.OldEntity` |
| **Microflow return types** | Entity type reference | `returns Module.OldEntity` |
| **Associations** | Cross-module child/parent refs | `"Module.OldEntity"` in `Child` field |
| **Generalization** | Parent entity reference | `extends Module.OldEntity` |
| **Enumeration attributes** | EnumerationRef on attribute types | `"Module.OldEnum"` |
| **View entity OQL** | Entity names in SELECT/FROM/JOIN | `from Module.OldEntity as e` |
| **Page datasources** | Entity in DATABASE/XPATH sources | `datasource: database Module.OldEntity` |
| **Page parameter types** | Entity type in page params | `params: { $p: Module.OldEntity }` |
| **Navigation** | Home page, login page refs | `"Module.OldPage"` |
| **Settings** | After-startup, before-shutdown microflow | `"Module.OldMicroflow"` |
| **Security** | Allowed module roles on microflows | (BY_NAME refs) |
| **Member access rules** | Attribute/association names | Column names in access rules |
| **Import/Export mappings** | Entity references in element mappings | `"Module.OldEntity"` |
| **Business events** | Entity references in messages | `"Module.OldEntity"` |
| **Java action parameters** | Entity type references | `"Module.OldEntity"` |
| **Scheduled events** | Microflow references | `"Module.OldMicroflow"` |

### Architecture: Unified Reference Scanner

Instead of writing one-off update functions per document type, implement a **generic reference scanner** that:

1. Reads every document in the project (all units from the MPR)
2. Walks the BSON tree looking for string values matching the old qualified name
3. Replaces matching strings with the new qualified name
4. Rewrites the modified documents

This brute-force approach is safe because:
- BY_NAME references are always stored as plain strings in BSON
- The qualified name format (`Module.Name`) is unambiguous — no false positives from substring matches
- It catches references we haven't explicitly listed (future-proof)

```go
// RenameReferences scans all documents and replaces qualified name strings.
func (w *Writer) RenameReferences(oldQualifiedName, newQualifiedName string) (int, error) {
    units, err := w.reader.ListRawUnits()
    if err != nil {
        return 0, err
    }

    count := 0
    for _, unit := range units {
        raw, err := bson.Unmarshal(unit.Contents)
        if err != nil {
            continue
        }
        if updated, changed := replaceStringValues(raw, oldQualifiedName, newQualifiedName); changed {
            contents, _ := bson.Marshal(updated)
            w.updateUnitContents(unit.ID, contents)
            count++
        }
    }
    return count, nil
}
```

The `replaceStringValues` function recursively walks the BSON document tree and replaces exact string matches. It also handles partial matches for attribute-qualified names (e.g., `Module.Entity.Attribute` when renaming `Module.Entity`).

### Matching Rules

For a rename of `Module.OldName` to `Module.NewName`:

| Pattern | Match? | Example |
|---------|--------|---------|
| Exact: `"Module.OldName"` | Yes | Entity ref in microflow action |
| Prefix: `"Module.OldName.Attribute"` | Yes → `"Module.NewName.Attribute"` | Validation rule attribute ref |
| Substring: `"SomeModule.OldName"` | No | Different module, different entity |
| Substring: `"Module.OldNameExtra"` | No | Longer name, not a match |

The match is: string equals `oldName` OR string starts with `oldName + "."`.

### Dry Run Mode

`dry run` scans without modifying, outputting what would change:

```
rename entity MyModule.Customer to client dry run;

Would rename: MyModule.Customer → MyModule.Client
references found: 23
  MyModule.ACT_Customer_Save (microflow) — 3 references
  MyModule.ACT_Customer_Delete (microflow) — 2 references
  MyModule.Customer_Overview (page) — 4 references
  MyModule.Customer_Edit (page) — 5 references
  MyModule.Order (entity) — 1 reference (association)
  MyModule.IMM_CustomerResponse (import mapping) — 2 references
  ...
```

This lets users preview the blast radius before committing.

### Module Rename

Module rename is the most impactful — every qualified name starting with `OldModule.` must change. The same scanner works, but matches on the prefix `OldModule.` and replaces with `NewModule.`.

Additionally, module rename must update:
- The module document's `Name` field
- The `themesource/oldmodule/` directory (rename to `themesource/newmodule/`)
- The `javasource/oldmodule/` directory
- Folder container names in the hierarchy

## Implementation Plan

### Phase 1: Reference scanner + dry run

1. Implement `RenameReferences(old, new string) ([]RenameHit, error)` in `sdk/mpr/writer_rename.go`
2. Implement `replaceStringValues()` BSON tree walker
3. Add `dry run` support to show affected documents without modifying
4. Wire `rename entity` and `rename module` grammar rules to AST + visitor + executor
5. Executor calls dry-run scanner and reports results

**Deliverable**: `rename entity Module.Old to New dry run;` works and lists all references.

### Phase 2: Entity and enumeration rename

6. Implement actual rename: update entity Name + run reference scanner
7. Handle attribute-qualified names (`Module.Entity.Attribute` patterns)
8. Handle OQL queries in ViewEntitySourceDocuments (string replacement in OQL text)
9. Test with roundtrip: rename → `mx check` → 0 errors
10. Add `rename enumeration` support

### Phase 3: Microflow, nanoflow, page rename

11. Implement `rename microflow/nanoflow/page` in grammar + executor
12. Update navigation references (home pages, login pages)
13. Update settings references (after-startup, before-shutdown)
14. Update scheduled event microflow references

### Phase 4: Module rename

15. Implement `rename module` — prefix replacement on all qualified names
16. Handle filesystem directories (themesource, javasource)
17. Handle the module security document name
18. Handle folder container hierarchy updates

### Phase 5: Association and constant rename

19. `rename association Module.Old to New`
20. `rename constant Module.Old to New`

## Risks

### False positives in string replacement

The brute-force scanner replaces any BSON string matching the qualified name. In theory, a string literal in a microflow expression could contain `"Module.EntityName"` as user-visible text. This is extremely unlikely (qualified names are an internal format) but possible.

**Mitigation**: The dry-run mode lets users review before applying. We could also skip known text-only fields (Documentation, Caption) if false positives become an issue.

### OQL query rewriting

OQL queries reference entities as `Module.Entity` in FROM/JOIN clauses. Simple string replacement works for entity renames, but module renames need care — the OQL alias (`as e`) stays the same, only the qualified name changes.

### Java source files

Renaming an entity changes the proxy class name in `javasource/<module>/proxies/`. Java source files that reference the old class name will break. This is out of scope for Phase 1-3 but should be documented as a limitation.

## Scope Exclusions

- **Java source file updates**: Out of scope — rename produces correct MPR but Java files need manual update
- **Widget property string references**: Pluggable widget properties may contain entity/attribute names as strings — these are not updatable without widget-specific knowledge
- **Git history**: Rename doesn't create a git rename operation — it modifies files in place
- **Cross-project references**: Only single-project rename (multi-project is deferred to multi-app support)

## Effort Estimate

- Phase 1 (scanner + dry run): Small — ~150 lines Go
- Phase 2 (entity rename): Medium — ~100 lines Go + tests
- Phase 3 (microflow/page rename): Small — reuses Phase 2 scanner
- Phase 4 (module rename): Medium — filesystem + prefix replacement
- Phase 5 (association/constant): Small — reuses scanner
