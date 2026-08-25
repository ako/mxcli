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
    local Postgres server if the port is down, and creates the app role + database
    if missing. It uses a service manager, or a user-owned `initdb`/`pg_ctl` cluster
    under `~/.mxcli/postgres` when no service becomes ready (e.g. Arch) — no
    `postgres` OS account or `sudo` required. For a non-local `--db-host` it only verifies
    reachability — mxcli won't provision a remote database.
    The user-owned cluster persists across sessions; its server log is
    `~/.mxcli/postgres/server.log`. Stop it with
    `pg_ctl -D "$HOME/.mxcli/postgres/data" stop`. To remove it, stop it first and
    then delete `~/.mxcli/postgres` (this permanently deletes its databases).
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
| `--hub-secret` | — | Shared auth (`user:pass`) matching an **open** hub's `--secret` |
| *(hub API key)* | — | For a **GitHub-authenticated** hub: get one from `https://<hub>/cli`, set `MXCLI_HUB_KEY` (see below) |
| `--watch` | off | Rebuild + hot-apply on every project change |
| `--ensure-db` | off | Provision local Postgres + the app database if missing (fresh-session bootstrap) |
| `--setup` | off | Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook |
| `--app-port` | 8080 | App HTTP port |
| `--admin-port` | 8090 | M2EE admin API port |
| `--serve-port` | 6543 | `mxbuild --serve` port |
| `--db-host` | 127.0.0.1:5432 | Database `host:port`; bracket IPv6 endpoints (`[::1]:5432`) |
| `--db-name` | derived from project | Database name |
| `--db-user` / `--db-password` | mendix / mendix | Database credentials |
| `--screenshot` | off | Capture a Playwright PNG after boot and each applied change |
| `--screenshot-path` | `<projectDir>/.mxcli/run-local.png` | Screenshot output PNG |
| `--screenshot-url` | app root | Page to shoot: full URL, or a path relative to the app root (e.g. `/p/customers`). Repeat for a multi-page set. |
| `--screenshot-user` / `--screenshot-password` | — | Log in once (Mendix form auth) and reuse the session, so pages behind login render authenticated |
| `--runtime-log` | `<projectDir>/.mxcli/runtime.log` | Runtime log file — JVM stdout/stderr **and** the application log (server stack traces + microflow `LOG` output); `-` disables |
| `--test-endpoint` | off | Host mxcli's token-guarded test endpoint so [`mxcli test … --attach`](running-tests.md) runs a suite against this app with no boot of its own. Installed before the boot (the handler registers from after-startup, so it cannot be added to a running app); your project's own after-startup microflow is chained, not displaced. Removed on exit. Tests then use **this app's database** |
| `--debug` | off | Enable the microflow debugger at boot; then use [`mxcli debug`](debug-microflows.md) from another terminal. Behaviour-neutral until a breakpoint is set |
| `--debug-pass` | `mxdebug` | Debugger password when `--debug` is set |
| `--metrics` | off | Register a Prometheus meter registry; metrics served at `http://127.0.0.1:<admin-port>/prometheus` |
| `--trace` | off | Enable OpenTelemetry tracing (bundled agent → runtime log) with default span filters |
| `--trace-service` | `.mpr` name | `OTEL_SERVICE_NAME` under `--trace` |
| `--trace-otlp` | off (console) | Export traces to this OTLP collector endpoint (e.g. `http://127.0.0.1:4318`) instead of the console; implies `--trace` (needed for flame charts) |
| `--runtime-setting Key=Value` | — | Merge an extra runtime setting into the boot config (Value parsed as JSON when possible); repeatable |

## Metrics and OpenTelemetry

`--metrics` registers a **Prometheus** meter registry at boot, so
`http://127.0.0.1:8090/prometheus` (the admin port) serves the runtime's Micrometer
metrics (`connectionbus_*`, `handler_requests_total`, `sessions_*`, `taskqueue_*`, …).
For another registry, use `--runtime-setting`:

```bash
mxcli run --local -p app.mpr --metrics
mxcli run --local -p app.mpr --runtime-setting 'Metrics.Registries=[{"type":"otlp"}]'
```

These are flags rather than a post-boot call because the admin `update_configuration`
action **replaces** the whole config (no read-back), so a separate call would wipe the
DB/BasePath settings — `--metrics`/`--runtime-setting` merge into mxcli's single boot
`update_configuration`.

**Traces (`--trace`):** attaches the bundled OpenTelemetry Java agent to the runtime
JVM (console exporter → `runtime.log`) and applies default span filters — unfiltered
per-activity tracing is ~10× slower, so `--trace` ships
`OpenTelemetry._RuntimeSpanFilters=["CreateOrChangeVariable","Loop","Gateway","RetrieveFromCache"]`
(override via `--runtime-setting`).

