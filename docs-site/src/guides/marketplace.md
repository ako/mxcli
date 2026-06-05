# Marketplace Content

How to discover, download, and install Mendix Marketplace content (modules, widgets) from the command line with `mxcli marketplace`.

## Authenticate first

Marketplace access needs a Mendix **Personal Access Token (PAT)**. Create one at
[user-settings.mendix.com](https://user-settings.mendix.com/) (Developer Settings → Personal Access Tokens), then:

```bash
mxcli auth login                 # prompts for the PAT
# or, non-interactively (CI):
mxcli auth login --token <PAT>
# or via the environment:
export MENDIX_PAT=<PAT>

mxcli auth status                # confirm it validates
```

Credentials are stored at `~/.mxcli/auth.json` (mode `0600`).

## Discover content

```bash
# search by name/publisher
mxcli marketplace search "database connector"

# show one item's details (by content id)
mxcli marketplace info 2888

# list available versions, optionally filtered by Mendix compatibility
mxcli marketplace versions 2888
mxcli marketplace versions 2888 --min-mendix 10.24.0
```

Each item has a numeric **content id** (shown by `search`/`info`); you pass it to `download` and `install`.

## Download a `.mpk`

```bash
# latest version, into the current directory under its CDN filename
mxcli marketplace download 2888

# a specific version, to a chosen path
mxcli marketplace download 2888 --version 7.0.2 -o ./mods/dbc.mpk
```

The download is atomic (written to a temp file and renamed), so a cancelled run never leaves a truncated `.mpk`.

## Install into a project

`install` downloads the content and places it according to its type:

```bash
mxcli marketplace install 20 -p app.mpr               # a widget
mxcli marketplace install 2888 --version 7.0.3 -p app.mpr   # a module
```

| Content type | What `install` does |
|---|---|
| **Widget** | Copies the `.mpk` into the project's `widgets/` folder (overwrites on update). Reload in Studio Pro or run `mx update-widgets` to pick it up. |
| **Module** (new) | Imports it via `mx module-import` (requires a matching mxbuild — run `mxcli setup mxbuild -p app.mpr` if missing). |
| **Module** (already present) | **Reported, not modified** — see below. |
| Theme / Starter App / Sample | Downloaded to disk with import instructions (import via Studio Pro). |

## Updating an existing module

Updating a module that is **already in the project is not done automatically**. `install` detects it, reports the installed and target versions, and stops:

```text
Module "DatabaseConnector" is already installed (version 7.0.1).
Target version: 7.0.3.
In-place module updates are not applied automatically (they can discard local
edits and change persistent-entity IDs, which loses data). Update via Studio Pro.
```

Two reasons make automatic in-place module updates unsafe:

1. **Local edits.** Teams sometimes modify a marketplace module after importing it; a blind re-import would discard those changes.
2. **Persistent-entity IDs.** A fresh import assigns new entity `$ID`s. The runtime database keys data by entity ID, so re-importing a module with persistent entities would make the runtime treat them as *different* entities — **losing data**.

Studio Pro's Marketplace **Update** performs an ID-preserving merge that the `mx` CLI does not expose, so module updates are left to Studio Pro for now.
