---
title: Visitor Wiring Gaps
category: bug-pattern
last-synced: fda04711
sources:
  - .claude/skills/fix-issue.md
  - mdl/visitor/visitor_enumeration.go
  - mdl/executor/cmd_odata.go
  - mdl/ast/ast_odata.go
---

> **Do not duplicate**: the specific fix recipes (issues #393, #594) live in the `.claude/skills/fix-issue.md` symptom rows, and the canonical wiring blocks live in `mdl/visitor/`. This page describes the pattern only.

## What this is

A family of bugs where a hand-written, field-by-field copy layer silently drops information. The grammar accepts the user's intent, the AST struct has a field for it, the model and writer both carry it, and `DESCRIBE` knows how to print it — yet the value never appears in the persisted document. The compiler can't help: every layer in mxcli's pipeline is bridged by Go code that walks fields one at a time, and there is no static check that every field was faithfully propagated.

## How it fits

mxcli's pipeline runs grammar → visitor → AST → executor → backend, and at least two layers in that chain are hand-written field-by-field code without compiler help: the **visitor** (parse tree → AST) and the **executor's `or modify` branches** (AST → existing model). Both fail in the same way — the user wrote something, every other layer is wired, but one layer's copy loop forgot to faithfully carry the value through.

The **visitor variant** is the classic. Each `ExitCreateXxxStatement` must explicitly read fields off the ANTLR parse-tree context and assign them onto the AST struct. When a new `CREATE` variant is added, it's easy to wire every other layer and forget one assignment in the visitor — so a doc-comment vanishes, or `OR MODIFY` silently degrades to plain `CREATE`. The canonical fix is to diff the broken visitor against a known-good sibling: the enumeration and constant visitors in [`mdl/visitor/`](../../mdl/visitor/visitor_enumeration.go) share the two standard blocks verbatim (`stmt.Documentation = findDocCommentText(ctx)` for doc-comments, and the `OR MODIFY`/`OR REPLACE` flag detection).

The **executor `or modify` variant** is the same pattern at a different layer. An `or modify` handler that reads `existingEntity.X = s.X` for every field unconditionally treats "user omitted X" (`s.X` is the zero value) and "user explicitly cleared X" (`s.X` is also the zero value) identically — so omitted fields get wiped to empty strings or `false` on the persisted entity. The fix shape is symmetric to the visitor case but lives in the AST representation rather than the visitor: model omitted-vs-explicit by changing the AST scalar fields to pointers (`*string`, `*bool`) so the executor can gate every assignment on `if s.X != nil` and preserve the prior value otherwise. See `CreateExternalEntityStmt` in [`mdl/ast/ast_odata.go`](../../mdl/ast/ast_odata.go) and the gated assignment block in [`mdl/executor/cmd_odata.go`](../../mdl/executor/cmd_odata.go).

The common signal across both variants: a roundtrip silently loses the user's intent — no error from mxcli, no failure from Studio Pro at write time, the property just isn't there when you read it back.

## See also

- [fix-issue symptom table](../../.claude/skills/fix-issue.md) — the per-instance fix recipes for both the visitor (#393) and the executor-omit-overwrite (#594) variants
- [[../architecture/mdl-execution]] — the grammar → visitor → AST → executor pipeline these gaps sit in
- [[../models/storage-vs-qualified-names]] — a related "Studio Pro silently ignores wrong value" failure mode, but in the writer
- [[../rationale/mdl-as-sql]] — why MDL statements roundtrip through `DESCRIBE`
