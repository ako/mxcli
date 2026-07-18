# Warm Local Dev Loop — `mxcli run --local`

## Overview

`mxcli run --local` runs a Mendix app **without Docker**, keeping a
`mxbuild --serve` process and a standalone runtime hot so model changes apply in
~1 s instead of a ~30–60 s rebuild-and-restart. Use it as the fast inner loop when
iterating on an app; use `mxcli docker run` when you need the fully-rendered browser
client (see the limitation below).

## When to Use This Skill

Use this when:
- You want the fastest edit → running-app loop for a Mendix 11.x project.
- You're driving the model programmatically (`mxcli exec`/MDL) and want each change
  live immediately.
- You're iterating on **page design** (the app renders in a real browser) or doing
  runtime/model/API/headless verification.

Prefer `mxcli docker run` when:
- The project is Mendix 9/10 (JDK 11/17 — not yet supported by `run --local`).
- You want a container-parity deployment rather than a standalone runtime.

## Usage

```bash
# boot once and keep serving (Ctrl-C to stop)
mxcli run --local -p app.mpr

# boot and hot-apply on every project change
mxcli run --local -p app.mpr --watch
```

## How apply is chosen

Every warm rebuild reports whether a restart is required; `run --local` applies the
cheapest action automatically:

| Change | Apply | Cost |
|--------|-------|------|
| page / microflow / nanoflow / text | hot `reload_model` (no restart) | ~1 s |
| entity / view entity / association | runtime restart + DDL | ~9 s |

Structural changes need a restart because the runtime reconciles its entity/
association catalog only at startup; behavioural changes are hot-reloaded.

## Prerequisites

- **Mendix 11.x** project (runtime launches under **JDK 21**).
- A reachable **PostgreSQL** with the database already created. In the devcontainer:

  ```bash
  createdb -h 127.0.0.1 -U mendix "$(basename app.mpr .mpr | tr '[:upper:]' '[:lower:]')"
  ```

  Defaults: `127.0.0.1:5432`, user `mendix`, db derived from the project name.
  Override with `--db-host/--db-name/--db-user/--db-password`. If the database is
  unreachable the command stops with an actionable message (it does not provision it).

## The intended loop

```bash
# terminal 1: keep the app hot
mxcli run --local -p app.mpr --watch

# terminal 2 (or an agent): edit the model — the change hot-applies automatically
mxcli exec add-page.mdl -p app.mpr
```

`--watch` observes the project directory (MPR v1 and v2 layouts), ignoring
`deployment/`, `.git`, and `node_modules`.

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker |
| `--watch` | off | Rebuild + hot-apply on each change |
| `--app-port` / `--admin-port` / `--serve-port` | 8080 / 8090 / 6543 | Ports |
| `--db-host` / `--db-name` / `--db-user` / `--db-password` | 127.0.0.1:5432 / derived / mendix / mendix | Database |

## Pages render in the browser

`run --local` bundles the browser client (`web/dist/`) with mxbuild's rollup tooling
after the deploy build, so the app renders in a real browser (verified with
Playwright + the devcontainer's Chromium). Pair it with Playwright screenshots for a
pixel-perfect page loop: edit → auto-rebuild → re-screenshot.

**Watch cost:** in `--watch` the client bundle is rebuilt on every change (~6–7 s full
bundle today). Behavioural changes (microflows) don't strictly need it; making the
client rebuild incremental/conditional is a tracked optimization.

## Validation checklist

- [ ] Project is Mendix 11.x.
- [ ] Postgres is running and the target database exists.
- [ ] `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/` returns `200`,
      and `.../dist/index.js` also returns `200` (client bundle served).
- [ ] With `--watch`, editing a microflow logs `applied via reload`; adding an entity
      logs `applied via restart` and creates the table in Postgres.
