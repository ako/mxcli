---
title: Replace Generated Playwright Tests with playwright-cli
status: draft
---

# Proposal: Replace Generated Playwright Tests with playwright-cli

**Status**: Draft
**Date**: 2026-03-11

---

## Problem

The current approach (documented in `proposal-playwright-testing.md`) has Claude Code **generate TypeScript test files** (`.spec.ts`), then run them via `npx playwright test`. This works but has significant overhead:

1. **Token cost** — Claude spends tokens generating boilerplate TypeScript test files, playwright config, utility modules
2. **Indirection** — failures are reported as JSON, which Claude must parse and map back to MDL widget names
3. **Brittleness** — generated test files drift from the actual app state; they test what was *intended*, not what *is*
4. **Maintenance burden** — test files must be regenerated when the app changes
5. **Slow feedback** — full suite runs even when only one page changed

Microsoft's [`playwright-cli`](https://github.com/microsoft/playwright-cli) (`@playwright/cli`) solves this by giving the agent **direct browser control via CLI commands**. Instead of generating test files, Claude drives the browser interactively and observes results in real time.

---

## What is playwright-cli?

A CLI tool from the Playwright team, designed specifically for coding agents. Key properties:

- **Token-efficient** — CLI invocations instead of loading tool schemas and accessibility trees
- **Stateful sessions** — browser stays open across commands, cookies persist
- **Element references** — `snapshot` command returns element refs that subsequent commands use
- **Skills integration** — `playwright-cli install --skills` adds skill files for Claude Code
- **No test files needed** — the agent IS the test runner

### Core Commands

| Category | Commands |
|----------|----------|
| Navigation | `open [url]`, `goto <url>`, `go-back`, `go-forward`, `reload` |
| Interaction | `click <ref>`, `fill <ref> <text>`, `type <text>`, `select <ref> <val>`, `check/uncheck <ref>` |
| Inspection | `snapshot`, `screenshot`, `console`, `network` |
| State | `state-save`, `state-load`, `cookie-*`, `localstorage-*` |
| Sessions | `list`, `close`, `close-all`, `-s=name` for named sessions |
| Debug | `tracing-start/stop`, `video-start/stop`, `run-code <js>` |

### Example: Verifying a Mendix Page

```bash
# open the app
playwright-cli open http://localhost:8080 --headed

# login (if security enabled)
playwright-cli fill e12 "MxAdmin"          # username input
playwright-cli fill e15 "AdminPassword1!"  # password input
playwright-cli click e18                   # login button

# Navigate to page
playwright-cli goto http://localhost:8080/p/Customer_Overview

# Take snapshot to see element refs
playwright-cli snapshot

# Verify widgets exist by clicking/interacting
playwright-cli click e42                   # btnNew button
playwright-cli snapshot                    # see the edit form
playwright-cli fill e55 "Test Customer"    # txtName input
playwright-cli click e60                   # btnSave
```

Claude sees the results of each command directly — no JSON parsing, no test file generation.

---

## Current vs Proposed Architecture

### Current: Generated Test Files

```
Claude generates MDL
  → Claude generates .spec.ts files (tokens spent on boilerplate)
  → mxcli docker run --wait
  → npx playwright test --reporter=json
  → Claude reads json results (more tokens parsing structured output)
  → Claude maps failures back to MDL widgets
  → Fix → regenerate tests → rebuild → rerun
```

**Problems**: Two generation steps, stale test files, indirect feedback.

### Proposed: Direct Browser Control

```
Claude generates MDL
  → mxcli docker run --wait
  → playwright-cli open http://localhost:8080
  → Claude drives browser: snapshot, click, fill, verify
  → Claude sees results immediately
  → Fix MDL → rebuild → re-verify
```

**Benefits**: One generation step, real-time feedback, no test files to maintain.

---

## Integration with mxcli

### Changes to `mxcli init`

#### 1. devcontainer.json — Replace `@playwright/test` with `@playwright/cli`

**Current** (`tool_templates.go:258`):
```
npm install -D @playwright/test && npx playwright install --with-deps chromium
```

**Proposed**:
```
npm install -g @playwright/cli@latest && playwright-cli install --with-deps chromium
```

This installs playwright-cli globally and sets up the Chromium browser. The `--skills` flag is optional since we provide our own skill file.

#### 2. Add playwright-cli config file

Generate `.playwright/cli.config.json` during `mxcli init`:

```json
{
  "browser": {
    "browserName": "chromium",
    "isolated": true,
    "launchOptions": {
      "headless": true
    }
  },
  "timeouts": {
    "action": 10000,
    "navigation": 30000
  },
  "network": {
    "allowedOrigins": ["http://localhost:8080"]
  }
}
```

#### 3. Session environment variable

Set `PLAYWRIGHT_CLI_SESSION` in the devcontainer so Claude Code gets a dedicated browser session:

```json
{
  "containerEnv": {
    "PLAYWRIGHT_CLI_SESSION": "mendix-app"
  }
}
```

### Changes to Skills

#### Replace `test-app.md` Skill

The current skill (506 lines) focuses on generating TypeScript test files. Replace with a playwright-cli skill that teaches Claude to:

1. **Open the app** after `mxcli docker run --wait`
2. **Login** using Mendix login page selectors (when security is enabled)
3. **Save login state** for reuse across verification rounds
4. **Navigate to pages** and take snapshots
5. **Verify widgets** using `.mx-name-*` class selectors via `run-code` or snapshot inspection
6. **Fill forms and click buttons** to test interactions
7. **Query the database** via `mxcli oql` for data assertions (no `pg` npm package needed)

#### Key Skill Content

```markdown
## Verifying a Mendix App with playwright-cli

### Start the browser session
playwright-cli open http://localhost:8080

### login (security enabled)
playwright-cli snapshot                    # find login form refs
playwright-cli fill <username-ref> "MxAdmin"
playwright-cli fill <password-ref> "AdminPassword1!"
playwright-cli click <login-ref>
playwright-cli state-save mendix-auth      # reuse in future runs

### Verify a page
playwright-cli goto http://localhost:8080/p/Customer_Overview
playwright-cli snapshot

### check widget presence via javascript
playwright-cli run-code "document.querySelector('.mx-name-dgCustomers') !== null"

### Fill a form
playwright-cli goto http://localhost:8080/p/Customer_Edit
playwright-cli snapshot
playwright-cli fill <name-ref> "Test Customer"
playwright-cli click <save-ref>

### Verify data persistence (via mxcli oql, not pg client)
mxcli oql -p app.mpr "select Name from MyModule.Customer where Name = 'Test Customer'"
```

### Changes to Commands

Simplify the `/test` command:

```markdown
# Verify App

Verify the running Mendix app using playwright-cli.

## Quick Start
playwright-cli open http://localhost:8080
playwright-cli snapshot
# Interact with elements using refs from snapshot
```

### Claude Code Permissions

Add to `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": [
      "Bash(playwright-cli:*)"
    ]
  }
}
```

---

## What We Don't Build

| Thing | Why Not |
|-------|---------|
| `mxcli playwright` Go subcommand | playwright-cli already provides the CLI; wrapping it adds no value |
| Test file generator in Go | Claude Code generates contextual verifications interactively; static generators are too rigid |
| Custom result parser | Claude sees command output directly; no JSON parsing layer needed |
| Playwright config generator | A static `.playwright/cli.config.json` template in `mxcli init` is sufficient |

---

## Comparison: Generated Tests vs Interactive Verification vs CLI Scripts

| Aspect | Generated `.spec.ts` | playwright-cli (interactive) | playwright-cli scripts |
|--------|---------------------|-------------------------------|------------------------|
| Token cost | High (generate + parse) | Low (short commands) | Low (one-time generate) |
| Feedback latency | Batch (full suite) | Immediate (per-command) | Batch (script run) |
| Readability | TypeScript boilerplate | N/A (interactive) | Plain shell commands |
| CI/CD integration | `npx playwright test` | Not applicable | `bash tests/verify-customers.sh` |
| Maintenance | Files drift from app | No files | Same commands agent uses |
| Debugging | JSON error messages | Screenshot/snapshot | Screenshot/snapshot |
| Skill required | TypeScript + Playwright API | None | Basic shell scripting |

---

## CI/CD: Test Scripts Instead of TypeScript

The key insight: playwright-cli commands are already readable shell commands. A CI/CD regression test doesn't need TypeScript — it's just a shell script of the same commands Claude uses interactively. Developers can read, edit, and debug these scripts without knowing TypeScript or the Playwright API.

### Script Format: `.test.sh`

Test scripts are plain bash files using playwright-cli commands. Assertions use `run-code` to evaluate JavaScript expressions — a non-zero exit code means failure.

