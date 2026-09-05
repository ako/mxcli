# SQLite Schema

The catalog uses an in-memory SQLite database with the following table definitions.

## Core Tables

### MODULES

```sql
CREATE TABLE MODULES (
    Name        TEXT PRIMARY KEY,
    ModuleID    TEXT,
    SortIndex   INTEGER
);
```

### ENTITIES

```sql
CREATE TABLE ENTITIES (
    Name            TEXT PRIMARY KEY,   -- Qualified: Module.Entity
    ModuleName      TEXT,
    EntityName      TEXT,
    Persistent      BOOLEAN,
    AttributeCount  INTEGER,
    Documentation   TEXT
);
```

### MICROFLOWS

```sql
CREATE TABLE MICROFLOWS (
    Name            TEXT PRIMARY KEY,   -- Qualified: Module.Microflow
    ModuleName      TEXT,
    MicroflowName   TEXT,
    ActivityCount   INTEGER,
    ParameterCount  INTEGER,
    ReturnType      TEXT,
    Folder          TEXT,
    Documentation   TEXT
);
```

### NANOFLOWS

```sql
CREATE TABLE NANOFLOWS (
    Name            TEXT PRIMARY KEY,
    ModuleName      TEXT,
    NanoflowName    TEXT,
    ActivityCount   INTEGER,
    Documentation   TEXT
);
```

### PAGES

```sql
CREATE TABLE PAGES (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    PageName    TEXT,
    Layout      TEXT,
    Url         TEXT,
    Documentation TEXT
);
```

### SNIPPETS

```sql
CREATE TABLE SNIPPETS (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    SnippetName TEXT
);
```

### ENUMERATIONS

```sql
CREATE TABLE ENUMERATIONS (
    Name        TEXT PRIMARY KEY,
    ModuleName  TEXT,
    EnumName    TEXT,
    ValueCount  INTEGER
);
```

### CONSTANTS

```sql
CREATE TABLE CONSTANTS (
    Id              TEXT PRIMARY KEY,
    Name            TEXT,
    QualifiedName   TEXT,
    ModuleName      TEXT,
    Folder          TEXT,
    Description     TEXT,
    DataType        TEXT,               -- String, Integer, Boolean, etc.
    DefaultValue    TEXT,
    ExposedToClient INTEGER DEFAULT 0
);
```

### CONSTANT_VALUES

Per-configuration constant overrides. Join with `CONSTANTS` on `ConstantName = QualifiedName`.

```sql
CREATE TABLE CONSTANT_VALUES (
    Id                INTEGER PRIMARY KEY AUTOINCREMENT,
    ConstantName      TEXT NOT NULL,     -- Qualified: Module.Constant
    ConfigurationName TEXT NOT NULL,     -- e.g., "Default", "Production"
    Value             TEXT
);
```

### WORKFLOWS

```sql
CREATE TABLE WORKFLOWS (
    Name            TEXT PRIMARY KEY,
    ModuleName      TEXT,
    WorkflowName    TEXT,
    ActivityCount   INTEGER
);
```

## Full Refresh Tables

These tables are only populated by `REFRESH CATALOG FULL`.

### ACTIVITIES

```sql
CREATE TABLE ACTIVITIES (
    DocumentName    TEXT,       -- Parent microflow/nanoflow
    ActivityType    TEXT,       -- e.g., "CreateObjectAction", "CallMicroflowAction"
    Caption         TEXT,       -- Activity caption/description
    SortOrder       INTEGER     -- Order within the flow
);
```

### WIDGETS

```sql
CREATE TABLE WIDGETS (
    DocumentName    TEXT,       -- Parent page/snippet
    WidgetName      TEXT,       -- Widget instance name
    WidgetType      TEXT,       -- e.g., "Forms$TextBox", "CustomWidgets$ComboBox"
    ModuleName      TEXT
);
```

### REFS

The reference graph: one row per edge. Populated by `refresh catalog full`.

```sql
CREATE TABLE REFS (
    Id          INTEGER PRIMARY KEY AUTOINCREMENT,
    SourceType  TEXT NOT NULL,  -- "MICROFLOW", "PAGE", "ENTITY", ...
    SourceId    TEXT NOT NULL,  -- element $ID, or '' where the builder has no id
    SourceName  TEXT NOT NULL,  -- referencing document, module-qualified
    TargetType  TEXT NOT NULL,  -- "ENTITY", "MICROFLOW", "WIDGET", ...
    TargetId    TEXT,           -- element $ID, or the widget ID for a WIDGET target
    TargetName  TEXT NOT NULL,  -- referenced element
    RefKind     TEXT NOT NULL,  -- see the vocabulary below
    ModuleName  TEXT,
    ProjectId   TEXT,
    SnapshotId  TEXT
);

CREATE INDEX idx_refs_source ON refs(SourceType, SourceName);
CREATE INDEX idx_refs_target ON refs(TargetType, TargetName);
CREATE INDEX idx_refs_kind   ON refs(RefKind);
```

`RefKind` values are lower-case, and the current vocabulary is whatever
`CATALOG.GRAPH_REFKIND_DISTRIBUTION` reports for your project — query that
rather than trusting a list here:

| RefKind | Edge |
|---------|------|
| `call` | flow calls a microflow / nanoflow / rule / Java action / REST operation |
| `create` / `change` / `delete` / `retrieve` | flow acts on an entity object |
| `return` | flow returns an entity type |
| `parameter` | page or flow parameter entity type |
| `generalize` | entity extends entity |
| `associate` | association targets entity |
| `layout` | page uses a layout |
| `datasource` | page or widget reads an entity |
| `action` | widget calls a microflow / nanoflow |
| `show_page` | flow or widget action opens a page |
| `home_page` / `login_page` / `menu_item` | navigation profile references a page |
| `calculate` | calculated attribute uses a microflow |
| `schedule` | scheduled event runs a microflow |
| `validate` | attribute validation rule uses a regular expression |
| `widget` | page or snippet uses a pluggable / custom widget |

#### WIDGET targets

A `widget` edge is the odd one out and is worth knowing about before you join
against it:

- `TargetName` is the widget's **MDL name** (`COMBOBOX`), not its dotted widget
  ID. The ID is in `TargetId`. A dotted target would be mis-read as a module by
  `GRAPH_MODULE_COUPLING` and friends, which take everything before the first
  dot as the module name.
- It is therefore the only `TargetName` that is not module-qualified — a widget
  definition belongs to no Mendix module. `GRAPH_GOD_NODES` excludes `WIDGET`
  targets from its asset list for that reason, while still counting a page's
  out-degree towards the widgets it uses.
- Only widgets with a definition get an edge. A built-in Mendix widget
  (`Forms$DynamicText`) has none, so it produces no row; use `CATALOG.WIDGETS`
  for those.
- One edge per page x widget, not per widget instance.

### PERMISSIONS

```sql
CREATE TABLE PERMISSIONS (
    RoleName    TEXT,           -- Module role
    TargetName  TEXT,           -- Entity, microflow, or page
    TargetKind  TEXT,           -- "Entity", "Microflow", "Page"
    Permission  TEXT            -- "Create", "Read", "Write", "Delete", "Execute", "View"
);
```

## Full-Text Search Tables

### STRINGS (FTS5)

```sql
CREATE VIRTUAL TABLE STRINGS USING fts5(
    QualifiedName,  -- Document qualified name, e.g. MyModule.Home
    ObjectType,     -- Document type, derived from the unit $Type: PAGE,
                    -- PAGE_TEMPLATE, BUILDING_BLOCK, MICROFLOW, ENUMERATION, ...
    StringValue,    -- The string itself
    StringContext,  -- Where it lives. For translatable text this is
                    -- <owner $Type>.<property>, e.g. Forms$ActionButton.Caption.
                    -- Non-translatable strings keep a plain label: page_url,
                    -- log_node, documentation, rest_path, task_name, ...
    Language,       -- Language code, empty for non-translatable strings
    ElementId,      -- The owning element's $ID — what distinguishes an
                    -- enumeration's twelve values from each other
    ModuleName
);
```

Every `Texts$Text` in the project is indexed, found by a type-agnostic walk
rather than per-document-type extraction, so a caption in a document type mxcli
cannot otherwise read is still searchable. That includes Atlas's design
templates (`PAGE_TEMPLATE`, `BUILDING_BLOCK`), which are roughly 70% of a stock
project's text and never render in a running app — filter them out with
`ObjectType` when you want only the app's own strings.

An empty translation — a text that exists but is not translated yet — is **not**
a row, so a language's presence in this table means it is actually translated
somewhere.

### SOURCE (FTS5)

```sql
CREATE VIRTUAL TABLE SOURCE USING fts5(
    name,           -- Document qualified name
    kind,           -- Document type
    source,         -- MDL source representation
    tokenize='porter unicode61'
);
```

## Querying Examples

```sql
-- Find entities with many attributes
SELECT Name, AttributeCount FROM CATALOG.ENTITIES
WHERE AttributeCount > 20 ORDER BY AttributeCount DESC;

-- Find all references to an entity
SELECT SourceName, RefKind FROM CATALOG.REFS
WHERE TargetName = 'Sales.Customer';

-- Which pages use a given pluggable widget?
SELECT SourceType, SourceName FROM CATALOG.REFS
WHERE RefKind = 'widget' AND TargetName = 'COMBOBOX';

-- Which installed widget packages does nothing use?
-- (MDL's SELECT has no NOT EXISTS / NOT IN — use an anti-join.)
SELECT d.MdlName, d.WidgetId FROM CATALOG.WIDGET_DEFINITIONS d
LEFT JOIN CATALOG.REFS r ON r.TargetId = d.WidgetId AND r.RefKind = 'widget'
WHERE r.Id IS NULL;

-- Full-text search
SELECT name, kind, snippet(STRINGS, 2, '<b>', '</b>', '...', 20)
FROM CATALOG.STRINGS WHERE strings MATCH 'validation error';

-- Find constants exposed to client
SELECT QualifiedName, DataType, DefaultValue FROM CATALOG.CONSTANTS
WHERE ExposedToClient = 1;

-- Compare constant values across configurations
SELECT c.QualifiedName, cv.ConfigurationName, cv.Value
FROM CATALOG.CONSTANTS c
JOIN CATALOG.CONSTANT_VALUES cv ON c.QualifiedName = cv.ConstantName
ORDER BY c.QualifiedName, cv.ConfigurationName;
```
