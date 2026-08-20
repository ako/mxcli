---
title: Structured description of irreducible microflow graphs
status: draft
date: 2026-08-20
---

# Proposal: Structured description of irreducible microflow graphs

**Status:** Draft
**Date:** 2026-08-20

`DESCRIBE MICROFLOW` renders a microflow's control flow as nested `if/then/else`.
That works only for graphs that are *properly nested*. A Mendix microflow is an
arbitrary directed graph, and when a branch re-enters a sibling branch's path the
describer walks it as though it were a tree and emits MDL that **means something
else** — with no error, no warning, and no indication that anything was lost.

Origin: [mendixlabs/mxcli#923](https://github.com/mendixlabs/mxcli/issues/923).

## Problem Statement

### The measured case

The reporter's microflow was reconstructed from the coordinates in their own
`describe` output and reproduced exactly (a unit test on a hand-built graph
emits their output verbatim, `@merge` annotations included):

```
start  → split1
split1 : true → split2      false → merge1
split2 : true → merge1      false → merge2
merge1 → log → merge2 → end
```

`log` runs when `¬c1 ∨ (c1 ∧ c2)`. mxcli describes it as:

```
if not(true) then
  if not(true) then
    log info node 'NODE' 'Do something';
  end if;
end if;
```

which means `c1 ∧ c2`. The edge `split1 --false--> merge1` is silently dropped and
rendered as an absent `else`.

**With the reporter's actual expressions this is the exact inverse on every run.**
Both conditions are `not(true)` = false, so `¬c1` is true and the original
**always** logs; the description's outer guard is false, so it **never** logs. The
issue title says "roundtrip does not work"; the accurate statement is that
`DESCRIBE` produces a program with the opposite behaviour.

### Why it is not nested

`merge1` is split1's join, but it also sits on split2's true path. The two blocks
*interleave*: split1's branches are not disjoint before their join. MDL's
`if/then/else` can only express single-entry/single-exit regions, and nothing
checks that the graph is one.

### The second, related defect

Two independent merge-finders disagree on exactly this graph:

| | split1 | split2 |
|---|---|---|
| `findSplitMergePointsForGraph` (drives structure) | **merge2** | merge2 |
| `commonMergeAfter` (emits `@merge(x, y)`) | **merge1** | merge2 |

On a properly nested graph they always agree, so **the disagreement is itself a
reliable detector**. Here it means the emitted `@merge(-200, 173)` places split1's
merge *before* the log at `(-50, 173)`, while structurally the outer `end if` falls
*after* it — which is the tangled diagram in the reporter's third screenshot. Their
"apart from positioning issues" is the same root cause, not a separate annoyance.

Not a regression: latent since the describer was written, visible only on
non-nested graphs.

## BSON Structure

No new BSON is written by this proposal. The relevant finding is a **negative**
one, and it forecloses the most obvious design.

### A merge node cannot carry a label

`Microflows$ExclusiveMerge` has no name, caption or documentation field. Three
independent sources agree:

| Source | Built from | Properties |
|---|---|---|
| `generated/metamodel` | Mendix reflection data | `RelativeMiddlePoint`, `Size` |
| `modelsdk/gen` | TypeScript SDK 4.114.0 | `RelativeMiddlePoint`, `Size` |
| Real documents (FeedbackModule, 11.13) | Studio Pro | `$ID`, `$Type`, `RelativeMiddlePoint`, `Size` |

`Microflows$ExclusiveSplit` sits directly above it in the metamodel *with*
`Caption` and `Documentation`. Mendix gave splits text and deliberately gave
merges none. The `generated/metamodel` snapshot caveat (it is 11.6.0) is covered
here by the third row: real 11.13 documents were dumped and carry nothing extra.

The only Mendix-native text attachable to a merge is an `Annotation`
(has `Caption`) wired by an `AnnotationFlow` (has `Origin`/`Destination`).
**Rejected**: it puts visible sticky notes in the user's diagram as mxcli
bookkeeping, the user can edit or delete them, and it authors model content the
model does not own — the failure mode ADR-0005 exists to prevent. Free
annotations also already have their own round-trip semantics
([PROPOSAL_microflow_free_annotation.md](PROPOSAL_microflow_free_annotation.md)),
which this would collide with.

### Labels do not need to survive storage

This is the key realisation, and it is why the negative finding above is not
fatal. A label is an artifact of the *text*, not of the model:

```
graph --DESCRIBE--> MDL (labels minted) --EXEC--> graph (labels consumed, discarded)
```

Nothing between two runs must remember `m1`. The requirement is not persistence
but **determinism**: the same graph must yield the same labels, so re-running a
script is a model-level no-op ([ADR-0008](../13-decisions/0008-identity-and-idempotence.md)).
Ordinal labels in a deterministic traversal order satisfy that, are more readable
than anything position-derived, and are *more* stable — moving a node in Studio
Pro changes its coordinates but not its ordinal.

Merge identity across re-runs is then the ordinary unnamed-element problem handled
by `canon.TransplantIDs`. **Flagged as a risk to measure, not to assume**: a merge
is the weakest possible case for that matcher — no `Name`, and a shape of just
`Microflows$ExclusiveMerge` — so matching degrades to positional.

## Proposed MDL Syntax

Three rendering modes. The first is today's behaviour, kept for the overwhelmingly
common case.

### Mode 1 — structured (default, unchanged)

Properly nested graphs describe exactly as they do today. No change, no new
syntax, no regression in readability for the ~all case.

### Mode 2 — faithful, with named merges (default for irreducible graphs)

A merge becomes a first-class **statement** rather than an inferred annotation,
and branches `join` it. Both words are Mendix's own vocabulary for the concept,
which keeps this closer to ADR-0003's "reads as English" than a bare `goto`:

```sql
create or modify microflow Test.Test_merge_boolean_codebug () returns Boolean as $output begin
  @position(-335, 173)
  if not(true) then
    @position(-335, 336)
    if not(true) then join m1; else join m2; end if;
  else
    join m1;
  end if;

  merge m1 @position(-200, 173);
  @position(-50, 173)
  log info node 'NODE' 'Do something';
  join m2;

  merge m2 @position(100, 173);
  @position(200, 173)
  return true;
end;
```

Properties:

- **`merge <name>` declares; `join <name>` jumps.** Forward and backward
  references both resolve, so declaration order is free.
- **It subsumes `@merge(x, y)`.** The merge carries its own `@position`, so there
  is nothing left to infer — which removes the two-merge-finders disagreement
  described above rather than papering over it.
- **No fall-through into a `merge` declaration.** In a microflow containing any
  `merge`, every path must end in `join`, `return`, or an end event. Fall-through
  would be ambiguous exactly where precision matters; a missing terminator is a
  `check` error, not a guess. (See Open Questions — the permissive alternative is
  viable and cheaper for hand-authors.)

### Mode 3 — normalized (opt-in)

Recombine guards into an equivalent nested form. For the reporter's graph,
`¬c1 ∨ (c1 ∧ c2)` simplifies to `¬c1 ∨ c2` and the whole microflow collapses to:

```sql
if not(not(true)) or not(true) then
  log info node 'NODE' 'Do something';
end if;
return true;
```

```sql
describe microflow Test.Test_merge_boolean_codebug normalized;
```

**Opt-in is load-bearing, not a convenience.** `DESCRIBE` → edit → `EXEC` is used
as a round-trip. Mode 3's output re-executes to a *different graph* — fewer nodes,
different layout, equivalent behaviour. Silently reshaping someone's diagram
because they asked to read it is its own guard-don't-drop violation. Someone
describing a microflow to change one activity must not get their canvas rebuilt.

## Can every graph be normalized? No — and the boundary is known

This is Böhm–Jacopini (1966): any control flow can be expressed with
sequence/selection/iteration, **but only if auxiliary boolean variables and/or
node duplication are permitted**. Without those two, no.

- **Recombinable** — the extra edges land on a shared *suffix*. The reporter's
  case: `merge1 → log → merge2` is one tail reached by two paths, so folding the
  guards costs nothing and duplicates nothing.
- **Interleaved** — genuinely crossed. Canonically `A → {B, C}`, `B → {D, E}`,
  `C → {D, E}`, where `D` runs iff `(a∧b) ∨ (¬a∧c)`. Nesting that requires either
  duplicating `D` and `E` — in Mendix, **two log activities where the user drew
  one** — or introducing a boolean temp the user never wrote. Both are model
  rewrites, not descriptions, and both are refused.

### Two preconditions before folding

**Purity and ordering.** Mendix split conditions are pure expressions, so
re-evaluating one is semantically safe. But the region being folded must contain
**only splits and merges**. If split1's true branch runs `create object` before
reaching split2, the guards cannot be folded without moving a side effect.
Checkable on the graph; cheap.

**Short-circuit evaluation — unresolved and load-bearing.** The original evaluates
`c2` *only when `c1` is true*. The folded `¬c1 ∨ c2` may evaluate `c2`
unconditionally. Mendix's reference guide documents `and`/`or` **without stating
whether they short-circuit**. If they do not, and `c2` can fail — division by
zero, or whatever the original guard was protecting against — normalization
introduces an error the original avoided. This must be **measured on a real
runtime** (`.claude/skills/verify-in-runtime.md`), not reasoned about, before Mode
3 ships. If `or` turns out to be eager, Mode 3 is restricted to conditions proven
total, or dropped.

## Implementation Plan

### Phase 0 — the detector, shipped first (gates everything else)

Nothing can choose a rendering mode without knowing whether a graph is nested, and
**no decision about how much machinery Modes 2 and 3 deserve should be made
without prevalence data**. Phase 0 ships the detector as a lint rule so the scan
below can be run against real projects.

Detection, for each split `S` with join `J`: compute each branch's reachable set
(`collectReachableDistances` already does this inside `findMergeForSplit`),
excluding `J`. If any two branches' sets intersect, the region is not
single-entry → irreducible. Cross-check: `findSplitMergePointsForGraph` and
`commonMergeAfter` disagree on `S`.

Classification for each irreducible split:

- **recombinable** — the intersection contains only splits and merges;
- **interleaved** — the intersection contains an activity, or the branches have
  more than one shared entry point.

### The prevalence scan (to be run against demo projects)

```bash
mxcli lint -p app.mpr --rule MDL-FLOW01 --format json
```

Emit per finding: qualified microflow name, module, split position,
classification, branch count, size of the overlap region. What the numbers decide:

| Result | Consequence |
|---|---|
| Irreducible graphs are rare | Ship Mode 2 only; refuse the rest. Mode 3 not worth building. |
| Common and mostly *recombinable* | Mode 3 earns its cost; it is the pretty answer for most of them. |
| Common and mostly *interleaved* | Mode 2 is the whole feature; Mode 3 would rarely apply. |

Until this is measured, Modes 2 and 3 are **unscheduled**.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/executor/cmd_microflows_show.go` | `classifyBranchStructure()` — the detector, reusing `collectReachableDistances` |
| `mdl/linter/rules/` | New rule `MDL-FLOW01`, reporting irreducible splits + classification (Phase 0) |
| `mdl/executor/cmd_microflows_show.go` | Warning line when describing an irreducible graph, alongside `duplicateOutputVariableWarnings` (Phase 0) |
| `mdl/grammar/domains/MDLMicroflow.g4` | `mergeStatement` / `joinStatement` rules (Phase 1) |
| `mdl/ast/ast_microflow.go` | `MergeDeclaration`, `JoinStatement` nodes (Phase 1) |
| `mdl/visitor/visitor_microflow_statements.go` | Parse-tree → AST (Phase 1) |
| `mdl/executor/cmd_microflows_builder_flows.go` | Resolve label → target, wire arbitrary `SequenceFlow`s (Phase 1) |
| `mdl/executor/cmd_microflows_show_helpers.go` | Emit the label form; retire `emitMergeAnnotation` for labelled output (Phase 1) |
| `mdl/executor/validate_microflow_*.go` | Unresolved `join`, duplicate `merge` name, missing terminator (Phase 1) |
| `mdl/executor/cmd_microflows_show.go` | `normalized` modifier + guard recombination (Phase 2) |
| `cmd/mxcli/syntax/features_*.go` | `mxcli syntax microflow` entries for `merge`/`join` |
| `docs-site/src/`, `.claude/skills/write-microflows.md` | User docs for the label form and when it appears |

Phase 0 is independently shippable and is the fix for #923 on its own: it turns a
silent semantic inversion into a named refusal.

## Version Compatibility

**No Mendix version gate.** `ExclusiveMerge` and arbitrary sequence flows exist in
every supported version; this is a describe/parse-side concern. No entry in
`sdk/versions/mendix-*.yaml` is needed.

## Test Plan

- **Unit, describer** — hand-built graphs via the existing
  `mkID`/`mkObj`/`mkFlow`/`mkBranchFlow` harness (`cmd_microflows_unpaired_merge_test.go`
  is the pattern). The reporter's exact graph is already reconstructed and
  reproduces their output verbatim; it becomes the regression fixture.
- **Control** — the same tests against the unfixed describer must show the
  inversion (`log` unreachable). A detector that only passes on fixed code has not
  been shown to detect anything.
- **Negative control** — properly nested graphs, including an inner split whose
  join *is* the outer's, must not be flagged and must describe byte-identically to
  today. This is the regression risk of the whole proposal.
- **Round-trip (Phase 1)** — describe → exec → describe on an irreducible graph
  must be a fixpoint, and the second exec must not write (ADR-0008 elision), with
  the `MXCLI_ALWAYS_WRITE=1` control run.
- **`mx check`** — every generated form at 0 errors.
- **Runtime (Phase 2 only)** — the short-circuit question above, settled by
  booting an app and observing whether an erroring right operand is evaluated.
- **Fixtures** — `mdl-examples/bug-tests/923-irreducible-microflow-graph.mdl`.

## Open Questions

1. **Short-circuit semantics of `and`/`or`** — undocumented; blocks Mode 3. Settle
   on a runtime before designing further.
2. **Fall-through into a `merge`** — this proposal requires an explicit terminator.
   The permissive alternative (fall-through means an implicit `join` to the next
   declared merge) is friendlier to hand-authors and shorter to read, at the cost
   of ambiguity in exactly the construct that exists to remove ambiguity.
3. **`normalized` as an MDL modifier vs a CLI flag.** The modifier keeps it in the
   language and works in the REPL; a flag keeps a rendering option out of the
   grammar. No existing `DESCRIBE` modifier sets precedent.
4. **`canon.TransplantIDs` on unnamed merges** — measure whether merge `$ID`s
   survive a describe → exec round-trip, or whether positional matching churns
   them. Affects whether Phase 1 output is genuinely idempotent.
5. **Verb choice** — `join <name>` vs `goto <name>`. `join` matches Mendix's own
   vocabulary and ADR-0003; `goto` is more immediately obvious to a developer and
   was the reporter's own instinct in the issue discussion.
6. **Loops.** This proposal addresses acyclic split/merge structure. Retry loops
   (issue #281) already make the flow graph cyclic and are handled by separate
   pass-through logic; whether the detector needs to exclude back-edges explicitly
   is unverified.
