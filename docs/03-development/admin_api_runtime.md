# Mendix Runtime API: XAS Client Protocol & Microflow Testing

This document captures findings from reverse-engineering the Mendix runtime's client-facing
`/xas/` protocol and the M2EE admin API, with a focus on executing microflows programmatically.

For the M2EE admin API (port 8090) details, see `proposal-runtime-admin-port.md`.

---

## XAS Client Protocol (`/xas/`)

The Mendix browser client communicates with the runtime via a JSON-RPC-style API at
`http://localhost:8080/xas/`. All requests are `post` with `content-type: application/json`.

### Authentication Flow

```bash
# 1. login — returns CSRF token and sets session cookie
curl -s -c cookies.txt -X post http://localhost:8080/xas/ \
  -H 'Content-Type: application/json' \
  -d '{"action":"login","params":{"username":"MxAdmin","password":"Test12345"}}'

# response:
# {"csrftoken":"aa7fd7a4-4ba6-4323-b3ad-80ada602ec41"}
# set-Cookie: XASSESSIONID=...; path=/; HttpOnly
```

All subsequent requests require:
- The `XASSESSIONID` cookie (session)
- The `X-Csrf-token` header (CSRF protection)

### Session Data

After login, call `get_session_data` to get the full session context:

```bash
curl -s -b cookies.txt -X post http://localhost:8080/xas/ \
  -H 'Content-Type: application/json' \
  -H 'X-Csrf-Token: TOKEN' \
  -d '{"action":"get_session_data","params":{}}'
```

Response includes: `uiconfig` (profile, widgets), `enumerations`, `metadata`, `constants`,
`microflows`, `roles`, `user`, and `csrftoken`.

Key fields:
- `uiconfig.profile.kind` — Navigation profile type (e.g., `"Responsive"`)
- `microflows` — Registry of callable microflow names
- `metadata` — Entity/attribute metadata for the client

### XAS Actions (Confirmed in Mendix 11.6.3)

Discovered by analyzing the bundled client JavaScript (`dist/DqJreuW4.js`):

| Action | Purpose | Format |
|--------|---------|--------|
| `login` | Authenticate and create session | `{"action":"login","params":{"username":"...","password":"..."}}` |
| `get_session_data` | Get full session context | `{"action":"get_session_data","params":{}}` |
| `runtimeOperation` | Execute a registered operation by ID | `{"action":"runtimeOperation","operationId":"...","params":{},"options":{},"changes":{},"objects":[]}` |
| `executeaction` | Execute a microflow by name | `{"action":"executeaction","params":{"actionname":"Module.MF","applyto":"none"},"context":[],"changes":{},"objects":[]}` |
| `retrieve_by_xpath` | XPath retrieve | `{"action":"retrieve_by_xpath","params":{"xpath":"//Module.Entity","schema":{},"count":false}}` |
| `instantiate` | Create a new object | `{"action":"instantiate","params":{"objecttype":"Module.Entity"},"changes":{},"objects":[]}` |
| `commit` | Commit objects | `{"action":"commit","params":{"guids":["..."]},"changes":{},"objects":[],"context":[]}` |
| `rollback` | Rollback objects | `{"action":"rollback","params":{"guids":["..."]},"changes":{},"objects":[]}` |
| `delete` | Delete objects | `{"action":"delete","params":{"guids":["..."]},"changes":{},"objects":[]}` |
| `poll_background_job` | Poll async task | `{"action":"poll_background_job","params":{"asyncid":"..."}}` |

### operations.json — The Operation Registry

MxBuild generates `deployment/model/operations.json` (also at `.docker/build/app/model/operations.json`
in PAD builds). This file maps every client-callable operation to an `operationId`.

**Structure:**

```json
[
  {
    "operationId": "+CCdBxEj71SVLZ3SEMdAwA",
    "operationType": "callMicroflow",
    "parameters": {},
    "constants": { "MicroflowName": "Administration.NewAccount" },
    "allowedUserRoleSets": [["Administrator"]]
  },
  {
    "operationId": "XqTp+mP8Zl2co+AipUfMZw",
    "operationType": "retrieve",
    "parameters": {},
    "constants": {
      "PageName": "Administration.ActiveSessions",
      "WidgetName": "Administration.ActiveSessions.dataGrid21",
      "xpath": "//System.Session"
    },
    "allowedUserRoleSets": [["Administrator"], ["user"]]
  }
]
```

**Operation types:** `callMicroflow`, `retrieve`, `commit`, `rollback`, `delete`

**Key insight:** Only microflows that are bound to UI elements (page buttons, data sources,
on-click actions) appear in `operations.json`. Standalone microflows with no UI binding are
**not callable** via `runtimeOperation`.

### The `executeaction` Problem

