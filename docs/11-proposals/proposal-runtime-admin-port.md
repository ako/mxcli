---
title: mxcli Feature Request: Runtime Integration via M2EE Admin API
status: proposed
---

# mxcli Feature Request: Runtime Integration via M2EE Admin API

## Background

During development with mxcli + Docker, we reverse-engineered the Mendix runtime's M2EE admin API
and the `/mxdevtools/` WebSocket protocol. The findings open up several high-value capabilities that
mxcli should expose: hot model reload (eliminating container restarts), CSS hot reload, microflow
debugging, and runtime statistics. This document covers the technical details and proposes concrete
feature additions.

### Implementation Priority

| Priority | Item | Rationale |
|----------|------|-----------|
| **P0** | `mxcli docker reload` | Immediate value, straightforward implementation, biggest time saving |
| **P1** | Admin `addresses = *` config | Small diff, unblocks `--direct` mode for `reload` and `oql` |
| **P1** | OQL verification test pattern | Document Playwright + OQL pattern; works today with no new code |
| **P2** | Update skill files | Document reload workflow post-implementation; `runtime-admin-api.md` already covers M2EE basics |
| **P2** | `create published rest service` | New MDL domain; enables microflow unit testing and API development |
| **P3** | `mxcli test microflow` | Purpose-built test command; depends on Published REST or `/xas/` protocol |
| **P3** | `mxcli stats --slow-queries` | High diagnostic value for AI-assisted development loop |
| **P4** | `mxcli debug` | Defer until there's a clear non-interactive interaction model (see review notes) |

---

## Discovery: M2EE Admin API

The Mendix runtime exposes an HTTP JSON API on the admin port (default 8090). mxcli already uses
this internally for `mxcli oql` — but it is not exposed for other uses.

**Authentication:** HTTP header `X-M2EE-authentication` with the password **base64-encoded**.

```bash
PASS_B64=$(echo -n "$M2EE_ADMIN_PASS" | base64)
curl -X post http://localhost:8090/ \
  -H "content-type: application/json" \
  -H "X-M2EE-authentication: $PASS_B64" \
  -d '{"action":"ACTION_NAME","params":{...}}'
```

This is already how `mxcli oql` works internally (verified via strace). The password is read from
`.docker/.env` → `M2EE_ADMIN_PASS`.

**Confirmed working actions (Mendix 11.6.3):**

| Action | Response | Notes |
|--------|----------|-------|
| `about` | `{"feedback":{"version":"11.6.3","name":"Mendix runtime",...}}` | Runtime info |
| `runtime_status` | `{"feedback":{"status":"running"}}` | Health check |
| `reload_model` | `{"feedback":{"startup_metrics":{"duration":98,...}}}` | **Hot reload** — reloads compiled model from disk in ~100ms, no container restart |
| `update_styling` | `{"feedback":{}}` | Pushes CSS hot reload to all connected browser clients |
| `update_configuration` | `{"feedback":{}}` | Updates runtime configuration |
| `preview_execute_oql` | (already used by `mxcli oql`) | OQL query execution |
| `enable_debugger` | Enables microflow debugger | |
| `disable_debugger` | Disables microflow debugger | |
| `get_ddl_commands` | `{"feedback":{"count":0}}` | Pending DDL command count |

---

## Discovery: `/mxdevtools/` WebSocket Protocol

The Mendix runtime exposes a WebSocket endpoint at `ws://host:8080/mxdevtools/` (on the app port,
not the admin port). This is a **server-push-only** channel — the server pushes instructions to
connected browser clients. Studio Pro uses the M2EE admin API to trigger these pushes.

On connect, the server immediately sends:

```json
{"type":"set_breakpoints","breakpoints":[]}
{"type":"set_deployment_id","deploymentId":"639072209152699435"}
```

**All instruction types the browser client handles:**

| Instruction (server → browser) | Effect |
|--------------------------------|--------|
| `set_deployment_id` | Sets/updates deployment ID; if it changes, triggers full browser reload |
| `reload` | Forces full browser reload of the app |
| `set_breakpoints` | Sets microflow debugger breakpoints |
| `debugger_step` | Steps through a paused breakpoint |
| `get_debugger_variable` | Reads a variable value during microflow execution |
| `update_styling` | Hot-reloads CSS in the browser without page reload |

