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
- A **PostgreSQL** database. Defaults: `127.0.0.1:5432`, user `mendix`, database
  derived from the project file name (`App1112.mpr` → `app1112`). Two ways to have it:
  - **`--ensure-db`** (recommended for a fresh session) provisions it: starts the
    local Postgres service if the port is down, and creates the app role + database
    if missing (via a local `sudo -u postgres` superuser). For a non-local `--db-host`
    it only verifies reachability — mxcli won't provision a remote database.
  - Otherwise create it once yourself; without `--ensure-db`, `run --local` stops with
    an actionable message if the DB is unreachable:

    ```bash
    createdb -h 127.0.0.1 -U mendix app1112
    ```

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker (implied by `--hub`) |
| `--hub` | — | Expose the running app in a browser at a tunnel-hub URL (see [External browser preview](#external-browser-preview---hub)) |
| `--hub-secret` | — | Shared auth (`user:pass`) matching the hub's `--secret` |
| `--watch` | off | Rebuild + hot-apply on every project change |
| `--ensure-db` | off | Provision local Postgres + the app database if missing (fresh-session bootstrap) |
| `--setup` | off | Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook |
| `--app-port` | 8080 | App HTTP port |
| `--admin-port` | 8090 | M2EE admin API port |
| `--serve-port` | 6543 | `mxbuild --serve` port |
| `--db-host` | 127.0.0.1:5432 | Database `host:port` |
| `--db-name` | derived from project | Database name |
| `--db-user` / `--db-password` | mendix / mendix | Database credentials |
| `--screenshot` | off | Capture a Playwright PNG after boot and each applied change |
| `--screenshot-path` | `<projectDir>/.mxcli/run-local.png` | Screenshot output PNG |
| `--screenshot-url` | app root | Page to shoot: full URL, or a path relative to the app root (e.g. `/p/customers`). Repeat for a multi-page set. |
| `--screenshot-user` / `--screenshot-password` | — | Log in once (Mendix form auth) and reuse the session, so pages behind login render authenticated |

## External browser preview (`--hub`)

`--hub <url>` makes the running app reachable **in a browser at a public URL** — without
the app leaving this machine and without committing. It's for reviewing work-in-progress
from a phone or tablet, or from an egress-only environment such as Claude Code on the web.

```bash
# on a small VPS with a public IP + domain (hub.mxcli.org -> it, inbound 80+443 open):
mxcli tunnel-hub --domain hub.mxcli.org --secret alice:s3cret

# where the app runs (here):
mxcli run --hub https://hub.mxcli.org --hub-secret alice:s3cret -p app.mpr
#   -> "Preview available at https://hub.mxcli.org"
```

How it works: the app stays local and a **chisel reverse tunnel** dials *out* to the hub
over 443; the hub proxies browser requests back down the tunnel. Nothing is pushed — only
live HTTP flows. Because everything rides a single 443 connection, it works even from an
egress-only environment.

- `--hub` **implies `--local`**, and boots the runtime with `ApplicationRootUrl` set to
  the hub URL so the SPA and `originURI` cookie work under the public origin.
- Combine with `--watch` for the full remote loop: edit here → hot-apply → refresh the
  browser tab.
- The control connection honours `NO_PROXY` — an external hub goes through the egress
  proxy, a loopback hub connects directly.

`mxcli tunnel-hub` is the static relay: run it once on a small VPS to front your previews.
TLS is automatic via Let's Encrypt for `--domain` (needs inbound 80 + 443), or supply
`--tls-cert`/`--tls-key`. This slice fronts a single app; multi-tenant registration and
per-preview subdomains are planned.

## The change signal

`--watch` watches two **source** trees and rebuilds when either changes:

- the **model source** — the `.mpr` file and the `mprcontents/` document tree (v2); and
- the **theme source** — `theme/` (app-level `main.scss`, `custom-variables.scss`, …)
  and `themesource/<module>/web/` (per-module SCSS/CSS/JS).

It does **not** watch the whole project dir. This is deliberate: the serve/mxbuild
build rewrites `deployment/`, `theme-cache/`, and `.mendix-cache/` on every run, and
screenshots land in `.mxcli/`; watching only the source keeps that build-output churn
from re-triggering the loop. Both signals are **mtime polling** (default 1 s), so they
work on container filesystems where inotify does not fire — no watcher fd is involved.

Each applied change is logged with a **build generation** counter (`build #2`,
`build #3`, …; the boot build is `#1`), so "did my change take?" is answerable from
the log instead of guessed.

The intended cycle: an agent (or you) edits the model with `mxcli exec`/MDL — or edits
a theme `.scss` — and the running `run --local` picks it up and hot-applies it.

## Editing themes (SCSS): rebuild, don't clear caches

A theme edit (e.g. `theme/web/main.scss`) needs a **rebuild**, not a cache-clear.
`mxbuild --serve` recompiles the theme on its next `/build`, and that recompile
correctly picks up SCSS **content** changes — there is no incremental-theme cache to
clear (verified: one `/build` after an `main.scss` content edit changes
`theme-cache/web/theme.compiled.css`). So:

- **With `--watch`** — just save the `.scss`; the theme source is watched and the loop
  rebuilds and hot-applies automatically.
- **Without `--watch`** — nothing watches anything, so a save changes nothing in the
  running app. Trigger a rebuild: restart `run --local`, or use `--watch`.

Do **not** `rm -rf theme-cache/ .mendix-cache/ deployment/` — clearing caches is a red
herring. If a theme edit "won't show up", the cause is that no rebuild ran (Problem
above) or a **stale process is still serving** (below), never a stale compiled-CSS
cache.

## "My edit didn't show up" — it's usually a stale process, not a cache

`run --local` refuses to boot if its ports (`8080` app, `8090` admin, `6543` serve)
are already answering — because a previous `run --local` (or a stray `mxbuild --serve`
/ runtime) left alive would otherwise be **silently adopted**: the startup readiness
probes only check that the port answers, so a fresh run would attach to the old
process and keep serving old output. That reads exactly like a stale cache but is a
stale **process**.

If you started `run --local` in the background and the wrapping shell exited non-zero
(e.g. a chained `sleep`/`curl` that failed), the `run --local` process can die while
its `mxbuild --serve` + runtime keep serving on `:8080`. Launch `run --local` as the
**sole** command in its own invocation — don't chain a `sleep`/status check after it in
the same shell — and poll separately. To recover from a stale process:

```bash
pgrep -af 'mxbuild --serve|runtimelauncher|mxcli run'   # find them
kill <pid>                                              # stop each
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080  # want 000 (port free)
```

Then start `run --local` again. Or run on different ports with `--app-port` /
`--admin-port` / `--serve-port`.

## Pages render in the browser

`run --local` bundles the browser client (`web/dist/`) so the app renders in a real
browser — verified by driving the pre-installed Chromium with Playwright (the Mendix
homepage renders fully). This makes it usable for **visual page-design iteration**,
not just headless checks.

- **Non-`--watch`**: a one-shot rollup bundle after the deploy build (~7 s cold).
- **`--watch`**: a long-lived incremental bundler stays hot (the client-side mirror
  of `mxbuild --serve`), so a page/widget edit re-bundles in ~3–4 s. It runs with
  `CHOKIDAR_USEPOLLING` because inotify does not fire on container overlay
  filesystems — without it, change detection takes tens of seconds. The loop
  re-bundles **only when the edit touched client source**: a microflow/entity edit
  skips the bundle and just hot-reloads.

## Pixel-perfect page loop

Pass `--screenshot` and each applied change is captured to a PNG (default
`<projectDir>/.mxcli/run-local.png`) using Playwright's built-in `screenshot`
command (Chromium from `PLAYWRIGHT_BROWSERS_PATH` — no `playwright-cli` needed):

```bash
mxcli run --local -p app.mpr --watch --screenshot
# edit a page with mxcli exec/MDL -> auto rebuild -> re-bundle -> reload -> new PNG
```

**Deep links.** `--screenshot-url /p/customers` shoots a specific page instead of the
app root (a bare path is resolved against the app URL; a full `http(s)://…` is used
as-is).

**Multi-page sets.** Repeat `--screenshot-url` to shoot several pages after every
change — a visual-regression sheet. Each page gets its own PNG, named from the page
(`run-local-p-customers.png`, `run-local-home.png`):

```bash
mxcli run --local -p app.mpr --watch --screenshot \
  --screenshot-url / --screenshot-url /p/customers --screenshot-url /p/orders
```

**Pages behind login.** `--screenshot-user`/`--screenshot-password` log in once via
the Mendix login form (Playwright drives `#usernameInput`/`#passwordInput`/
`#loginButton`), save the session as a Playwright storage state, and reuse it for
every screenshot — so authenticated pages render. Login is best-effort: if no login
form appears (anonymous app) it proceeds unauthenticated.

```bash
mxcli run --local -p app.mpr --watch --screenshot \
  --screenshot-user demo_admin --screenshot-password '<pw>' \
  --screenshot-url /p/customer_overview
```

## Fresh sessions (Claude Code Web)

Background processes (Postgres, the JVM) are reaped on idle, so a resumed web session
needs to bring prerequisites back up. `mxcli init` emits a **SessionStart hook** into
`.claude/settings.json` that runs `./mxcli run --local --setup --ensure-db -p <app.mpr>`
on every session start — the non-blocking `--setup` mode caches MxBuild+runtime and
provisions the database, then exits, leaving the session ready to `run --local`.

To start from an **empty repo** on the web or an iPad, use the
[bootstrap prompt](bootstrap-prompt.md) instead of a GitHub template.

See also: [PROPOSAL_mxcli_dev_warm_loop](../../../docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md),
[mxcli docker run](docker-run.md), [Playwright Testing](playwright.md).
