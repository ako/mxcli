# CLAUDE.md

This file provides guidance for Claude Code when working with this repository.

## Welcome — Contributing to mxcli

If you're starting a new task, here's how contributions work in this repo:

1. **File an issue first** — describe the bug or feature before coding. See `CONTRIBUTING.md` for details.
2. **Get approval** — wait for maintainer sign-off before starting work.
3. **Create a feature branch** — `feature/123-short-description` or `fix/456-what-broke`.
4. **Use the contributor commands** to stay on track:
   - `/mxcli-dev:proposal` — create a structured feature proposal (asks the right questions, investigates BSON storage)
   - `/mxcli-dev:review` — review your changes against the PR checklist before pushing
5. **Validate locally** — `make build && make test && make lint` must all pass.
6. **Open a PR** — link the issue, document Mendix Studio Pro validation, confirm agentic testing.

For the full workflow, read `CONTRIBUTING.md`. For the review checklist applied to every PR, see the "PR / Commit Review Checklist" section below.

## Project Overview

**ModelSDK Go** is a Go library for reading and modifying Mendix application projects (`.mpr` files) stored locally on disk. It's a Go-native alternative to the TypeScript-based Mendix Model SDK, enabling programmatic access without cloud connectivity.

## Build & Test Commands

```bash
# build the CLI (preferred - uses Makefile)
make build

# run tests
make test

# format and vet code
make fmt
make vet

# run a specific example
go run ./examples/read_project/main.go /path/to/project.mpr
go run ./examples/modify_project/main.go /path/to/project.mpr

# run the code generator
go run ./cmd/codegen/main.go -reflection-dir ./reference/mendixmodellib/reflection-data -version 10.0.0 -output ./generated/metamodel
```

**Note**: This project uses `modernc.org/sqlite` (pure Go) and does **not** require CGO. No C compiler is needed.

**Note**: The VS Code extension (`vscode-mdl/`) uses **bun**, not npm/node. Use `bun install`, `bun run compile`, etc. The Makefile targets (`make vscode-ext`, `make vscode-install`) already use bun.

## Mendix Tools

The `mx` command-line tool validates and builds Mendix projects. Location depends on environment:

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| This repo | `reference/mxbuild/modeler/mx` |

```bash
# Auto-download mxbuild for the project's Mendix version
mxcli setup mxbuild -p app.mpr

# check/validate a Mendix project
~/.mxcli/mxbuild/*/modeler/mx check /path/to/app.mpr

# or use the integrated command (auto-downloads mxbuild)
mxcli docker check -p app.mpr
```

## Project Architecture

```
ModelSDKGo/
├── modelsdk.go              # Main public api (open, OpenForWriting, helpers)
├── model/                   # Core types: ID, QualifiedName, module, Element interface
│
├── api/                     # High-level fluent api (inspired by Mendix Web Extensibility api)
│   ├── api.go               # ModelAPI entry point with namespace access
│   ├── domainmodels.go      # EntityBuilder, AssociationBuilder, AttributeBuilder
│   ├── enumerations.go      # EnumerationBuilder
│   ├── microflows.go        # MicroflowBuilder
│   ├── pages.go             # PageBuilder, widget builders
│   └── modules.go           # ModulesAPI
│
├── sdk/                     # SDK implementation packages
│   ├── domainmodel/         # entity, attribute, association, DomainModel
│   ├── microflows/          # microflow, nanoflow, activities (60+ types)
│   ├── pages/               # page, layout, widget types (50+ widgets)
│   ├── widgets/             # Embedded widget templates for pluggable widgets
│   │   ├── loader.go        # template loading with go:embed
│   │   └── templates/       # json widget type definitions by Mendix version
│   └── mpr/                 # MPR file format handling
│       ├── reader.go        # read-only MPR access
│       ├── writer.go        # read-write MPR modification
│       ├── parser.go        # BSON parsing and deserialization
│       └── utils.go         # UUID generation utilities
│
├── mdl/                     # MDL (Mendix Definition Language) parser & CLI
│   ├── grammar/             # ANTLR4 grammar definition
│   │   ├── MDLLexer.g4      # ANTLR4 lexer grammar (tokens)
│   │   ├── MDLParser.g4     # ANTLR4 parser grammar (rules)
│   │   └── parser/          # Generated Go parser code
│   ├── ast/                 # AST node types for MDL statements
│   ├── visitor/             # ANTLR listener to build AST
│   ├── executor/            # Executes AST against modelsdk-go
│   ├── catalog/             # SQLite-based catalog for querying project metadata
│   ├── linter/              # Extensible linting framework
│   │   └── rules/           # Built-in lint rules (MPR001, MPR002, etc.)
│   └── repl/                # Interactive REPL interface
│
├── sql/                     # external database connectivity (PostgreSQL, Oracle, sql Server)
│   ├── driver.go            # DriverName type, ParseDriver()
│   ├── connection.go        # Manager, connection, credential isolation
│   ├── config.go            # DSN resolution (env vars, YAML config)
│   ├── query.go             # execute() — query via database/sql
│   ├── meta.go              # ShowTables(), DescribeTable() via information_schema
│   ├── format.go            # table and json output formatters
│   ├── mendix.go            # Mendix DB DSN builder, table/column name helpers
│   └── import.go            # import pipeline: batch insert, ID generation, sequence tracking
│
├── cmd/                     # Command-line tools
│   ├── mxcli/               # CLI entry point (Cobra-based)
│   └── codegen/             # Code generator CLI
│
├── internal/                # Internal packages (not exported)
│   └── codegen/             # Metamodel code generation system
│       ├── schema/          # json reflection data loading
│       ├── transform/       # transform to Go types
│       └── emit/            # Go source code generation
│
├── generated/metamodel/     # Auto-generated type definitions
├── examples/                # Usage examples
│
└── reference/               # reference materials (not Go code)
    ├── mendixmodellib/      # TypeScript library + reflection data
    ├── mendixmodelsdk/      # TypeScript SDK reference
    └── mdl-grammar/         # Comprehensive MDL grammar reference
```

