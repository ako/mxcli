---
title: Mendix Automated Testing Pipeline
status: proposed
---

# Mendix Automated Testing Pipeline
**Implementation Proposal for Claude Code**

Integration of Playwright UI testing and PostgreSQL data validation into the mxcli-based generation pipeline.

---

## Overview

This document describes the automated testing layer to be added to the existing Mendix app generation pipeline. The pipeline currently uses mxcli to build Mendix apps from MDL definitions and Docker to run the runtime. This proposal adds two complementary testing planes that close the feedback loop and enable self-correcting generation.

---

## Current Infrastructure

The following capabilities already exist and form the foundation for this proposal:

| Capability | Status | How |
|------------|--------|-----|
| Docker Compose with Mendix + PostgreSQL | Done | `mxcli docker init` generates the stack |
| One-command build + deploy | Done | `mxcli docker run -p app.mpr --wait` |
| Runtime readiness detection | Done | `--wait` tails logs until "Runtime successfully started" |
| Healthchecks on both services | Done | curl on mendix, pg_isready on db |
| DB credentials with known defaults | Done | `mendix/mendix/mendix` in `.env` |
| `postgresql-client` in devcontainer | Done | psql available for DB assertions |
| Volume mounts (fast rebuild cycle) | Done | No Docker image rebuild needed |
| OQL view entities in MDL grammar | Done | `create view entity ... as (select ...)` |
| Widget names → `.mx-name-*` CSS classes | Done | Mendix renders these deterministically |

**Gaps to close:**

| Gap | Fix | Phase |
|-----|-----|-------|
| PostgreSQL port not exposed to test runner | Add `5432:5432` to db service in compose template | 1 |
| No Node.js in devcontainer | Add `ghcr.io/devcontainers/features/node:1` feature | 1 |
| No Playwright in devcontainer | Add `npx playwright install --with-deps` to postCreateCommand | 1 |
| No `TEST_DB_URL` env var | Add to `.env.example` and compose template | 1 |
| No test skill or command | New `.claude/skills/mendix/test-app.md` + command | 1 |

---

## Pipeline Architecture

```
Claude Code generates MDL
  → mxcli docker run --wait (builds + deploys + waits for ready)
    → Playwright UI tests run (Layer 1)
    → PostgreSQL assertions run (Layer 2)
      → Results feed back to Claude Code (structured json)
        → Claude Code iterates on failures → rebuild → retest
```

Runtime readiness is already handled by `mxcli docker run --wait`, which tails container logs and returns only after "Runtime successfully started" (or exits with an error on failure/timeout). No separate health polling script is needed.

---

## Why Playwright

### The Core Problem

Mendix apps are React single-page applications — the server returns a JavaScript shell, and the actual UI is rendered client-side. An HTTP 200 from `/p/LeaveRequest` tells you nothing about whether the page's widgets actually rendered. A button requested in MDL might be missing from the DOM because of a conditional visibility bug, a wrong container nesting, or a BSON serialization issue — none of which are detectable without executing JavaScript in a real browser.

**Real example**: Claude Code adds an action button to a page via MDL. The MDL is syntactically valid, `mxcli check` passes, the app starts. But the button doesn't appear — it's nested inside a container whose visibility expression evaluates to false, or the widget BSON is malformed in a way the runtime silently ignores. Only a browser-based assertion on `.mx-name-myButton` catches this.

### Tool Comparison

| | Playwright | Cypress | Puppeteer | HTTP checks |
|---|---|---|---|---|
| Detects missing widgets | Yes | Yes | Yes | No (SPA) |
| Headless Linux (devcontainer) | Excellent | Needs Xvfb | Good | N/A |
| Structured JSON reporter | Built-in | Plugin | Manual | Manual |
| Auto-wait for elements | Built-in | Built-in | Manual | N/A |
| Test API complexity | Low | Low | High | Trivial |
| Binary footprint | ~150MB (chromium) | ~200MB+ | ~150MB | 0 |
| Generated test footguns | Few | Few | Many | None |

**Decision**: Playwright. It has the best headless Linux support (critical for devcontainers), a built-in JSON reporter (critical for the feedback loop), and a high-level API that minimizes generation errors. Cypress is close but heavier and less ergonomic for headless CI-style loops. Puppeteer is too low-level for auto-generated tests.

HTTP smoke checks (`curl -sf http://localhost:8080/p/page`) are a useful lightweight complement — they catch 500 errors and missing pages before the heavier browser tests run — but they cannot replace browser-based widget assertions.

---

## Layer 1: Playwright UI Testing

### Widget Name Selectors

