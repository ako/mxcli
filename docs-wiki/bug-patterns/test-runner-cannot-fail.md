---
title: A Test Runner That Cannot Fail
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli.jsonl
  - cmd/mxcli/testrunner/parser.go
  - cmd/mxcli/testrunner/generator_endpoint.go
---

> **Do not duplicate**: the annotation reference and the `.test.mdl` format live
> in `.claude/skills/test-microflows.md` and `docs/15-testing/`; the individual
> fixes live in the findings. This page describes why the class is uniquely
> dangerous.

## What this is

Fourteen `cmd/mxcli` findings are `mxcli test` reporting **PASS** for something
that did not hold, or did not run, or was never evaluated. `@expect 1 = 2`
passed. `@verify` — the annotation covering the harder half of Mendix testing,
where a microflow's only observable effect is rows written — was parsed and read
by nothing but `--list`. `--require-assertions` exited 0 on a suite where nothing
asserted. A test with both `@throws` and `@expect` reported two assertions and
made one.

Every other class in this wiki costs a debugging session. This one **spends
confidence that was never earned**: green output is the entire product, and a
runner that cannot fail is worse than no runner, because a real suite gets
written against it and believed.

## How it fits

**Silent absence, not silent breakage.** The recurring mechanism is not a wrong
answer but a missing consumer. An annotation is parsed into a struct field, and
nothing downstream reads it. `grep -n '\.Verify'` over the package was the whole
diagnosis. The parse succeeding is what makes it invisible: `--list` prints the
annotation back, so the feature looks wired.

**Two result-assembly paths, and a decision that lives in one of them.** The
runner has two backends — the after-startup runner and the HTTP test endpoint —
and results were assembled separately in each. A flag consulted in one loop, a
field populated at one of five `TestResult` literal sites, a verdict computed in
the endpoint path only: each is the [[duplicate-resolver-drift]] shape landing
where its consequence is a false pass. The remedy that held was structural — one
constructor, one pre-run verdict function — pinned by a test asserting it is the
*only* call site.

**Fail closed.** An annotation that claims to assert something and cannot be
honoured is now an **error**, and the test does not run. That is the opposite of
the instinct (drop what you cannot handle and carry on), and it is the only
policy under which a passing report means anything.

**The default output has to distinguish an assertion from a silence.** A test
that asserted nothing printed the same `PASS` as one with six assertions. Putting
the count behind `--verbose` would have missed the point: the *unread* output is
where the false confidence lives.

**Substring where structure was needed.** Two defects came from searching text
instead of parsing it. Unanchored annotation patterns matched inside prose, so a
doc comment *explaining* `@expect $x = 1` gave the test that assertion — found
while writing the reproduction for another bug, whose header caused it. And a
line-number estimate searched reconstructed text inside the raw bytes, which
cannot match under CRLF and panicked with a negative slice bound.

**The environment is part of the assertion.** A suite that passes under
`--attach` and fails under `--local` is usually neither: the two boot paths
differed in whether constant overrides were applied and whether the project's own
after-startup microflow was chained. Nothing in either output explained the
difference. Both are now printed, and both boot paths share one options mapping.

**A test run must not change the project.** The runner injects a module and takes
it back out; restoring the model still moved the `.mpr`'s bytes, so a
"tests pass and the tree is clean" CI step failed on a meaningless diff. Snapshot
and restore the file, and refuse to restore when cleanup failed — a tree that
reads clean while the project is not is worse than a visible discrepancy.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each annotation,
  its consumer, and the control that proves it can now fail
- [[duplicate-resolver-drift]] — the same structural cause, in the executor
- `.claude/skills/test-microflows.md` — the annotation reference itself