## Key Concepts

### MPR File Formats
- **v1**: Single `.mpr` SQLite database file (Mendix < 10.18)
- **v2**: `.mpr` metadata + `mprcontents/` folder with individual documents (Mendix >= 10.18)
- Format detection is automatic

### BSON Storage Names vs Qualified Names

**CRITICAL**: Mendix uses different "storage names" in BSON `$type` fields than the "qualified names" shown in the TypeScript SDK documentation. Using the wrong name causes `TypeCacheUnknownTypeException` when opening in Studio Pro.

| Qualified Name (SDK/docs) | Storage Name (BSON $Type) | Note |
|---------------------------|---------------------------|------|
| CreateObjectAction | CreateChangeAction | |
| ChangeObjectAction | ChangeAction | |
| DeleteObjectAction | DeleteAction | |
| CommitObjectsAction | CommitAction | |
| RollbackObjectAction | RollbackAction | |
| AggregateListAction | AggregateAction | |
| ListOperationAction | ListOperationsAction | |
| ShowPageAction | ShowFormAction | "Form" was original term for "Page" |
| ClosePageAction | CloseFormAction | "Form" was original term for "Page" |

When adding new types, always verify the storage name by:
1. Examining existing MPR files with the `mx` tool or SQLite browser
2. Checking the reflection data in `reference/mendixmodellib/reflection-data/`
3. Looking at the parser cases in `sdk/mpr/parser_microflow.go`

**IMPORTANT**: When unsure about the correct BSON structure for a new feature, **ask the user to create a working example in Mendix Studio Pro** so you can compare the generated BSON against a known-good reference.

### Pluggable Widget Templates

For pluggable widgets (DataGrid2, ComboBox, Gallery, etc.), templates must include **both** `type` AND `object` fields:
- `type`: Widget PropertyTypes schema (defines what properties exist)
- `object`: Default WidgetObject with all property values

**CE0463 "widget definition changed" error**: This error occurs when the Object's property structure doesn't match the Type's PropertyTypes. Always extract templates from Studio Pro-created widgets, not programmatically generated ones. See `sdk/widgets/templates/README.md` for details. For debugging CE0463 and other BSON issues, follow the workflow in `.claude/skills/debug-bson.md`.

### TypeEnumeration vs TypeEntity Ambiguity

The MDL visitor (`buildDataType` in `visitor_helpers.go`) cannot distinguish between entity types and enumeration types for bare qualified names like `Module.EntityName`. Both parse as `ast.TypeEnumeration` with `EnumRef` set. Code that consumes data types must handle `TypeEnumeration` alongside `TypeEntity` and use `EnumRef` as a fallback for the entity name.

### Mendix Expression String Escaping

When generating Mendix expression strings (e.g., in `expressionToString()`), single quotes within string literals must be escaped by doubling them: `'it''s here'`. Do NOT use backslash escaping (`\'`). This matches Mendix Studio Pro's expression syntax.

### Association Parent/Child Pointer Semantics (Counter-Intuitive)

