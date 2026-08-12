# Bootstrap prompt (empty repo → running Mendix app)

The **primary** way to start a Mendix + mxcli project from the web or an iPad — no
local CLI, no GitHub template to pick from a (short) mobile list. Open an **empty
repo** in Claude Code Web and paste the prompt below; the agent asks you what the app
is, then provisions everything and commits the result so future sessions
self-bootstrap.

The prompt itself is deliberately tiny — install mxcli, unpack its skills, hand over to
the `bootstrap-app` skill. Everything with detail in it lives in that skill, which
ships inside the binary, so the paste stays phone-sized and the procedure is versioned
with mxcli instead of with whatever text someone copied months ago.

Why a prompt instead of a GitHub template repo: the mobile "New repository" template
dropdown shows only a small subset of templates, and a template repo needs per-Mendix-
version upkeep. A prompt starts from a *truly empty* repo, runs *current* mxcli, and
can seed the model from a design prototype in the same session — nothing to maintain.

## The prompt

````text
This is an empty repo. Provision it as a Mendix app developed with mxcli.

1. Make sure `mxcli` is available. The environment may already have it; if not,
   download the prebuilt binary for your OS/arch and put it at `./mxcli`:

   ```bash
   curl -fsSL -o ./mxcli \
     https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-linux-amd64
   chmod +x ./mxcli
   ```

2. Unpack the skills that ship inside it — this needs no project, so it works in an
   empty repo:

   ```bash
   ./mxcli init --sync-skills
   ```

3. Read `.ai-context/skills/bootstrap-app.md` and follow it end to end. It begins by
   interviewing me about the app, so ask me those questions and wait for my answers
   before running anything else.

If I gave you a design to work from, use it as the source of truth for the model and
the pages: <paste or link a design here — otherwise ignore this line>.
````

That is the whole prompt. The procedure it used to spell out — the interview, the
provisioning steps, the multi-app deltas, the model proposal — now lives in the
**`bootstrap-app` skill**, which is embedded in the mxcli binary and unpacked by
step 2. Two things follow: the prompt is short enough to paste from a phone, and the
procedure is fixed by shipping a new mxcli rather than by asking everyone to re-paste
a longer prompt.

## What the skill does once it takes over

1. **Interviews you** — one app or a solution, app name, what the app is for, what it
   keeps track of, who logs in, theme, Mendix version. The app name comes first
   because it becomes the `.mpr` file name, the Studio Pro app name and the path baked
   into the SessionStart hook.
2. **Provisions** — `mxcli new` into a subfolder and moves it to the repo root (the
   root is where `.claude/` and `./mxcli` must live), `mxcli init --tool claude`, then
   `run --local --setup --ensure-db` to cache MxBuild + runtime and create the
   database.
3. **Writes the brief** — `README.md` (what is being built, in your words) and
   `FINDINGS.md` (anything surprising or broken, appended as work proceeds). These are
   what an idle-reaped session reads to know what it is working on.
4. **Commits, then boots and verifies** — HTTP 200 at `http://localhost:8080/`, plus
   an optional `run --hub` preview URL.
5. **Proposes the model in MDL and waits** — module, entities, roles, pages — before
   building anything.

For a solution repo it also covers the parts that bite: per-app ports, a hostname per
app so the two apps do not share one cookie jar, the root SessionStart hook that
`mxcli init` will not write for you, and wiring OData in dependency order.

## Which mxcli version gets installed

Prebuilt binaries are the working install path. CI publishes them on every `vX.Y.Z`
tag (latest is v0.16.0) **and** as a rolling `nightly` pre-release, with assets named
`mxcli-<os>-<arch>`.

- **`nightly` — recommended while mxcli is fast-moving alpha.** New features (the whole
  warm-loop surface: `run --local`, `--watch`, `--ensure-db`, `--setup`, screenshots)
  land in `nightly` before they reach a tagged release, so the bootstrap flow above
  needs it. Download `.../releases/download/nightly/mxcli-<os>-<arch>`, or once mxcli is
  present, `mxcli setup mxcli --tag nightly`.
