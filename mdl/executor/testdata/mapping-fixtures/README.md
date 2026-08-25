# Mapping round-trip fixture

Eleven **real Studio Pro mapping documents**, byte-for-byte as they were stored,
plus the MDL that recreates everything they reference. This is the CI-sized
counterpart to the demo-app census — the regression fixture for
[#260](https://github.com/ako/mxcli/issues/260).

## Why the documents are transplanted rather than authored

The test needs mappings mxcli **cannot author** — that is the bug. So the
documents come from real projects instead, and the transplant is sound for one
reason: a mapping document names entities, attributes, associations, structures
and microflows **by qualified name, never by ID**. Drop it into a project that
declares the same names and it is intact.

That is why `deps.mdl` keeps the original module and element names, and why the
documents must not be re-encoded.

## Contents

| file | what it is |
|---|---|
| `<Module>.<Name>.bson` | one unit, verbatim — a mapping, a JSON structure or a message definition |
| `deps.mdl` | enumerations, entities, associations, microflow stubs |
| `manifest.json` | `modules` to create, `documents` to transplant, and per mapping its type, file, source project and blocking constructs |

**JSON structures are transplanted, not regenerated.** A structure rebuilt from
its snippet is not the same document — mxcli names an array's item `DataItem`
where Studio Pro singularises to `Datum`, and gives the root `MinOccurs 0` where
Studio Pro writes 1 ([#272](https://github.com/ako/mxcli/issues/272)). A mapping
element clones those values from the structure it resolves against, so a fixture
that regenerated them would measure the structure builder instead of the mapping
describer. That is not hypothetical: it is what made `KrogerAPI.IM_Location`
appear lossy until the structure was transplanted.

## Coverage

One mapping per construct, plus a control that uses none.

| mapping | blocks on | issue |
|---|---|---|
| `KrogerAPI.IM_Location` | — **control**: a real Studio Pro document that must round-trip cleanly | — |
| `MendixSSO.AppRolesResponse` | array root, message-definition source | #248, #263 |
| `Email_Connector.IMM_EmailTemplateMapping` | array root, message definition, `find`+error, `create`+error | #248, #263, #261 |
| `KrogerAPI.IM_AccessToken` | custom object handling | #264 |
| `MxGenAIConnector.IM_Collection_RetrieveNearestNeighbors` | custom handling, nested root | #264, #267 |
| `MxGenAIConnector.IM_CohereEmbed_Response` | mapping input parameter, custom handling, attribute-less value | #265, #264 |
| `FeedbackModule.IMM_PostResponse` | converter microflow | #266 |
| `FeedbackModule.EXM_PostFeedback` | converter microflow (export) | #266 |
| `KrogerAPI.IM_ProductList` | primitive-array wrapper | #268 |
| `MxGenAIConnector.EM_CohereEmbed_Request` | primitive-array wrapper (export) | #268 |
| `OpenAI_API.IM_OpenAI` | nested schema root | #267 |

Not yet represented: `object element with no entity` (#262),
`find`+ignore and `allow override` (#261), the XML-schema shapes. Add them with
`extract-fixture.py --append`.

## Loading it

```bash
scripts/mapping-census/load-fixture.py \
    --project /path/to/App.mpr \
    --fixture mdl/executor/testdata/mapping-fixtures
```

The loader creates the modules one at a time (a module the base project already
has must not abort the file), runs `deps.mdl`, then inserts each document as a
unit **keyed on the document's own `$ID`** — a unit's `UnitID` *is* its
document's `$ID`, and a transplant that mints a fresh one is listed by `show` and
then not found by `describe`. A mapping whose name is already in the project is
skipped, not duplicated.

## The test

`TestMappingFixtureRoundTrip` (`mdl/executor/mapping_fixture_roundtrip_test.go`,
build tag `integration`) describes each mapping, re-executes that output — DESCRIBE
emits `create or modify <same name>`, so it rewrites the same document — and
compares before and after with `canon.Equal`, the canonical form the write path
already elides on (ADR-0008).

```bash
go test -tags integration ./mdl/executor/ -run TestMappingFixtureRoundTrip
```

`knownLossy` in that file records what is broken today with the issue that tracks
it, and the test **fails in both directions**: a mapping that starts round-tripping
must be struck off, and one that stops must be explained. Verified by removing an
entry and watching exactly that mapping fail.

Two positive controls, because "everything is lossy" and "the comparison never
looked" are otherwise indistinguishable:

- `RoundtripTest.IM_RoundtripControl`, authored by mxcli in the test — proves the
  harness can pass.
- `KrogerAPI.IM_Location`, a real Studio Pro document that uses nothing outside
  MDL's range — proves the fixture is loaded faithfully.

Current baseline: both controls pass, nine mappings are lossy. Four fail loudly
(DESCRIBE emits MDL the grammar rejects); five parse and rebuild a different
document. `scripts/mapping-census/roundtrip.sh` sees only the loud four — parsing
is not fidelity.


## Regenerating

```bash
scripts/mapping-census/extract-fixture.py \
    --project /path/to/source.mpr \
    --out mdl/executor/testdata/mapping-fixtures \
    [--append] Module.MappingName ...
```

Microflow dependencies are reduced to signature stubs, and access rules and
event handlers are stripped from entities — otherwise the fixture drags in the
transitive closure of a marketplace module. `System` is never authored.

Sources: CitrusGrove-InventoryApp, ComposableFactory, EnquiriesManagement — all
under `mx-test-projects/`, which is not in the repo.