**CRITICAL**: Mendix BSON uses inverted naming for association pointers:

| BSON Field | Points To | MDL Keyword |
|------------|-----------|-------------|
| `ParentPointer` | **FROM** entity (FK owner) | `from Module.Child` |
| `ChildPointer` | **TO** entity (referenced) | `to Module.Parent` |

`create association Mod.Child_Parent from Mod.Child to Mod.Parent` stores:
- `ParentPointer = Child.$ID` (the FROM entity owns the foreign key)
- `ChildPointer = Parent.$ID` (the TO entity is being referenced)

This affects **entity access rules**: MemberAccess entries for associations must only be added to the **FROM** entity (the one stored in `ParentPointer`). Adding them to the TO entity triggers CE0066 "Entity access is out of date".

The same convention applies in `domainmodel.Association`: `ParentID` = FROM entity, `ChildID` = TO entity.

### Public API Pattern
```go
// read-only access
reader, err := modelsdk.Open("/path/to/project.mpr")
defer reader.Close()

// read-write access
writer, err := modelsdk.OpenForWriting("/path/to/project.mpr")
defer writer.Close()
```

### High-Level Fluent API (in api/)
The `api/` package provides a simplified, fluent API inspired by Mendix Web Extensibility Model API:

```go
modelAPI := api.New(writer)
module, _ := modelAPI.Modules.GetModule("MyModule")
modelAPI.SetModule(module)

entity, _ := modelAPI.DomainModels.CreateEntity("Customer").
    persistent().
    WithStringAttribute("Name", 100).
    WithIntegerAttribute("Age").
    build()
```

Available namespaces: `DomainModels`, `enumerations`, `microflows`, `pages`, `modules`

## Code Style Guidelines

- Follow standard Go conventions (`go fmt`, `go vet`)
- Use descriptive names matching Mendix terminology
- Keep BSON/JSON tags consistent with Mendix serialization format
- Export types that should be part of the public API
- Use interfaces for polymorphic types (e.g., `Element`, `MicroflowObject`)

## Documentation Artifacts

mxcli uses a layered documentation system — each artifact type has a single canonical home. If a value can change without anyone touching the artifact, it does not belong there; link to the canonical home instead.

| Artifact | Lives in | Created via | Purpose |
|----------|----------|-------------|---------|
| PRD / feature proposal | `docs/11-proposals/` | `/mxcli-dev:proposal` | What to build and why |
| Bug report | `docs/12-bug-reports/` | — | Reproduction + diagnosis |
| ADR | `docs/13-decisions/` | `/mxcli-dev:adr-new` | Cross-cutting decisions (immutable audit trail) |
| User manual | `docs-site/src/` | hand-edited | How to use mxcli / MDL |
| Concept wiki | `docs-wiki/` | `/mxcli-dev:wiki-sync` | Synthesized brain — framing and connecting only |
| Skill | `.claude/skills/` | hand-edited | Step-by-step procedure for a recurring task |
| Symptom table | `.claude/skills/fix-issue.md` | append on every bug fix | Bug symptom → file → fix recipe |
| Load-bearing rule | this file | hand-edited | Always-in-context invariants and routing |

**State stays in its native home.** Proposal status, PR / issue numbers, roadmap, version registries — these live only where they're authoritative (proposal frontmatter, GitHub, the `sdk/versions/*.yaml` files). The wiki and CLAUDE.md may cite this state but never mirror it.

**ADRs are immutable once accepted.** Supersede with a new ADR rather than editing in place. Conventions and template in [`docs/13-decisions/README.md`](docs/13-decisions/README.md).

**The wiki is synthesized, not stated.** It frames and connects across the other artifacts — it never restates content that has a canonical home. Rules and seed page list in [`.claude/skills/maintain-wiki.md`](.claude/skills/maintain-wiki.md).

## PR / Commit Review Checklist

When reviewing pull requests or validating work before commit, verify these items:

### Bug fixes
- [ ] **Fix-issue skill consulted** — read `.claude/skills/fix-issue.md` before diagnosing; match symptom to table before opening files
- [ ] **Symptom table updated** — new symptom/layer/file mapping added to `.claude/skills/fix-issue.md` if not already covered
- [ ] **Test written first** — failing test exists before implementation (parser test in `sdk/mpr/`, backend mutation test in `mdl/backend/mpr/`, executor handler test in `mdl/executor/` using `MockBackend`)

### Overlap & duplication
- [ ] Check `docs/11-proposals/` for existing proposals covering the same functionality
- [ ] Search the codebase for existing implementations (grep for key function names, command names, types)
- [ ] Check `mdl-examples/doctype-tests/` for existing test coverage of the feature area
- [ ] Verify the PR doesn't re-document already-shipped features as new

