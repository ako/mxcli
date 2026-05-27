---
title: Eval Framework for mxcli + Claude Code
status: partial
date: 2026-02-25
---

# Proposal: Eval Framework for mxcli + Claude Code

**Status:** Phase 1 Implemented
**Date:** 2026-02-25
**Author:** AI-assisted design

---

## Summary

An evaluation framework that systematically tests how well Claude Code + mxcli handles real-world Mendix app generation tasks. Given a set of test definitions (prompt, acceptance criteria, automated checks), the framework can validate a project against those criteria, score the results, and produce reports with improvement suggestions.

The framework supports **multi-turn evaluation**: after the initial prompt, an iteration scenario tests whether Claude can successfully modify the app it just created.

---

## Motivation

Claude Code can generate Mendix apps using mxcli and MDL, but we have no systematic way to:

1. **Measure quality** — Does the generated app meet the user's requirements?
2. **Track regression** — Did a skill/prompt/model change make things better or worse?
3. **Identify gaps** — What capabilities are missing from mxcli, MDL syntax, or skills?
4. **Compare models** — How does Sonnet vs Opus perform on the same tasks?
5. **Test iteration** — Can Claude successfully modify an app it just created?

Manual testing is slow, inconsistent, and not repeatable. We need an automated pipeline that evaluates end-to-end: from user prompt to working application.

---

## Architecture Overview

```
docs/14-eval/eval-1.md          (test definitions)
         │
         ▼
┌──────────────────────────────────────────────────┐
│  mxcli eval                                      │
│                                                  │
│  Phase 1 (implemented):                          │
│  ├── Parse test definitions (Markdown + YAML)    │
│  ├── run structural checks (L0)                  │
│  ├── run validation checks (L1: mx check)        │
│  ├── run lint checks (L2)                        │
│  ├── Score results (pass/fail per check)         │
│  └── generate reports (console, json, Markdown)  │
│                                                  │
│  Phase 2 (planned):                              │
│  ├── Copy template project for each test         │
│  ├── Invoke Claude Code CLI with prompt           │
│  ├── run Docker build/deploy (L3)                │
│  ├── execute iteration scenarios (multi-turn)    │
│  └── microflow tests via mxcli test (L4)         │
│                                                  │
│  Phase 3 (planned):                              │
│  ├── LLM-as-judge scoring                        │
│  ├── Playwright UI tests (L5)                    │
│  ├── Improvement suggestion generation           │
│  └── Trend tracking across runs                  │
└──────────────────────────────────────────────────┘
         │
         ▼
eval-results/                   (json + Markdown reports)
```

---

## Eval Test Definition Format

Tests are defined in Markdown with YAML frontmatter — human-readable for authoring, machine-parseable for automation.

```markdown
---
id: APP-001
category: App/Crud
tags: [entity, crud, pages, navigation]
timeout: 10m
---

# APP-001: Bookstore Inventory

## Prompt
create an app to manage my bookstore inventory. I need to track books
with title, author, ISBN, price, and stock quantity.

## Expected outcome
Domain model with Book entity, CRUD pages (overview, detail, edit),
navigation, and basic microflows for create/update/delete.

## Checks
- entity_exists: "*.Book"
- entity_has_attribute: "*.Book.Title string"
- entity_has_attribute: "*.Book.Author string"
- entity_has_attribute: "*.Book.ISBN string"
- entity_has_attribute: "*.Book.Price decimal"
- entity_has_attribute: "*.Book.StockQuantity integer"
- page_exists: "*overview*"
- page_exists: "*Edit*"
- navigation_has_item: true
- mx_check_passes: true

## Acceptance Criteria
- Book entity has all specified attributes with appropriate types
- overview page with data grid
- New/Edit page with form
- delete confirmation
- navigation menu item

## Iteration

### Prompt
add a category field to the books, and let me filter the book list
by category.

### Checks
- entity_has_attribute: "*.Book.Category"

### Acceptance Criteria
- Category attribute added to Book entity
- Book list can be filtered by category
```

