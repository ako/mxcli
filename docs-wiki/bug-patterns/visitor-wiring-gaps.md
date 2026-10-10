---
title: Visitor Wiring Gaps
category: bug-pattern
last-synced: a918689d
covers:
  - mdl/visitor
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor/
  - mdl/visitor/visitor_enumeration.go
  - mdl/visitor/visitor_helpers.go
  - mdl/visitor/visitor_silent_drops.go
  - mdl/visitor/visitor_doc_comment_placement.go
  - mdl/visitor/visitor_query.go
  - mdl/visitor/visitor_page_v3.go
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

**The mechanism underneath most of them is a default that is also a legal
value.** `buildDataType` ends in `return TypeString`, so a type keyword the
grammar accepts and the builder has no branch for stores an unlimited String —
no error, no warning, and `describe` printing `String(unlimited)` as if that is
what the author wrote. `HashedString` shipped that way; so did `float` and
`currency`. Nothing downstream can object, because the value is one a correct
script also produces. The guard that finds these is a table test over **every**
keyword in the rule, asserting each builds its own kind, and it found exactly the
one nobody had reported. The same shape with no default at all hides behind a
helper: a type switch over AST nodes with no `default` arm, reached through an
`extractVariableName`-style "return the name, or empty" helper, loses the operand
behind a value that reads as absence.

**Three control-flow shapes swallow a grammar alternative whole.** An `if/else`
chain with **no final else** in an action-rule listener: `alter enumeration … set
comment` parsed, reported success, and appended no statement at all — and a parse
that yields zero statements is silent by construction. A **token-presence chain**
where a branch keyed on one token claims every alternative containing it: `show
features for version 10.24` was answered by the `show version` branch 380 lines
above, because `for version` carries the same token. And a **flag read by one
branch** of a shared rule: `if not exists` was accepted on all five entity
alternatives and read by one, the view-entity AST having no field for it. Each
has a cheap structural guard — grep the rule's alternatives against the chain,
order the chain most-specific first, drive the test from the grammar's own list
of kinds — and none is caught by a test that only asserts *some* statement was
produced.

**One rule serving two documents, disambiguated by shape, has no error path.**
The placeholder rule means "declare a placeholder" without a body and "fill this
placeholder" with one, told apart by the presence of braces. So `placeholder Main
{ }` inside a `CREATE LAYOUT` does not fail — it silently means the other thing,
and the failure surfaces later as a runtime message flatly contradicting the
script (`layout "X" declares no placeholder`, on a script that says `placeholder
Main`). **A runtime error that contradicts the source text is the tell for this
shape**, and the mistake it punishes is the natural one: every other layout
element takes a body.

**Record what was parsed and not consumed, at the rule that admits it.** The
grammar lets a doc comment precede *every* statement; a handful of exits read
one. So a `drop microflow if exists` written between a flow's doc comment and its
`create` silently ate the documentation of six flows — `mx check` clean, `exec`
clean, the round trip stable. Auditing every reader is the wrong size of fix:
recording the comment that was parsed and consumed by nobody, once, at the rule
that admits it, covers the readers nobody has written yet. Surveying mxcli's own
examples with that guard then found the same loss in this repository's scripts.

**Resolve a shorthand in the visitor, not as a grammar alternative.** A bare
entity as a data source (`DataSource: Mod.Car`) matched no alternative and fell
through to the generic key/value branch, so it parsed, executed and stored
nothing. The first fix added a grammar alternative and broke pluggable widgets,
whose generic keys are their own package's keys and whose `datasource:` may bind
an attribute. Keying the shorthand on the widget type in the visitor keeps the
generic path intact — the grammar is the wrong place to encode which widgets a
shorthand is for.

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
- [[the-visitor-walks-a-failed-parse]] — the same bridge failing loudly, on a
  tree the grammar says is impossible
- [[two-meanings-one-script]] — where the wiring is right and the question is
  which meaning applies