### Syntax design for MDL features
New or modified MDL syntax must follow the design guidelines. See [ADR-0003: MDL is SQL-shaped](docs/13-decisions/0003-mdl-is-sql-shaped.md) for the underlying decision and rejected alternatives; the design checklist below operationalises it.
- [ ] **Design skill consulted** — read `.claude/skills/design-mdl-syntax.md` before designing syntax
- [ ] **Follows standard patterns** — uses `create`/`alter`/`drop`/`show`/`describe`, not custom verbs
- [ ] **Reads as English** — a business analyst understands the statement on first reading
- [ ] **Qualified names** — uses `Module.Element` everywhere, no implicit module context
- [ ] **Property format** — uses `( key: value, ... )` with colon separators, one per line
- [ ] **LLM-friendly** — one example is sufficient for an LLM to generate correct variants
- [ ] **Diff-friendly** — adding one property is a one-line diff

### Version compatibility
New features that depend on a specific Mendix version must be version-gated:
- [ ] **Registry entry** — feature added to `sdk/versions/mendix-{9,10,11}.yaml` with correct `min_version`
- [ ] **Executor pre-check** — `checkFeature()` called before BSON writes, with actionable error and hint
- [ ] **Test coverage** — version-gated tests use `-- @version:` directives or `requireMinVersion()`
- [ ] **Skill updated** — `.claude/skills/version-awareness.md` updated if the feature has a workaround for older versions

### Backend abstraction compliance
All executor code must go through the backend abstraction layer — the executor must never import `sdk/mpr` for write paths. See [ADR-0002: Backend Abstraction Layer](docs/13-decisions/0002-backend-abstraction.md) for the context and alternatives.
- [ ] **No `sdk/mpr` write imports in executor** — executor files must not call `sdk/mpr` writer/parser types directly; use `ctx.Backend.*` instead
- [ ] **New backend methods on the interface** — any new data access or mutation goes in the appropriate interface in `mdl/backend/` (e.g., `DomainModelBackend`, `MicroflowBackend`), not as a direct SDK call
- [ ] **MPR implementation in `mdl/backend/mpr/`** — the concrete implementation lives here; all BSON/reader/writer logic stays in this package
- [ ] **Mock stub in `mdl/backend/mock/`** — every new backend method has a `Func`-field stub with a descriptive `"MockBackend.X not configured"` error default (not `nil, nil`)
- [ ] **Compile-time interface check** — new backend implementations have `var _ backend.SomeInterface = (*impl)(nil)`
- [ ] **ALTER operations use mutator pattern** — page/workflow mutations go through `ctx.Backend.OpenPageForMutation()` / `OpenWorkflowForMutation()`, not inline BSON construction
- [ ] **New shared types in `mdl/types/`** — types used by both `mdl/` and `sdk/mpr` go in `mdl/types/`; `sdk/mpr` re-exports as type aliases (`type Foo = types.Foo`), never as duplicate definitions
- [ ] **Map iteration is deterministic** — any map iterated for serialization output must sort keys first (`sort.Strings(keys)` pattern); non-deterministic output causes flaky diffs and BSON instability
- [ ] **Pluggable widgets via WidgetEngine** — new pluggable widget support uses `.def.json` + `WidgetRegistry`; no hardcoded BSON widget builders in the executor

### Full-stack consistency for MDL features
New MDL commands or language features must be wired through the full pipeline:
- [ ] **Grammar** — rule added to `MDLParser.g4` (and `MDLLexer.g4` if new tokens)
- [ ] **Parser regenerated** — `make grammar` run; generated files in `mdl/grammar/parser/` are **not** committed (they are regenerated by `make` at build time)
- [ ] **AST** — node type added in `mdl/ast/`
- [ ] **Visitor** — ANTLR listener bridges parse tree to AST in `mdl/visitor/`
- [ ] **Executor** — thin handler in `mdl/executor/` dispatches to `ctx.Backend.*`; no BSON in the handler
- [ ] **Backend method** — data access or mutation wired through `mdl/backend/` interface and implemented in `mdl/backend/mpr/`
- [ ] **LSP** — if the feature adds formatting, diagnostics, or navigation targets, wire it into `cmd/mxcli/lsp.go` and register the capability
- [ ] **DESCRIBE roundtrip** — if the feature creates artifacts, `describe` should output re-executable MDL
- [ ] **VS Code extension** — if new LSP capabilities are added, update `vscode-mdl/package.json`