These browser-side instructions are triggered by M2EE API calls (Studio Pro's workflow). When
`reload_model` is called, a new `set_deployment_id` with a changed ID is presumably pushed to
browsers, triggering automatic refresh.

---

## Discovery: Docker Bind Mount

The current mxcli Docker setup (`.docker/`) uses a **bind mount** from `.docker/build/` to
`/mendix/` inside the container:

```
/workspaces/project/.docker/build  →  /mendix/  (bind mount)
```

This means `mxcli docker build` output (the PAD — Portable App Distribution) is **immediately
visible** to the running runtime. Calling `reload_model` after a build causes the runtime to
re-read from the bind-mounted directory in ~100ms, with no Docker image rebuild or container
restart required.

**Current hot reload cycle (discovered):**

```
mxcli exec script.mdl -p app.mpr       # ~1s   — update model
mxcli docker build -p app.mpr          # ~55s  — compile PAD
[M2EE call] reload_model               # ~100ms — live reload, no restart
# Total: ~56s
```

vs the current `mxcli docker run` approach (~75s including container restart + startup overhead).

---

## Proposed Feature 1: `mxcli docker reload` command

A new subcommand that chains build + reload_model into one step, replacing the need for `docker up`
restarts during development.

**Interface:**

```bash
mxcli docker reload -p app.mpr                # full: mxbuild + reload_model
mxcli docker reload -p app.mpr --model-only   # skip mxbuild, just call reload_model
mxcli docker reload -p app.mpr --css          # update_styling only (instant, no mxbuild)
mxcli docker reload -p app.mpr --wait         # wait for reload confirmation + print duration
mxcli docker reload -p app.mpr --direct       # connect to admin API directly (requires admin.addresses = *)
```

**Implementation notes:**

- Reads `M2EE_ADMIN_PASS` and `ADMIN_PORT` from `.docker/.env` (same pattern as `oql`)
- Routes through `docker compose exec` to reach the admin API on container localhost by default
  (same mechanism as `oql`)
- If `--direct` flag is set, connects directly via `http://localhost:ADMIN_PORT/` — requires the
  admin API to be bound to `*` (see Config Change below)
- On success, prints reload duration from `startup_metrics.duration`

**Expected output:**

```
Building PAD...       53s
Reloading model...    done (98ms)
App updated at http://localhost:8080
```

**When to use `reload` vs `run`:**

| Scenario | Command |
|----------|---------|
| Page, microflow, or logic change | `mxcli docker reload` |
| CSS / theme change only | `mxcli docker reload --css` |
| New entity or attribute (additive schema change) | `mxcli docker reload` (runtime applies DDL on reload) |
| First-time setup | `mxcli docker run` |
| Volume corruption or database reset | `mxcli docker run --fresh` |

### Open Questions — DDL Handling

> **TODO: Verify experimentally.** Does `reload_model` auto-apply DDL for additive schema changes
> (new entities, new attributes)? The `get_ddl_commands` action returns a pending DDL count, which
> suggests DDL may require explicit approval rather than automatic application. Test scenarios:
>
> 1. Add a new entity + `reload_model` → does the table appear in PostgreSQL?
> 2. Add a new attribute to an existing entity + `reload_model` → does `alter table add column` run?
> 3. Remove an attribute + `reload_model` → does the column get dropped, or does the runtime error?
> 4. Change an attribute type (e.g., String → Integer) + `reload_model` → what happens?
>
> If DDL is not auto-applied, `mxcli docker reload` may need to call `get_ddl_commands` and then
> an `execute_ddl_commands` action (if one exists), or fall back to a container restart for schema
> changes.

### Open Questions — Error Handling

> **TODO: Verify experimentally.** What happens when `reload_model` encounters a model that has
> validation errors MxBuild didn't catch? Possible outcomes:
>
> 1. Returns `result != 0` with an error message (ideal — `docker reload` can report it)
> 2. Runtime enters a broken state requiring restart (needs detection + automatic recovery)
> 3. Runtime crashes entirely (needs container health check to detect and restart)
>
> This determines whether `docker reload` can safely be the default iteration path, or whether it
> needs a pre-flight health check / rollback strategy.

---

## Proposed Feature 2: Docker Configuration — Admin API accessibility

**Problem:** The admin API currently binds to `localhost` inside the container only
(`admin.addresses = [localhost]` in the generated `Default.conf`). Even though port 8090 is mapped
to the host in `docker-compose.yml`, connections from the host are refused because the API is not
listening on the container's external interface.

**Proposed change:** When generating `etc/configurations/Default.conf` via `mxcli docker init`,
set:

```hocon
admin {
  port = 8090
  addresses = ["*"]   # was: [ localhost ]
}
```

Or expose this as an environment variable override via `variables.conf`:

```hocon
admin {
  addresses = ${?ADMIN_ADDRESSES}
}
```

With `ADMIN_ADDRESSES=["*"]` added to the generated `.docker/.env`.

This enables:

- `mxcli docker reload --direct` (bypass docker exec, faster)
- `mxcli oql --direct` to work without docker exec
- Direct host access to admin API from tooling and CI pipelines
- Future IDE integrations that talk directly to the running runtime

> **Security note:** This is appropriate for development-only Docker stacks. Production deployments
> should never expose the admin port externally, and mxcli should document this clearly.

**Migration note:** The `docker exec` workaround in `mxcli oql` already solves the connectivity
problem, so this is not blocking. `mxcli docker init --force` regenerates `Default.conf`, so the
change only affects projects that re-initialize. Existing projects keep working via docker exec.

---

## Proposed Feature 3: Hot Reload Skill

**Status:** Partially done. The skill `.claude/skills/mendix/runtime-admin-api.md` already covers
topic 2 (M2EE auth, curl examples, troubleshooting). Once `docker reload` is implemented, update
that skill (or create a companion `hot-reload.md`) with the reload workflow.

**Remaining topics to document:**

1. **When to use `mxcli docker reload` vs `mxcli docker run`** — see table in Feature 1 above

2. ~~**The M2EE admin API**~~ — *(done in `runtime-admin-api.md`)*

3. **`update_styling` for CSS-only changes** — no mxbuild needed if only theme files changed;
   the PAD styling files can be copied directly into `.docker/build/` and `update_styling` called

4. **The mxdevtools WebSocket** — what it is, server-push-only, used for browser-side auto-refresh
   and debugger integration

5. **When `reload_model` is NOT sufficient** — if the database schema requires destructive changes
   (dropped columns, type changes), a `--fresh` restart may be needed; the skill should explain
   how to recognize this from runtime logs

---

## Future Feature: Microflow Debugger Integration

The M2EE admin API has `enable_debugger` / `disable_debugger` actions and the `/mxdevtools/`
WebSocket delivers `set_breakpoints`, `debugger_step`, and `get_debugger_variable` instructions to
browser clients.

A separate `debugger/` HTTP endpoint is also registered at startup:

```
added request handler 'debugger/' with class com.mendix.modules.debugger.internal.DebuggerHandler
```

This endpoint likely handles breakpoint management and variable inspection — the same backend that
Studio Pro's debugger panel connects to.

**Potential `mxcli debug` subcommand:**

```bash
mxcli debug enable -p app.mpr                            # enable the debugger
mxcli debug breakpoint add F1.ACT_SaveDriver -p app.mpr  # set a breakpoint on a microflow
mxcli debug breakpoint list -p app.mpr                   # list active breakpoints
mxcli debug watch -p app.mpr                             # tail debugger events (pauses, variable values)
mxcli debug disable -p app.mpr                           # disable the debugger
```

This would allow CLI-driven microflow debugging without Studio Pro, which is particularly valuable
for AI-assisted development where the agent can set breakpoints, inspect variable values at runtime,
and validate microflow behavior programmatically — closing the loop between code generation and
execution verification.

### Review Notes

The full `debug` subcommand tree is ambitious but the value for AI-assisted development is unclear
— Claude can't interactively step through breakpoints in a conversation. A more focused scope that
would deliver immediate value:

- **`mxcli debug enable/disable`** — toggle the debugger on/off
- **`mxcli debug inspect`** — run a microflow, set a breakpoint, capture variable values at that
  point, and return them as structured output

This "run and capture" model fits the AI development loop: generate a microflow, deploy it via
`docker reload`, then verify "what was `$Order.Total` at line 5?" without requiring interactive
stepping. Defer the full breakpoint management UX until there's demand.

---

## Future Feature: Runtime Statistics via Admin API

The Mendix runtime exposes a **Prometheus metrics endpoint** registered at startup:

```
added admin request handler '/prometheus' with class com.mendix.metrics.prometheus.PrometheusServlet
```

This is accessible at `http://localhost:8090/prometheus` (admin port) and likely exposes:

- JVM heap usage and GC pressure
- Active and idle database connection pool counts
- HTTP request throughput and latency
- Active session counts
- Task queue depths

Combined with `LogMinDurationQuery` (already a known runtime parameter in `variables.conf`), this
enables meaningful application performance insight during development.

**Proposed `mxcli stats` command:**

```bash
mxcli stats -p app.mpr                  # summary: sessions, memory, DB connections
mxcli stats -p app.mpr --prometheus     # raw Prometheus text output
mxcli stats -p app.mpr --slow-queries   # queries logged above LogMinDurationQuery threshold
mxcli stats -p app.mpr --watch          # live tail, refresh every N seconds
```

**Why this matters for AI-assisted development:**

These insights close a feedback loop that currently doesn't exist. Claude can generate a microflow
or OQL query, deploy it via `mxcli docker reload`, then immediately run `mxcli stats` to check
whether it introduced connection pool pressure or slow queries — before the developer notices a
problem. Over time, this enables proactive performance awareness as part of the normal generation
workflow.

### Review Notes

The `--slow-queries` angle is the killer feature here. If `LogMinDurationQuery` logs are accessible
through the admin API or container logs, Claude could diagnose OQL performance issues automatically
after `mxcli oql` or after deploying a VIEW ENTITY. The raw Prometheus dump is less useful in a CLI
context — consider a curated summary by default (sessions, heap, DB pool) and only expose
`--prometheus` for piping to monitoring tools.

---

## Proposed Feature: Microflow Unit Testing

### The Problem

Playwright tests verify UI behavior end-to-end, but there's no way to test an individual microflow
in isolation: call it with specific inputs, check the return value, and verify side effects. This
gap matters for AI-assisted development where Claude generates microflows and needs fast, precise
feedback — not "click through 5 pages and check a database row."

### Approaches Investigated

**Approach 1: Published REST endpoint (most Mendix-native)**

Mendix supports publishing microflows as REST operations. The workflow would be:

```
1. create microflow Module.ACT_CalculateTotal (...)     -- write the logic
2. create published rest service Module.TestAPI (...)    -- expose it as HTTP
3. mxcli docker reload -p app.mpr                       -- hot deploy
4. curl http://localhost:8080/rest/testapi/v1/calculate  -- call it
5. Assert on the HTTP response                           -- verify
```

The metamodel types exist (`rest$PublishedRestService`, `rest$PublishedRestServiceOperation` with a
`microflow` field), but MDL doesn't support `create published rest service` yet. This is the most
stable approach since it uses Mendix's own REST infrastructure.

**Approach 2: The `/xas/` client API (browser protocol)**

The Mendix browser client calls microflows through `/xas/` using a JSON-RPC-style protocol. We
could replicate what the browser does:

```bash
# 1. login to get a session + CSRF token
curl -c cookies.txt http://localhost:8080/xas/ \
  -d '{"action":"login","params":{"username":"MxAdmin","password":"AdminPassword1!"}}'

# 2. call a microflow via the same protocol the browser uses
curl -b cookies.txt http://localhost:8080/xas/ \
  -H "X-Csrf-token: $token" \
  -d '{"action":"executeaction","params":{"actionname":"MyModule.ACT_CalculateTotal","applyto":"none"}}'
```

This avoids needing a Published REST Service, but it's an undocumented internal API that could
change between Mendix versions. Needs experimentation to confirm the exact request format,
parameter passing, and response shape.

**Approach 3: Admin API (if an execute action exists)**

The M2EE admin API might have an undocumented action to execute microflows directly on port 8090.
This would be the cleanest solution — no REST service needed, no session management — but needs
experimentation. Even if it doesn't exist today, Mendix could add it (the runtime already has the
execution infrastructure).

