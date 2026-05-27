---
title: MDL Language Server for VS Code and Claude Code
status: done
---

# Proposal: MDL Language Server for VS Code and Claude Code

## Goal

Enable developers to open, review, and edit `.mdl` files in VS Code with full IDE support — syntax highlighting, document outline, diagnostics, hover documentation, code completion, and go-to-definition. The Language Server also benefits Claude Code by providing real-time validation feedback during MDL generation.

## Architecture Question: mxcli vs. VS Code Extension

**Answer: Both, but mxcli does the heavy lifting.**

The LSP architecture has two sides:

| Component | Role | Implementation |
|-----------|------|----------------|
| **LSP Server** | Parses MDL, provides diagnostics, symbols, completions | `mxcli lsp` subcommand (Go binary) |
| **LSP Client** | Registers `.mdl` file type, launches server, renders UI | Thin VS Code extension (TypeScript) |

The VS Code extension is minimal — its only jobs are:

1. Register `.mdl` as a language (file icon, comment toggling, bracket matching)
2. Bundle a TextMate grammar for syntax highlighting
3. Launch `mxcli lsp --stdio` and connect via JSON-RPC
4. Provide extension settings (path to mxcli binary, project `.mpr` file)

All intelligence lives in `mxcli`. This means:

- Claude Code benefits automatically (it can run `mxcli check` already)
- Other editors (Neovim, Emacs, Zed) can use the same LSP server
- The Go codebase stays as the single source of truth for MDL semantics

## What Already Exists

The codebase has strong infrastructure to build on:

| Capability | Existing Code | LSP Feature It Powers |
|------------|---------------|----------------------|
| ANTLR4 lexer (600+ tokens) | `mdl/grammar/MDLLexer.g4` | Syntax highlighting, tokenization |
| ANTLR4 parser (150+ rules) | `mdl/grammar/MDLParser.g4` | Parse errors → diagnostics |
| Typed AST (64+ node types) | `mdl/ast/` | Document symbols, outline |
| `mxcli check` | `mdl/executor/validate_microflow.go` | Semantic diagnostics |
| `describe` commands | `cmd_pages_describe.go`, `cmd_microflows_show.go` | Hover documentation |
| Linter (4 rules) | `mdl/linter/` | Additional diagnostics |
| Catalog queries | `mdl/catalog/` | Workspace symbols, references |
| `show references to` | `mdl/executor/cmd_codesearch.go` | Find references |
| Cobra CLI | `cmd/mxcli/main.go` | Add `lsp` subcommand |

## Proposed Phases

### Phase 1 — Syntax Highlighting + Diagnostics (no project needed)

**TextMate Grammar** (`mdl.tmLanguage.json`)

Derived from `MDLLexer.g4`. Maps tokens to TextMate scopes:

```
Keywords (create, entity, begin, if, return)   → keyword.control.mdl
Types (string, integer, boolean)               → storage.type.mdl
widget names (textbox, dataview, actionbutton)  → entity.name.tag.mdl
string literals ('...')                         → string.quoted.single.mdl
Numbers (42, 3.14)                             → constant.numeric.mdl
Comments (// ... , /* ... */)                  → comment.mdl
variables ($Name)                              → variable.other.mdl
Qualified names (Module.Entity)                → entity.name.type.mdl
Operators (=, +, -, and, or)                   → keyword.operator.mdl
```

This gives immediate syntax coloring without the LSP server running.

**LSP Methods:**

| Method | Implementation |
|--------|----------------|
| `textDocument/didOpen` | Parse file with ANTLR, cache AST |
| `textDocument/didChange` | Re-parse, publish updated diagnostics |
| `textDocument/publishDiagnostics` | ANTLR parse errors + `mxcli check` semantic errors + linter violations |
| `textDocument/documentSymbol` | Walk AST, emit symbols for each statement |

**Document Symbols (Outline):**

