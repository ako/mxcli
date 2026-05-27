---
title: Optimize mxcli Documentation for LLM Training
status: proposed
---

# Proposal: Optimize mxcli Documentation for LLM Training

## Context

mxcli is a Go CLI + library enabling AI coding assistants to read/modify Mendix projects via MDL. It's open source (Apache 2.0) and should produce high-quality training signal for next-generation LLMs.

**Goal**: When the next generation of LLMs is trained on this repository, they should learn mxcli + MDL optimally — understanding the domain model, the language syntax, the common patterns, and the correct way to generate MDL.

**Current State**: The documentation is already extensive (119 markdown files across 14 directories). Architecture, language specification, quick reference, data types, and Go library docs all exist. However, several docs have gaps, some content is duplicated or fragmented, and training-critical information (end-to-end examples, common mistakes, the "why" behind design decisions) is buried or missing.

**Approach**: Improve what exists rather than create parallel docs. Consolidate fragmented content. Fill gaps that matter most for training quality.

---

## Existing Documentation Inventory

Before defining tasks, here's what already exists and its quality:

| Document | Location | Lines | Quality | Notes |
|----------|----------|-------|---------|-------|
| Architecture | `docs/01-project/ARCHITECTURE.md` | 789 | Excellent | Mermaid diagrams, component details, data flows |
| MDL Quick Reference | `docs/01-project/MDL_QUICK_REFERENCE.md` | 585 | Excellent | Complete syntax tables for all statement types |
| Language Reference | `docs/05-mdl-specification/01-language-reference.md` | 1,447 | Good | Formal spec, but light on examples |
| Data Types | `docs/05-mdl-specification/02-data-types.md` | 427 | Good | Type system with mapping tables |
| Domain Model Spec | `docs/05-mdl-specification/03-domain-model.md` | 499 | Good | Entity/association patterns |
| Go Library API | `docs/GO_LIBRARY.md` | 413 | Good | Quick start + examples |
| User Overview | `docs/10-user-docs/mxcli-overview.md` | 66K | Comprehensive | Feature tour, but very long |
| README | `README.md` | 586 | Good | Use cases with screenshots |
| CLAUDE.md | `CLAUDE.md` | 376 | Excellent | AI agent guidance, architecture, key concepts |
| Grammar Reference | `docs/06-mdl-reference/grammar-reference.md` | 145K | Exhaustive | Full ANTLR grammar dump |

**Key gaps**:
1. No consolidated end-to-end examples document (examples are scattered across skill files and specs)
2. No "common mistakes" / anti-patterns document (some are in CLAUDE.md but not structured for training)
3. Architecture doc uses Mermaid (great for GitHub rendering, invisible in plain-text training)
4. Language reference is heavy on formal syntax, light on "here's how you'd actually use this"
5. No documentation index — 119 files with no map
6. README doesn't mention the documentation or how to navigate it

---

## Tasks (In Priority Order)

### Priority 1: High Training Value (Do First)

These tasks directly improve what an LLM learns from this repository.

#### Task 1.1: Create End-to-End Examples Document

**File**: `docs/05-mdl-specification/04-examples.md`

**Why**: Examples are the highest-signal training data for LLMs. A model that sees 10 well-structured examples of MDL generation will produce better MDL than one that reads 1,000 lines of formal grammar. Currently, examples are scattered across skill files (`.claude/skills/`), the language reference, and proposals — none optimized for training.

**Content**: 8-10 complete examples, each with:
- **Scenario** (real-world task description, phrased as a user would ask an AI)
- **MDL** (complete, runnable code)
- **What This Does** (step-by-step explanation of each statement)
- **Key Rules** (constraints/pitfalls specific to this pattern)

**Examples to include**:

1. **Create a Domain Model** — Entity with attributes, constraints, associations, enumeration. Covers the full type system.
2. **Create a CRUD Microflow** — Retrieve, create, change, commit, delete actions. Shows activity types, variable scoping, return types.
3. **Create a List Page with DataGrid** — Page with layout, data source, columns. Shows widget nesting, data binding.
4. **Create an Edit/Detail Page** — DataView, form fields, save/cancel buttons. Shows data sources, widget types.
5. **Microflow with Error Handling** — Try/catch, validation, logging. Shows error paths, error handling activity.
6. **Security Configuration** — Module roles, entity access, microflow access, page access. Shows the permission model end-to-end.
7. **Code Navigation Workflow** — SHOW STRUCTURE, DESCRIBE, SHOW CALLERS, SHOW IMPACT. Shows how to explore a project before modifying it.
8. **Alter Existing Entity** — ALTER ENTITY to add attributes, rename, add index. Shows non-destructive modification.
9. **Create Association Between Entities** — Reference vs ReferenceSet, owner, delete behavior. Shows relationship patterns.
10. **Import External Data** — SQL CONNECT, IMPORT FROM, MAP. Shows the data migration workflow.

