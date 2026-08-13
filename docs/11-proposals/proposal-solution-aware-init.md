---
title: Solution-aware `mxcli init` — one dev container for a multi-project solution
status: proposed
date: 2026-08-11
author: Generated with Claude Code
related:
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PROPOSAL_multi_project_tree.md
---

# Proposal: Solution-aware `mxcli init` — one dev container for a multi-project solution

**Status:** Proposed
**Date:** 2026-08-11

## Problem statement

A Mendix **solution** increasingly spans several projects — a frontend and a
backend joined by OData, or a small service landscape. Two sibling proposals
already assume that shape:

- [`PROPOSAL_mxcli_dev_warm_loop.md`](PROPOSAL_mxcli_dev_warm_loop.md) **slice 5**
  proposes `mxcli run --solution` + `mxcli.solution.yaml` to boot N apps in one
  container, and states the expected layout: *"projects live in subdirs of one
  repo, e.g. `apps/web/Web.mpr`"*.
- [`PROPOSAL_multi_project_tree.md`](PROPOSAL_multi_project_tree.md) makes the VS
  Code tree view multi-project, discovering `<workspace>/<app-dir>/<name>.mpr` —
  the same one-level-deep layout.

Both take that repo layout as a given. **Nothing creates it.** `mxcli init` and
`mxcli new` are single-project throughout, so the container, the session
bootstrap, and the agent context for a solution have to be hand-assembled — and
`init` actively fights the layout, because re-running it inside a second project
writes a second, competing dev container.

This is the missing prerequisite: slice 5 orchestrates the *runtimes*, the tree
view reads the *models*, and neither can happen until something produces a repo
whose container hosts every project at once.

### Why this bites now

Slice 5's premise is one container holding several apps. That is only reachable
if a single dev container is the workspace root. Today, initialising two projects
produces two dev containers in two sibling directories, and VS Code attaches to
one at a time — so the agent editing the frontend cannot see, build, or run the
backend it integrates with.

### Current state — where the single-project assumption lives

| Layer | File | Assumption |
|-------|------|-----------|
| **Project discovery** | `cmd/mxcli/init.go` `findMprFile()` | Returns the **first** `.mpr` in the directory, non-recursive |
| **Dev container** | `cmd/mxcli/init.go` (~L477) | `.devcontainer/` is written into the project dir — one per project |
| **Forwarded ports** | `cmd/mxcli/tool_templates.go` `generateDevcontainerJSON()` | `forwardPorts: [8080, 8090, 5432]` — one app's port triple |
| **Session bootstrap** | `cmd/mxcli/init_hook.go` `sessionStartHookCommand()` | Bakes one `-p <mprFile>`; marker-guarded on `"run --local --setup"`, so a second project's hook is **silently skipped** |
| **Agent context** | `cmd/mxcli/tool_templates.go` (`UniversalFiles`, `generateClaudeSettings`, …) | `CLAUDE.md` / `AGENTS.md` stamped with one `projectName` + `mprFile` |
| **CLI binary** | `cmd/mxcli/cmd_new.go` (step 5) | One `./mxcli` per project dir |

**Encouraging finding:** the dev container is *already* nearly solution-agnostic.
`generateDevcontainerJSON` and `generateDockerfile` both accept an `mprPath`
parameter and **never reference it** — the image (JDK 21, Node 22, PostgreSQL,
Playwright, Claude Code) is entirely generic, and the only project-specific field
in `devcontainer.json` is `"name"` (plus the `PLAYWRIGHT_CLI_SESSION` label). So
this proposal is mostly about *where* files are written and *what* the hook
covers — not about new container machinery.

## Non-goals — what this proposal does **not** cover

The user-facing ask that prompted this was "one dev container **and** one run
orchestrator". The orchestrator half is **already proposed** and is deliberately
excluded here:

