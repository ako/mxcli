# Playwright Testing

mxcli integrates with [playwright-cli](https://github.com/microsoft/playwright-cli) for automated UI testing of Mendix applications. This enables verification that generated pages render correctly, widgets appear in the DOM, and navigation works as expected.

## Why Browser Testing?

Mendix apps are React single-page applications. The server returns a JavaScript shell, and the actual UI is rendered client-side. An HTTP 200 from `/p/Customer_Overview` tells you nothing about whether the page's widgets actually rendered. A button defined in MDL might be missing from the DOM due to conditional visibility, incorrect container nesting, or a BSON serialization issue -- none of which are detectable without executing JavaScript in a real browser.

## Prerequisites

`mxcli init` provisions everything below inside the generated devcontainer, so in a normal project you do not install anything manually:

- **Node.js** (LTS) -- via the base image; required for playwright-cli.
- **playwright-cli** -- installed globally and **pinned** to a known-good version (`npm install -g @playwright/cli@0.1.15`). It is deliberately *not* `@latest`: the package is young and its CLI surface shifts between releases.
- **Chromium (headless shell)** -- installed via `@playwright/cli`'s **bundled** `playwright-core` into a shared `PLAYWRIGHT_BROWSERS_PATH`, and exposed at the stable path `/usr/local/bin/mx-headless-shell`. The generated `.playwright/cli.config.json` pins `executablePath` to that symlink.
- **Running Mendix app** -- start with `mxcli docker run -p app.mpr --wait`.

`mxcli init` also generates `.playwright/cli.config.json` (headless Chromium, timeouts, allowed origins) and sets `PLAYWRIGHT_CLI_SESSION=mendix-app` in the devcontainer so the runner and your scripts share one browser session.

### Manual / non-devcontainer setup (Linux arm64 gotchas)

If you provision outside `mxcli init` or debug a browser-launch failure:

- `playwright-cli install` **initializes the workspace** -- it does *not* install a browser (the browser command is `playwright-cli install-browser`).
- `open --browser` only accepts `chrome | firefox | webkit | msedge` (no `chromium`), and defaults to the **chrome channel**, which has **no distribution on Linux arm64**. Use the bundled Chromium instead and pin it explicitly:

  ```bash
  node "$(npm root -g)/@playwright/cli/node_modules/playwright-core/cli.js" \
    install chromium chromium-headless-shell
  ```

  Then point `.playwright/cli.config.json` at the headless-shell binary (headless mode needs the `chromium_headless_shell-*` build, not the full `chromium-*` one):

  ```json
  "browser": {
    "browserName": "chromium",
    "launchOptions": {
      "headless": true,
      "executablePath": "/usr/local/bin/mx-headless-shell"
    }
  }
  ```

## mxcli playwright verify

The `mxcli playwright verify` command runs `.test.sh` scripts against a running Mendix application and collects results.

```bash
# Run all test scripts in a directory
mxcli playwright verify tests/ -p app.mpr

# Run a specific script
mxcli playwright verify tests/verify-customers.test.sh

# List discovered scripts without executing
mxcli playwright verify tests/ --list

# Output JUnit XML for CI integration
mxcli playwright verify tests/ -p app.mpr --junit results.xml

# Verbose output (show script stdout/stderr)
mxcli playwright verify tests/ -p app.mpr --verbose

# Custom app URL (auto-detected from .docker/.env by default)
mxcli playwright verify tests/ --base-url http://localhost:9090

# Skip the app health check
mxcli playwright verify tests/ --skip-health-check
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--list`, `-l` | `false` | List test scripts without executing |
| `--junit`, `-j` | | Write JUnit XML results to file |
| `--verbose`, `-v` | `false` | Show script stdout/stderr during execution |
| `--color` | `false` | Use colored terminal output |
| `--timeout`, `-t` | `2m` | Timeout per script execution |
| `--base-url` | `http://localhost:8080` | Mendix app base URL |
| `--skip-health-check` | `false` | Skip app reachability check before running |
| `--keep-open` | `false` | Leave the browser session open after the run so the next verify reuses it |
| `-p` | | Path to the `.mpr` project file |

### How It Works

The verify runner performs these steps:

1. **Discovers** all `.test.sh` files in the provided paths
2. **Checks** that `playwright-cli` is available in PATH
3. **Health-checks** the app at the base URL (unless `--skip-health-check`)
4. **Reuses** a live browser session if one is already open on the same origin (left by a prior `--keep-open` run), re-navigating to the base URL so a rebuilt app loads fresh; otherwise **opens** a new one (Chromium by default)
5. **Runs** each `.test.sh` script sequentially via `bash`
6. **Captures** a screenshot on failure for debugging
7. **Closes** the browser session -- unless `--keep-open` was passed, in which case it is left warm
8. **Reports** pass/fail per script with timing
9. **Writes** JUnit XML if `--junit` is specified
10. **Exits** with non-zero status if any script failed

### Reusing the session across runs (the iterate loop)

The generate → build → verify → fix loop re-runs `verify` after each change.
Passing `--keep-open` leaves the browser warm and logged in, and the next run
detects that live session and **skips the ~1–2s Chromium cold start**:

```bash
# First run opens the browser (log in once inside a script or via state-save)
mxcli playwright verify tests/ -p app.mpr --keep-open

# ... edit MDL, mxcli exec, mxcli docker run ...

# Subsequent runs reuse the warm, still-authenticated session
mxcli playwright verify tests/ -p app.mpr --keep-open
```

Reuse only happens when a session is alive **and** on the target origin; if not,
verify opens fresh, so there is no downside to leaving `--keep-open` on. On reuse
the runner re-navigates to the base URL, so a rebuilt app is always loaded fresh
(no risk of verifying a stale, pre-rebuild page). For CI regression runs, omit
`--keep-open` so the browser is torn down at the end.

> **Note:** if a script ends with its own `playwright-cli close`, it tears down
> the shared session regardless of `--keep-open` — drop the trailing `close`
> from scripts you want to reuse across runs.

## `eval` vs `run-code` -- read this first

`@playwright/cli` has **two** evaluation commands with **different execution contexts**. Getting this wrong is the most common scripting mistake:

| Command | Runs in | Use for |
|---------|---------|---------|
| `playwright-cli eval "() => ..."` | **browser page** (`document`, `window` exist) | DOM assertions, clicks, filling fields, reading `.mx-name-*` |
| `playwright-cli run-code "..."` | **Node** (Playwright API; `document` is **undefined**) | Playwright-level scripting, not page DOM |

`eval` takes a **function** (`"() => ..."`) and prints its return value under `### Result`; it awaits a returned Promise. **Do not** use `run-code "document.querySelector(...)"` -- it throws `ReferenceError: document is not defined`. Every page/DOM assertion below uses `eval`.

## Test Script Format

Test scripts are plain bash files using playwright-cli commands. The naming convention is `tests/verify-<name>.test.sh`. Scripts should use `set -euo pipefail` so that any failing command causes the script to exit with a non-zero code.

### Example Test Script

```bash
#!/usr/bin/env bash
# tests/verify-customers.test.sh -- Customer module smoke test
set -euo pipefail

# --- Login (page context -> eval) ---
playwright-cli open http://localhost:8080
playwright-cli eval "() => { document.querySelector('#usernameInput').value = 'MxAdmin' }"
playwright-cli eval "() => { document.querySelector('#passwordInput').value = 'AdminPassword1!' }"
playwright-cli eval "() => document.querySelector('#loginButton').click()"
playwright-cli eval "() => new Promise(r => setTimeout(r, 3000))"
playwright-cli state-save mendix-auth

# --- Customer Overview page (throw to fail the script under set -e) ---
playwright-cli goto http://localhost:8080/p/Customer_Overview
playwright-cli eval "() => { if (!document.querySelector('.mx-name-dgCustomers')) throw new Error('dgCustomers not found') }"
playwright-cli eval "() => { if (!document.querySelector('.mx-name-btnNew')) throw new Error('btnNew not found') }"
playwright-cli eval "() => { if (!document.querySelector('.mx-name-btnEdit')) throw new Error('btnEdit not found') }"

# --- Create a customer ---
playwright-cli eval "() => document.querySelector('.mx-name-btnNew').click()"
playwright-cli eval "() => new Promise(r => setTimeout(r, 2000))"
playwright-cli fill txtName "CI Test Customer"
playwright-cli fill txtEmail "ci@test.com"
playwright-cli eval "() => document.querySelector('.mx-name-btnSave').click()"
playwright-cli eval "() => new Promise(r => setTimeout(r, 2000))"

# --- Verify persistence (via OQL, not direct DB query) ---
mxcli oql -p app.mpr --json \
  "SELECT Name FROM MyModule.Customer WHERE Name = 'CI Test Customer'" \
  | grep -q "CI Test Customer"

# --- Cleanup ---
playwright-cli close
echo "PASS: verify-customers"
```

## Widget Selectors

Mendix renders each widget's name property as a CSS class on the corresponding DOM element:

```html
<div class="mx-name-submitButton form-group">
```

These `.mx-name-*` classes are stable, predictable, and directly derived from MDL widget names. This makes them ideal for test assertions.

### Using Selectors in Scripts

The recommended approach for CI scripts is to use `eval` (page context) with CSS selectors, since dynamic element refs (`e12`, `e15`) change between page loads:

```bash
# Check widget presence
playwright-cli eval "() => document.querySelector('.mx-name-btnSave') !== null"

# Click a widget
playwright-cli eval "() => document.querySelector('.mx-name-btnSave').click()"

# Check text content
playwright-cli eval "() => document.querySelector('.mx-name-lblTitle').textContent.includes('Customers')"
```

For assertions that should cause failures, `throw` inside the function so the script exits non-zero under `set -e`:

```bash
playwright-cli eval "() => { if (!document.querySelector('.mx-name-btnSave')) throw new Error('btnSave not found') }"
```

## Three Test Layers

### Layer 1: Smoke Tests

Fast checks that pages are reachable and the app starts without errors. These run first as a gate before heavier tests.

```bash
# HTTP reachability
playwright-cli open http://localhost:8080
playwright-cli goto http://localhost:8080/p/Customer_Overview
playwright-cli goto http://localhost:8080/p/Customer_Edit
```

### Layer 2: UI Widget Tests

Verify that every widget generated in MDL is present and interactive in the DOM.

```bash
# Widget presence
playwright-cli eval "() => document.querySelector('.mx-name-dgCustomers') !== null"
playwright-cli eval "() => document.querySelector('.mx-name-btnNew') !== null"

# Form interaction (dispatch an input event so Mendix registers the change)
playwright-cli eval "() => { const el = document.querySelector('.mx-name-txtName input'); el.value = 'Test'; el.dispatchEvent(new Event('input', {bubbles: true})) }"
playwright-cli eval "() => document.querySelector('.mx-name-btnSave').click()"
```

### Layer 3: Data Assertions

Verify that UI interactions persist the correct data. Use `mxcli oql` for data validation instead of direct database queries.

```bash
# Submit a form via the UI, then verify persistence
mxcli oql -p app.mpr --json \
  "SELECT Name, Email FROM MyModule.Customer WHERE Name = 'Test'" \
  | grep -q "Test"
```

## Example Output

```
Found 3 test script(s)
Checking app at http://localhost:8080...
  App is reachable
Opening browser session (chromium)...
  [1/3] verify-login... PASS (1.2s)
  [2/3] verify-customers... PASS (3.4s)
  [3/3] verify-orders... FAIL (2.1s)
         Screenshot saved: verify-orders-failure.png
         Error: btnSubmit not found

Playwright Verify Results
============================================================
  PASS  verify-login (1.2s)
  PASS  verify-customers (3.4s)
  FAIL  verify-orders (2.1s)
         Error: btnSubmit not found
------------------------------------------------------------
Total: 3  Passed: 2  Failed: 1  Time: 6.7s
Some scripts failed.
```

## CI/CD Integration

For continuous integration, combine `mxcli docker run --wait` with `mxcli playwright verify`:

```bash
# Build and start the app
mxcli docker run -p app.mpr --wait

# Run all verification scripts
mxcli playwright verify tests/ -p app.mpr --junit results.xml

# JUnit XML output is consumed by CI systems (GitHub Actions, Jenkins, etc.)
```

## Related Pages

- [Testing](testing.md) -- MDL test framework (`mxcli test`)
- [Docker Run](docker-run.md) -- Building and running the Mendix app in Docker
- [Docker Check](docker-check.md) -- Validating projects without building
