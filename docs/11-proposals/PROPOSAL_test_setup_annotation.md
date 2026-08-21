---
title: "@setup for mxcli test"
status: draft
date: 2026-08-21
---

# Proposal: `@setup` — give a test the state it needs, and say so

**Status:** Draft
**Date:** 2026-08-21

## Problem Statement

`@setup` is parsed today and read by nothing. `parseAnnotations` matches it into
`TestCase.Setup`, and no runner, generator or reporter ever looks at that field.
`@setup CreateTestData` in a `.test.mdl` file is a no-op that reports nothing.

It arrived with the original framework in
[proposal-mdl-test-framework.md](proposal-mdl-test-framework.md) (draft,
2026-02-23) as one row of an annotation table — "Reference to setup block" — and
was never designed past that row. There is no syntax anywhere for declaring a
setup block, no execution semantics, and no other mention of setup in the
proposal body. `git log -S` confirms the field and its pattern landed with the
first testrunner commit and have not been touched since.

This is the same silent-absence class as the three defects fixed in #927 and
#926 before it — an annotation present in the file and absent from the run — but
it fails in the *other* direction. A dropped `@expect` makes a test pass when it
should fail. A dropped `@setup` means the fixture never exists, so the test
usually **fails confusingly**: it asserts against an empty database and reports a
mismatch that has nothing to do with the code under test. Where the test asserts
nothing, it passes vacuously.

Two things follow. The annotation must stop being a no-op, whichever way that is
resolved. And the shape it takes should be the one that earns its place over the
line of MDL an author can already write.

There is no BSON in this feature. It touches the test runner's generated
microflows and its own file format; no Mendix document type is read or written.

## What a setup can and cannot do here

The design space is set by how a test executes, so this comes first.

`mxcli test --local` (and `--attach`) invokes **one microflow per HTTP request**
against the generated endpoint. Per request, the handler
(`cmd/mxcli/testrunner/endpoint.go`):

- creates a **fresh system context**,
- when `rollback=1`, calls `ctx.startTransaction()`, runs the microflow, and
  rolls back in a `finally`,
- refuses any microflow not named `MxTest.Test_*`.

Three consequences, and they decide the design:

1. **The transaction is per request.** Anything that must be undone with the test
   has to run *inside the test's own microflow call*. A separate request runs in
   its own context and its own transaction; its writes are not covered by the
   test's rollback.
2. **A once-per-file fixture therefore cannot be rolled back at all.** The
   endpoint has no seam for a transaction spanning several requests. Whatever a
   suite-level setup writes stays written when the run ends. Under `--local` that
   is the `<project>_test` database and merely untidy; under `--attach` it is
   **the developer's own dev database**, which mxcli would be seeding behind
   their back. That asymmetry is the strongest single argument in this document.
3. **A setup emitted into the test's microflow needs no protocol change.** No
   Java handler edit, no new route, no client change, and it behaves identically
   on `--local`, `--attach`, Docker and the legacy after-startup runner. Anything
   requiring a new request needs all four to agree.

## Proposed design

**`@setup <Module.Microflow>` names a microflow to call before the test's own
statements, in the test's own transaction.** It is repeatable, and it may be
declared once for a whole file.

```mdl
/**
 * Seeds every test in this file. A header comment's annotations apply to the
 * tests below it.
 * @setup eShop.ACT_SeedCatalog
 */

/**
 * @test the catalogue seeds five brands
 * @expect count($Brands) = 5
 */
retrieve $Brands from eShop.CatalogBrand;
/

/**
 * @test a brand can be renamed
 * @setup eShop.ACT_SeedOneBrand
 * @expect $brand/Name = 'Renamed'
 */
retrieve $brand from eShop.CatalogBrand;
$brand = call microflow eShop.ACT_Rename(Brand = $brand, Name = 'Renamed');
/
```

The generated microflow is what an author would write by hand, with the setup
calls first:

```mdl
CREATE OR REPLACE MICROFLOW MxTest.Test_test_1 ()
RETURNS String AS $Verdict
BEGIN
  DECLARE $Verdict String = 'PASS';
  $mxtest_setup_1 = CALL MICROFLOW eShop.ACT_SeedCatalog() ON ERROR {
    SET $Verdict = 'SETUP:eShop.ACT_SeedCatalog';
    RETURN $Verdict;
  };
  retrieve $Brands from eShop.CatalogBrand;
  ...
```

### The rules

- **It is a microflow, not a new kind of block.** A fixture in a Mendix app is a
  microflow; the runner's whole job is calling microflows. Naming one needs no
  declaration syntax, no cross-block reference resolution, and no new concept in
  the file format.
- **Setups run in order, before the body**, file-level ones first, then the
  test's own. Repeating the annotation is how you compose two fixtures.
- **A setup runs inside the test's transaction.** Under the `@cleanup rollback`
  default it is undone with the test, so every test starts from the same state —
  which is the property that makes a fixture worth having. Under `@cleanup none`
  it persists, like everything else that test writes.
- **A failing setup is an ERROR, not a FAIL.** The distinction is the point: the
  test never ran, so it neither passed nor failed, and a suite full of assertion
  mismatches caused by one broken seed is the failure mode this annotation exists
  to prevent. The verdict protocol gains a third prefix (`SETUP:`) alongside
  `PASS` and `FAIL:`.
- **An unresolvable microflow is refused before the run**, by name. Fail-closed,
  as with every other annotation in this package: a `@setup` naming a microflow
  that does not exist must not produce a run that quietly did no setup.
- **`--list` shows it**, so `mxcli test <file> --list` says what a test depends on
  without booting anything.

### Why this over the alternatives

| Alternative | Why not |
|---|---|
| **A named setup *block* in the `.test.mdl` file**, as the original table row implies | Needs a declaration syntax, a reference resolver, and an error for every way the two can disagree — to express something a microflow already expresses. It also puts fixture logic in a file the app cannot run, so nothing else in the project can reuse it. |
| **Once per file, one request before the tests** | Cannot be rolled back (consequence 2 above), so it seeds the developer's own database under `--attach`. Also needs a teardown story, an ordering story with `--filter`, and agreement across four runners. Worth revisiting only with a suite-scoped transaction, which the endpoint cannot express today. |
| **Leave it to the author: `call microflow X;` as the body's first line** | Already possible, and for a single test it is honestly fine. What it cannot do is attribute the failure (a throwing seed reports "exception during execution" of the *test*) or apply to a whole file without copy-paste. Those two are the feature. |
| **Delete `@setup`** | Cheapest, and a real option — see Open Questions. It leaves the copy-paste case unimproved but costs nothing to maintain. |

### What this does not do

- No teardown counterpart. `@cleanup rollback` already covers the common case,
  and a `@teardown` that runs after a rolled-back test would be asserting against
  state that no longer exists — the trap `@verify` hit.
- No parameters. `@setup Mod.Flow` calls a microflow with no arguments; a fixture
  that needs arguments gets a wrapper microflow.
- No sharing across files. File scope is the largest scope that stays inside one
  test's transaction.

## Implementation Plan

Order: parse → generate → report → validate. Each step is independently testable
and the first two are the whole feature.

### Files to modify

