---
title: GitHub Actions MDL Integration Tests
status: draft
date: 2026-03-16
---

# Proposal: GitHub Actions MDL Integration Tests

**Status:** Draft
**Date:** 2026-03-16
**Author:** AI-assisted design

---

## Summary

Add a GitHub Actions workflow that validates MDL example scripts against a real Mendix project after every merge to `main`. This catches parser regressions, executor bugs, and invalid BSON generation automatically.

The proposal is split into two slices:
1. **Slice 1** — Syntax check only (`mxcli check`), no Mendix tooling required
2. **Slice 2** — Full integration with `mx check` against a real Mendix project

## Motivation

The `mdl-examples/doctype-tests/` directory contains 15+ MDL scripts covering domain models, microflows, pages, security, navigation, and more. These scripts are the canonical reference for what the MDL parser and executor support.

Today, regressions in the parser or executor can go unnoticed until someone manually runs the scripts. A CI workflow catches these automatically:

| Failure mode | Caught by |
|---|---|
| ANTLR grammar regression | `mxcli check` (Slice 1) |
| Parser crash on valid MDL | `mxcli check` (Slice 1) |
| Executor crash | `mxcli exec` (Slice 2) |
| Invalid BSON generation | `mx check` (Slice 2) |
| Missing references / broken associations | `mx check` (Slice 2) |

## Slice 1: Syntax Validation (no Mendix tooling)

**Goal:** Verify all MDL example scripts parse without errors.

`mxcli check` validates syntax using the ANTLR4 parser. It requires no Mendix project, no mxbuild, and no JDK — just the compiled `mxcli` binary.

### Workflow

```yaml
# .github/workflows/mdl-check.yml
name: MDL Syntax check

on:
  push:
    branches: [main]
  pull_request:

jobs:
  mdl-syntax-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v4
        with:
          go-version: '1.26'

      - name: build mxcli
        run: make build

      - name: check MDL example scripts
        run: |
          FAILED=0
          for f in mdl-examples/doctype-tests/*.mdl; do
            [[ "$f" == *.test.mdl ]] && continue
            NAME=$(basename "$f")
            echo "::group::$NAME"
            if ./bin/mxcli check "$f"; then
              echo "PASS: $NAME"
            else
              echo "::error file=$f::Syntax check failed: $NAME"
              FAILED=1
            fi
            echo "::endgroup::"
          done
          exit $FAILED
```

### What this covers

- All 15 doctype-test `.mdl` files are parsed
- Skips `.test.mdl` files (these use test-specific annotations)
- Runs on both PRs and merges — catches regressions before merge
- No external dependencies beyond Go
- Adds ~30 seconds to CI

### What this does NOT cover

- Execution against a real project (no BSON validation)
- `mx check` validation (no Mendix tooling)
- Runtime behavior (no Docker/PostgreSQL)

## Slice 2: Full Integration with `mx check`

**Goal:** Execute MDL scripts against a blank Mendix project and validate with `mx check`.

### Prerequisites

| Dependency | Size | Source |
|---|---|---|
| mxbuild | ~500 MB | Mendix CDN (cached) |
| JDK 21 | ~200 MB | Eclipse Temurin (actions/setup-java) |
| Blank Mendix project | ~5 MB | Created via `mx create-project` |

### How it works

1. **Build** `mxcli`
2. **Download mxbuild** from Mendix CDN using `mxcli setup mxbuild --version $version`
3. **Create a blank project** using `mx create-project --app-name mdl-test`
4. **For each `.mdl` script:**
   - Copy the blank project to a temp directory (fresh state)
   - Execute the script with `mxcli exec script.mdl -p $project`
   - Validate with `mx check $project`
5. **Report** pass/fail per script with GitHub Actions annotations

### Workflow

