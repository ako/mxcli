---
title: Architecture Improvements for Agentic Development and Reduced Merge Conflicts
status: done
---

# Proposal: Architecture Improvements for Agentic Development and Reduced Merge Conflicts

## Problem Statement

Two related friction points have been identified:

1. **PR merge conflicts on grammar files** — Every MDL feature that adds syntax touches `MDLParser.g4` (4,082 lines) and triggers a full regeneration of `mdl/grammar/parser/mdl_parser.go` (119,737 lines). When two branches both run `make grammar`, the generated file produces an unresolvable conflict. Multiple recent PRs have been blocked or delayed by this.

2. **High agent cognitive load when adding features** — Adding a new MDL command requires coordinated edits across up to six layers (grammar, AST, visitor, executor, backend interface, backend implementation) with no structural guidance on where to look or what to change. Mistakes are caught only by the PR checklist, not by the compiler.

These are separate problems with separate fixes, but both stem from the same root: the codebase has grown to a point where the implicit conventions that work for a small human team become friction for parallel development — whether that parallel work comes from multiple contributors or from AI-assisted generation.

## Current Architecture (What We Have)

The backend abstraction in `mdl/backend/` is already a sound hexagonal boundary:

```
Grammar (.g4)  →  Generated Parser  →  Visitor (AST)  →  Executor  →  Backend Interface  →  MPR Implementation
                  [119k lines, git]                      [241 files]   [mdl/backend/]       [mdl/backend/mpr/]
                                                                        [mock/]
```

The backend layer is correct: executor handlers depend only on the interface, never on the MPR implementation directly. The existing PR checklist enforces this as a human review step.

The problems are:

- The generated parser is in git, so grammar changes always produce a massive conflict
- `MDLParser.g4` is monolithic — any two PRs adding syntax to different domains conflict on the same file
- The executor has 241 files with no structural guide for where to add a new command
- The mock (`mdl/backend/mock/`) is hand-written and must be kept in sync with the interface manually
- Boundary rules (e.g. "no sdk/mpr imports in executor") are enforced by checklist, not by the compiler

## Proposed Changes

### Change 1: Remove Generated Parser Files from Git ✅ Implemented

**Status:** Shipped. `mdl/grammar/parser/` is excluded via `.gitignore` (line 42–43). The directory is not tracked; contributors regenerate with `make grammar`.

**What was done:** Added `mdl/grammar/parser/` to `.gitignore`. The `.g4` source files remain in git; the generated Go parser is produced locally and in CI by `make grammar`. This eliminates the 119,737-line generated file as a source of merge conflicts entirely.

---

### Change 2: Split the Grammar Source by Domain ✅ Implemented

**Status:** Shipped. `MDLParser.g4` is now a thin master grammar; domain rules live in `mdl/grammar/domains/`.

**What was done:** The monolithic parser grammar was split into nine domain files imported via ANTLR4's `import` directive:

```
mdl/grammar/
  MDLParser.g4          # master grammar: import directives + top-level statement rule
  MDLLexer.g4           # unchanged — tokens are shared across domains
  domains/
    MDLAgent.g4
    MDLCatalog.g4
    MDLDomainModel.g4
    MDLMicroflow.g4
    MDLPage.g4
    MDLSecurity.g4
    MDLService.g4
    MDLSettings.g4
    MDLWorkflow.g4
```

Two PRs touching different domains now edit independent files and cannot conflict at the grammar source level. The generated parser is still a single file — the split is source-level only; all visitor/listener code is unchanged.

---

### Change 3: Command Self-Registration in the Executor ✅ Implemented

**Status:** Shipped. See `mdl/executor/registry.go` and `mdl/executor/register_stubs.go`.

**What was proposed:** Replace the central dispatch switch with a registration pattern using `init()` so each `cmd_*.go` file self-registers its handler, making a new command self-contained in one file.

**What was implemented:** The registry pattern without `init()`. `NewRegistry()` in `registry.go` is the single composition root that calls 29 named `registerXxxHandlers(r)` functions — one per domain. The `init()` approach was explicitly rejected because it creates package-level global state that breaks test isolation.