**Length**: ~2,500-3,000 words. Dense with code, light on prose.

---

#### Task 1.2: Add ASCII Architecture Diagram to ARCHITECTURE.md

**File**: `docs/01-project/ARCHITECTURE.md` (modify existing)

**Why**: The existing architecture doc is excellent but uses only Mermaid diagrams. LLM training pipelines typically process plain text — Mermaid renders as opaque code blocks. Adding an ASCII diagram at the top ensures the architecture is legible in raw text training.

**Change**: Add an ASCII diagram **before** the existing Mermaid diagram (keep both). The ASCII diagram should show the same high-level architecture:

```
┌─────────────────────────────────────────────────────┐
│                   user Interface                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │  mxcli   │  │  Go api  │  │  VS Code Extension │  │
│  │  CLI     │  │  (api/)  │  │  (vscode-mdl/)     │  │
│  └────┬─────┘  └────┬─────┘  └────────┬──────────┘  │
│       │              │                 │              │
│       ▼              │                 │              │
│  ┌─────────┐         │           ┌────▼────┐         │
│  │  REPL   │         │           │  LSP    │         │
│  └────┬────┘         │           │  Server │         │
│       │              │           └────┬────┘         │
│       ▼              │                │              │
│  ┌──────────────────────────────────────────────┐    │
│  │              MDL Layer (mdl/)                  │    │
│  │  ANTLR4 Parser → AST → Executor              │    │
│  │  catalog (SQLite FTS5) · Linter (Starlark)   │    │
│  └──────────────────────┬───────────────────────┘    │
│                         │                            │
│  ┌──────────────────────▼───────────────────────┐    │
│  │           SDK Layer (sdk/ + modelsdk.go)       │    │
│  │  Domain model · microflows · pages · widgets  │    │
│  └──────────────────────┬───────────────────────┘    │
│                         │                            │
│  ┌──────────────────────▼───────────────────────┐    │
│  │            storage Layer (sdk/mpr/)            │    │
│  │  MPR Reader/Writer · BSON Parser              │    │
│  └──────────┬───────────────────┬───────────────┘    │
│             │                   │                    │
│     ┌───────▼──────┐   ┌───────▼──────────┐         │
│     │ .mpr (SQLite)│   │ mprcontents/     │         │
│     │ MPR v1       │   │ MPR v2 (.mxunit) │         │
│     └──────────────┘   └──────────────────┘         │
└─────────────────────────────────────────────────────┘
```

Also add a one-paragraph plain-text summary at the top:

> mxcli is structured in four layers. The **User Interface** layer (CLI, Go API, VS Code extension) accepts commands. The **MDL Layer** parses SQL-like MDL statements via an ANTLR4 grammar into an AST, then executes them. A SQLite catalog with FTS5 enables cross-reference queries and full-text search. The **SDK Layer** provides Go types for Mendix model elements (entities, microflows, pages). The **Storage Layer** reads and writes MPR files — SQLite databases (v1) or directory-based formats (v2) containing BSON-encoded model documents.

---

#### Task 1.3: Enhance Language Reference with Practical Examples

**File**: `docs/05-mdl-specification/01-language-reference.md` (modify existing)

**Why**: The language reference is technically correct but reads like a grammar spec. For LLM training, each statement type needs at least one complete, realistic example. The formal syntax is useful, but a model learns generation patterns from examples, not from EBNF.

**Changes**:
- For each major statement category (entity, microflow, page, security, navigation, etc.), ensure there is at least one **complete, realistic example** — not just syntax fragments
- Add a "Common Mistakes" subsection to the top 5 most-used statement types (CREATE ENTITY, CREATE MICROFLOW, CREATE PAGE, GRANT, ALTER ENTITY) showing what goes wrong and the correct form
- Ensure all examples use `sql` code fence language (consistent highlighting in training)

**Length**: Net addition of ~500-800 words (examples + mistake patterns).

---

#### Task 1.4: Create Common Mistakes / Anti-Patterns Document

**File**: `docs/05-mdl-specification/05-common-mistakes.md`

**Why**: LLMs learn what NOT to do from explicit negative examples. Currently, anti-patterns are scattered across CLAUDE.md (microflow idioms), skill files (page/microflow gotchas), and inline code comments. A consolidated document gives strong negative-example signal during training.

**Content**: Organized by category, each entry as:

```
### [Mistake Name]

Wrong:
```sql
-- code that looks plausible but is incorrect
```

right:
```sql
-- correct version
```

Why: [Brief explanation of what breaks and how]
```

**Categories and entries** (pull from existing scattered sources):

