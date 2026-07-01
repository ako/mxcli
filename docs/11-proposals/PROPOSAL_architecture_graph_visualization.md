---
title: Architecture Graph Visualization (communities, layers, god-nodes)
status: draft
---

# Proposal: Architecture Graph Visualization

**Status:** Draft
**Date:** 2026-07-01
**Issue:** TBD (file before implementation)
**Depends on:** `PROPOSAL_graph_analysis.md` (implemented — the data source)
**Relates to:** `PROPOSAL_vscode_visualizations.md` (general diagram preview — this
is the architecture-graph-specific concretion), `proposal_sprotty_visualization.md`
(alternative renderer PoC — **not** pursued here; we stay on ELK)

## Problem Statement

mxcli already *computes* the full project graph — Leiden communities
(de-facto bounded contexts), topological layers, and centrality/god-nodes — and
exposes it over SQL (`communities_data`, `graph_layers_data`,
`graph_centrality_data`, the `graph_god_nodes` view) and MDL (`SHOW COMMUNITIES`,
`SHOW COMMUNITY [MEMBERS] OF`). But the output is **tabular**. The topological
questions this analysis answers — *"which subsystems really exist?"*, *"is my
layering violated?"*, *"what's the change-risk hotspot?"* — are spatial questions
that a table can't answer. An architect inheriting an app needs to *see* the
graph, not read a list of community IDs.

The goal: render the existing graph-analysis output as an interactive graph in
the VS Code MDL extension, where **color encodes community**, **position encodes
topological layer**, and **size encodes centrality (god-nodes)** — with
click-to-source navigation.

### Why not Foam (decided)

Foam renders a markdown workspace of `[[wikilinks]]` as a force-directed graph
with zero extension code, and several of us already run it. It was evaluated and
rejected as the primary path because it is a **note-graph, not an
architecture-diagram tool**: edges are undirected and untyped, there is no
concept of a topological layer (no banding), nodes cannot be sized by centrality,
community can only be *approximated* via tag colors, and it requires generating a
folder of `.md` stub files that pollute the user's workspace. It cannot express
the three semantics our data uniquely provides. A Foam/markdown export was
considered as a cheap secondary surface and dropped from v1 scope to keep one
maintained surface.

## Current State (what we build on — not greenfield)

The VS Code extension is **already a graph renderer**, which is the decisive fact
for this proposal:

- `vscode-mdl/src/previewProvider.ts` — a `WebviewPanel` (`MdlPreviewProvider`)
  that renders **ELK.js** graphs and **Mermaid** diagrams.
- `vscode-mdl/src/preview/mxcliRunner.ts` — pipes
  `mxcli describe --format elk <type> <name>` (JSON) into the webview.
- `mxcli describe --format elk systemoverview SystemOverview` already emits a
  **module dependency graph** (`mdl/executor/cmd_module_overview.go`), with
  click-a-node → open MDL source already wired.

So the infrastructure (webview, ELK layout engine, extension→mxcli JSON pipe,
node→source navigation) exists. **The gap is a single new ELK view** that carries
the graph-analysis attributes, plus the webview styling that maps those
attributes to color/position/size. This is an extension of a shipped feature, not
a new subsystem.

## BSON Structure

**N/A.** This feature is read-only and touches no Mendix document serialization.
Its entire input is the already-populated catalog graph tables. No `.mpr` writes,
no `$Type` handling, no CE-error surface.

## Proposed Design

### 1. A new ELK export: `describe --format elk architecture`

Mirror the existing `systemoverview` path in `cmd/mxcli/cmd_describe.go`'s
`--format elk` dispatch, adding an `ARCHITECTURE` case that calls a new
`exec.ArchitectureELK(name)` (new file `mdl/executor/cmd_architecture_elk.go`,
modeled on `cmd_module_overview.go`).

It queries the graph-analysis tables and emits ELK JSON where **each node carries
the three analysis attributes** in addition to the usual id/label/type:

```jsonc
{
  "id": "Sales.CreateOrder",
  "labels": [{ "text": "CreateOrder" }],
  "assetType": "microflow",
  "module": "Sales",
  "community": 3,        // communities_data.CommunityId  -> fill color
  "layer": 2,            // graph_layers_data.Layer        -> layer band / partition
  "centrality": 0.087,   // graph_centrality_data (PageRank) -> node size
  "godRank": 1           // graph_god_nodes rank (nullable) -> highlight ring
}
```

Edges come from `CATALOG.REFS` (typed edge list), carrying `refKind` so the
webview can style edge types.

**Scoping (this matters — see Open Questions):** a full project graph is large
(the reference app had ~2,500 asset nodes / 5,581 edges). v1 defaults to a
**community-aggregated top view** — one super-node per community, edges =
inter-community coupling (`graph_module_coupling` aggregated by community) — with
drill-down into a single community/module:

```bash
# top-level: communities as super-nodes
mxcli describe -p app.mpr --format elk architecture SystemOverview

# drill-down: assets within one community or module
mxcli describe -p app.mpr --format elk architecture --scope community:3
mxcli describe -p app.mpr --format elk architecture --scope module:Sales
```

**Precondition guard:** the graph tables are populated by the graph-analysis pass
(`REFRESH CATALOG COMMUNITIES`), not a plain refresh. If `communities_data` is
empty, the command must fail with an actionable hint, not an empty graph:

```
Error: no graph-analysis data. Run:
  mxcli -p app.mpr -c "refresh catalog communities"
```

### 2. Webview rendering (the three semantics)

Extend the ELK webview (new `vscode-mdl/src/preview/architectureTemplate.ts`,
sibling to `elkTemplate.ts`) to map the node attributes:

| Semantic   | Encoding | Source |
|------------|----------|--------|
| Community  | Categorical node fill color (stable palette keyed by `CommunityId`); optionally ELK compound/parent container per community for visual clustering | `communities_data` |
| Layers     | Vertical position via ELK `layered` algorithm with node partitioning by `layer` → horizontal bands, layer 0 at top | `graph_layers_data` |
| God-nodes  | Node size scaled by `centrality`; top-N get a highlight ring | `graph_centrality_data`, `graph_god_nodes` |

Reuse the existing click-node → `openElement` navigation. A small legend maps
color→community and documents the layer axis.

### 3. Command + menu wiring

Add `mendix.showArchitectureGraph` to `vscode-mdl/package.json` (command palette
+ tree-view context menu at project/module level), calling `generateElk(...,
'architecture', ...)`.

## Implementation Plan

Order: Go export first (testable via CLI JSON), then webview styling, then
command wiring.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/executor/cmd_architecture_elk.go` | **New.** `ArchitectureELK(name, scope)` — query graph tables, emit ELK JSON with community/layer/centrality/godRank per node; edges from `refs`. Empty-data guard. |
| `cmd/mxcli/cmd_describe.go` | Add `ARCHITECTURE` case to the `--format elk` dispatch (mirrors `SYSTEMOVERVIEW`); add `--scope` flag; extend the `elk` help/type list. |
| `mdl/executor/cmd_module_overview.go` | Reference only — reuse its edge-building/aggregation helpers where shared. |
| `vscode-mdl/src/preview/architectureTemplate.ts` | **New.** Webview HTML/JS: ELK layered layout with layer partitioning, community palette, centrality sizing, legend. |
| `vscode-mdl/src/preview/mxcliRunner.ts` | `generateElk` already takes `elementType` — pass `'architecture'`; thread `--scope`. |
| `vscode-mdl/src/previewProvider.ts` | Handle `'architecture'` type → `architectureTemplate`; wire drill-down messages (click community → re-fetch with `--scope community:N`). |
| `vscode-mdl/package.json` | New `mendix.showArchitectureGraph` command + menu contributions. |
| `cmd/mxcli/cmd_describe.go` help / `cmd/mxcli/syntax/*` | Document the new elk type. |

## Version Compatibility

**N/A for Mendix version-gating** — reads catalog data derived from any project.
The only precondition is that `refresh catalog communities` has been run (graph
pass available since `PROPOSAL_graph_analysis.md` landed).

## Test Plan

- **Go unit test** (`mdl/executor/cmd_architecture_elk_test.go`): seed a small
  catalog with known `communities_data`/`graph_layers_data`/`graph_centrality_data`
  rows, assert the emitted ELK JSON has correct per-node attributes, edges, and
  the empty-data guard error when tables are empty.
- **CLI smoke**: `mxcli describe --format elk architecture SystemOverview` on a
  fixture project produces valid ELK JSON (schema-check the node attributes).
- **Extension**: manual — open the architecture graph on a real project, verify
  community colors are stable, layers band correctly, god-nodes are visibly
  larger, click navigates to source, drill-down re-fetches a scoped view.
- No `mdl-examples/doctype-tests/` entry (no new MDL statement); no Studio Pro
  validation (read-only).

## Open Questions

1. **Default scope / scale.** Is community-aggregated-top-view + drill-down the
   right v1 default, or do users want a filtered full-asset graph (e.g. top-N
   god-nodes + their 1-hop neighborhood)? ELK layered on ~2,500 nodes in a
   webview may be too slow to be the default.
2. **Community as container vs. color-only.** ELK compound nodes (community =
   parent box) give stronger visual clustering but complicate layer banding
   (a node's layer and its community box may conflict). Color-only is simpler;
   start there?
3. **Layer axis direction.** Layer 0 = "depends on nothing" (leaves) vs. layer 0
   = entry points. Pick the convention that reads as "arrows point downward to
   dependencies" and document it in the legend.
4. **New MDL surface?** Should there also be a headless `SHOW ARCHITECTURE GRAPH`
   / export command for non-VS-Code users (CI artifact, docs), or is the webview
   the only consumer for v1? (Leaning: CLI JSON export exists regardless as the
   webview's data source, so a documented `describe --format elk architecture` is
   effectively that surface.)
