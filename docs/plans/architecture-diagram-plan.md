# Implementation Plan: Cross-cutting Architecture Diagram

## Summary

Add a new diagram type to mxcli that visualizes a Mendix project's architecture organized by **horizontal layers** (Pages → Microflows → Entities → External Services), scoped to a module or set of pages. This replaces the original "journey" proposal with a simpler, catalog-driven approach that reuses the existing ELK pipeline and avoids the unsolved problem of auto-detecting user journeys.

## Why not the original "Journey" proposal?

The original proposal (`journey-architecture-viz.md`) has several issues when mapped to the actual codebase:

1. **Navigation parsing doesn't exist.** Only `NavigationDocument.Name` is parsed today — no profiles, home pages, menu items, or role mappings. Building a full navigation parser is a large standalone task (BSON structure exploration, new parser functions, new types) that should not be coupled to a visualization feature.

2. **Journey auto-detection is an unsolved problem.** Real Mendix apps have cyclic page navigation, role-dependent paths, shared utility pages, and modal popup flows that don't fit a linear left-to-right journey model. The proposal's "topological sort with tie-breaking" approach will produce noisy, misleading diagrams for non-trivial projects.

3. **The proposed file structure doesn't match the codebase.** The proposal suggests `cmd/journey/`, `internal/journey/`, `internal/mendix/` — none of which exist. The actual pattern is one Go file in `mdl/executor/`, one renderer function in `previewProvider.ts`.

4. **The data extraction is already done.** The catalog `refs` table already has `call`, `create`, `retrieve`, `show_page`, `datasource`, `parameter`, `action` ref kinds. The `show context` command already walks these relationships recursively.

## What we build instead

A **layered architecture diagram** that shows, for a given scope (module or explicit page list):

```
┌─────────────────────────────────────────────────────┐
│  pages          [page A]    [page B]    [page C]    │
│                    │            │           │        │
│  microflows    [MF_1] [MF_2] [MF_3]   [MF_4]      │
│                  │       │      │         │         │
│  entities      [Order] [Customer] [Product]         │
│                                                     │
│  external      [rest: PaymentAPI]                   │
└─────────────────────────────────────────────────────┘
```

- **Vertical layers** with ELK layer constraints (pages on top, entities on bottom)
- **Edges** show actual relationships from the `refs` table (action, call, create/retrieve, show_page, datasource)
- **Grouped by module** when scope is multi-module
- **Proportional node sizes** based on complexity (activity count, widget count)
- **Sketchy rendering** reusing existing primitives (roughLine, roughRoundedRect, markerFill)
- **Color-coded by layer** (blue=pages, orange=microflows, purple=entities, pink=external)

## Scope options

```bash
# Architecture of a single module (most useful default)
mxcli describe -p app.mpr --format elk architecture MyModule

# Architecture scoped to a specific page and its transitive dependencies
mxcli describe -p app.mpr --format elk architecture MyModule.Customer_Overview

# full project architecture (may be large)
mxcli describe -p app.mpr --format elk architecture _all
```

When scoped to a module:
1. Find all pages in that module
2. Find all microflows called from those pages (via `action` refs) + microflows in the module
3. Find all entities accessed by those microflows (via `create`/`retrieve` refs) + entities used by pages (`datasource`/`parameter` refs)
4. Find external service calls (from `activities` table where `ActivityType` is REST/SOAP-related)

When scoped to a page:
1. Start from the page
2. Walk `action` refs to find triggered microflows
3. Walk `call` refs transitively to find sub-microflows
4. Walk `create`/`retrieve` refs to find entities
5. Walk `show_page` refs to find navigated-to pages (1 level deep)

## Files to change

### 1. `mdl/executor/cmd_architecture.go` (NEW)

Go backend that queries the catalog and emits ELK-compatible JSON.

**Data structures:**

```go
type architectureData struct {
    format string              `json:"format"`
    type   string              `json:"type"`    // "architecture"
    Scope  string              `json:"scope"`   // module name or page name
    Layers []architectureLayer `json:"layers"`
    Nodes  []architectureNode  `json:"nodes"`
    Edges  []architectureEdge  `json:"edges"`
}

type architectureLayer struct {
    ID    string `json:"id"`    // "pages", "microflows", "entities", "external"
    label string `json:"label"`
}

type architectureNode struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Layer    string `json:"layer"`     // which layer this belongs to
    module   string `json:"module"`    // source module
    NodeType string `json:"nodeType"`  // "page", "microflow", "entity", "service"
    Metric   int    `json:"metric"`    // complexity metric (widget/activity/attribute count)
    Details  []string `json:"details"` // hover details
}

type architectureEdge struct {
    ID       string `json:"id"`
    source   string `json:"source"`
    Target   string `json:"target"`
    RefKind  string `json:"refKind"`  // from refs table
}
```

**Algorithm (`ArchitectureDiagram` method on `Executor`):**

1. `ensureCatalog(true)` — need refs
2. Determine scope (module name vs qualified page name vs `_all`)
3. Query pages in scope from `pages` table
4. Query microflows connected to those pages:
   ```sql
   select distinct SourceName from refs
   where TargetName in (<pages>) and RefKind = 'action'
   union
   select distinct TargetName from refs
   where SourceName in (<pages>) and RefKind = 'action'
   ```
   Plus microflows in the same module, plus transitive `call` refs
5. Query entities connected to those microflows:
   ```sql
   select distinct TargetName from refs
   where SourceName in (<microflows>) and RefKind in ('create', 'retrieve')
   union
   select distinct TargetName from refs
   where SourceName in (<pages>) and RefKind in ('datasource', 'parameter')
   ```