**Approach 4: OQL side-effect verification (works today)**

For microflows that create/modify data, we can verify side effects without calling the microflow
directly:

```bash
# Trigger the microflow through the UI (Playwright) or a rest endpoint
# then verify the result via OQL:
mxcli oql -p app.mpr "select Total from MyModule.Order where OrderNumber = 'TEST-001'"
```

This already works with the current `mxcli oql` command. It doesn't test the microflow in
isolation, but it closes the verification loop for data-producing microflows.

### Recommended Implementation Path

**Phase 1 (works today): OQL verification pattern**

Document a testing pattern that combines Playwright (to trigger microflows via UI) with OQL (to
verify results). No new code needed:

```bash
# 1. Deploy changes
mxcli exec changes.mdl -p app.mpr
mxcli docker reload -p app.mpr

# 2. Trigger via Playwright (clicks a button that calls the microflow)
npx playwright test tests/order-processing.spec.ts

# 3. Verify side effects
mxcli oql -p app.mpr --json \
  "select Total, status from MyModule.Order where OrderNumber = 'TEST-001'" \
  | jq '.[0] | select(.Status == "Completed" and .Total == "150.00")'
```

**Phase 2: `create published rest service` in MDL**

Add grammar, visitor, and executor support for publishing microflows as REST endpoints:

```sql
create published rest service MyModule.TestAPI
  path '/test-api'
  version 'v1'
begin
  operation CalculateTotal
    method post
    path '/calculate'
    microflow MyModule.ACT_CalculateTotal;

  operation GetCustomer
    method get
    path '/customer/{id}'
    microflow MyModule.ACT_GetCustomer;
end;
```

This is a significant feature (new MDL domain, BSON serialization for `rest$PublishedRestService`)
but has value beyond testing — it enables building APIs from the CLI.

**Phase 3: `mxcli test microflow` command**

A purpose-built command that wraps the deploy-publish-call-verify cycle:

```bash
# Test a microflow that returns a value
mxcli test microflow MyModule.ACT_CalculateTotal -p app.mpr \
  --param Order=TEST-001 \
  --expect '{"Total": 150.00}'

# Test a microflow with side-effect verification
mxcli test microflow MyModule.ACT_ProcessOrder -p app.mpr \
  --param OrderId=TEST-001 \
  --verify-oql "SELECT Status FROM MyModule.Order WHERE OrderNumber = 'TEST-001'" \
  --expect-oql '[{"Status": "Completed"}]'
```

Under the hood this would:
1. Auto-create a temporary Published REST Service exposing the microflow
2. Call `docker reload` to deploy
3. `curl` the REST endpoint with the provided parameters
4. Optionally run an OQL query to verify side effects
5. Clean up the temporary REST service

