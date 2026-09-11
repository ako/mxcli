---
description: Rebuild the catalog database used for querying project metadata
argument-hint: [full]
---

# Refresh Catalog

Rebuild the catalog database for querying project metadata.

## Commands

```sql
-- Fast mode (metadata only, ~5 seconds)
REFRESH CATALOG;

-- Full mode (includes activities, widgets, and strings FTS table)
REFRESH CATALOG FULL;

-- Source mode (includes full + MDL source FTS table)
REFRESH CATALOG SOURCE;

-- Force rebuild (ignores cache)
REFRESH CATALOG FULL FORCE;
```

## When to Use

- After making changes to the project
- Before running catalog queries
- When exploring a new project
- Use FULL mode before using SEARCH command
- Use SOURCE mode for searching MDL definitions

The cache's mode is sticky. Once a project has a FULL or SOURCE cache, a plain
`REFRESH CATALOG` rebuilds at that level rather than dropping to FAST, and a
command that needs less (`mxcli check -p`, `show structure`, `describe`) never
replaces it with a narrower one. To go back to a cheaper level, delete
`.mxcli/catalog.db` and refresh.

## Example Queries After Refresh

```sql
-- Find entities
SELECT Name, AttributeCount FROM CATALOG.ENTITIES WHERE ModuleName = 'Sales';

-- Find microflows
SELECT QualifiedName, ParameterCount FROM CATALOG.MICROFLOWS WHERE Name LIKE '%Customer%';

-- Find pages
SELECT Name, WidgetCount FROM CATALOG.PAGES;

-- Full-text search (requires FULL mode)
SEARCH 'validation';

-- Raw FTS query on strings
SELECT * FROM CATALOG.STRINGS WHERE strings MATCH 'error';

-- Raw FTS query on source (requires SOURCE mode)
SELECT * FROM CATALOG.SOURCE WHERE source MATCH 'CREATE ENTITY';
```
