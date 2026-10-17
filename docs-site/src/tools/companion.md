# Claude Code Companion

The mxcli companion is a [Claude Code](https://code.claude.com) mod: a pane inside
your Claude Code session that shows the Mendix app you are building while the
agent builds it.

> **Early access.** Mods are an early-access Claude Code feature, and the companion
> is at its first slice. Its plan is in
> `docs/11-proposals/proposal-mxcli-companion-mod.md`.

## What it shows

- **Project** — the app's structure: modules, their domain model, folders and
  documents, project security, settings and navigation, as `mxcli project-tree`
  reports it.
- **MDL source** — select a document to see its MDL, as `mxcli describe` prints
  it. Folders and other containers list what they hold instead.
- **Domain model diagrams** — select a module's *Domain Model* node in the desktop
  app or VS Code to see its entities, attributes, associations and
  generalizations, drawn where Studio Pro places them (`mxcli describe --format
  svg domainmodel <Module>`). Hover a box for its attributes. The terminal draws
  no pictures and lists the entities instead.

In a wide pane (90 columns or more) the tree sits on the left and the source on
the right, like a file explorer; each column pages on its own with its ▲ / ▼
rows. A narrower pane stacks them.

The tree reloads by itself when the agent changes the model (`mxcli exec`, a
writing `-c` one-liner, `new`, `rename`, `marketplace`). Press **Refresh** after
changing the app elsewhere, for example in Studio Pro.

## Workflows

Prepared workflows hold each change to quality gates that mxcli checks. The
first is **Vibe code**: describe a change in a sentence and it lands with
security, a clean build, tests and a short change note.

- Start one from the banner, or type `/workflow-vibe <the change>`; `/workflows`
  lists them. The commands work in every Claude Code session, including the web,
  where the banner does not show.
- While a run is open, the banner shows its intent and where its gates stand.
- Under the hood: `mxcli workflow start | check | note | close | status`. A run
  is a file in `docs/brain/workflows/`; `check` records what each gate answered,
  and `close` is refused while a blocking gate fails. Security findings that were
  there before the change do not block it; new ones do.

## Getting it

`mxcli init` writes the companion to `.claude/skills/mxcli-companion/` in a project
set up for Claude Code, and `mxcli init --sync-skills` (run by the session hook
on every start) keeps it current with your mxcli. Claude Code loads it from there
once you trust the folder; there is nothing to install.

Start Claude Code from the project folder, then type:

```
/mxcli-project
```

The first time the companion loads in a project, a band above the prompt says it
is installed and offers **Open Project pane**. It does not come back once you
open the pane or press **Not now**.

A companion written during the current session — as the bootstrap prompt does —
loads after `/reload-plugins` or in the next session; Claude Code reads plugins
at session start.

## Options

Both are set in Claude Code's plugin configuration for `mxcli-companion`:

| Option | Default |
|---|---|
| `mprPath` | the single `.mpr` in the session folder, or one level down; with several, the pane asks |
| `mxcliPath` | `./mxcli` when present, else `mxcli` on `PATH` |

## Notes

- Every click runs mxcli, which reads the model. On a large app a document takes a
  few seconds the first time; going back to it is instant until the model changes.
- The companion only reads. It never changes the project.
- The pane draws on the surface that runs the session. A session started in a
  terminal and continued from the desktop app over remote control shows the pane
  in the terminal only.
- It is for Claude Code only; other tools get what `mxcli init` gives them today.
