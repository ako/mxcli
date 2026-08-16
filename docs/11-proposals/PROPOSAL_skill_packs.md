---
title: Skill packs — shipping a skill that carries more than prose
status: partial
date: 2026-08-15
related:
  - cmd/mxcli/skills_content.go
  - cmd/mxcli/init.go
  - cmd/mxcli/init_skills_sync.go
  - cmd/mxcli/theme/assets.go
  - docs/13-decisions/0005-semantic-model-interface-currency.md
---

# Skill packs — shipping a skill that carries more than prose

## Problem

mxcli ships 65 skills today and every one of them is a single Markdown file. That
was never a design decision; it is what the mechanism can carry.

The [mxcli-ledger](https://github.com/ako/mxcli-ledger) project has produced two
blocks that do not fit:

| Pack | Carries |
|---|---|
| `mendix-vega-charts` | `SKILL.md`, 3 `references/*.md`, 7 spec templates with sample data (`specs/*.json`), a headless checker (`scripts/check-spec.mjs` + `package.json`) |
| `mendix-bulk-oql-dml` | `SKILL.md`, 2 `references/*.md`, an MDL file applying three Java actions (`mdl/oql-dml-actions.mdl`) |

Both exist for the same reason: the block is **easier for a coding agent than for
a person in Studio Pro** — a large JSON specification, a Java action that has to
be written and compiled. The agent absorbs the awkward part once, and the pack is
what stops it rediscovering the awkward part every time.

A third is wanted (`mendix-odata-pushdown`, Java actions that push `$filter` /
`$orderby` / `$top` / `$skip` into database-connector SQL) and there will be more.
Skills that carry assets is the general shape, not a special case for these two.

### mxcli cannot ship any of it

Four independent places block it, and each fails differently:

| Layer | Today | Failure |
|---|---|---|
| Embed | `//go:embed skills/*.md` | Flat glob, `.md` only. `.json`, `.mjs`, `.mdl` and subdirectories are not in the binary at all |
| Sync | `for f in .claude/skills/mendix/*.md` (Makefile) | Flat copy; nothing below the top level is seen |
| Write | 3 loops, `filepath.Join(dir, d.Name())` | **Basename.** Nesting is flattened, so `references/install.md` and `specs/install.md` would silently overwrite each other |
| Refresh | `syncAIContextSkills`: `if e.IsDir() { continue }` | Directory skills are skipped by `--sync-skills`, so a pack would never follow a binary upgrade — the bug fixed for flat skills in mxcli-todo #114 |

The write layer is the one to watch. It does not error on a nested pack; it
produces a plausible-looking directory with files missing or overwritten. That is
the failure mode this repo keeps meeting and keeps writing down: *the tool
accepting something it does not implement is worse than rejecting it*, because
every check comes back green.

## What a pack is, and why it is not just a bigger skill

**Packs are opt-in; skills are not.** This is the load-bearing distinction and it
belongs in the mechanism rather than in documentation.

The 65 current skills are pure prose. Writing them into every project is free and
reversible — worst case an agent reads a page it did not need.

A pack is not free:

- `mendix-vega-charts` requires **installing a custom pluggable widget** into the
  project, and re-namespacing it away from the ledger's `ledger.widget.web.*`.
- `mendix-bulk-oql-dml` **applies three Java actions to the model** via MDL, which
  is a model write with a build cost and a review surface.

Writing either into every `mxcli init` would be wrong. So a pack is an
**installable unit with a manifest**, and `mxcli init` keeps shipping exactly the
prose skills it ships today unless asked otherwise.

## Design

### Source of truth: vendored

Packs live in the mxcli repo and ship inside the binary, the same as skills,
commands and lint rules do now. Versioned with the binary, works offline, no
trust or caching questions. A third-party pack arrives as a PR.

A fetch/registry model was considered and rejected **for now**: it adds network,
provenance and cache-invalidation concerns to solve a problem nobody has yet
(there are three packs, all in-house). The manifest below is deliberately
sufficient for a fetched pack, so this does not foreclose it.

### Layout

```
.claude/skills/packs/<pack-name>/     source of truth, edited here
  pack.yaml                           manifest
  SKILL.md                            frontmatter name + description
  references/*.md                     loaded on demand
  specs/*.json  scripts/*  mdl/*.mdl  assets
```

Synced by `make sync-skill-packs` into `cmd/mxcli/skillpacks/` for embedding, the
same build-time flow the flat skills already use. **Edit the source, never the
embed dir.**

### The four mechanism changes

1. **`//go:embed all:skillpacks`.** The `all:` prefix is load-bearing, not
   defensive: a plain `go:embed` of a directory skips `_`- and `.`-prefixed files.
   `cmd/mxcli/theme/assets.go` carries the same prefix for exactly this reason —
   `_partial.scss` is how SCSS spells a partial, and the theme package lost them
   once already. A pack is just as likely to carry a `_helper.mjs` or a
   `.eslintrc`.

2. **Recursive sync.** `cp -R` preserving structure, with the existing
   `copy-if-changed` discipline so unchanged files do not invalidate the build
   cache.

3. **Write by relative path.** Strip the embed root, keep the remainder, create
   parents. The three near-duplicate walk loops in `init.go` (`.ai-context/`,
   `.opencode/`, `.vibe/`) collapse into one projector with per-agent targets;
   they have already drifted apart once.

4. **Prune on refresh.** Today's sync overwrites but never deletes. A pack that
   drops an asset in v2 would leave the v1 file behind forever, and a stale spec
   template is worse than a missing one because it looks current.

### Local edits are refused, not overwritten

A pack writes files into a project the user then owns. `theme/` solved this
already: generated regions are digest-fenced, and a block carrying local edits is
**refused rather than overwritten** — guard-don't-drop, the same principle as
[ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md).

Packs reuse it. `mxcli skill upgrade` reports what it refused and why; it never
silently reverts a spec the user tuned.

### The namespace has to be right before the build

A pluggable widget's id (`acme.widget.web.vegachart.VegaChart`) is its identity.
Two apps whose widgets share an id are two apps claiming the same widget, and the
symptom is not a build error — it is a widget resolving to somebody else's build.

So the widget source ships with **placeholders**, not a real namespace, and
`skill add` substitutes the destination project's:

| File | Carries |
|---|---|
| `widget/package.json` | `packagePath`, and the build's `projectPath` |
| `widget/src/package.xml` | the client-module file path |
| `widget/src/VegaChart.xml` | the widget id |

Three properties make a missed substitution impossible rather than unlikely:

1. **Placeholders, not a real namespace.** Leaving the harvested project's name
   in place means a bug ships *their* namespace silently; an unsubstituted
   `{{NAMESPACE}}` fails loudly.
2. **A whitelist, not a scan.** Only files named in `rewrite.files` are touched.
   A pack ships megabytes of built JS and spec JSON, and a blind replace is how a
   spec containing brace syntax quietly becomes something else.
3. **Drift in either direction is an error** — a declared file with no token
   (the file changed under the manifest) and a declared file the pack does not
   ship both refuse the install.

`skill upgrade` re-substitutes what the install recorded in `pack.lock.yaml`
rather than re-deriving. Re-deriving would change the id when the project is
renamed, and a changed widget id is every page pointing at a widget that no
longer exists under that name.

**Widgets ship as source, not as a built `.mpk`.** The built package is 3.1 MB of
bundled Vega, which has no business in a source repo or in the mxcli binary; and
the namespace has to be right *before* the build, so shipping a prebuilt package
would mean rewriting paths inside a zip and hoping, where rewriting source is the
path the ledger verified.

### A pack can place Java, and only Java, outside its own directory

`installs.java` is the third target and the only one that writes outside
`.claude/skills/<pack>/`. That is not a convenience: MDL cannot author a
standalone class — `createJavaActionStatement` accepts a **method body**, no
class declaration and no imports — so a pack whose actions delegate into helper
classes could ship only prose telling somebody to copy a directory by hand,
which is the manual step packs exist to remove.

A Java `package` is a class's identity exactly as a widget id is, so it reuses
the substitution machinery unchanged: `{{MODULE}}` and `{{MODULE_PATH}}`,
declared in `rewrite.files`, supplied by `--module`.

Three rules, each of which had a wrong default available:

1. **`java/actions/` is not placed.** mxcli writes those classes itself from the
   MDL, so placing the pack's copies means two sources of truth for the same
   files and applying the MDL overwrites them immediately. They stay in the pack
   to be read.
2. **An existing file that differs is refused, never overwritten** — guard-don't-drop
   ([ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md)). A
   locally fixed helper and a stale copy are indistinguishable from here, and
   silently replacing somebody's edited parser is not a trade to make on their
   behalf. The refusal names the files.
3. **A namespace is a widget question, a module is a Java one.** `NeedsNamespace`
   keys on `installs.widgets`, not on `rewrite.files` — the Java pack tokenises
   eight files and wants a `MODULE`, never a `NAMESPACE`. Asking for the wrong
   one invites an answer that then goes nowhere.

### Manifest

```yaml
name: mendix-vega-charts
version: 1.0.0
description: ...                    # mirrors SKILL.md frontmatter
min_mendix_version: 10.18.0         # gates on the project, per sdk/versions/*.yaml
installs:
  widgets: [VegaChart.mpk]          # copied into widgets/
  mdl: [mdl/oql-dml-actions.mdl]    # applied to the model, requires --apply
verify: scripts/check-spec.mjs      # exit code, runnable in seconds
```

`min_mendix_version` uses the existing version registry rather than inventing a
second gate. `installs.mdl` is the part that writes to the model, so it is
explicit and separately confirmable — installing a pack must never modify the
model as a side effect of copying documentation.

### Commands

```
mxcli skill list                     # available packs + what is installed here
mxcli skill add <pack> [--apply]     # copy assets; --apply runs installs.mdl
mxcli skill remove <pack>
mxcli skill upgrade [<pack>]         # re-sync, prune, refuse locally-edited files
mxcli init --with <pack>[,<pack>]    # at project creation
```

`mxcli init` with no `--with` behaves exactly as today.

## What this does not solve

**Prose still names the project the packs came from.** The widget source is
placeholdered and guarded by a test, and the specs are the ledger's own sample
data, which is fine. But `references/*.md` still uses `Ledger.*` entity names in
its examples. That is illustrative rather than load-bearing, and left alone.

**Verifying a pack in CI.** `mendix-vega-charts` ships a Node checker and seven
specs; `mendix-bulk-oql-dml` ships an MDL file that `make check-skill-mdl` should
be checking. Neither runs today. A pack whose own verifier is not run in CI is a
pack that rots.

## Plan

| Slice | Content |
|---|---|
| 1 | Embed + recursive sync + relative-path write + prune. One pack fixture, no CLI surface. Proves the mechanism carries nested non-Markdown assets through `init`. |
| 2 | `pack.yaml`, version gating, `mxcli skill list/add/remove/upgrade`, digest-fenced local-edit refusal. |
| 3 | Vendor `mendix-bulk-oql-dml` (no widget, so no namespace question), wire its MDL into `make check-skill-mdl`. |
| 4 | Vendor `mendix-vega-charts` with install-time namespace substitution. |
| 5 | Run `check-spec.mjs` over the shipped specs in CI. |
| 6 | `installs.java` + vendor `mendix-odata-pushdown`, the third pack. |

Slice 1 is worth landing on its own: it removes the silent-flattening hazard in
the write path whether or not any pack ever ships.
