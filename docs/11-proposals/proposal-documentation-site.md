---
title: Unified mxcli & MDL Documentation Site
status: proposed
---

# Proposal: Unified mxcli & MDL Documentation Site

## Problem

mxcli has extensive documentation — 119 markdown files, a complete language specification, architecture docs, examples, and user guides — but it's scattered across 14 directories with no unified structure. There's no way for a user to go from "what is mxcli?" to "how do I CREATE ENTITY with constraints?" in a logical progression. The docs can't be put online or bundled as a single PDF.

**What we need**: A PostgreSQL-style documentation site that works as:
1. A browsable website (GitHub Pages or custom domain)
2. A single downloadable PDF
3. High-quality LLM training material (plain text, structured, example-rich)

## Design Principles (Learned from PostgreSQL)

PostgreSQL's docs work because they separate concerns:

| Layer | Purpose | PostgreSQL Example | mxcli Equivalent |
|-------|---------|-------------------|-------------------|
| **Tutorial** | Learn by doing | Part I: Tutorial (3 chapters) | Getting started, first project |
| **Conceptual** | Understand features | Part II: SQL Language (11 chapters) | MDL language guide, Mendix concepts |
| **Reference** | Look up syntax | Part VI: SQL Commands (one page per statement) | MDL statement reference |
| **Administration** | Run and configure | Part III: Server Administration | Installation, CLI usage, Docker, LSP |
| **Internals** | Understand implementation | Part VII: Internals | Architecture, BSON format, parser |

Key insight: **never mix tutorial prose with reference syntax**. The conceptual chapters in Part II teach *how* to use CREATE TABLE. The reference page for CREATE TABLE gives the exact syntax, parameters, and edge cases. Both exist, both link to each other.

---

## Proposed Structure

