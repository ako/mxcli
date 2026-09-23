---
title: MDL Execution Pipeline
category: architecture
last-synced: 4e185f73
sources:
  - mdl/grammar/MDLParser.g4
  - mdl/visitor/visitor.go
  - mdl/executor/executor.go
  - mdl/backend/backend.go
  - mdl/backend/doc.go
  - mdl/backend/domainmodel.go
  - docs/03-development/MDL_PARSER_ARCHITECTURE.md
---

> **Do not duplicate**: MDL syntax (see `docs/01-project/MDL_QUICK_REFERENCE.md`), function-level implementation (read source), per-feature design (see `docs/11-proposals/`), or decision rationale (cite ADRs in `docs/13-decisions/`).

## What this is

The path a single MDL statement travels from text on disk to a write against a `.mpr` file. It is five layers — grammar, parse tree, AST, visitor, executor, backend — each one narrowing untrusted input into a more structured, more validated form before the next stage touches storage. Understanding the layering matters because almost every class of bug lives at the seam between two of them.

## How it fits

MDL is parsed by an [ANTLR4 grammar](../../mdl/grammar/MDLParser.g4) split into a top-level dispatch file plus per-domain grammar imports (domain model, microflow, page, security, and so on). ANTLR produces a parse tree; a [listener-based builder](../../mdl/visitor/visitor.go) walks that tree and constructs strongly-typed AST nodes. The visitor is also where raw syntax errors are caught and rewritten into human-actionable hints — reserved-keyword collisions, unescaped apostrophes, quoted GRANT attributes — so the failure a user sees explains the fix rather than ANTLR's internal token names. The full layer-by-layer design lives in [MDL_PARSER_ARCHITECTURE.md](../../docs/03-development/MDL_PARSER_ARCHITECTURE.md).

The [executor](../../mdl/executor/executor.go) dispatches each AST statement to a handler, enforcing per-statement output and wall-clock guards and tracking session state (created/dropped units, modified domain models for security reconciliation). Crucially, handlers never reach into the storage engine directly: they call through the [backend interface](../../mdl/backend/backend.go), a composition of domain-specific sub-interfaces ([DomainModelBackend](../../mdl/backend/domainmodel.go), `MicroflowBackend`, `PageBackend`, and others). Shared value types live in `mdl/types` so the [backend package](../../mdl/backend/doc.go) stays free of BSON dependencies — see [[rationale/backend-abstraction]]. This is what lets a mock backend exercise handler logic without an MPR file, and what isolates all BSON/SQLite concerns behind one boundary.

Common failure modes track the seams: parse errors at the grammar/visitor boundary, nil parse-tree nodes when the visitor reads partial trees (see [[bug-patterns/visitor-wiring-gaps]]), and storage-name or serialization faults below the backend in [[architecture/mpr-read-write]].

## See also

- [docs/03-development/MDL_PARSER_ARCHITECTURE.md](../../docs/03-development/MDL_PARSER_ARCHITECTURE.md) — full layer-by-layer parser design
- [mdl/backend/](../../mdl/backend/) — backend interface and per-domain sub-interfaces
- [[rationale/backend-abstraction]] — why the executor never reaches past `ctx.Backend`
- [[rationale/mdl-as-sql]] — why MDL is SQL-shaped in the first place
- [[architecture/mpr-read-write]] — what the backend writes into
- [[bug-patterns/visitor-wiring-gaps]] — failures at the visitor seam