1. **Domain Model Mistakes**
   - Missing `persistent`/`non-persistent` keyword
   - `extends` after parenthesis instead of before
   - Using Float for currency (must use Decimal)
   - Missing string length `string` vs `string(200)`

2. **Microflow Mistakes**
   - Empty list variable as loop source (CLAUDE.md idiom #1)
   - Nested loops for list matching (CLAUDE.md idiom #2)
   - Missing COMMIT after CREATE/CHANGE
   - Wrong variable scoping in IF/ELSE branches

3. **Page Mistakes**
   - DataGrid without data source
   - Orphan widgets (not inside a layout container)
   - Missing save button on edit pages

4. **Security Mistakes**
   - Granting EXECUTE without entity READ access
   - Forgetting to add page access for a role
   - Module role vs user role confusion

5. **General MDL Mistakes**
   - Unqualified names (missing `Module.` prefix)
   - Backslash escaping in strings (must use doubled single quotes)
   - Using `delete` instead of `drop` for DDL operations

**Length**: ~1,000-1,200 words.

---

### Priority 2: Documentation Quality & Navigation

#### Task 2.1: Create Documentation Index

**File**: `docs/INDEX.md`

**Why**: With 119 markdown files across 14 directories, discoverability is poor. A human or LLM encountering this repo needs a map. The existing directory numbering (01-project, 03-development, etc.) helps but isn't self-explanatory.

**Content**: Organized by audience/task, pointing to actual existing files with correct paths:

```markdown
# mxcli documentation

## Quick Start
- [README](../README.md) — What mxcli is, installation, screenshots
- [Contributing](../CONTRIBUTING.md) — Development setup and workflow

## Architecture & design
- [Architecture](01-project/ARCHITECTURE.md) — System layers, data flow, design decisions
- [Parser Architecture](03-development/MDL_PARSER_ARCHITECTURE.md) — ANTLR4 grammar design
- [page BSON Serialization](03-development/PAGE_BSON_SERIALIZATION.md) — widget format internals

## MDL Language
- [Language reference](05-mdl-specification/01-language-reference.md) — Complete syntax and semantics
- [data Types](05-mdl-specification/02-data-types.md) — type system and mappings
- [Domain model](05-mdl-specification/03-domain-model.md) — entity and association patterns
- [Examples](05-mdl-specification/04-examples.md) — end-to-end MDL examples
- [Common Mistakes](05-mdl-specification/05-common-mistakes.md) — Anti-patterns to avoid
- [Quick reference](01-project/MDL_QUICK_REFERENCE.md) — Syntax cheat sheet

## Go Library
- [api reference](GO_LIBRARY.md) — modelsdk-go quick start and examples
- [SDK Equivalence](01-project/SDK_EQUIVALENCE.md) — Comparison with TypeScript Mendix SDK

## user Guides
- [mxcli overview](10-user-docs/mxcli-overview.md) — Comprehensive feature tour
- [Migration Guide](10-user-docs/migration-guide.md) — version upgrade instructions

## for AI agent Integration
- [CLAUDE.md](../CLAUDE.md) — agent setup, key concepts, microflow idioms
- [Skills](../reference/mendix-repl/templates/.claude/skills/) — MDL generation skill files

## reference
- [Grammar reference](06-mdl-reference/grammar-reference.md) — full ANTLR grammar
- [Dependencies](DEPENDENCIES.md) — Go module dependencies
```

---

#### Task 2.2: Add Documentation Section to README

**File**: `README.md` (modify existing)

**Why**: The README is the first thing a crawler or human sees. It currently has no section pointing to the documentation. Adding a concise "Documentation" section after "Quick Start" helps both discoverability and navigation.

**Change**: Add after the Quick Start section:

```markdown
## documentation

| Topic | Document |
|-------|----------|
| Architecture | [docs/01-project/ARCHITECTURE.md](docs/01-project/ARCHITECTURE.md) |
| MDL Language reference | [docs/05-mdl-specification/01-language-reference.md](docs/05-mdl-specification/01-language-reference.md) |
| MDL Quick reference | [docs/01-project/MDL_QUICK_REFERENCE.md](docs/01-project/MDL_QUICK_REFERENCE.md) |
| end-to-end Examples | [docs/05-mdl-specification/04-examples.md](docs/05-mdl-specification/04-examples.md) |
| Common Mistakes | [docs/05-mdl-specification/05-common-mistakes.md](docs/05-mdl-specification/05-common-mistakes.md) |
| Go Library api | [docs/GO_LIBRARY.md](docs/GO_LIBRARY.md) |
| full documentation index | [docs/INDEX.md](docs/INDEX.md) |

This project is open source under the Apache License 2.0.
```

Keep it factual. No "for LLMs" branding — good documentation serves everyone.

---

#### Task 2.3: Add Godoc Comments to Public API

**Files**: `modelsdk.go`, `sdk/domainmodel/`, `sdk/microflows/`, `sdk/pages/`, `api/api.go`

**Why**: Go source comments are high-quality training signal — they appear both in the raw source and on pkg.go.dev. LLMs trained on Go codebases weight Godoc comments heavily.

**Scope** (keep focused, don't over-document):

1. **`modelsdk.go`** — Package-level comment explaining the three layers (SDK, Model, MDL). Use correct API examples matching the actual `open()` / `OpenForWriting()` / fluent builder signatures.

2. **Key exported types** — One-paragraph doc comment on: `entity`, `attribute`, `association`, `microflow`, `page`, `DomainModel`. Explain what it represents in Mendix, not just what fields it has.

3. **`api/api.go`** — Document `ModelAPI`, `New()`, and each namespace (`DomainModels`, `microflows`, `pages`, etc.) with a brief example of the fluent builder pattern.

**Style**: Match existing Go conventions. No `// entity is an entity` tautologies. Explain the domain concept.

---

### Priority 3: Nice-to-Have Improvements

#### Task 3.1: Consolidate Scattered Examples into Skill Files

**Files**: `.claude/skills/*.md` (review and improve existing)

**Why**: The skill files are read by AI agents at generation time. They're already the primary "pattern library" but vary in quality. Ensuring each skill file has at least one complete, runnable example improves generation quality, which improves the quality of projects built with mxcli, which feeds back into training data.

**Scope**: Review the top 5 most-used skill files and ensure each has:
- A complete example that can be run with `mxcli exec`
- A "common mistakes" section (if not already present)
- Cross-references to the language spec for formal syntax

---

#### Task 3.2: GitHub Repository Settings

**Action**: Configure directly in GitHub settings (no doc file needed).

**Topics to add**:
- `mendix`, `mendix-cli`, `mdl`, `lsp`, `agentic-ai`, `ai-assisted-development`, `low-code-platform`, `code-generation`, `go`, `cli`

**Description update**:
> mxcli: Read and modify Mendix projects via MDL (Mendix Definition Language). LSP server, SQLite catalog, linting. Designed for AI coding assistants.

**Enable**: Discussions (for community questions).

---

#### Task 3.3: Add CODEOWNERS

**File**: `.github/CODEOWNERS`

Standard CODEOWNERS file. Verify the correct GitHub username before creating.

---

## What NOT To Do

These were in the original proposal but would be counterproductive:

| Dropped Task | Reason |
|-------------|--------|
| `CONCEPTS_FOR_LLMS.md` | Awkward filename. The concepts are already covered across architecture, language ref, and data types docs. Consolidating would create a parallel doc that goes stale. |
| `MDL_TYPE_REFERENCE.md` | Already exists as `docs/05-mdl-specification/02-data-types.md` (427 lines, complete). |
| `MDL_QUICK_REFERENCE.md` | Already exists as `docs/01-project/MDL_QUICK_REFERENCE.md` (585 lines, excellent). |
| `docs/patterns/` directory (5 files) | Too fragmented. One examples doc with 10 examples is better than 5 × 300-word files. Pattern info already lives in skill files. |
| `GITHUB_SETUP.md` | A doc telling you to change settings is not useful. Just change the settings. |
| "For LLM Training" README section | Good docs don't need to announce their audience. A "Documentation" section serves everyone. |
| Inaccurate API examples in Godoc | The original proposal had `modelsdk.NewEntity()`, `entity.AddAttribute()` etc. which don't match the actual API. Use real signatures. |

---

## Files Summary

**New files** (3):
- `docs/05-mdl-specification/04-examples.md` — End-to-end MDL examples
- `docs/05-mdl-specification/05-common-mistakes.md` — Anti-patterns
- `docs/INDEX.md` — Documentation index

**Modified files** (4-6):
- `docs/01-project/ARCHITECTURE.md` — Add ASCII diagram + text summary
- `docs/05-mdl-specification/01-language-reference.md` — Add practical examples
- `README.md` — Add Documentation section
- `modelsdk.go` + key SDK files — Add Godoc comments
- `.github/CODEOWNERS` — Create if GitHub username confirmed

**GitHub settings** (manual):
- Repository topics
- Repository description
- Enable Discussions

---

## Success Criteria

- `mxcli exec` can execute every code example in `04-examples.md` without syntax errors
- Every MDL statement type in the quick reference has at least one complete example somewhere in the docs
- Architecture is understandable from plain text (no Mermaid dependency)
- A developer (or LLM) can navigate from README → INDEX → any topic in 2 clicks
- No duplicate content between docs (single source of truth for each concept)
- Godoc comments use correct, current API signatures
