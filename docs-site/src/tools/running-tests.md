# Running Tests

The `mxcli test` command executes test files and reports results.

## Prerequisites

Tests need a Mendix runtime to execute against. There are two ways to get one,
and **Docker is only needed for the first**:

- **Docker** — the container path. Requires a running Docker daemon.
- **`--local`** — mxcli's own runtime, no daemon involved. It uses its own ports
  (8081/8091) and its own `<project>_test` database, so a `mxcli run --local`
  dev loop can keep serving the same project while tests run.

A `--local` run boots the app with the same **constant values** `mxcli run --local`
uses — the project configuration's shared overrides layered over each constant's
default — and prints what it applied. Use `--configuration <name>` to choose
between several, and `--constant Module.Name=value` (repeatable) to set one for
this run only — it wins over everything and is never written anywhere. For a
value that should persist on this machine without being committed, use
`mxcli constant set` (see [Constant values](#constant-values) below). A constant
the project does not declare is refused before anything boots. `--attach` takes
none of these, since it inherits the constants of the app it attaches to. This
matters whenever a test asserts on something a constant feeds: the two modes
used to disagree silently.

`--local` also downloads what it needs on first use. To pre-cache it:

```bash
mxcli setup mxbuild -p app.mpr
```

The `mx` binary, when you need it directly:

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| Repository | `reference/mxbuild/modeler/mx` |

## Constant values

Highest layer wins:

| Layer | Set with | In git? |
|---|---|---|
| this run | `--constant Module.Name=value` | no |
| this machine | `mxcli constant set Module.Name value` | no — gitignored, 0600 |
| this configuration | `alter settings constant … in configuration 'X'` | yes |
| default | `create constant … default '…'` | yes |

`mxcli constant list` shows the winner for each constant and which layer set it,
masking machine-local values unless `--show-values` is passed.

A machine-local value normally takes effect at the next boot. `mxcli constant
set … --apply` pushes it into a `mxcli run --local` that is already up, as
`update_configuration` followed by `reload_model` — both are needed, since the
first call only stages the change.

## Basic Usage

```bash
# Run all tests in a directory
mxcli test tests/ -p app.mpr

# Run a specific test file
mxcli test tests/sales.test.mdl -p app.mpr

# Run markdown test files
mxcli test tests/integration.test.md -p app.mpr
```

## Choosing a mode

|  | Boot cost per run | Database | Needs Docker |
|---|---|---|---|
| `mxcli test …` | container restart | the container's | yes |
| `--local` | ~30s | `<project>_test` | no |
| `--local --watch` | ~30s once, then ~2s | `<project>_test` | no |
| `--attach` | none | **the running app's** | no |

```bash
# No Docker daemon needed
mxcli test tests/ -p app.mpr --local

# Keep the runtime warm; re-runs on every test or model change (Ctrl-C to stop)
mxcli test tests/ -p app.mpr --local --watch

# Attach to an app you already have running — no boot at all
mxcli run  --local --test-endpoint -p app.mpr    # terminal 1
mxcli test tests/ -p app.mpr --attach            # terminal 2
```

`--watch` is the everyday loop: edit a test *or* the microflow under test, and
the verdict lands in about two seconds.

`--attach` skips even the first boot, at one cost worth knowing: the tests run
against the running app's database rather than a scratch one, so they can leave
data behind in the app you are looking at. It needs the dev loop to have been
started with `--test-endpoint`, because the endpoint's handler is registered by
the after-startup microflow and cannot be added to an app that is already up.

## How tests execute

**`--local` — the test endpoint.** One microflow is generated per test, plus a
Java action that registers a token-guarded HTTP endpoint. The app boots once;
startup only registers the endpoint and runs no tests. Each test is then invoked
by name over HTTP and returns its verdict in the response.

Two consequences when reading a failing run:

- A test that throws fails **only itself** and is reported as an error with the
  root-cause message; the next test still runs.
- Results are **returned**, not recovered from the runtime log.

The endpoint executes microflows under a system context, so it is gated: it is
not registered at all without a per-run token in the runtime's environment,
every request must present that token, non-loopback callers are refused, and it
will only ever invoke the generated `MxTest.Test_*` microflows. The token is
never written into your project.

### The app's own after-startup microflow

Boot registers the endpoint and then runs the project's own after-startup
microflow, so tests see the app in the state it really boots into. The run
prints which of the two happened, and `--skip-app-startup` opts out when a suite
wants an empty, deterministic baseline.

This keeps a suite behaving the same under `--local` and `--attach`. Note that
what the startup microflow writes is not covered by `@cleanup rollback` — it
runs at boot, outside any test's transaction.

### `@cleanup`: what happens to a test's data

`rollback` is the **default**, so a test's database writes do not survive it.
The endpoint opens a transaction around the call and rolls it back afterwards,
including when the test throws.

```mdl
/**
 * @test creating an order does not leak
 * @expect $result = 'ok'
 */
$result = CALL MICROFLOW Sales.CreateOrder(Amount = 100);
/

/**
 * @test seed a fixture the app should keep
 * @cleanup none
 */
$result = CALL MICROFLOW Sales.SeedCatalogue();
/
```

| Strategy | Effect |
|---|---|
| `rollback` (default) | The test's writes are rolled back, even if it throws |
| `none` | The writes commit and persist |

Rollback needs the test endpoint, so it applies to `--local` and `--attach`.
The Docker / `--legacy-runner` path runs tests inside the after-startup action
and has no context of its own to roll back, so it always commits.

A rollback that fails is reported per test and summarised at the end — data
left behind while the suite still says PASS is exactly what this is for.
`--verbose` tags every test `[rolled back]`, `[committed]` or
`[ROLLBACK FAILED]`. A misspelled strategy is a parse error, not a silent
commit.

**Docker — the after-startup runner.** The whole suite is compiled into the
project's after-startup microflow, the container is restarted, and results are
parsed out of its log. `--legacy-runner` selects this on a local run too.

Both paths restore the project when they finish, and both report loudly if that
restore fails — a modified project must never read as a clean pass.

## Isolated Testing Pattern

For manual testing or debugging, you can replicate the test runner's workflow:

```bash
# Create a fresh project
cd /tmp/test-workspace
~/.mxcli/mxbuild/*/modeler/mx create-project

# Apply MDL changes
mxcli exec script.mdl -p /tmp/test-workspace/App.mpr

# Validate
~/.mxcli/mxbuild/*/modeler/mx check /tmp/test-workspace/App.mpr
```

Expected output for a passing test:

```
Checking app for errors...
The app contains: 0 errors.
```

## Test Validation Checklist

Before marking tests as complete:

- MDL script executes without errors
- `mx check` passes with 0 errors
- Created elements appear correctly if inspected with `DESCRIBE`
- Security rules are valid (no CE0066 errors)

## Debugging Test Failures

### TypeCacheUnknownTypeException

```
The type cache does not contain a type with qualified name DomainModels$Index
```

This means a BSON `$Type` field uses the wrong name. Check the reflection data for the correct `storageName`.

### CE0066 "Entity access is out of date"

Security access rules don't match the entity's current structure. Make sure:
- All attributes in the entity are included in access rules
- Association member access is only on the FROM entity
- Run `GRANT` after adding new attributes

### Common Fixes

```bash
# Re-run with verbose output
mxcli exec script.mdl -p app.mpr --verbose

# Inspect the generated BSON
mxcli dump-bson -p app.mpr --doc "Module.EntityName"
```
