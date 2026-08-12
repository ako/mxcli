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
| **Module** (new) | Copies the module in with mxcli's own writer, preserving the project's MPR format, plus everything the package ships (widgets, themesource, ...). Requires a matching mxbuild — run `mxcli setup mxbuild -p app.mpr` if missing. |
| **Module** (already present) | **Reported, not modified** — see below. |
| Theme / Starter App / Sample | Downloaded to disk with import instructions (import via Studio Pro). |

## Why installs do not use `mx module-import`

`mx module-import` rewrites an MPR v2 project as v1. Measured on a blank Mendix 11.12.1 app, a single import turned a 69 KB `.mpr` plus 341 `.mxunit` files into one 14 MB SQLite blob with no `mprcontents/` — and the same was observed independently on 11.13.0, so it is not version-specific:

```text
before   .mpr     69,632 bytes  +  341 .mxunit   tables: Unit, _MetaData, _Transaction
after    .mpr 14,295,040 bytes  +    0 .mxunit   tables: Unit, _MetaData
```

That is not cosmetic. The v2 layout is what makes the model diffable and mergeable per document: it is what [`mxcli diff-local`](../tools/diff.md) reads, and what makes an idempotent re-run observable as "no files changed". The conversion is **one-way** — `mx convert` targets Mendix *versions*, not storage formats.

So `install` copies the module's units directly instead, which keeps the project in whatever format it already uses and also works for theme modules (`module-import` refuses those outright). Measured: CommunityCommons 11.5.1 into a vanilla 11.12.1 app — 128 units and 126 bundled files, `mprcontents/` grew from 369 to 497 `.mxunit` files, and `mx check` reports 0 errors.

`--allow-format-change` selects the legacy `module-import` path, which still refuses to run silently:

```text
refusing to import: app.mpr uses the MPR v2 storage format, and 'mx module-import'
would rewrite it as v1.
...
  - Import the module in Studio Pro, which preserves the format; or
  - pass --allow-format-change to accept the conversion to MPR v1.
```

If you take that route the command states plainly that the project is now v1.

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

## Updating a module (`marketplace update`)

`update` replaces an installed module with another published version, preserving the two things a plain replace destroys.

```bash
# Refuses if you have edited the module, naming what it would discard
mxcli marketplace update 23513 -p app.mpr --to 4.5.0

# Park those edits as re-executable MDL, then update over them
mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --save-edits ./local-edits
mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --force
mxcli exec ./local-edits/entity-Account.mdl -p app.mpr
```

```text
Administration updated 4.3.2 → 4.5.0
  28 units copied, 9 element identities preserved, 2 role grant(s) restored.

  Removed in 4.5.0 (1) — their database columns or tables will go on the next deploy:
    Account/MyLocalEdit
```

### What it preserves, and why

- **Element identity.** The runtime keys entities and their attributes on the model's `GUID`, so a module whose documents are replaced without carrying the old GUIDs is a *different* module to the database and its tables are dropped on the next deploy. Studio Pro transplants them; so does this.
- **Access.** A user role's grant of a module role lives in the project's security document, not the module, so removing the module takes the grants with it and putting it back does not return them.
- **Everything else the package ships.** A module is not only its model: the `.mpk` carries widget binaries under `widgets/`, styling and design-property declarations under `themesource/`, and whatever else it needs. All of it is replaced — only `project.mpr` and `package.xml` are manifest rather than payload. DataWidgets 3.11.3 replaces 49 such files, and skipping them leaves the app running old widget code and reporting `CE6083` for design properties the module itself declares.

It does not use `mx module-import`, which would rewrite an MPR v2 project as v1 and refuses theme modules. Units are copied with mxcli's own writer, so the project keeps its format.


### Local edits

Local edits are **not** preserved. `update` refuses when it finds any, `--save-edits` writes them out first, and `--force` proceeds. Two limits on the saved files:

- They are the element's **resulting state, not a diff**, so replaying restores additions and changes but not removals.
- An element that could not be described has nothing to save, and is reported rather than skipped.

### Afterwards

Run `mx update-widgets <project.mpr>`. A newer module's pages reference widget definitions the project has not resynced, so `mx check` reports CE0463 until told to — measured on Administration 4.3.2 → 4.5.0: 11 errors before, 0 after. This is expected after any headless module install, not a fault in the update.

Then `mxcli docker check -p <project.mpr>` and `mxcli diff-local -p <project.mpr>`. Measured after that resync: Administration 4.3.2 → 4.5.0 and DataWidgets 3.5.0 → 3.11.3 both reach **0 errors**.

`update` does **not** roll back. Work on a copy or have the project in version control.

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
