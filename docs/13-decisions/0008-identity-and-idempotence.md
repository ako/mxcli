# ADR-0008: Skip unchanged writes; never renumber element IDs in place

- **Status**: Accepted
- **Date**: 2026-08-10
- **Related**: [ADR-0005](0005-semantic-model-interface-currency.md) (semantic model as interface currency); `docs/11-proposals/PROPOSAL_marketplace_module_upgrade.md` §4–§6; `mxcli-formula1` FINDINGS §50; PR #125 (reverted)

## Context

Two goals arrived at the same underlying question and were nearly solved the same
wrong way.

1. **Version-control hygiene.** Re-running an MDL script against a project already
   in sync rewrites documents with different bytes every time. Three consecutive
   re-runs of one script gave three different tree hashes across 143 documents
   while the semantic content never moved. Studio Pro shows the project as
   modified; `git diff` cannot answer "did this script change anything"; two
   people running the same script commit different bytes; a `.mxunit` merge is not
   resolvable by hand.
2. **Marketplace module upgrade.** Updating an installed module is blocked, and
   `PROPOSAL_marketplace_modules.md` parked it as needing "a future ID-preserving
   merge".

Both look like *"make element IDs stable"*. That framing produced a change
(PR #125) that made projects **unopenable**, and it is wrong twice over.

### What is actually true about identity

Measured, not assumed (`docs/11-proposals/data/marketplace-upgrade/`):

- **The unit is the identity boundary.** Of 9,910 binary `$ID` pointers in a real
  project, **0 cross a unit boundary**. Cross-unit references are qualified-name
  strings. Nothing outside a unit can observe its IDs.
- **Studio Pro renumbers freely.** Its module update preserved **0 of 94**
  `$ID`s in `Administration` and **9 of 9** `GUID`s, and consumers — a module
  that both generalizes and retrieves `Administration.Account` — were untouched.
- **Within a unit, IDs are load-bearing.** Pointers reference them, and Mendix's
  loader resolves those references after the fact
  (`StreamingBsonUnitReader.ResolvePostponedProperties`).

So an `$ID` is *internal wiring*, with one exception: an entity attribute's ID is
consumed by the runtime's database synchronizer, where a fresh ID reads as
"attribute departed, new attribute added" and drops the column (fixed separately
in `06a9face`).

### Why the obvious fix failed

PR #125 carried stored `$ID`s onto a rebuilt document. It rewrote element IDs and
left the pointers that referenced them holding the discarded values:

```
KeyNotFoundException: The given key '553f4a64-…' was not present in the dictionary
  at StreamingBsonUnitReader.ResolvePostponedProperties()
```

Microflows and nanoflows corrupted; pages and navigation survived — because a
page's widget tree is containment and a microflow's is a graph. The surviving
document types were not evidence of safety, and a local test project containing
no microflow reported `mx check` clean.

The structural reason it could not have worked: pointers are **not** child
elements. `Microflows$SequenceFlow.OriginPointer` is a primitive property holding
an `element.ID` (`InitFromRaw` in `modelsdk/gen/microflows/types.go`), so a walk
over `ChildProperty`/`ChildListProperty` traverses the whole document and never
sees a reference. mxcli cannot currently enumerate reference-valued properties at
all.

### Why the write happens in the first place

`create or replace` does not reconcile. It rebuilds the document from the MDL and
overwrites unconditionally — no write path compares against what is stored — and
every sub-element in that rebuild is a new object whose ID comes from
`GenerateID()`, a random UUID. The output is a function of the script *and a
random source*, so byte stability was never reachable by any amount of care in
the builders.

## Decision

**1. Do not write a unit whose new content is semantically equal to the stored
content.** Compare before writing, at the single choke point, and skip when equal.

Equality is judged on a **canonical form**: the document with every element `$ID`
replaced by its index in a deterministic containment walk. The set of element IDs
comes from a containment walk, and any occurrence of one of those IDs anywhere in
the document is a reference by definition — so this needs no knowledge of *which
properties* are references, and works today.

The comparison is **biased toward writing**. If canonicalisation cannot decide,
it reports "different" and the write proceeds. A false *different* costs a
redundant write, which is today's behaviour; a false *equal* silently discards
the user's intent.

**2. Never renumber an element `$ID` inside a unit that is otherwise being
preserved.** A unit is rewritten wholesale or not at all. Any future operation
that wants to move IDs — an ID-preserving merge, seeded rebuilds, deterministic
derivation — is blocked until reference-valued properties are enumerable, and
must rewrite IDs and references in one atomic pass.

**3. Marketplace module upgrade is import-and-transplant-`GUID`, not a merge.**
Write the package's units wholesale and carry each element's `GUID` across by
name. Their internal pointers arrive consistent, nothing is renumbered in place,
and consumers reference by qualified name, so decision 2 is satisfied for free.

## Consequences

**Both goals are reachable without the dangerous machinery.** Neither
version-control hygiene nor module upgrade requires ID preservation, deterministic
derivation, or reference remapping. Those remain available for a later, narrower
purpose — a *smaller* diff when something genuinely changed — and are explicitly
not prerequisites.

**One policy has to change first, and it is sufficient.**
`microflow_write.go` registers `StableId` as a `FreshGUIDField`, minting a new GUID
on every microflow write. While that holds, a microflow always differs from itself
and decision 1 skips nothing for the document type that dominates a script-authored
app.

That policy is also wrong on its own terms — see
[What `StableId` is](#what-stableid-is) below. It is not an incidental GUID that
happens to churn; it is the one field on a microflow whose stated purpose is to
*not* change, and its value determines the identifier the browser uses to call
that microflow.

**Where this lives.** `modelsdk/canon` holds the whole policy: `Digest`/`Equal`
(canonical comparison), `CarryIdentity` (the stored value of an identity property
is carried onto the rebuilt document, keyed by `$Type`), and `Reconcile`, which
composes them into the one decision a storage layer asks for. Both engines call
it at their single write choke point — `modelsdk/mpr/writer_core.go`
(`reconcileWithStored`, reached by `updateUnit` *and* `WriteTransaction.WriteUnit`,
the latter being how `codec.Store` writes) and `sdk/mpr/writer_units.go`
(`updateUnit`). Which engine ran is an `--engine` flag; it must not be visible in
a user's diff. `MXCLI_ALWAYS_WRITE=1` disables elision for bisecting, and
deliberately does *not* disable identity preservation: a forced write that
re-minted `StableId` would renumber the deployed model's operation ids, which is
a change to the app rather than a debugging aid.

Note the ordering inside `Reconcile`: identity is carried **before** the
comparison. That is what makes a re-run of an unchanged microflow compare equal at
all, and it is why canonicalisation does not mask `StableId` — a field that
reaches storage is content. Masking it in the comparison would skip the write
while leaving stored and intended values silently disagreeing.

Measured on `mxcli-sudoku` — 412 units, MPR v2, an idempotent 30-document script
set, two identical re-runs (~61s):

```
units                412
  identical          386
  volatile-only       26     all 26 are microflows
  real differences     0
```

**Zero real differences at scale**, and the split falls exactly along document
type rather than arbitrarily. `StableId` is registered on `Microflows$Microflow`
only, and the result mirrors that precisely: of the 30 documents the scripts
touch, the 26 microflows are volatile-only while the nanoflow, both pages and the
navigation document are *identical*. So freezing `StableId` is not merely
necessary for decision 1 — on this evidence it is **sufficient**.

For one microflow across the two runs:

```
run a: canon=ec4c23b68471026a  masked=6bc0ff14154da8de
run b: canon=46db2776c892421c  masked=6bc0ff14154da8de
StableId run 1: 26LgMMVoiE+ex1dmtRbDHQ==
StableId run 2: KwVkeqX3gU+s/ncDaO/7qw==
```

Canon moves, masked does not, and the field itself is what moved.

**The zero is a measurement, not a blind spot.** The same probe was run against
the withdrawn `PreserveIDs` binary on the same project and reported **10 real
differences** — the 9 microflows and the 1 nanoflow in that script set, with the
page unchanged. That independently reproduces the microflows-corrupt /
pages-survive split, and demonstrates the probe can report non-zero on exactly
the failure this ADR exists to prevent.

**Confirmed after implementation.** The prediction above — that freezing
`StableId` is sufficient, not merely necessary — was checked against
`mxcli-sudoku` once decision 1 shipped, and the project re-runs clean. That closes
the loop the pre-implementation measurement opened: the 26 volatile-only units
were volatile *only* because of `StableId`, and carrying it from storage moves
them into the skipped set.

**Breadth, from a second project.** `mxcli-formula1` — a multi-app solution using
published OData services, external entities and the external database connector —
also came back green. Its value is not another zero; it is *coverage*. Every
measurement behind this ADR until then came from projects whose documents were
microflows, nanoflows, pages and navigation, and the open question was whether
elision holds for document types nobody had exercised.

It matters most for the invariant named under
[generalisation](#does-this-generalise-to-document-types-that-do-not-exist-yet):
elision assumes no binary pointer crosses a unit boundary. A multi-app solution
with external entities and OData contracts is exactly the shape where a
cross-document binary reference would be most likely to exist, and where its
absence has the most to say. It is corroboration rather than proof — a passing
project shows no *observed* violation, not that none can occur — but it is the
strongest evidence available short of enforcing the invariant in code.

Recorded as coverage rather than as an idempotence measurement: unlike the
sudoku run, it is not established here that the `MXCLI_ALWAYS_WRITE=1` control was
taken alongside it, and by this ADR's own standard a clean result without its
control is not an idempotence measurement.

**Verification has a standing shape.** `scripts/mprsnapshot -canon` emits per-unit
canonical digests; two runs plus a diff answers "how many units would be skipped"
without changing any write behaviour. Now that elision is live, the more direct
question — "did anything change?" — is answered by hashing the stored units
(or `git status`) across a re-run.

Any claim about idempotence should cite a measurement on a project that contains
microflows, **and the control run with `MXCLI_ALWAYS_WRITE=1`**. A clean diff on
its own is not evidence: the first end-to-end measurement taken during
implementation reported zero changed units purely because the script aborted on
its second run at `enumeration already exists` and wrote nothing — and the control
reported zero too, which is what exposed it.

**A green `mx check` is not evidence about the write path** unless the fixture
contains the constructs at risk. This is the direct lesson of PR #125 and belongs
in any future attempt's test plan.

### What `StableId` is

Freezing it was the last blocker, so "what breaks if we stop regenerating it?"
had to be answered before decision 1 could be implemented. mxcli's own reference
material is silent — the field appears nowhere in
the metamodel reflection data, `reference/mendixmodelsdk/`, or
`generated/metamodel/`. That silence means only that mxcli never had a
description of it, so the evidence below comes from Mendix's own binaries and
from a build's output.

**Declaration** (`Mendix.Modeler.Microflows.dll`, via `monodis`):

```
.property instance System.Guid StableId ()
  ModelPropertyAttribute("StableId", RetentionType.DesignTime)
    SdkName      = "stableId"
    IsIdentifier = true
```

Mendix's own metamodel marks it `IsIdentifier`. `RetentionType.DesignTime` means
it is kept in the model but not exported as a property — matching a strict scan
of all **624 runtime jars**, which finds the name zero times. (A loose substring
scan appears to find three; they are `newPersistableIds`.)

**Where it came from.** `MicroflowStableIdConversion` is an `IOneTimeConversion`
that back-fills every microflow:

```csharp
foreach (var mf in project.GetAllStorageObjects<Microflow>())
    mf.StableId = GuidUtil.Create(mf.ID, "stableId");
```

Seeded *once* from the storage `$ID`, then retained independently of it. The name
is the specification: it is the identity that survives the `$ID` renumbering
Studio Pro does freely (0 of 94 preserved, §"What is actually true about
identity").

**Studio Pro transplants it on module update.**
`PackageUtils.RescueStableIDs` — sitting directly beside
`PackageUtils.RescueDataStorageGuids` — matches old to new microflow **by
`Name`** and copies the value across. Studio Pro's own marketplace path is
replace-wholesale-then-transplant-durable-identity, which is decision 3 arrived
at independently by the vendor.

**It is load-bearing in the build.** `RuntimeOperationRegistry` in
`Mendix.Modeler.WebUI.Export` keys every client-callable microflow operation on
it:

```
GetOrCreateOperation(stableID: microflow.StableId.ToString(), namespaceId: project.ID)
  -> StringUtil.CreateShortIdentifier(namespaceId, stableID)
  -> GuidUtil.Create(namespaceId, stableID, version: 5)      // RFC 4122 v5, SHA-1
  -> Convert.ToBase64String(guid.ToByteArray()).Replace("=", "")
```

Reproduced against a real build (Mendix 11.10, `mxbuild --target=deploy`): all
**10 of 10** `callMicroflow` entries in `deployment/model/operations.json` are
regenerated exactly by `base64(uuid5(projectId, StableId).bytes_le)`, e.g.

```
BERAieo94VWMPTH77gmtrA  Administration.ShowMyPasswordForm
L6RSxgkEsVmFIgNwW/XX7Q  Administration.NewAccount
```

`com.mendix.webui.jar` reads that file. So the *property* is design-time, but its
*value* reaches the runtime as the operation identifier the browser calls — and
`operations.json` attaches `allowedUserRoleSets` per operation id.

**Consequence.** A fresh `StableId` renames every client-callable microflow
operation in the deployed model. Since `operationId` is a pure function of
`(projectId, StableId)` and mxcli demonstrably moves `StableId` on every write,
that rename follows with no gap — a build after an mxcli write emits different
operation ids for unchanged microflows. Within a single build client and server
still agree, so this is not a "the app is broken" claim and has not been measured
as one; it is a claim about the artifact, which is exactly what decision 1 is
about.

**Scope.** Only `Microflows$Microflow` carries the field: in the 369-unit fixture,
16 of 16 microflows have it and all 13 nanoflows do not. That independently
explains the sudoku split — 26 microflows volatile-only, the nanoflow identical —
and is why freezing this one field is sufficient rather than merely necessary.

**How this was established**, for the next field that needs the same treatment:

| Question | Method |
|---|---|
| Does it reach the runtime? | strict-boundary `strings` scan over every runtime jar — substring matching gives false positives |
| Who declares it, with what semantics? | `monodis` the modeler assembly; read the `ModelPropertyAttribute` blob |
| When did it appear, seeded how? | look for an `IOneTimeConversion` named after it |
| Does Studio Pro preserve it? | search the package/import assembly for a get→set pair |
| Does its value escape the model? | build with `mxbuild --target=deploy`, then reproduce the derivation against `deployment/` |

### Does this generalise to document types that do not exist yet?

The two halves answer differently, and the difference is worth keeping straight.

**Elision generalises by construction.** It operates on raw BSON and knows nothing
about document types: it collects `$ID`s by containment walk, normalises them, and
compares. A document type added tomorrow is covered the day it is added, with no
registration and no table. It also fails safe — a document it cannot parse (a
custom blob, a future non-BSON unit) reports an error, and every error path
writes.

It rests on one invariant, which is measured rather than enforced: **no binary
pointer crosses a unit boundary** (0 of 9,910 in a real project; cross-unit
references are qualified-name strings). If a future document type introduced one,
elision could produce a dangling reference from the *opposite* direction to
PR #125: unit B is elided and keeps its stored IDs, while unit A is written
holding pointers to the fresh IDs B's discarded rebuild had minted. Note the
shape — the danger is not in the unit being skipped, but in another unit that was
*not* skipped having already been built against the skipped unit's would-be IDs.
Nothing in the code detects this, and it is invisible until a project fails to
load. A new document type that references another unit by `$ID` rather than by
qualified name invalidates the argument in this ADR and must be measured, not
assumed.

**Identity preservation does not generalise.** `identityFields` is hand-written
and cannot currently be generated: Mendix's `IsIdentifier` flag lives in the
modeler assemblies, and the reflection data `generated/metamodel` is built from
does not carry it. A new document type with an identity property needs a row, and
nothing about adding the type will remind you. The failure is silent — no error,
no failing test, just churn quietly returning for that type.

`TestFreshGUIDFieldsHaveAnIdentityDecision` closes the most likely path to that.
A property registered as a codec `FreshGUIDField` is minted anew on every write,
so by construction it makes a document differ from itself; the guard fails unless
each one is recorded as either identity (carried) or deliberate churn (waived with
a reason). It cannot catch an identity property the encoder does *not* mint fresh.
For that there is no substitute for establishing the property's status the way
`StableId` was established, before the document type ships.

### Known limit of the evidence

The measurements above cover microflows, nanoflows, pages and navigation — the
*internal wiring* class, where this ADR's argument is strongest. They do **not**
cover domain-model documents, because the probe needs scripts that apply cleanly
twice and the available domain scripts do not (`create module` and
`alter entity … add attribute` fail on a second run).

That leaves untested precisely the exception this ADR names: an entity attribute's
`$ID`, which the database synchronizer reads as identity. Two things are already
true there and should be checked rather than assumed when it is measured:

- `create or modify entity` **already reuses each retained attribute's existing ID
  by name** (findings #13, `06a9face`) — so the identity concern is addressed on
  that path, and a probe run over it would be confirming a fix rather than
  discovering one.
- `create or modify entity` still **rebuilds the entity from the statement alone**,
  so any attribute the statement omits is deleted. It is no longer silent — a
  non-blocking warning lists what is dropped (findings #24) — but a script used to
  probe idempotence must list every attribute or it will measure a real change it
  caused itself.

Closing this gap needs an idempotent domain-model script. Until then, decision 1's
safety argument for attributes rests on the identity fix above rather than on a
measurement.

## Alternatives considered

**Preserve stored IDs onto the rebuilt tree** (PR #125). Rejected: needs reference
remapping mxcli cannot express, and measured at ~1% churn reduction where it was
safe, because unnamed sub-elements dominate a page and cannot be matched by name.

**Derive IDs deterministically** from a stable seed, making the rebuild a pure
function. Attractive — pointers computed during the build are consistent by
construction, and `GenerateDeterministicID` already exists for System entities.
Rejected *as the first step*: unnamed elements have no stable seed but their
structural path, which changes exactly when a list shifts; adoption rewrites every
derived ID once; and attributes must still be seeded from storage. Decision 1
delivers the same user-visible outcome with none of that.

**Compare and skip on raw bytes.** Rejected: the rebuilt bytes always differ,
because the IDs are freshly random. Byte comparison would skip nothing.

**An element-level merge for marketplace upgrade.** Rejected on evidence: Studio
Pro does not merge, and §4 shows it does not need to. A merge would also require
in-place renumbering, which decision 2 forbids.