**Why `init()` was not used:** With `init()`-based registration, every import of the `executor` package pre-populates a global handler map before any test runs. This makes it impossible to create a registry with zero handlers for targeted isolation tests. The existing test suite depends on `emptyRegistry()` (a factory that returns a handler-free `*Registry`) for six tests covering dispatch, completeness, and duplicate-registration panics. `init()` also makes duplicate-registration panics surface at package-load time rather than at `NewRegistry()`, producing an obscure failure with no clear test attribution.

**How it works now:**

```go
// mdl/executor/registry.go — the composition root
func NewRegistry() *Registry {
    r := &Registry{handlers: make(map[reflect.Type]StmtHandler)}
    // Registration functions are called here explicitly (no init()).
    registerEntityHandlers(r)
    registerMicroflowAndNanoflowHandlers(r)
    // ... 27 more domain-specific register calls
    return r
}
```

```go
// mdl/executor/register_stubs.go — one function per domain
func registerEntityHandlers(r *Registry) {
    r.Register(&ast.CreateEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
        return execCreateEntity(ctx, stmt.(*ast.CreateEntityStmt))
    })
    // ...
}
```

**Adding a new command today:** Create `cmd_yourfeature.go` with the handler function, then add a `registerYourFeatureHandlers(r)` call in `NewRegistry()` and a corresponding stub function in `register_stubs.go`. The completeness test (`TestNewRegistry_Completeness`) will fail at CI if the registration step is missed.

**Agent discovery cost:** An agent can read `register_stubs.go` as the canonical index of all registered commands and their handler signatures. Any existing `registerXxxHandlers` function is a complete, copy-pasteable example.

---

### Change 4: Code-Generate the Backend Mock

**What:** Replace the hand-written `mdl/backend/mock/` with a generated mock produced by `mockgen` (or equivalent) from the backend interfaces.

**Why this helps:** Adding a new backend method currently requires four coordinated edits: interface definition, MPR implementation, mock stub, and compile-time check. The mock is the most error-prone because it must be kept in sync manually. An agent can easily miss adding the mock stub, causing compilation failures in unrelated tests. Code generation makes the mock always correct by construction.

**Current state:** The mock has 17 files for 26 sub-interfaces — the counts don't align, indicating the mock is already drifting from the interface.

**Implementation steps:**
1. Add `mockgen` (or `moq`) as a Go tool dependency in `go.mod`
2. Add a `//go:generate mockgen ...` directive to each backend interface file in `mdl/backend/`
3. Add `make mocks` target that runs `go generate ./mdl/backend/...`
4. Add generated mock files to `.gitignore` (same rationale as the parser: derived from source)
5. Add `make mocks` step to CI before `make test`
6. Delete the hand-written mock files in `mdl/backend/mock/`

---

### Change 5: Compiler-Enforced Backend Boundary

**What:** Move the `sdk/mpr` write types behind Go's `internal/` package visibility rules so that importing them from `mdl/executor/` is a compile error, not a checklist item.

**Why this helps:** The PR checklist currently has: *"No sdk/mpr write imports in executor."* This rule exists because executor handlers must go through the backend interface, not call the MPR writer directly. Today this is caught only in code review. An agent that bypasses the backend will produce code that compiles but violates the architecture — and the mistake ships unless a reviewer catches it.

**Proposed structure:**

```
sdk/
  mpr/
    internal/
      writer/     # write types moved here — only sdk/mpr can import
      parser/     # parser types moved here
    reader.go     # public read API (no change)
    writer.go     # delegates to internal/writer
```

Because Go's `internal/` rule allows only the parent package and its children to import, any attempt by `mdl/executor/` to import `sdk/mpr/internal/writer` will be a compile error.

