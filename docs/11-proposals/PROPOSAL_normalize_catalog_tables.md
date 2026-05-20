# Proposal: Normalize Catalog Tables

**Status:** Draft
**Date:** 2026-05-20
**Author:** Generated with Claude Code

## Problem

Every domain table in the local catalog (`mdl/catalog/tables.go`) carries the
same eight columns:

```
ProjectId, ProjectName,
SnapshotId, SnapshotDate, SnapshotSource,
SourceId, SourceBranch, SourceRevision
```

These are repeated across ~30 tables — `modules`, `entities`, `associations`,
`attributes`, `microflows`, `pages`, `snippets`, `layouts`, `enumerations`,
`java_actions`, `activities`, `widget_definitions`,
`widget_definition_properties`, `widgets`, `xpath_expressions`, `odata_clients`,
`odata_services`, `workflows`, `business_event_services`, `navigation_profiles`,
`rest_clients`, `rest_operations`, `published_rest_services`,
`published_rest_operations`, `external_entities`, `external_actions`,
`business_events`, `contract_entities`, `contract_actions`,
`contract_messages`, `database_connections`, `jar_dependencies`,
`constants`, `json_structures`, `import_mappings`, `export_mappings`.

The lookup tables (`projects`, `snapshots`) **already exist** as proper
normalized tables. Six of the eight columns are pure denormalization of
`snapshots`:

| Column           | Already on `snapshots` |
|------------------|------------------------|
| `ProjectName`    | yes                    |
| `SnapshotDate`   | yes                    |
| `SnapshotSource` | yes                    |
| `SourceId`       | yes                    |
| `SourceBranch`   | yes                    |
| `SourceRevision` | yes                    |

This costs:

1. **Storage** — six redundant TEXT columns × ~30 tables × N rows. On a
   medium project (~10k entities/attributes/widgets/activities) this is
   measurable.
2. **Write code** — every builder in `mdl/catalog/builder_*.go` has to thread
   snapshot metadata into every INSERT. `builder_modules.go` alone has 24
   references to these column names.
3. **Drift risk** — a row's `SnapshotDate` can disagree with its
   `snapshots.SnapshotDate` if a future builder forgets to update it. Single
   source of truth eliminates this class of bug entirely.
4. **Schema noise** — `tables.go` is 1082 lines, of which a large fraction is
   the same six column lines repeated.

## Investigation

Searched for consumers of the denormalized columns outside `mdl/catalog/`:

| Location                                       | Hits | Relevant? |
|------------------------------------------------|------|-----------|
| `mdl/catalog/` (Go)                            | ~70  | yes — builders + views |
| `mdl/linter/` (`ProjectName` field on report)  | 5    | no — unrelated Go struct |
| `mdl/executor/validate_duplicates.go`          | 1    | no — local function name |
| `mdl-examples/use-cases/02-agentic-search.mdl` | 4    | no — microflow parameter named `ProjectName` |
| `reference/mendix-repl/templates/` (user skills) | 0  | nothing user-facing depends on these columns |
| `docs/` (excluding archived proposals)         | 0    | nothing documented for users |

**Conclusion:** the denormalized columns have no external consumers. They
exist only because `tables.go` declared them and `builder_*.go` populates
them. Refactoring is internal.

## Design

Decisions captured during scoping:

1. **Drop six columns from every domain table.** Drop `ProjectName`,
   `SnapshotDate`, `SnapshotSource`, `SourceId`, `SourceBranch`,
   `SourceRevision`. Keep `ProjectId` and `SnapshotId` as FKs.
2. **`ProjectId` stays for direct filtering** without forcing a JOIN through
   `snapshots`. Pragmatic: one column of denormalization in exchange for
   `WHERE ProjectId = ?` ergonomics, which is the most common filter.
3. **Replace tables with views to preserve query compatibility.** Rename
   the underlying storage table (e.g. `entities` → `entities_data`) and
   create `entities` as a SQL view that JOINs `snapshots` and exposes the
   dropped columns. Existing queries — including the `objects` UNION view
   and any user-written `SELECT * FROM entities WHERE SnapshotSource = 'git'`
   — keep working unchanged.

### Schema, before vs after

Before (current `entities` table, abbreviated):

```sql
CREATE TABLE entities (
    Id TEXT PRIMARY KEY,
    Name TEXT,
    QualifiedName TEXT,
    ...
    ProjectId TEXT,
    ProjectName TEXT,
    SnapshotId TEXT,
    SnapshotDate TEXT,
    SnapshotSource TEXT,
    SourceId TEXT,
    SourceBranch TEXT,
    SourceRevision TEXT
)
```

After:

```sql
CREATE TABLE entities_data (
    Id TEXT PRIMARY KEY,
    Name TEXT,
    QualifiedName TEXT,
    ...
    ProjectId TEXT,
    SnapshotId TEXT
)

CREATE VIEW entities AS
SELECT
    e.*,
    s.ProjectName,
    s.SnapshotDate,
    s.SnapshotSource,
    s.SourceId,
    s.SourceBranch,
    s.SourceRevision
FROM entities_data e
LEFT JOIN snapshots s ON s.SnapshotId = e.SnapshotId
```