```bash
#!/usr/bin/env bash
# tests/verify-customers.sh — Customer module smoke test
set -euo pipefail

# --- Setup ---
playwright-cli open http://localhost:8080
playwright-cli fill e12 "MxAdmin"
playwright-cli fill e15 "AdminPassword1!"
playwright-cli click e18
playwright-cli state-save mendix-auth

# --- Customer Overview page ---
playwright-cli goto http://localhost:8080/p/Customer_Overview
playwright-cli run-code "document.querySelector('.mx-name-dgCustomers') !== null"
playwright-cli run-code "document.querySelector('.mx-name-btnNew') !== null"
playwright-cli run-code "document.querySelector('.mx-name-btnEdit') !== null"

# --- Create a customer ---
playwright-cli click btnNew   # open edit form
playwright-cli snapshot
playwright-cli fill txtName "CI Test Customer"
playwright-cli fill txtEmail "ci@test.com"
playwright-cli click btnSave

# --- Verify persistence ---
mxcli oql -p app.mpr --json "SELECT Name FROM MyModule.Customer WHERE Name = 'CI Test Customer'" \
  | grep -q "CI Test Customer"

# --- Cleanup ---
playwright-cli close
echo "PASS: verify-customers"
```

### Why This Works Better Than TypeScript

1. **Readable** — any developer can follow `playwright-cli fill txtName "Test"` without knowing Playwright's API
2. **Debuggable** — copy-paste individual lines into a terminal to reproduce failures
3. **No build step** — no `npm install`, no `tsc`, no `node_modules`
4. **Same language as the agent** — Claude uses the exact same commands interactively; capturing them as a script is trivial
5. **Easy to review in PRs** — shell commands are self-documenting, TypeScript test files are not

### Element References: Static vs Dynamic

playwright-cli uses dynamic element refs (e.g., `e12`, `e15`) from snapshots. These change between page loads. For CI scripts, use two stable alternatives:

**Option A: CSS selectors via `run-code`** (preferred for Mendix)
```bash
# Stable — uses .mx-name-* classes from MDL widget names
playwright-cli run-code "document.querySelector('.mx-name-btnSave').click()"
playwright-cli run-code "document.querySelector('.mx-name-txtName') !== null"
```

