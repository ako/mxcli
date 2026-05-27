---
title: Multi-Project Tree View
status: draft
date: 2026-04-06
---

# Proposal: Multi-Project Tree View

**Status:** Draft
**Date:** 2026-04-06

## Motivation

Mendix solutions increasingly span multiple applications: a frontend and a backend, or a microservices landscape (product catalog, orders, fulfillment). Another use case is monolith-to-multi-app refactoring — start with one large app and use AI to split it into smaller apps.

Today the VS Code extension assumes a single `.mpr` project per workspace. Supporting multiple projects in the tree view is the first step toward multi-app development workflows.

## Current State

The single-project assumption runs through five layers:

| Layer | File | Assumption |
|-------|------|-----------|
| **Setting** | `package.json` | `mdl.mprPath` is a single `string` |
| **Discovery** | `extension.ts` `findMprPath()` | Returns first `.mpr` only |
| **Tree provider** | `projectTreeProvider.ts` | One `mprPath`, one `treeData[]` |
| **CLI** | `project_tree.go` | Accepts one `-p` flag, returns flat array |
| **LSP** | `lsp.go` | `mprPath string` on server struct |

## Design

### Principle: project as root node

Each `.mpr` project becomes a collapsible root node in the tree. Modules, domain models, and documents nest underneath their project. This mirrors Studio Pro's project explorer for multi-app solutions.

```
Mendix Projects
├── ProductCatalog (ProductCatalog.mpr)
│   ├── settings
│   ├── navigation
│   ├── project security
│   ├── catalog
│   │   ├── Domain model
│   │   ├── pages
│   │   └── microflows
│   └── ...
├── OrderService (OrderService.mpr)
│   ├── settings
│   ├── navigation
│   ├── project security
│   ├── Orders
│   │   ├── Domain model
│   │   └── microflows
│   └── ...
└── Fulfillment (Fulfillment.mpr)
    └── ...
```

When the workspace has only one `.mpr` file, the project root node is omitted and the tree looks exactly like today (no visual change for the common case).

### Changes by layer

#### 1. Project discovery (`extension.ts`)

Replace `findMprPath()` (returns first match) with `findAllMprPaths()` (returns all `.mpr` files):

```typescript
async function findAllMprPaths(): Promise<string[]> {
    const config = vscode.workspace.getConfiguration('mdl');
    const configured = config.get<string[]>('mprPaths', []);
    if (configured.length > 0) {
        return configured;
    }
    // Auto-discover: find all .mpr files, one level deep per workspace folder
    const files = await vscode.workspace.findFiles('*/*.mpr', '**/node_modules/**', 20);
    return files.map(f => f.fsPath);
}
```

Discovery looks for `<workspace>/<app-dir>/<name>.mpr` — one level deep matches the expected layout where each project has its own subdirectory.

#### 2. Setting (`package.json`)

Add a new array setting alongside the existing one (backward compatible):

```jsonc
"mdl.mprPaths": {
    "type": "array",
    "items": { "type": "string" },
    "default": [],
    "description": "Paths to Mendix .mpr files. if empty, auto-discovers in workspace."
}
```

The existing `mdl.mprPath` (singular) continues to work for single-project workspaces.

#### 3. Tree provider (`projectTreeProvider.ts`)

The tree provider manages multiple projects:

```typescript
interface ProjectTree {
    mprPath: string;
    name: string;        // derived from .mpr filename
    treeData: MendixTreeNode[];
}

class MendixProjectTreeProvider {
    private projects: ProjectTree[] = [];

    async refresh(): Promise<void> {
        const paths = await findAllMprPaths();
        this.projects = [];
        for (const mprPath of paths) {
            const treeData = await this.loadProjectTree(mprPath);
            this.projects.push({
                mprPath,
                name: path.basename(mprPath, '.mpr'),
                treeData,
            });
        }
        this._onDidChangeTreeData.fire(undefined);
    }

    getChildren(element?: MendixTreeNode): MendixTreeNode[] {
        if (!element) {
            // Root level
            if (this.projects.length === 1) {
                // single project: flat (same as today)
                return this.projects[0].treeData;
            }
            // multiple projects: project root nodes
            return this.projects.map(p => ({
                label: p.name,
                type: 'project',
                qualifiedName: p.mprPath,
                children: p.treeData,
            }));
        }
        return element.children ?? [];
    }
}
```

