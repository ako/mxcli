---
title: MCP BSON Benchmark — Correctness Oracle + Efficiency Measurement
status: draft
date: 2026-05-03
---

# Proposal: MCP BSON Benchmark — Correctness Oracle + Efficiency Measurement

**Status:** Draft  
**Date:** 2026-05-03  
**Author:** AI-assisted design

---

## Summary

Use the Studio Pro MCP server as a **dual-purpose tool**:

1. **BSON correctness oracle** — Studio Pro is the authoritative BSON serializer. Executing the same logical operations via MCP and via mxcli, then diffing the resulting mxunit files, reveals structural bugs in mxcli's BSON output that `mx check` does not catch.

2. **Empirical efficiency benchmark** — Running the doctype-test scripts through both paths measures the token and time cost claims in `MXCLI_STRATEGIC_POSITIONING.md` rather than arguing them analytically. The MDL path externalises all execution to a subprocess; the MCP path routes every schema, document read, and incremental update through Claude's context window. The difference should be quantifiable and significant.

---

## Motivation

### BSON correctness

`mx check` validates semantic correctness ("does this microflow make sense?") but not structural completeness. mxcli could:
- Omit optional fields that Studio Pro always writes
- Use wrong `$type` storage names that happen to parse but behave differently at runtime
- Write wrong default values that affect UI or runtime behaviour
- Produce subtly wrong association pointer structure (the ParentPointer/ChildPointer inversion is a documented example)

None of these are caught by `mx check`. They become visible only when Studio Pro opens the file and the developer sees unexpected behaviour — or when a comparison against Studio Pro's own output is performed.

### Empirical benchmark

The strategic positioning document (`docs/01-project/MXCLI_STRATEGIC_POSITIONING.md`) argues that MDL has a structural token advantage over MCP-style tool-call-by-tool-call editing. The argument is sound analytically, but "10–100× fewer tokens" is more persuasive as a measured result than as a back-of-envelope calculation.

The doctype-test scripts are an ideal corpus: they cover the full feature surface, are already written, and span domain model, microflows, pages, security, and workflows. Running each through both paths yields a reproducible dataset.

---

## Architecture

```
doctype-test script (e.g. 01-domain-model-examples.mdl)
            │
            ├──────────────────────────────────────────────────┐
            │ Path A: MDL                                      │ Path B: MCP
            ▼                                                  ▼
  mxcli exec script.mdl -p ProjectA.mpr          Claude reads MDL statement by statement
            │                                     → ped_get_schema (per type)
            │                                     → ped_find_document (idempotency check)
            │                                     → ped_create_document / ped_update_document
            │                                     → ped_check_errors
            ▼                                                  ▼
  mprcontents/**/*.mxunit (new files)         mprcontents/**/*.mxunit (new files in SP project)
            │                                                  │
            └─────────────────────┬────────────────────────────┘
                                  ▼
                        BSON diff (normalised)
                        + metrics report
```

Both paths start from a copy of the same blank template project (`mx-template-projects/Template1024App-main`). New documents appear as new mxunit files in `mprcontents/`. The comparison reads these files, matches them by document name and type, and diffs their BSON content field-by-field.

---

## BSON Comparison

### mxunit file layout

Each document in an MPR v2 project is a separate BSON file at:
```
mprcontents/<uuid[0:2]>/<uuid[2:4]>/<uuid>.mxunit
```

After executing each path, the new mxunit files are identified by comparing the file set against the baseline template. These are the documents created by that path.

### Matching documents across paths

Documents are matched by **qualified name + document type**, not by UUID. The document name is embedded in the BSON content (e.g., `name` field for microflows, entity name for domain model entries).

For domain models, the comparison is at the subdocument level (entity by entity, attribute by attribute), since the entire domain model lives in one mxunit file.

### Normalisation before diff

| Field class | Treatment |
|---|---|
| All UUID-typed fields (`$id`, element IDs, pointer references) | Replace with stable ordinal derived from structural path (e.g., `entity[0]/attribute[2]`) |
| Canvas position fields (`relativeMiddlePoint`, `x`, `y`, `width`, `height`) | Strip — legitimately differ |
| Array element ordering | Sort canonically by name field where present |
| Auto-generated captions / default display strings | Include — discrepancies here are real bugs |

Fields that remain after normalisation must match exactly. Any mismatch is a correctness finding.

### Output format

```json
{
  "script": "01-domain-model-examples.mdl",
  "document": "DmTest/DomainModel",
  "status": "DIFF",
  "findings": [
    {
      "path": "/entities/0/attributes/2/type",
      "mxcli": "String",
      "studio_pro": "String",
      "note": "match"
    },
    {
      "path": "/entities/0/defaultEntityAccessRights",
      "mxcli": null,
      "studio_pro": "ReadWrite",
      "note": "mxcli omits field Studio Pro always writes"
    }
  ]
}
```

---

## Efficiency Metrics

For each script / path combination, record:

| Metric | MDL path | MCP path |
|---|---|---|
| Wall time (s) | Time for `mxcli exec` to complete | Time from first tool call to `ped_check_errors` returning clean |
| Claude tool calls | 1 (`mxcli exec` bash call) | N (sum of all ped_* calls) |
| Tokens in (est.) | Script size + 1 tool invocation | Sum of all tool call inputs (schemas + document reads + payloads) |
| Tokens out (est.) | mxcli stdout (pass/fail summary) | Sum of all tool call responses |
| Errors encountered | 0 or exit code | ped_check_errors findings requiring fix iterations |