**Option B: `testIdAttribute` config**
```json
// .playwright/cli.config.json
{ "testIdAttribute": "class" }
```
Then use widget class names as refs directly. (Needs testing with Mendix's multi-class elements.)

**Option C: Snapshot + grep** (for login pages with known structure)
```bash
# Parse ref from snapshot output
REF=$(playwright-cli snapshot | grep -oP 'ref="(\w+)".*usernameInput' | head -1)
playwright-cli fill "$REF" "MxAdmin"
```

For Mendix apps, **Option A is recommended** — the `.mx-name-*` classes are stable, predictable, and directly derived from MDL widget names.

### Test Runner: `mxcli playwright verify`

A thin Go wrapper that runs test scripts and collects results:

```bash
# run all test scripts
mxcli playwright verify tests/ -p app.mpr

# run a specific script
mxcli playwright verify tests/verify-customers.sh -p app.mpr

# Output JUnit xml for CI
mxcli playwright verify tests/ -p app.mpr --junit results.xml
```

The runner:
1. Ensures the app is running (checks `http://localhost:8080`)
2. Opens a playwright-cli session
3. Runs each `.test.sh` script sequentially
4. Captures stdout/stderr and exit codes
5. Reports pass/fail per script
6. Takes a screenshot on failure for debugging
7. Closes the browser session
8. Exits with non-zero if any script failed

This follows the same pattern as `mxcli test` (the microflow test runner) — consistent UX across test types.

---

## Implementation Plan

### Phase 1: Core Integration

1. **Update `tool_templates.go`** — Replace `@playwright/test` with `@playwright/cli` in devcontainer template
2. **Add `.playwright/cli.config.json`** template to `mxcli init`
3. **Rewrite `test-app.md` skill** — playwright-cli commands instead of TypeScript generation
4. **Update `/test` command** — simplified workflow
5. **Add `PLAYWRIGHT_CLI_SESSION` env var** to devcontainer template
6. **Add `Bash(playwright-cli:*)` permission** to settings.json template

### Phase 2: Mendix-Specific Helpers

1. **Login state management** — skill documents `state-save`/`state-load` for Mendix auth
2. **Widget verification patterns** — `run-code` snippets for `.mx-name-*` checks
3. **OQL data assertions** — use existing `mxcli oql` instead of `pg` npm package
4. **Screenshot on failure** — skill documents `screenshot` for debugging

### Phase 3: CI/CD Test Runner

1. **`mxcli playwright verify`** — Go subcommand that runs `.test.sh` scripts against a running app
2. **Script conventions** — `tests/*.test.sh`, `set -euo pipefail`, exit code = pass/fail
3. **Screenshot on failure** — automatic screenshot capture when a script exits non-zero
4. **JUnit output** — `--junit results.xml` for CI integration
5. **Parallel execution** — optional `--workers N` flag for independent test scripts

### Phase 4: Test Generation from MDL (Optional)

Claude can generate `.test.sh` scripts from page definitions. Since the scripts are just shell commands, generation is simpler than TypeScript:

```bash
# Auto-generated from: create page MyModule.Customer_Overview
playwright-cli goto http://localhost:8080/p/Customer_Overview
playwright-cli run-code "document.querySelector('.mx-name-dgCustomers') !== null"
playwright-cli run-code "document.querySelector('.mx-name-btnNew') !== null"
playwright-cli run-code "document.querySelector('.mx-name-btnEdit') !== null"
playwright-cli run-code "document.querySelector('.mx-name-btnDelete') !== null"
```

This could also be implemented as `mxcli playwright generate -p app.mpr` — walk all pages, emit widget presence checks. But Claude Code generating them contextually is likely better since it can add interaction tests, not just presence checks.

---

## Open Questions

1. **playwright-cli maturity** — The tool is relatively new. Is it stable enough for production use? Need to evaluate version stability and breaking change risk.
2. **Headless in devcontainer** — Confirm that `playwright-cli` works headlessly in the devcontainer without X11/Xvfb.
3. **Snapshot format** — What does `playwright-cli snapshot` output look like? Can we reliably extract element refs from it in shell scripts?
4. **Session lifecycle** — How does the browser session interact with Docker container restarts (`mxcli docker run --fresh`)?
5. **Skills installation** — Does `playwright-cli install --skills` conflict with our custom skill files, or do they coexist?
6. **`run-code` exit codes** — Does `playwright-cli run-code` return non-zero when the expression evaluates to `false`? This is critical for `set -e` scripts. If not, we need a wrapper: `playwright-cli run-code "if (!document.querySelector('.mx-name-X')) throw new error('missing')"`.
7. **Login ref stability** — The Mendix login page (`/login.html`) is not generated from MDL, so its element refs may change across Mendix versions. Need a stable login approach (possibly `run-code` with `#usernameInput`/`#passwordInput` selectors).

---

## Files Changed

| File | Change |
|------|--------|
| `cmd/mxcli/tool_templates.go` | Update devcontainer template: replace `@playwright/test` with `@playwright/cli`, add session env var, add config file generation |
| `.claude/skills/mendix/test-app.md` | Rewrite for playwright-cli (interactive verification + script generation instead of TypeScript) |
| `.claude/commands/mendix/test.md` | Simplify for playwright-cli workflow |
| `cmd/mxcli/init.go` | Generate `.playwright/cli.config.json` during init |
| `cmd/mxcli/cmd_playwright.go` | New: `mxcli playwright verify` subcommand (Phase 3) |
| `reference/mendix-repl/templates/.claude/skills/test-app.md` | Source of truth for updated skill |

---

## Summary

playwright-cli eliminates the most expensive part of the current testing approach: **generating and maintaining TypeScript test files**. Instead of Claude spending tokens writing boilerplate `.spec.ts` files and then parsing JSON results, it directly drives the browser and sees results immediately.

For CI/CD, the same playwright-cli commands are captured as **plain shell scripts** (`.test.sh`) instead of TypeScript test files. These are:
- **Readable** — `playwright-cli fill txtName "Test"` vs `await page.locator('.mx-name-txtName input').fill('Test')`
- **Debuggable** — copy-paste any line into a terminal
- **Zero build step** — no `npm install`, no `tsc`, no `node_modules`
- **Same language** — identical commands whether Claude runs them interactively or CI runs them from a script

The integration is lightweight — mostly skill/template updates in `mxcli init`, plus a thin `mxcli playwright verify` runner for CI (Phase 3). The existing `mxcli oql` command handles data assertions, so the `pg` npm dependency is also eliminated.

The key insight: **the agent itself is the test runner during development, and the scripts it would generate for CI are just recordings of the same commands it already runs**.
