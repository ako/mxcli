---
title: Size containers from their contents, not from a statement count
status: accepted
---

# Proposal: Size containers from their contents, not from a statement count

Closes the last open item of upstream [#884](https://github.com/mendixlabs/mxcli/issues/884) —
problem 1, "container `Size` is unrelated to the positions of the activities inside it".

## Problem

A `LoopedActivity` is the only microflow element mxcli sizes rather than stamping
with a constant. Its `Size` is computed **before** its body is built, from a
pre-pass over the AST that counts statements. The nested builder that actually
places the children runs afterwards, by which time the box is frozen — so child
positions, including explicit `@position` overrides, have **no** influence on the
box that is supposed to contain them.

### Measured

Four microflows, one project, blank Mendix 11.6.6 app, mxcli `bcc3406`
(`mdl-examples/bug-tests/container-autosize-884.mdl`; `LOOP` body varied,
everything else identical):

| | Body | Child centres (X) | Loop `Size` |
|---|---|---|---|
| **A** | 2 activities, default placement | 150, 310 | `480;160` |
| **B** | same 2, `@position(1500,60)` / `@position(2000,60)` | 1500, 2000 | `480;160` |
| **C** | same 2, `@position(160,60)` / `@position(170,60)` | 160, 170 | `480;160` |
| **D** | 4 activities, default placement | 150 … 630 | `800;160` |

A, B and C are the same box around contents spanning 160px, 500px and 10px.
D differs from A only in statement count, and only D changes the box. Width is
exactly `(n-1)·HorizontalSpacing + ActivityWidth + 2·LoopPadding + iteratorSpace`
— a function of `n` alone.

**B is a correctness problem, not a cosmetic one.** The box interior spans
x ∈ [0, 480]; both children sit at 1500 and 2000, entirely outside their own
container. `mx check` reports the *same* error count before and after the script
runs — validation does not model geometry, so nothing catches it. It is visible
only by opening the flow in Studio Pro.

This is what the reporter observed as "identical child positions, different
`Size`" — the two quantities are simply not connected.

### Where it comes from

`measureStatementsSpan` (`mdl/executor/layout.go:92`) takes
`[]ast.MicroflowStatement` and returns `(count-1)*HorizontalSpacing + ActivityWidth`.
It is called at exactly two places, both of which size a container:

- `mdl/executor/cmd_microflows_builder_control.go:572` — `addLoopStatement`
- `mdl/executor/cmd_microflows_builder_control.go:896` — `addWhileStatement`

Neither has any child object at that point. `loopBuilder` — the nested
`flowBuilder` whose `objects` carry the real positions — is constructed about
twenty lines later.

The comment on `measureStatementsSpan` is honest about the half-fix it already
is (#790): the count-based span was introduced to *stop over-sizing* boxes, and
it falls back to the older `measureStatements` for compound bodies precisely
because it "cannot reproduce the builder's geometry without duplicating it".
That is the tell — the pre-pass is trying to predict a computation that has not
run yet.

## Why this is not a one-line change

The current ordering contains a cycle that the obvious fix walks straight into:

```go
loopHeight  := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)  // needs body
innerStartY := loopHeight / 2                                        // body needs this
loopLeftX   := fb.posX
loopCenterX := loopLeftX + loopWidth/2                               // needs width
```

Children are placed relative to an inner origin derived **from the size**, and
under the fix the size must be derived **from the children**. Anything that
computes the size from real positions has to break that dependency first, which
is why this is a layout-engine change rather than a patch.

## Proposal

**Build first, size after, translate once.**

1. **Build the body at a provisional origin** — run `loopBuilder` from `(0, 0)`.
   Nothing in the body placement genuinely needs the box; it needs *an* origin.
2. **Take the real bounding box** from `loopBuilder.objects`
   (`Position ± Size/2` over every object, recursively for nested containers).
   This is ground truth, and it accounts for `@position` for free — no annotation
   plumbing required.
3. **Size the box** as bbox + `LoopPadding` on all sides, plus `iteratorSpace` on
   the left for `LOOP`, floored at `MinLoopWidth` / `MinLoopHeight`.
4. **Translate the body once** — a single pass over `loopBuilder.objects` adding
   the delta between the provisional origin and the final inner origin, mutating
   `Position` only.

Step 4 is what keeps this safe. No builder call site learns about sizing; the
translation is a pure post-pass over objects that already exist. That is the
same single-choke-point shape used for `@curve` (`applyFlowCurves`) and `@merge`
(`mergePosition`) — chosen for the same reason: a rule threaded through N
placement sites fails silently at the one site that was missed.

### Nested containers

The bbox must be computed bottom-up: an inner loop has to be sized before the
outer loop measures it. This already falls out of the recursion — `addStatement`
sizes the inner `LoopedActivity` before it is appended to
`loopBuilder.objects` — but it becomes load-bearing under this change and should
be asserted by a test with two nesting levels, not left to hold by accident.

### Semantics of `@position` inside a container

Two readings, and they must be decided explicitly rather than emerging from the
implementation:

- **(a) container-relative, translated with everything else** — the box grows to
  contain the annotated child, and the child keeps its position *relative to the
  body*.
- **(b) authoritative, exempt from translation** — the annotated child stays put
  and unannotated siblings move around it.

**Recommend (a).** Positions inside a `LoopedActivity` are already stored
relative to the container, so (a) matches the storage model; (b) would leave two
adjacent children obeying different origins, which is unexplainable in a doc and
unpredictable in a diff. Whichever is chosen, it belongs in
`.claude/skills/mendix/` alongside the `@position` reference.

## Outcome — shipped

Implemented for `LOOP` and `WHILE`: the box is sized from the children that were
actually built (`fitContainerSize`), after the body exists rather than from a
pre-pass over the AST. Measured on the four cases above, which previously all
reported `480;160` except D:

| | children (X) | before | after |
|---|---|---|---|
| A | 150, 310 | `480;160` | `420;160` |
| B | `@position` 1500, 2000 | `480;160` | `2110;140` |
| C | `@position` 160, 170 | `480;160` | `280;140` |
| D | 150 … 630 | `800;160` | `740;160` |

Nesting needed no extra work: an inner loop is sized when its own
`addLoopStatement` returns, so the outer container measures a correct inner box
(verified — inner `580;160`, outer `1320;260`).

### One deliberate divergence: the body is NOT translated

The plan above called for "build-first / size-after / **translate-once**". The
translation step was dropped, and the recommendation on `@position` semantics
(container-relative, translated) is superseded by it.

A child's position round-trips through `DESCRIBE` as `@position`. Translating the
body would therefore make a describe→exec cycle store *different* coordinates
than it read — and under [ADR-0008](../13-decisions/0008-identity-and-idempotence.md)
that converts an otherwise-quiet re-run into a write. Growing the box to fit the
contents achieves the same containment without moving anything the author placed,
which is also what a hand-laid-out loop needs. Verified: a loop with
`@position(300,90)` / `@position(700,90)` round-trips byte-identically, and
re-running a script is a no-op (with the `MXCLI_ALWAYS_WRITE=1` control confirming
the comparison detects writes).

Also shipped: the containment lint rule, as **MPR011** — MPR009 and MPR010 were
already taken (gallery selection listener, dataview layout grid), so the ID named
below was wrong.

Still open from this proposal: the `@size` escape hatch (still rejected by MDL059).

## Non-goals

- **Splits.** `IF` / enum split / inheritance split have no `Size` property in
  Mendix storage — there is no box, so there is nothing to fix. `measureStatements`
  stays: it is still needed to reserve horizontal room for branches *before* they
  are laid out, which is a genuinely predictive use.
- **Re-flowing a body to fit a box.** Out of scope. The box follows the contents,
  never the reverse.

## Alternative considered: an `@size(w, h)` escape hatch

The reporter asked for this directly, and it is far cheaper — one annotation, one
field, no layout change. It is rejected as the *primary* fix for two reasons: it
makes the author responsible for a number the engine can compute, and it leaves
the default behaviour wrong for everyone who does not use it.

Worth revisiting **after** the derived size lands, if a real case wants a box
deliberately larger than its contents. Note the coupling introduced by the #884
annotation work: `@size` is currently **rejected** by MDL059, so adding it means
adding it to `knownActivityAnnotations` in `mdl/executor/validate_microflow.go`.
The drift test compares the visitor's case labels against that list in both
directions and fails if only one side changes — which is the intent.

## Risk

**Every generated flow containing a `LOOP` or `WHILE` changes geometry.** That is
unavoidable for a fix of this shape, and has two consequences worth stating in
the release note:

- Fixtures and golden BSON comparisons churn once, in a single commit.
- Under idempotent writes (ADR-0008), the first re-run of any existing script
  against an existing project **writes** the affected microflows; subsequent runs
  go quiet again. Users will see one round of version-control changes they did
  not author.

Neither is a reason not to do it, but a silent geometry shift across an existing
project is exactly the kind of thing that gets reported as a new bug.

## Verification

- **Regression test from the table above.** Cases A–D as a builder-level test
  asserting the invariant directly: *every child's bounding box lies inside its
  parent's*. That is the property; the specific pixel values are not.
- **Nesting test** — loop inside a loop, both boxes containing their contents.
- **`@position` test** — case B: the box must grow to contain a child pushed to
  x=2000, not leave it outside.
- **Control run.** Per the standing rule, the test must be shown to fail against
  a pre-fix binary with the reported symptom — a same-size box around different
  contents — and not merely pass against fixed code.
- **`mx check`** on every fixture, both engines.

### Candidate lint rule: MPR011 (shipped)

The measurement above shows `mx check` is blind to this. A rule
*"every child of a `LoopedActivity` lies within its parent's box"* would have
caught the entire class from the outside, and would keep catching it if a future
layout change reintroduces it. It sits naturally beside MPR008 (which, after the
#884 work, already partitions objects by canvas and therefore has the container
geometry in hand). Shipped as MPR011: verified to fire on a project built by a
pre-fix binary — where `mx check` reports nothing — and silent on the fixed
output, the stock blank app and a nested-loop project.
