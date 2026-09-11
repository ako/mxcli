# Bug Report: what a microflow rewrite loses — a property-by-property audit

## Summary

`describe microflow` → `exec` is the documented copy operation, and it does not
round-trip. Audited across **342 microflows in 4 projects** (TestApp, CapTrack,
RestLab, Ledger; Mendix 11.14.0), **two** properties change with no error from
`mxcli check` or from mxbuild, and one of them is a security setting:

| property | class | affected | effect |
|---|---|---|---|
| `ApplyEntityAccess` | **writer hardcodes `false`** | 16 / 342 | a microflow that ran **under entity access** starts running without it |
| `ShowMessageAction.Blocking` | **DESCRIBE cannot spell it** | 16 microflows | a **blocking** message box becomes non-blocking |

Everything else that differs is layout or an empty-vs-absent artefact — listed in
full below, because "we checked and the rest is fine" is worth nothing unless the
checking is shown.

## Impact

`ApplyEntityAccess` is the serious one. Mendix's "apply entity access" makes a
microflow run under the current user's entity access rules rather than with full
access. Turning it off **widens** what the microflow can read and write, and
nothing downstream reports it: the model is valid, the app builds, and the
behaviour only differs for a user whose role should have been constraining them.

The path that triggers it is the one the docs recommend. CLAUDE.md names
`describe` → rename → `exec` as *the* copy operation (there is deliberately no
`COPY DOCUMENT` verb), and `CREATE OR REPLACE MICROFLOW` rebuilds from the
statement, so anything the statement does not restate is gone.

## Reproduction

```bash
cp -r <project> /tmp/audit && cd /tmp/audit
mxcli -p App.mpr -c "describe microflow Administration.SaveNewAccount" > rt.mdl
MXCLI_ALWAYS_WRITE=1 mxcli exec rt.mdl -p App.mpr

mxcli bson dump -p App.mpr --type microflow --object Administration.SaveNewAccount \
  | grep -A1 ApplyEntityAccess
```

`Administration.SaveNewAccount` ships with every blank Mendix app, stores
`ApplyEntityAccess: true`, and comes back `false`. `MXCLI_ALWAYS_WRITE=1` is only
there to defeat idempotent-write elision so the write definitely lands.

## Root cause

Two different causes wearing one symptom, which is why they need two fixes:

**1. `ApplyEntityAccess` — hardcoded in both writers.**

```go
// mdl/backend/modelsdk/microflow_write.go:208
out.SetApplyEntityAccess(false)

// sdk/mpr/writer_microflow.go:96
{Key: "ApplyEntityAccess", Value: false},
```

Not an unknown property: `microflows.Rule` carries it correctly on both engines
(`rule_write.go:104`, `parser_rule.go:52`). A **rule** keeps its setting and a
**microflow** does not, in the same codebase. `microflows.Microflow` has no field
for it at all, so the read side drops it before the writer is reached.

**2. `ShowMessageAction.Blocking` — a DESCRIBE gap, not a writer gap.**

`Blocking` is carried correctly through the model on both engines
(`microflow_read_actions.go:263`, `microflow_write.go:794`,
`writer_microflow_actions.go:511`). The loss is in the middle: MDL has no
`blocking` keyword, so `describe` emits

```mdl
show message 'The password has been updated.' type Information;
```

and the re-parse sets `Blocking: false`. Anything rewriting the microflow from
**stored BSON** keeps it; only the round trip through MDL text loses it.

## Method, and what it is worth

For every microflow in each project: dump the stored BSON, `describe` it, `exec`
that with `MXCLI_ALWAYS_WRITE=1`, dump again, and diff with element `$ID`s
normalised away and list order ignored (so reordering does not masquerade as a
value change). 341 of 342 were confirmed rewritten (`Replaced microflow` in the
exec log); the one exception is recorded in `exec_failures.txt`.