```
mxcli documentation
│
├── Preface
│   ├── What is mxcli?
│   ├── What is MDL?
│   ├── Mendix Concepts for Newcomers
│   └── Document Conventions
│
├── Part I: Tutorial
│   ├── 1. Setting Up
│   │   ├── Installation (binary, go install, dev container)
│   │   ├── Opening Your First project
│   │   └── The REPL
│   ├── 2. Exploring a project
│   │   ├── show modules, show entities
│   │   ├── describe, search
│   │   └── show structure
│   ├── 3. Your First Changes
│   │   ├── Creating an entity
│   │   ├── Creating a microflow
│   │   ├── Creating a page
│   │   └── Validating with mxcli check
│   └── 4. Working with AI Assistants
│       ├── Claude Code Integration
│       ├── Cursor / Continue.dev / Windsurf
│       ├── Skills and CLAUDE.md
│       └── The MDL + AI workflow
│
├── Part II: The MDL Language
│   ├── 5. MDL Basics
│   │   ├── Lexical structure (keywords, identifiers, literals)
│   │   ├── Qualified Names (Module.Name)
│   │   ├── Comments and documentation (/** */)
│   │   └── script Files (.mdl)
│   ├── 6. data Types
│   │   ├── Primitive Types (string, integer, long, decimal, boolean, datetime, ...)
│   │   ├── Constraints (not null, default, unique)
│   │   ├── enumerations
│   │   └── type mapping (MDL → Mendix → database)
│   ├── 7. Domain model
│   │   ├── entities (persistent, non-persistent, external, view)
│   │   ├── attributes and validation rules
│   │   ├── associations (reference, ReferenceSet, ownership, delete behavior)
│   │   ├── generalization (extends)
│   │   ├── Indexes
│   │   └── alter entity
│   ├── 8. microflows and nanoflows
│   │   ├── structure (parameters, variables, activities, return)
│   │   ├── activity Types (retrieve, create, change, commit, delete, ...)
│   │   ├── Control Flow (if/else, loop, error handling)
│   │   ├── Expressions
│   │   ├── nanoflows vs microflows
│   │   └── Common Patterns (CRUD, validation, batch processing)
│   ├── 9. pages
│   │   ├── page structure (layout, content, data source)
│   │   ├── widget Types (dataview, datagrid, container, textbox, button, ...)
│   │   ├── data Binding (-> operator)
│   │   ├── snippets
│   │   ├── alter page / alter snippet
│   │   └── Common Patterns (list page, edit page, master-detail)
│   ├── 10. security
│   │   ├── module roles and user roles
│   │   ├── entity access (create, read, write, delete)
│   │   ├── microflow, page, and nanoflow access
│   │   ├── grant / revoke
│   │   └── demo users
│   ├── 11. navigation and settings
│   │   ├── navigation Profiles
│   │   ├── home pages and Menus
│   │   └── project settings
│   ├── 12. workflows
│   │   ├── workflow structure
│   │   ├── activity Types (user tasks, decisions, parallel splits, ...)
│   │   └── workflow vs microflow
│   └── 13. business events
│       ├── event services
│       └── Publishing and Consuming events
│
├── Part III: project Tools
│   ├── 14. Code navigation
│   │   ├── show callers / callees
│   │   ├── show references / impact
│   │   ├── show context
│   │   └── full-text search (search)
│   ├── 15. catalog Queries
│   │   ├── refresh catalog
│   │   ├── Available tables (modules, entities, microflows, pages, ...)
│   │   ├── sql Queries (select from CATALOG.*)
│   │   └── use Cases (impact analysis, unused elements, complexity metrics)
│   ├── 16. Linting and Reports
│   │   ├── Built-in rules (14 Go rules)
│   │   ├── Starlark rules (27 extensible rules)
│   │   ├── Writing Custom rules
│   │   ├── mxcli lint (json, sarif output)
│   │   └── mxcli report (scored best practices)
│   ├── 17. Testing
│   │   ├── .test.mdl and .test.md Formats
│   │   ├── Test Annotations (@test, @expect)
│   │   ├── Running Tests (mxcli test, Docker requirement)
│   │   └── Diff (mxcli diff, mxcli diff-local)
│   ├── 18. external sql
│   │   ├── sql connect (PostgreSQL, Oracle, sql Server)
│   │   ├── Querying external Databases
│   │   ├── import from ... into ... map
│   │   ├── Credential Management
│   │   └── database connector Generation
│   └── 19. Docker Integration
│       ├── mxcli docker build (PAD)
│       ├── mxcli docker run
│       ├── OQL Queries (mxcli oql)
│       └── Dev container Setup
│
├── Part IV: IDE Integration
│   ├── 20. VS Code Extension
│   │   ├── Installation
│   │   ├── Syntax Highlighting and Diagnostics
│   │   ├── Completion, Hover, Go-to-Definition
│   │   ├── project Tree
│   │   └── context menu Commands
│   ├── 21. LSP Server
│   │   ├── Protocol (stdio)
│   │   ├── Capabilities
│   │   └── Integration with Other Editors
│   └── 22. mxcli init
│       ├── What Gets created (.claude/, .devcontainer/, skills)
│       ├── Customizing Skills
│       └── Syncing with Updates
│
├── Part V: Go Library
│   ├── 23. Quick Start
│   │   ├── Installation (go get)
│   │   ├── Reading a project
│   │   └── Modifying a project
│   ├── 24. Public api (modelsdk.go)
│   │   ├── open / OpenForWriting
│   │   ├── Reader Methods
│   │   └── Writer Methods
│   └── 25. Fluent api (api/)
│       ├── ModelAPI Entry Point
│       ├── EntityBuilder, MicroflowBuilder, PageBuilder
│       └── Examples
│
├── Part VI: MDL Statement reference
│   │
│   │   (One page per statement, PostgreSQL-style:
│   │    Synopsis → description → parameters → Notes → Examples → See Also)
│   │
│   ├── connection Statements
│   │   ├── open project
│   │   └── close project
│   ├── query Statements
│   │   ├── show modules
│   │   ├── show entities
│   │   ├── show microflows / nanoflows
│   │   ├── show pages / snippets
│   │   ├── show enumerations
│   │   ├── show associations
│   │   ├── show constants
│   │   ├── show workflows
│   │   ├── show business events
│   │   ├── show structure
│   │   ├── show widgets
│   │   ├── describe entity
│   │   ├── describe microflow / nanoflow
│   │   ├── describe page / snippet
│   │   ├── describe enumeration
│   │   ├── describe association
│   │   └── search
│   ├── Domain model Statements
│   │   ├── create entity
│   │   ├── alter entity
│   │   ├── drop entity
│   │   ├── create enumeration
│   │   ├── drop enumeration
│   │   ├── create association
│   │   ├── drop association
│   │   └── create constant
│   ├── microflow Statements
│   │   ├── create microflow
│   │   ├── create nanoflow
│   │   ├── drop microflow / nanoflow
│   │   └── create java action
│   ├── page Statements
│   │   ├── create page
│   │   ├── create snippet
│   │   ├── alter page / alter snippet
│   │   ├── drop page / snippet
│   │   └── create layout
│   ├── security Statements
│   │   ├── create module role
│   │   ├── create user role
│   │   ├── grant
│   │   ├── revoke
│   │   └── create demo user
│   ├── navigation Statements
│   │   ├── alter navigation
│   │   └── show navigation
│   ├── workflow Statements
│   │   ├── create workflow
│   │   └── drop workflow
│   ├── business event Statements
│   │   ├── create business event service
│   │   └── drop business event service
│   ├── catalog Statements
│   │   ├── refresh catalog
│   │   ├── select from catalog
│   │   ├── show callers / callees
│   │   ├── show references / impact / context
│   │   └── show catalog tables
│   ├── external sql Statements
│   │   ├── sql connect
│   │   ├── sql disconnect
│   │   ├── sql (query)
│   │   ├── sql generate connector
│   │   └── import from
│   ├── settings Statements
│   │   ├── show settings
│   │   └── alter settings
│   ├── Organization Statements
│   │   ├── create module
│   │   ├── create folder
│   │   └── move
│   └── session Statements
│       ├── set
│       └── show status
│
├── Part VII: Architecture & Internals
│   ├── 26. System Architecture
│   │   ├── Layer Diagram (ASCII + Mermaid)
│   │   ├── Package structure
│   │   └── design Decisions
│   ├── 27. MPR file format
│   │   ├── v1 (SQLite) vs v2 (mprcontents/)
│   │   ├── BSON Document structure
│   │   ├── storage Names vs Qualified Names
│   │   └── widget template System
│   ├── 28. MDL Parser
│   │   ├── ANTLR4 Grammar design
│   │   ├── Lexer → Parser → AST → Executor Pipeline
│   │   └── Adding New Statements
│   └── 29. catalog System
│       ├── SQLite schema
│       ├── FTS5 full-text search
│       └── reference Tracking
│
├── Part VIII: Appendixes
│   ├── A. MDL Quick reference (cheat sheet)
│   ├── B. data type mapping table
│   ├── C. Reserved Words
│   ├── D. Mendix version Compatibility
│   ├── E. Common Mistakes and Anti-Patterns
│   ├── F. error messages reference
│   ├── G. Glossary (Mendix terms for non-Mendix developers)
│   ├── H. TypeScript SDK Equivalence
│   └── I. Changelog
│
└── index
```

