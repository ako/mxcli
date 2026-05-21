# Changelog

All notable changes to mxcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.11.0] - 2026-05-21

### Added

- **Pluggable widget property validation** — `mxcli check` flags unknown widget property keys as `MDL-WIDGET01`; respects MDL builtin property names (e.g. `Label`, `Caption`, `DataSource`) so they aren't reported as typos
- **`mxcli check --post-migration`** — scans for legacy native widgets in pages/snippets and reports `MDL-WIDGET02` with qualified module.document names; version-gated via the legacy-widget catalog
- **LSP widget integration** — completion suggests widget property keys inside `(...)` blocks; hover shows property descriptions extracted from `.mpk`; widget property typos surface as real-time diagnostics
- **Widget definition workflow** — `widget init --force` re-extracts existing `.def.json` files; `widget init` and `refresh catalog` auto-refresh stale definitions; `mxcli init` now runs `widget init` so new projects pick up widget defs automatically
- **Catalog: widget tables** — `widget_definitions` and `widget_definition_properties` queryable via `SELECT ... FROM CATALOG.widget_definitions`
- **`ALTER` for agent-editor documents** — `ALTER AGENT/MODEL/KNOWLEDGEBASE/MCPSERVICE` (#464)
- **Skill docs include MDL keyword routing** — generated widget skill files document object-list and child-slot keywords driven from `.def.json`

### Fixed

- DataGrid2 column `tooltip` / `exportValue` / `dynamicText` TextTemplate now matches Studio Pro's per-column-kind convention (CE0463 on attribute-bound columns, #578)
- DataGrid2 column `CaptionParams` / `ContentParams` / `ShowContentAs` / `Content` roundtrip (#547); `$localVar` references in column captions emit `Forms$PageVariable.LocalVariable`
- Pluggable widget engine wrote `CustomWidgets$AttributeRef` (not a registered Mendix type); now emits `DomainModels$AttributeRef` with the fully-qualified path so `mx update-widgets` no longer fails with `TypeCacheUnknownTypeException` (#64)
- Object-list item TextTemplate slots emit `null` when unset (Accordion `groups`, Maps `markers`, AreaChart `series`, PopupMenu items) instead of placeholder ClientTemplate that triggered CE0463 (#548)
- Pluggable widget CE0463 on Mendix 11.9 — `FormattingInfo TimeFormat` + `Selection` PascalCase normalization
- DataView `FormOrientation` / `LabelWidth` now controllable from MDL (#554)
- `ALTER PAGE` fixes: `INSERT`/`REPLACE` serializes DataGrid2 columns correctly; `set Title` actually updates the title (#561); column `SET` is case-insensitive and supports TextTemplate captions (#560); column inserts use the grid's data source as entity context
- Master-detail page round-trip — Gallery `ItemSelectionMode` + DataView selection-source described correctly
- `DesktopWidth` / `TabletWidth` / `PhoneWidth`: `AutoFill` now actually sets `Weight: -1` instead of dropping the override
- Pluggable widget validator respects MDL builtin property names (no false positives on `Label:`, `Caption:`, `DataSource:`, etc.)
- `mxcli check` detects custom-content column INSERT issues before MxBuild
- `--references` no longer flags `DROP + CREATE` of the same name as a conflict
- Reject Mendix reserved words on non-persistent entity attributes (#552)
- Cached catalog applies the current schema on load (no more "no such table" after schema bump)
- Nightly CE0117 on Mendix 10.24.19 — drop redundant `toString()` on string parameter

### Changed

- Test infrastructure: `TestMain` runs `widget init` on the shared source project so per-test copies inherit `.def.json` files; integration tests now exercise pluggable widget fixtures end-to-end
- Robust cleanup for doctype/mx-check tests eliminates ENOTEMPTY flake on CI
- `modernc.org/sqlite` bumped from 1.50.0 to 1.50.1

### Known limitations

- Two CE0463 cases remain for widgets with property-conditional TextTemplate visibility (VideoPlayer with `type='expression'`, Timeline with `customVisualization='true'`). Root cause and proposal in `docs/11-proposals/PROPOSAL_widget_property_visibility.md`; tracked under #574
- `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'` form is less feature-complete than the `datagrid` keyword form (no CONTROLBAR/customContent/per-column filter routing). Tracked under #529 Phase 4

## [0.10.0] - 2026-05-12

### Added

- **Maven/JAR dependency management** — `CREATE/DROP/SHOW JAR DEPENDENCY` statements; `jar_dependencies` catalog table; skill and docs-site pages (MDL-JARDEP)
- **Object-list pluggable widget properties** — grammar keywords for object-list blocks, extraction to `.def.json` (Phase 1), and BSON serialization through the executor (Phase 1 Layer 3)
- **LEGACYDATAGRID grammar** — keyword dispatch table and `LEGACYDATAGRID` grammar rule (Phase 2 pluggable widget overhaul)
- **`AllowCreateChangeLocally` flag** — exposed on external OData entities (#534)
- **Catalog: contract_entities → external_entities link** — cross-reference between contract catalog and integration catalog
- **`not(expr)` grammar enforcement** — grammar now requires parenthesised form; bare `not expr` rejected with CE0117 diagnostic

### Fixed

- `mxcli fmt` exits 1 on unparseable input and pipes describe output correctly (#398)
- ALTER SNIPPET failing with "page not found" (#402)
- `SHOW CONTEXT OF` entity showing empty definition (#396)
- `CREATE ENTITY` rejects unknown attribute type names (#392)
- `CREATE ENUMERATION` rejects duplicate value names (#390)
- `DROP ENUMERATION` errors on ambiguous unqualified name (#391)
- `CREATE ASSOCIATION` rejects duplicates for cross-module associations (#389)
- `GRANT/REVOKE ON ENTITY` validates module roles (#399)
- Enum XPath comparisons stored as string literals instead of enum refs (#176)
- Catalog crash on duplicate OData contract entities/actions
- `CATALOG.JAR_DEPENDENCIES` missing from `Tables()` list
- Three nightly CI failures on Mendix 10.24
- DataGrid2 `WidgetObject` boolean defaults aligned with `PropertyType` schema
- `TextTemplate` translation defaults populated; `Editable=Always` set on filters
- Required `CustomWidget` envelope fields added to filter widgets
- `WidgetObject Properties` reordered to match `WidgetType PropertyTypes` order
- `AllowUpload` field added to `WidgetValueType` BSON (closes one CE0463 gap)
- Unique placeholder IDs for `TextTemplate` translations (#30)
- Two ALTER PAGE bugs caught in test feedback
- ComboBox CE0463 — guard auto-populate and null `selectAllButtonCaption`
- Grammar added as explicit dependency of `build`, `test`, and `release` targets

## [0.9.0] - 2026-05-08

### Added

- **Inheritance split and cast** — `CASE $var IS Module.SubType THEN ... END CASE` and `CAST $var AS Module.SubType` statements in microflow/nanoflow bodies; full BSON roundtrip with branch anchors, nested continuation cases, and merge emission (CE0079)
- **CREATE OR MODIFY** — Standardised `OR MODIFY` variant across all remaining document types so scripts are idempotent by default (#510)
- **MDL-DUPDEF** — `mxcli check` detects duplicate `CREATE` for the same qualified name and reports `MDL-DUPDEF`

### Fixed

- Catalog crash on duplicate business event channels (#533)
- `flowRefCollector` skipping EnumSplitStmt case and else bodies — impacted `show callers/callees` accuracy
- CE0079: inheritance split branches that continue after the split were missing their merge node
- Nested `traverseFlowUntilMerge` guard could cross an outer merge boundary (#528)
- Inheritance split: branch anchors, case order, nested continuation tails, and nodes outside cases all preserved
- List-typed Java action arguments not emitting the `empty` keyword (#521); broadened to cover all resolved `BasicParameterType` params
- REST mapping cardinality not roundtripping — `as list of` syntax now parsed and emitted (#519)
- Import mapping: `MinOccurs`/`MaxOccurs` not parsed on mapping elements; repeating Object root treated as list; `SingleObject` inferred when `JsonStructure` absent
- Microflow layout: spacing, branch heights, and loop containment improved
- `TEXTFILTER` inside `DATAGRID COLUMN` not wired to the column filter slot (#189)
- `SET $obj/Assoc` path target rejected and produced wrong BSON (#511)
- `SHOW WIDGETS WHERE … LIKE` silently degraded to equality match
- Reserved OData attribute names not renamed when importing entities (#526)
- Virtual `System.*` Java actions missing from `ListJavaActions` and catalog
- `ConcurrencyMode=Fixed` incorrectly marked as Creatable during OData import (#525)
- Reverse-Reference traversal through entity inheritance misclassified
- `mxcli check --references` reporting false positives on `System.*` references (#523)
- ANTLR4 version unpinned in CI caused flaky Maven Central lookup failures

### Changed

- Generated ANTLR parser removed from git; `make grammar` step added to CI (#514)
- `MDLParser.g4` split into domain-specific grammar files for maintainability (#515)

## [0.8.0] - 2026-05-05

### Added

- **CREATE/DROP NANOFLOW** — Full nanoflow authoring pipeline: grammar, AST, visitor, executor, BSON writer, CALL NANOFLOW statement, GRANT/REVOKE nanoflow access, and nanoflow ELK diagram support in VS Code preview
- **CALL JAVASCRIPT ACTION** — `call javascript action Module.ActionName(params)` fully supported in CREATE NANOFLOW/MICROFLOW bodies: grammar, parser, builder, serializer, and roundtrip
- **CASE/WHEN enum split** — Enum-value split statements with `CASE $var WHEN Module.Value THEN ... END CASE` syntax; replaces the earlier `split on enum` draft
- **CALL WEB SERVICE (SOAP)** — Legacy SOAP microflow call statement with unsupported-detail preservation as raw BSON
- **RENAME WORKFLOW / RENAME MODULE** — RENAME now covers workflows and modules with reference refactoring
- **Ellipsis placeholder expression** — `...` as a placeholder in microflow expressions
- **Add-to-list expressions** — `add expression to $list` syntax in microflow/nanoflow bodies
- **Free microflow annotations** — Unattached `@annotation` nodes in microflow bodies survive describe → exec round-trip
- **@anchor sequence flow annotation** — `@anchor(from: X, to: Y)` on microflow statements pins SequenceFlow attachment sides; split and loop forms supported; builder-default and layout-equivalent anchors suppressed from DESCRIBE output
- **OpenAPI import for REST clients** — `CREATE REST CLIENT` accepts `OpenAPI: 'path/or/url'` to auto-generate a consumed REST service from an OpenAPI 3.0 spec (#207)
- **DESCRIBE CONTRACT OPERATION FROM OPENAPI** — Preview OpenAPI-generated operations without writing to the project
- **mxcli catalog search** — Search Mendix Catalog for data sources and services (#213)
- **Local file metadata for OData clients** — `CREATE ODATA CLIENT` supports `file://` URLs and relative paths for `MetadataUrl` (#206)
- **CATALOG.ASSOCIATIONS table** — Query association metadata via `select ... from CATALOG.ASSOCIATIONS` (#419)
- **SET format = json** — Session-level `SET key = value` command; `SET format = json` applies to all subsequent output
- **Java action improvements** — DROP/RENAME updates source file references; `void` qualified name resolved as VoidType; explicit void returns parsed correctly
- **SHOW LANGUAGES** — Language listing with Languages array parsing and executor handler (#480)
- **VS Code extension** — LSP coverage extended to all document types (nanoflows, workflows, Java actions, JSON structures, import/export mappings)
- **LSP snippet completions** — `CREATE NANOFLOW`, `CALL MICROFLOW`, `CALL NANOFLOW`, `CALL JAVASCRIPT ACTION`, `CALL JAVA ACTION` snippets added
- **make check-mdl** — Fast doctype script syntax validation target; wired into CI
- **Nanoflow diff support** — `mxcli diff` detects and displays nanoflow changes
- **Nanoflow validation parity** — `mxcli check` runs full body validation on nanoflows via shared `validateFlowBody` helper

### Fixed

- SIGSEGV in `buildPublishedRestResourceDef` on malformed REST syntax (#429)
- nil panic in ALTER WORKFLOW when activity ref is missing or uses a keyword (#430)
- Single quotes not escaped in DESCRIBE ENTITY XPath output (#431)
- `diff-local` git-error propagation and regression tests (#424)
- DataGrid2 column name derivation for ALTER PAGE (#116)
- O(N²) `GetMicroflow`/`GetNanoflow` replaced with direct unit lookup (#397)
- `CALL MICROFLOW`/`CALL NANOFLOW` validates targets exist before writing model (#395)
- `mxcli new` exits 0 on download failure (#422)
- Reject obviously malformed `MetadataUrl` in CREATE ODATA CLIENT (#427)
- Rename commands reject collisions with existing names (#432)
- Exit codes and error messages for marketplace, eval list, widget init, TUI (#425)
- `connect`/`disconnect`/`status` registered in syntax registry (#441)
- `resolveSnippetRef` checks session cache before querying backend (#509)
- DESCRIBE WORKFLOW output was missing the `CREATE` keyword (#478)
- RENAME MODULE failed due to uppercase ObjectType comparison in visitor (#473)
- JSON structure qualified-name lookup through folder hierarchy (#508)
- Retry-style error handler tail now loops back to a merge before the source (#507)
- Cross-module associations preserved on CREATE object actions (#502)
- Negative annotation coordinates parsed correctly (#494)
- Multiple retrieve XPath predicates preserved (#500)
- Custom error handler routing, empty else branch preservation, and structured conditional emit (#366)
- Validation feedback targets preserved with fully-qualified association paths (#359)
- Mapping result range cardinality and explicit REST mapping output variables (#372)
- SNIPPETCALL on parameterised snippets no longer corrupts model
- SHOW_PAGE button actions no longer produce null `PageParameterMapping.Variable` (#295)
- `Forms$SnippetParameterMapping` used for snippet call parameter mappings
- Marketplace search applies client-side filtering (#479)
- Recursion depth limit added to EXECUTE SCRIPT (#472)
- `CATALOG.ASSOCIATIONS`/`CONSTANTS`/`OBJECTS` returning no rows (#419)

### Changed

- **MDL string literal escapes** — `\n`, `\r`, `\t`, `\\` inside single-quoted literals are now escape sequences. Scripts embedding raw backslash sequences must double the backslash.
- **CatalogDB/CatalogTx interfaces** — Catalog, Builder, and LintContext migrated to interface; SQLite implementation extracted to `catalogdb_sqlite.go`
- **LintReader interface** — `sdk/mpr` removed from linter and executor; all reads go through `LintReader`
- **Type-safe BSON helpers** — `bsonString`/`bsonBool` consolidated in `mdl/bsonutil` package

## [0.7.0] - 2026-04-21

### Added

- **Agent Editor** — CREATE/DROP Agent, Knowledge Base, Consumed MCP Service, and Model documents; read support for all four types; DESCRIBE MODULE WITH ALL includes agent-editor documents
- **Consumed REST Client v2** — Redesigned syntax with full mapping support, parameter support for SEND REST REQUEST, BODY JSON FROM clause roundtrip, and TRANSFORM microflow action (JSLT/XSLT, Mendix 11.9+)
- **Platform Authentication** — `mxcli auth login/logout/status/list` with PAT scheme for mendix.com; credentials stored at `~/.mxcli/auth.json` (mode 0600), MENDIX_PAT env override
- **Marketplace Browsing** — `mxcli marketplace search/info/versions` with `--min-mendix` compatibility filtering
- **Entity Event Handlers** — Full MDL support for before/after create/change/delete event handlers with entity parameter validation
- **System Attributes** — AutoOwner, AutoChangedBy, and other audit pseudo-types; ALTER ENTITY ADD/DROP ATTRIBUTE for system attributes
- **ALTER PUBLISHED REST SERVICE** — Full in-place modification of published REST services (#161)
- **GRANT/REVOKE ACCESS on PUBLISHED REST SERVICE** (#162)
- **GitHub Copilot support** — First-class Copilot integration in `mxcli init`
- **Unified --json output** — All commands support structured JSON output (#134); `mxcli check --format json/sarif` outputs structured results
- **OData TripPin bulk-import** — Executable bulk-import example with @Constant syntax for ServiceUrl
- **Backend Abstraction** — `ExecContext` with typed backend interfaces, dispatch registry replacing type-switch, mutation backends (`page_mutator`, `widget_builder`, `datagrid_builder`, `workflow_mutator`) decoupled from `sdk/mpr`
- **mdl/types package** — Shared types and utilities extracted from `sdk/mpr` (EDMX, AsyncAPI, ID, navigation, infrastructure, JSON utils)
- **bsonutil package** — BSON utility functions (IDToBsonBinary, BsonBinaryToID, NewIDBsonBinary)
- **Mock-based handler tests** — 189 tests across 33 files covering all executor command handlers
- **OperationRegistry extensibility** — Pluggable operation registry with ContainerSnippet constant

### Fixed

- REST client BASIC auth uses correct `Rest$ConstantValue` BSON key (#200)
- ConnectionIndex lost on roundtrip (int64 vs int32 type mismatch) (#204)
- OData: ByAssociation DataSource serialization for DataGrid 2, capability annotations for entity/association CRUD (#201), bulk-create NPEs for primitive collections, derived/abstract/contained entities, and navigation associations (#143)
- UUID v4 version/variant bits in `GenerateDeterministicID`; panic on invalid UUID in `IDToBsonBinary`
- Cascade-delete associations on DROP ENTITY and DROP ODATA CLIENT
- Reserved keywords now allowed as module names in CREATE MODULE
- Quoted identifiers accepted in CREATE MODULE
- Find, Filter, ListRange list operations parsed and rendered (#212)
- DESCRIBE REST CLIENT resolves constant credentials to literal values (#192)
- DESCRIBE microflow roundtrip issues; eliminate redundant Merge nodes when IF branch returns
- COLUMN name falls back to attribute + scope association lookup by module (#202)
- Schema-level external `<Annotations>` blocks parsed in OData $metadata
- OData ServiceUrl validated as constant reference
- Agent-editor commands conformed to backend abstraction

### Changed

- Executor fully decoupled from storage layer — all BSON writes go through mutation backends (PRs #225, #237, #238, #239)
- All executor handlers migrated to free functions using `ExecContext` (removed 233 unused wrapper methods)
- `show*` executor functions renamed to `list*` for consistency
- Type aliases added in `sdk/mpr` for backward compatibility after shared-type extraction

## [0.6.0] - 2026-04-09

### Added

- **RENAME** — Automatic reference refactoring when renaming entities, attributes, associations, and other elements
- **CREATE EXTERNAL ENTITIES** — Bulk import entities from OData contracts (#143)
- **@excluded Annotation** — Mark documents and microflow activities as excluded, with Excluded column in catalog and `[EXCLUDED]` indicator in LIST
- **LIST Alias** — LIST as alias for SHOW in MDL and CLI
- **ALTER WORKFLOW** — Full activity manipulation (INSERT, DROP, REPLACE) for workflow definitions
- **Primitive Page Parameters** — Support for String, Integer, and other primitive types in page parameters
- **DataGrid Column Targeting** — Addressable columns in ALTER PAGE via dotted refs (e.g., `DataGrid.ColumnName`)
- **diff-local --ref** — Accept git ranges directly via `--ref` for comparing arbitrary revisions
- **Virtual System Module** — Complete module listing including System module
- **PasswordPolicy.ValidatePassword** — Demo user password validation against project policy
- **Multiple XPath Predicates** — Support `[cond1][cond2]` in WHERE clauses
- **DESCRIBE Enhancements** — Missing types added to mxcli describe command, view entity Source object preservation
- **Proposals** — Bulk external action support from OData contracts, RENAME with reference refactoring

### Fixed

- INTO clause in CREATE EXTERNAL ENTITIES not routing to target module
- Mendix 11.9.0 integration test failures
- Demo user password updated to meet 12-char policy
- JSON number type inference and mxcli new locale duplicates
- BSON properties aligned with Mendix schema for mx diff compatibility
- View entity Source object ID preserved with CREATE OR MODIFY in DESCRIBE

### Changed

- Refactored large files: executor.go (4 files), init.go (3 files), tui/app.go (4 files), cmd_entities.go (3 files)
- Simplified diff-local to accept git ranges via `--ref` directly (removed `--base` flag)
- Pre-warmed name lookup maps to eliminate O(n²) BSON parsing in catalog source
- Updated CI to test against Mendix 11.9.0
- Documentation updates: LIST preferred over SHOW, execution modes, DataGrid column targeting, IMAGE datasource properties

## [0.5.0] - 2026-04-06

### Added

- **Import/Export Mappings** — CREATE/DESCRIBE/DROP IMPORT MAPPING and EXPORT MAPPING with JSON Structure integration, array mapping, and BSON roundtrip
- **IMPORT FROM MAPPING / EXPORT TO MAPPING** — Microflow actions for mapping-based data transformation
- **JSON Structure FOLDER** — FOLDER clause for organizing JSON Structures into folders
- **DESCRIBE NANOFLOW** — Display nanoflow activities, control flows, and return type
- **Pluggable Widget Engine v2** — Redesigned widget engine with 25+ new widget templates (accordion, maps, charts, timeline, etc.), filter widget migration, and `generateDefJSON` property mapping
- **WidgetDemo** — Baseline scripts and widget analysis tools for widget testing
- **mxcli new** — Create Mendix projects from scratch (downloads MxBuild, creates project, runs init, installs Linux mxcli binary)
- **setup mxcli** — Download platform-specific mxcli binary from GitHub releases
- **Podman Support** — Podman as Docker alternative with devcontainer configuration (#34)
- **Catalog Tables** — Import/export mapping catalog tables for project metadata queries
- **Project Tree** — Missing document types added to project tree and syntax highlighting
- **GRANT Additive** — GRANT is now additive with partial REVOKE for entity access
- **Version Pre-checks** — Executor commands validate Mendix version before BSON writes
- **SHOW FEATURES** — Display version registry feature availability
- **SHOW LANGUAGES** — Language listing and QUAL005 missing translations linter rule
- **Proposals** — Design proposals for i18n, workflow improvements, and multi-project tree view
- **BSON Tooling Guide** — Contributor documentation for BSON debugging workflow
- **CONTRIBUTING.md** — Rewritten with accurate project references

### Fixed

- CE1613 and Studio Pro crash from invalid CrossAssociation BSON (ParentConnection/ChildConnection fields) (#50)
- Import/export mapping BSON alignment with Studio Pro (JsonPath, ExposedName, ObjectHandling, array elements)
- Sort translation map iteration in all serializers for deterministic output
- Docker and diaglog tests cross-platform compatibility (macOS Unix socket paths)
- Roundtrip test stability with idempotency strategy
- Version gates for Mendix 10.24 nightly test failures and 11.0+-only MOVE commands
- Nanoflow BSON parsing for activities, flows, and return type
- mxcli new MPR filename detection from create-project
- Bun setup in nightly and release workflows for vscode-ext build
- Replace unreleased Mendix 11.9.0 with 11.8.0 in CI workflows

### Changed

- Redesigned import/export mapping syntax (v2) with comma separators
- Bumped dependencies: esbuild 0.28.0, typescript 6.0.2, sqlite 1.48.1, go-runewidth 0.0.22, @vscode/vsce 3.7.1
- Bumped CI actions: checkout v6, deploy-pages v5, upload-pages-artifact v4
- Bumped mdbook to v0.5.2 with musl for aarch64
- PR review checklist requires working MDL examples for syntax changes

## [0.4.0] - 2026-03-31

### Added

- **SEND REST REQUEST** — Microflow action for consumed REST services with full BSON serialization roundtrip
- **Pluggable Image Widget** — Full roundtrip support for `com.mendix.widget.web.image.Image` with Studio Pro-extracted templates
- **ALTER PAGE SET Url** — Change page URLs via MDL
- **ALTER PAGE SET Layout** — Switch page layout via MDL
- **ALTER ENTITY SET POSITION** — Set entity position in domain model diagrams
- **VISIBLE IF / EDITABLE IF** — Conditional visibility and editability with XPath expressions, plus TabletWidth/PhoneWidth properties
- **EXECUTE DATABASE QUERY** — Microflow action for static, dynamic, and parameterized SQL with runtime connection override
- **Contract Browsing** — SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS from cached OData $metadata, CONTRACT CHANNELS/MESSAGES from AsyncAPI
- **Integration Catalog** — 7 new catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events, contract tables)
- **SHOW EXTERNAL ACTIONS / PUBLISHED REST SERVICES** — Integration pane commands
- **SHOW CONSTANT VALUES** — Display constant values and catalog tables
- **CREATE/DROP CONFIGURATION** — Configuration management with constant overrides
- **JavaScript Actions** — NDSL/MDL support for JavaScript action definitions
- **DROP/MOVE FOLDER** — Remove empty folders and reorganize project structure
- **GALLERY Columns** — DesktopColumns/TabletColumns/PhoneColumns properties
- **Forward-Reference Hints** — Helpful error messages when exec fails on later-defined objects
- **IMAGE FROM FILE** — Image collection syntax for file-based images
- **OpenSSF Baseline Level 1** — Security foundations and CodeQL fixes
- **Multi-Agent Merge Proposal** — Design proposal for parallel agent work on Mendix projects
- **Documentation Site** — mdBook-based site with tutorials, language reference, migration guide, and internals
- **Tool Integrations** — Added support for OpenCode, Mistral Vibe, and GitHub Copilot in `mxcli init`
- **TUI Enhancements** — Agent channel (Unix socket), UX improvements, auto-create module support
- **Custom Widget AIGC Skill** — Skill for AI-generated custom pluggable widgets
- **AI Issue Triage** — GitHub Actions workflow for automated issue classification
- **Daily Project Digest** — Scheduled workflow for project activity summaries

### Fixed

- Skip null TextTemplate in opTextTemplate to avoid CE0463 widget definition errors
- Set Editable to Conditional and fix Visible XPath expression serialization
- REST client BSON serialization field ordering and roundtrip correctness
- Image widget template extraction (imageObject defaults, Parameters version marker, Texts$Translation)
- Escape single quotes in page DESCRIBE output via `mdlQuote()`
- Resolve association/attribute and entity/enumeration ambiguity in MDL parser
- LSP diagnostics for editable `mendix-mdl://` documents
- Gallery CE0463 by re-extracting template and fixing augmentation
- DataGrid2 column name derivation from attribute or caption
- ComboBox association EntityRef via IndirectEntityRef with association path
- XPath tokens written unquoted to prevent CE0161
- Long type written as `DataTypes$LongType` instead of IntegerType
- Date as distinct type from DateTime throughout the pipeline
- MPR version detection using DB schema and `_FormatVersion` field
- Recurse into loop bodies when extracting catalog references
- CodeQL symlink path traversal alerts in tar extraction
- Multiple TUI data races and agent channel stability fixes

### Changed

- Bumped dependencies: pgx v5.9.1, zap v1.27.1, go-runewidth v0.0.21, cobra v1.10.2, mongo-driver v1.17.9, sqlite v1.48.0
- Refactored Visible/Editable syntax to `visible: [xpath]` and `editable: [xpath]`
- Used dedicated CWTest module in custom widget examples
- Always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Added scope & atomicity and documentation sections to PR review checklist

## [0.3.0] - 2026-03-26

### Added

- **TUI** — Interactive terminal UI (`mxcli tui`) with yazi-style Miller columns, BSON/MDL preview, search, tabs, command palette (`:` key), session restore (`-c`), and mouse support
- **Workflows** — Full CREATE/DESCRIBE WORKFLOW support with activities (UserTask, Decision, CallMicroflow, CallWorkflow, Jump, WaitForTimer, ParallelSplit, BoundaryEvent), BSON round-trip, and ANNOTATION statements
- **Consumed REST Clients** — SHOW/DESCRIBE/CREATE consumed REST services with BSON writer and mx check validation
- **Image Collections** — SHOW/DESCRIBE/CREATE/DROP IMAGE COLLECTION with BSON writer and Kitty/iTerm2/Sixel inline image rendering in TUI
- **WHILE Loops** — WHILE loop support in microflows with examples
- **ALTER PAGE Variables** — ALTER PAGE ADD/DROP VARIABLE support (Phase 3)
- **XPath** — Dedicated XPath expression grammar, catalog table population, and skills reference
- **BSON Tools** — `bson dump --format ndsl`, `bson compare` with smart array matching, `bson discover` for field coverage analysis
- **Documentation Site** — mdBook-based site with full language reference, tutorials, and internals documentation
- **Anti-pattern Detection** — `mxcli check` detects nested loops and empty list anti-patterns (issue #21)
- **CREATE OR MODIFY** — Additive upsert for USER ROLE and DEMO USER
- **AI PR Review** — GitHub Actions workflow using GitHub Models API for automated pull request review
- **RETRIEVE FROM $Variable** — Support for in-memory and NPE list association traversal (issue #22)
- **Constants** — Constant syntax help topic, LSP snippet, and CREATE OR MODIFY examples
- **UnknownElement Fallback** — Table-driven parser registries with graceful fallback for unrecognized BSON types (issue #19)

### Fixed

- MPR corruption from dangling GUIDs after attribute drop/add (#4)
- BSON field ordering loss in ALTER PAGE operations (#3)
- ALTER PAGE SET Attribute property support (issue #10)
- ALTER PAGE REPLACE deep GUID regeneration for stale $ID fields (issue #9)
- Quoted identifiers not resolved in page widget references (issue #8)
- DATAGRID placeholder ID leak during template augmentation (issue #6)
- COMBOBOX association EntityRef via IndirectEntityRef with association path
- Page/layout unit type mismatch (Forms$ vs Pages$ prefix)
- VIEW entity types, constant value BSON, and test error detection
- False positive OQL type inference for CASE expressions
- RETRIEVE using DatabaseRetrieveSource for reverse Reference association traversal
- RETURNS Void treated as void return type like Nothing
- ANNOTATION keyword added to annotationName grammar rule
- System entity types and RETURN keyword formatting in microflows
- 10 CodeQL security alerts
- XPath token quoting for `[%CurrentDateTime%]` (#1)
- DROP MODULE/ROLE cascade-removes module roles from user roles
- Security script CE0066 entity access out-of-date errors
- Slow integration tests with build tags and TestMain (issue #16)
- Docker run failing on fresh projects (issue #13)

### Changed

- Aligned `mxcli check` and `mxcli lint` reporting with shared Violation format (issue #10)
- Promoted BSON commands from debug-only to release build
- Auto-discover `.mpr` file when `-p` is omitted
- Moved `bson/` and `tui/` packages under `cmd/mxcli/` for better encapsulation
- Consolidated show-describe proposals into `docs/11-proposals/` with archive
- Documented association ParentPointer/ChildPointer semantics in CLAUDE.md
- Normalized CRLF to LF in bug reports via `.gitattributes`

## [0.2.0] - 2026-03-15

### Added

- **CI/CD** — GitHub Actions workflow for build, test, and lint on push; release workflow for tagged versions
- **Makefile Lint Targets** — `make lint`, `make lint-go` (fmt + vet), `make lint-ts` (tsc --noEmit)
- **Playwright Testing** — Browser name config support, port-offset fixes, project directory CWD for session discovery
- **VS Code Extension** — Project tree auto-refresh via file watchers, association cardinality label fix

### Fixed

- Enum truncation, DROP+CREATE cache invalidation, duplicate variable detection, subfolder enum resolution
- IMPORT FK column NULL fallback and entity attribute validation
- Docker exec using host port instead of container-internal port
- AGGREGATE syntax in skills docs
- Association cardinality labels in domain model diagrams
- 3 MDL bugs and standardized enum DEFAULT syntax

### Changed

- Default to always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Communication Style section in generated CLAUDE.md for human-readable change descriptions
- Shortened mxcli startup warning to single line
- Chromium system dependencies added to devcontainer Dockerfile

## [0.1.0] - 2026-03-13

First public release.

### Added

- **MDL Language** — SQL-like syntax (Mendix Definition Language) for querying and modifying Mendix projects
- **Domain Model** — CREATE/ALTER/DROP ENTITY, CREATE ASSOCIATION, attribute types, indexes, validation rules
- **Microflows & Nanoflows** — 60+ activity types, loops, error handling, expressions, parameters
- **Pages** — 50+ widget types, CREATE/ALTER PAGE/SNIPPET, DataGrid, DataView, ListView, pluggable widgets
- **Page Variables** — `variables: { $name: type = 'expression' }` in page/snippet headers for column visibility and conditional logic
- **Security** — Module roles, entity access rules, GRANT/REVOKE, UPDATE SECURITY reconciliation
- **Navigation** — Navigation profiles, menu items, home pages, login pages
- **Enumerations** — CREATE/ALTER/DROP ENUMERATION with localized values
- **Business Events** — CREATE/DROP business event services
- **Project Settings** — SHOW/DESCRIBE/ALTER for runtime, language, and theme settings
- **Database Connections** — CREATE/DESCRIBE DATABASE CONNECTION for Database Connector module
- **Full-text Search** — SEARCH across all strings, messages, captions, labels, and MDL source
- **Code Navigation** — SHOW CALLERS/CALLEES/REFERENCES/IMPACT/CONTEXT for cross-reference analysis
- **Catalog Queries** — SQL-based querying of project metadata via CATALOG tables
- **Linting** — 14 built-in rules + 27 Starlark rules across MDL, SEC, QUAL, ARCH, DESIGN, CONV categories
- **Report** — Scored best practices report with category breakdown (`mxcli report`)
- **Testing** — `.test.mdl` / `.test.md` test files with Docker-based runtime validation
- **Diff** — Compare MDL scripts against project state, git diff for MPR v2 projects
- **External SQL** — Direct queries against PostgreSQL, Oracle, SQL Server with credential isolation
- **Data Import** — IMPORT FROM external DB into Mendix app PostgreSQL with batch insert and ID generation
- **Connector Generation** — Auto-generate Database Connector MDL from external schema discovery
- **OQL** — Query running Mendix runtime via admin API
- **Docker Build** — `mxcli docker build` with PAD patching
- **VS Code Extension** — Syntax highlighting, diagnostics, completion, hover, go-to-definition, symbols, folding
- **LSP Server** — `mxcli lsp --stdio` for editor integration
- **Multi-tool Init** — `mxcli init` with support for Claude Code, Cursor, Continue.dev, Windsurf, Aider
- **Dev Container** — `mxcli init` generates `.devcontainer/` configuration for sandboxed AI agent development
- **MPR v1/v2** — Automatic format detection, read/write support for both formats
- **Fluent API** — High-level Go API (`api/` package) for programmatic model manipulation
