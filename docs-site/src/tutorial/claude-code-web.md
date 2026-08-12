# Developing on the Web (Claude Code)

You can build a Mendix app with mxcli **entirely in the browser** — no local CLI,
no Studio Pro, no machine setup. [Claude Code on the web](https://claude.ai/code)
runs the agent in a cloud container against a GitHub repository; mxcli provisions
the whole toolchain inside that container on first run and commits the result so
future sessions come back up on their own. It works from a laptop, and from an
iPad or phone.

This page walks the **entire workflow**:

1. [Create a GitHub repository](#1-create-a-github-repository)
2. [Get a hub key for browser preview](#2-get-a-hub-key-for-browser-preview) (optional)
3. [Create a Claude Code environment](#3-create-a-claude-code-environment)
4. [Start the session with the bootstrap prompt](#4-start-the-session-with-the-bootstrap-prompt)
5. [Iterate — the warm loop, preview, and screenshots](#5-iterate)

> **New to how Claude Code on the web works** (sessions, environments, network
> policies)? See Anthropic's guide:
> <https://code.claude.com/docs/en/claude-code-on-the-web>.

## 1. Create a GitHub repository

Create a **new, empty** repository on GitHub (private is fine). This is where the
app will live and where every Claude Code session runs. You don't need a template
or any starter files — the bootstrap prompt starts from a truly empty repo and
runs the *current* mxcli, so there is nothing to keep up to date.

If you already have a Mendix project in a repo, you can skip the bootstrap prompt
and use `mxcli init` instead (step 4 notes how).

## 2. Get a hub key for browser preview

*Optional, but recommended — it's what lets you actually see the running app in a
browser tab from a web session.*

A cloud session has no localhost you can open, so mxcli can reverse-tunnel the
running app out to the **mxcli hub** and give you a public preview URL (see
[External browser preview](../tools/run-local.md)).
The hosted hub at `hub.mxcli.org` is GitHub-authenticated, so each preview is
private to you — which means you need a per-user **hub key**:

1. Open <https://hub.mxcli.org/cli> in any browser and **sign in with GitHub**.
2. Click **Create a hub key** and copy it.
3. Keep it for step 3 — you'll set it as `MXCLI_HUB_KEY`.

The key is durable (no expiry, survives hub restarts); you set it once and it
stays valid until you revoke it from the same page. If you'd rather run your own
hub, see [`mxcli tunnel-hub`](../tools/run-local.md).

You can skip this step and add the key later — without it, the app still builds
and runs in the container; you just won't have a browser preview URL.

## 3. Create a Claude Code environment

In Claude Code on the web, create an **environment** pointed at the repository
from step 1. The environment is the reusable container definition for your
sessions; configure two things on it:

**Environment variables**

| Variable | Value | Why |
|----------|-------|-----|
| `MXCLI_HUB_KEY` | the hub key from step 2 | mxcli reads it automatically so `run --hub` registers previews as you. Survives container reaping. |

Set the hub key **here, on the environment — never in a committed file.** It's a
credential, and a gitignored file wouldn't survive container recycling anyway;
environment variables are re-injected into every session, so the environment is
the only place it belongs.

**Network policy**

The bootstrap prompt downloads a few things on first run, so the environment's
network policy must allow outbound HTTPS to:

- **`github.com` / `objects.githubusercontent.com`** — the prebuilt `mxcli` binary
  from GitHub Releases.
- **Mendix's CDN** — MxBuild and the runtime (`mxcli` fetches the version your app
  needs).
- **`hub.mxcli.org`** — the browser-preview tunnel (only if you're using step 2).

A policy that permits general outbound HTTPS covers all of these. If your
organization restricts egress, allow-list those hosts. See the
[network policy docs](https://code.claude.com/docs/en/claude-code-on-the-web) for
where to configure this.

> **Even faster startup:** if you control the environment image, pre-installing
> `mxcli` (and pre-caching MxBuild + the runtime) makes step 4 nearly instant and
> removes the one network-dependent step. This is the most robust setup for a team.

## 4. Start the session with the bootstrap prompt

Open a session on the environment and paste the **bootstrap prompt**. It tells the
agent to set up the complete toolchain, create the app, wire the AI tooling and
Dev Container, **start a findings document**, boot the app, and commit everything
so the next session self-bootstraps.

👉 **[Copy the bootstrap prompt](../tools/bootstrap-prompt.md)** — it's a single
paste. In short, the agent will:

- ensure `mxcli` is available (pre-installed, or download the `nightly` binary);
- `mxcli init --sync-skills` — unpack the skills embedded in the binary, then follow
  the **`bootstrap-app`** skill, which carries the rest of this list (the paste itself
  is only those two steps plus "read the skill");
- interview you about the app, then `mxcli new App --version <X.Y.Z>` (or `mxcli init`
  if an `.mpr` already exists);
- `mxcli init --tool claude` — adds a **SessionStart hook** so future sessions come
  back up automatically;
- `mxcli run --local --setup --ensure-db` — cache MxBuild + runtime, start
  Postgres, create the app database;
- create a **`FINDINGS.md`** (see below) and start logging to it;
- **commit** `App.mpr`, `.devcontainer/`, and `.claude/` so the steady state is
  file-driven;
- boot `mxcli run --local` and verify the app answers HTTP 200.

You can append your own goal to the end of the prompt — e.g. *"Then seed a domain
model, pages, and microflows for a time-registration app"*, or link a design
prototype to seed from.

### Keep a findings document

The bootstrap prompt asks the agent to create a **`FINDINGS.md`** at the repo root
and to append to it as it works. Treat it as a lab notebook for the session:

- anything **surprising or broken** — an mxcli command that errored, a workaround
  you had to apply, a check that passed but a real `mx check` later flagged;
- the **Mendix / mxcli versions** in play and how each finding was verified;
- decisions and dead-ends, so a later session (or a teammate) doesn't repeat them.

This is high-leverage for two reasons. It gives *your next session* durable
context that survives container reaping, and — because mxcli is fast-moving alpha
— a clear findings doc is the single most useful thing you can share back to
improve mxcli. If something misbehaves, capture the exact MDL, the command, and
the error in `FINDINGS.md`; that's often enough for a fix.

## 5. Iterate

Once the app is up, use the **warm local dev loop** — a Docker-free, ~1-second
edit→test cycle:

```bash
mxcli run --local -p App.mpr --watch --screenshot   # hot-reload + a PNG per change
```

Edit the model with MDL and the running loop hot-applies it:

```bash
mxcli exec change.mdl -p App.mpr                     # the loop picks it up and reloads
```

To **see the app in a browser** from the web session, add `--hub` (this is where
the key from step 2 pays off):

```bash
mxcli run --hub https://hub.mxcli.org -p App.mpr     # prints a shareable preview URL
```

`--hub` implies `--local`, so you get the warm loop *and* a public preview URL in
one command — edit here, hot-apply, refresh the tab.

See [mxcli run --local](../tools/run-local.md) for `--watch`, `--ensure-db`,
`--setup`, the screenshot flags, and the full `--hub` reference.

## After idle: sessions self-bootstrap

Cloud containers are reaped when idle, so a resumed session starts from the
committed files, not from re-pasting the prompt. Because step 4 ran
`mxcli init`, `.claude/settings.json` carries a **SessionStart hook** that runs
`mxcli run --local --setup --ensure-db` on every new session — it re-caches
MxBuild + runtime and re-provisions the database, leaving the session ready to
`run --local`. You never re-paste the bootstrap prompt; it's a one-time seed.

## Next steps

- [The bootstrap prompt](../tools/bootstrap-prompt.md) — the exact copy-paste text
  and the rules that make it robust.
- [Local Dev Loop](../tools/run-local.md) — the warm loop, browser preview, and
  screenshots in depth.
- [Claude Code Integration](claude-code.md) — how skills, `CLAUDE.md`, and slash
  commands shape what the agent does with your project (applies to web and local).