---

## Per-Statement Reference Format

Every statement in Part VI follows the same template (matching PostgreSQL):

```markdown
# create entity

## Synopsis

    create [or modify] [persistent | non-persistent] entity module.name
        [extends parent.entity]
    (
        attr_name: data_type [not null] [default value] [unique],
        ...
    );

## description

Creates a new entity in the specified module's domain model. Entities
are the data objects in a Mendix application, similar to database tables.

## parameters

**persistent | non-persistent**
: persistent entities are stored in the database. non-persistent entities
  exist only in memory during a session. default: PERSISTENT.

**extends parent.entity**
: Creates a generalization (inheritance) relationship. The new entity
  inherits all attributes from the parent. Must appear before the
  opening parenthesis.

**attr_name: data_type**
: Defines an attribute. See data Types for available types.

## Notes

- extends must appear before `(`, not after `)` — this is a common mistake.
- string attributes require an explicit length: `string(200)`, not `string`.
- use `or modify` for idempotent scripts that may be re-run.

## Examples

### basic entity

    create persistent entity Sales.Customer (
        Name: string(200) not null,
        Email: string(200),
        IsActive: boolean default true
    );

### entity with generalization

    create persistent entity Sales.VIPCustomer extends Sales.Customer (
        DiscountPercentage: decimal,
        LoyaltyTier: string(50) default 'Silver'
    );

### Idempotent creation

    create or modify persistent entity Sales.Customer (
        Name: string(200) not null,
        Email: string(200),
        Phone: string(50)
    );

## See Also

alter entity, drop entity, create association, describe entity
```

