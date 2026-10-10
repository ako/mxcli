---
title: Cost That Scales With the Project, Not the Request
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/untimed_work.go
  - mdl/executor/cmd_microflows_derived_layout_backend.go
  - mdl/executor/cmd_microflows_derived_layout.go
  - mdl/executor/validate_duplicates.go
  - mdl/executor/helpers.go
---

> **Do not duplicate**: each memo's key and lifetime are canonical in the Go
> comments (`layoutCheckBackend`, `flowAnalysisMemo`, `untimedWork`); the
> measurements and the call counts live in the findings. This page is about the
> shape the slowness takes and how to find it.

## What this is

A one-statement script on a small app and the same script on a 140 MB one are
the same request, and for a while they were not the same work. The defects in
this class all have the form **the request is bounded and the work is a function
of the project**: `check` preloading every one of ~40 document kinds before
looking at the script, a describe re-deriving the stored flow's graph analysis
once per layout round, a single `if Module.Rule(...)` resolving every rule's
module — and each module resolution listing the whole project, which on MPR v1
reads every unit's contents out of SQLite.

The reason these arrive late and together is that correctness work created them.
The canonical describe derives layout by **re-rendering** the description several
times; `create or modify` compares what the statement builds with what is
stored, so it builds; `check` predicts `exec`, so it consults what `exec` would
consult. Each is the right design, and each multiplies a per-construct cost by a
round count.

## How it fits

**The symptom is a product of factors, every one of them individually
harmless.** A describe that ran past a five-minute timeout was *per-statement
backend lookup* × *rebuild rounds* × *per-candidate project listing* ×
*per-ancestor listing*: four factors, none worth a second look alone, 53 seconds
for one lookup together. Finding it is not finding "the slow function"; it is
finding the chain. Two consequences follow. Filter by the **cheap key before the
expensive one** — compare a bare name before resolving modules — and a read
cache built for a repeated derivation has to memoise **every** backend read the
builder makes, not the ones the first profile happened to show.

**Profile before believing the issue's theory.** This is the most reliable
lesson in the class, and it has been paid for three times. A report blaming the
layout round count was right that rounds existed and wrong about the cost: the
rounds were already capped, the rebuild was cheap, and the expense was the
*render* inside each round. A CI suite blamed on `create or modify` building
every declared flow was, by CPU profile, a sixth spent in the **test harness**
fully unmarshalling every unit of the fixture to read two fields. And a timeout
blamed on graph complexity was on one of the app's *smallest* flows.

Which gives the cheap triage: **compare the pathological document with the
largest ones first.** If the slow one is small, the cost cannot be a traversal,
and the search moves to per-construct work multiplied by something. A `SIGQUIT`
goroutine dump at a few moments, showing the same frame each time, plus one
hand-timed call, settled that case before any profile was taken.

**Assert counts, not wall time.** Every fix in this class is guarded by a
counter — split/merge analyses per describe, builds per plan, `IsRule` calls per
describe, project listings per lookup, backend `List*` calls per check — because
a wall-clock assertion on CI is flaky and a count is exact and names the
mechanism. A call-counting mock (reflect over its `List*Func` fields and fill
the nil ones) measures a check's appetite directly: 36 listings before, 3 after.
Reverting the fix must move the count, which is the control the number gives you
for free.

**A performance fix has to prove it changed nothing else.** The output of a
memoised describe is asserted **byte-identical** to the unfixed build's, and the
round trip still reports `Unchanged` with the MPR unchanged. Without that, a
"speed-up" is indistinguishable from a quietly different answer, and this is the
area where that mistake is easiest: the whole point of the change is to stop
recomputing something.

**Laziness is a semantic change unless you enumerate what can be read.** Making
`check`'s project name sets load per key is only semantics-preserving alongside a
static pre-pass over the statements for *which keys can ever be read*: a
mutation (drop, rename) on a key nothing reads may go to a throwaway set, and a
statement kind that shares a name space must still pay for it. The rewrite is
cheap; knowing it is equivalent is the work.

**A guard that discards half-finished work turns slow into permanently
broken.** The wall-clock guard counted an implicit catalog rebuild, and a
catalog build is only saved on completion — so a timeout left the cache exactly
as stale as before and guaranteed the next call repeated the same doomed build.
Two rules come out of it. **Exempt the work, not the statement**: the explicit
`REFRESH CATALOG` was exempt by statement type while the identical build reached
from `SHOW REFERENCES` was not, so the exemption marks the bounded section
wherever it runs. And a guard that throws work away must **say** so, or every
retry reads as a fresh failure rather than as the same one.

**The remedy has its own failure mode: parallelism shares the cache.** The
catalog's describe worker pool runs over one `ExecContext.Cache`, and a lazily
filled field with no synchronisation is a data race — several workers each
listing the modules and publishing the slice while others read it. The
measurement trap there is specific and worth knowing: **the race detector
reports a location pair once per process**, so a `-race` run with `-count>1`
passes on later iterations against unfixed code. Pair it with a
detector-free assertion — N concurrent lookups on a slow mock must call the
backend once.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the four
  multiplier chains, with their before/after counts
- [[scripts-that-cannot-rerun]] — the `create or modify` design whose comparisons
  are what repeats the build
- [[describe-round-trip-gaps]] — the derived-layout rounds exist to make the
  description faithful
- [[local-loop-silence]] — the other class where the cost is wall time rather
  than a wrong answer