### Test coverage
- [ ] New packages have test files
- [ ] New executor commands have MDL examples in `mdl-examples/doctype-tests/`
- [ ] **MDL syntax changes** — any PR that adds or modifies MDL syntax must include working examples in `mdl-examples/doctype-tests/`
- [ ] **Bug fixes** — every bug fix should include an MDL test script in `mdl-examples/bug-tests/` that reproduces the issue, so the fix can be verified in Studio Pro if applicable
- [ ] Integration paths (not just helpers) are tested
- [ ] Tests don't rely on `time.Sleep` for synchronization — use channels or polling with timeout

### Security & robustness
- [ ] Unix sockets use restrictive permissions (`os.Chmod(path, 0600)`)
- [ ] File I/O is not in hot paths (event loops, per-keystroke handlers) — cache in memory
- [ ] No silent side effects on typos (e.g., auto-creating resources on misspelled names should be flagged)
- [ ] Method receivers are correct (pointer vs value) for mutations

### Scope & atomicity
- [ ] Each commit does **one thing** — a feature, a bugfix, or a refactor, not a mix
- [ ] Each PR is scoped to a **single feature or concern** — if the description needs "and" between unrelated items, split it
- [ ] Independent features (e.g., a new command, a formatter, UX improvements) go in separate PRs even if developed together
- [ ] Refactors that touch many files (e.g., renaming a helper across executors) are their own commit, not bundled with feature work

### Documentation
- [ ] **Skills** — new features documented in `.claude/skills/` (syntax, examples, gotchas)
- [ ] **CLI help (Cobra)** — `mxcli` subcommand help text updated (Cobra `Short`/`Long`/`Example` fields)
- [ ] **CLI help (syntax topics)** — `cmd/mxcli/syntax/features_*.go` updated with new/changed MDL syntax; new `SyntaxFeature` entries added for new document types; `OR MODIFY` / `OR REPLACE` variants reflected in existing `Syntax` fields; accessible via `mxcli syntax <topic>` and REPL `help`
- [ ] **Syntax reference** — `docs/01-project/MDL_QUICK_REFERENCE.md` updated with new statement syntax
- [ ] **MDL examples** — working examples added to `mdl-examples/` for new commands
- [ ] **Site docs** — `docs-site/src/` pages added or updated for user-facing features

### Code quality
- [ ] Refactors are applied consistently across all relevant files (grep for the old pattern)
- [ ] Manually maintained lists (keyword lists, type mappings) are flagged as maintenance risks
- [ ] Design docs match the actual implementation — remove or update stale plans
- [ ] Numeric type conversions are bounds-checked — `float64→int` casts need overflow guards (`±2^53` for safe integer range); silent overflow produces garbage in serialized output
- [ ] `convert.go` updated when structs in `mdl/types/` gain or lose fields — `TestFieldCountDrift` will catch this at test time, but `convert.go` must be updated before merging

## Dependencies

- `modernc.org/sqlite` - Pure Go SQLite driver (no CGO required)
- `go.mongodb.org/mongo-driver` - BSON parsing for Mendix document format
- `github.com/jackc/pgx/v5` - PostgreSQL driver for external SQL connectivity
- `github.com/sijms/go-ora/v2` - Oracle driver for external SQL connectivity
- `github.com/microsoft/go-mssqldb` - SQL Server driver for external SQL connectivity

## MDL CLI (mxcli)

The `mxcli` command-line tool allows reading and modifying Mendix projects using MDL (Mendix Definition Language), a SQL-like syntax.

```bash
# build the CLI
go build -o bin/mxcli ./cmd/mxcli

# run interactive REPL
./bin/mxcli

# execute commands directly
./bin/mxcli -p /path/to/app.mpr -c "show entities"

# execute MDL script file
./bin/mxcli exec script.mdl -p /path/to/app.mpr

# check MDL syntax (no project needed)
./bin/mxcli check script.mdl

# check syntax and validate references
./bin/mxcli check script.mdl -p app.mpr --references
```

### Key CLI Features