### Format Details

| Section | Required | Description |
|---------|----------|-------------|
| YAML frontmatter | Yes | `id` (unique), `category`, `tags`, `timeout` |
| `## Prompt` | Yes | The user prompt given to Claude Code |
| `## Expected outcome` | No | Human-readable description of what should be built |
| `## Checks` | Yes | Machine-executable assertions (see check types below) |
| `## Acceptance Criteria` | No | Human-readable criteria (for LLM-as-judge in Phase 3) |
| `## Iteration` | No | Follow-up prompt with its own checks and criteria |

### Check Types

| Check | Args Pattern | Example | Description |
|-------|-------------|---------|-------------|
| `entity_exists` | `pattern` | `*.Book` | Entity matching pattern exists |
| `entity_has_attribute` | `Pattern.Attr [type]` | `*.Book.Title string` | Attribute exists, optionally with type |
| `page_exists` | `pattern` | `*overview*` | Page matching pattern exists |
| `page_has_widget` | `pattern widget` | `*overview* datagrid` | Page contains widget type |
| `microflow_exists` | `pattern` | `*.ACT_Create*` | Microflow matching pattern exists |
| `navigation_has_item` | `true` | `true` | Navigation menu is non-empty |
| `mx_check_passes` | `true` | `true` | `mx check` reports no errors |
| `lint_passes` | `true` | `true` | `mxcli lint` reports no errors |

**Pattern matching**: `*` is a wildcard. `*.Book` matches `MyModule.Book` or `Inventory.Book`. `*overview*` matches `Book_Overview`, `My_Overview_Page`, etc. Case-insensitive.

---

## CLI Interface

### `mxcli eval check` — Validate project against criteria

```bash
# Validate a single eval test against a project
mxcli eval check docs/14-eval/eval-1.md -p app.mpr

# Validate all tests in a directory
mxcli eval check docs/14-eval/ -p app.mpr

# run only a specific test
mxcli eval check docs/14-eval/ -p app.mpr --test APP-001

# Skip expensive mx check
mxcli eval check docs/14-eval/eval-1.md -p app.mpr --skip-mx-check

# write reports to a directory
mxcli eval check docs/14-eval/eval-1.md -p app.mpr --output eval-results/

# Colored output
mxcli eval check docs/14-eval/eval-1.md -p app.mpr --color
```

### `mxcli eval list` — List available tests

```bash
mxcli eval list docs/14-eval/

# Output:
# ID           Category          Checks  Iteration  title
# ----------------------------------------------------------------------
# APP-001      App/Crud              10   1 checks  APP-001: Bookstore Inventory
# 1 eval test(s) found.
```

### `mxcli eval run` (Phase 2) — Full automated pipeline

```bash
# Copy template, invoke Claude, validate, score
mxcli eval run docs/14-eval/ -p template.mpr

# use specific model
mxcli eval run docs/14-eval/ -p template.mpr --model sonnet

# run tests in parallel
mxcli eval run docs/14-eval/ -p template.mpr --parallel 3
```

---

## Validation Pipeline

Six validation layers, ordered from cheapest to most expensive. Each layer gates the next — if L1 fails, L3 is pointless.

| Layer | Name | Time | Requires | Phase |
|-------|------|------|----------|-------|
| L0 | Structure Inspection | ~2s | MPR file | 1 (done) |
| L1 | Mendix Validation | ~5s | MPR + mx binary | 1 (done) |
| L2 | Lint | ~2s | MPR file | 1 (done) |
| L3 | Runtime | 1-3m | Docker | 2 |
| L4 | Microflow Tests | 1-3m | Docker + .test.mdl | 2 |
| L5 | Playwright UI | 30s-2m | Docker + browser | 3 |

### L0: Structure Inspection (Phase 1 — implemented)

Runs `mxcli` commands against the MPR to verify structural expectations:

```
entity_exists  →  mxcli -c "show entities"  →  parse for matching name
entity_has_attribute  →  mxcli -c "describe entity X"  →  parse for attribute + type
page_exists  →  mxcli -c "show pages"  →  parse for matching name
navigation_has_item  →  mxcli -c "show navigation menu"  →  check non-empty
```

Results are cached per run (e.g., entity list fetched once, reused for all entity checks).

### L1: Mendix Validation (Phase 1 — implemented)

Runs `mx check app.mpr` and checks the exit code. This catches:
- Broken associations
- Missing attributes referenced in microflows
- Invalid widget configurations
- Type mismatches

### L2: Lint (Phase 1 — implemented)

Runs `mxcli lint -p app.mpr --format json` and checks for errors. Catches:
- Naming convention violations (MDL001)
- Empty microflows (MDL002)
- Domain model size issues (MDL003)
- Unconfigured widgets (MDL005, MDL006)

### L3: Runtime (Phase 2 — planned)

Runs `mxcli docker run -p app.mpr --wait` to build and deploy. Catches:
- Build failures (MxBuild errors)
- Runtime startup crashes
- After-startup microflow errors
- Database migration failures

### L4: Microflow Tests (Phase 2 — planned)

Uses existing `mxcli test` framework with `.test.mdl` files:

```bash
mxcli test tests/ -p app.mpr --junit results.xml
```

Test files can be generated alongside the eval definition or auto-generated from the acceptance criteria.

### L5: Playwright UI Tests (Phase 3 — planned)

Browser-based assertions using `.mx-name-*` CSS selectors:
- Widget presence (every generated widget renders in the DOM)
- Form interactions (fill inputs, click buttons)
- Navigation flows (page transitions work)
- No JavaScript errors

See `docs/11-proposals/proposal-playwright-testing.md` for the full Playwright design.

---

## Scoring System

### Per-Check Scoring

Each check produces a binary pass/fail:

```
score = checks_passed / checks_total
```

Scores are computed per phase (initial, iteration) and overall.

### Console Output

```
Eval: APP-001 (App/Crud) — Bookstore Inventory
============================================================
  Initial:
    [PASS] entity_exists *.Book — found: Bookstore.Book
    [PASS] entity_has_attribute *.Book.Title string — found: Bookstore.Book.Title (string)
    [FAIL] entity_has_attribute *.Book.ISBN string — attribute "ISBN" not found in Bookstore.Book
    [PASS] page_exists *overview* — found: Bookstore.Book_Overview
    [PASS] mx_check_passes true — mx check passed
    Score: 8/10 (80%)
  Iteration:
    [PASS] entity_has_attribute *.Book.Category — found
    Score: 1/1 (100%)
------------------------------------------------------------
  Overall: 9/11 (82%)
```

### JSON Report (`eval-results/<run>/APP-001/score.json`)

```json
{
  "test_id": "APP-001",
  "category": "App/Crud",
  "title": "APP-001: Bookstore Inventory",
  "timestamp": "2026-02-25T10:30:00Z",
  "duration": 7500000000,
  "initial": {
    "phase": "initial",
    "checks": [
      { "check": { "type": "entity_exists", "args": "*.Book" }, "passed": true, "detail": "found: Bookstore.Book" },
      { "check": { "type": "entity_has_attribute", "args": "*.Book.ISBN string" }, "passed": false, "detail": "attribute not found" }
    ],
    "passed": 8, "total": 10, "score": 0.8
  },
  "iteration": {
    "phase": "iteration",
    "checks": [
      { "check": { "type": "entity_has_attribute", "args": "*.Book.Category" }, "passed": true }
    ],
    "passed": 1, "total": 1, "score": 1.0
  },
  "overall_score": 0.82,
  "criteria": [
    "Book entity has all specified attributes with appropriate types",
    "overview page with data grid",
    "New/Edit page with form",
    "delete confirmation",
    "navigation menu item"
  ]
}
```

### Markdown Report (`eval-results/<run>/summary.md`)

Generated automatically with a summary table and detailed per-test results:

```markdown
# Eval run 2026-02-25 10:30

Tests: 5 | Duration: 23m | average Score: 78%

| Test | Category | Score | Checks | Iteration |
|------|----------|-------|--------|-----------|
| APP-001 | App/Crud | 82% | 9/11 | 100% |
| APP-002 | App/workflow | 70% | 7/10 | 60% |
```

---

## Phase 1: Implemented

Phase 1 delivers the foundation: test definition parsing, structural validation, and reporting.

### What Was Built

| Component | File | Description |
|-----------|------|-------------|
| Parser | `cmd/mxcli/evalrunner/parser.go` | Markdown + YAML frontmatter parser |
| Check Execution | `cmd/mxcli/evalrunner/checks.go` | 8 check types, wildcard pattern matching, output caching |
| Result Types | `cmd/mxcli/evalrunner/results.go` | Scoring logic, phase/overall score computation |
| Reporting | `cmd/mxcli/evalrunner/report.go` | Console, JSON, and Markdown report generation |
| CLI Commands | `cmd/mxcli/cmd_eval.go` | `eval check` and `eval list` Cobra commands |
| Tests | `cmd/mxcli/evalrunner/parser_test.go` | Parser, pattern matching, attribute detection tests |
| Test Definition | `docs/14-eval/eval-1.md` | First eval test (APP-001: Bookstore Inventory) |

### How Checks Work

The eval runner shells out to the `mxcli` binary for each check type. This keeps the eval framework decoupled from the executor internals and makes it testable against any mxcli version.

```
RunChecks()
  ├── Pre-fetch: show entities, show pages, show microflows, show navigation menu
  ├── for each check:
  │   ├── entity_exists → search entity list for pattern match
  │   ├── entity_has_attribute → resolve entity, describe it, search for attribute
  │   ├── page_exists → search page list for pattern match
  │   ├── page_has_widget → resolve page, describe it, search for widget type
  │   ├── microflow_exists → search microflow list for pattern match
  │   ├── navigation_has_item → check navigation menu is non-empty
  │   ├── mx_check_passes → run mx check, check exit code
  │   └── lint_passes → run mxcli lint --format json, check for errors
  └── return CheckResult[] with pass/fail + detail per check
```

Pattern matching supports `*` wildcards: `*.Book` matches any module's `Book` entity. DESCRIBE output parsing handles comma-separated attributes, type annotations like `string(200)`, and case-insensitive matching.

### Usage

```bash
# list available eval tests
mxcli eval list docs/14-eval/

# Validate a project (e.g., after Claude generated an app)
mxcli eval check docs/14-eval/eval-1.md -p app.mpr --skip-mx-check

# with full reports
mxcli eval check docs/14-eval/eval-1.md -p app.mpr --output eval-results/
```

---

## Phase 2: Claude Invocation + Docker (Planned)

Phase 2 adds automated Claude Code invocation and runtime validation, enabling fully automated end-to-end evaluation.

### New Command: `mxcli eval run`

```bash
mxcli eval run docs/14-eval/ -p template.mpr --model sonnet --output eval-results/
```

### Pipeline

For each eval test:

1. **Copy template project** — Fresh `.mpr` for each test (no cross-contamination)
2. **Initialize for Claude** — `mxcli init` to set up skills, commands, CLAUDE.md
3. **Invoke Claude Code** — `claude -p "$PROMPT" --model sonnet --max-turns 50`
4. **Run L0-L2 checks** — Structure, validation, lint (reuse Phase 1)
5. **Run L3: Docker** — `mxcli docker run -p app.mpr --wait`
6. **Run L4: Microflow tests** — `mxcli test tests/ -p app.mpr` (if test files exist)
7. **Score and report**
8. **Iteration** — If iteration scenario exists:
   - Invoke Claude again with `--continue` (same session)
   - Re-run L0-L4 checks
   - Score iteration phase separately

### Claude Code Invocation