| Concern | Where it lives |
|---------|----------------|
| `mxcli.solution.yaml` manifest, `mxcli run --solution`, port auto-allocation, per-app DB provisioning, registration under one `--hub-solution`, sibling-URL constant wiring | warm loop **slice 5** — do not duplicate |
| Multi-project tree view, per-project LSP, cross-project catalog queries | `PROPOSAL_multi_project_tree.md` |
| Per-preview sharing on the hub (external testers) | unproposed — see Open questions |

This proposal is the **repo-shape prerequisite** for both. It ends where slice 5
begins: once `mxcli init --solution` has produced the container and the manifest
skeleton, `run --solution` is what reads it.

## Proposed design

### Layout

Adopt the layout both sibling proposals already assume — projects one level deep,
solution-level tooling at the root:

```
shop-suite/                    # workspace root; the ONLY dev container
  .devcontainer/               # generic image + every project's ports
  .claude/                     # one agent context, sees all projects
  CLAUDE.md                    # solution-level, indexes the projects
  mxcli.solution.yaml          # skeleton; consumed by slice 5
  ./mxcli                      # one binary
  backend/
    Backend.mpr
    .claude/                   # per-project skills/commands (no .devcontainer)
    CLAUDE.md
  frontend/
    Frontend.mpr
    .claude/
    CLAUDE.md
```

Per-project `.claude/` and `CLAUDE.md` are **kept** — they carry project-specific
model context, and Claude Code composes nested context. What becomes
solution-scoped is exactly the container-shaped state: the dev container, the
session hook, the mxcli binary, and the manifest.

### `mxcli init --solution`

```bash
mxcli init --solution                     # discover *​/*.mpr from the root
mxcli init --solution -p backend/Backend.mpr -p frontend/Frontend.mpr
```

Behaviour:

1. **Discover** projects — each `.mpr` one level deep (mirroring
   `findAllMprPaths()` in the tree-view proposal, so discovery is one convention
   across the two features). Explicit repeated `-p` overrides discovery.
2. **Write one root `.devcontainer/`** whose `forwardPorts` covers every project's
   allocated triple, plus 5432 (see port assignment below).
3. **Write a root `CLAUDE.md`/`AGENTS.md`** that indexes the projects rather than
   describing one — "this is a solution of N apps; each has its own CLAUDE.md".
4. **Run the existing per-project init** in each project dir for skills, commands,
   and per-project context, but **suppress the dev container** there. This is the
   one behavioural change to the existing path, and it wants an explicit flag
   (`--no-devcontainer`) rather than an implicit mode, so the single-project path
   is untouched.
5. **Emit one SessionStart hook covering every project** (see below).
6. **Write a `mxcli.solution.yaml` skeleton** — the projects, a solution name, and
   commented-out placeholders for the inter-app constant wiring slice 5 defines.
   `init` writes the skeleton; slice 5 owns the schema and the consumer.

`mxcli new --solution <name>` is the greenfield companion, but is **deferred** —
`new` composes `create-project` + `theme` + `init` for one app, and multi-app
creation is a larger change with no user demand yet. `init --solution` on an
existing repo is the path that unblocks slice 5.

### Port assignment

Ports must agree between three artifacts: the dev container's `forwardPorts`,
whatever `run --solution` allocates, and any hand-run `run --local`. Slice 5
already specifies the allocation rule — *"auto-allocates the port triples
(`8080/8090/6543`, `8081/8091/6544`, …)"* — indexed by project order.

`init --solution` must therefore **use the same rule, not invent one**: forward
`8080+i / 8090+i / 6543+i` for each discovered project, plus 5432. The ordering
that defines `i` is the manifest's project order, which is why `init` writes the
manifest rather than leaving it to the user — the file is what makes the
assignment stable across `init` re-runs and `run --solution` boots.

`portsAttributes` already covers `8080-8099` and `5432-5499` silently, so only
the `forwardPorts` list changes.

### SessionStart hook for N projects

`sessionStartHookCommand` currently emits:

```sh
test -x ./mxcli && ./mxcli run --local --setup --ensure-db -p App.mpr || true
```

For a solution this must prepare **every** project's database and cache the shared
MxBuild/runtime once. Two options:

- **(a) One command per project**, chained — works today with zero new CLI surface:
  ```sh
  test -x ./mxcli && for p in backend/Backend.mpr frontend/Frontend.mpr; do \
    ./mxcli run --local --setup --ensure-db -p "$p"; done || true
  ```
- **(b) `run --solution --setup`** — one invocation reading the manifest. Cleaner,
  but it is slice 5 surface, so `init` would emit a command that does not exist yet.

**Recommendation: (a) now, (b) when slice 5 lands.** The marker constant
(`sessionStartHookMarker = "run --local --setup"`) still matches (a), so
idempotency and the "never clobber a user's hooks" guarantee are preserved
unchanged. When slice 5 ships, the marker moves to `--setup` alone and the hook
is rewritten in place.

Note the current failure mode this fixes: because the marker is a substring match,
running `init` in a second project today finds the first project's hook, concludes
"already present", and returns `changed=false` — the second app never bootstraps
and nothing says so.

## BSON structure

**Not applicable.** This feature touches no Mendix documents — it generates repo
scaffolding (`.devcontainer/`, `.claude/`, `CLAUDE.md`, `mxcli.solution.yaml`) and
reads `.mpr` files only to discover their paths and names. No parser or writer
changes, no `$Type` involved, so none of the storage-name hazards in CLAUDE.md
apply.

## Proposed MDL syntax

**None.** This is CLI surface (`mxcli init --solution`), not MDL. No grammar, AST,
visitor, or executor changes.

