---
title: Visitor Wiring Gaps
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor.jsonl
  - mdl/visitor/visitor_enumeration.go
  - mdl/visitor/visitor_helpers.go
---

> **Do not duplicate**: the per-instance fix recipes and the exact blocks to copy
> live in the findings; the canonical wiring lives in `mdl/visitor/`. This page
> describes the pattern only.

## What this is

A family of "parsed-but-not-stored" bugs. The grammar accepts the input, the AST
struct has a field for it, the model and writer both carry it, and `DESCRIBE`
knows how to print it — and it never appears, because the value was never copied
out of the parse tree into the AST.

The visitor is the one **hand-written** bridge in the grammar → visitor → AST →
executor → backend pipeline, and it is per-statement boilerplate with no compiler
check that every field was copied. That is the whole mechanism.

## How it fits

**The gap comes in three sizes, and they present very differently.**

*A field.* The original instance: an enum-level doc comment vanishing after a
round trip, a `CREATE OR REPLACE` flag silently running as a plain `CREATE`. Every
layer below the visitor is correct, and the loss is one document type wide. The
canonical fix is to diff the broken visitor against a known-good sibling — the
enumeration and constant visitors share the same two standard blocks verbatim.

*A structure.* An `ELSIF` chain whose middle arms were dropped on write. Mendix
has no native `elsif`, so each arm has to be lowered into a nested `if` in the
previous arm's `else` branch; a visitor that reads only the first and last arms
produces a valid microflow that is missing behaviour. The read path was never
wrong — a round trip showed `if … else …` because the arms were absent from the
model, which is the diagnostic that separates a write gap from a read gap.

*A whole statement.* A `DROP` or a `SHOW` that parses, exits 0, prints nothing
and does nothing, because the statement reached the AST and no dispatch case. The
symptom reads as an empty result — "this page has no content", "there are no
connections" — rather than as a missing feature, which is what makes it survive.
Anything that can end in an empty output needs to distinguish *nothing found*
from *nothing ran*.

**The neighbouring failure is a field wired to the wrong thing**, which is worse
than one not wired at all: a delete-behaviour keyword storing a different
behaviour, a dollar-quoted SQL body overwritten by a parameter's default. Those
report success and change the model's meaning, where an unwired field merely
loses it.

**Guard the class, not the instance.** These recur because the boilerplate is
per-statement. The remedies that have held are structural: route every qualified
name through one accessor rather than `GetText()`, and assert reachability — a
test that every exported validator is called from the one entry point catches a
whole statement that was never wired, which no per-feature test can.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-instance
  recipes and the blocks to copy
- [[mdl-execution]] — the pipeline this gap sits in
- [[duplicate-resolver-drift]] — the sibling shape one layer down
- [[expression-translation-drift]] — where the visitor changes meaning rather than
  dropping it
