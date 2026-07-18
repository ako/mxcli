---
title: warm dev loop — Docker-free run and iPad split-screen preview
status: draft
date: 2026-07-17
---

# Proposal: Warm dev loop (`mxbuild --serve` + `reload_model`)

**Status:** Draft
**Date:** 2026-07-17
**Relates to:** `PROPOSAL_check_mxbuild_gap_heuristics.md` (the static-check gate that
runs *before* this loop ever builds), `.claude/skills/mendix/docker-workflow.md`
(the loop this replaces for inner-loop iteration).

## Problem Statement

The current way to run and test a Mendix app with mxcli is `mxcli docker build` /
`mxcli docker run`. It has two costs that dominate the edit→test loop:

1. **Full build every time.** The one-shot `mxbuild` CLI recompiles the whole
   deployment on each invocation. Measured on a one-entity/one-page/one-microflow
   app: **~30–60 s per build** (Java compile + full model export + package).
2. **Docker.** The runtime and database run in containers, adding image pulls and
   container lifecycle to every cycle — and Docker is **unavailable** in important
   contexts: Claude Code on the web, and iPad. In those environments the current
   loop cannot run at all.

Two primitives, both verified to exist and work in Mendix **11.6.3+**, make a far
tighter loop possible:

- **`mxbuild --serve`** — an HTTP build server (default port 6543) that keeps the
  model loaded and rebuilds **incrementally**, skipping unchanged work (including
  Java compilation when no Java changed). Each build returns `restartRequired`.
- **`reload_model`** — a runtime admin-port action that swaps the model into the
  **running** JVM in-place, draining in-flight actions first (near-zero-downtime).
  No process restart.

This proposal wires those two primitives into mxcli to serve **two scenarios**:

- **Scenario A — a fast, Docker-free `mxcli dev` loop** to replace `mxcli docker`
  for inner-loop iteration.
- **Scenario B — an iPad split-screen workflow**: Claude Code on the web in one
  pane, a browser preview in the other, prompt → build → test, all on the iPad,
  **without committing to the repo** before testing.

## End-to-end workflow: Claude design prototype → secured production Mendix app (on iPad)

Scenario B is one leg of a larger journey this loop is meant to unlock — taking an
idea all the way to a **secured, production-grade Mendix app entirely on an iPad**,
with no desktop Studio Pro in the loop:

1. **Prototype in Claude.** A Claude design Artifact (a clickable HTML/React mockup)
   captures the app's screens, data, and flows — fast and throwaway, no Mendix yet.
2. **Translate to Mendix in Claude Code (web).** In the same iPad session, mxcli/MDL
   turns the prototype into a real Mendix model: `create entity` / `create page` /
   `create microflow` / navigation. The prototype's structure becomes actual `.mpr`
   documents — a running Mendix app, not a mockup.
3. **Secure it for production.** Raise the app from prototype to production security
   and make it pass the real Mendix consistency rules — the exact path proven during
   the investigation behind this proposal:
   - `ALTER PROJECT SECURITY LEVEL PRODUCTION` — required before `DTAPMode=P` will
     even boot (otherwise `start` returns `result:11 — security must be
     CHECKEVERYTHING`).
   - `grant` / `revoke` page, entity, and microflow access per module role; define
     user roles and their mappings; wire the login and anonymous navigation profiles.
   - `mxcli check` (instant) plus the warm `mxbuild` build surface the
     production-only errors up front (e.g. *"page needs an allowed role"*, which we
     hit and fixed with a single `grant`) instead of discovering them at deploy time.
4. **Test each change live, on the iPad.** The warm loop (§Proposed CLI) previews
   every edit in ~1 s in the Safari pane, under **real production security** (login
   enforced, anonymous blocked) — so the author iterates prototype → model → secured
   app → test without ever leaving the tablet.

