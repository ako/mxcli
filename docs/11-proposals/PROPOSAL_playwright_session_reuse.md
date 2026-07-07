---
title: Playwright Session Reuse and Lifecycle Control
status: draft
---

# Proposal: Playwright Session Reuse and Lifecycle Control

**Status:** Draft
**Date:** 2026-07-07

Builds on [proposal-playwright-cli.md](proposal-playwright-cli.md), which
established the `playwright-cli` + `mxcli playwright verify` approach and noted
(but did not implement) "save login state for reuse across verification rounds."

## Problem Statement

`playwright-cli` maintains a **persistent browser session** — each CLI
invocation (`open`, `snapshot`, `click`, `eval`, `screenshot`) is a separate
process that attaches to a named session (`PLAYWRIGHT_CLI_SESSION=mendix-app`).
That shared session is what makes a stream of independent commands behave like
one coherent browser: state, cookies, auth, and page position survive between
calls. This is the foundation of the whole integration.

But `mxcli playwright verify` **owns and destroys** that session on every run —
it opens the browser at the start (`runner.go` step 1) and closes it at the end
(step, `runPlaywrightCLI("close")`). So the one workflow that benefits most from
a persistent session — Claude's **generate → build → verify → fix** loop —
throws it away each iteration:

- Every `mxcli playwright verify` **cold-launches** headless Chromium (~1–2s).
- Every run **re-logs-in** from scratch (or re-runs a login script).
- The browser is **torn down** the moment scripts finish, so a failing page is
  no longer inspectable and the next run starts cold again.

For an agent iterating on MDL and re-verifying after each change, this is the
dominant cost and directly negates the shared-session design.

Separately, the runner uses **one** shared session for **all** `.test.sh`
scripts. That is correct for agentic exploration (continuity) but wrong for CI
regression, where one test's leftover navigation / auth / open dialog can
contaminate the next. There is currently no way to isolate scripts.

## Goals

1. Let the browser session **persist and be reused** across `verify` runs so the
   agentic loop is warm and stays authenticated.
2. Give an explicit, named way to **manage the session lifecycle** across turns
   (open / status / close) instead of only implicitly through `verify`.
3. Offer **per-script isolation** for CI, where clean state per test matters more
   than speed.

## Non-Goals

- No change to the `.test.sh` script format or the `eval`/`run-code` semantics.
- No new MDL syntax, no BSON, no Mendix-version-gated behavior (this is mxcli
  tooling that works against any running app).
- Not a replacement for `state-save`/`state-load` — this complements them.

## BSON Structure

**Not applicable.** This proposal changes only the `mxcli playwright` runner and
CLI surface. It reads/writes no Mendix documents.

## Proposed CLI Changes

### 1. Session reuse on `verify` (the primary win)

Treat the `mendix-app` session as a durable resource the runner *attaches to*
rather than *owns*:

```bash
# Attach to a live, healthy session if one exists; otherwise open fresh.
# Leave it open at the end so the next run is warm and still logged in.
mxcli playwright verify tests/ -p app.mpr --keep-open

# CI: fresh session, torn down at the end (current behavior, now explicit)
mxcli playwright verify tests/ -p app.mpr --no-keep-open
```

Behavior:

- **Reuse (automatic):** before opening, probe for the session. If a healthy
  `mendix-app` session already points at the target base URL, **skip the open
  step** and reuse it. If none exists, it's dead, or it's on a different origin,
  open fresh (no regression vs today).
- **`--keep-open`:** skip the final `close`, leaving the browser warm for the
  next `verify` (or for interactive inspection of a failure).
- **Auto state-load (opt-in):** on a fresh open, if `--auth-state <name>` is
  given (or a saved `mendix-auth` state exists), `state-load` it so the run
  starts authenticated without a login script.

Health probe (cheap, page-context):

```bash
playwright-cli list                       # is `mendix-app` present?
playwright-cli eval "() => location.origin"   # alive? matches base URL origin?
```

### 2. Explicit lifecycle subcommands

Thin wrappers over `playwright-cli`, reusing the runner's existing base-URL /
browser / CWD resolution, so Claude can manage the browser deliberately:

```bash
mxcli playwright open [url] -p app.mpr    # launch/attach mendix-app, optional --auth-state
mxcli playwright status    -p app.mpr     # session live? which origin? logged in?
mxcli playwright close                    # tear down (--all for close-all)
```

`open` resolves the URL the same way `verify` does (explicit `--base-url`, else
`APP_PORT` from `.docker/.env`, else `:8080`) and the browser from
`.playwright/cli.config.json`. `status` is what an agent calls to decide whether
it needs to open or log in.

### 3. Per-script isolation for CI

```bash
mxcli playwright verify tests/ -p app.mpr --isolated --no-keep-open
```

Each `.test.sh` runs against its **own** short-lived session
(`PLAYWRIGHT_CLI_SESSION=verify-<script-basename>`, opened before and closed
after the script), so scripts cannot contaminate each other. Slower (a browser
per script) — documented as the CI/correctness-over-speed mode.

### Composition