**Key property**: Each tree node gains an implicit project context through its ancestor project node. Commands look up the tree to find which project a node belongs to.

#### 4. Command context

Tree item click handlers (DESCRIBE, context menu commands) need the project path. Store it on the node:

```typescript
interface MendixTreeNode {
    label: string;
    type: string;
    qualifiedName?: string;
    children?: MendixTreeNode[];
    projectPath?: string;   // NEW: which .mpr this node belongs to
}
```

The `projectPath` is set during tree construction and passed as `-p` when invoking `mxcli` commands.

#### 5. CLI (`project_tree.go`)

No changes needed. The extension calls `mxcli project-tree -p <path>` once per project and assembles the combined tree client-side. This avoids coupling the CLI to multi-project concerns.

#### 6. LSP server (future, out of scope)

The LSP server currently binds to one project. For multi-project, the extension could spawn one LSP server per project (each with its own `-p` flag). This is a larger change and is deferred — the tree view works without LSP changes since it uses `mxcli project-tree` directly.

## Implementation Plan

### Phase 1: Multi-project tree view

1. Add `findAllMprPaths()` to `extension.ts`
2. Add `mdl.mprPaths` array setting to `package.json`
3. Refactor `MendixProjectTreeProvider` to hold `ProjectTree[]`
4. Add `projectPath` to `MendixTreeNode` interface
5. Update `getChildren()` for single-project-flat vs multi-project-nested
6. Update command handlers to read `projectPath` from tree node context
7. Add `project` type icon (`symbol-namespace`)

### Phase 2: Project-aware commands (future)

8. Context menu "Open in Terminal" scoped to project directory
9. DESCRIBE / SHOW commands pass correct `-p` per project
10. MDL script execution targets the right project

### Phase 3: Multi-project LSP (future)

11. Spawn separate LSP server per project
12. Route diagnostics/completions to correct server based on file location

### Phase 4: Cross-project catalog queries (future)

13. Use SQLite `ATTACH database` to mount each project's catalog under an alias
14. Enable cross-project queries: `select * from orders.catalog.entities join catalog.catalog.entities on ...`
15. `show callers of Module.Microflow ACROSS PROJECTS` for cross-project dependency analysis

SQLite natively supports attaching multiple databases to a single connection. Each project's catalog (built by `refresh catalog`) is a standalone `.db` file. By attaching them with project-scoped aliases, the existing `select ... from CATALOG.*` syntax extends naturally to cross-project joins without a new query engine.

```sql
-- Attach project catalogs
ATTACH 'orders/.mxcli/catalog.db' as orders;
ATTACH 'fulfillment/.mxcli/catalog.db' as fulfillment;

-- Cross-project query: which entities exist in both?
select o.name, f.name
from orders.entities o
join fulfillment.entities f on o.name = f.name;
```

### Phase 5: Cross-project operations (future)

16. MOVE entity/microflow between projects
17. Dependency visualization (which project calls which)
18. Shared module extraction

## Single-Project Backward Compatibility

When only one `.mpr` is found:
- Tree looks identical to today (no wrapping project node)
- `mdl.mprPath` (singular) still works
- All commands behave exactly as before
- No new UI elements shown

## Scope Exclusions

- **Multi-project LSP**: Out of scope — deferred to Phase 3
- **Cross-project references**: Out of scope — deferred to Phase 4
- **Shared module management**: Out of scope
- **Multi-project MDL scripts**: Out of scope (scripts already target one `-p` at a time)

## Effort Estimate

- Phase 1: Medium — ~200 lines TypeScript, no Go changes
- Phase 2: Small — command handler updates
- Phase 3: Medium — LSP spawning and routing
- Phase 4: Large — new CLI commands and BSON operations