```
create entity Module.Customer       → SymbolKind.Class
  Name : string(100)                → SymbolKind.Field
  Email : string(200)               → SymbolKind.Field

create microflow Module.ACT_Save    → SymbolKind.Function
  $Customer parameter               → SymbolKind.Variable
  if condition                      → SymbolKind.Struct
  return                            → SymbolKind.Event

create page Module.Customer_Edit    → SymbolKind.File
  dataview dvCustomer               → SymbolKind.Object
    textbox txtName                  → SymbolKind.Field
    actionbutton btnSave            → SymbolKind.Event
```

**Diagnostics Example:**

```
Line 5: Parse error: expected ';' after commit statement          (error)
Line 12: all code paths must end with return for non-void microflow (error)
Line 8: microflow 'ACT_Process' has no activities                  (warning, MDL002)
Line 3: entity name 'customer' should be PascalCase               (info, MDL001)
```

### Phase 2 — Hover + Go-to-Definition (project-aware)

When an `.mpr` file is configured, the LSP server opens it read-only and provides project-aware features.

**LSP Methods:**

| Method | Implementation |
|--------|----------------|
| `textDocument/hover` | Reuse `describe` output for the element under cursor |
| `textDocument/definition` | Resolve qualified name → file position or project element |
| `textDocument/references` | Reuse `show references to` via catalog |

**Hover Examples:**

Hovering over `MyModule.Customer` in a RETRIEVE statement:

```
entity: MyModule.Customer (persistent)
────────────────────────────
attributes:
  Name : string(100)
  Email : string(200)
  status : MyModule.CustomerStatus (enum)

associations:
  Customer_Order → MyModule.Order (1-*)
```

Hovering over `call microflow MyModule.ACT_Validate`:

```
microflow: MyModule.ACT_Validate
────────────────────────────
parameters:
  $Customer : MyModule.Customer
return: boolean

Activities: 3 | Complexity: 2
folder: ACT/validation
```

**Go-to-Definition:**

For references within the same MDL file, jump to the definition. For references to project elements, show the DESCRIBE output in a peek window (since the source lives in the `.mpr`, not in a text file).

### Phase 3 — Code Completion

**LSP Methods:**

| Method | Implementation |
|--------|----------------|
| `textDocument/completion` | Context-aware suggestions from grammar + catalog |
| `textDocument/signatureHelp` | Parameter hints for CALL MICROFLOW, SHOW PAGE |

**Completion Contexts:**

| Context | Trigger | Suggestions |
|---------|---------|-------------|
| Top-level | `create ` | `entity`, `microflow`, `page`, `association`, ... |
| After `create entity` | `Module.` | Existing module names |
| Attribute type | `: ` | `string`, `integer`, `boolean`, `decimal`, `datetime`, `Module.Entity` |
| Widget inside DATAVIEW | newline | `textbox`, `textarea`, `checkbox`, `actionbutton`, ... |
| After `attribute:` | `` | Attributes of the enclosing data source entity |
| After `datasource:` | `` | `DATABASE`, `$ParameterName`, `SELECTION`, `MICROFLOW` |
| After `call microflow` | `Module.` | Microflow names from catalog |
| After `retrieve ... from` | `` | Entity names from catalog |
| Inside expression | `$` | Declared variables in scope |

**Signature Help:**

```
call microflow MyModule.ACT_Validate(
                                     ▲
  $Customer: MyModule.Customer, $message: string
  ─────────────────────────────
  parameter 1 of 2
)
```

### Phase 4 — Formatting + Code Actions

| Method | Implementation |
|--------|----------------|
| `textDocument/formatting` | Consistent indentation, keyword casing, statement alignment |
| `textDocument/codeAction` | Quick fixes for diagnostics (e.g., add missing `;`, fix casing) |
| `textDocument/rename` | Rename entity/microflow across the file |

## VS Code Extension Structure

```
vscode-mdl/
├── package.json              # Extension manifest
├── language-configuration.json  # Brackets, comments, auto-closing
├── syntaxes/
│   └── mdl.tmLanguage.json   # TextMate grammar (from MDLLexer.g4)
├── src/
│   └── extension.ts          # Launch mxcli lsp, connect client
└── README.md
```

**`package.json` (key parts):**