This is the most ambitious option and depends on Phase 2 (Published REST) and `docker reload`.

**Alternative Phase 3: `/xas/` protocol approach**

If the `/xas/` protocol proves stable across Mendix versions, skip the Published REST Service
entirely and call microflows through the client API:

```bash
mxcli test microflow MyModule.ACT_CalculateTotal -p app.mpr \
  --param Order=TEST-001
```

Under the hood: login as MxAdmin via `/xas/`, obtain CSRF token, call `executeaction` with the
microflow name and parameters. Simpler implementation (no REST service creation), but relies on an
undocumented API.

### Open Questions

> **TODO: Experiment with `/xas/` protocol.** Login, obtain CSRF token, and call `executeaction`
> with a simple microflow. Document the exact request/response format. If this works reliably, it
> may be simpler than the Published REST approach for testing purposes.

> **TODO: Check admin API for `execute_action` or similar.** Try calling undocumented actions on
> port 8090 to see if direct microflow execution is possible without HTTP session management.

> **TODO: Test parameter passing.** How are entity-typed parameters passed via REST vs `/xas/`?
> The REST approach uses JSON body mapping; `/xas/` likely uses object GUIDs. Entity parameters
> may need a `retrieve` + pass-by-ID pattern rather than pass-by-value.

---

## Summary of Proposed Additions

| Priority | Item | Type | Status | Impact |
|----------|------|------|--------|--------|
| **P0** | `mxcli docker reload` | New command | Proposed | Cuts iteration time from ~75s to ~56s; eliminates container restarts. Open questions on DDL handling and error recovery need experimental verification. |
| **P0** | `mxcli docker reload --css` | New flag | Proposed | Instant CSS hot reload via `update_styling`; no mxbuild required |
| **P1** | Admin API bind to `*` in dev config | Config change | Proposed | Enables direct host access to admin API; unblocks `--direct` mode. Non-blocking since docker exec workaround exists. |
| **P1** | OQL verification test pattern | Docs/skill | Proposed | Document Playwright + OQL side-effect verification. Works today, no new code. |
| **P2** | Hot reload / docker runtime skill | Skill file | Partial | M2EE API basics done in `runtime-admin-api.md`. Update with reload workflow post-implementation. |
| **P2** | `create published rest service` | New MDL domain | Proposed | Publish microflows as HTTP endpoints. Enables microflow unit testing and API development from CLI. |
| **P3** | `mxcli test microflow` | New command | Proposed | Wraps deploy-call-verify cycle. Depends on Published REST or `/xas/` protocol. |
| **P3** | `mxcli stats --slow-queries` | Future command | Proposed | High diagnostic value; `--slow-queries` is the killer feature. Curated summary preferred over raw Prometheus dump. |
| **P4** | `mxcli debug` | Future command | Proposed | Defer full subcommand tree. Focus on "run and capture" model (`debug inspect`) that fits AI development loop. |