The `executeaction` XAS action takes `actionname` (a microflow qualified name) and `applyto`
(`"none"`, `"selection"`, or `"set"`). However, in practice it fails with HTTP 560 and this
server-side error:

```
connector: An error has occurred while handling the request. : No value found for 'null'
java.util.NoSuchElementException: No value found for 'null'
    at scala.Enumeration.$anonfun$withName$1(Enumeration.scala:159)
    at com.mendix.webui.requesthandling.helpers.ContextHandling.inContext(ContextHandling.scala:31)
```

The `ContextHandling.inContext` method tries to resolve a form/page context enum that is `null`
when calling from curl (no active page). The Mendix client always sends requests from within a
page context; raw API calls lack this context.

**Conclusion:** Direct microflow execution via the XAS `/xas/` endpoint is **not feasible**
from external tools without a full browser session and page context. The `executeaction` action
is designed for in-browser use only.

### The `runtimeOperation` Approach

The `runtimeOperation` action executes by `operationId` from `operations.json`:

```bash
curl -s -b cookies.txt -X post http://localhost:8080/xas/ \
  -H 'Content-Type: application/json' \
  -H 'X-Csrf-Token: TOKEN' \
  -d '{
    "action": "runtimeOperation",
    "operationId": "+CCdBxEj71SVLZ3SEMdAwA",
    "params": {},
    "options": {},
    "changes": {},
    "objects": []
  }'
```

This also fails with the same `ContextHandling` error when called from curl, for the same reason:
no page context.

---

## M2EE Admin API — Microflow Execution

The M2EE admin API (port 8090) does **not** have a direct microflow execution action.

**Confirmed actions:** See `proposal-runtime-admin-port.md` for the full list. Key ones:
`about`, `runtime_status`, `reload_model`, `update_styling`, `preview_execute_oql`,
`enable_debugger`, `disable_debugger`, `create_admin_user`, `set_log_level`.

**Not available:** `execute_microflow`, `execute_action`, `run_microflow`, or any variant.
All attempts return `{"result":-5,"message":"action not found."}`.

The `create_admin_user` action is useful for resetting the admin password:

```bash
curl -s -X post http://localhost:8090/ \
  -H "content-type: application/json" \
  -H "X-M2EE-authentication: $(echo -n 'AdminPassword1!' | base64)" \
  -d '{"action":"create_admin_user","params":{"password":"NewPassword123"}}'
```

---

## After-Startup Microflow Testing (Recommended Approach)

Since neither the XAS protocol nor the M2EE admin API support direct microflow execution from
external tools, the **after-startup microflow** pattern is the most reliable way to test
microflows generated by MDL scripts.

### How It Works

1. Create a test runner microflow that calls the microflows under test
2. Set it as the project's after-startup microflow
3. Build and start (or restart) the runtime
4. Check the runtime logs for test results
5. Verify database side-effects via OQL

### Step-by-Step

**1. Create a test runner microflow:**

```sql
create or replace microflow MfTest.TestRunner ()
returns boolean as $success
begin
  log info node 'TEST' '=== Test Runner Starting ===';

  -- Test basic microflow
  $r1 = call microflow MfTest.M001_HelloWorld();
  log info node 'TEST' 'M001_HelloWorld: PASS';

  -- Test with parameters
  $r3 = call microflow MfTest.M003_StringOperations(
    FirstName = 'John', LastName = 'Doe'
  );
  log info node 'TEST' 'M003_StringOperations: result = ' + $r3;

  -- Test entity creation
  $product = call microflow MfTest.M012_CreateEntity(
    Name = 'TestProduct', Code = 'TP-001'
  );
  commit $product;
  log info node 'TEST' 'M012_CreateEntity: PASS';

  -- Test retrieve
  $list = call microflow MfTest.M025_SimpleRetrieve();
  log info node 'TEST' 'M025_SimpleRetrieve: PASS';

  log info node 'TEST' '=== Test Runner Complete ===';
  declare $success boolean = true;
  return $success;
end;
/
```

**Important:** The after-startup microflow **must return Boolean**. Mendix enforces this.

**2. Set as after-startup and deploy:**

```bash
# create the test runner microflow
mxcli exec test_runner.mdl -p app.mpr

# set as after-startup
mxcli -p app.mpr -c "alter settings model AfterStartupMicroflow = 'MfTest.TestRunner'"

# build and restart
mxcli docker build -p app.mpr --skip-check
mxcli docker down -p app.mpr
mxcli docker up -p app.mpr --detach --wait
```

Note: `docker reload` does **not** re-run after-startup. A full container restart is required.

**3. Check the runtime logs:**

```bash
mxcli docker logs -p app.mpr | grep "TEST\|after-startup\|error"
```

Expected output:

```
Core: Running after-startup-action...
TEST: === Test Runner Starting ===
TEST: M001_HelloWorld: PASS
TEST: M003_StringOperations: result = John Doe
TEST: M012_CreateEntity: PASS
TEST: M025_SimpleRetrieve: PASS
TEST: === Test Runner Complete ===
Core: Successfully ran after-startup-action.
```

If any microflow has a runtime error (bad BSON, missing references, type mismatches), it will
appear as a stack trace between the "Starting" and "Complete" lines.

**4. Verify database state via OQL:**

```bash
mxcli oql -p app.mpr "select Name, Code from MfTest.Product where Code = 'TP-001'"
```

### Security Configuration

For testing, set the project security to OFF to avoid needing access rules on every microflow:

```sql
alter project security level off;
```

This eliminates the need for module roles and access grants on MfTest microflows. Remember to
restore security before production deployment.

### What This Tests

The after-startup approach validates:

- **BSON structure** — The runtime loads and deserializes every microflow without errors
  (MxBuild catches most issues, but some only surface at runtime)
- **Microflow logic** — Variables, control flow, loops, arithmetic, string operations
- **Entity operations** — CREATE, CHANGE, COMMIT, RETRIEVE, DELETE
- **XPath navigation** — `$entity/attribute`, association traversal
- **Sub-microflow calls** — CALL MICROFLOW with parameters and return values
- **List operations** — HEAD, TAIL, FILTER, SORT, UNION, etc.
- **Error handling** — ON ERROR CONTINUE/ROLLBACK/custom handler
- **Database integration** — Entities are persisted and retrievable via OQL

### What This Does NOT Test

- **Page integration** — Show Page, Close Page, widget data sources
- **Security rules** — Access is bypassed with security OFF
- **Client-side logic** — Nanoflows, pluggable widget behavior
- **REST calls** — External HTTP calls (these execute but depend on external services)
- **Concurrent execution** — Single-threaded after-startup context

---

## Verified Test Results (Feb 2026)

Tested on Mendix 11.6.3, Docker (eclipse-temurin:21-jre + PostgreSQL 17-alpine), ARM64.

**Input:** 89 microflows from `mdl-examples/doctype-tests/02-microflow-examples.mdl`

**Build result:** `build SUCCEEDED` — all microflow BSON structures valid.

**Runtime result:** All tested microflows executed correctly:

| Microflow | Category | Result |
|-----------|----------|--------|
| M001_HelloWorld | Basic return | `true` |
| M002_BasicVariables | Arithmetic (10 * 5) | `50` |
| M003_StringOperations | String concat | `John Doe` |
| M004_BooleanLogic | AND logic | `true` |
| M006_SimpleIf | IF condition (20 > 10) | `value is greater than 10` |
| M007_IfElse | IF/ELSE (200 > 100) | `High` |
| M008_NestedIf | Nested IF (score 85) | `B` |
| M009_Logging | LOG with NODE | Logs visible in runtime output |
| M010_ArithmeticOperations | div operator (100 / 4) | `25` |
| M012_CreateEntity | CREATE object | Product created and committed |
| M015_UpdateEntity | CHANGE + COMMIT WITH EVENTS | `true` |
| M016_XPathAttributeAccess | XPath `$Product/Name` | `UpdatedProduct` |
| M025_SimpleRetrieve | RETRIEVE FROM entity | List returned |
| M035_MultipleReturnPaths | Three IF paths (-5, 0, 42) | `negative, zero, positive` |
| M038_1_TestCallWithResult | Sub-microflow call chain | `true` |

**Database verification:** OQL confirmed `Name=UpdatedProduct, Code=TP-001, IsActive=true`.

---

## Docker Credential Fix (DevContainer)

When using Docker-in-Docker in a devcontainer, `docker pull` may fail with:

```
error getting credentials - err: exit status 255, out: ``
```

This is caused by a devcontainer credential store helper that doesn't exist inside the container.
Fix by clearing the Docker config:

```bash
echo '{}' > ~/.docker/config.json
```

The devcontainer sets `credsStore` to a helper like `dev-containers-c4ce6673-...` which isn't
available inside the DinD context. Clearing this allows anonymous pulls from Docker Hub.

---

## Future Work

1. **`mxcli test` command** — Automate the after-startup test pattern: create test runner,
   set after-startup, build, restart, parse logs, report pass/fail. See `proposal-runtime-admin-port.md`
   for the full testing roadmap.

2. **Published REST approach** — `create published rest service` in MDL would allow direct
   HTTP calls to microflows, bypassing the XAS context requirement.

3. **XAS protocol improvements** — If Mendix adds a context-free `executeaction` variant
   (or an M2EE admin API `execute_microflow` action), direct microflow testing becomes trivial.