| Context | Flags | Behavior |
|---|---|---|
| Local / Claude loop | `--keep-open` (reuse automatic) | one warm, logged-in browser reused across runs |
| CI regression | `--isolated --no-keep-open` | fresh context per script, all torn down at the end |
| Default (unchanged) | none | opens fresh, closes at end — today's behavior |

Reuse-attach is safe to enable by default (it only *skips a redundant open* when
a healthy matching session exists). `--keep-open` stays opt-in initially so the
default teardown behavior is unchanged; a follow-up can consider making it the
local default once proven.

## Implementation Plan

Localized to the `playwright` package and its Cobra command. No changes outside
`cmd/mxcli/`.

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/playwright/runner.go` | Add `sessionAlive(baseURL)` health probe (`list` + `eval "() => location.origin"`); in `Verify`, skip the open step when a healthy matching session exists; gate the final `close` on `!KeepOpen`; add `--isolated` path that sets a per-script `PLAYWRIGHT_CLI_SESSION` in `runScript`'s env and opens/closes around each script; optional `state-load` on fresh open. Extract `resolveBaseURL` / `sessionName` helpers |
| `cmd/mxcli/playwright/runner.go` (`VerifyOptions`) | New fields: `KeepOpen bool`, `Isolated bool`, `AuthState string` |
| `cmd/mxcli/cmd_playwright.go` | New flags on `verify`: `--keep-open`, `--no-keep-open`, `--isolated`, `--auth-state`. New subcommands `open`, `status`, `close` (with `--all`) wired to the shared resolution helpers |
| `cmd/mxcli/playwright/runner_test.go` | Unit tests for the reuse decision (fake `playwright-cli` on PATH recording args), per-script session-name derivation under `--isolated`, and that `--keep-open` omits the final `close` |
| `docs-site/src/tools/playwright.md` | Document reuse, the lifecycle subcommands, `--isolated`, and the composition table |
| `.claude/skills/mendix/test-app.md` | Add the agentic loop pattern (`playwright open` once → iterate `verify --keep-open`) and the CI `--isolated` pattern |
| `.claude/commands/mendix/test.md` | Mention the lifecycle subcommands and `--keep-open` for the iterate-and-re-verify loop |

### Order of operations

1. ✅ **Done** — `sessionAlive` probe (`eval "() => location.origin"` + origin
   match) + reuse-attach in `Verify` (skip redundant open). Lowest surface,
   immediate speedup, no behavior change to teardown.
2. ✅ **Done** — `--keep-open` (gate the final `close`) + `VerifyOptions.KeepOpen`
   + flag. (Steps 1–2 shipped together as "Add 1: session reuse on verify",
   with `docs-site` + command-doc updates.)
3. `open` / `status` / `close` subcommands (extract shared resolution helpers first).
4. `--isolated` per-script session wrapping.
5. `--auth-state` auto state-load.
6. Skill (`test-app.md`) agentic-loop pattern.

> **Implementation note (shipped):** the probe uses `eval "() => location.origin"`
> with a substring/origin match rather than parsing `list` output — it checks
> liveness and same-origin in one call and is tolerant of the CLI's exact output
> formatting. On reuse the runner **re-navigates** to the base URL (`goto`), so a
> rebuilt app is loaded fresh — otherwise the reused page keeps the pre-rebuild
> DOM/JS and the run would silently verify the stale build (the failure mode in
> the very loop this targets). `resolveBaseURL`/`sessionName` helper extraction
> was deferred to the subcommand step (3), where they're actually shared.

## Version Compatibility

**Not applicable to Mendix versions** — this is mxcli tooling behavior and works
against any running app. It depends only on `@playwright/cli`'s existing session
support (`list`, `-s=<name>`, `state-load`), which the integration already
requires; `@playwright/cli` is pinned to `0.1.15` in the generated devcontainer,
so verify the exact `list` / `eval` output shape against that pin.

## Test Plan

- **Unit** (`runner_test.go`, extends the existing fake-`playwright-cli`-on-PATH
  pattern): reuse decision (healthy match → no `open`; dead/mismatched → `open`);
  `--keep-open` omits the trailing `close`; `--isolated` derives a distinct
  session name per script and opens/closes around each.
- **Manual / e2e:** `mxcli playwright open` → `status` shows the session →
  `verify --keep-open` twice → confirm the second run reuses the warm session
  (no cold launch, still authenticated) and `status` reports the same session;
  then `verify --isolated` shows a browser per script and a clean end state.

## Open Questions

- **Default for `--keep-open`:** opt-in first (conservative), or make it the
  local default once proven? Reuse-*attach* is safe to default regardless.
- **Auth auto-load:** default to trying `mendix-auth` when present, or require
  `--auth-state`? Risk of applying stale auth silently.
- **"Same app" check:** origin match only, or also account for the port-offset
  interplay with `readAppPort` (`.docker/.env`)?
- **Isolation granularity:** a new session per script (simple, launch cost) vs a
  cheaper per-script context/state reset if `@playwright/cli 0.1.15` exposes one.
- **`--session <name>` override** on `verify` for parallel CI shards?
