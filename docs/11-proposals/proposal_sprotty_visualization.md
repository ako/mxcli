---
title: Claude Code Prompt: Sprotty-based Domain Model Visualization PoC
status: draft
---

# Claude Code Prompt: Sprotty-based Domain Model Visualization PoC

## Context

I'm building a VS Code extension that visualizes Mendix Domain Models using Sprotty. I currently have a working solution using Mermaid diagrams, where my CLI tool `mxcli` generates Mermaid syntax for a domain model and it's rendered in VS Code. However, Mermaid is limited — I need more control over interactivity, styling, and functionality.

### What `mxcli` does

`mxcli` is a Go-based CLI tool that can read and manipulate Mendix project models. It can output domain model data in structured formats. For this PoC, assume we can get domain model data as JSON from `mxcli` (we'll define the schema below).

### What a Mendix Domain Model looks like

A Mendix Domain Model consists of:
- **Entities** — class-like boxes with a name, optional documentation
  - Each entity has **Attributes** — typed fields (String, Integer, Boolean, DateTime, Decimal, Enum, AutoNumber, etc.)
  - Entities can be **generalized** (inheritance) — an entity can extend another entity
  - Entities can be **persistent** or **non-persistent** (transient)
- **Associations** — relationships between entities
  - Have a name, type (Reference [1-1, *-1], ReferenceSet [*-*]), owner (default/both), and optionally navigability
  - Connect a parent entity to a child entity
  - Can have delete behavior (delete, error) on both sides

Think of it as a UML class diagram with domain-specific semantics.

### Example JSON input

The extension should accept JSON like this (either from `mxcli` stdout or a file):

```json
{
  "moduleName": "MyModule",
  "entities": [
    {
      "name": "Customer",
      "persistent": true,
      "generalization": null,
      "documentation": "Represents a customer in the system",
      "attributes": [
        { "name": "Name", "type": "string" },
        { "name": "Email", "type": "string" },
        { "name": "DateOfBirth", "type": "datetime" },
        { "name": "IsActive", "type": "boolean" }
      ]
    },
    {
      "name": "Order",
      "persistent": true,
      "generalization": null,
      "documentation": "A customer order",
      "attributes": [
        { "name": "OrderNumber", "type": "autonumber" },
        { "name": "OrderDate", "type": "datetime" },
        { "name": "TotalAmount", "type": "decimal" },
        { "name": "status", "type": "enum" }
      ]
    },
    {
      "name": "OrderLine",
      "persistent": true,
      "generalization": null,
      "documentation": "",
      "attributes": [
        { "name": "Quantity", "type": "integer" },
        { "name": "UnitPrice", "type": "decimal" }
      ]
    },
    {
      "name": "Product",
      "persistent": true,
      "generalization": null,
      "documentation": "Product catalog item",
      "attributes": [
        { "name": "ProductName", "type": "string" },
        { "name": "SKU", "type": "string" },
        { "name": "Price", "type": "decimal" }
      ]
    },
    {
      "name": "SpecialOrder",
      "persistent": true,
      "generalization": "Order",
      "documentation": "An order with special handling",
      "attributes": [
        { "name": "Priority", "type": "enum" },
        { "name": "SpecialInstructions", "type": "string" }
      ]
    },
    {
      "name": "CustomerSearchParams",
      "persistent": false,
      "generalization": null,
      "documentation": "non-persistent helper entity for search",
      "attributes": [
        { "name": "SearchName", "type": "string" },
        { "name": "SearchEmail", "type": "string" }
      ]
    }
  ],
  "associations": [
    {
      "name": "Customer_Order",
      "parentEntity": "Customer",
      "childEntity": "Order",
      "type": "reference",
      "parentDeleteBehavior": "delete",
      "childDeleteBehavior": "error"
    },
    {
      "name": "Order_OrderLine",
      "parentEntity": "Order",
      "childEntity": "OrderLine",
      "type": "reference",
      "parentDeleteBehavior": "delete",
      "childDeleteBehavior": "error"
    },
    {
      "name": "OrderLine_Product",
      "parentEntity": "OrderLine",
      "childEntity": "Product",
      "type": "reference",
      "parentDeleteBehavior": "error",
      "childDeleteBehavior": "error"
    }
  ]
}
```

## Goal

Build a **VS Code extension** that renders a domain model diagram using **Sprotty** inside a VS Code webview panel. This is a PoC to validate the approach before investing further.

## Requirements

### Must have (PoC scope)
1. **Sprotty-based rendering** in a VS Code webview panel using `sprotty-vscode`
2. **Entity nodes** rendered as boxes showing:
   - Entity name in a header bar
   - Visual distinction between persistent (solid border) and non-persistent (dashed border) entities
   - Attribute list with name and type, collapsible/expandable
3. **Association edges** between entities showing:
   - Association name as label
   - Multiplicity indicators (1, *) at each end based on type
4. **Generalization edges** — dashed arrow from child to parent entity (inheritance)
5. **Automatic layout** using `sprotty-elk` (ELK layered algorithm)
6. **Pan and zoom** — built-in Sprotty viewport controls
7. **Expand/collapse** — click entity header to toggle showing/hiding attributes
8. **Load from JSON file** — a VS Code command that lets you pick a `.json` file and renders it
9. **Hardcoded sample data** — also include the example JSON above as built-in sample data for quick testing

### Nice to have (if straightforward)
- Color coding based on entity type (persistent vs non-persistent) or custom colors
- Tooltip on hover showing entity documentation
- Fit-to-screen button
- Context menu on entities (placeholder actions like "Open in Mendix", "Show Details")
- Entity selection highlighting

### Explicitly NOT in scope
- Integration with `mxcli` CLI (we'll add that later)
- Editing capabilities (this is view-only for now)
- Language server / LSP integration
- Custom file type registration
- Publishing to marketplace

## Technical approach

### Project structure

```
sprotty-domain-model/
├── package.json              # VS Code extension manifest
├── tsconfig.json
├── webpack.config.js         # Builds both extension and webview
├── src/
│   ├── extension/
│   │   ├── extension.ts      # VS Code extension entry point
│   │   └── domain-model-panel.ts  # Webview panel manager
│   ├── webview/
│   │   ├── main.ts           # Sprotty diagram setup in webview
│   │   ├── di.config.ts      # Inversify DI container config
│   │   ├── model.ts          # Sprotty model element types
│   │   ├── views.tsx         # SVG views for entities, edges
│   │   └── model-source.ts   # Transforms json → Sprotty model
│   └── common/
│       └── domain-model.ts   # Shared types (json schema interfaces)
├── sample/
│   └── sample-domain-model.json
└── media/
    └── styles.css            # Diagram styling
```

### Key packages to use
- `sprotty` — core diagram framework
- `sprotty-vscode` — VS Code webview integration  
- `sprotty-vscode-webview` — webview-side library
- `sprotty-vscode-protocol` — shared protocol
- `sprotty-elk` — ELK layout integration
- `elkjs` — layout engine
- `inversify` — dependency injection (required by Sprotty)
- `reflect-metadata` — required by inversify

### Key implementation notes

1. **Use the `sprotty-vscode` WebviewPanelManager pattern** — look at the official states-example in the `eclipse-sprotty/sprotty-vscode` repo for reference. That's the canonical example of Sprotty in VS Code with a Langium language server. We don't need the language server part, just the webview panel setup.

2. **Model elements to define** (extending Sprotty's SNode, SEdge, etc.):
   - `DomainModelGraph` — root graph element
   - `EntityNode` — an SNode with compartments for header and attributes
   - `EntityHeader` — compartment with entity name
   - `AttributeCompartment` — collapsible compartment containing attributes
   - `AttributeRow` — individual attribute (name + type)
   - `AssociationEdge` — edge with multiplicity labels
   - `GeneralizationEdge` — dashed inheritance edge

3. **Views (SVG rendering)** — use JSX/TSX to define custom SVG views for each model element. Entities should look like UML class diagram boxes with a colored header and white body.

4. **Expand/collapse** — use Sprotty's built-in `Expandable` feature. Mark `EntityNode` as expandable and toggle the `AttributeCompartment` visibility.

5. **Layout** — configure ELK with `layered` algorithm, appropriate spacing, and `DOWN` direction.

6. **Communication** — the extension sends the JSON domain model data to the webview via `postMessage`. The webview's model source transforms it into a Sprotty graph model and renders it.

### Reference

The best reference implementation is:
- **GitHub: `eclipse-sprotty/sprotty-vscode`** — specifically the `examples/` folder which has the states (state machine) example
- **GitHub: `eclipse-sprotty/sprotty`** — the `examples/classdiagram/` example shows expand/collapse with compartments, which is very close to what we need
- **npm: `sprotty-vscode`** — check the README for the WebviewPanelManager setup pattern

Study these examples before starting implementation. The class diagram example in the core Sprotty repo is the closest to our domain model visualization.

## How to validate the PoC

1. Run the extension in the VS Code Extension Development Host
2. Execute command "Domain Model: Show Sample" → should render the hardcoded sample
3. Execute command "Domain Model: Open from JSON" → file picker → renders the selected JSON
4. Verify: entities render as boxes with headers and attributes
5. Verify: associations render as labeled edges with multiplicity
6. Verify: generalization renders as dashed inheritance arrow
7. Verify: pan/zoom works
8. Verify: clicking an entity header collapses/expands its attributes
9. Verify: persistent vs non-persistent entities look visually distinct

## Important

- Keep the code clean and well-structured — this PoC will evolve into a production extension
- Use TypeScript strict mode
- Add comments explaining non-obvious Sprotty concepts (DI bindings, action handlers, etc.)
- Make the JSON→Sprotty model transformation clean and separate, as the JSON schema will evolve
- Don't over-engineer — this is a PoC, but write it in a way that's easy to extend
