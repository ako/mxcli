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

> **Search caching.** The marketplace Content API has no server-side search, so the first `search` fetches the full catalog listing and caches it under `~/.mxcli/marketplace-catalog-<profile>.json` for a day. After that, searches (for any keyword) are served from the cache instantly. Pass `--refresh` to bypass the cache and re-fetch (e.g. to pick up a brand-new module).

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
| **Module** (new) | Imports it via `mx module-import` (requires a matching mxbuild — run `mxcli setup mxbuild -p app.mpr` if missing). **Refused on an MPR v2 project** — see below. |
| **Module** (already present) | **Reported, not modified** — see below. |
| Theme / Starter App / Sample | Downloaded to disk with import instructions (import via Studio Pro). |

## Module import is refused on MPR v2 projects

`mx module-import` rewrites an MPR v2 project as v1. Measured on a blank Mendix 11.12.1 app, a single import turned a 69 KB `.mpr` plus 341 `.mxunit` files into one 14 MB SQLite blob with no `mprcontents/` — and the same was observed independently on 11.13.0, so it is not version-specific:

```text
before   .mpr     69,632 bytes  +  341 .mxunit   tables: Unit, _MetaData, _Transaction
after    .mpr 14,295,040 bytes  +    0 .mxunit   tables: Unit, _MetaData
```

That is not cosmetic. The v2 layout is what makes the model diffable and mergeable per document: it is what [`mxcli diff-local`](../tools/diff.md) reads, and what makes an idempotent re-run observable as "no files changed". The conversion is **one-way** — `mx convert` targets Mendix *versions*, not storage formats.

So `install` refuses rather than warning:

```text
refusing to import: app.mpr uses the MPR v2 storage format, and 'mx module-import'
would rewrite it as v1.
...
  - Import the module in Studio Pro, which preserves the format; or
  - pass --allow-format-change to accept the conversion to MPR v1.
```

Import the module in **Studio Pro**, which preserves the format. If your project is not kept in git and the v1 layout is fine, `--allow-format-change` accepts the conversion — and the command then states plainly that the project is now v1.

## Updating an existing module

Updating a module that is **already in the project is not done automatically**. `install` detects it, reports the installed and target versions, and stops:

```text
Module "DatabaseConnector" is already installed (version 7.0.1).
Target version: 7.0.3.
In-place module updates are not applied automatically (they can discard local
edits and change persistent-entity IDs, which loses data). Update via Studio Pro.
```

Before you update in Studio Pro, the question worth answering is **whether anyone has edited the module since it was installed** — because the update will not ask. That is what `marketplace diff` is for; see below.

Two reasons make automatic in-place module updates unsafe:

1. **Local edits.** Teams sometimes modify a marketplace module after importing it; a blind re-import would discard those changes.
2. **Persistent-entity IDs.** A fresh import assigns new entity `$ID`s. The runtime database keys data by entity ID, so re-importing a module with persistent entities would make the runtime treat them as *different* entities — **losing data**.

Studio Pro's Marketplace **Update** performs an ID-preserving merge that the `mx` CLI does not expose, so module updates are left to Studio Pro for now.

## Has this module been edited? (`marketplace diff`)

Studio Pro's Marketplace **Update** replaces the module and discards local edits without asking. `marketplace diff` answers the question that decides whether that is safe:

```bash
# What have I changed in this module since installing it?
mxcli marketplace diff 23513 -p app.mpr
```

```text
Administration — installed 4.3.2 (Mendix 11.12.1)

  Locally modified (1 of 21 elements):
    changed   ENTITY Account
```

An untouched module reports how much was actually checked, not just a verdict:

```text
  No local modifications: 21 of 21 elements verified unchanged.
```

### What it does

1. Reads which marketplace version each module in the project records. The project stores the marketplace **version UUID** per module, so the module and the exact release it came from are both identified without guessing — the listing name does not help here (content 23513 is listed as "Administration module" and installs a module called `Administration`; "Data Widgets" installs `DataWidgets`).
2. Downloads that version's `.mpk` and imports it into a throwaway reference project built **at the project's own Mendix version**, so the package goes through the same conversion the installed copy did.
3. Describes every element of the module on both sides and compares the descriptions.

Comparison is on `DESCRIBE` output rather than raw storage: an *untouched* module differs from its own published package in thousands of BSON paths, because the installed copy carries subtrees the package does not.

Requires the mxbuild toolchain for the project's Mendix version — `mxcli setup mxbuild -p app.mpr`. Building the reference at a *different* version is refused rather than warned about, because Mendix's own conversions would then show up as your edits.

### Theme modules

`mx module-import` refuses a theme module outright ("Importing theme module is not supported"), which would take Atlas_Core, Atlas_Web_Content and Conversational UI off the table. The refusal is gated on a single flag on the module document inside the package, so `diff` clears it on **its own throwaway copy** before importing — the published package and your project are untouched.

Atlas modules are among the most-edited in real projects, so this matters more than the module count suggests:

```text
Atlas_Web_Content — installed 4.1.0 (Mendix 11.12.1)

  No local modifications found, but 46 of 89 elements could not be read —
  this is not a clean bill of health.

  Not comparable (46) — reported as unknown, never as unchanged:
    unknown   PAGE_TEMPLATE Blank (no DESCRIBE support for PAGE_TEMPLATE)
    ...
```

The 46 are page templates, which have no `DESCRIBE` handler yet. They are reported rather than quietly counted as unchanged — see [`CATALOG.PAGE_TEMPLATES`](../tools/catalog-tables.md).

### What an upgrade would touch

`--to` adds the other half of the question: what the module's author changed, and whether it collides with what you changed.

```bash
mxcli marketplace diff 23513 -p app.mpr --to 4.5.0
```

```text
  Upgrading to 4.5.0 would touch 5 element(s), 1 of which you have modified:
    CONFLICT  ENTITY Account

  Studio Pro's update would discard those local edits without asking.
```

### In CI

`--json` emits the machine-readable form, for a build gate that fails when a marketplace module has been edited:

```bash
mxcli marketplace diff 23513 -p app.mpr --json
```

```json
{
  "module": "Administration",
  "installedVersion": "4.3.2",
  "mendixVersion": "11.12.1",
  "locallyModified": true,
  "verified": true,
  "modified": ["ENTITY Account"],
  "unchangedCount": 20
}
```

Read **both** `locallyModified` and `verified`. `verified: false` means at least one element could not be described, so "no modifications found" is not a conclusion you can act on — an element that cannot be read is reported as `unknown`, never as unchanged:

```text
  No local modifications found, but 1 of 21 elements could not be read —
  this is not a clean bill of health.
```

### Flags

| Flag | Purpose |
|---|---|
| `-p, --project` | The project holding the installed module (required). |
| `--to <version>` | Also report what upgrading to this version would touch, and which of those you have modified. |
| `--module <name>` | Name the module explicitly, when the project records no marketplace version for it (a hand-imported copy) or several modules match. |
| `--json` | Emit JSON instead of text. |
