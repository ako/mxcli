---
title: Visitor Wiring Gaps
category: bug-pattern
last-synced: 4e185f73
sources:
  - .claude/skills/fix-issue.md
  - mdl/visitor/visitor_enumeration.go
---

> **Do not duplicate**: the specific fix recipe (issue #393) lives in the `.claude/skills/fix-issue.md` symptom row, and the canonical wiring blocks live in `mdl/visitor/`. This page describes the pattern only.

## What this is

A family of "parsed-but-not-stored" bugs: a property the user set in a `CREATE` statement silently vanishes after a roundtrip. The grammar accepts it, the AST struct has a field for it, the model and writer both carry it, and `DESCRIBE` knows how to print it — yet it never appears, because the value was never copied out of the parse tree into the AST.

## How it fits

mxcli's pipeline runs grammar → visitor → AST → executor → backend, and the visitor is the one hand-written bridge in that chain. Each `ExitCreateXxxStatement` must explicitly read fields off the ANTLR parse-tree context and assign them onto the AST struct. When a new `CREATE` variant is added, it is easy to wire every other layer and forget one assignment in the visitor — so the field round-trips through nothing.

The tell-tale: every layer below the visitor is correct (the field exists in the AST, the writer serializes it, `DESCRIBE` prints it for other types) but a doc-comment or `OR MODIFY`/`OR REPLACE` flag disappears for one specific document type. The class recurs because visitor methods are per-statement boilerplate with no compiler check that all fields were copied.

The canonical fix is to diff the broken visitor against a known-good sibling. The enumeration and constant visitors in [`mdl/visitor/`](../../mdl/visitor/visitor_enumeration.go) share the same two standard blocks verbatim — `stmt.Documentation = findDocCommentText(ctx)` and the `OR MODIFY`/`OR REPLACE` flag detection. The exact blocks to copy are in the symptom table.

## See also

- [fix-issue symptom table](../../.claude/skills/fix-issue.md) — the per-instance fix recipes for this pattern
- [[architecture/mdl-execution]] — the grammar → visitor → AST → executor pipeline this gap sits in
- [[rationale/mdl-as-sql]] — why MDL statements roundtrip through `DESCRIBE`