---

## Content Sourcing

Most content already exists. The work is reorganization and gap-filling, not writing from scratch.

| Part | Source | Work Required |
|------|--------|---------------|
| Preface | README.md, mxcli-overview.md | Rewrite as introductory chapters |
| I: Tutorial | New | Write from scratch (~4 chapters). Most important new content. |
| II: MDL Language | 01-language-reference.md, 02-data-types.md, 03-domain-model.md, skill files | Reorganize into chapters, add examples. ~60% exists. |
| III: Project Tools | MDL_QUICK_REFERENCE.md, mxcli-overview.md, CLAUDE.md | Reorganize and expand. ~70% exists. |
| IV: IDE Integration | README.md, vscode-mdl/package.json | Partially exists, needs expansion. ~40% exists. |
| V: Go Library | GO_LIBRARY.md, api/api.go | Exists, needs minor updates. ~80% exists. |
| VI: Statement Reference | 01-language-reference.md, MDL_QUICK_REFERENCE.md | **Major work**: split into per-statement pages, add examples to each. ~30% exists (syntax yes, examples/notes no). |
| VII: Internals | ARCHITECTURE.md, MDL_PARSER_ARCHITECTURE.md, PAGE_BSON_SERIALIZATION.md | Exists, minor reorganization. ~90% exists. |
| VIII: Appendixes | MDL_QUICK_REFERENCE.md, 02-data-types.md, CLAUDE.md, SDK_EQUIVALENCE.md | Mostly exists, needs formatting. ~75% exists. |

**Estimated new writing**: ~40% of total content. Heaviest in Tutorial (Part I) and Statement Reference (Part VI).

---

## Tooling

### Option A: mdBook (Recommended)

