# Download, Install and Update Marketplace Content

This skill covers the full lifecycle of Mendix Marketplace content (modules and widgets)
from the command line: discover → download → install → check for local edits → update.
These are **CLI commands**, not MDL statements.

## When to Use This Skill

- User wants to add a marketplace module or widget to a project
- User wants to upgrade a module that is already installed
- User asks whether a marketplace module has been edited locally, or what an upgrade would overwrite
- User asks to download a specific `.mpk` (e.g. for CI, or to import in Studio Pro)
- User asks which versions of a marketplace item are compatible with their Mendix version
- User reports `CE0463` right after installing or updating a module

## Prerequisites: Authenticate

Marketplace access needs a Mendix Personal Access Token (PAT), created at
<https://user-settings.mendix.com/> (Developer Settings → Personal Access Tokens).

```bash
mxcli auth login                 # interactive prompt for the PAT
mxcli auth login --token <PAT>   # non-interactive (CI)
export MENDIX_PAT=<PAT>          # or via environment
mxcli auth status                # verify it validates
```

Credentials are stored at `~/.mxcli/auth.json` (mode `0600`).

Module installs also need the mxbuild toolchain for the project's Mendix version:
`mxcli setup mxbuild -p app.mpr`.

## Step 1 — Discover

```bash
mxcli marketplace search "database connector"   # find content by name/publisher
mxcli marketplace info 2888                      # details for a content id
mxcli marketplace versions 2888                  # available versions
mxcli marketplace versions 2888 --min-mendix 10.24.0   # compatible versions only
```

The numeric **content id** (from `search`/`info`) is what every other command takes.

**Search caching.** The Content API has no server-side search, so the first `search`
fetches the whole catalog (tens of seconds) and caches it under `~/.mxcli/` for 24h;
later searches are instant. If the first search seems slow, it is scanning the catalog —
let it finish. Pass `--refresh` to bypass the cache (e.g. for a brand-new module). If
`search` returns nothing, the content may be private or listed under a different name —
look it up by id with `info <id>` (ids come from the marketplace URL
`.../link/component/<id>`).

**The listing name is not the module name.** Content 23513 is listed as "Administration
module" and installs a module called `Administration`; "Data Widgets" installs
`DataWidgets`. Never match a module to its marketplace listing by name — the commands
below identify it by the marketplace **version UUID** the project records per module.

Content ids that come up often:

| Content | Id | Installs module |
|---|---|---|
| Administration | 23513 | `Administration` |
| Community Commons | 170 | `CommunityCommons` |
| Data Widgets | 116540 | `DataWidgets` |
| Atlas Core | 117187 | `Atlas_Core` (theme module) |
| Atlas Web Content | 117183 | `Atlas_Web_Content` (theme module) |

## Step 2 — Download a `.mpk` to disk (optional)

```bash
mxcli marketplace download 2888                              # latest, CDN filename
mxcli marketplace download 2888 --version 7.0.2 -o dbc.mpk   # specific version + path
```

Use this when you only want the file (to commit to `mx-modules/`, or to import in Studio
Pro yourself). `download` needs no project; `install` does.

## Step 3 — Install into a project

```bash
mxcli marketplace install <content-id> -p app.mpr [--version X.Y.Z]
```

`install` is **type-aware**:

| Content type | Behaviour |
|---|---|
| **Widget** | Copied into `widgets/` (overwrites on update). |
| **Module** (new) | Copied in with mxcli's own writer — every unit, plus everything else the package ships (`widgets/`, `themesource/`, `javasource/`, ...). Preserves the project's storage format, and works for theme modules. |
| **Module** (already present) | **Reported, not modified.** Use `marketplace update` (step 5). |
| Theme / Starter App / Sample | Downloaded with import instructions (import via Studio Pro). |

Measured: CommunityCommons 11.5.1 into a vanilla 11.12.1 app — 128 units and 126 bundled
files, `mprcontents/` grew from 369 to 497 `.mxunit` files, `mx check` reports 0 errors.

### Do not use `mx module-import`

`mx module-import` rewrites an **MPR v2** project as v1: one import turned a 69 KB `.mpr`
plus 341 `.mxunit` files into a single 14 MB SQLite blob with no `mprcontents/`
(measured on 11.12.1 and again on 11.13.0). The conversion is one-way — `mx convert`
targets Mendix *versions*, not storage formats — and it takes `mxcli diff-local`, per-document
git diffs and mergeability with it. `mx module-import` also refuses theme modules outright
("Importing theme module is not supported").

`install` therefore copies the units itself. `--allow-format-change` selects the legacy
`module-import` path; without it, that path refuses to run on a v2 project rather than
converting silently.

### Dependencies are not resolved

`install` installs exactly the content you name. A module whose dependencies are missing
produces a large error count that only shrinks as you add them, and the count is **not
monotonic** — adding a module can raise it before it falls (observed on a real agentic
stack: 156 → 16 → 227 → 211 → 1 → 0). Install one module at a time, re-check after each,
and read the remaining errors to find the next missing dependency rather than treating a
rising count as a regression.

## Step 4 — Resync widget definitions (required after any headless install)

```bash
mxcli docker check -p app.mpr
```

A freshly installed or updated module's pages reference widget definitions the project has
not resynced, so a check reports **CE0463** ("the definition of this widget has changed")
until it is told to. Measured on Administration 4.3.2 → 4.5.0: 11 errors before the
resync, 0 after. This is expected after any headless module install — it is **not** a
mxcli defect, and it is not the CE0463 that `.claude/skills/diagnose-ce0463.md` is for.

