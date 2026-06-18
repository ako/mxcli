# Proposal: Graph analysis — architecture report views + Leiden communities

**Status:** Draft
**Date:** 2026-06-18

## Problem Statement

A large Mendix app is a graph (entities, microflows, pages, associations, calls),
but mxcli today only answers **node-level** questions ("tell me about X",
`show callers/callees/impact/context of X`). It can't answer the **topological**
questions that matter when you inherit an app and must decide *where to
intervene*:

- Which assets are **god nodes** — change-risk hotspots everything depends on?
- Which modules are **architecturally entangled** ("surprise edges") despite being
  nominally separate?
- What are the app's **de-facto bounded contexts** — clusters of assets that
  belong together regardless of the module they live in?
- What's **dead** (no inbound references)?

These are precisely the analyses [Graphify](https://graphify.net/) performs for
general code (god nodes + Leiden community detection over a Tree-sitter graph).
mxcli has a decisive advantage: the `CATALOG.REFS` table is already a **typed**
edge list — richer than any AST parser can produce — so most of this is a
rendering problem, not an extraction problem.

### Empirical validation

Run against `Evora-FactoryManagement` (771 entities, 1,594 microflows, 5,581
refs) after `refresh catalog full`:

- **God nodes** (pure SQL on `refs`): `Encryption.Decrypt` (22 callers),
  `TcConnector.ModelObject` (46 in-degree), fan-out `Pert_CreateDemoData` (48).
- **Module coupling** (pure SQL): `AmazonBedrockConnector→GenAICommons` (107
  edges), Teamcenter stack (`TeamcenterToolkit→TcConnector` 77, `Viewer3D_TC→
  TcConnector` 43).
- **Leiden** (reference `leidenalg`, used only to validate): **105 communities,
  modularity 0.878**, computed in <1s. Communities map to real cross-module
  subsystems — the Teamcenter integration, the GenAI stack, the AWS stack, the
  OIDC user-provisioning flow. Resolution γ tunes granularity meaningfully
  (γ=0.5→94, γ=1.0→105, γ=2.0→119 communities); RefKind edge-weighting was
  marginal (Q 0.878→0.867), so weighting is **not** worth exposing in v1.

This proposal delivers the same analytical value **natively** — SQL views for the
report, a pure-Go Leiden for communities — with no Python, no CGO, no graph-DB,
preserving mxcli's single-static-binary property.

## Target use cases (drive the design)

The feature must serve two concrete refactoring journeys. They share the graph
substrate but each needs one analysis beyond raw community detection:

### UC1 — Spaghetti → layered, modular app

Untangle a single app with poor internal structure into clean modules and layers.
What's needed:

- **Candidate modules** — Leiden communities propose cohesive groupings the
  current module structure doesn't reflect (assets that belong together).
- **Cycle detection** — spaghetti *is* cyclic dependency. Strongly-connected
  components (SCC) over the directed `refs` graph reveal mutually-entangled
  asset/module groups that must be broken before the app can be layered.
- **Layering** — condense SCCs, topologically level the result; every asset/module
  gets a **sequence number** (observed topological order). mxcli exposes the number
  and the directed dependency edges; the team decides what ordering is "correct"
  and enforces it via their own Starlark rule (Part 1c).

### UC2 — Monolith → multi-app solution (REST/OData/events)

Split one large app into several deployable apps connected by integration
contracts. What's needed:

- **Candidate apps** — Leiden at a coarse `--resolution` yields low-cut partitions
  (modularity already minimises inter-cluster edges → good app seams).
- **Integration surface (cut analysis)** — for a proposed partition, enumerate the
  edges crossing each boundary, **classified by `RefKind` into the integration
  mechanism** they'd require. Validated on Evora:

  | Crossing `RefKind` | Becomes | Example (Evora) |
  |--------------------|---------|-----------------|
  | `associate` | published/shared entity (**OData**) or denormalise | `AgentCommons→AmazonBedrockConnector ×8` |
  | `retrieve` | **OData** read contract | `FactoryManagement→TcConnector ×9` |
  | `create` / change | **event** or REST write | — |
  | `call` | **REST** (published microflow) | `FactoryManagement→AgentCommons ×7` |
  | `generalize` | **hard constraint** — inheritance can't cross an app boundary; keep in same app | `…×6` flagged |

  Because mxcli already models **published REST services, OData, and business
  events**, the report can both *quantify* a split ("needs N OData entities, M REST
  ops, K events") and later *scaffold* those contracts.

Design consequence: the partitioner (Leiden) is shared, but **UC1 needs SCC +
layering** and **UC2 needs cut/integration-surface analysis** — both added below,
both pure-Go over the same `refs` substrate.

## Scope

In scope (this proposal):

1. **Analysis views** over `CATALOG.REFS` (+ `objects`/`entities`/`modules`) — god
   nodes, module coupling, cohesion, dead assets, RefKind distribution, entity
   hotspots, **plus dependency cycles (SCC) and layering for UC1**, **plus the
   community-cut integration surface for UC2**. Independently queryable
   (`select * from CATALOG.graph_god_nodes`).
2. **`mxcli graph-report`** — a thin renderer: markdown assembled from `SELECT`s
   over the views above. No new analysis logic lives in the command.
3. **Community detection** — a **pure-Go Leiden** implementation, results stored
   in a `communities` catalog table, computed via `refresh catalog communities`,
   surfaced through `SHOW COMMUNITY [MEMBERS] OF` and a `community_summary` view.
   The cut/integration-surface analysis (UC2) reads back from this table.
4. **Starlark linter builtins** exposing the graph facts (`layer_of`, `cycles`,
   `module_dependencies`, `community_of`, `degree`, …) so users validate their
   *own* architecture guidelines — mxcli ships facts, not an opinion (Part 1c).

Explicitly **out of scope**:

- **Graph export** (GraphML/DOT/JSON for Gephi/Neo4j). Deliberately dropped — the
  built-in report + communities cover the stated goal (understand & improve an
  app) without mxcli owning an interchange format.
- Edge weighting by RefKind (validated as marginal; default unweighted).
- Multi-modal extraction, vector embeddings, a graph-DB backend.

## Dependency: refs completeness

Both halves are only as good as the edges in `CATALOG.REFS`. The recent #663 work
(nanoflow/REST/association/layout edges + nanoflow sources) is what makes the
validation above meaningful. Known remaining edge gaps — **widget→microflow
button actions, attribute-level, calculated-by** — will under-weight some nodes
and slightly blur page/UI clusters. Closing those (#663 remainder) multiplies the
value here and should proceed in parallel. This is noted as a limitation, not a
blocker.

## BSON Structure

Not applicable. This feature only **reads** the catalog (`refs`, `objects`,
`entities`, `modules`) and **writes** a derived `communities` table into
`catalog.db`. It never reads or writes Mendix document BSON, so there is no
storage-name / pointer-inversion risk.

## Part 1 — Analysis views + `graph-report`

### Views (added to the catalog schema like existing views)

All read from `refs`, so they are populated only after `refresh catalog full`
(fast mode leaves `refs` empty). Module names are derived from the qualified-name
prefix (`substr(.., 1, instr(.., '.')-1)`) — **not** by joining `entities`, which
would silently drop microflow/page/layout targets (a bug in the discussion's
draft SQL).

| View | Purpose | Shape (key columns) |
|------|---------|---------------------|
| `graph_god_nodes` | centrality (degree always; PageRank/betweenness when computed) | `Asset, ObjectType, InDegree, OutDegree, Degree, PageRank, Betweenness, ModuleName` |
| `graph_module_coupling` | cross-module "surprise edges" | `SourceModule, TargetModule, Edges, RefKinds` |
| `graph_module_cohesion` | intra vs inter-module ratio | `ModuleName, IntraEdges, InterEdges, CohesionPct` |
| `graph_dead_assets` | no inbound reference | `QualifiedName, ObjectType, ModuleName` |
| `graph_refkind_distribution` | edge-vocabulary calibration | `RefKind, SourceType, TargetType, Count, Pct` |
| `graph_entity_hotspots` | entities most used by flows | `Entity, UsedByFlows, AcrossModules` |

Notes:
- `graph_god_nodes` computes **degree** in pure SQL (available after `refresh
  catalog full`) and **LEFT JOINs** the `graph_centrality_data` table for
  **PageRank** and **betweenness** (NULL until `refresh catalog communities` runs
  the algorithmic pass — see Part 1b). Degree, PageRank, and betweenness are
  complementary signals, validated as distinct on Evora (top-8 by PageRank
  overlaps degree only 3/8): degree = "referenced a lot", PageRank = "referenced by
  *important* things", betweenness = "bridge/chokepoint on many paths".
- `graph_god_nodes` exposes `ModuleName` so the renderer/consumer can filter
  framework modules (`System`, `Atlas_Core`, marketplace connectors) — the raw
  top-N is dominated by shared infrastructure (layouts, `System.User`), which is
  "expected", not actionable. **Filtering is a presentation concern; the view
  stays unfiltered** so the data is complete.
- `graph_dead_assets` excludes known entry points / framework modules via a small
  allowlist (navigation home pages are reachable but not via `refs`; treat pages
  referenced by `home_page`/`menu_item` as live).

### `mxcli graph-report`

A thin command: for each section it runs `SELECT … FROM CATALOG.graph_*` and
formats the rows as markdown (god nodes table, coupling matrix, cohesion table,
dead-asset list, RefKind summary). It ensures a full refresh first (or errors with
a hint, mirroring `show callers`). `--format markdown|json`, `--top N`,
`--include-framework`. Output mirrors the existing doc-suite style and is meant to
sit alongside `CONCEPTS_FOR_LLMS.md` as a high-level map an agent reads before
diving in.

Because the analysis lives entirely in views, `graph-report` is ~a formatting
loop, and every section is independently reproducible by a human or agent via a
plain catalog `SELECT`.

## Part 1b — Refactoring analyses (UC1 layering, UC2 integration surface)

These add the two capabilities the use cases need beyond community detection. Two
need a graph algorithm (computed in the same pure-Go pass as Leiden, materialised
to tables); one is pure SQL.

### UC1 — cycles & layering (algorithm → tables)

- **`graph_cycles`** — strongly-connected components via **Tarjan** (pure Go,
  deterministic) over the *directed* `refs` graph. An SCC of size > 1 is a
  dependency tangle (the spaghetti). Computed at **asset level** (call/data
  cycles) and rolled up to **module level** (the actionable signal: which modules
  are mutually entangled and can't be layered until broken).
- **`graph_layers`** — condense each SCC to a supernode (the condensation is a
  DAG), topologically level it, and assign every asset/module a **plain sequence
  number** (`Layer = 0,1,2,…` in observed topological order). mxcli deliberately
  attaches **no meaning** to the numbers — some teams want UI→logic→domain→
  integration, others organise by functional module. We ship the observed order
  and the directed dependency facts; the *guideline* (and therefore what counts as
  a "violation") lives in the user's own rules — see **Part 1c**.

No opinionated `graph_layer_violations` view ships: a violation is defined by the
user's architecture, not ours.

### Centrality beyond degree (algorithm → `graph_centrality_data`)

Degree centrality (god nodes) is pure SQL, but two richer signals need the graph
and are computed in the same pure-Go pass:

- **PageRank** — power-method on the *directed* `refs` graph (uses edge direction
  naturally; ~30 lines; deterministic with a fixed damping factor and stable node
  order). Ranks an asset by the importance of what references it, not just the
  count — surfaces structurally-central nodes degree misses (Evora:
  `TcConnector.POM_application_object`, degree 10 → PageRank #2).
- **Betweenness** — Brandes' algorithm; identifies **bridge/chokepoint** assets on
  many shortest paths (distinct from both degree and PageRank — the thing that, if
  changed, ripples through unrelated parts). **Cost caveat:** Brandes is O(V·E) —
  fine at Evora scale (3k·5k ≈ 15M ops, sub-second) but expensive on the largest
  apps (#651-scale: ~30k nodes). Compute it **opt-in** (a flag on
  `refresh catalog communities`, default on up to a node-count threshold, skipped
  with a logged note above it), so a routine refresh never silently becomes slow.

Both write to `graph_centrality_data (Id, PageRank, Betweenness)`, LEFT-JOINed by
`graph_god_nodes`.

### UC2 — integration surface (pure SQL view over communities)

- **`graph_integration_surface`** — *no new algorithm*: once communities exist, a
  cross-community edge is a join. The view groups boundary-crossing `refs` by
  `(SourceCommunity, TargetCommunity, RefKind)` and maps each `RefKind` to the
  integration mechanism a split would require:

  ```sql
  CREATE VIEW graph_integration_surface AS
  SELECT cs.CommunityId AS SourceCommunity, ct.CommunityId AS TargetCommunity,
         r.RefKind, count(*) AS Edges,
         CASE r.RefKind
           WHEN 'associate'  THEN 'OData / shared entity'
           WHEN 'retrieve'   THEN 'OData read'
           WHEN 'create'     THEN 'event / REST write'
           WHEN 'call'       THEN 'REST (published microflow)'
           WHEN 'generalize' THEN 'BLOCKER: inheritance across boundary'
           ELSE 'review'
         END AS Mechanism
  FROM refs r
  JOIN communities cs ON r.SourceName = cs.AssetName
  JOIN communities ct ON r.TargetName = ct.AssetName
  WHERE cs.CommunityId <> ct.CommunityId
  GROUP BY SourceCommunity, TargetCommunity, r.RefKind;
  ```

  This is the contract list for a proposed split. Because mxcli already models
  published REST services, OData, and business events, a follow-up can both
  *quantify* the integration burden and *scaffold* the contracts. `generalize`
  crossings are surfaced as **blockers** (the two communities must stay in one app
  or the inheritance must be removed first).

The `--resolution` knob on `refresh catalog communities` is what lets the same
machinery serve both grains: high γ for fine candidate **modules** (UC1), low γ
for coarse candidate **apps** (UC2).

## Part 1c — Starlark linter integration (validate your *own* architecture)

**This is the primary consumer of the graph facts, not `graph-report`.** Teams
disagree on what "good architecture" means — strict layering, functional modules,
allowed/forbidden module dependencies, max coupling, no cycles. mxcli must not
pick one. The design rule is therefore: **mxcli exposes graph facts; the policy
lives in user Starlark rules.** Users already write Starlark lint rules (e.g. to
enforce their own layering) — today they'd have to reconstruct the graph
themselves. We give them the primitives.

Starlark rules consume a curated builtin API (not raw SQL) — `entities()`,
`microflows()`, `refs_to()`, etc. We add graph builtins, each a thin reader over
the views above (same pattern as the existing `refs_to`):

| Builtin | Returns | Enables the rule to… |
|---------|---------|----------------------|
| `layer_of(asset)` | sequence number (int) | compare layers per the team's own ordering |
| `community_of(asset)` | `{id, label}` | assert co-located assets stay together |
| `module_dependencies()` | list of `{source_module, target_module, ref_kind, edges}` | forbid/allow specific module→module deps |
| `refs_from(source)` | outbound refs (complements existing `refs_to`) | walk a node's own dependencies |
| `cycles()` | list of SCCs (members, scope: asset/module) | fail if any cycle (or a *specific* forbidden cycle) exists |
| `centrality(asset)` | `{in, out, total, pagerank, betweenness}` | flag hotspots above the team's threshold on *any* metric |
| `god_nodes(metric="degree"\|"pagerank"\|"betweenness", min=N)` | high-centrality assets | same, as a filtered list |
| `integration_surface()` | cross-community edges by kind+mechanism | gate that an app split's contract count stays under budget |

These read the `graph_*` / `communities` views, so they're only meaningful after
`refresh catalog full` + `refresh catalog communities` (the linter already ensures
catalog freshness; rules using graph builtins additionally need the communities
build — document this, and have the builtins return an empty list with a one-time
warning if the table is absent rather than failing the whole lint run).

Example — a team that enforces strict layering writes *their* rule (mxcli ships no
such rule):

```python
# my-layering.star — "a module may only depend on lower or equal layers"
def check():
    violations = []
    for dep in module_dependencies():
        if dep.ref_kind in ("layout", "show_page"):  # ignore UI navigation
            continue
        if layer_of(dep.source_module) < layer_of(dep.target_module):
            violations.append(violation(
                message = "%s (layer %d) depends upward on %s (layer %d)" % (
                    dep.source_module, layer_of(dep.source_module),
                    dep.target_module, layer_of(dep.target_module)),
                severity = "error"))
    return violations
```

A different team enforces "the Payments module must never depend on Reporting":

```python
def check():
    return [violation(message = "Payments must not depend on Reporting")
            for d in module_dependencies()
            if d.source_module == "Payments" and d.target_module == "Reporting"]
```

Same facts, opposite-shaped policies — neither baked into mxcli. This is the
"expose just enough" requirement met: the graph numbers are facts; the guideline
is the user's.

## Part 2 — Communities (pure-Go Leiden)

### Storage

A new catalog table + views, following the existing `<name>_data` + view pattern:

```sql
CREATE TABLE communities_data (
  Id            TEXT,   -- asset id
  AssetName     TEXT,   -- qualified name
  AssetType     TEXT,   -- ObjectType (ENTITY/MICROFLOW/PAGE/…)
  ModuleName    TEXT,
  CommunityId   INTEGER,
  ProjectId     TEXT,
  SnapshotId    TEXT
);
-- view `communities` adds project/snapshot framing (like other views)
-- view `community_summary`: per-community size, type breakdown, dominant module, members
```

`community_summary` is plain SQL over `communities` (size, `COUNT(... ) FILTER`
per AssetType, `GROUP_CONCAT(members)`, dominant module) — the same "view layer"
the report uses.

### Computation: `refresh catalog communities`

A new refresh mode parallel to `full`/`full source`. It performs **one edge load
and graph build**, then runs all three algorithms (they share the substrate):

1. Ensure `refs` is current (trigger/require `refresh catalog full`).
2. Read the edge set from `refs` (structural kinds: `call, retrieve, create,
   associate, generalize`; navigational `layout/parameter/show_page` excluded by
   default, opt-in via flag). Keep both a **directed** view (for SCC/layering) and
   an **undirected** view (for Leiden).
3. Run **native Go Leiden** (communities) + **Tarjan SCC** (cycles) +
   **condensation topo-leveling** (layers) + **PageRank** and **betweenness**
   (centrality) — all pure Go, deterministic.
4. Upsert `communities_data`, `graph_cycles_data`, `graph_layers_data`,
   `graph_centrality_data`; the dependent views (`community_summary`,
   `graph_integration_surface`, `graph_module_dependencies`, and the
   PageRank/betweenness columns of `graph_god_nodes`) read from them directly.

`--resolution <γ>` (default 1.0) exposes the validated granularity knob. Results
must be **deterministic** (stable node ordering + fixed seed) per the repo's
map-iteration-determinism rule — no `Math.random`, sort node ids before
iterating.

### Native Go Leiden

New pure-Go package `mdl/catalog/community/` (no CGO, no external deps):

- Build a weighted undirected graph (parallel edges between the same pair
  collapse to an integer weight).
- **Phase 1 — local moving**: greedily move each node to the neighbouring
  community maximising modularity gain ΔQ with resolution γ:
  `Q = 1/2m · Σ_ij [A_ij − γ·k_i·k_j/2m] · δ(c_i,c_j)`.
- **Phase 2 — refinement** (Leiden's guarantee): split any community that is not
  internally connected, so every output community is a connected subgraph.
- **Phase 3 — aggregation**: collapse communities to supernodes (summing edge
  weights) and repeat 1–3 until ΔQ ≈ 0.

The graph is tiny (≤ a few thousand nodes; <1s even naive), so this needs no
performance engineering. Implementation is a few hundred lines.

### MDL surface

| Statement | Result |
|-----------|--------|
| `SHOW COMMUNITY OF Module.Asset` | the community id + summary the asset belongs to |
| `SHOW COMMUNITY MEMBERS OF Module.Asset` | all assets in the same community |
| `SHOW COMMUNITIES` | `community_summary` listing |

These read the `communities`/`community_summary` views; if the table is empty they
error with a hint to run `refresh catalog communities`. (Following
`.claude/skills/design-mdl-syntax.md`: standard `SHOW` verb, qualified names,
reads as English.)

## Algorithm choice (and why not Infomap)

This was contested, so the rationale is recorded here to avoid re-litigation. The
choices are deliberate, not defaults, and were validated empirically on
`Evora-FactoryManagement`.

**Different questions need different algorithms — they are complementary, not
alternatives:**

| Question | Algorithm | Why |
|----------|-----------|-----|
| What groups belong together (candidate modules/apps)? | **Leiden** (undirected, modularity) | modularity *minimises inter-cluster edges* — which **are** the integration contracts in UC2, so it directly minimises split cost |
| Where are the dependency tangles? | **Tarjan SCC** (directed) | cycles are inherently directional; pure-SQL CTEs cannot compute SCC |
| What's the dependency depth / layering? | **topo-level on the SCC condensation** (directed) | must break cycles first (SCC), then level — not expressible in SQL on a cyclic graph |
| What's most important? | **degree + PageRank + betweenness** (PageRank/betweenness directed) | three distinct, complementary signals (count / referrer-importance / bridge) |

**Why undirected Leiden for communities, despite `refs` being directed.**
Direction matters — but for *layering and cut-asymmetry*, which SCC + the directed
`graph_integration_surface` already capture (A→B with no B→A is a clean one-way
contract; A↔B is a hard cut = same SCC). For the *grouping* question, "should A
and B be one app?" is symmetric coupling, and modularity is the objective that
yields the **fewest crossing edges = fewest contracts**.

**Why not Infomap (the directed-flow alternative), tested head-to-head:**

| | communities | modularity | module-purity |
|---|---|---|---|
| Leiden (undirected) | 105 | **0.878** | 71% |
| Infomap (directed) | **320** | 0.782 | 86% |

Infomap optimises random-walk description length, not cut size, and on this graph
it is **worse on every axis that matters for refactoring**: 3× more communities
(too granular to action, and no resolution knob), **lower modularity → more
crossing edges → more integration contracts** (the opposite of the UC2 goal), and
**higher module-purity → it mostly reproduces the existing modules**, hiding the
cross-module bounded contexts that are the whole point of UC1. The same pattern
held on the homogeneous call-flow subgraph (Infomap's best case). Its objective
also assumes edges are *flow*; `refs` is mostly typed static dependencies
(`generalize`/`associate`/`retrieve`), where the flow model doesn't apply. Infomap
is also materially harder to implement natively than Leiden (Leiden = Louvain + a
refinement pass). *Niche where it could earn a place later:* its high
module-purity is a useful **conformance** signal ("do my modules match the natural
structure?") — a possible `graph-conformance` follow-up, not the partitioner.

**Other considered and declined for v1:** Louvain (Leiden minus the connectivity
guarantee — we get it nearly free natively); label propagation (non-deterministic,
conflicts with the catalog-stability requirement; viable only as an opt-in
`--fast` with fixed ordering).

## Implementation Plan

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/catalog/tables.go` | add report views (`graph_god_nodes`, `graph_module_coupling`, `graph_module_cohesion`, `graph_dead_assets`, `graph_refkind_distribution`, `graph_entity_hotspots`) + `graph_module_dependencies` + `graph_integration_surface` views; `communities_data`/`graph_cycles_data`/`graph_layers_data`/`graph_centrality_data` tables + their framed views + `community_summary`; bump `CatalogSchemaVersion` |
| `mdl/linter/starlark.go` | add graph builtins: `layer_of`, `community_of`, `module_dependencies`, `refs_from`, `cycles`, `centrality`, `god_nodes`, `integration_surface` |
| `mdl/linter/context.go` | `LintContext` accessors backing the graph builtins (read `graph_*`/`communities` views) |
| `mdl/catalog/graph/leiden.go` (new) | pure-Go Leiden (local-move, refine, aggregate) with resolution γ |
| `mdl/catalog/graph/scc.go` (new) | pure-Go Tarjan SCC + condensation topo-leveling (cycles, layers) |
| `mdl/catalog/graph/centrality.go` (new) | pure-Go PageRank (power method) + betweenness (Brandes, opt-in/capped) |
| `mdl/catalog/graph/*_test.go` (new) | algorithm unit tests (Zachary karate club; planted communities; known-cycle graphs for SCC; DAG layering; PageRank/betweenness vs reference values) |
| `mdl/catalog/builder_graph.go` (new) | load edges from `refs` once, build directed+undirected graph, run Leiden/SCC/layers/centrality, write the four `_data` tables |
| `mdl/catalog/builder.go` | wire the graph build into the `communities` refresh mode |
| `mdl/ast/ast_query.go` | `RefreshCatalogStmt.Communities` + `Resolution`; `ShowCommunity`, `ShowCommunityMembers`, `ShowCommunities` |
| `mdl/grammar/domains/MDLCatalog.g4` | `REFRESH CATALOG COMMUNITIES [RESOLUTION n]`; `SHOW COMMUNITY [MEMBERS] OF qn`; `SHOW COMMUNITIES` |
| `mdl/visitor/visitor_catalog.go` | build the new AST nodes |
| `mdl/executor/cmd_catalog.go` | handle the `communities` refresh mode |
| `mdl/executor/cmd_community.go` (new) | `SHOW COMMUNITY*` handlers (read views) |
| `cmd/mxcli/cmd_graph_report.go` (new) | `mxcli graph-report` — render markdown/json from the `graph_*` views (god nodes, coupling, cohesion, dead, cycles, layers, integration surface) |
| `mdl/catalog/catalog.go` | add the new views/tables to `Tables()` (drift test enforces) |
| docs-site `tools/code-navigation.html` / `tools/catalog.html` | document views, report, communities, refactoring analyses |

### Order of operations

1. **Views + `graph-report`** (no algorithm) — ships standalone value, pure SQL.
2. **Native Go Leiden + SCC/layering + PageRank/betweenness + tests** — algorithms
   in isolation, validated on known graphs before touching the catalog.
3. **`refresh catalog communities`** + `communities`/`graph_cycles`/`graph_layers`/
   `graph_centrality` tables and dependent views (`graph_integration_surface`,
   `graph_module_dependencies`, `graph_god_nodes` PageRank/betweenness columns).
4. **`SHOW COMMUNITY*`** MDL surface.
5. **Starlark graph builtins** (`layer_of`, `cycles`, `module_dependencies`, …) +
   an example user-layering rule in the docs — the UC1/UC2 validation surface.
6. Docs + `mxcli init` skill note.

## Version Compatibility

Version-independent — pure analysis over the catalog, no Mendix-version-gated
BSON. Works on any project that produces a `refs` table (`refresh catalog full`).
No `sdk/versions/*.yaml` entry needed.

## Test Plan

- **Graph algorithm unit tests** (`mdl/catalog/graph/`):
  - *Leiden*: Zachary's karate club (~2–4 communities, modularity ≈ 0.4) +
    planted-partition synthetic graphs; assert determinism (same input →
    identical partition), every community is internally connected (Leiden's
    guarantee), and resolution monotonically increases community count.
  - *SCC*: graphs with known cycles (a 3-cycle, nested cycles, a DAG with zero
    multi-node SCCs); assert exact SCC membership and determinism.
  - *Layering*: a known DAG → assert the sequence numbers match the topological
    order; a graph with a back-edge → assert the two ends land in the expected
    relative order (no opinionated "violation" — that's the user's rule).
  - *Centrality*: PageRank against a hand-computed small graph (and sums to 1);
    betweenness against a known graph (e.g. a star → centre has max, leaves 0);
    both deterministic across runs.
  - *Starlark builtins*: a fixture `.star` rule using `layer_of`,
    `module_dependencies`, `cycles`, `community_of` runs against a seeded catalog
    and produces the expected violations; builtins return `[]` (not error) when the
    communities table is absent.
- **View tests** (`mdl/catalog/`): seed a small `refs` set, assert
  `graph_module_coupling` counts cross-module edges for **all** target types (not
  just entities — the corrected SQL), `graph_dead_assets` excludes referenced
  assets, `graph_god_nodes` degrees are correct, and `graph_integration_surface`
  maps each crossing `RefKind` to the right mechanism and flags `generalize` as a
  blocker. Extend `TestTables_CoversAllViews` (auto-covers the new views).
- **Integration** (`//go:build integration`, `roundtrip_catalog_refs_test.go`
  style): build a project, `refresh catalog communities`, assert
  `SHOW COMMUNITY MEMBERS OF` returns co-clustered assets, `SHOW COMMUNITY OF`
  resolves, and a deliberately-cyclic two-module fixture shows up in
  `graph_cycles`.
- **MDL examples**: `mdl-examples/doctype-tests/` script exercising
  `refresh catalog communities` + `SHOW COMMUNITY*` + a `graph-report` snapshot.

## Resolved design decisions (driven by UC1/UC2)

1. **Node granularity → asset-level, with module rollups.** Both use cases need
   asset-level nodes to find the *real* clusters (the current module structure is
   exactly what UC1 distrusts and UC2 may discard). Communities, SCC, and layers
   are computed per asset, then rolled up to module level (the actionable unit) via
   summary views. So we keep one finest-grained computation and expose both grains.
2. **Framework filtering → excluded from partitioning, shown as boundary deps.**
   You don't refactor `System`/`Atlas_*`/marketplace connectors, so they must not
   pollute communities or layers — but they *are* real dependencies. Decision:
   exclude a built-in framework allowlist from community/layer assignment by
   default (`--include-framework` to override), but still count their edges in
   `graph_module_coupling` and `graph_integration_surface` as **external/shared**
   dependencies (a shared connector across two candidate apps is a key UC2 signal).
3. **Edge set → structural by default, direction-aware.** Community detection uses
   undirected structural kinds; **SCC/layering require direction** (kept from the
   same load). Navigational kinds (`show_page/layout/parameter`) are opt-in via
   flag — they add UI coupling that's useful for some module-split questions but
   noise for app-split.
4. **Resolution is the UC1/UC2 selector.** Same machinery, two grains: high γ →
   fine candidate **modules** (UC1); low γ → coarse candidate **apps** (UC2).
   `refresh catalog communities --resolution` exposes it; the report can render
   both a "module view" and an "app view".
5. **`graph-report` is distinct from `report`.** `report` scores best practices;
   `graph-report` is the architecture map (god nodes, cycles, layers, communities,
   integration surface). Different audience, different cadence.
6. **Community ids get a stable `Label`.** Raw Leiden ids renumber across runs, so
   `community_summary` includes a derived `Label` (dominant module / highest-degree
   member) used in `SHOW COMMUNITY*` output and the integration-surface report
   instead of bare integers.

## Open Questions

1. **Integration-surface scaffolding.** This proposal only *reports* the contract
   list (UC2). Auto-generating the published REST service / OData / business-event
   stubs from a chosen cut is a natural but separate follow-up — confirm it's
   out of scope here.
2. **Whole-portfolio vs single-app.** UC2 ends with multiple apps; do we ever need
   to analyse the graph *across* already-split apps (federated catalog)? Assumed no
   for v1 — single `.mpr` scope.
3. **Graph builtins freshness.** Should the graph Starlark builtins auto-trigger
   `refresh catalog communities` when the table is stale/absent, or only warn-and-
   skip (proposed)? Auto-trigger is convenient but makes a lint run unexpectedly
   expensive; warn-and-skip keeps lint fast and predictable.

## Relation to Graphify — coverage & deferred scope

This proposal covers, and in places exceeds, Graphify's **offline structural
analysis**: god nodes (degree **+ PageRank + betweenness** vs Graphify's degree
only), surprise edges, Leiden communities (**+ directed SCC/layering + cut→contract
analysis** Graphify lacks), and the `GRAPH_REPORT.md` convention — over **typed**
edges a generic AST extractor can't produce.

It deliberately does **not** cover Graphify's **runtime agent-context** half:

- **Query-scoped subgraph as an agent primitive** — Graphify's headline pitch
  (the "71.5× token reduction") is that an agent pulls the slice around a node
  instead of ingesting everything. mxcli's `show context of X depth N` already does
  the BFS walk, but emits human-readable markdown, isn't a **token-minimal
  machine-readable** subgraph, and isn't exposed as an **MCP tool** an agent can
  call. Closing this — plus *measuring* the compression on a real Mendix app — is a
  distinct concern (runtime serving, not offline analysis) and belongs in a
  **sibling proposal** built on the catalog graph this one establishes, not here.
- **Semantic / multi-modal extraction** — Graphify also extracts concepts from
  prose/docs/diagrams via LLMs. For a self-describing typed model this is low value;
  the one Mendix analogue (concept extraction from `Documentation` fields) is a
  separate, fuzzier project and is **not pursued**.

So: this proposal is the **analysis layer**; the **agent-context-compression
layer** (BFS subgraph → MCP, with a measured token-reduction claim) is
acknowledged here and left to follow-up work.

## Relation to engalar's `mxgraph` — substrate & the deferred sibling

The engalar fork carries an `internal/mxgraph` package (a "file-based graph index
framework") that overlaps this proposal — but **at a different layer**, so it is
not a conflict. Recorded here so the substrate decision is deliberate.

**What `mxgraph` is:** an extraction + incremental-index + traversal-query engine —
an in-memory graph (adjacency/label/property indexes) maintained **incrementally**
via an append-only delta log + snapshot compaction and a `Watch()` file watcher,
populated by pluggable adapters (MPR docs **plus** access rules, document grants,
widget instances, theme/SCSS, design properties), exposing a path/traversal query
API (`FindPathSchemas`/`ExplorePath`/`Traverse`/`Neighbors`) aimed at AI model
exploration. It does **no** community/centrality/cycle analysis (verified: no
Leiden/PageRank/betweenness/modularity anywhere in the package).

**No overlap on this proposal's core.** The analysis half — Leiden communities,
SCC/cycles, layering, PageRank/betweenness, integration-surface, the Starlark
builtins and `SHOW COMMUNITY` surface — has no counterpart in `mxgraph`. This
proposal proceeds unchanged, on `refs`, single-binary and SQL-native.

**Two genuine points of contact:**

1. **Substrate (a tension to weigh later, not now).** This proposal's premise is
   "`refs` is already a rich typed edge list — this is a rendering problem, not an
   extraction problem." `mxgraph` is a more elaborate extraction layer that
   directly addresses the two limitations this proposal flags: it is **incremental**
   (Watch/delta vs `refs`' full rebuild) and its adapters cover the exact edge gaps
   called out under *Dependency: refs completeness* (widget→microflow, attribute-
   level, theme/design). The cost is a second model-graph substrate and persistence
   store (`mxgraph` snapshots vs `catalog.db`). **Decision: ship the analysis over
   `refs` as designed.** Whether the analysis should later read `mxgraph`'s edges
   instead of `refs` (richer + incremental) is a substrate migration to evaluate
   explicitly as a separate piece of work — out of scope here.

2. **`mxgraph` ≈ the deferred agent-context sibling.** The
   query-scoped-subgraph / token-minimal / MCP-tool layer this section defers to a
   "sibling proposal" is essentially what `mxgraph`'s traversal + path-schema query
   API already implements. When that sibling is taken up, **evaluate porting
   `mxgraph` as its base rather than building greenfield.**

(Naming note: engalar's spec uses "graphify" for a *knowledge/docs* indexer and
`mxgraph` for the *model* graph; this proposal is named after the graphify.net
analysis it reproduces. The collision is incidental.)
