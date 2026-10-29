---
title: The Platform CI Does Not Run
category: bug-pattern
last-synced: 5c24d899
covers:
  - cmd/mxcli
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - internal/procalive/
  - cmd/mxcli/docker/linkdir_windows.go
  - cmd/mxcli/docker/session_other.go
  - cmd/mxcli/docker/session_linux.go
  - cmd/mxcli/docker/ensuredb.go
  - cmd/mxcli/testmain_test.go
  - mdl-examples/doctype-tests/
  - mdl/executor/doctype_version_gating_test.go
  - modelsdk/canon/versionshape.go
---

> **Do not duplicate**: each platform call's correct form lives in the code and
> the findings; the CI matrix is in `.github/workflows/`. This page is about the
> defects whose *only* habitat is a machine no automated run inhabits.

## What this is

A sizeable share of `cmd/mxcli` findings cannot fail in CI — not because they are
timing-dependent, but because the environment that exhibits them is not one any
run provides. An ordinary Windows user account without Developer Mode. A
non-root devcontainer. A machine whose newest cached mxbuild is a version that
dropped a tool. A Mendix version below the one the integration matrix pins.

The pattern is worth naming because the usual instinct — "add a test" — does not
reach it, and the usual reassurance — "Windows CI is green" — is actively
misleading. A Windows *runner* often has the privilege or the toolchain that the
user lacks, so it passes for the same reason the user fails.

## How it fits

**Three environments, not two.** For the symlink failure the states were: Linux
(always works), Windows with Developer Mode or an elevated token (works), and
Windows as an ordinary user (`CreateSymbolicLink` needs
`SeCreateSymbolicLinkPrivilege`, so it fails). CI sat in the first two. The
repository's own test for the function had been failing on such machines for a
while, unseen — which is the recurring shape: **the test already existed and the
machine that runs it did not.** When a package has a family of these, the
question is which environment the whole family needs, not what each test asserts.

**"Can I open it" is not "is it running."** Two separate copies of the
`os.FindProcess(pid)` + `Signal(0)` idiom were written for liveness, and Go
supports no signal but `Kill` on Windows — so every pid read *dead*, and
`test --attach` reported an app that was alive as gone. The inverse also shipped:
on Windows an exited process object stays openable until the last handle closes,
so `run stop` called a dead run alive and exited 1. The correct form
(`OpenProcess(SYNCHRONIZE)` + `WaitForSingleObject(h, 0)`) **already existed in
the tree** from an earlier incident and had never been moved over. The remedy
that holds is one place for the question (`internal/procalive`) rather than a
third correct copy — and when porting a liveness fix forward, grep for every pid
helper, not the one the report named.

**Features can be unreachable rather than broken.** `run --local --watch` had
never worked on Mendix 10.24 or 11.6: the bundler-status parser understood only
the modern protocol and the older versions print a plain line. Nothing exercised
the watcher below 11.12 until an integration test reached the nightly matrix —
and it got there partly by accident, because the version resolver substitutes any
cached mxbuild for the test's requested one. "No report in two years" was not
evidence that the path worked; it was evidence that nobody on those versions used
it.

**A whole-file `@version` gate is a coverage hole, not a skip.** The doctype
gate runs each script on every matrix version, but a script gated `11.0+` as a
whole writes nothing on the 10.24 leg. That gate is usually set because of one
statement, so it also hides every other construct in the file. It has hidden a
load failure twice. Consumed REST clients wrote a type Mendix 10 lacks, until
`06c-…-mendix10` was split out. Every workflow written below 11.3 was broken
(#1754), until `24b-…-mendix10` was split out. The first run of the 10.0+ script
also found a third gap on its first try: 10.24's property set. The matrix has a
second hole of the same kind: its lowest 11.x is 11.6.8, past both workflow
boundaries (11.1 and 11.3). The full workflow script still fails on 11.0 and 11.1
for reasons unrelated to #1754, and nothing runs there. When a script needs a
gate, split out what the older versions can hold rather than gating the file.

**What lives on those versions is mostly one fact in an older shape.** The
workflow's implicit start and end, and a 9.24 page's design properties, data
view editability, data view parameter and placeholder (#1209), are the same
facts the current shape holds, stored differently. The property floor sees
keys, not facts. It refuses the newer key where the type declares it, and misses
it entirely where an abstract supertype does, because the generated tables carry
no inheritance. So no older-version test noticed, and every reader and writer
knew one shape. The remedy that covers every writer is to lower at the one hook
every unit write passes (`canon.LowerToProjectShape`, before the floor), and to
read both shapes. A starter app is the cheapest source of the older shape's real
bytes: `mxcli new --version 9.24.30.51827` authored by Studio Pro.

A refusal is not always an older shape, though. In the next 9.24 run (#1163), one
of five refusals was the floor's own data being wrong: an aggregate's function had
been given the row of the fold `reduce` added in 10.0. One was a document type the
version does not have: a published OData service before 10.4. That needs a gate,
because lowering its property would have produced a project that does not load.
For one more, the older shape was "nothing stored": Studio Pro 9 infers a flow
argument that is the enclosing object. Three things settled each one: the SDK's
version data, Studio Pro-authored bytes (a Marketplace `.mpk` holds a whole
`project.mpr` of its Mendix version), and `mx convert -p` on the older version
run against a floor-off build. Convert strips undeclared keys and empties values
it does not understand, so compare values as well as key sets.

**mxcli sometimes generates the environment it then fails in.** The non-root
devcontainer that could not provision PostgreSQL was scaffolded by `mxcli new` —
the same code emits that base image, installs postgresql, and runs as `vscode`.
That reframes the fix: it is not user misconfiguration, and putting the repair in
code rather than in the template also fixes every project already scaffolded.

**A helper that validates a whole toolchain breaks callers that needed one piece
of it.** Finding the Node binary went through a resolver that also requires the
rollup runner; when mxbuild 11.15 dropped the runner, a caller that never needed
it stopped working. Found by a smoke test on a machine whose newest mxbuild was
11.15 — not by unit tests, which put `node` on `PATH`.

**The Linux mirror: per-machine state, not timing.** A whole family of
run-lifecycle tests failed together in one CI run and passed in the next. That
shape — several at once, then none — points at machine state rather than at a
race inside the test: here the fractional second of the VM's boot time, which a
process start-time comparison rebuilt from whole-second `btime`. The fix makes
the comparison a pure function and pins the worst case, so the test fails
deterministically against the old code instead of occasionally.

**Guard the property, not the budget.** A flake that asserted elapsed wall-clock
time as a multiple of a poll interval was really guarding a **poll count** — "a
quiet source costs one extra poll". Widening the budget only moves the
threshold; injecting the timer makes the real property countable. The diagnosis
shortcut is worth keeping: the same workflow ran twice on the same tree, once
green, which rules out the tree and leaves the machine.

**Tests inherit the developer's environment unless something takes it away.**
In-process command tests appended to the developer's real `~/.mxcli/logs` and
could delete their older logs. A package-level `TestMain` that redirects the log
directory (unless already set) is the one place that covers every present and
future test; per-test setup only protects the tests whose author remembered. The
proof is a sentinel home: run the suite with `HOME` pointed somewhere else and
assert nothing was written there.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each platform
  call, its correct form, and the environment that showed it
- [[local-loop-silence]] — the loop these failures mostly surface in
- [[cli-contract-defects]] — where the defect is the tool's contract rather than
  the platform underneath it