```json
{
  "contributes": {
    "languages": [{
      "id": "mdl",
      "extensions": [".mdl"],
      "configuration": "./language-configuration.json"
    }],
    "grammars": [{
      "language": "mdl",
      "scopeName": "source.mdl",
      "path": "./syntaxes/mdl.tmLanguage.json"
    }],
    "configuration": {
      "properties": {
        "mdl.mxcliPath": {
          "type": "string",
          "default": "mxcli",
          "description": "path to mxcli binary"
        },
        "mdl.projectPath": {
          "type": "string",
          "description": "path to .mpr file for project-aware features"
        }
      }
    }
  }
}
```

**`extension.ts` (~30 lines):**

```typescript
import { LanguageClient, TransportKind } from 'vscode-languageclient/node';

export function activate(context) {
  const mxcliPath = workspace.getConfiguration('mdl').get('mxcliPath', 'mxcli');
  const client = new LanguageClient('mdl', 'MDL Language Server', {
    command: mxcliPath,
    args: ['lsp', '--stdio'],
    transport: TransportKind.stdio
  }, {
    documentSelector: [{ language: 'mdl' }]
  });
  client.start();
}
```

## Zero-Step Distribution via `mxcli init`

### Goal

Running `mxcli init` should configure everything — skills, commands, CLAUDE.md, **and** the VS Code extension. No additional steps for the user.

### How `mxcli init` Currently Works

The init command uses Go's `embed.FS` to bundle files into the binary at compile time, then extracts them during initialization:

```go
// skills_content.go
//go:embed skills/*.md
var skillsFS embed.FS

//go:embed commands/*.md
var commandsFS embed.FS

//go:embed lint-rules/*.star
var lintRulesFS embed.FS
```

During `mxcli init`, each embedded filesystem is walked with `fs.WalkDir()` and files are written to the project's `.claude/` directory. Currently creates:

```
<project>/
├── .claude/
│   ├── settings.json      # Claude Code permissions
│   ├── skills/            # 23 skill files (304 KB)
│   ├── commands/          # 9 command files (36 KB)
│   └── lint-rules/        # 10 lint rule files (40 KB)
└── CLAUDE.md              # project instructions
```

### Proposed Extension: Embed and Auto-Install the VS Code Extension

Add a fourth embedded filesystem for the pre-built `.vsix` file:

```go
// skills_content.go (addition)
//go:embed extensions/*
var extensionsFS embed.FS
```

**Build pipeline addition:**

```makefile
sync-extensions:
    @mkdir -p cmd/mxcli/extensions
    @cp .claude/extensions/*.vsix cmd/mxcli/extensions/ 2>/dev/null || true

sync-all: sync-skills sync-commands sync-lint-rules sync-extensions
```

**During `mxcli init`:**

1. Extract the `.vsix` to the project directory
2. Create `.vscode/extensions.json` recommending the extension
3. Create `.vscode/settings.json` with MDL/LSP configuration
4. Attempt auto-install via `code --install-extension` (best-effort)

```go
// in init.go (proposed addition)

// 1. Extract .vsix to project
vsixPath := filepath.Join(absDir, ".vscode", "mdl-extension.vsix")
os.MkdirAll(filepath.Join(absDir, ".vscode"), 0755)
vsixContent, _ := extensionsFS.ReadFile("extensions/vscode-mdl.vsix")
os.WriteFile(vsixPath, vsixContent, 0644)

// 2. write extensions.json (VS Code shows "install recommended" prompt)
extensionsJSON := `{
    "recommendations": ["mendix.mdl-language"]
}`
os.WriteFile(filepath.Join(absDir, ".vscode", "extensions.json"),
    []byte(extensionsJSON), 0644)

// 3. write VS Code settings for MDL
settingsPath := filepath.Join(absDir, ".vscode", "settings.json")
vscodeSettings := fmt.Sprintf(`{
    "mdl.mxcliPath": "./mxcli",
    "mdl.projectPath": "%s",
    "files.associations": { "*.mdl": "mdl" }
}`, mprFile)
os.WriteFile(settingsPath, []byte(vscodeSettings), 0644)

// 4. Best-effort auto-install (silent fail if `code` CLI not available)
exec.Command("code", "--install-extension", vsixPath).Run()
```

### Result After `mxcli init`