| Feature | Commands | Details |
|---------|----------|---------|
| **Project structure** | `show structure [depth 1\|2\|3] [in module] [all]` | Compact overview at 3 depth levels |
| **Catalog queries** | `show catalog tables`, `select ... from CATALOG.table` | SQL querying of project metadata |
| **Code search** | `show callers\|callees\|references\|impact\|context of ...` | Cross-reference navigation (requires `refresh catalog full`) |
| **Full-text search** | `search 'keyword'` | Search across all strings and source |
| **Linting** | `mxcli lint -p app.mpr [--format json\|sarif]` | 14 built-in rules + 27 Starlark rules (MDL, SEC, QUAL, ARCH, DESIGN, CONV) |
| **Report** | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Scored best practices report with category breakdown |
| **Testing** | `mxcli test tests/ -p app.mpr` | `.test.mdl` / `.test.md` files, requires Docker |
| **Diff** | `mxcli diff -p app.mpr changes.mdl` | Compare script against project state |
| **Diff local** | `mxcli diff-local -p app.mpr --ref head` | Git diff for MPR v2 projects |
| **Diff revisions** | `mxcli diff-local -p app.mpr --ref main..feature` | Compare two arbitrary git revisions |
| **OQL** | `mxcli oql -p app.mpr "select ..."` | Query running Mendix runtime |
| **Widgets** | `show widgets`, `update widgets set ...` | Widget discovery and bulk updates (experimental) |
| **External SQL** | `sql connect`, `sql <alias> select ...`, `mxcli sql` | Direct SQL queries against PostgreSQL, Oracle, SQL Server (credential isolation) |
| **Data import** | `import from <alias> query '...' into Module.Entity map (...)` | Import from external DB into Mendix app PostgreSQL (batch insert with ID generation) |
| **Connector gen** | `sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec]` | Auto-generate Database Connector MDL from discovered schema |
| **Diagnostics** | `mxcli diag [--bundle]` | Session logs, version info, bug report bundles |
| **New project** | `mxcli new <name> --version X.Y.Z [--output-dir dir]` | Downloads mxbuild, creates blank project, runs init, installs Linux mxcli for devcontainer |
| **Setup mxcli** | `mxcli setup mxcli [--os linux] [--arch amd64] [--output ./mxcli]` | Download platform-specific mxcli binary from GitHub releases |

### mxcli new

`mxcli new` creates a complete Mendix project from scratch in one step:

```bash
mxcli new MyApp --version 11.8.0
mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
```

Steps performed: downloads MxBuild → `mx create-project` → `mxcli init` → downloads correct Linux mxcli binary for devcontainer. The result is a ready-to-open project with `.devcontainer/`, AI tooling, and a working `./mxcli` binary.

### Slash Command Namespaces

Commands in `.claude/commands/` are organised by audience:

| Namespace | Folder | Invoked as | Purpose |
|-----------|--------|------------|---------|
| `mendix:` | `.claude/commands/mendix/` | `/mendix:lint` | mxcli **user** commands — synced to Mendix projects via `mxcli init` |
| `mxcli-dev:` | `.claude/commands/mxcli-dev/` | `/mxcli-dev:review` | **Contributor** commands — this repo only, never synced to user projects |

Both namespaces are discoverable by typing `/mxcli` in Claude Code. Add new contributor tooling (review workflows, debugging helpers, etc.) under `mxcli-dev/`. Add commands intended for Mendix project users under `mendix/`.

### mxcli init

`mxcli init` creates a `.claude/` folder with skills, commands, CLAUDE.md, and VS Code MDL extension in a target Mendix project. Source of truth for synced assets:
- Skills: `reference/mendix-repl/templates/.claude/skills/`
- Commands: `.claude/commands/mendix/` (the `mxcli-dev/` folder is **not** synced)
- VS Code extension: `vscode-mdl/vscode-mdl-*.vsix`

Build-time sync: `make build` syncs everything automatically. Individual targets: `make sync-skills`, `make sync-commands`, `make sync-vsix`.

### VS Code Extension

The `vscode-mdl` extension provides MDL language support: syntax highlighting, parse/semantic diagnostics, completion, symbols, folding, hover, go-to-definition, clickable terminal links, and context menu commands. The extension spawns `mxcli lsp --stdio` as the language server. Build with `make vscode-ext` (requires bun).

### ANTLR4 Parser

Regenerate after modifying `MDLLexer.g4`, `MDLParser.g4`, or any `domains/*.g4` file: `make grammar`. Generated files in `mdl/grammar/parser/` are **not** committed to git. See `docs/03-development/MDL_PARSER_ARCHITECTURE.md` for design details.

## IMPORTANT: Before Writing MDL Scripts or Working with Data