| File | Change |
|------|--------|
| `cmd/mxcli/testrunner/parser.go` | `Setup` becomes `[]string` (repeatable); `setupPattern` already anchored; merge the file header's setups ahead of each test's |
| `cmd/mxcli/testrunner/parser.go` | Header annotations: a leading doc comment with no `@test` currently yields no test and is discarded — keep its annotations as file-level defaults. #927's `scanDocComments` already isolates the header, which is what makes this cheap |
| `cmd/mxcli/testrunner/generator_endpoint.go` | Emit the setup calls at the top of `writeExpectFlowBody` / `writeThrowsFlowBody`, each with an `ON ERROR` handler setting the `SETUP:` verdict and returning |
| `cmd/mxcli/testrunner/generator.go` | Same for the monolithic runner, with the per-test variable suffix (`$mxtest_setup_1_3`). It reports over the log protocol rather than a returned verdict, so a setup failure emits a new `MXTEST:ERROR:` line — added to the marker list `ParseLogResults` scans, which today knows only START/RUN/PASS/FAIL/SKIP/END |
| `cmd/mxcli/testrunner/generator_endpoint.go` | `verdictSetupPrefix` beside `verdictPass` / `verdictFailPrefix` |
| `cmd/mxcli/testrunner/client.go` | `toResult` maps a `SETUP:` verdict to `StatusError` with the microflow named — a fourth arm on the switch that already tells "the test failed" from "the microflow threw". Its `default` reports an unrecognised verdict, so an unhandled `SETUP:` would error rather than pass, which is the right way round to get this wrong |
| `cmd/mxcli/testrunner/results.go` | Nothing: `StatusError` is already counted with the failures |
| `cmd/mxcli/testrunner/runner.go` | Validate the generated flows with `mxcli check … --references` before `exec`, so an unknown `@setup` microflow is named before the app boots |
| `cmd/mxcli/testrunner/runner.go` | `--list` prints each test's setups |
| `.claude/skills/mendix/test-microflows.md`, `docs-site/src/tools/test-annotations.md` | Document the annotation, the ordering, and the ERROR-not-FAIL rule |
| `mdl-examples/doctype-tests/` | A `.test.mdl` exercising file-level and per-test setup |

The validation step is the one with a blast radius: `--references` resolves
*every* reference in the generated flows, not just the setups, so a test body
that names something the project does not have would start being refused before
the run instead of failing during it. That is an improvement, and it is also a
behaviour change that can turn a suite red on upgrade. It should land as its own
commit, after the feature, so it can be reverted alone.

## Version Compatibility

None. This is mxcli's own test-file format and its own generated microflows; no
Mendix version gate is involved. The generated MDL uses `CALL MICROFLOW … ON
ERROR`, which the runner already emits for every test body.

The legacy after-startup runner supports the feature (it is a generated
statement, not a protocol change), unlike `@cleanup rollback` and `@verify`,
which it cannot honour.

## Test Plan

Unit, in `cmd/mxcli/testrunner`:

- `@setup` parses repeatably, and file-level setups precede a test's own.
- A file header's annotations reach the tests below it — with the control that a
  header carrying `@test` is still the two-tests-in-one-block error from #927.
- The generated flow calls the setups **before** the body, and the emitted MDL
  parses (`visitor.Build`, as the existing generator tests do).
- Monolith: two tests with the same setup do not collide after renaming.
- A `SETUP:` verdict becomes `StatusError`, is counted with the failures, and
  names the microflow.
- Control: a test with no `@setup` generates byte-identical MDL to today.

End to end, the way #927's count support was verified — unit tests prove the
parser, not Mendix: exec the generated flows into a real project and run
`mx check`, with a pristine-copy baseline. A generated `CALL MICROFLOW … ON
ERROR` that does not compile is invisible to `mxcli check`.

And the control that matters: revert the generator change and confirm the setup
microflow is absent from the generated MDL — the symptom this proposal exists to
fix is an annotation that generates nothing.

## Open Questions

1. **Is the feature worth its weight at all?** Deleting `@setup` — pattern,
   field, and the one control test that pins it as parseable — is a defensible
   answer and costs nothing to maintain. The case for keeping it rests on two
   things: failure attribution (ERROR naming the seed, instead of a FAIL blaming
   the test) and a file-level fixture without copy-paste. If neither is worth a
   day, delete it and say so in the docs.
2. **Should a bare `@setup` with no argument mean "this block is the fixture"?**
   It would let a fixture live in the test file without a microflow — at the cost
   of the second meaning for one tag, and of fixture logic the app cannot reuse.
   Recommended: no, at least not first.
3. **Does the header-annotation concept want to be general?** Once a file header
   can carry `@setup`, the question of whether it can carry `@cleanup` — a
   file-wide default — answers itself in the affirmative, and that is a bigger
   change than this proposal. Recommended: `@setup` only, and refuse the others
   in a header by name rather than ignoring them.
4. **Should the `--references` validation land at all?** It is the only way to
   refuse an unknown setup microflow before boot, and it can turn an existing
   suite red for unrelated reasons. Alternative: resolve only the setup names,
   via a targeted lookup rather than a whole-script check.
