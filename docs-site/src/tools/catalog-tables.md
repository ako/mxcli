# Available Tables

The catalog provides several tables that can be queried using standard SQL syntax. Use `SHOW CATALOG TABLES` to list all available tables in your catalog.

## Discovering Tables

```sql
SHOW CATALOG TABLES;
```

## Core Tables

### CATALOG.ENTITIES

Information about all entities in the project.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Entity name |
| `ModuleName` | Module containing the entity |
| `QualifiedName` | Full qualified name (Module.Entity) |
| `EntityType` | Persistent, Non-Persistent, View, External |
| `Description` | Documentation text |
| `AttributeCount` | Number of attributes |
| `AccessRuleCount` | Number of access rules defined |

```sql
SELECT Name, EntityType, AttributeCount
FROM CATALOG.ENTITIES
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

### CATALOG.ATTRIBUTES

Information about entity attributes.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Attribute name |
| `EntityId` | Parent entity ID |
| `AttributeType` | String, Integer, Decimal, Boolean, DateTime, etc. |

```sql
SELECT a.Name, a.AttributeType
FROM CATALOG.ATTRIBUTES a
JOIN CATALOG.ENTITIES e ON a.EntityId = e.Id
WHERE e.QualifiedName = 'Sales.Customer';
```

### CATALOG.ASSOCIATIONS

Information about entity associations.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Association name |
| `ParentEntity` | Parent (FROM) entity qualified name |
| `ChildEntity` | Child (TO) entity qualified name |
| `AssociationType` | Reference or ReferenceSet |

```sql
SELECT Name, ParentEntity, ChildEntity, AssociationType
FROM CATALOG.ASSOCIATIONS
WHERE ParentEntity LIKE '%Order%' OR ChildEntity LIKE '%Order%';
```

### CATALOG.MICROFLOWS

Information about microflows and nanoflows.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Microflow name |
| `ModuleName` | Module containing the microflow |
| `QualifiedName` | Full qualified name |
| `ReturnType` | Return type of the microflow |
| `Description` | Documentation text |
| `Parameters` | Parameter information |
| `ObjectUsage` | Entities used in the microflow |

```sql
SELECT Name, ReturnType, Description
FROM CATALOG.MICROFLOWS
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

### CATALOG.PAGES

Information about pages and their properties.

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `Name` | Page name |
| `ModuleName` | Module containing the page |
| `QualifiedName` | Full qualified name |
| `URL` | Page URL if configured |
| `DataSource` | Primary data source |
| `WidgetTypes` | Types of widgets used |

```sql
SELECT Name, URL, DataSource
FROM CATALOG.PAGES
WHERE ModuleName = 'Sales'
ORDER BY Name;
```

### CATALOG.ACCESS_RULES

Information about entity access rules (available after full refresh).

| Column | Description |
|--------|-------------|
| `Id` | Unique identifier |
| `EntityId` | Entity this rule applies to |
| `UserRole` | Role this rule grants access to |
| `AllowRead` | Whether read access is granted |
| `AllowWrite` | Whether write access is granted |

```sql
SELECT e.QualifiedName, ar.UserRole, ar.AllowRead, ar.AllowWrite
FROM CATALOG.ACCESS_RULES ar
JOIN CATALOG.ENTITIES e ON ar.EntityId = e.Id
WHERE e.ModuleName = 'Sales';
```

### CATALOG.SCHEDULED_EVENTS

Scheduled events — Mendix's cron.

| Column | Description |
|--------|-------------|
| `Name`, `QualifiedName`, `ModuleName`, `Folder` | Identity |
| `Microflow` | Qualified name of the microflow the event runs |
| `Repeat` | Schedule variant: `Minute`, `Hour`, `Day`, `Week`, `MonthDate`, `MonthWeekday`, `YearDate`, `YearWeekday` |
| `RepeatDescription` | The schedule as a phrase, e.g. `weekly Mon/Fri at 09:30` |
| `IntervalSeconds` | Gap between runs, derived from the schedule |
| `Enabled` | 1 if the event runs |
| `TimeZone` | `UTC` or `Server` |
| `OnOverlap` | `DelayNext` or `SkipNext` |

`IntervalSeconds` comes from the schedule, **not** from the stored
`Interval`/`IntervalType` pair. Those are a legacy sibling that Studio Pro writes
and does not keep in sync — a shipped Mendix module stores `0`/`Minute` beside a
daily schedule — so a query keyed on them would read a nightly job as firing
every 0 seconds. Month and year figures are averages (30 and 365 days): the
column is for thresholds and ordering, not calendar arithmetic.

```sql
-- Anything that fires more often than once a minute
select QualifiedName, RepeatDescription, Microflow
from CATALOG.SCHEDULED_EVENTS
where Enabled = 1 and IntervalSeconds < 60;

-- Scheduled events whose microflow no longer exists
select se.QualifiedName, se.Microflow
from CATALOG.SCHEDULED_EVENTS se
left join CATALOG.MICROFLOWS m on m.QualifiedName = se.Microflow
where m.Id is null;
```

A scheduled event also produces a `schedule` row in `CATALOG.REFS`, so
`show callers of <microflow>` and the dead-asset analysis both see it. Without
that edge a microflow run only by a scheduled event looked unreferenced.

### CATALOG.QUEUES

Task queues.

| Column | Description |
|--------|-------------|
| `Name`, `QualifiedName`, `ModuleName`, `Folder` | Identity |
| `Parallelism` | How many tasks run at once — an **expression string**, not a number |
| `ClusterWide` | 1 if the limit applies across the cluster |

`Parallelism` is stored as text because Mendix stores an expression: a query must
not assume it parses as an integer.

```sql
select QualifiedName, Parallelism, ClusterWide from CATALOG.QUEUES;
```

## Graph-Analysis Tables

The dependency graph (`CATALOG.REFS`, full refresh) is analysed by a family of
`graph_*` views and tables — god nodes, module coupling/cohesion, dead documents,
communities, cycles, layers, centrality, and the integration surface. The
community/cycle/layer/centrality tables are populated by `REFRESH CATALOG
COMMUNITIES`. See **[Graph Analysis](graph-analysis.md)** for the full reference
and the `mxcli graph-report` command.

```sql
select * from CATALOG.graph_god_nodes order by Degree desc limit 20;
select * from CATALOG.graph_module_coupling order by Edges desc;
show communities;
```

## Listing All Tables

To see the complete list of available tables in your catalog (which may vary by project and refresh level):

```sql
SHOW CATALOG TABLES;
```

```bash
mxcli -p app.mpr -c "SHOW CATALOG TABLES"
```