Mendix renders each widget's unique name property as a CSS class on the corresponding DOM element:

```html
<div class="mx-name-submitButton form-group">
```

This makes selectors stable, semantic, and directly traceable to the MDL definition. Claude Code already knows all widget names from the model it generated, so test generation is mechanical: walk the MDL, identify interactive widgets, emit the corresponding `.mx-name-*` assertions.

### Generated Test Example

For a leave request form generated from MDL, the corresponding Playwright test looks like:

```typescript
test('submit leave request', async ({ page }) => {
  await page.goto('/p/LeaveRequest');

  // Fill form using mx-name widget selectors
  await page.locator('.mx-name-startDatePicker').fill('2026-03-01');
  await page.locator('.mx-name-endDatePicker').fill('2026-03-05');
  await page.locator('.mx-name-leaveTypeSelect').selectOption('Vacation');
  await page.locator('.mx-name-reasonInput').fill('Annual holiday');
  await page.locator('.mx-name-submitButton').click();

  // Assert confirmation feedback widget is visible
  await expect(page.locator('.mx-name-confirmationMessage')).toBeVisible();
});
```

### Test Structure

Tests are generated alongside the app, not written separately afterwards. The generation prompt instructs Claude Code to emit a `tests/` directory containing:

- `playwright.config.ts` — base URL, browser config, timeout settings
- `tests/smoke.spec.ts` — HTTP reachability for all pages, login, no console errors (app-agnostic, fast)
- `tests/<module>.spec.ts` — per-module widget presence and interaction tests using `.mx-name-*` selectors

The smoke test runs first as a fast gate — if pages return 500 or the app can't start, there's no point running the full Playwright suite. Module tests then verify that every generated widget is actually present and interactive in the DOM.

### Runtime Readiness

Playwright must not start until the Mendix runtime is fully initialized. This is already handled:

```bash
# build, deploy, and wait for runtime — single command
mxcli docker run -p app.mpr --wait

# then run tests
npx playwright test --reporter=json > playwright-results.json
```

---

## Layer 2: PostgreSQL Validation

### Data Integrity Assertions

After a UI interaction, the test queries the underlying PostgreSQL database to confirm the correct data was persisted. This catches cases where the UI shows success but the microflow silently failed or committed partial data.

```typescript
import { client } from 'pg';

const db = new client({ connectionString: process.env.TEST_DB_URL });
await db.connect();

test('leave request persisted to database', async ({ page }) => {
  await submitLeaveRequestViaUI(page);

  const { rows } = await db.query(`
    select * from "leaverequest$leaverequest"
    where "status" = 'Pending'
    ORDER by "createddate" desc limit 1
  `);

  expect(rows).toHaveLength(1);
  expect(rows[0].startdate).toEqual('2026-03-01');
  expect(rows[0].enddate).toEqual('2026-03-05');
  expect(rows[0].leavetype).toEqual('Vacation');
});
```

### Table Name Convention

Mendix maps entity names to PostgreSQL table names using the pattern `modulename$entityname` (lowercase). Since Claude Code generates both the domain model and the tests, it can emit the correct table and column names directly — no manual mapping required.

---

## Layer 3: Performance — Query Analysis and View Entity Optimization

### Why This Matters

The Mendix runtime generates all SQL queries internally — Claude Code cannot rewrite them directly. However, there are two powerful levers for optimization that Claude Code _can_ control via MDL:

1. **Indexes** — Add indexes to entity attributes that appear in XPath/OQL WHERE clauses or sort expressions
2. **OQL View Entities** — Replace complex data retrieval patterns (multiple associations, aggregations, derived columns) with pre-defined OQL views that the runtime can query more efficiently

View entities are particularly impactful for:
- DataGrid/Gallery widgets showing data from multiple associated entities
- Dashboard/chart pages with aggregations (counts, sums, averages)
- List screens with computed columns or filtered subsets

### OQL View Entities in MDL

MDL already supports view entity creation with full OQL queries:

```sql
-- Replace a complex retrieve-with-associations pattern
create view entity Dashboard.ActiveOrderSummary (
  CustomerName: string(200),
  OrderCount: integer,
  TotalValue: decimal,
  LastOrderDate: datetime
) as (
  select
    c.Name as CustomerName,
    count(o.ID) as OrderCount,
    sum(o.TotalAmount) as TotalValue,
    max(o.OrderDate) as LastOrderDate
  from Dashboard.Customer c
  join Dashboard.Order o on c.Orders = o.ID
  where o.Status != 'Cancelled'
  GROUP by c.Name
);
```

A DataGrid bound to this view entity executes a single optimized query instead of N+1 retrieves across associations.