**Read the relevant skill files FIRST before writing any MDL, seeding data, or doing database/import work:**
- `.claude/skills/version-awareness.md` - **CHECK project version first** - Run `show features` before using version-gated syntax
- `.claude/skills/design-mdl-syntax.md` - **READ before designing new MDL syntax** - Design principles, decision framework, anti-patterns, checklist
- `.claude/skills/write-microflows.md` - Microflow syntax, common mistakes, validation checklist
- `.claude/skills/write-nanoflows.md` - Nanoflow syntax, restrictions, disallowed activities, validation checklist
- `.claude/skills/create-page.md` - Page/widget syntax reference
- `.claude/skills/alter-page.md` - ALTER PAGE/SNIPPET in-place modifications (SET, INSERT, DROP, REPLACE, SET Layout)
- `.claude/skills/overview-pages.md` - CRUD page patterns
- `.claude/skills/master-detail-pages.md` - Master-detail page patterns
- `.claude/skills/generate-domain-model.md` - Entity/Association syntax
- `.claude/skills/check-syntax.md` - Pre-flight validation checklist
- `.claude/skills/organize-project.md` - Folders, MOVE command, project structure conventions
- `.claude/skills/manage-security.md` - Security roles, access control, GRANT/REVOKE patterns
- `.claude/skills/manage-navigation.md` - Navigation profiles, home pages, menus, login pages
- `.claude/skills/demo-data.md` - **READ for any database/import work** - Mendix ID system, association storage, demo data insertion
- `.claude/skills/xpath-constraints.md` - XPath syntax in WHERE clauses, association paths, nested predicates, functions
- `.claude/skills/database-connections.md` - External database connections from microflows
- `.claude/skills/test-microflows.md` - **READ for testing work** - Test annotations, file formats, Docker setup requirement

### Mendix Microflow/Nanoflow Idioms (MUST follow)

These rules apply whenever generating microflow or nanoflow MDL. Violations are caught by `mxcli check`.

1. **NEVER create empty list variables as loop sources.** If processing imported data, accept the list as a microflow parameter — `declare $Items list of ... = empty` followed by `loop $item in $Items` is always wrong.
2. **NEVER use nested LOOPs for list matching.** Loop over the primary list and use `retrieve $match from $TargetList where key = $item/key limit 1` for O(N) lookup. Nested loops are O(N^2).
3. **Use append logic when merging**, not overwrite: `$Existing/Field + '\n' + $New/Field` inside an `if $New/Field != empty` guard.
4. **Read `.claude/skills/patterns-data-processing.md`** for delta merge, batch processing, and list operation patterns.

**Always validate before presenting to user:**
```bash
./bin/mxcli check script.mdl                    # Syntax + anti-pattern check
./bin/mxcli check script.mdl -p app.mpr --references  # With reference validation
```

## MDL Syntax Quick Reference

Full syntax tables for all MDL statements (microflows, pages, security, navigation, settings, business events, ALTER PAGE, reserved words) are in **[docs/01-project/MDL_QUICK_REFERENCE.md](docs/01-project/MDL_QUICK_REFERENCE.md)**.

## Current Implementation Status

**Implemented:**
- MPR v1/v2 reading and writing
- Domain model (entities, attributes, associations)
- ALTER ENTITY (add/rename/modify/drop attributes, indexes, documentation)
- Microflows/Nanoflows with 60+ activity types, JavaScript action calls, nanoflow validation parity
- Pages with 50+ widget types
- ALTER PAGE/SNIPPET (SET, INSERT, DROP, REPLACE operations on widget trees)
- Image widgets (IMAGE, STATICIMAGE, DYNAMICIMAGE) with Width/Height properties
- Code generator for metamodel types
- MDL CLI (`mxcli`) with ANTLR4 parser
- MDL support for domain model, microflows, pages, and security
- Security management (module roles, user roles, access control, demo users)
- High-level fluent API (`api/` package) for simplified model manipulation
- LSP server with hover, go-to-definition, completion, diagnostics, symbols, folding
- VS Code extension (`vscode-mdl`) with context menu commands (Run/Check/Selection)
- Docker build integration (`mxcli docker build`) with PAD patching (Phase 1)
- OQL query execution against running runtime (`mxcli oql`)
- Business event services (SHOW/DESCRIBE/CREATE/DROP)
- Project settings (SHOW/DESCRIBE/ALTER)
- External SQL query execution against PostgreSQL, Oracle, SQL Server (`mxcli sql`, MDL `sql connect/query`)
- Data import from external databases into Mendix app DB (`import from ... into ... map ...`)
- Database Connector generation from external schema (`sql <alias> generate connector into <module>`)
- EXECUTE DATABASE QUERY microflow action (static, dynamic SQL, parameterized, runtime connection override)
- CREATE/DROP WORKFLOW with user tasks, decisions, parallel splits, and other activity types
- ALTER WORKFLOW (SET properties, INSERT/DROP/REPLACE activities, outcomes, paths, conditions, boundary events)
- CALCULATED BY microflow syntax for calculated attributes
- Image collections (SHOW/DESCRIBE/CREATE/DROP)
- AI agent documents: Model, Knowledge Base, Consumed MCP Service, Agent (LIST/DESCRIBE/CREATE/DROP, with variables, tools, KB tools, dollar-quoted multi-line prompts; requires AgentEditorCommons module, Mendix 11.9+)
- OData contract browsing (SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS FROM cached $metadata)
- AsyncAPI contract browsing (SHOW/DESCRIBE CONTRACT CHANNELS/MESSAGES FROM cached AsyncAPI)
- SHOW EXTERNAL ACTIONS, SHOW PUBLISHED REST SERVICES
- CREATE/DROP/DESCRIBE PUBLISHED REST SERVICE with resources, operations, path params, CREATE OR REPLACE
- Integration catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events)
- Contract catalog tables (contract_entities, contract_actions, contract_messages — parsed from cached $metadata and AsyncAPI)
- Platform authentication (`mxcli auth login/logout/status/list`) with PAT scheme for marketplace-api.mendix.com and catalog.mendix.com; credentials stored at ~/.mxcli/auth.json (mode 0600), MENDIX_PAT env override
- Marketplace browsing (`mxcli marketplace search/info/versions`) with --min-mendix compatibility filtering; install blocked upstream (API does not expose download URLs)

