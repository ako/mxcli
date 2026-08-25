# Mapping census

Two measurement tools for the import/export mapping gap. Neither is wired into
CI — they are run on demand, against real projects, to keep the denominator in
[`PROPOSAL_mapping_coverage.md`](../../docs/11-proposals/PROPOSAL_mapping_coverage.md)
honest as marketplace modules change shape.

## `census.py` — what real mappings use that MDL cannot express

Decodes every `ImportMappings$ImportMapping` and `ExportMappings$ExportMapping`
in one or more projects and classifies each against the filed issues.

```bash
scripts/mapping-census/census.py mx-test-projects/*.mpk
scripts/mapping-census/census.py --json app.mpr > census.json
```

Accepts `.mpr` (v1 or v2) and `.mpk` packages. Classification reads the **stored
document**, never `describe` output, so it stays independent of the DESCRIBE
defects it is used to measure (#260).

Baseline, 327 mappings across the eight demo apps in `mx-test-projects/`
(2026-08-24): **97 (30%) use only constructs MDL can express.** The largest
gaps are the array root (#248, 122 docs), the message-definition source (#263,
74) and custom object handling (#264, 56).

## `roundtrip.sh` — does DESCRIBE output parse?

```bash
scripts/mapping-census/roundtrip.sh app.mpr [bin/mxcli]
```

Describes every mapping and re-parses the result with `mxcli check`. Exits
non-zero if any mapping fails to parse or describes to nothing.

**A `PARSE_OK` row is not proof of fidelity.** The output can parse cleanly and
still rebuild a *different* mapping — 112 of 327 do exactly that (#260). Pair
this with `census.py`: a mapping that `census.py` reports as blocked and
`roundtrip.sh` reports as `PARSE_OK` is in the silent-loss set.

## `mprbson.py`

Minimal BSON reader shared by both. `units(mpr)` yields
`(unit_id, container_id, containment_name, document)` for either storage format
— v1 reads `Unit.Contents`, v2 reads the matching `mprcontents/**/*.mxunit`.
`kids(v)` drops the leading list marker from a Mendix typed array.

It raises on BSON types the Mendix codec never emits rather than skipping them,
so an unexpected document fails loudly.

## `extract-fixture.py` / `load-fixture.py` — the CI-sized counterpart

The census needs the demo `.mpk`s (2.5 GB, not in the repo). The regression
fixture does not: `extract-fixture.py` pulls real mapping documents out of a
project verbatim, together with the MDL that recreates everything they
reference, and `load-fixture.py` transplants them into any project.

```bash
scripts/mapping-census/extract-fixture.py --project src.mpr \
    --out mdl/executor/testdata/mapping-fixtures [--append] Module.Mapping ...

scripts/mapping-census/load-fixture.py --project App.mpr \
    --fixture mdl/executor/testdata/mapping-fixtures
```

This works because a mapping document names everything **by qualified name,
never by ID** — so a document dropped into a project declaring the same names is
intact. See
[`mdl/executor/testdata/mapping-fixtures/README.md`](../../mdl/executor/testdata/mapping-fixtures/README.md)
for what is in the committed fixture and the traps the loader handles (a unit's
`UnitID` is its document's `$ID`; microflow deps are reduced to stubs).