[mdBook](https://rust-lang.github.io/mdBook/) is a Rust tool that builds books from Markdown. Used by the Rust Programming Language book.

**Pros**:
- Markdown source (matches existing docs)
- Built-in search (client-side, no server)
- Single-binary, fast builds
- PDF export via `mdbook-pdf` plugin (uses Chrome headless)
- Clean, readable theme
- GitHub Pages deployment via GitHub Actions
- TOC sidebar with collapsible sections
- Previous/Next navigation
- `SUMMARY.md` defines structure (similar to PostgreSQL's hierarchy)

**Cons**:
- Less customizable than full static site generators
- PDF quality depends on Chrome rendering (good enough, not typeset quality)

**Example `SUMMARY.md`**:
```markdown
# Summary

[Preface](preface.md)

# Part I: Tutorial

- [Setting Up](tutorial/setup.md)
- [Exploring a project](tutorial/exploring.md)
- [Your First Changes](tutorial/first-changes.md)
- [Working with AI Assistants](tutorial/ai-assistants.md)

# Part II: The MDL Language

- [MDL Basics](language/basics.md)
- [data Types](language/data-types.md)
- [Domain model](language/domain-model.md)
  ...
```

### Option B: Docusaurus

Facebook's documentation framework (React-based).

**Pros**:
- Versioned docs (useful for Mendix version-specific content)
- Rich plugin ecosystem
- Better PDF via `docusaurus-pdf` or Typst pipeline
- Blog feature (for release announcements)
- Algolia DocSearch integration

**Cons**:
- Node.js dependency (heavier build)
- More configuration overhead
- Overkill for current scope

### Option C: Typst + Custom Pipeline

Already used for `mxcli-overview.typ`. Typst produces high-quality PDFs.

**Pros**:
- Best PDF quality (proper typesetting)
- Already familiar (mxcli-overview.typ exists)
- Programmable (variables, includes, templates)

**Cons**:
- No built-in web output (need separate HTML generation)
- Would need a dual pipeline: Typst for PDF, something else for web
- Smaller ecosystem than mdBook/Docusaurus

### Recommendation: mdBook + Typst Hybrid

- **mdBook** for the website (GitHub Pages) and search
- **Typst** for the PDF (high-quality typeset output)
- **Shared Markdown source** — mdBook reads Markdown natively; a build script converts to Typst
- **GitHub Actions** deploys both on merge to main

```
docs-site/
├── book.toml              # mdBook config
├── SUMMARY.md             # table of contents
├── src/                   # Markdown source (shared)
│   ├── preface.md
│   ├── tutorial/
│   ├── language/
│   ├── tools/
│   ├── reference/         # Per-statement pages
│   ├── internals/
│   └── appendixes/
├── typst/
│   ├── main.typ           # Typst entry point (includes from src/)
│   └── template.typ       # PDF styling
└── .github/workflows/
    └── docs.yml           # build + deploy both formats
```

---

## Hosting

### GitHub Pages (Recommended for Start)

- Free, automatic HTTPS
- Deploy via `gh-pages` branch or GitHub Actions
- URL: `mendixlabs.github.io/mxcli/` or custom domain
- No server to maintain

### Custom Domain (Later)

- `docs.mxcli.dev` or `mxcli.mendixlabs.com`
- CNAME record pointing to GitHub Pages
- Configure in repository settings

---

## Implementation Plan

### Phase 1: Skeleton and Tutorial (2-3 days)

1. Set up `docs-site/` with mdBook config
2. Write `SUMMARY.md` with full structure
3. Write Part I: Tutorial (4 chapters) — **this is the critical new content**
4. Stub all other parts with single-line descriptions
5. Deploy to GitHub Pages
6. Verify navigation, search, and linking work

### Phase 2: Reorganize Existing Content (2-3 days)

1. Move language reference content into Part II chapters (split by topic)
2. Move quick reference + CLI features into Part III chapters
3. Move architecture docs into Part VII
4. Move data type tables, reserved words, etc. into Part VIII appendixes
5. Add cross-links between conceptual chapters and statement reference

### Phase 3: Statement Reference (3-5 days)

1. Create per-statement pages for all MDL statements (~50-60 pages)
2. Each page: Synopsis, Description, Parameters, Notes, Examples, See Also
3. Start with the 10 most-used statements, then fill in the rest
4. Extract examples from mdl-examples/ and skill files

### Phase 4: PDF Pipeline (1 day)

1. Set up Typst template matching mdBook structure
2. Build script to generate Typst from Markdown source
3. GitHub Actions workflow to produce PDF on release
4. Add download link to the website

### Phase 5: Polish (1-2 days)

1. Review all cross-links
2. Add glossary (Appendix G) — essential for non-Mendix developers
3. Write Preface (what is mxcli, what is MDL, Mendix concepts)
4. Test PDF output, fix formatting issues
5. Add version selector if multiple Mendix versions need documenting

---

## What Happens to Existing Docs

| Current Location | Fate |
|-----------------|------|
| `docs/05-mdl-specification/` | Content moves to Parts II + VI. Original files become redirects or are removed. |
| `docs/01-project/ARCHITECTURE.md` | Moves to Part VII Chapter 26. |
| `docs/01-project/MDL_QUICK_REFERENCE.md` | Becomes Appendix A. |
| `docs/10-user-docs/mxcli-overview.md` | Content distributed across Parts I, III, IV. Original kept as standalone marketing doc. |
| `docs/GO_LIBRARY.md` | Moves to Part V. |
| `docs/03-development/` | Technical docs move to Part VII. |
| `README.md` | Stays as repo README, links to docs site. |
| `CLAUDE.md` | Stays as AI agent config, unchanged. |
| `docs/11-proposals/` | Stays as-is (internal, not part of user docs). |
| `docs/12-bug-reports/`, `docs/13-vision/`, `docs/14-eval/` | Stay as-is (internal). |
| `mdl-examples/` | Examples are referenced/included from Part VI statement pages. |

---

## Success Criteria

- A new user can go from "what is mxcli?" to running their first MDL command in under 10 minutes using the Tutorial
- Every MDL statement has a dedicated reference page with synopsis, parameters, and at least 2 examples
- The site is searchable (client-side search, no external service required)
- PDF is downloadable and includes all content with proper page numbers and TOC
- Deployed automatically on merge to main via GitHub Actions
- Non-Mendix developers can understand the docs (glossary, Mendix concepts preface)