**Not Yet Implemented:**
- 47 of 52 metamodel domains (REST, etc.)
- Delta/change tracking system
- Runtime type reflection

## Useful Files for Context

- `README.md` - User documentation and API reference
- `api/api.go` - High-level fluent API entry point
- `api/domainmodels.go` - Entity/Association/Attribute builders
- `docs/01-project/SDK_EQUIVALENCE.md` - Detailed comparison with TypeScript SDK, gap analysis
- `sdk/mpr/parser.go` - BSON parsing logic (complex, handles polymorphic types)
- `sdk/mpr/writer_widgets.go` - Widget BSON serialization
- `sdk/widgets/templates/` - Embedded widget templates for pluggable widgets (ComboBox, DataGrid2, etc.)
- `sdk/widgets/templates/README.md` - **Critical**: Template extraction requirements (must include both `type` AND `object`)
- `generated/metamodel/enums.go` - All Mendix enumeration types
- `mdl/grammar/MDL.g4` - ANTLR4 grammar for MDL syntax (production)
- `mdl/executor/executor.go` - MDL statement execution logic
- `reference/mdl-grammar/` - Comprehensive MDL grammar reference
- `reference/mendixmodellib/reflection-data/` - Type definitions with storage names and default values
- `docs/03-development/MDL_PARSER_ARCHITECTURE.md` - ANTLR4 parser design documentation
- `docs/03-development/PAGE_BSON_SERIALIZATION.md` - Page/widget BSON format, type mappings, required defaults
- `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` - What's version-resilient vs version-fragile in widget BSON output, and how to onboard a new Mendix minor (e.g. 11.10)
- `.claude/skills/debug-bson.md` - Workflow for debugging BSON serialization issues with `mx` tool (includes the "Studio Pro Update Widget" diff methodology that closed CE0463)
- `cmd/mxcli/lsp.go` - LSP server implementation (hover, definition, diagnostics, completion, symbols)
- `cmd/mxcli/init.go` - `mxcli init` command (project initialization + VS Code extension install)
- `cmd/mxcli/docker/oql.go` - OQL query execution against running Mendix runtime via M2EE admin API
- `sql/connection.go` - External SQL connection manager (credential isolation)
- `sql/config.go` - DSN resolution (env vars, YAML config)
- `sql/import.go` - IMPORT pipeline (batch insert, Mendix ID generation, sequence tracking)
- `sql/generate.go` - Database Connector MDL generation from external schema
- `sql/typemap.go` - SQL → Mendix type mapping, DSN → JDBC URL conversion
- `sql/mendix.go` - Mendix DB helpers (DSN builder, table/column name conversion)
- `cmd/mxcli/cmd_sql.go` - `mxcli sql` CLI subcommand
- `mdl/executor/cmd_sql.go` - SQL statement executor handlers
- `mdl/executor/cmd_import.go` - IMPORT statement executor (auto-connects to Mendix DB)
- `vscode-mdl/src/extension.ts` - VS Code extension entry point
- `vscode-mdl/package.json` - VS Code extension manifest (commands, menus, settings)