### Performance Detection Flow

When query performance issues are detected:

```
1. pg_stat_statements captures slow queries during test run
2. EXPLAIN ANALYZE provides execution plan for slow queries
3. Claude Code analyzes the plan and source MDL to identify:
   a. Missing indexes → add index in alter entity
   b. N+1 association patterns → replace with OQL view entity
   c. full table scans on filtered lists → add where-targeted index
4. Claude Code generates the fix in MDL, rebuilds, retests
```

### pg_stat_statements Snapshot

```typescript
async function captureQueryStats(db) {
  const { rows } = await db.query(`
    select query, calls, mean_exec_time, rows
    from pg_stat_statements
    where dbid = (select oid from pg_database where datname = current_database())
  `);
  return rows;
}

test('page load stays within query budget', async ({ page }) => {
  const before = await captureQueryStats(db);
  await page.goto('/p/OrderDashboard');
  await page.locator('.mx-name-dataGrid1').waitFor();
  const after = await captureQueryStats(db);

  const newQueries = diffStats(before, after);
  expect(newQueries.length).toBeLessThan(10);
  newQueries.forEach(q => expect(q.mean_exec_time).toBeLessThan(200));
});
```

### EXPLAIN ANALYZE Integration

When a query exceeds the performance threshold, the test captures the execution plan. Claude Code uses this to decide between index addition and view entity refactoring:

```typescript
async function explainQuery(db, query) {
  const { rows } = await db.query(`EXPLAIN (ANALYZE, format json) ${query}`);
  return rows[0]['QUERY PLAN'];
}

// if slow query detected, attach plan to test output
if (slowQuery) {
  const plan = await explainQuery(db, slowQuery.query);
  fs.writeFileSync('explain-output.json', JSON.stringify(plan, null, 2));
}
```

**Optimization decision guide for Claude Code:**

| Signal in EXPLAIN plan | MDL fix |
|------------------------|---------|
| Seq Scan on large table with filter | Add index on filtered attribute |
| Nested Loop with high row count | Replace with OQL view entity using JOIN |
| Multiple sequential queries for one page load (N+1) | Create view entity that joins in a single query |
| Sort on unindexed column | Add index on sort attribute |
| Aggregate across association (COUNT, SUM) | Create view entity with GROUP BY |

---

## Claude Code Feedback Loop

### Result Format

Both Playwright and PostgreSQL test results are written as structured JSON. Claude Code reads these after each test run to determine what to fix:

```bash
mxcli docker run -p app.mpr --wait
npx playwright test --reporter=json > playwright-results.json
```

A failed Playwright assertion on `.mx-name-submitButton` directly identifies the widget named `submitButton` in the MDL. A failed DB assertion on `leaverequest$leaverequest.status` directly identifies the entity and attribute in the domain model. There is no ambiguity about which generated component to fix.

### CLAUDE.md Instructions

Add the following block to the project `CLAUDE.md`:

```markdown
## Automated Testing

after building and starting the app:

1. run: mxcli docker run -p app.mpr --wait
2. run: npx playwright test --reporter=json > playwright-results.json
3. read the result file.
4. for each failure:
   - Playwright failure on .mx-name-X  → fix widget X in MDL
   - DB assertion failure on Table.Column  → fix entity/attribute or microflow
   - Slow query with Seq Scan  → add index to entity attribute
   - Slow query with N+1 / nested loops  → replace with OQL view entity
5. Rebuild with mxcli, restart Docker, retest.
6. Only mark the task complete when all tests pass.

## Test Generation

when generating a new app module, also generate:
- tests/<module>.spec.ts using .mx-name-* selectors for all interactive widgets
- DB assertion for each entity created or modified by the module's microflows
- for DataGrids/Galleries with associations: consider an OQL view entity
```

---

## Implementation Phases

### Phase 1: Playwright UI Testing (functional correctness)

**Goal**: Verify that every widget generated in MDL actually renders in the browser. This is the most critical gap today — MDL can be syntactically valid and pass `mxcli check`, but widgets may not appear at runtime due to conditional visibility, container nesting issues, or BSON serialization problems. Only a real browser can catch this.

**Infrastructure changes:**
- Add `ghcr.io/devcontainers/features/node:1` to devcontainer template
- Add `npx playwright install --with-deps chromium` to postCreateCommand
- Add `5432:5432` port mapping to db service in docker-compose template
- Add `TEST_DB_URL=postgresql://mendix:mendix@localhost:5432/mendix` to `.env.example`

**New files/skills:**
- `.claude/skills/mendix/test-app.md` — skill for generating and running tests
- `.claude/commands/mendix/test.md` — `/test` command for test execution