Token counts for the MCP path can be estimated from the raw JSON payload sizes of each tool call, or measured precisely if the benchmark is run via the Claude API with usage reporting enabled.

The MDL path token cost intentionally excludes script authoring time — the doctype-test scripts are pre-authored. This isolates execution cost and represents the steady-state case (an agent that has already written the script or is replaying it).

---

## Phased Scope

### Phase 1 — Domain model (tractable now)

Script: `01-domain-model-examples.mdl`

Covers: `CREATE MODULE`, `CREATE ENUMERATION`, `CREATE ENTITY` (all attribute types, constraints, indexes, generalization, documentation), `CREATE ASSOCIATION`.

MCP translation: mechanical. Each MDL statement maps to 1–2 MCP calls. No expression encoding or widget hierarchy required.

Expected findings: missing default field values (access rights, documentation defaults), possible `$type` field differences for enumeration subtypes.

### Phase 2 — Microflows

Script: `02-microflow-examples.mdl`

Covers: parameters, return types, activities (CreateObject, ChangeObject, Retrieve, Commit, microflow calls, show message, decisions, loops, etc.).

MCP translation: harder. Each activity requires schema fetch + correct ID references between sequenceflows and activities. Expression strings must be correctly encoded. Ordering within the flow matters.

Expected findings: sequenceflow connection points, expression field encoding differences, missing default activity properties.

### Phase 3 — Pages

Script: `03-page-examples.mdl`

Covers: layouts, widget hierarchies, pluggable widgets.

MCP translation: most complex. Widget trees are deeply nested and highly order-sensitive.

Expected findings: widget default property differences, pluggable widget type/object field completeness.

---

## MDL → MCP Translation Methodology

The MCP path does not use an automated translator. Claude reads the MDL script and executes the equivalent operations using MCP tools, following the same conventions it uses for any Studio Pro editing task. This is intentional:

- It tests the realistic MCP workflow (how an agent would actually use PED to achieve the same result)
- It captures the real token cost of interactive MCP editing, including schema fetching and error recovery
- It does not require building a separate translator that could short-circuit the measurement

The translation is logged as part of the benchmark output — the full sequence of MCP calls made to achieve the task is captured alongside the metrics.

---

## What the Benchmark Validates

If the BSON correctness diff is clean for a given script, it confirms mxcli produces Studio Pro-equivalent output for that feature area — a strong quality signal.

If the efficiency measurement shows, e.g., that `01-domain-model-examples.mdl` (creating ~20 entities + enumerations) takes 1 mxcli call vs. 60+ MCP tool calls with 40k+ tokens of schema and document payload, that is a concrete, citable data point for the strategic positioning argument — not an analytic estimate.

---

## What to Build

The benchmark is primarily a **workflow**, not a new mxcli command. The infrastructure needed:

| Component | Description | Location |
|---|---|---|
| Blank project copy script | Copy template, reset module state | `scripts/benchmark-setup.sh` |
| mxunit diff tool | Read two mxunit file sets, normalise UUIDs/positions, output JSON diff | `cmd/mxcli/cmd_bson_diff.go` (or standalone script) |
| Benchmark runner | Orchestrates both paths, collects metrics, writes report | `docs/14-eval/benchmark/` |
| Results store | JSON per script per run, for trend tracking | `benchmark-results/` (gitignored) |

The mxunit diff tool (`mxcli bson-diff`) is the only new mxcli command required. It takes two project directories and a document list, and outputs the normalised JSON diff. It reuses the existing BSON parser in `sdk/mpr/parser.go`.

---

## Directory Structure

```
docs/11-proposals/
└── PROPOSAL_mcp_bson_benchmark.md        (this file)

docs/14-eval/benchmark/
├── README.md                             (how to run)
├── run-benchmark.sh                      (orchestration script)
└── results/                             (gitignored)
    └── 2026-05-03/
        ├── 01-domain-model/
        │   ├── mdl-metrics.json
        │   ├── mcp-metrics.json
        │   └── bson-diff.json
        └── summary.md

cmd/mxcli/
└── cmd_bson_diff.go                     (new: mxcli bson-diff command)
```

---

## Resolved Questions

1. **Studio Pro project path** — the mx-demo-1 MCP server project is at `./mx-test-projects/test6-app/`, which is accessible on the local filesystem. It is v2 format with 373 mxunit files. Both sides of the comparison can be done as direct BSON file reads — no `ped_read_document` JSON proxy needed.

## Open Questions

2. **Project reset between runs** — the MCP path modifies the live Studio Pro project (`test6-app`). Between benchmark runs, do we reset by reverting the mprcontents state with `git checkout -- mx-test-projects/test6-app/mprcontents`, or does Studio Pro need to close and reopen a clean copy?

3. **Token measurement precision** — for Phase 1, estimating from payload JSON sizes is sufficient. For Phase 2+, running via the Claude API with `usage` reporting gives exact counts. Worth deciding whether Phase 1 uses the estimation approach or sets up the API harness from the start.