The deliverable is not a throwaway preview but a **secured, production-representative
Mendix app** — the same artifact you would deploy. The warm dev loop is what makes
step 4 fast enough for this to be a genuine authoring experience rather than a batch
process, and Scenario B is what makes it work on a device that has neither Docker nor
Studio Pro.

## BSON Structure

Not applicable — this is a build/runtime orchestration feature. It touches no
Mendix document serialization. It drives two already-existing interfaces: the
mxbuild HTTP build API and the runtime M2EE admin API.

## Measured evidence

All numbers below were measured directly (Mendix 11.6.3, Linux x86-64, JDK 21) on
a one-entity/one-page/one-microflow app during the investigation that motivated
this proposal.

| Step | Time | Notes |
|------|------|-------|
| `mxbuild` one-shot CLI (full) | **~30–60 s** | current loop; no incremental mode |
| `mxbuild --serve`, cold (first build) | ~13.7 s | loads model into the server |
| `mxbuild --serve`, warm, **no change** | ~1.1 s | model cached |
| `mxbuild --serve`, warm, **microflow change** | **~0.8 s** | Java **not** recompiled |
| `mxbuild --serve`, warm, **entity/attribute change** | ~7.3 s | `restartRequired: true` |
| `reload_model` (hot, no restart) | **~0.07–0.27 s** | `"reload": true`, JVM pid unchanged |
| **Full warm loop** (exec → serve build → reload) | **~1 s** | microflow/page change, no restart |

`restartRequired` from the serve API cleanly separates the two change classes:

- **page / microflow change** → `restartRequired: false` → `reload_model` (~0.1 s)
- **entity / domain change** → `restartRequired: true` → `execute_ddl_commands`
  + runtime restart (the DDL gate is real and unavoidable — the runtime refuses a
  stale schema)

`mxbuild --serve` and `reload_model` are present in both **11.6.3** and **11.12.1**
(verified). The 11.12 CLI adds no build-skip flags (only `--export-secrets`), so the
incremental path is `--serve`, not a new one-shot flag.

## Hot-reload scope: what `reload_model` can and cannot do (verified)

`reload_model` reloads exactly three subsystems — the model store
(`ProjectModelStoreLoader.load`), the microflow engine
(`MicroflowEngineModule.reload`), and translations (`I18NProcessor.reload`) — after
draining in-flight actions. It does **not** touch the datastorage / entity layer.

The runtime maintains a **metamodel catalog inside the app database** —
`mendixsystem$entity`, `mendixsystem$entityidentifier`, `mendixsystem$attribute`,
`mendixsystem$association` — and reconciles it **at startup**, not during a reload.
This was verified directly: a view entity added via `reload_model` (`TaskView`)
returned `Success` but **never appeared in `mendixsystem$entity`**, while the base
`Task` (present at the last startup) has its row (`entity_name` → `table_name` → a
stable identifier GUID). So any change that needs a catalog row — a new/changed
entity, **including OQL view entities**, and associations in
`mendixsystem$association` (e.g. a view entity referencing a persistent entity) —
requires a **restart**, because that is when the catalog is reconciled.

This makes the apply-decision **two-dimensional**: `restartRequired` (from the serve
build) and `get_ddl_commands` (from the runtime) are **independent** signals.

| Change class | `restartRequired` | `get_ddl_commands` | Apply via |
|---|---|---|---|
| microflow / page / text | `false` | 0 | `reload_model` (~0.1 s, hot) |
| **OQL view entity** | **`true`** | **0** | **restart** — catalog sync, **no DDL** |
| persistable entity / attribute | `true` | > 0 | `execute_ddl_commands` → restart |
| association (adds FK) | `true` | > 0 | `execute_ddl_commands` → restart |

The loop must branch on **both** signals: `restartRequired == false` → `reload_model`;
`restartRequired == true` → restart, running `execute_ddl_commands` **only if**
`get_ddl_commands > 0`. Treating "restart" as implying "DDL" is wrong — **OQL view
entities are the counterexample**: they need a restart (to write the
`mendixsystem$entity` catalog row) but no DDL (they are query-time, not materialized —
confirmed: no view/table for `TaskView` in Postgres).

