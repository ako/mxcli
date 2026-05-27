---
title: Prompt: Journey Architecture Diagram Generator for Mendix Projects
status: proposed
---

# Prompt: Journey Architecture Diagram Generator for Mendix Projects

## Context

I'm building a CLI tool (part of mxcli) that generates a "Customer Journey Architecture" diagram from a Mendix project. This diagram type visualizes a software system organized by business purpose rather than technical decomposition:

- **Horizontal axis**: process flow left-to-right, representing the user journey through the system
- **Vertical layers** (top to bottom): User Roles → Pages/Screens → Business Logic (Microflows) → Domain Entities → External Systems
- **Column width** is proportional to relative complexity (number of microflows/entities involved)
- **Shared entities** across steps are shown as dashed horizontal connections in the data layer

The output should be an SVG diagram with a hand-drawn/sketchy aesthetic (wobbly lines, marker-style fills, Caveat font). See the reference implementation in `reference/journey-architecture.jsx` for the exact visual style.

## Data Source

Mendix projects are stored as JSON files following the Mendix Definition Language (MDL). The relevant structures to extract are:

### 1. User Roles
- Location: `security$ProjectSecurity` → `userRoles`
- Each role has a `name` and is associated with module roles

### 2. Navigation
- Location: `navigation$NavigationDocument`
- Contains navigation profiles (Responsive, NativePhone, etc.)
- Each profile has a `homePage` and menu items with `page` references
- Navigation items are linked to specific user roles

### 3. Pages
- Location: `pages$page` files within each module
- Pages contain widgets, some of which trigger microflows (`on-click → call microflow`)
- Pages have `allowedRoles` defining which user roles can see them
- Pages contain data views, list views, etc. that reference entities
- Buttons and actions on pages link to other pages (showing navigation flow)

### 4. Microflows
- Location: `microflows$microflow` files within each module
- Contain activities: `RetrieveAction`, `CreateObjectAction`, `ChangeObjectAction`, `DeleteAction`, `CommitAction`, `ShowPageAction`, `RestCallAction`, `WebServiceCallAction`, `JavaActionCallAction`
- `ShowPageAction` links to target pages (enabling flow tracing)
- Entity access can be extracted from retrieve/create/change/delete activities
- External calls can be identified from `RestCallAction` and `WebServiceCallAction`

### 5. Domain Models
- Location: `DomainModels$DomainModel` per module
- Contains entities with attributes and associations
- Entity count and attribute count can be used as complexity proxies

### 6. Consumed Services
- `ConsumedRest$ConsumedRestService` — REST integrations
- `WebServices$ConsumedWebService` — SOAP integrations
- These represent external system dependencies

## Algorithm

### Phase 1: Extract the Graph

Build an in-memory graph from the Mendix project:

```
role -[can access]-> page
page -[triggers]-> microflow
page -[navigates to]-> page (via buttons/ShowPageAction)
microflow -[accesses]-> entity (via retrieve/create/change/delete)
microflow -[calls]-> microflow (sub-microflow calls)
microflow -[calls]-> ExternalService (via rest/SOAP actions)
microflow -[shows]-> page (via ShowPageAction)
```

### Phase 2: Identify Journeys

A journey is a connected path through the page navigation graph, scoped to one or more user roles. To extract journeys:

1. For each role, find the navigation home page
2. Walk the page→microflow→page graph (following button actions and ShowPageAction targets)
3. Build a directed graph of page transitions per role
4. The longest path or the most common path through this graph is a candidate journey
5. Group pages into "steps" — a step is a page (or tightly coupled set of pages like a wizard) that represents a user task

Since determining the "canonical" journey order may be ambiguous, provide two modes:
- **Auto mode**: use a topological sort of the page navigation graph, breaking ties by navigation menu order
- **Guided mode**: accept a config file where the user specifies step order:

```yaml
journey:
  name: "Order-to-Delivery"
  steps:
    - pages: ["Product_Overview", "Product_Search"]
      label: "Browse"
    - pages: ["Product_Detail", "Product_Configurator"]
      label: "Configure"
    - pages: ["Checkout_*"]
      label: "Order"
```