**Never run bare `mx update-widgets` on an MPR v2 project.** It performs the resync and
converts the project to v1 in the process — measured on 11.12.1: 200 `.mxunit` files
became 0, and a 69,632-byte index became 14 MB. `mxcli docker check` runs the same
`mx update-widgets` step with the v2 storage snapshotted and restored around it, so the
check sees the resynced model and the project keeps its format.

The resync is therefore not *persisted*: the check passes, and the stored model still
holds the pre-resync widget definitions, so a later `mx check` reports CE0463 again.
To persist it, open the project in Studio Pro once and use **Update all widgets**.
`mxcli widget sync -p app.mpr` is the headless equivalent, but is **partial** — on the
reference fixture it clears 7 of 40.

## Step 5 — Before updating: has the module been edited?

Studio Pro's Marketplace **Update** replaces the module and discards local edits without
asking. `marketplace diff` answers the question that decides whether that is safe:

```bash
mxcli marketplace diff 23513 -p app.mpr                # what have I changed?
mxcli marketplace diff 23513 -p app.mpr --to 4.5.0     # ...and what would an upgrade touch?
mxcli marketplace diff 23513 -p app.mpr --json         # for a CI gate
```

```text
Administration — installed 4.3.2 (Mendix 11.12.1)

  Locally modified (1 of 21 elements):
    changed   ENTITY Account

  Upgrading to 4.5.0 would touch 5 element(s), 1 of which you have modified:
    CONFLICT  ENTITY Account
```

It downloads the installed version's `.mpk`, imports it into a throwaway reference project
built **at the project's own Mendix version** (a mismatch is refused, not warned about —
Mendix's own conversions would otherwise read as your edits), and compares `DESCRIBE`
output on both sides.

**Read `verified`, not just `locallyModified`.** An element that cannot be described is
reported as `unknown`, never as unchanged, and `verified: false` means "no modifications
found" is not a conclusion:

```text
  No local modifications found, but 46 of 89 elements could not be read —
  this is not a clean bill of health.
```

Flags: `-p/--project` (required), `--to <version>`, `--module <name>` (when the project
records no marketplace version for it, i.e. a hand-imported copy), `--json`,
`--profile`.

## Step 6 — Update an installed module

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
```

Flags: `-p/--project`, `--to <version>` (required), `--module <name>`,
`--save-edits <dir>`, `--force`, `--profile`.

### What it preserves, and why it matters

- **Element identity (`GUID`).** The runtime keys entities and attributes on the model's
  `GUID` — `mendixsystem$entity.id` holds it verbatim. A module whose documents are
  replaced without carrying the old `GUID`s is a *different* module to the database, and
  its tables are dropped on the next deploy. `$ID` renumbering is irrelevant here; `GUID`
  is everything. This is why deleting a module and re-importing it is never a valid
  update.
- **Role grants.** A user role's grant of a module role lives in the *project's* security
  document, not the module, so removing the module takes the grants with it and putting
  it back does not return them.
- **Everything else the package ships.** Widget binaries under `widgets/`, styling and
  design-property declarations under `themesource/`, and so on — only `project.mpr` and
  `package.xml` are manifest rather than payload. DataWidgets 3.11.3 replaces 49 such
  files; skipping them leaves the app running old widget code and reporting `CE6083` for
  design properties the module itself declares.

### Limits — state these to the user before running it

- **Local edits are not preserved.** `update` refuses when it finds any; `--save-edits`
  writes them out first; `--force` proceeds. Saved files are the element's **resulting
  state, not a diff**, so replaying restores additions and changes but not removals, and
  an element that could not be described has nothing to save (it is reported, not skipped).
- **No rollback.** Work on a copy, or have the project committed to version control first.
- **No dependency resolution** (same as `install`).
- **`update` does not run `mx check` itself** — do step 4 afterwards.

### Afterwards

```bash
mxcli docker check -p app.mpr     # resyncs widgets; expect 0 errors
mxcli diff-local -p app.mpr       # review what landed, per document
```

Measured after the resync: Administration 4.3.2 → 4.5.0 (28 units, 9 identities, 2 grants)
and DataWidgets 3.5.0 → 3.11.3 (49 files) both reach **0 errors**.

## Worked example: the agent-editor stack on a vanilla app

The seven modules in `.claude/skills/mendix/agents.md` must all be present before any
`create agent` statement will build. Install them one at a time, checking between:

| Listing | Id | Module |
|---|---|---|
| GenAI Commons | 239448 | `GenAICommons` |
| Mendix Cloud GenAI Connector | 239449 | `MxGenAIConnector` |
| Agent Commons | 240371 | `AgentCommons` |
| Agent Editor | 257918 | `AgentEditorCommons` |
| MCP Client | 244893 | `MCPClient` |
| Conversational UI | 239450 | `ConversationalUI` |
| Encryption | 1011 | `Encryption` |

```bash
mxcli new MyAgentApp --version 11.12.1
cd MyAgentApp
for id in 239448 239449 240371 244893 239450 1011 257918; do
  mxcli marketplace install "$id" -p MyAgentApp.mpr
  mxcli docker check -p MyAgentApp.mpr | tail -3
done
```

Install `AgentEditorCommons` (257918) **last** — it depends transitively on the other six.
Expect the error count to move non-monotonically until the last dependency lands.

The ids above were resolved with `mxcli marketplace search` and are a convenience, not an
authority: confirm with `search`/`info` rather than trusting them from memory, and note
that the listing name never matches the module name.

## Notes

- `install`/`update`/`diff` require `-p <app.mpr>`; `download` does not.
- All of them require `mxcli auth login` first; an expired or missing PAT gives an auth
  error with a login hint.
- Marketplace CDN TLS handshakes time out occasionally. Retry once before reporting a
  failure.