```yaml
# .github/workflows/mdl-integration.yml
name: MDL Integration Tests

on:
  push:
    branches: [main]

jobs:
  mdl-integration:
    runs-on: ubuntu-latest
    env:
      MENDIX_VERSION: "11.6.3"

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v4
        with:
          go-version: '1.26'

      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: '21'

      - name: build mxcli
        run: make build

      - name: Cache mxbuild
        uses: actions/cache@v4
        with:
          path: ~/.mxcli/mxbuild
          key: mxbuild-${{ env.MENDIX_VERSION }}-amd64

      - name: Download mxbuild
        run: ./bin/mxcli setup mxbuild --version $MENDIX_VERSION

      - name: create blank Mendix project
        run: |
          mkdir -p /tmp/mdl-test-source
          cd /tmp/mdl-test-source
          ~/.mxcli/mxbuild/$MENDIX_VERSION/modeler/mx \
            create-project --app-name mdl-test

      - name: run MDL scripts and validate with mx check
        run: |
          PROJECT_DIR=/tmp/mdl-test-source
          MX=~/.mxcli/mxbuild/$MENDIX_VERSION/modeler/mx
          FAILED=0
          PASSED=0
          SKIPPED=0

          for f in mdl-examples/doctype-tests/*.mdl; do
            [[ "$f" == *.test.mdl ]] && continue
            NAME=$(basename "$f")
            echo "::group::$NAME"

            # Fresh copy for each script
            WORKDIR=$(mktemp -d)
            cp -r "$PROJECT_DIR"/* "$WORKDIR/"
            WORK_MPR="$WORKDIR/mdl-test.mpr"

            # execute MDL
            echo "Executing $NAME..."
            if ! ./bin/mxcli exec "$f" -p "$WORK_MPR" 2>&1; then
              echo "::error file=$f::exec failed: $NAME"
              FAILED=$((FAILED + 1))
              rm -rf "$WORKDIR"
              echo "::endgroup::"
              continue
            fi

            # Validate with mx check
            echo "Validating with mx check..."
            if ! "$MX" check "$WORK_MPR" 2>&1; then
              echo "::error file=$f::mx check failed: $NAME"
              FAILED=$((FAILED + 1))
            else
              echo "PASS: $NAME"
              PASSED=$((PASSED + 1))
            fi

            rm -rf "$WORKDIR"
            echo "::endgroup::"
          done

          echo ""
          echo "Results: $PASSED passed, $FAILED failed"
          exit $FAILED
```

### Runtime estimate

| Step | Duration |
|---|---|
| Build mxcli | ~60s |
| Download mxbuild (cold) | ~90s |
| Download mxbuild (cached) | ~5s |
| Create blank project | ~10s |
| Per MDL script (exec + mx check) | ~15-30s |
| **Total (15 scripts, cached)** | **~6-8 min** |

### Mendix version management

The `MENDIX_VERSION` env var pins which mxbuild to use. When upgrading:
1. Update the env var in the workflow
2. The cache key includes the version, so a new download is triggered automatically

An alternative is to store the version in a `.mendix-version` file at the repo root and read it in CI, making it easier to keep in sync with the test project.

## Scope of MDL files tested

| File | Domain |
|---|---|
| `01-domain-model-examples.mdl` | Entities, enumerations, associations, views, ALTER, generalization |
| `02-microflow-examples.mdl` | Microflows, activities, expressions, error handling |
| `03-page-examples.mdl` | Pages, widgets, layouts, data views |
| `04-math-examples.mdl` | Mathematical expressions and operations |
| `05-database-connection-examples.mdl` | External SQL connectivity |
| `06-rest-client-examples.mdl` | REST client operations |
| `07-java-action-examples.mdl` | Java action calls |
| `08-security-examples.mdl` | Module/user roles, access rules |
| `09-constant-examples.mdl` | Constants |
| `10-odata-examples.mdl` | OData services |
| `11-navigation-examples.mdl` | Navigation profiles, menus |
| `12-styling-examples.mdl` | Styling and theming |
| `13-business-events-examples.mdl` | Business event services |
| `14-project-settings-examples.mdl` | Project settings |
| `15-fragment-examples.mdl` | Page fragments/snippets |

Not all scripts may pass `mx check` immediately — some may use features that produce warnings vs errors. The workflow treats `mx check` exit code as the pass/fail signal.

## Implementation plan

1. **Slice 1 first** — add `mdl-check.yml` to `.github/workflows/`, merge, verify it passes on `main`
2. **Fix any syntax failures** uncovered by Slice 1
3. **Slice 2** — add `mdl-integration.yml`, initially with just `01-domain-model-examples.mdl` to validate the setup
4. **Expand Slice 2** — add remaining scripts one-by-one, fixing executor/BSON issues as found
5. **Optional: version file** — add `.mendix-version` to decouple from hardcoded env var

## Open questions

- **Which scripts to include in Slice 2?** Some scripts (e.g., `05-database-connection-examples.mdl`) may require external services. These could be excluded or run conditionally.
- **Failure tolerance:** Should `mx check` warnings be treated as failures? The `mx check` command distinguishes errors (non-zero exit) from warnings (zero exit with output).
- **Parallel execution:** Scripts are independent — could run in a matrix strategy for faster feedback, at the cost of more `mx create-project` calls.
