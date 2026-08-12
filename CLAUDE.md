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

**Devcontainer gotcha — libSkiaSharp/FreeType crash on some mxbuild releases.** Certain bundled `mx` binaries (observed on 11.10.0) abort with `symbol lookup error: .../libSkiaSharp.so: undefined symbol: FT_Get_BDF_Property`. Root cause: `mx`/mxbuild run under the Temurin JVM, whose bundled libfreetype is stripped and lacks `FT_Get_BDF_Property`, so Skia loads the *JVM's* FreeType instead of the system one (which has the symbol). Preloading the system libfreetype makes it load first and fixes `mx check`/`build`/`run` while keeping Skia working.

`mxcli docker check`/`build`/`new` apply this automatically (`docker.PrepareMxCommand`, which globs the system libfreetype and sets `LD_PRELOAD` on the `mx` child — no-op on non-Linux or when none is found). To invoke a bundled `mx` directly, use the wrapper (same fix) or export `LD_PRELOAD` yourself:

```bash
scripts/mx-check.sh -p /path/to/app.mpr --version 11.10.0
# or, for any mx command:
export LD_PRELOAD=/usr/lib/$(uname -m)-linux-gnu/libfreetype.so.6
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

### Quoting Escapes Parser Keywords, Not Platform-Reserved Member Names

The skills advise **quoting all identifiers** to avoid keyword collisions, but this only escapes **MDL parser** keywords (so `"create"`, `"status"`, `"end"` become valid attribute names). It does **not** exempt names the Mendix **platform** reserves for entity members — those are rejected by Studio Pro (and by `mxcli check --references`) **even when quoted**, because the check strips the quotes and validates the bare name:

- `Type` → CE7247 / `MDL021` ("reserved word"). Rename (e.g. `ResourceType`, `TypeValue`). Also `ID`, `GUID`, `CurrentUser`, and the Java-keyword list.
- `CreatedDate` / `ChangedDate` / `Owner` / `ChangedBy` → `MDL020` on persistent entities. Use the `AutoCreatedDate` / `AutoChangedDate` / `AutoOwner` / `AutoChangedBy` pseudo-types for the audit fields, or a different name for an unrelated value.

The reserved-word lists live in `mdl/executor/cmd_enumerations.go` (`mendixReservedWords`, `mendixSystemAttributeNames`). "Always safe to quote" in the skills means *parser*-safe, not *platform*-safe.

### AfterStartupMicroflow Must Return Boolean

A microflow wired as the project's **after-startup** microflow must return `Boolean` — Mendix build fails with **CE0142** on a void (no-return) microflow. A common trip-up: a seed/demo-data microflow wired to after-startup will not build until it ends with a `return true` (Boolean). This is a Mendix platform rule, not an mxcli check.

### Overlay Writes: Never Invent a Key, Branch on `$Type`

When a write overlays fields onto preserved BSON (`mdl/settingsoverlay`, and any
future storage that follows ADR-0005 guard-don't-drop), two rules are load-bearing.
Breaking either produces a document `mx check` accepts and **Studio Pro cannot
open**: it resolves every stored property against the type's property list and
throws `System.InvalidOperationException: Sequence contains no matching element`
at `MprProperty.cs`. mxbuild's deserializer tolerates unknown properties, so the
build is not a safety net here.

1. **Write only keys the document already carries.** Property names are
   version-specific — Mendix renamed `JavaVersion` (`"Java21"`) to
   `JavaMajorVersion` (`"21"`) and `Tracing` to `OpenTelemetry` between 11.6 and
   11.12. Read the key off the stored document and write back to that same key;
   when neither is present, write neither (an absent optional property is filled
   in on load). See `settingsoverlay.JavaVersionKey` (#759).
2. **A polymorphic child must be dispatched on `$Type` before any field
   assignment.** Variants can differ in *arity*, not just field values:
   `Settings$SharedValue` carries a `Value`, while `Settings$PrivateValue` is a
   bare marker with no properties at all (the value lives on the developer's
   workstation). Assigning `Value` to whichever node is there corrupts the marker.

The same reasoning bans authoring what the model does not own: mxcli preserves a
constant override's shared/private choice and refuses statements that would flip
it, rather than silently converting one to the other.

Enum-valued properties are the sibling trap: validate against
`generated/metamodel` (e.g. `SettingsDatabaseType` is `Hsqldb`, never `HSQLDB`)
rather than passing a user string through.

**On a CREATE there is no stored document to read the key off.** Rule 1 then
becomes: branch on the project's Mendix version and write exactly one spelling —
never both as a hedge. `mdl/dbconnector` does this for the 11.13 rename of
`DatabaseQuery.QueryType` (int) to `Type` (string enum), which mxbuild *does*
catch, as CE5277 on every activity using the query. To learn the target shape
without guessing, run the new mxbuild's own migration over an old project
(`mx convert -p -s <project>`) and diff the BSON: Mendix ships a one-time
conversion per renamed property, so the converted document is authoritative.

### Writes Are Conditional, and an `$ID` Is Never Renumbered In Place

Storage does not write a unit whose new content is **semantically equal** to what
is stored ([ADR-0008](docs/13-decisions/0008-identity-and-idempotence.md)). The
comparison is on a canonical form — every element `$ID` replaced by its index in a
containment walk — because a rebuild mints a fresh random `$ID` per sub-element,
so comparing bytes would skip nothing. The policy lives in `modelsdk/canon`
(`Reconcile`) and is called at the single write choke point of **both** engines:
`modelsdk/mpr/writer_core.go` (`updateUnit` *and* `WriteTransaction.WriteUnit` —
`codec.Store` reaches storage through the latter) and `sdk/mpr/writer_units.go`.

Three rules follow, and each has already been violated once:

1. **Never rewrite an element `$ID` without rewriting every reference to it in the
   same pass.** Pointers are *primitive* properties holding an `element.ID`, not
   `ChildProperty`, so a containment walk traverses the whole document and never
   sees one. PR #125 renumbered IDs this way and made projects unopenable
   (`KeyNotFoundException` at `ResolvePostponedProperties`). A unit is rewritten
   wholesale or not at all.
2. **Adding a write path means wiring it to `canon.Reconcile`.** A new choke point
   that writes directly will silently churn while everything else is quiet — the
   worst kind of inconsistency, because the diff blames the wrong change.
3. **A new document type with an identity property needs a row in
   `canon.identityFields`.** It cannot be generated: Mendix's `IsIdentifier` lives
   in the modeler assemblies, not in the reflection data `generated/metamodel` is
   built from. `TestFreshGUIDFieldsHaveAnIdentityDecision` catches the common case
   (a property the codec mints fresh on every write) but cannot catch an identity property
   the codec does not mint. Establish the property's status the way `StableId` was
   — the method table is in ADR-0008.

Elision itself is type-agnostic and covers new document types for free, but it
assumes **no binary pointer crosses a unit boundary** (measured 0 of 9,910, not
enforced). A document type that references another *unit* by `$ID` rather than by
qualified name breaks that assumption and invalidates the argument in ADR-0008.

`MXCLI_ALWAYS_WRITE=1` forces every write to land, for bisecting. It does not
disable identity preservation. **Any test asserting "nothing changed" must include
the control run with it set** — otherwise the test passes against a build that
never had the fix, which is exactly how PR #125 shipped green.

### Theme Files: Where SCSS Actually Compiles

Styling written to the wrong place fails **silently** — the build succeeds and the
rules are simply absent, which is indistinguishable in the browser from a
specificity problem. Verified on Mendix 11.13 (probe rules compiled, then grepped
out of `theme-cache/web/theme.compiled.css`):

- **`theme/web/main.scss` compiles LAST** — after Atlas Core *and* after every
  module theme source. A partial imported from it overrides any Atlas rule with no
  `!important`. This is the home for app-level styling (Layer 2), and it is a
  three-line file of Mendix's own imports, not an Atlas-owned file.
- **`themesource/<name>/` is only compiled when `<name>` matches a real module.**
  mxbuild walks the model's modules; it never globs the directory. An invented
  folder is skipped without a warning. Use a module's theme source only when the
  styling belongs to that module.
- **`theme/web/custom-variables.scss` is imported once per module** (8× in a blank
  app), so it must hold **declarations only** — a rule there is emitted N times.
  Tokens go here (Layer 1); rules go in the partial.
- **Mendix 11 Atlas is CSS-custom-property-first**: `:root { --brand-primary: … }`,
  not SCSS `!default`. The derived ramp is CSS `color-mix()` against
  `var(--brand-primary)`, so retuning the primary re-derives it live.

`cmd/mxcli/theme` encodes all four. Its embed uses `//go:embed all:assets` — a
plain `go:embed assets` skips `_`-prefixed files, which is exactly how SCSS spells
a partial. Files the project already owns are written as digest-fenced blocks
(guard-don't-drop, as in ADR-0005): a block with local edits is refused, not
overwritten.

Two more, learned by flipping the variant on a running app:

- **Atlas ships `:root.theme-dark` / `:root.theme-neutral` in `theme/web/` but
  nothing that applies them** — the slot exists, the switcher does not. A theme's
  own dark block must be declared at `:root.theme-dark` *after* Mendix's
  `_theme-dark.scss` (same specificity, later wins), or the app reverts to stock
  Mendix blue the moment the class appears. Because the class lands on `<html>`,
  popups and modals rendered at `<body>` follow it too.
- **Never pin an Atlas leaf to a literal colour.** Map it to a theme token
  (`--bg-color: var(--mxt-ground)`) so a variant restates ~30 values instead of
  ~60. A hardcoded `--font-color-default` is invisible the moment the ground goes
  dark. Two Atlas rules also assume a *dark navigation rail* and paint topbar text
  with `--color-base`, so every mxcli theme keeps the rail dark in both variants
  and forces `color: inherit` on those widgets.

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
- [ ] **Symptom table updated** — new symptom/layer/file mapping added to `.claude/skills/fix-issue.md` if not already covered. Append at the END of the table — for readable diffs, not for merging: conflicts are handled by the `merge=union` driver in `.gitattributes`, which keeps both sides when two fixes append at once. (Insert position alone never fixed this; moving rows from the top to the bottom just moved the collision.) The table is looked up by matching a symptom, not read in order, so position carries no meaning
- [ ] **Test written first** — failing test exists before implementation (parser test in `sdk/mpr/`, backend mutation test in `mdl/backend/mpr/`, executor handler test in `mdl/executor/` using `MockBackend`)
- [ ] **Verified at the layer the symptom lives in** — a test proves something about the layer it exercises and nothing more. Parser/grammar → unit test. BSON we write → unit test on the encoded document. Files on disk after `mx` runs → integration test (`-tags integration`). **The rendered app's behaviour or appearance → `.claude/skills/verify-in-runtime.md`** (boot with `run --local`, assert in Playwright). A page can serialize to valid-looking BSON, pass `mx check`, build cleanly, and still render wrong — that was #812.
- [ ] **Fix proven to be the cause** — revert the fix (or stub the guard) and confirm the test fails with the reported symptom. A test that only passes against fixed code has not been shown to detect anything; two bugs this week had a green suite while live (#812 a clobbered `RegisterTypeDefaults`, #808 an integration test that had only ever skipped)

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
All executor code must go through the backend abstraction layer — the executor must never import `sdk/mpr` for write paths. See [ADR-0002: Backend Abstraction Layer](docs/13-decisions/0002-backend-abstraction.md) for the context and alternatives. The experimental `modelsdk` engine (behind `MXCLI_ENGINE`) routes **all** document types — domain models included — through the codec, not a codec/legacy hybrid; see [ADR-0004: Full codec engine](docs/13-decisions/0004-full-codec-engine.md). Where the codec path cannot yet reproduce a construct, the backend **refuses** the op rather than dropping data. The backend interface speaks the **semantic model**, not gen/BSON or AST types — gen+codec are the MPR backend's internal storage adapter, one of several (MPR, MCP/PED, a future storage format); see [ADR-0005](docs/13-decisions/0005-semantic-model-interface-currency.md). CREATE is model→gen; fidelity-sensitive ALTER uses backend-internal gen-mutation, not a model round-trip.
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
| **Linting** | `mxcli lint -p app.mpr [--format json\|sarif]` | 15 built-in rules + 27 Starlark rules (MDL, SEC, QUAL, ARCH, DESIGN, CONV) |
| **Report** | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Scored best practices report with category breakdown |
| **Testing** | `mxcli test tests/ -p app.mpr [--local] [--watch] [--attach]` | `.test.mdl` / `.test.md` files. `--local` runs on mxcli's own runtime (no Docker daemon), on its own ports + `<project>_test` database, driving a **token-guarded test endpoint** (one microflow per test, invoked over HTTP — a throwing test fails only itself, results are returned not log-scraped). `--watch` keeps the runtime warm (~30s first run, then ~2s). `--attach` runs against an app already up under `run --local --test-endpoint` (no boot; uses **that app's** database) |
| **Diff** | `mxcli diff -p app.mpr changes.mdl` | Compare script against project state |
| **Diff local** | `mxcli diff-local -p app.mpr --ref head` | Git diff for MPR v2 projects |
| **Diff revisions** | `mxcli diff-local -p app.mpr --ref main..feature` | Compare two arbitrary git revisions |
| **OQL** | `mxcli oql -p app.mpr "select ..."` | Query running Mendix runtime |
| **Widgets** | `show widgets`, `update widgets set ...`, `mxcli widget describe <id>` | Widget discovery, bulk updates (experimental), and inspecting a widget's discovered properties + dynamic rules |
| **External SQL** | `sql connect`, `sql <alias> select ...`, `mxcli sql` | Direct SQL queries against PostgreSQL, Oracle, SQL Server (credential isolation) |
| **Data import** | `import from <alias> query '...' into Module.Entity map (...)` | Import from external DB into Mendix app PostgreSQL (batch insert with ID generation) |
| **Connector gen** | `sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec]` | Auto-generate Database Connector MDL from discovered schema |
| **Diagnostics** | `mxcli diag [--bundle]` | Session logs, version info, bug report bundles |
| **New project** | `mxcli new <name> --version X.Y.Z [--output-dir dir] [--theme none]` | Downloads mxbuild, creates blank project, applies default styling, runs init, installs Linux mxcli for devcontainer |
| **Default styling** | `mxcli theme list\|show\|apply\|remove` | Applies a built-in theme (signal/ledger/console) — files under `theme/` only, the model is never touched |
| **Theme switching** | `mxcli theme apply <name> --variant auto\|light\|dark`, `mxcli theme switcher install` | `auto` ships both palettes (follows the OS + honours a `theme-light`/`theme-dark` class); `switcher install` adds the JS actions + nanoflow for a user toggle (**this one does write to the model**) |
| **Setup mxcli** | `mxcli setup mxcli [--os linux] [--arch amd64] [--output ./mxcli]` | Download platform-specific mxcli binary from GitHub releases |

### mxcli new

`mxcli new` creates a complete Mendix project from scratch in one step:

```bash
mxcli new MyApp --version 11.8.0
mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
```

Steps performed: downloads MxBuild → `mx create-project` → `mxcli theme apply` → `mxcli init` → one `mxbuild --target=deploy` run (`--skip-build` to skip) → downloads correct Linux mxcli binary for devcontainer. That build settles the JS/Java action stubs MxBuild rewrites on first build (48 tracked files in a blank 11.12 app), so a fresh clone does not go dirty the first time anyone builds it. The result is a ready-to-open project with `.devcontainer/`, AI tooling, mxcli's default styling, and a working `./mxcli` binary. Pass `--theme none` for plain Atlas.

### Slash Command Namespaces

Commands in `.claude/commands/` are organised by audience:

| Namespace | Folder | Invoked as | Purpose |
|-----------|--------|------------|---------|
| `mendix:` | `.claude/commands/mendix/` | `/mendix:lint` | mxcli **user** commands — synced to Mendix projects via `mxcli init` |
| `mxcli-dev:` | `.claude/commands/mxcli-dev/` | `/mxcli-dev:review` | **Contributor** commands — this repo only, never synced to user projects |

Both namespaces are discoverable by typing `/mxcli` in Claude Code. Add new contributor tooling (review workflows, debugging helpers, etc.) under `mxcli-dev/`. Add commands intended for Mendix project users under `mendix/`.

### mxcli init

`mxcli init` creates a `.claude/` folder with skills, commands, CLAUDE.md, and VS Code MDL extension in a target Mendix project. Source of truth for synced assets:
- Skills: `.claude/skills/mendix/` — `make sync-skills` copies these to the `cmd/mxcli/skills/` embed dir (`//go:embed skills/*.md`), which `mxcli init` writes into the project. **Edit the `mendix/` source, not the embed dir** (it is regenerated). The top-level `.claude/skills/*.md` are contributor/dev skills and are **not** synced.
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
- `.claude/skills/write-workflows.md` - **Workflow authoring** (CREATE/DROP/ALTER WORKFLOW): activities (user task, decision, parallel split, jump, wait, boundary events), header options, gotchas. Workflows are authorable, not read-only.
- `.claude/skills/create-page.md` - Page/widget syntax reference
- `.claude/skills/alter-page.md` - ALTER PAGE/SNIPPET in-place modifications (SET, INSERT, DROP, REPLACE, SET Layout)
- `.claude/skills/overview-pages.md` - CRUD page patterns
- `.claude/skills/master-detail-pages.md` - Master-detail page patterns
- `.claude/skills/generate-domain-model.md` - Entity/Association syntax
- `.claude/skills/mendix/scheduled-events-and-queues.md` - **Scheduled events (Mendix's cron) and task queues**: the eight Repeat variants and which fields each one takes, why a queue does NOT throttle a scheduled event, and why rewriting a microflow with a queued call is refused
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
2. **NEVER use nested LOOPs for list matching.** Loop over the primary list and use `$match = FIND($TargetList, key = $item/key)` for an O(N) in-memory lookup. A plain `retrieve … where` **cannot** filter a list variable (only a database/association source), so `retrieve $match from $TargetList where …` is a parse error — use `FIND`/`FILTER`. Nested loops are O(N^2).
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
- Default styling + runtime theme switching (`mxcli theme list/show/apply/remove/switcher`, `mxcli new --theme`): three embedded themes (**signal** light-first, **ledger** light-first, **console** dark-first), each a palette in `theme/web/custom-variables.scss` + a shared Atlas wiring partial + a theme partial imported from `theme/web/main.scss` (which compiles last), plus vendored fonts. **No model changes**, so it hot-applies under `run --local --watch` and cannot affect a build. Generated regions are digest-fenced: a block carrying local edits is refused rather than overwritten. Applying a theme removes the previous one. `--variant auto` (default) ships both palettes — the app follows `prefers-color-scheme` before first paint and honours a `theme-light`/`theme-dark` class on `<html>`; `light`/`dark` bakes one. `theme switcher install` is the only part that writes to the model (JS actions + a nanoflow for a toggle button). Package: `cmd/mxcli/theme/`. See `docs/11-proposals/PROPOSAL_default_styling.md`
- MPR v1/v2 reading and writing
- Idempotent writes (ADR-0008): a unit whose new content is semantically equal to what is stored is **not written**, so re-running an MDL script against an in-sync project leaves the `.mpr` and `mprcontents/` byte-identical and Studio Pro shows no version-control changes. Comparison is on a canonical form (element `$ID`s normalised away — a rebuild mints them randomly, so byte comparison would skip nothing); `Microflows$Microflow.StableId` is carried from the stored document rather than re-minted, because the build derives every client-callable microflow's operation id from it. One policy in `modelsdk/canon`, called from both engines' write choke points. `MXCLI_ALWAYS_WRITE=1` disables elision (not preservation) for bisecting. See `docs-site/src/internals/idempotent-writes.md`
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
- Warm test loop (`mxcli test --local [--watch]`, `--attach`, `run --local --test-endpoint`): local test runs go through a **token-guarded HTTP endpoint** registered by a generated Java custom request handler, instead of compiling the suite into the project's after-startup microflow. Boot registers the endpoint and then **chains the project's own after-startup microflow**, so tests see the app as it really boots (`--skip-app-startup` opts out) — without that, a suite depending on startup state passed under `--attach` and failed under `--local`. One microflow per test, resolved by name at request time from `Core.getMicroflowNames()` and invoked with `Core.microflowCall(...).execute(...)` — so a throwing test fails only itself (not the boot), results are returned rather than scraped from the runtime log, and each test has its own variable scope. Owning the `IContext` is also what finally makes **`@cleanup rollback`** (the annotation's documented default, previously parsed and ignored) real: the handler wraps the call in `startTransaction()`/`rollbackTransaction()`, so a test's writes do not survive it — verified against Postgres, with `@cleanup none` as the in-run control. A rollback that fails is reported per test and summarised, never silent; an unknown strategy is a parse error. The handler **survives `reload_model`** (after-startup does not re-run, the JVM is unchanged), which is what makes `--watch` possible: ~30s first run, then ~2s from an edit — to a test *or* to the microflow under test — to a verdict. `--attach` skips even that boot by running against an app already up under `run --local --test-endpoint`, driving that process's serve + admin APIs over loopback; it uses **that app's database**, only ever adds/removes its own test microflows, and refuses a change needing a restart. Security: the handler is **not registered at all** without `MXCLI_TEST_TOKEN` in the runtime env (so a project that kept the `MxTest` module through a failed cleanup is inert in production), the token is constant-time compared, non-loopback callers are refused, `/list` is clamped to the test namespace, and only `MxTest.Test_*` may be invoked. The token reaches the runtime via its environment and is never written into the project. Docker keeps the after-startup runner (`--legacy-runner` selects it locally). Packages: `cmd/mxcli/testrunner/` (`endpoint.go`, `client.go`, `watch.go`, `host.go`, `handshake.go`). See `docs/15-testing/SPIKE_test_endpoint_request_handler.md`
- Warm local dev loop (`mxcli run --local [--watch] [--screenshot]`): Docker-free `mxbuild --serve` + standalone runtime, hot `reload_model` for behavioural changes and restart+DDL for structural ones (chosen from the serve build's `restartRequired`). Bundles the browser client (`web/dist/` via mxbuild's rollup runner, which the serve Deploy target skips) so Mendix 11.x apps render in a browser. `--watch` keeps an incremental rollup bundler hot (CHOKIDAR_USEPOLLING for container fs; ~3-4s page re-bundle, skipped for model-only edits) and watches only model source (`.mpr`+`mprcontents/`). `--ensure-db` provisions the local Postgres + app database if missing; `--setup` does the non-blocking prerequisites (cache mxbuild+runtime, ensure DB) and exits — `mxcli init` wires it into a Claude Code SessionStart hook so a fresh/reaped web session self-bootstraps, and `docs-site/src/tools/bootstrap-prompt.md` is the empty-repo seed prompt. `--screenshot` captures a Playwright PNG each change (pixel-perfect page loop), with `--screenshot-url` deep links (repeatable for multi-page sets, one PNG per page) and `--screenshot-user`/`--screenshot-password` form login (session saved as Playwright storage state, reused via `screenshot --load-storage`). See `docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md`
- External browser preview (`mxcli run --hub <url>` + `mxcli tunnel-hub`): the app stays local and reverse-tunnels out over a single 443 connection (embedded chisel) to a static relay, so it is reachable in a browser at a public URL — works from egress-only environments (Claude Code web), verified live through the session's MITM egress proxy. `run --hub` implies `--local`, boots the runtime with `ApplicationRootUrl` set to the assigned URL (so the SPA/`originURI` work under the public origin), resolves the control proxy honouring `NO_PROXY`, and retries forever. `mxcli tunnel-hub --domain <base>` is the **multi-tenant** relay: a registry keyed by prefix/project/solution/branch/worktree (stable URLs on reconnect) fronts many previews at per-subdomain hosts (`[prefix-]project[-branch].<base>`; main collapses to the project) over one 443 with per-subdomain autocert, a registration API (`/api/register|status|deregister|backends|sessions`), and an availability overview at `hub.<base>/` **grouped by Claude Code session** (`/api/sessions`): each session lists the endpoints it exposed and links back to its `claude.ai/code` conversation. Client identity flags: `--hub-prefix`/`--hub-project`/`--hub-solution`/`--hub-branch`/`--hub-worktree` (project + branch auto-detected); `--hub-session` groups a session's endpoints (auto-detected from `CLAUDE_CODE_REMOTE_SESSION_ID`). Past sessions are retained: a durable per-session endpoint history (`--sessions-file`, default `~/.mxcli/hub-sessions.json`) survives restarts and reaping, and is pruned after `--session-retention` (default 30d) — so the overview shows offline sessions too (`SessionLog` in `cmd/mxcli/tunnelhub/sessions.go`). Package: `cmd/mxcli/tunnelhub/`. See `docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md` (slices 3–4)
- Tunnel-hub GitHub authentication (opt-in, gated on `--github-oauth-client-id`; absent = today's open hub): **viewer plane** — GitHub OAuth web flow + HMAC-signed SSO session cookie (`Domain=.<domain>`), owner-checked previews (`--require-auth` default on → 302 to login / 403 non-owner; soft mode filters the listing only), `/api/backends` filtered to the viewer (unauthenticated → 401), admin "signed in as" via `/api/whoami`. **Registration plane** — durable, hashed hub API keys (`--keys-file`, default `~/.mxcli/hub-keys.json`, survive restarts) presented as `X-Hub-Key` → stamps `Backend.Owner`; shared `X-Hub-Secret` still works as an owner-less fallback. **Key issuance** — the hub's `/cli` browser page mints a key from the session cookie (no PAT; the device flow was removed as Claude Code containers block GitHub's device endpoints), rotate-by-default + count + revoke-all; `mxcli auth hub login --token <pat>` is the headless path; `run --hub` reads `MXCLI_HUB_KEY` (env → `~/.mxcli/auth.json`) and degrades to local-only if registration fails. Append-only JSONL audit trail (`--audit-log`, no secrets). Packages: `cmd/mxcli/tunnelhub/` (+`audit/`), `cmd/mxcli/hubauth/`. See `docs/11-proposals/PROPOSAL_hub_authentication.md`
- Runtime metrics + settings passthrough (`mxcli run --local --metrics` / `--runtime-setting Key=Value`): `--metrics` registers a Prometheus Micrometer registry at boot (served at `http://127.0.0.1:<admin-port>/prometheus`); `--runtime-setting` merges arbitrary runtime config (e.g. `Metrics.Registries` for otlp/influx/statsd, or `OpenTelemetry._RuntimeSpanFilters`) into mxcli's **single** boot `update_configuration` call — the admin action replaces rather than merges, so folding settings into the one boot call is the only safe way. OTel traces via `--trace` attach the bundled `opentelemetry-javaagent` to the runtime JVM (console exporter → the tee'd runtime log) and ship default `OpenTelemetry._RuntimeSpanFilters` (unfiltered per-activity tracing is ~10× slower); `--trace-service` sets `OTEL_SERVICE_NAME`. The console exporter omits timestamps/parent span IDs (no flame charts), so `--trace-otlp <endpoint>` (implies `--trace`) switches to the OTLP exporter (protocol `http/protobuf`) pointed at a collector; user-set `OTEL_*` env still takes precedence. See `.claude/skills/mendix/run-local.md`
- OQL query execution against running runtime (`mxcli oql`)
- Microflow/nanoflow debugger (`mxcli debug`): set breakpoints **by name** (activity resolved from the model), inspect paused flows + variables, step over/into/out, continue — against a `run --local` runtime. Two M2EE planes wired behind one command (admin `enable/disable/status`, app `/debugger/` session); `run --local --debug` enables it at boot. **Nanoflows** are auto-detected (uses the `nanoflow_name` breakpoint param; paused nanoflows are merged from `poll_events`, which `get_paused_microflows` omits). Nanoflow `LOG` output is rewritten to the `Client_Nanoflow` node in the runtime log. See `.claude/skills/mendix/debug-microflows.md` and `docs/11-proposals/PROPOSAL_microflow_debugger.md`
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
- Scheduled events — Mendix's cron (LIST/DESCRIBE/CREATE [OR MODIFY]/DROP). `Repeat:` names one of the eight `ScheduledEvents$*Schedule` variants and only that variant's fields are accepted; a field from another repeat is refused by `mxcli check` (MDL-SCHED01) and by exec, which call the same function. The document shape is pinned by re-serializing three whole Studio Pro-authored events (Workflow Commons 4.11.0, OIDC SSO 4.6.0, SAML 4.2.1) element by element — `modelsdk/gen` is **wrong** about two properties here: the integers are stored as int64 (gen says int32, the #585 mismatch) and `StartDateTime` is a BSON datetime (gen says string), so both engines share one raw-BSON codec in `mdl/scheduledevents`. `Interval`/`IntervalType` are legacy siblings of `Schedule` that Studio Pro writes and does not keep in sync — derived on CREATE, carried through untouched on MODIFY. Only the Day and Hour variants have a Studio Pro reference; the other six are metamodel-derived and verified to load. Both are in the catalog (`CATALOG.SCHEDULED_EVENTS`, `CATALOG.QUEUES`) and a scheduled event emits a `schedule` edge into `CATALOG.REFS` — without it a microflow run only by a scheduled event was reported as dead by `show callers`, `GRAPH_DEAD_ASSETS` and lint rule QUAL004. See `.claude/skills/mendix/scheduled-events-and-queues.md`
- Task queues (LIST/DESCRIBE/CREATE [OR MODIFY]/DROP QUEUE). `Config.ParallelismExpression` is a **string** and the sibling int32 `Parallelism` is not written — matching all four Studio Pro queues in Business Events 3.12.1. Binding a *call* to a queue is not yet authorable, so `CREATE OR REPLACE|MODIFY MICROFLOW` is **refused** when the stored microflow has a queued call (guard-don't-drop, ADR-0005): the rebuild used to write `QueueSettings` back as null, which made `mx check` go from CE1613 to 0 errors by deleting the user's configuration
- AI agent documents: Model, Knowledge Base, Consumed MCP Service, Agent (LIST/DESCRIBE/CREATE/DROP, with variables, tools, KB tools, dollar-quoted multi-line prompts; requires AgentEditorCommons module, Mendix 11.9+)
- OData contract browsing (SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS FROM cached $metadata)
- AsyncAPI contract browsing (SHOW/DESCRIBE CONTRACT CHANNELS/MESSAGES FROM cached AsyncAPI)
- SHOW EXTERNAL ACTIONS, SHOW PUBLISHED REST SERVICES
- CREATE/DROP/DESCRIBE PUBLISHED REST SERVICE with resources, operations, path params, CREATE OR REPLACE
- Integration catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events)
- Contract catalog tables (contract_entities, contract_actions, contract_messages — parsed from cached $metadata and AsyncAPI)
- Platform authentication (`mxcli auth login/logout/status/list`) with PAT scheme for marketplace-api.mendix.com, marketplace.mendix.com, and catalog.mendix.com; credentials stored at ~/.mxcli/auth.json (mode 0600), MENDIX_PAT env override
- Marketplace browsing (`mxcli marketplace search/info/versions`) with --min-mendix compatibility filtering
- Marketplace download/install (`mxcli marketplace download/install`) — the content API now exposes a per-version downloadUrl (303→public CDN); install is type-aware (widget→widgets/, new module→`mx module-import`); existing-module updates are reported, not applied (entity-ID/local-edit safety — see PROPOSAL_marketplace_modules.md)

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
- `docs/03-development/MODELSDK_ENGINE_ARCHITECTURE.md` - **Read before extending the modelsdk engine**: layers, the canonical write/read/ALTER patterns, codec mechanisms (TypeDefaults, list markers, storage-name overrides), the engalar harvest rule, and the add-a-document-type recipe
- `docs/03-development/PAGE_BSON_SERIALIZATION.md` - Page/widget BSON format, type mappings, required defaults
- `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` - What's version-resilient vs version-fragile in widget BSON output, and how to onboard a new Mendix minor (e.g. 11.10)
- `.claude/skills/debug-bson.md` - Workflow for debugging BSON serialization issues with `mx` tool (includes the "Studio Pro Update Widget" diff methodology that closed CE0463)
- `.claude/skills/diagnose-ce0463.md` - **Read first for any CE0463 report**: the elimination order, the two controls that separate "the user upgraded a widget package" (not our bug) from a real mxcli defect, and the measurement traps that make CE0463 investigations go wrong
- `.claude/skills/verify-in-runtime.md` - Proving a fix in a real app in a real browser (`run --local` + Playwright). For symptoms that only exist at render time, where valid-looking BSON and a clean `mx check` prove nothing — see #812
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
