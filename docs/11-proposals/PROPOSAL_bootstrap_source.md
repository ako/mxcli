---
title: Bootstrap hooks that fetch the mxcli the project was built with
status: proposed
date: 2026-08-18
related:
  - .claude/skills/mendix/bootstrap-app.md
  - docs-site/src/tools/bootstrap-prompt.md
---

# Bootstrap hooks that fetch the mxcli the project was built with

## Problem

`mxcli init` writes `.claude/bootstrap-mxcli.sh` with a hard-coded download of
`https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-<os>-<arch>`.

For a project pinned to a fork that is wrong, and wrong *silently*: after an idle
reap the next session comes back on a different mxcli than the one the app was
built with. Nothing announces the swap — the binary is there, it runs, and its
behaviour differs. A test project hit this and rewrote the generated script by
hand (mxcli-dbreplication, finding F2).

The same script also conflates two audiences that want opposite things:

- a **user** of a Mendix app wants a working `./mxcli` in seconds, from a
  release, with no toolchain;
- a **contributor to mxcli** wants the binary built from *their* checkout, which
  means a clone, an ANTLR jar, and `make build` — minutes, not seconds, and
  worth it because the point is to exercise local changes.

One script cannot be both. Today's is the first with no way to ask for the
second, so the fork case is served by hand-editing generated output — which the
next `mxcli init` overwrites.

## Proposal

Emit **two** bootstrap prompts rather than one parameterised script.

### 1. User bootstrap (default)

What `init` writes today, with one change: the release source is resolved rather
than hard-coded. In order of preference —

1. an explicit `--bootstrap-source <owner/repo>` passed to `init`;
2. the origin remote of the repository the running mxcli was built from, when
   that is discoverable and is not `mendixlabs/mxcli`;
3. `mendixlabs/mxcli` nightly, as now.

Rule 2 is what makes the fork case work without anyone having to know about the
flag: a binary built from `ako/mxcli` writes a hook that fetches from
`ako/mxcli`. The version it pins should be the version that wrote it, not
`nightly`, so a reap restores the *same* binary rather than the newest one.

### 2. Developer bootstrap (opt-in)

`mxcli init --bootstrap developer` writes a script that clones and builds:
installs the pinned ANTLR jar if `antlr4` is absent, `make build`, and falls back
to a release download when the source build fails so a broken tree does not leave
the session with no mxcli at all. `MXCLI_REPO` / `MXCLI_REF` override the source;
this is essentially the script the reporting project wrote by hand, promoted to
something `init` can emit.

## Open questions

- **Is the running binary's origin discoverable?** `main.Version` carries a
  commit, not a remote. This may need a build-time ldflag, which is cheap but is
  a change to the release pipeline rather than to `init`.
- **Which default for a project created by a fork build?** Rule 2 says "the
  fork", which is right for a fork's own test projects and wrong for someone who
  built from a fork once and wants the upstream release afterwards. The flag
  settles it; the question is which way the *unflagged* case should fall.
- **Cold-start cost.** The developer script takes minutes from a cold container.
  Whether that fits inside a SessionStart hook's budget has not been measured —
  the reporting project flagged the same worry as its OPEN-1.

## Not doing

Making one script switch on an environment variable. It reads as a single
supported path with a hidden mode, and the two paths differ in prerequisites,
runtime and failure modes — a user who accidentally triggers the developer path
gets a multi-minute clone and an ANTLR download they never asked for.