```bash
mxcli run --local -p app.mpr --trace
tail -f .mxcli/runtime.log     # microflow spans: mx.microflow.name / mx.microflow.depth
```

The console exporter omits start/end timestamps and parent span IDs, so call trees
and durations can't be reconstructed from it — for flame charts, export to an OTLP
collector with `--trace-otlp <endpoint>` (implies `--trace`; sets
`OTEL_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`, and the
endpoint for you):

```bash
mxcli run --local -p app.mpr --trace-otlp http://127.0.0.1:4318
```

You can still set the OTEL env yourself for full control (`--trace` / `--trace-otlp`
won't override an exporter you've set):

```bash
export OTEL_TRACES_EXPORTER=otlp OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
mxcli run --local -p app.mpr --trace
```

## Debugging a server-side error

When a page action throws, the browser shows the generic Mendix error dialog with no
detail. The runtime log is written to `<projectDir>/.mxcli/runtime.log` (the path is
printed at boot) — `tail -f .mxcli/runtime.log` while you reproduce the action to see the
server stack trace and your microflow `LOG` output.

A standalone runtime attaches **no** log subscriber by default (a Studio Pro / m2ee run
does), so mxcli wires two sources into that file: it tees the runtime JVM's stdout/stderr,
and after start it attaches a Mendix **file log subscriber** so the application log
(microflow `LOG` output and server-side stack traces) lands there too — otherwise the file
would be nearly empty. The file is appended across restarts, each marked with
`=== runtime start … ===`; the subscriber is re-attached on each restart and never rotates
the file. Use `--runtime-log <path>` to relocate it or `--runtime-log -` to turn it off.

## External browser preview (`--hub`)

> **Linux builds only.** `--hub` and `mxcli tunnel-hub` are available in the
> **Linux** build of mxcli only.
>
> The tunnel exists to get a preview *out of a Linux container*, which is the only
> place it ever ran. Shipping it in the Windows and macOS binaries meant those
> binaries embedded a general-purpose tunnelling tool they could never use — and
> endpoint security noticed: Microsoft Defender flagged the Windows binary, and
> enterprise EDR (Defender for Endpoint, CrowdStrike, SentinelOne) flags this class
> of payload harder still. That blocked mxcli on exactly the managed corporate
> laptops most Mendix developers work on.
>
> So it is built for Linux only. On Windows and macOS the commands still exist and
> still show help, but fail with a message pointing you here. To use `--hub`, run
> mxcli **inside the project's devcontainer** (or any Linux container) — which is
> where the warm loop already runs. Everything else in `mxcli run --local` is
> unaffected.
>
> We deliberately did **not** hide the dependency to dodge the scanners; that would
> be dishonest and would make the binary less trustworthy, not more. The fix is not
> shipping the capability where it is not used. See
> [ADR-0009](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0009-tunnel-is-linux-only.md).


`--hub <url>` makes the running app reachable **in a browser at a public URL** — without
the app leaving this machine and without committing. It's for reviewing work-in-progress
from a phone or tablet, or from an egress-only environment such as Claude Code on the web.
The app stays local and a **reverse tunnel** dials *out* to a hub over 443; the hub
proxies browser requests back down the tunnel. Nothing is pushed — only live HTTP — and
because everything rides a single 443 connection, it works even from an egress-only proxy.

**You run your own hub — there is no hosted service.** Stand up `mxcli tunnel-hub` once on
a host you control (a small VPS with a domain), then point apps at it.

```bash
# on your VPS: *.example.com + hub.example.com -> this host, inbound 80+443 open
mxcli tunnel-hub --domain example.com --secret alice:s3cret

# where the app runs:
mxcli run --hub https://hub.example.com --hub-secret alice:s3cret -p app.mpr
#   -> registers and prints e.g. "Preview available at https://app.example.com"
```

The hub is **multi-tenant**: it fronts many previews at per-preview subdomains
(`<project>-<branch>.example.com`; `main`/`master` collapses to `<project>`) — across
projects, solutions, branches, and worktrees — with a sortable overview at
`https://hub.example.com/`. Each `run --hub` self-registers:

- **Project** and **branch** auto-detect from the `.mpr` name and git; override with
  `--hub-project`/`--hub-branch`, and `--hub-worktree` separates worktrees of one branch.
- **`--hub-prefix`** namespaces the hostname (org/solution/team/env) →
  `<prefix>-<project>-<branch>`; **`--hub-solution`** groups a solution's apps in the overview.
- The overview **groups previews by Claude Code session** (agent): each session lists the
  endpoints it exposed, links back to its `claude.ai/code` conversation, and shows its
  availability — a reaped/idle container turns **stale**, then **offline**. `run --hub`
  auto-detects the session from `CLAUDE_CODE_REMOTE_SESSION_ID` (override with
  `--hub-session` / `MXCLI_HUB_SESSION`). Past sessions are **retained** so you can see
  older ones: the hub persists a per-session endpoint history to `--sessions-file` (default
  `~/.mxcli/hub-sessions.json`, survives restarts) and prunes it after `--session-retention`
  (default 30 days). Re-registering keeps a **stable URL**.