**Test generation:**
- `playwright.config.ts` — base URL `http://localhost:8080`, chromium only, JSON reporter
- `tests/smoke.spec.ts` — HTTP reachability check for all pages (fast gate), login flow, no console errors
- `tests/<module>.spec.ts` — per-module tests:
  - **Widget presence**: every widget generated in MDL has a corresponding `await expect(page.locator('.mx-name-widgetName')).toBeVisible()` assertion
  - **Form interactions**: fill inputs, click buttons, verify feedback
  - **Navigation**: page transitions via action buttons

**What this validates:**
- Generated widgets are actually present and visible in the DOM (the "button doesn't show up" problem)
- Form fields accept input and buttons respond to clicks
- Page navigation works between generated pages
- No JavaScript errors or 500 responses

### Phase 2: PostgreSQL Data Assertions (data correctness)

**Goal**: Verify that UI interactions persist the correct data.

**New test utilities:**
- `tests/utils/db.ts` — pg client wrapper using `TEST_DB_URL`
- DB assertions in `tests/<module>.spec.ts` alongside UI assertions

**What this validates:**
- Entity creation persists correct attribute values
- Microflow logic produces expected database state
- Association references are set correctly
- Delete/rollback operations work

### Phase 3: Query Performance and View Entity Optimization

**Goal**: Detect slow queries and fix them via indexes or OQL view entities.

**Prerequisites:**
- Enable `pg_stat_statements` in PostgreSQL container (custom config or image)
- Sufficient test data to make performance assertions meaningful

**New capabilities:**
- `pg_stat_statements` snapshot before/after each page load test
- EXPLAIN ANALYZE capture for queries exceeding threshold
- Optimization guide in test output: "Add index on X" or "Replace with OQL view entity"
- Claude Code uses MDL to apply the fix:
  - `alter entity Module.Entity add index idx_name (attributename);`
  - `create view entity Module.OptimizedView (...) as (select ... join ... GROUP by ...);`
  - Rebind DataGrid/Gallery datasource to the view entity

**What this validates:**
- Page loads stay within query budget
- No N+1 query patterns on list pages
- Aggregation queries use view entities instead of client-side computation

### Optional Extensions (not phased)

- Accessibility assertions via Playwright's axe integration on generated pages
- Visual regression snapshots for layout-sensitive widgets
- `pg_stat_statements` baseline file committed to repo, diffed across generations
- Lint rule: warn on DataGrid with 2+ association columns and no view entity

---

## New Files Summary

| File | Phase | Purpose |
|------|-------|---------|
| `playwright.config.ts` | 1 | Base URL, browser, timeouts, reporter config |
| `tests/smoke.spec.ts` | 1 | App-agnostic: login, nav, no console errors |
| `tests/<module>.spec.ts` | 1+2 | Per-module UI + data tests from MDL widget names |
| `tests/utils/db.ts` | 2 | pg client wrapper |
| `tests/utils/perf.ts` | 3 | pg_stat_statements snapshot + EXPLAIN helpers |
| `.claude/skills/mendix/test-app.md` | 1 | Skill for generating and running tests |
| `.claude/commands/mendix/test.md` | 1 | `/test` slash command |

---

## MDL / mxcli Conventions

- All interactive widgets must have a unique `name` property — treat unnamed interactive widgets as a lint error
- Generated entity names follow `ModuleName$EntityName` convention in PostgreSQL (lowercase)
- Index definitions are expressible in MDL: `alter entity ... add index ...`
- OQL view entities are expressible in MDL: `create view entity ... as (select ...)`
- For DataGrids/Galleries with 2+ association columns, prefer an OQL view entity over direct entity binding

---

## Summary

This testing layer turns the generation pipeline into a self-correcting loop. The key properties that make it tractable are:

- **Widget names in the DOM** mean Playwright selectors are derivable directly from the MDL, making test generation mechanical rather than manual.
- **Entity and attribute names** map deterministically to PostgreSQL table and column names, so DB assertions are equally auto-derivable.
- **OQL view entities** give Claude Code a powerful optimization lever — replacing N+1 association traversals and client-side aggregations with single, efficient database queries, all expressible in MDL.
- **Structured JSON output** from both test layers gives Claude Code precise, actionable failure context with direct pointers back into the model.
- **The fix-rebuild-retest loop** is fully automatable: `mxcli docker run --wait` handles build+deploy+readiness, Playwright provides structured failure output, and Claude Code iterates until all tests pass.

The result is generated apps that are not just runnable but verifiably correct and performant, with automated feedback that improves output quality over successive generations.
