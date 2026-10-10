---
title: An Absent Edge Reads As An Absent Fact
category: bug-pattern
last-synced: ce063188
covers:
  - mdl/catalog
sources:
  - .claude/skills/fix-issue/findings/
  - mdl/catalog/builder_references.go
  - mdl/catalog/builder_graph.go
  - mdl/catalog/tables.go
---

> **Do not duplicate**: the CATALOG view list and column meanings are in
> `mxcli help catalog` and `mdl/catalog/tables.go`; each missing edge and its
> fix are in the findings. This page is about why a gap in the index is graded
> as a wrong answer rather than a thin one.

## What this is

The catalog is an index, and an index answers two very different questions with
the same silence: *nothing references this* and *I did not look there*. The
`mdl/catalog` findings are overwhelmingly the second masquerading as the first —
an edge the builder never wrote, read back as a fact about the model.

What makes the class expensive is the consumer. A user does not read
`CATALOG.REFS` and shrug; they read `show callers of`, `impact`,
`GRAPH_DEAD_ASSETS` or lint QUAL004, and those phrase the absence as a
conclusion. One of them appends **"Remove if unused."**

## How it fits

**A missing edge is worse than a missing table.** A missing table is an absence
the user notices — the query errors, or the column is obviously not there. A
missing edge is a confident wrong answer, and in the reported cases it arrived
from three directions at once: `show callers of` saying `(no callers found)`,
`GRAPH_DEAD_ASSETS` listing the document, and QUAL004 recommending its deletion.
That agreement is not corroboration. All three read the same empty `refs` table,
so one missing `emitActionRefs` call reproduces itself as a chorus.

**An entry point is not a caller, and must still count as one.** Everything that
runs a microflow from outside the call graph has been this bug once: a scheduled
event, the project's `AfterStartupMicroflow`, an entity event handler, a
published REST operation, a workflow event handler, a user task's
`on created microflow`. Each was reachable, in production, and reported dead.
The pattern is not "the call graph is incomplete" — it is that *reachability* and
*invocation* are different relations, and a model built from the second answers
questions asked of the first.

**Know which consumer filters on kind before blaming the kind list.**
`graphRefKinds` governs the *analysis* graph — communities, layers, cycles,
centrality. `GRAPH_DEAD_ASSETS` does not consult it: the view asks only whether
any `refs` row targets the name, whatever its kind. A comment claiming otherwise
sat beside the list and sent one investigation's root-cause analysis to the wrong
place. The distinction decides whether a fix is "add the kind to a list" or "emit
the row at all".

**Depth is where the walk stops, and the walk stops early.** An entity used only
inside a List View specialization template, a DataGrid2 column, a gallery item or
a chart series was absent from `CATALOG.WIDGETS` — so a page holding nineteen
sparklines never appeared under "which pages use this widget?", while the grid
around them did. The same for a `commit`, `delete` or `change` on a loop
iterator: the iterator's type lives on the `LoopedActivity`, so a walk over
flattened actions never resolves it. Recursing on the structure beats
special-casing a depth, because the next container is already being designed.

**One switch resolving a fact does not mean the other does.** Two builders derive
per-action facts from the same parsed actions — the refs switch and the activities
switch — and drift independently, so an action can carry its target in `refs` and
an empty column in `activities`. When a row's field is empty, check whether a
sibling switch already resolves it before writing new resolution. That is the
local form of [[duplicate-resolver-drift]].

**A stale cache is silent under-reporting, and it looks exactly like a clean
project.** Adding a table, a view, a column or a `RefKind` obliges a
`CatalogSchemaVersion` bump: `refs` is written only by `REFRESH CATALOG FULL`,
and the schema applies with `CREATE TABLE IF NOT EXISTS`, so without the bump an
existing `.mxcli/catalog.db` keeps the old shape, the new query fails, the error
is swallowed and the rule reports zero of its findings. The constant is a merge
hazard of its own: two branches bumping it write the same line, so the collision
is invisible and one side's caches never rebuild.

**Discoverability is part of correctness.** A view queryable by name but absent
from `show catalog tables` is a view nobody finds. Two hand-maintained lists of
the same thing need a guard test comparing them — which is the difference between
a five-second fix and a feature that stays hidden for months.

**When two tables disagree, ask whether they answer different questions.**
`GRAPH_CYCLES` returning zero rows while `GRAPH_MODULE_COUPLING` listed the same
pair in both directions was not a broken algorithm: the granularity and the edge
filter were both correct, undocumented, and invisible from SQL. Making the
difference queryable is the fix; "one of them is wrong" was not.

**The diagnosis tip that generalises.** When a write appears not to have
persisted, read the raw stored unit before suspecting the writer. A hardcoded
catalog column and a missing `DESCRIBE` clause both present as a silent no-op,
and both are read-path illusions — the model on disk was right the whole time.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  missing edges, their kind lists and their controls
- [[duplicate-resolver-drift]] — the same shape when two resolvers answer one question
- [[a-rule-that-cannot-fire]] — the lint rules built on this index, and their own silence
- [[misleading-diagnostics]] — when the wrong answer is a hint rather than a row