- Each endpoint shows **first seen** alongside last-seen, last-used, and uptime. First seen
  is read from that persisted history, so it is the first time the endpoint was *ever*
  exposed — unlike uptime, it does not reset when an idle container is reaped and the
  preview reconnects.
- `--hub` **implies `--local`**, boots the runtime with `ApplicationRootUrl` set to the
  assigned URL (so the SPA and `originURI` cookie work), and the tunnel reconnects forever.
  Combine with `--watch` for the full remote loop: edit here → hot-apply → refresh the tab.

**Hub setup:** a wildcard `*.example.com` A record (and `hub.example.com`) pointed at the
VPS; inbound 80 + 443 open — a Let's Encrypt cert is issued per subdomain on demand.

### Authenticated hub (GitHub)

A hub started **without** the GitHub flags is open — the shared `--secret` gates
registration, so keep it to people you trust. A hub started **with** GitHub OAuth adds
per-user isolation: viewers sign in with GitHub and see only their own previews, and each
preview is owned by whoever registered it.

```bash
# create a GitHub OAuth App (callback https://hub.example.com/auth/github/callback), then:
mxcli tunnel-hub --domain example.com --secret alice:s3cret \
  --github-oauth-client-id <id> --github-oauth-client-secret <secret> \
  --session-secret "$(openssl rand -hex 32)" --audit-log ~/.mxcli/hub-audit.jsonl
```

With auth on, `--require-auth` (default) 302s an unauthenticated viewer to GitHub and
returns a 403 to a non-owner; `--require-auth=false` filters the admin listing but leaves
previews open. Hub API keys are stored durably (`--keys-file`, default
`~/.mxcli/hub-keys.json`) so they survive restarts.

**Get a key (any device, including Claude Code web/mobile):** open `https://hub.example.com/cli`,
sign in with GitHub, click **Create a hub key**, and set it as an environment/repo secret:

```bash
export MXCLI_HUB_KEY=<key>
mxcli run --hub https://hub.example.com -p app.mpr   # registers previews as you
```

The key is durable and does not expire — set it once. Manage it from the same page ("Revoke
all keys"), or headless with `mxcli auth hub login --token <github-pat>`. If registration
fails (unreachable hub, stale key), `run --hub` warns and continues as a normal local run
instead of aborting.

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

**A rebuild starts once the source stops changing, not on the first change.** An
`mxcli exec` of a real script rewrites the `.mpr` and many `mprcontents/*.mxunit` files
over several seconds; building on the first change would deploy whatever was on disk at
that instant — a half-applied model. The watcher therefore waits for a couple of quiet
polls before it builds, so a long `exec` produces one build of the finished model
rather than a build of the first file it touched.

This matters more than it used to. The old escape hatch was "run the script again", and
byte-idempotent `exec` closed it: re-running an already-applied script writes nothing,
so nothing re-triggers the watcher and the stale build has no way out. If you do need to
force a rebuild without changing anything, `touch` the `.mpr` — the signal is mtime.

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
the same shell — and poll separately.

**The refusal names the offending process.** On Linux mxcli resolves the port's
listener through `/proc` and prints its pid and command line, so recovery is one
command rather than a `pgrep` hunt:

```
port 8080 (app) is already in use.
  A stale process is silently adopted otherwise, so edits appear to do nothing
  (looks like a stale cache — it isn't).
  Held by pid 11893: /root/.mxcli/mxbuild/11.13.0/modeler/mxbuild --serve …
  That is a leftover from an earlier run that did not shut down cleanly
  (a kill -9 or a reaped container skips mxcli's own teardown).
    kill 11893
    # confirm it is gone: curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080   (want 000)
  Or run on different ports with --app-port (and --admin-port/--serve-port).
```

The two cases need opposite remedies, so they are reported differently. A **leftover
of a previous run** is safe to kill, as above. A **foreign** listener — anything mxcli
did not start — is not, and mxcli says so instead of offering a `kill`:

```
  Held by pid 4820: python3 -m http.server 8080
  That is not a process mxcli started, so it is not a leftover run —
  pick another port rather than killing it.
```

Off Linux, or when the listener belongs to another user (so `/proc/<pid>/fd` cannot be
read), it falls back to a generic hint.

A *graceful* stop needs none of this: each child — mxbuild's JVM, the runtime, the
rollup bundler — is started in its own process group and the whole group is killed on
Ctrl-C/SIGTERM. Seeing this error means the previous run never got to run its teardown:
`kill -9`, a crash, or a reaped container. Avoid `pkill -f 'mxcli run'` — that pattern
also matches the shell you type it into.

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
