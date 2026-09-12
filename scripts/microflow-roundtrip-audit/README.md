# Microflow round-trip audit

Measures what `describe microflow` → `exec` changes about a microflow — the
documented copy operation, which does not round-trip. Not wired into CI: it
rewrites every microflow in a project and takes minutes, so it is run on demand
against real projects.

Its first run produced
[`microflow-rewrite-property-audit.md`](../../docs/12-bug-reports/microflow-rewrite-property-audit.md);
re-run it after a fix to show the property has stopped moving.

## Use

```bash
make build
scripts/microflow-roundtrip-audit/roundtrip.sh <project-dir> <App.mpr> <out-dir>
python3 scripts/microflow-roundtrip-audit/classify.py <out-dir>
```

`roundtrip.sh` copies the project first — it never touches the original. For each
microflow it dumps the stored BSON, describes it, re-executes that MDL with
`MXCLI_ALWAYS_WRITE=1` (so idempotent-write elision cannot hide a change), and
dumps again. `<out-dir>/exec_failures.txt` lists any describe output that did not
re-execute, which is a finding in its own right.

`classify.py` reports what moved, split into the microflow's top-level
properties and its flow graph (`ObjectCollection` / `Flows`) — different concerns,
and the graph is where the layout noise lives.

## Reading the output

Three distinctions decide whether a difference matters, and all three have
already caught something:

- **Layout versus behaviour.** Roughly 90% of a real microflow's diff is bezier
  control vectors, sizes, positions and connection indices. They bury the lines
  that matter; filter them before drawing conclusions.
- **Empty versus absent.** `""` → key absent is not a loss. `ConcurrenyErrorMessage`
  looked like 58 dropped translations and every one held `Text: ""`.
- **Hardcoded-in-the-writer versus unspellable-in-DESCRIBE.** Identical in the
  diff, completely different fixes — model plumbing versus grammar. `Blocking` is
  carried perfectly by both engines and still lost, because DESCRIBE cannot say it.

**Keep a preservation control.** `classify.py` normalises element `$ID`s away so
that re-minted ids do not swamp the output; `StableId` is the check that the
normalisation has not gone too far, since ADR-0008 says it is carried and the
audit should therefore report it unchanged (342 / 342 on the first run). A run
that shows `StableId` moving is measuring its own blind spot, not a bug.