```
<project>/
├── .claude/
│   ├── settings.json
│   ├── skills/              # Claude Code skills
│   ├── commands/            # Claude Code commands
│   └── lint-rules/          # Linter rules
├── .vscode/
│   ├── extensions.json      # Recommends MDL extension
│   ├── settings.json        # MDL/LSP configuration
│   └── mdl-extension.vsix   # Bundled extension (for manual install)
├── CLAUDE.md
└── mxcli                    # The binary itself
```

### User Experience

**Best case** (VS Code CLI available):
1. User runs `mxcli init /path/to/project`
2. Extension auto-installs, `.mdl` files get syntax highlighting immediately
3. LSP server starts automatically when opening `.mdl` files

**Fallback** (VS Code CLI not available):
1. User runs `mxcli init /path/to/project`
2. User opens project in VS Code
3. VS Code shows "This workspace has extension recommendations" prompt (from `extensions.json`)
4. User clicks "Install All" — installs from the local `.vsix`, no marketplace needed

**Offline / air-gapped environments:**
- The `.vsix` is bundled in the binary — no internet required
- Works in corporate environments with restricted marketplace access

### Size Impact

| Component | Size | Notes |
|-----------|------|-------|
| Current mxcli binary | ~35 MB | Go binary with SQLite |
| Typical LSP extension .vsix | 200-500 KB | Thin client, no bundled dependencies |
| Binary size increase | < 1% | Go embed compresses well |

### Build Prerequisites

The VS Code extension must be pre-built before `make build`:

```bash
# One-time setup (extension development)
cd vscode-mdl && npm install && npm run package
# Produces vscode-mdl.vsix

# Copy to embed source
cp vscode-mdl/vscode-mdl.vsix .claude/extensions/

# build mxcli (syncs and embeds everything)
make build
```

This could be automated in the Makefile with a `build-extension` target, or the `.vsix` could be committed as a build artifact.

---

## mxcli LSP Server Implementation

Add `mxcli lsp` subcommand using a Go LSP library:

```
mxcli lsp --stdio                    # Default: stdin/stdout (for VS Code)
mxcli lsp --tcp :9257               # TCP mode (for debugging)
mxcli lsp --project /path/to/app.mpr # Pre-configure project
```

**Recommended Go LSP libraries:**

| Library | Pros | Cons |
|---------|------|------|
| `github.com/tliron/glsp` | Lightweight, clean API | Smaller community |
| `go.lsp.dev/protocol` | Standards-compliant, well-maintained | More boilerplate |
| Custom (JSON-RPC only) | Full control, minimal deps | More work |

**Server Package Layout:**

```
mdl/
├── lsp/
│   ├── server.go           # LSP server, method dispatch
│   ├── diagnostics.go      # Parse errors + check + lint → diagnostics
│   ├── symbols.go          # AST → document symbols
│   ├── hover.go            # describe commands → hover content
│   ├── completion.go       # Grammar + catalog → completions
│   └── document.go         # Document state management (open/change/close)
```

## What Works Without a Project

Phase 1 features work on standalone `.mdl` files with no `.mpr`:

- Syntax highlighting (TextMate, no server needed)
- Parse error diagnostics (ANTLR parser)
- Semantic checks (missing RETURN, scoping errors)
- Linter rules (naming conventions, empty microflows)
- Document outline (AST-based symbols)
- Keyword completion (grammar-based)

Project-aware features (hover with entity details, cross-reference navigation, catalog-based completion) require configuring an `.mpr` path.

## Summary

| Phase | Features | Dependencies |
|-------|----------|-------------|
| **Phase 1** | Syntax highlighting, diagnostics, outline | ANTLR parser, linter (exists) |
| **Phase 2** | Hover docs, go-to-definition, references | Executor describe commands, catalog (exists) |
| **Phase 3** | Code completion, signature help | Grammar rules, catalog (exists) |
| **Phase 4** | Formatting, code actions, rename | New code |

The foundation is strong — the ANTLR grammar, AST types, describe commands, linter, and catalog already exist. The main new work is the LSP protocol layer (JSON-RPC handling, document synchronization) and the thin VS Code extension.