**The control is `StableId`: preserved in 342 / 342.** ADR-0008 says it is
carried rather than re-minted, and a method that could not tell preservation from
loss would have reported it as churn. It did not.

Two honest limitations:

- **All 16 microflows with `ApplyEntityAccess: true` are the same 4
  Administration microflows**, present in all four projects because every blank
  app ships that Marketplace module. No *user-written* microflow in this corpus
  sets the flag. So "16 / 342" is not a base rate — what is established is that
  **every microflow observed to carry the flag lost it, 4 of 4 distinct
  documents**, which follows from the hardcode regardless of sample size.
- The corpus is four projects on one Mendix version. A property no document here
  sets cannot be cleared by this audit — see `ConcurrenyErrorMessage` below.

## Everything else that differs, and why it is not in the table

| what changes | count | why it is benign |
|---|---|---|
| `ConcurrenyErrorMessage` loses its `Texts$Translation` entry | 20 / 58 stored | **the `Text` is `""` in all 58** — an empty translation entry versus no entry. The writer does hardcode an empty `Texts$Text`, so a real message *would* be lost, but no document in this corpus has one, so nothing here shows it. Same for the hardcoded `ConcurrencyErrorMicroflow: ""` (all 342 already `""`). Both are inert unless `AllowConcurrentExecution` is false, which nothing here is |
| `BezierCurve` Origin/DestinationControlVector → `"0;0"` | 87 / 56 | connector curvature; layout only |
| `ActionActivity.Caption` `"Activity"` → `""` | 74 | **only where `AutoGenerateCaption: true`** — a user-set caption (`"Save password"`, auto=false) is preserved exactly |
| `Size`, `RelativeMiddlePoint`, `Origin/DestinationConnectionIndex` | 16–28 | layout only |
| `CaseValues` gains `Microflows$NoCase` | 37 | required from Mendix 10; a bare `CaseValues: [marker]` is CE0079/CE0773. This is mxcli **fixing** an older shape |
| `Attribute`, `XpathConstraint`, `LocalVariable`, `Documentation` `""` → absent | 21–30 | empty-vs-absent; no value lost. A non-empty value survives (`System.User.WebServiceUser` is kept while its `""` sibling vanishes) |
| `CloseFormAction.NumberOfPages` absent → `1` | 16 | mxcli adds a key Studio Pro omits; the value is the default |
| `Documentation` `\r\n` → `\n` | 1 | CRLF normalisation |

None of these changes behaviour. They do explain why a one-line edit produces a
**417-line diff** on a real microflow, which is its own reviewability problem and
worth a separate issue.

## Suggested fixes, in severity order

1. **`ApplyEntityAccess`** — add the field to `microflows.Microflow`, read it on
   both engines, and give MDL a way to say it. The rule path is the precedent to
   copy. Until the syntax exists, the safe interim is **guard-don't-drop**
   (ADR-0005): refuse to rewrite a microflow whose stored flag is `true`, rather
   than silently clearing it.
2. **`ShowMessageAction.Blocking`** — a `blocking` modifier on `show message`.
   The model already carries it end to end; only the grammar, visitor and
   describe formatter are missing.
3. **`ConcurrenyErrorMessage` / `ConcurrencyErrorMicroflow`** — carry both on the
   model rather than hardcoding. No loss is demonstrated here, so this is closing
   a hole rather than fixing a bug, and it needs a project that sets them before
   the shape can be pinned. (Note Mendix's own spelling: `ConcurrenyErrorMessage`
   is one `c` short of "Concurrency", unlike its sibling.)

The first two need MDL syntax or model plumbing; neither is a one-line change.

## Environment

- mxcli: branch `claude/mxcli-unit-test-perf-n7ggx8`, commit `7261dfdd`
- Mendix: 11.14.0 (mxbuild 11.14.0)
- Corpus: ako/TestApp (42), CapTrack (138), RestLab (60), Ledger (102)
- Both engines: the two writers hardcode the same values, so `MXCLI_ENGINE` makes
  no difference to any finding here
