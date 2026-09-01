---
title: Two Engines, One Interface
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - mdl/backend/modelsdk/microflow.go
  - docs/13-decisions/0004-full-codec-engine.md
---

> **Do not duplicate**: the abstraction's rationale is canonical in ADR-0002 and
> ADR-0004 and framed in [[backend-abstraction]]; the per-field fixes live in the
> findings. This page describes what goes wrong while two implementations exist.

## What this is

The backend interface has two implementations — the legacy `sdk/mpr` writer and
the codec-based `modelsdk` engine — and roughly a quarter of the `mdl/backend`
findings are one of them doing something the other does not. The default is the
newer engine, so a gap in it is the behaviour most users get, while the tests and
habits formed against legacy still pass.

A gap on one engine is **invisible from inside that engine**. Everything is
self-consistent: the write stores what the read returns, the round trip is
stable, and the field that never existed is never missed.

## How it fits

**Two failure modes, and only one of them is honest.** A refusal —
`… not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy` —
costs the user a flag and tells the truth. The alternative is a placeholder:
`-- Empty action` from a describer that does not recognise an activity type, a
sort clause silently absent, an argument list quietly dropped. The placeholder is
worse than the refusal in every way, because `describe → edit → exec` then
deletes the construct and everything reports success.

**The worst instance was a read that under-reported access.** A restricted page
came back as "no roles" on the default engine, so `SHOW ACCESS` and the security
matrix both understated who could reach it. A missing field is a cosmetic bug
almost everywhere and a security-relevant one here.

**The check that finds these is the cross-engine matrix.** Write with engine A,
read with engine B, all four combinations. Every other test shape is
engine-local and therefore blind to exactly this class.

**Read against the keys the writer builds, not against gen's accessors.** The
generated bindings and the stored document disagree in places — one action's
result is bound as `VariableName` where the model stores `OutputVariableName` —
so an accessor-based reader silently returns nothing. Round-trip tests
(`toGen → encode → decode → fromGen`) catch it; a reader test over hand-written
BSON does not, because the hand-written BSON was written from the same wrong
assumption. These have to be Go tests rather than MDL ones: `create or modify`
rebuilds the document from MDL, so it cannot reproduce a read-back defect at all.

**Count the gap before fixing the instance.** Decoding every `$Type` through the
real reader turned one reported unsupported action into twenty-seven, one of
which was a shipped feature that could not be described back. Related: several
activity types live in **their own sub-metamodel** — `DatabaseConnector$…`, not
`Microflows$…` — so a probe or a grep under one prefix systematically
under-counts what is covered.

**Fixing one field and not its sibling is the characteristic mistake.** A reader
restored for `TableMappings` and not `Parameters`, a source type ported without
its key parts. Enumerate every child of the element rather than the one the
report named.

**Guard the write side with a round trip, not with `mx check`.** Several of these
produced models mxbuild accepts: a box size of zero renders every activity as a
one-pixel sliver in Studio Pro and validates at 0 errors, because `mx check` does
not look at geometry.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  fields, sub-metamodels and round-trip guards
- [[backend-abstraction]] — why the seam exists
- [[describe-round-trip-gaps]] — the same read-side failure, engine-independent