6. Query external service references from `activities` table:
   ```sql
   select distinct EntityRef from activities
   where MicroflowQualifiedName in (<microflows>)
   and ActivityType in ('RestCallAction', 'WebServiceCallAction', 'HttpCallAction')
   ```
7. Build node list with layer assignments
8. Build edge list from refs between the collected nodes
9. Emit JSON

### 2. `cmd/mxcli/main.go`

Add `architecture` to the describe command's ELK format dispatch (alongside `systemoverview`, `microflow`, `domainmodel`, `page`, `entity`):

```go
case "ARCHITECTURE":
    if err := exec.ArchitectureDiagram(args[1]); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
```

### 3. `vscode-mdl/src/previewProvider.ts`

Three additions:

**a) ELK graph construction** (~line 820, alongside existing diagram type branches):

```javascript
} else if (d.type === 'architecture') {
    d.nodes = d.nodes || [];
    d.edges = d.edges || [];
    elkGraph = {
        id: 'root',
        layoutOptions: {
            'elk.algorithm': 'layered',
            'elk.direction': 'DOWN',
            'elk.spacing.nodeNode': '30',
            'elk.layered.spacing.nodeNodeBetweenLayers': '60',
            'elk.edgeRouting': 'ORTHOGONAL',
            'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
        },
        children: d.nodes.map(function(n) {
            var w = Math.max(100, n.name.length * 8 + 20);
            var h = 36;
            return {
                id: n.id, width: w, height: h,
                layoutOptions: {
                    'elk.layered.layerConstraint': layerIndex(n.layer),
                },
            };
        }),
        edges: d.edges.map(function(e, i) {
            return { id: 'e' + i, sources: [e.source], targets: [e.target] };
        }),
    };
}
```

ELK's `layerConstraint` or `partitioning` options can enforce the vertical layer ordering (pages always above microflows, microflows above entities, etc.).

**b) Renderer function** `renderArchitecture(layout)`:

Follow the same pattern as `renderModuleOverview` and `renderMicroflow`:

- SVG with pencil/marker filters, Architects Daughter font
- Title from `data.scope`
- Layer background bands (subtle horizontal stripes, one per layer, with label on left)
- Nodes rendered per layer with layer-specific colors:
  - Pages: blue (`#4a90d9` / `#dce8f5`)
  - Microflows: orange (`#d4881c` / `#fce8cc`)
  - Entities: purple (`#7b5ea7` / `#e8dff0`)
  - External: pink (`#c44e8a` / `#f5dce8`)
- Edges: rough lines with arrowheads, styled by ref kind:
  - `action`/`call`: solid
  - `create`/`retrieve`: dotted
  - `datasource`/`parameter`: dashed
  - `show_page`: thin, gray
- Node click handler: `vscodeApi.postMessage` to navigate to the element's MDL source

**c) renderSvg dispatch** (line 882):

```javascript
} else if (diagramType === 'architecture') {
    renderArchitecture(layout);
}
```

### 4. `vscode-mdl/src/previewProvider.ts` — `showDiagram()`

Add `'architecture'` to the effective type mapping (line 50-75) so the VS Code command can trigger it.

### 5. `vscode-mdl/src/previewProvider.ts` — `generateElk()`

Ensure `architecture` type gets routed through the ELK path (it will naturally if the `effectiveType` is in the set that calls `generateElk`).

## Implementation order

### Phase 1: Go backend (smallest useful vertical slice)

1. Create `mdl/executor/cmd_architecture.go` with the `ArchitectureDiagram` method
2. Add CLI dispatch in `cmd/mxcli/main.go`
3. Test with: `./bin/mxcli describe -p test.mpr --format elk architecture ModuleName`
4. Verify JSON output has correct nodes, edges, and layer assignments

**Verification:** pipe JSON through `jq` and manually inspect that:
- Pages from the module appear with `layer: "pages"`
- Microflows called from those pages appear with `layer: "microflows"`
- Entities accessed by those microflows appear with `layer: "entities"`
- Edges match expected ref kinds

### Phase 2: TypeScript renderer (visual output)

1. Add ELK graph construction branch for `architecture` type
2. Add `renderArchitecture()` function with layer bands + nodes + edges
3. Add to `renderSvg()` dispatch
4. Wire up in `showDiagram()` and `generateElk()`

**Verification:** open a module's architecture diagram in VS Code preview, verify:
- Nodes are arranged in horizontal layers (pages on top, entities on bottom)
- Edges connect across layers correctly
- Colors match the layer scheme
- Click on a node opens popover with details

### Phase 3: Polish

1. Add collapse/expand per layer (reuse the pattern from domain model/microflow collapse)
2. Add edge styling by ref kind (solid/dashed/dotted)
3. Add module grouping boxes when scope is `_all` or multi-module
4. Add node sizing proportional to complexity metric

## What this explicitly defers

- **Navigation parsing**: not needed — we use catalog refs instead
- **Journey auto-detection**: not needed — user picks the scope
- **YAML config files**: not needed — scope parameter is sufficient
- **HTML interactive output**: not needed — the VS Code webview already provides pan/zoom/click
- **Role-based filtering**: can be added later if role→page refs are added to the catalog

## Complexity estimate

- `cmd_architecture.go`: ~200 lines (mostly SQL queries + JSON assembly, modeled on `cmd_module_overview.go` which is 225 lines)
- `main.go` dispatch: ~10 lines
- `previewProvider.ts` ELK construction: ~30 lines
- `previewProvider.ts` renderer: ~150 lines (modeled on `renderModuleOverview` which is ~120 lines, plus layer bands)
- Total: ~400 lines of new code