## Implementation plan

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/init.go` | `--solution` / `--no-devcontainer` flags; `findMprFile` → `findSolutionProjects()` (one level deep, repeatable `-p` override); skip the `.devcontainer/` write under `--no-devcontainer`; drive the per-project init loop |
| `cmd/mxcli/tool_templates.go` | `generateDevcontainerJSON` takes the project list → `forwardPorts` from the slice-5 allocation rule; drop the unused `mprPath` parameter while touching the signature |
| `cmd/mxcli/init_hook.go` | `sessionStartHookCommand` takes `[]string` of mpr paths and emits the loop form; marker unchanged |
| `cmd/mxcli/init_solution.go` *(new)* | Solution discovery, port allocation, `mxcli.solution.yaml` skeleton writer, root `CLAUDE.md` generator |
| `cmd/mxcli/init_hook_test.go` | Multi-project hook: every project appears; re-running is idempotent; an existing single-project hook is upgraded rather than duplicated |
| `cmd/mxcli/tool_templates_test.go` | `forwardPorts` covers N triples and matches the slice-5 rule |
| `cmd/mxcli/init_solution_test.go` *(new)* | Discovery (one level deep, ignores nested/`deployment/` copies); no `.devcontainer/` written into project dirs; manifest skeleton shape |
| `docs-site/src/tools/devcontainer.md` | Document the solution layout |
| `.claude/skills/mendix/run-local.md` | Document running two apps side by side: the per-app port triples, the per-project defaults that already do not collide, and the loopback rule below |

### Order of operations

1. `findSolutionProjects()` + port allocation, with tests — pure functions, no I/O.
2. `--no-devcontainer` on the existing single-project path (no behaviour change by
   default; makes step 4 possible).
3. Multi-project SessionStart hook + the silent-skip fix.
4. `--solution` orchestration: root container, root context, per-project init loop.
5. `mxcli.solution.yaml` skeleton — **schema agreed with slice 5 first**, since
   slice 5 owns the consumer.
6. Docs.

Steps 1–3 are independently useful and mergeable; the silent-skip fix in step 3 is
a bug fix that stands alone.

## Version compatibility

No Mendix version dependency — this generates repo scaffolding and never reads or
writes model content. No entry in `sdk/versions/mendix-{9,10,11}.yaml`, no
`checkFeature()` gate.

The generated dev container inherits whatever `mxcli run --local` supports; each
project may target a **different Mendix version**, since MxBuild is cached per
version under `~/.mxcli/mxbuild/{version}` and resolved per project. That is worth
an explicit test — a solution spanning two Mendix minors is a realistic migration
scenario and the shared container must not assume one version.

## Test plan

No `mdl-examples/` scripts — there is no MDL surface. Coverage is Go tests plus one
integration check:

| Test | Asserts |
|------|---------|
| `TestFindSolutionProjects` | One level deep; explicit `-p` overrides; `deployment/` and `node_modules/` ignored; deterministic order (the port allocation depends on it) |
| `TestSolutionPortAllocation` | Triples match slice 5's rule for N projects; `forwardPorts` contains every one plus 5432 |
| `TestSessionStartHook_MultiProject` | Every project appears; idempotent re-run; a pre-existing single-project hook is upgraded, not duplicated |
| `TestSessionStartHook_SecondProjectNotSilentlySkipped` | Regression for the marker substring-match bug |
| `TestInitSolution_NoNestedDevcontainers` | Exactly one `.devcontainer/` exists, at the root |
| `TestInitSolution_ManifestSkeleton` | Manifest lists every project and parses as the slice-5 schema |
| Integration (`-tags integration`) | `init --solution` on a two-project fixture, then `run --local` on each with the allocated ports, both reachable |

Per the "verified at the layer the symptom lives in" rule, the container itself is
only assertable as generated output here — whether the image *builds* and both apps
*boot* inside it is a dev-container-level check that cannot run in CI today. It
should be recorded as a manual verification step on the PR, not asserted by a unit
test that would only prove the template string.

## Open questions

1. **Does the manifest schema belong here or in slice 5?** This proposal writes a
   *skeleton*; slice 5 defines the fields and consumes it. If slice 5 is
   implemented first, `init --solution` should simply emit its schema. Needs a
   decision on ordering before step 5.
2. **Root `CLAUDE.md` vs per-project — how much duplication?** Nested context
   composes, but the current generated `CLAUDE.md` is substantial. An index at the
   root plus unchanged per-project files is proposed; whether the per-project files
   should shrink is unresolved.
3. **`mxcli new --solution`** — deferred. Worth it only if greenfield multi-app
   creation is a real workflow rather than a rare one.
4. **Existing single-project repos** — is there a migration path (`init --solution`
   detecting an existing project-level `.devcontainer/` and offering to hoist it),
   or is hand-editing acceptable for what should be a rare conversion?
5. **Hub previews for external testers.** Adjacent and currently unproposed: the
   hub's `authorizePreview` is owner-only, so a solution's previews cannot be shared
   with testers who are not the owner without dropping `--require-auth` for the
   whole hub. A per-preview allow-list is the obvious shape. Out of scope here, but
   it is the thing that makes a running solution *useful* to someone other than its
   author.

## Notes for slice 5

Two things learned while implementing the two-app workflow by hand, which slice 5's
constant wiring should absorb:

- **The primitive already exists.** [`PROPOSAL_constant_values.md`](PROPOSAL_constant_values.md)
  (accepted, shipped) defines one precedence chain — `--constant Module.Name=Value`
  over the machine store over the configuration over the deployment default — with
  the winning layer reported per constant, and `ApplyConstants` to change a value on
  a running app. Slice 5's sibling-URL wiring should emit `--constant` values into
  that chain rather than inventing a channel, and can rely on its refusal of names
  the project does not declare.
- **`--runtime-setting MicroflowConstants=…` is still unguarded.** It replaces the
  whole key, dropping every constant the deployment resolved, and the app then 530s
  at the first microflow reading one. The constant chain gives users no reason to
  reach for it, but nothing stops them; a refusal pointing at `--constant` would be
  a cheap guard.
- **Intra-container wiring must use loopback, not the public subdomain.** Slice 5
  offers both. On a hub with `--require-auth` the public URL is owner-gated, and a
  server-side OData call carries no session cookie — it receives an HTML login page
  where it expected `$metadata`. The manifest should make loopback the default for
  app-to-app links and reserve the subdomain for browser-facing values.