`LEFT JOIN` preserves rows even if a snapshot row is missing during a partial
build; the surfaced columns become NULL in that case, matching today's
behavior when a builder forgets to populate them.

### What this does NOT change

- The `objects` UNION view (continues to reference column names that the new
  views expose).
- Any MDL query users have written (`SELECT … FROM CATALOG.entities …`).
- Mendix BSON — this is purely a SQLite-side refactor.
- The `projects` and `snapshots` lookup tables — unchanged.

## Implementation Plan

### Phase 1 — Schema migration

| File | Change |
|------|--------|
| `mdl/catalog/tables.go` | Rename each domain table to `<name>_data`; drop the six denormalized columns from each; add a `CREATE VIEW <name> AS SELECT * JOIN snapshots` for each |
| `mdl/catalog/catalog.go` | Bump `schemaVersion` (catalog DB is regenerated, not migrated in place — confirm with maintainer) |

### Phase 2 — Builder cleanup

| File | Change |
|------|--------|
| `mdl/catalog/builder.go` | Drop the helper that threads denormalized fields through every INSERT (if present) |
| `mdl/catalog/builder_modules.go` | Remove 6 columns × N INSERTs (24 hits today) |
| `mdl/catalog/builder_pages.go` | Same (8 hits) |
| `mdl/catalog/builder_rest.go` | Same (6 hits) |
| `mdl/catalog/builder_microflows.go` | Same (4 hits) |
| `mdl/catalog/builder_widget_definitions.go` | Same (4 hits) |
| `mdl/catalog/builder_navigation.go` | Same (4 hits) |
| `mdl/catalog/builder_contract.go` | Same (3 hits) |
| `mdl/catalog/builder_workflows.go` | Same (2 hits) |
| `mdl/catalog/builder_constants.go` | Same (2 hits) |
| `mdl/catalog/builder_associations.go` | Same (2 hits) |
| `mdl/catalog/builder_external.go` | Same (2 hits) |
| `mdl/catalog/builder_xpath.go` | Same (2 hits) |
| `mdl/catalog/builder_permissions.go` | Same (1 hit) |
| `mdl/catalog/builder_references.go` | Same (1 hit) |
| `mdl/catalog/builder_roles.go` | Same (1 hit) |

INSERTs target the new `*_data` tables. Each builder loses six bind
parameters; the SQL strings shrink accordingly.

### Phase 3 — Test updates

| File | Change |
|------|--------|
| `mdl/catalog/catalog_test.go` | Update INSERT-expectation tests to match the new schema; verify `SELECT * FROM entities` still surfaces all eight columns through the view |
| `mdl/catalog/builder_*_test.go` | Adjust any test that asserts on column counts or denormalized values |

Add one new test: a single snapshot update propagates correctly through the
views (i.e. updating `snapshots.SourceRevision` is visible via
`SELECT SourceRevision FROM entities …` immediately, without a rebuild).

### Phase 4 — Documentation

| File | Change |
|------|--------|
| `docs/01-project/MDL_QUICK_REFERENCE.md` | If catalog schema is documented, mention that `*_data` tables exist as the storage layer; users should query the views |
| `.claude/skills/explore-project.md` (or similar) | Same note, if it references the schema |

## Version Compatibility

Not applicable — this is a SQLite-side refactor of the local catalog cache.
It does not touch Mendix BSON, Mendix versions, or MPR format.

The catalog DB is a cache (not source-of-truth); existing caches will be
regenerated on next build. Bump `schemaVersion` to force regeneration.

## Test Plan

- [ ] `make test` passes after schema and builder changes
- [ ] `make build && ./bin/mxcli refresh catalog -p <fixture.mpr>` succeeds
- [ ] `./bin/mxcli -c "select * from entities limit 5"` returns rows with all
      eight historical columns populated (now via JOIN)
- [ ] `./bin/mxcli -c "select * from objects limit 5"` (UNION view) works
- [ ] `./bin/mxcli -c "select count(*) from entities where SnapshotSource = 'git'"`
      works without modification
- [ ] DB file size on a known fixture is meaningfully smaller (record before
      and after — order-of-magnitude check, not strict assertion)
- [ ] `mxcli lint`, `mxcli report` still pass — they query catalog tables

## Open Questions

1. **Schema migration vs regeneration.** The catalog is described as a cache
   in `mdl/catalog/catalog.go`. Confirm with the maintainer that bumping
   `schemaVersion` and forcing rebuild is acceptable, or whether an in-place
   `ALTER TABLE` migration is preferred.
2. **Naming of the storage table.** This proposal uses `entities_data` for
   the underlying table and `entities` for the view. Alternatives:
   `_entities` (leading underscore = "internal"), `entities_raw`,
   `entities_t`. Pick one convention before implementing.
3. **`navigation_menu_items`, `navigation_role_homes`, `role_mappings`,
   `permissions`, `refs`, `constant_values`** already use only
   `ProjectId` + `SnapshotId` (no denormalized columns). Leave them alone or
   wrap them in views for consistency? Recommendation: leave alone — they
   are already clean.
4. **`objects` UNION view performance.** Today it reads denormalized columns
   directly; after refactor it reads through 20+ JOINs (one per UNION arm).
   SQLite typically inlines this fine, but worth measuring on a large
   fixture before merging.
