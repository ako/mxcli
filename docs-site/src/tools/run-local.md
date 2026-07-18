# mxcli run --local

`mxcli run --local` runs a Mendix app in a **warm, Docker-free dev loop**. It keeps
a `mxbuild --serve` process and a standalone Mendix runtime hot, so after the first
(cold) build a model change rebuilds incrementally and is hot-applied — without a
full rebuild-image-and-restart cycle.

```bash
mxcli run --local -p app.mpr
mxcli run --local -p app.mpr --watch
```

## Why

The Docker path (`mxcli docker run`) rebuilds a full deployment package and restarts
the container on every change (~30–60 s). `run --local` instead:

- keeps the model loaded in `mxbuild --serve` — a warm rebuild is ~1 s;
- keeps the runtime process up and applies each change over the M2EE admin API;
- chooses the cheapest apply automatically from the build's `restartRequired` flag:

| Change | Apply | Cost |
|--------|-------|------|
| page / microflow / nanoflow / text | hot `reload_model` (no restart) | ~1 s |
| entity / view entity / association | runtime restart + DDL | ~9 s |

The metamodel catalog (entities/associations) is reconciled only at runtime startup,
so structural changes need a restart; behavioural changes do not.

## What it does

1. Detects the project's Mendix version.
2. Ensures MxBuild and the runtime are cached (downloads once, reused after).
3. Checks the database is reachable (it does **not** provision it — see below).
4. Starts `mxbuild --serve` and does the first (cold) build into `deployment/`.
5. Bundles the browser client (`web/dist/`) with mxbuild's rollup tooling — the
   serve Deploy target writes client *source* but not the bundle, so this step is
   what makes pages render.
6. Boots a standalone runtime against that deployment and serves it.
7. With `--watch`, rebuilds, re-bundles the client, and hot-applies on every
   project change until `Ctrl-C`.

## Requirements

- **Mendix 11.x** project. The runtime is launched under **JDK 21**; version-aware
  JDK selection for Mendix 9/10 is a follow-up.
- A reachable **PostgreSQL**, with the database already created. The devcontainer
  provides one. Defaults: `127.0.0.1:5432`, user `mendix`, database derived from the
  project file name (`App1112.mpr` → `app1112`). If the DB is unreachable, `run
  --local` stops with an actionable message rather than booting.

```bash
# devcontainer Postgres, one-time DB creation
createdb -h 127.0.0.1 -U mendix app1112
```

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker |
| `--watch` | off | Rebuild + hot-apply on every project change |
| `--app-port` | 8080 | App HTTP port |
| `--admin-port` | 8090 | M2EE admin API port |
| `--serve-port` | 6543 | `mxbuild --serve` port |
| `--db-host` | 127.0.0.1:5432 | Database `host:port` |
| `--db-name` | derived from project | Database name |
| `--db-user` / `--db-password` | mendix / mendix | Database credentials |

## The change signal

`--watch` watches the project directory (both MPR v1 single-file and v2
`mprcontents/` layouts), ignoring `deployment/`, `.git`, and `node_modules`. The
intended loop is: an agent (or you) edits the model with `mxcli exec`/MDL, and the
running `run --local` picks the change up and hot-applies it.

## Pages render in the browser

`run --local` bundles the browser client (`web/dist/`) after the deploy build, so
the app renders in a real browser — verified by driving the pre-installed Chromium
with Playwright against a booted app (the Mendix homepage renders fully). This makes
`run --local` usable for **visual page-design iteration**, not just headless checks.

**Watch cost.** In `--watch`, the client bundle is rebuilt on every change — a full
rollup bundle (~6–7 s) today. Behavioural changes (microflows) don't actually need
it, and page changes could use an incremental watch-mode bundler; making the client
rebuild incremental/conditional is a tracked optimization. For pixel-perfect page
work, pair this with Playwright screenshots (see below): edit → auto-rebuild →
re-screenshot.

See also: [PROPOSAL_mxcli_dev_warm_loop](../../../docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md),
[mxcli docker run](docker-run.md), [Playwright Testing](playwright.md).