### Phase 3: Enrich Each Step

For each step (group of pages), collect:

- **User roles**: union of `allowedRoles` across the step's pages
- **Pages**: the pages themselves, with a primary label
- **Microflows**: all microflows triggered (directly or transitively) from these pages, grouped by purpose if possible (use the microflow name as label)
- **Entities**: all entities accessed by those microflows, with access type (read/write/create/delete)
- **External systems**: all REST/SOAP calls made by those microflows, with the service name
- **Complexity metrics**: count of microflows, count of entities, count of external calls — used to determine column width

### Phase 4: Generate SVG

Generate an SVG using the sketchy/hand-drawn style from the reference implementation:

- **Layout**: horizontal flow, vertical layers
- **Column width**: proportional to step complexity (normalize microflow + entity counts)
- **Visual encoding**:
  - Person icons for user roles (top layer)
  - Rounded boxes for pages (screen layer)
  - Smaller boxes for microflows (logic layer), clustered by module
  - Entity boxes (data layer), with entity name and row count hint if available
  - External system boxes (bottom layer) — only shown when present
  - Dashed lines connecting shared entities across steps
  - Vertical connectors between layers showing the traceability chain
- **Sketchy rendering**: use seeded RNG for deterministic wobbly lines, marker-fill backgrounds, hand-drawn font (Caveat via Google Fonts CDN in the SVG)
- **Colors**: muted, per-layer color scheme (green=users, blue=screens, orange=logic, purple=data, pink=external)

## Implementation Structure

```
cmd/journey/
  journey.go          # CLI command: `mxcli journey generate`
  config.go           # Journey config file parsing (YAML)

internal/journey/
  extractor.go        # Phase 1-3: walk Mendix project, build journey model
  model.go            # data types: Journey, Step, ScreenNode, LogicNode, DataNode, ExternalNode
  renderer.go         # Phase 4: SVG generation with sketchy style
  sketch.go           # Sketchy drawing primitives (rough lines, marker fills, etc.)
  layout.go           # layout engine: compute positions, widths, vertical spacing

internal/mendix/
  # Reuse existing MDL parsing infrastructure from mxcli
  # Needs: project loader, entity resolver, microflow walker, navigation parser
```

## CLI Interface

```bash
# Auto-detect journeys from the project
mxcli journey generate --project ./my-app.mpr --output journey.svg

# use a config file for step ordering
mxcli journey generate --project ./my-app.mpr --config journey.yaml --output journey.svg

# generate for a specific role
mxcli journey generate --project ./my-app.mpr --role "Customer" --output journey.svg

# Output as HTML (SVG embedded, with hover interactions)
mxcli journey generate --project ./my-app.mpr --format html --output journey.html
```

## Key Design Decisions

1. **Deterministic rendering**: use a seeded PRNG so the same project always produces the same diagram. The seed should be derived from the project content hash.
2. **Entity deduplication**: when the same entity (e.g., `Order`) appears in multiple steps, show it in each step's data layer but connect instances with dashed lines to show it's the same entity.
3. **Microflow grouping**: group microflows by their parent module, and use the module name as a category label. Individual microflow names appear inside the boxes.
4. **Progressive disclosure**: the SVG output is static but the HTML output can include hover effects (dim other steps when hovering one) and click-to-expand for microflow details.
5. **Graceful degradation**: if the project has no navigation model, fall back to analyzing all pages and grouping by module. If roles are minimal, collapse the user layer.

## Reference Files

- `reference/journey-architecture.jsx` — React component showing the target visual output with sample data
- The existing mxcli MDL parsing code in `internal/mendix/` — reuse the project loader and type resolvers

## What to Build First

Start with a vertical slice:
1. Parse a single Mendix project's navigation, pages, microflows, domain model, and consumed services
2. Build the extraction graph (Phase 1)
3. Generate a journey using auto mode for one role (Phase 2-3)
4. Render a minimal SVG with the correct layout and layer structure, even without the full sketchy style (Phase 4)
5. Then layer in the sketchy rendering primitives
6. Then add the config file / guided mode

Focus on getting the graph walking and layout correct first. The visual polish is well-understood from the reference implementation and can be ported directly.