**Caveat (not fully closed):** `reload_model` *returns* `Success` for a view-entity
change (it doesn't reject it), and I could not run the functional tiebreaker —
querying `TaskView` post-reload — because the standalone hand-boot does not register
the `preview_execute_oql` admin action (it needs the dev-preview flag `mxcli docker`
sets). The conclusion rests on the authoritative `restartRequired` signal plus the
catalog evidence, which agree. Enabling the dev-preview flag on the standalone boot
(so the agent can verify via OQL) is itself a small item worth doing — see Open
Questions.

## Proposed CLI

### Scenario A: `mxcli dev` — Docker-free warm run loop

A long-lived local dev supervisor. Boots (or reuses) the standalone runtime +
Postgres, starts `mxbuild --serve`, and on each model change runs
`serve build (Deploy)` → branch on `restartRequired` → `reload_model` or
DDL+restart.

```bash
# start a warm dev session (no Docker)
mxcli dev -p app.mpr
#   → downloads/caches mxbuild + runtime (once), starts Postgres,
#     boots the runtime, starts `mxbuild --serve`, prints the local URL,
#     and holds everything warm.

# apply a change and hot-reload (from another shell, or driven by the agent)
mxcli dev reload -p app.mpr          # serve-build + restartRequired-aware reload
mxcli dev exec change.mdl -p app.mpr # exec MDL, then reload in one step
mxcli dev status                     # runtime status, warm-build server health
mxcli dev stop
```

`mxcli dev` **subsumes the inner loop of `mxcli docker run`** while keeping
`mxcli docker` for produced-artifact / container-parity builds.

Optionally a `--watch` mode: watch the `.mpr` (v2 `mprcontents/`) for changes and
auto-run the reload cycle.

### Scenario B: `mxcli dev serve` + a preview container (iPad)

The iPad has two panes: **Claude Code on the web** (prompt + edit) and **Safari**
(the running app). The catch, established during investigation: the Claude Code web
sandbox is **egress-only and cannot expose a port** (all reverse tunnels are blocked
by a 443-only + TLS-intercepting egress policy). So the *running* app must live in a
container that has real public ingress — a **Codespace** (public port forwarding →
`https://<name>-8080.app.github.dev`) is the natural fit.

Two containers, coupled **without a commit** (the explicit requirement — preview WIP
before committing):

```
┌─────────────────────────┐         push built deployment          ┌──────────────────────────┐
│  DEV: Claude Code Web    │  ── (443, authenticated, no git) ──▶   │  PREVIEW: Codespace       │
│  edits app.mpr           │                                        │  runtime + mxbuild --serve │
│  mxbuild --serve (build) │                                        │  Postgres                  │
│                          │  ◀── agent verifies via public URL ──  │  :8080 forwarded PUBLIC    │
└─────────────────────────┘         (curl / Playwright)             └──────────────────────────┘
                                                                      Safari tab ▲ (iPad)
```

- **DEV** (Claude Code Web): prompt Claude → `mxcli` edits `app.mpr` →
  `mxbuild --serve` produces the Deploy artifact → `mxcli dev push` uploads it to the
  Preview container over 443.
- **PREVIEW** (Codespace): receives the artifact, runs `reload_model` (or DDL+restart
  on `restartRequired`), serves the public URL. Booted by a `devcontainer.json` +
  `postStart` script that reproduces the standalone-boot recipe.
- **iPad UX:** prompt in the Claude pane → ~1–2 s → refresh the Safari tab → see the
  change. Nothing is committed until the user is happy.

Proposed commands:

```bash
# in the Preview container (Codespace), started by the devcontainer:
mxcli dev serve -p app.mpr --intake-port 9000 --intake-secret $TOKEN
#   → boots runtime + Postgres + mxbuild --serve, forwards 8080 public,
#     exposes an authenticated /deploy intake for artifact pushes.

# in the DEV container (Claude Code Web):
mxcli dev push --to https://<name>-9000.app.github.dev --secret $TOKEN -p app.mpr
#   → mxbuild --serve (Deploy) locally, then upload the deployment delta;
#     the Preview container reloads and returns the live status.
```

### Coupling: options considered

| Option | Preview WIP w/o commit? | Notes |
|--------|-------------------------|-------|
| **A. Push built deployment artifact over 443** (recommended) | ✅ | robust; runtime in a legit host; ~1–2 s + network |
| B. Push MDL/model, build in the Preview container | ✅ | keeps a synced `.mpr` in the Preview container; heavier transfer for MPR v2 |
| C. Git branch, Codespace pulls | ❌ | requires a commit — **rejected**, violates the core requirement |
| D. Live tunnel (app stays in DEV, Codespace relays) | ✅ | instant, but fragile (chisel over two proxies), ties the app to the ephemeral sandbox, and Codespace-as-relay is an AUP gray area — **not recommended** |

## Implementation Plan

Reuse the existing `cmd/mxcli/docker/` plumbing wherever possible — it already
downloads/caches mxbuild + runtime and speaks the M2EE admin API.

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/dev.go` | New `mxcli dev` command tree (`dev`, `dev reload`, `dev exec`, `dev serve`, `dev push`, `dev status`, `dev stop`). |
| `cmd/mxcli/docker/mxserve.go` | New: `mxbuild --serve` client — start the daemon, `POST /build {target:Deploy}`, parse `status` + `restartRequired`. |
| `cmd/mxcli/docker/admin.go` | Extract the M2EE admin client (auth header `X-M2EE-Authentication: base64(pass)`, actions `update_appcontainer_configuration`, `update_configuration`, `start`, `execute_ddl_commands`, **`reload_model`**, `runtime_status`) out of `docker/oql.go` into a reusable client. |
| `cmd/mxcli/docker/localboot.go` | New: standalone runtime boot (see the config-set below), so `dev` can boot without Docker. |
| `cmd/mxcli/docker/download.go` | Reuse `DownloadMxBuild` / `DownloadRuntime` (no change). |
| `cmd/mxcli/docker/mxenv.go` | Reuse `PrepareMxCommand` (LD_PRELOAD FreeType fix) for the `mx`/`mxbuild` children (no change). |
| `.devcontainer/preview/devcontainer.json` + `boot.sh` | New: Codespace preview container — `forwardPorts: [8080]` public, Postgres, `postStart` → `mxcli dev serve`. |
| `.claude/skills/mendix/docker-workflow.md` | Add a "warm dev loop" section pointing at `mxcli dev`. |
| `cmd/mxcli/dev_test.go` | Tests for the serve client and the `restartRequired` branch logic (mock the serve/admin HTTP endpoints). |

### The standalone-boot config set (discovered, must be encoded)

The standalone runtime launcher (`com.mendix.container.boot.Main`, driven via the
admin API) requires a specific config that Studio Pro / the buildpack normally
supply. `mxcli dev` must set all of it or `start` fails:

- Env: `MX_INSTALL_PATH`, `M2EE_ADMIN_PASS`, `M2EE_ADMIN_PORT`, and the FreeType
  `LD_PRELOAD` on the JVM.
- Ensure the `data/{files,tmp,model-upload}` dirs exist under the deployment.
- `update_configuration`: `BasePath` (deployment dir), `RuntimePath`
  (`<install>/runtime`), `DatabaseType/Host/Name/UserName/Password`
  (`DatabaseHost` includes the port, e.g. `127.0.0.1:5432`), `DTAPMode`, and
  **`MicroflowConstants`** (design-time default values are *not* auto-applied
  standalone — missing a constant surfaces at runtime as HTTP 530 → login bounce).
- Boot sequence: `update_appcontainer_configuration` (runtime port) →
  `update_configuration` → `start` → on `"database has to be updated"`:
  `execute_ddl_commands` → `start`.
- Reload cycle (two independent signals — see § Hot-reload scope): `mxbuild --serve`
  Deploy build → if `restartRequired == false` then `reload_model`; else restart,
  running `execute_ddl_commands` first **only if** `get_ddl_commands > 0`. Do **not**
  couple "restart" to "DDL" — OQL view entities need a restart with zero DDL.

## Version Compatibility

- `mxbuild --serve` and `reload_model`: **≥ 11.6.3** (verified on 11.6.3 and
  11.12.1). No new flag needed on 11.12 — the incremental path is `--serve` in both.
- Register the feature in `sdk/versions/mendix-11.yaml` (and 10 if we verify the
  admin action name `reload_model` there) with the correct `min_version`, and add a
  `checkFeature()` pre-check with an actionable hint.
- **Production-representative testing note:** running `DTAPMode=P` requires the app's
  security level to be `CHECKEVERYTHING` (Production) or `start` refuses
  (`result:11`). `mxcli dev` should default to `D` for the inner loop and expose a
  `--dtap P` flag; when `P` is set, surface the security-level requirement up front.

## Test Plan

- `cmd/mxcli/docker/mxserve_test.go` — mock the serve HTTP endpoint; assert request
  shape (`{target:Deploy, projectFilePath}`) and `restartRequired` parsing.
- `cmd/mxcli/docker/admin_test.go` — mock the admin endpoint; assert the boot
  sequence and the `reload_model` vs DDL+restart branch.
- Integration (gated, needs mxbuild + runtime + Postgres, like the existing docker
  tests): boot a fixture app, do a microflow change → assert warm reload with pid
  unchanged; do an entity change → assert `restartRequired` → DDL path.
- `mdl-examples/` — a small fixture app the dev-loop tests drive.

## Open Questions

1. **Process lifecycle.** Long-lived `mxbuild --serve` and the runtime JVM need
   supervision (restart on crash, port cleanup). PID files under `.mxcli/`? A
   `dev status`/`dev stop`? (In the sandbox, background JVMs were reaped on idle —
   the Codespace/preview host must keep them alive; document the idle-stop caveat.)
2. **Artifact transfer for Scenario B.** Push the whole Deploy `model/` each time, or
   compute a delta? MPR v2 is a folder; the *deployment* `model/` is more compact,
   but still worth measuring. What's the auth model for the intake endpoint (bearer
   secret over the public forward)?
3. **Codespaces AUP.** Running your own app for preview is intended use; the
   Codespace-as-relay variant (Option D) is a gray area. The proposal recommends A
   (artifact push) precisely to stay clearly on the "previewing your own project"
   side. Confirm this framing.
4. **Warm-serve memory.** `mxbuild --serve` holds the model in memory; quantify for
   large apps and document guidance.
5. **`reload_model` scope (largely resolved — see § Hot-reload scope).** Verified:
   microflow changes hot-reload; entity/view-entity/association changes flip
   `restartRequired` because the runtime reconciles its DB metamodel catalog
   (`mendixsystem$entity` / `entityidentifier` / `attribute` / `association`) at
   startup, not on reload. Open sub-items: (a) enable the `preview_execute_oql`
   dev flag on the standalone boot so the loop can *functionally* verify a change via
   OQL (needed to close the view-entity caveat and to power agentic verification);
   (b) exercise styling-only changes (`update_styling` is a distinct admin action —
   likely a third hot path); (c) multi-unit and mixed changes.
6. **Relationship to `check`.** The instant `mxcli check` gate (see
   `PROPOSAL_check_mxbuild_gap_heuristics.md`) should run *before* every warm build so
   most errors never reach even the ~0.8 s build. `mxcli dev` should call it first.