```bash
# primary: automated via claude CLI
claude -p "You are working on a Mendix project at $DIR. $PROMPT" \
  --model sonnet \
  --max-turns 50 \
  --allowedTools "Bash(mxcli*),Bash(mx*),Read,Write,Edit,Glob,Grep,Skill" \
  --output-format json \
  2>&1 | tee eval-results/APP-001/claude-transcript.json

# Iteration: continue same session
claude -p "$ITERATION_PROMPT" \
  --continue --session-id eval-APP-001 \
  ...
```

### New Check Types

| Check | Description |
|-------|-------------|
| `docker_starts` | App builds and starts in Docker |
| `docker_no_startup_errors` | No errors in startup logs |
| `microflow_tests_pass` | All .test.mdl tests pass |
| `oql_returns_rows` | OQL query returns expected data |

### Template Project

A minimal Mendix 11.6+ project with:
- One empty user module
- Default security (Prototype)
- Standard Atlas UI layout
- No custom entities, pages, or microflows

Located at `docs/14-eval/template/blank-app.mpr`.

### Files to Create

| File | Purpose |
|------|---------|
| `cmd/mxcli/evalrunner/runner.go` | Full orchestrator (copy project → Claude → validate → score) |
| `cmd/mxcli/evalrunner/claude.go` | Claude CLI invocation wrapper |
| `docs/14-eval/template/blank-app.mpr` | Blank Mendix project template |

---

## Phase 3: LLM-as-Judge + Playwright (Planned)

Phase 3 adds qualitative assessment and browser-based UI validation.

### LLM-as-Judge

After automated checks, feed the project state to Claude (via API) for holistic assessment:

```
Given this Mendix project structure:
{show structure depth 3 output}

and these page descriptions:
{describe page outputs for all generated pages}

Evaluate against these acceptance criteria:
{criteria list from eval test}

for each criterion, rate 0 (not met), 1 (partially met), or 2 (fully met).
Explain your rating.

Also identify:
- Missing skills or documentation that would help
- MDL syntax gaps (things Claude wanted to express but couldn't)
- Patterns that should be automated as new check types
- Code quality suggestions
```

Output: structured JSON with per-criterion scores and improvement suggestions.

### Qualitative Scores

| Dimension | Scale | Description |
|-----------|-------|-------------|
| Correctness | 0-10 | Does it work as specified? |
| Completeness | 0-10 | Are all requirements addressed? |
| Quality | 0-10 | Is the code well-structured? (naming, organization, patterns) |
| Iteration | 0-10 | How well was the follow-up handled? |

### Playwright UI Testing

Browser-based validation using `.mx-name-*` CSS selectors (see `proposal-playwright-testing.md`):

```typescript
test('book overview page renders', async ({ page }) => {
  await page.goto('/p/Book_Overview');
  await expect(page.locator('.mx-name-dataGrid1')).toBeVisible();
  await expect(page.locator('.mx-name-btnNew')).toBeVisible();
});
```

### New Check Types

| Check | Description |
|-------|-------------|
| `page_loads` | HTTP 200 on page URL |
| `widget_visible` | Widget renders in the browser (Playwright) |
| `form_submits` | Fill form + submit, verify no errors |
| `llm_criterion_met` | LLM-as-judge rates criterion >= 1 |

### Improvement Suggestion Pipeline

The LLM-as-judge output feeds into actionable improvement categories:

| Category | Example | Action |
|----------|---------|--------|
| Skill gap | "Delete confirmation not generated" | Update `create-page.md` skill |
| Missing syntax | "Filter widget not supported in MDL" | Add to MDL grammar |
| Tooling gap | "No auto-navigation when creating CRUD" | Enhance `create-crud` skill |
| Documentation | "COMBOBOX for enums not documented" | Update skill docs |

### Trend Tracking

Compare results across runs to detect regressions and improvements:

```bash
mxcli eval report eval-results/ --compare

# Output:
# run 2026-02-20  → average: 72%  (Sonnet 4.5)
# run 2026-02-25  → average: 85%  (Sonnet 4.6)  ↑ +13%
#   APP-001: 70% → 90% ↑  (entity types now correct)
#   APP-003: 80% → 75% ↓  (regression: delete confirmation missing)
```

### Files to Create

| File | Purpose |
|------|---------|
| `cmd/mxcli/evalrunner/judge.go` | LLM-as-judge scoring via Claude API |
| `cmd/mxcli/evalrunner/playwright.go` | Playwright test generation and execution |
| `cmd/mxcli/evalrunner/trends.go` | Cross-run comparison and trend detection |

---

## Eval Test Categories

Planned test categories for comprehensive coverage:

| Category | ID Range | Examples |
|----------|----------|---------|
| App/Crud | APP-001+ | Bookstore inventory, employee directory, product catalog |
| App/Workflow | WF-001+ | Leave request, approval process, order pipeline |
| App/Dashboard | DASH-001+ | Sales dashboard, KPI overview, reporting |
| Domain Model | DM-001+ | Complex associations, generalization, enums |
| Microflow | MF-001+ | CRUD logic, validation, error handling |
| Pages | PG-001+ | Master-detail, wizard, popup forms |
| Security | SEC-001+ | Role-based access, entity rules |
| Integration | INT-001+ | REST consumption, business events |

---

## Directory Structure

```
docs/14-eval/
├── eval-1.md                    # Test definitions (one or more per file)
├── eval-2.md                    # More test definitions
├── template/                    # Blank Mendix project template (Phase 2)
│   └── blank-app.mpr
└── README.md                    # How to write eval tests

eval-results/                    # Generated output (gitignored)
├── run-2026-02-25T10-30/
│   ├── summary.json             # all results for this run
│   ├── summary.md               # Human-readable report
│   ├── APP-001/
│   │   ├── score.json           # Per-test scores
│   │   └── claude-transcript.json  # (Phase 2) Claude session log
│   └── APP-002/
│       └── ...

cmd/mxcli/
├── cmd_eval.go                  # CLI commands
└── evalrunner/
    ├── parser.go                # Test definition parser (Phase 1 ✓)
    ├── parser_test.go           # Parser tests (Phase 1 ✓)
    ├── checks.go                # check execution (Phase 1 ✓)
    ├── results.go               # Result types + scoring (Phase 1 ✓)
    ├── report.go                # Report generation (Phase 1 ✓)
    ├── runner.go                # full orchestrator (Phase 2)
    ├── claude.go                # Claude CLI wrapper (Phase 2)
    ├── judge.go                 # LLM-as-judge (Phase 3)
    ├── playwright.go            # Playwright integration (Phase 3)
    └── trends.go                # cross-run comparison (Phase 3)
```

---

## Existing Infrastructure Reused

| Need | Existing Tool | Status |
|------|---------------|--------|
| Entity/page/microflow inspection | `mxcli show/describe` commands | Used in Phase 1 |
| Project validation | `mx check app.mpr` | Used in Phase 1 |
| Code quality | `mxcli lint --format json` | Used in Phase 1 |
| YAML parsing | `gopkg.in/yaml.v3` | Already a dependency |
| Docker build/deploy | `mxcli docker run --wait` | Phase 2 |
| Microflow tests | `mxcli test` + JUnit XML | Phase 2 |
| OQL queries | `mxcli oql` | Phase 2 |
| Project initialization | `mxcli init` | Phase 2 |
| Playwright proposal | `proposal-playwright-testing.md` | Phase 3 |

---

## Summary

The eval framework turns quality measurement into an automated, repeatable process. Phase 1 (implemented) provides the foundation for validating any Mendix project against structured acceptance criteria. Phase 2 will close the loop by automating Claude invocation and runtime testing. Phase 3 will add qualitative assessment and browser-based validation for comprehensive coverage.

The key design principle is **progressive validation**: cheap structural checks run first and gate expensive runtime tests, so failed builds don't waste Docker resources, and the feedback loop is fast when things break early.