- **`vX.Y.Z` — pin for reproducibility / stability.** The CI marks nightly a
  pre-release ("use tagged releases for production"). Download
  `.../releases/download/vX.Y.Z/mxcli-<os>-<arch>` or `mxcli setup mxcli --tag vX.Y.Z`.
  With no `--tag`, `mxcli setup mxcli` matches the mxcli already running it (nightly →
  `nightly`, `vX.Y.Z` → that release) — mainly useful for replicating a version onto
  another OS/arch (e.g. the Linux binary in a Dev Container), not the first install.
- **Environment pre-install** (the robust path) installs whatever the Claude Code Web
  image bakes in — the way to pin a known-good version fleet-wide.
- **`go install …@latest` does not work yet.** The module *is* public (tags v0.1.0–
  v0.16.0), but the generated ANTLR parser (`mdl/grammar/parser/`) is gitignored and
  not committed, so a `go install` from the tagged source fails on the missing package.
  Building from source works only via `make build`/`make release` (which run
  `make grammar` first). Enabling `go install` would require committing the generated
  parser (or generating it during module build) — a maintainer decision.

## Which Mendix version to ask for

The skill defaults to the newest version that has a published MxBuild — everything
mxcli does starts with downloading it, so "supported" means "on the CDN". It runs this
check itself when asked for a newer version, and it is the check to run before bumping
the default:

```bash
curl -sI -o /dev/null -w '%{http_code}\n' https://cdn.mendix.com/runtime/mxbuild-11.13.0.tar.gz   # 200
curl -sI -o /dev/null -w '%{http_code}\n' https://cdn.mendix.com/runtime/mendix-11.13.0.tar.gz    # 200 (runtime)
```

Both have to answer `200` — `run --local` needs the runtime tarball as well as
MxBuild. In a solution, give every app the **same** version: they share the
`~/.mxcli/mxbuild` cache, and a mismatch means a second multi-hundred-MB download and
two runtimes to keep straight.

## Two rules that make this robust

- **Committing the config is mandatory** (the skill's provisioning step 6). The prompt
  is a *one-time seed*. Its output — `.mpr` + `.devcontainer/` + `.claude/` with the
  SessionStart hook and `bootstrap-mxcli.sh` — must be committed so the steady state is
  file-driven and deterministic. After that, every new session runs the hook
  automatically; you never re-paste the prompt. Miss the script and the hook has
  nothing to run after a reap.
- **mxcli delivery is an environment concern, not the prompt's.** The download in
  step 1 is the fragile part in a gated web session (a GitHub release `curl` may be
  blocked), and it is the one thing that cannot move into the skill — the skill is
  inside the binary. The robust fix
  is for the Claude Code Web **environment image / setup script to pre-install mxcli**
  (and pre-cache MxBuild + runtime); `go install` via `proxy.golang.org` is the fallback
  and needs mxcli published as a public Go module.

## After bootstrap — the inner loop

```bash
./mxcli run --local -p <AppName>.mpr --watch --screenshot   # warm dev loop + screenshots
./mxcli exec change.mdl -p <AppName>.mpr                     # edit the model; the loop hot-applies
```

In a solution, run one loop per app from its own folder, with the second app on the
alternate ports, and start the producer first so the consumer's external entities
resolve:

```bash
(cd backend  && ./mxcli run --local -p Backend.mpr --watch)
(cd frontend && ./mxcli run --local -p Frontend.mpr --watch \
                  --app-port 8180 --admin-port 8190 --serve-port 6643)
```

With `127.0.0.1 backend.local frontend.local` in `/etc/hosts`, browse them at
`http://backend.local:8080/` and `http://frontend.local:8180/` so each app gets its
own cookie jar.

See [mxcli run --local](run-local.md) for the warm loop, `--watch`, `--ensure-db`, and
the screenshot flags.
