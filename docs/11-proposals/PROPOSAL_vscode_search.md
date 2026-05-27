---
title: VS Code Search — Quick Pick + Workspace Symbol
status: proposed
---

# Proposal: VS Code Search — Quick Pick + Workspace Symbol

## Context

Full-text search exists in mxcli (`mxcli search`) but is only accessible via the terminal. The VS Code extension has no search UI. The LSP server's `Symbols()` method is a stub returning nil. This proposal adds both:

1. **Quick Pick full-text search** — command palette search that shows results as a clickable list
2. **Workspace Symbol (Ctrl+T)** — LSP `workspace/symbol` for fast element name lookup

## Files to Change

### 1. New CLI subcommand: `mxcli symbols` — `cmd/mxcli/main.go`

Add a `symbolsCmd` that queries the catalog `objects` view by name using SQL LIKE (fast — no FTS5 needed). Uses `ensureCatalog(false)` for minimal catalog build (~1-3s vs 5-20s for full).

```go
var symbolsCmd = &cobra.Command{
    use:   "symbols <query>",
    Short: "search project elements by name",
    Args:  cobra.ExactArgs(1),
    // connect, ensureCatalog(false), query objects where Name like '%query%', output json
}
```

Output format (always JSON):
```json
[{"name":"Customer","qualifiedName":"MyModule.Customer","objectType":"entity","moduleName":"MyModule"}]
```

The query:
```sql
select Name, QualifiedName, ObjectType, ModuleName from objects
where Name like '%query%' or QualifiedName like '%query%'
ORDER by ObjectType, QualifiedName limit 100
```

Add the executor method in a new file `mdl/executor/cmd_symbols.go` following the same pattern as `cmd_search.go`.

### 2. LSP Server: Implement `Symbols()` — `cmd/mxcli/lsp.go`

**Add capability** in `Initialize()` (line 218):
```go
WorkspaceSymbolProvider: true,
```

**Implement `Symbols()`** (replace stub at line 1368):
- Skip empty or 1-char queries (return nil)
- Check `lspCache` with key `"symbols:" + query` (TTL 30s)
- Shell out: `runMxcli(ctx, "symbols", query, "-q")`
- Parse JSON, convert to `[]protocol.SymbolInformation`
- Map objectType to `protocol.SymbolKind`:
  - ENTITY -> Class, MICROFLOW -> Function, NANOFLOW -> Event, PAGE -> File, ENUMERATION -> Enum, MODULE -> Module
- Set `Location.URI` to `mendix-mdl://describe/<type>/<qualifiedName>` (reuses existing virtual document system)
- Cache result

### 3. VS Code Extension: Quick Pick Search — `vscode-mdl/src/extension.ts`

Register `mendix.searchProject` command in `activate()`:

1. `vscode.window.showInputBox()` — prompt for search query
2. `findMprPath()` — get .mpr path (reuse existing helper at line 282)
3. `vscode.window.withProgress()` — show progress notification (cancellable, 30s timeout)
4. `cp.execFile(mxcliPath, ['-p', mprPath, 'search', query, '-q', '--format', 'json'])` — run search
5. Parse JSON, build Quick Pick items:
   - `label`: `$(icon) qualifiedName` (codicon based on objectType)
   - `description`: objectType lowercase
   - `detail`: match snippet (strip `>>>` / `<<<` markers)
6. `vscode.window.showQuickPick()` — display results
7. On pick: `vscode.commands.executeCommand('mendix.openElement', type, qualifiedName)` (reuses existing command at line 80)

Icon mapping (matches `projectTreeProvider.ts` patterns):
- ENTITY -> `symbol-class`, MICROFLOW -> `symbol-method`, PAGE -> `browser`, ENUMERATION -> `symbol-enum`, etc.

### 4. VS Code Extension: Register command — `vscode-mdl/package.json`

Add to `contributes.commands`:
```json
{ "command": "mendix.searchProject", "title": "Mendix: search project", "icon": "$(search)" }
```

Add to `contributes.keybindings`:
```json
{ "command": "mendix.searchProject", "key": "ctrl+alt+f", "mac": "cmd+alt+f" }
```

### 5. VS Code Extension: Rebuild — `vscode-mdl/`

Run `cd vscode-mdl && npm run compile` to compile TypeScript changes.

## Catalog Build Delay Handling

- **Quick Pick**: Uses `withProgress()` notification + 30s timeout + cancellable. First search may take 5-20s for catalog build.
- **Workspace Symbol**: Uses `ensureCatalog(false)` (fast mode, ~1-3s). 10s LSP timeout is sufficient. Returns empty on timeout. Cache avoids repeated builds.

## Verification

1. `make build` — build mxcli with new `symbols` subcommand
2. Test `symbols` CLI: `bin/mxcli symbols "Customer" -p path/to/app.mpr -q`
3. Test `search` JSON: `bin/mxcli search "validation" -p path/to/app.mpr -q --format json`
4. Rebuild VS Code extension: `cd vscode-mdl && npm run compile`
5. Open VS Code, open a workspace with `.mpr` file
6. Test Quick Pick: Ctrl+Alt+F -> type "Customer" -> verify results appear -> click to navigate
7. Test Workspace Symbol: Ctrl+T -> type "Customer" -> verify elements appear -> click to navigate