**Implementation steps:**
1. Create `sdk/mpr/internal/writer/` and `sdk/mpr/internal/parser/`
2. Move write-path types into `internal/writer/`
3. Keep `sdk/mpr/writer.go` as the public facade (re-exports what `mdl/backend/mpr/` needs)
4. Verify `mdl/backend/mpr/` still compiles (it is a child of `sdk/mpr` — wait, it is not; it's under `mdl/`)

**Note:** This requires careful analysis. `mdl/backend/mpr/` is the legitimate consumer of the MPR write API, but it lives under `mdl/`, not `sdk/mpr/`. Go's `internal/` rule would block it too. An alternative is to enforce the boundary via a linting rule (`depguard` or `gomodguard`) rather than the compiler — flagging any import of specific `sdk/mpr` write symbols from `mdl/executor/` in CI.

---

### Change 6: Reduce Folder Navigability Cost in Large Source Directories

**What:** Several source folders have grown large enough that `ls` is a wall of text and a grep for a domain keyword matches dozens of files. Change 3 made the executor's *registration* discoverable but did not address the underlying file-count problem.

**Current state (largest flat folders, excluding generated/reference):**

| Folder | Files | Pattern |
|---|---|---|
| `mdl/executor/` | 261 (118 source + 143 test) | flat, `cmd_<domain>_*.go` |
| `sdk/mpr/` | 102 | flat, `writer_*` (52), `parser_*` (27), `reader_*` (5) |
| `mdl/visitor/` | 77 | flat, all `visitor_*` |
| `cmd/mxcli/tui/` | 62 | flat |
| `cmd/mxcli/` | 56 | flat |
| `mdl/ast/` | 35 | flat |

**Why this helps:** An agent starting a task in `mdl/executor/` cannot scan the folder structure to orient itself — there is no structure to scan. The naming convention `cmd_<domain>_<aspect>.go` is a *virtual* hierarchy that only pays off if you already know the domain word to grep for. First-touch tasks pay a full-folder-listing cost.

**Two approaches considered:**

1. **Split into subpackages by domain** (e.g. `mdl/executor/entities/`, `mdl/executor/pages/`, `mdl/executor/microflows/`). Real structural separation, but in Go each subdirectory is a separate package — anything currently shared between `cmd_*.go` files (helpers, types, the registry) would need to be exported and moved to a shared internal package. That's real churn, especially for the executor where handlers share `ExecContext`, dispatch helpers, and unexported parser/builder utilities. The same shared-state argument applies to `sdk/mpr/`, where `writer_*.go` and `parser_*.go` share private BSON types.

2. **Keep flat packages, add a per-folder index file** — a `_index.md` (or extend the existing `register_stubs.go`-style index) that maps symptom/domain → file. Cheap, no code movement, no exports churn. The naming convention `cmd_<domain>_<aspect>.go` is already a virtual hierarchy; an index just makes it greppable in one place instead of via `ls | grep`. Same playbook as the existing `.claude/skills/fix-issue.md` symptom table.

**Recommendation:**

- **Option 2 for `mdl/executor/`, `sdk/mpr/`, `mdl/visitor/`** — low effort, immediate agent benefit. Add a `_index.md` per folder; treat it as part of the PR checklist when adding new files to these folders. For the executor, `register_stubs.go` already serves part of this role and could be lightly extended.
- **Option 1 selectively for `cmd/mxcli/tui/` and `cmd/mxcli/`** where the files are more genuinely independent (separate CLI subcommands, separate TUI views) and don't share as much internal state.

**Implementation steps (option 2, executor first):**
1. Add `mdl/executor/_index.md` mapping every `cmd_*.go` to its domain, command keywords, and the corresponding `registerXxxHandlers` function
2. Add to the PR checklist: "If you added a `cmd_*.go` file in `mdl/executor/`, update `_index.md`"
3. Repeat for `sdk/mpr/_index.md` (writer/parser by document type) and `mdl/visitor/_index.md`
4. Re-evaluate `cmd/mxcli/` and `cmd/mxcli/tui/` for an option-1 split as a separate PR

---

## Summary and Priority

| Change | Problem solved | Risk | Effort |
|---|---|---|---|
| 1. Gitignore generated parser | Eliminates 119k-line conflict entirely | ✅ Done | — |
| 2. Split grammar by domain | Reduces source-level grammar conflicts | ✅ Done | — |
| 3. Command self-registration | Reduces agent discovery cost | ✅ Done — explicit `NewRegistry()`, no `init()` | — |
| 4. Code-generate mock | Eliminates mock drift, reduces sync errors | Low | Low |
| 5. Compiler-enforced boundary | Converts checklist rule to compile error | Medium (needs design) | Medium |
| 6. Folder navigability indexes | Reduces first-touch discovery cost in 260+/100+ file folders | Low | Low |

**Status:** Changes 1, 2, and 3 are shipped. Remaining work: Change 4 (code-generate mock), Change 5 (compiler-enforced boundary, needs design decision on the `internal/` approach), and Change 6 (per-folder `_index.md` for the largest source directories).
