---
title: mxcli marketplace diff — detect local modification and plan an ID-preserving module upgrade
status: draft
date: 2026-08-04
---

# Proposal: `mxcli marketplace diff` — detect local modification of an installed module

**Status:** Draft
**Date:** 2026-08-04

Follows on from [`PROPOSAL_marketplace_modules.md`](PROPOSAL_marketplace_modules.md),
which ships discovery + download + install and explicitly parks the update path:
*"A future ID-preserving merge is the remaining work."* This proposal picks up that
remaining work and argues it should be approached back-to-front — ship the safety
question first, because it is separable, useful on its own, and a precondition for
the merge.

## Problem Statement

Updating a marketplace module is routine maintenance, and today there is no path
through it that does not involve Studio Pro. Field report from a Mendix app authored
end-to-end through mxcli ([`ako/mxcli-sudoku` FINDINGS #32/#37](https://github.com/ako/mxcli-sudoku)):
six of seven marketplace modules were behind — `DataWidgets` at 3.5.0 against 3.11.3 —
and every route closed:

```
$ mxcli marketplace install 116540 -p Sudoku.mpr
Module "DataWidgets" is already installed (version 3.5.0). Target version: 3.11.3.
In-place module updates are not applied automatically … Update via Studio Pro.

$ mx module-import DataWidgets-3.11.3.mpk Sudoku.mpr
error 3: Project already contains a module with the name of an importing module.
```

The reporter's conclusion is the one that matters: *"An app buildable but not
maintainable through the CLI is only half-automatable."*

### Why mxcli's refusal is right but unhelpful

mxcli refuses because an in-place update can discard local edits and change
persistent-entity `$ID`s. That reasoning is sound but **unconditional** — there is no
way to tell mxcli a given case is safe, and no way to find out whether it is.

The user cannot answer "is this safe?" because the question that actually decides it
— *has anyone modified this module since it was installed?* — has no tool. That is
the gap this proposal closes.

## Investigation

Everything below was measured, not assumed. Commands and results are reproducible
with the packages named.

### 1. `mx` has no module upgrade

`mx` 11.13.0's full command list contains `module-import`, `create-module-package`,
`show-module-version`, `set-module-version`, `merge`, `diff` — and no upgrade.
`module-import` takes positional arguments only; there is no `--replace`, `--force`
or `--overwrite`, and error 3 (name collision) is unconditional.

`mx merge BASE MINE THEIRS` is the right *shape* — a three-way merge is exactly what
separates "the module author changed it" from "the user changed it" — but it is
ID-keyed and operates on whole projects that share history. Between two marketplace
packages it would see every element as deleted-plus-added (see §2), not modified.

### 2. Marketplace packages do NOT carry stable element IDs

A natural assumption is that a module author keeps element IDs stable across releases
so the module can be swapped. **Mendix's own platform-supported modules do not.**

| Module | Versions compared | Shared unit IDs | Entity IDs |
|---|---|---|---|
| DataWidgets | 3.10 → 3.11.3 | **0 of 17** | *(module has no entities)* |
| Administration | 4.3.2 → 4.5.0 | **0 of 34** | `Account` **changed**, `AccountPasswordData` **changed** |

Every element is regenerated per release — same name, same `$Type`, same folder, new
`$ID`. `Filter_Operators` moves `3eb4c31c…` → `6e70cf59…`; `Account` moves
`6dbb53a1…` → `815a92ab…`.

**Consequence: a literal replace is not merely risky, it is incorrect.** Replacing the
installed module's units with the package's would renumber every entity in it, and
every reference *from the consuming app* — an association to `Administration.Account`,
a security rule, a microflow retrieve — points at the old `$ID`. The upgrade would
break the callers, not just the database mapping.

An upgrade therefore has to be a **name-keyed merge into the existing module that
preserves the in-project `$ID`s**. This is consistent with what
`PROPOSAL_marketplace_modules.md` already records about Studio Pro's behaviour, and it
explains the failure mode Mendix users report: a name-keyed merge is well-defined
right up until the user has also edited the element, at which point it cannot decide
whose change wins.

### 3. The installed module is not the package — so you cannot diff against it

This is the finding that shapes the design, and it invalidates the obvious
implementation.

Control: a **blank Mendix 11.13 project**, whose `Administration` module is untouched
by definition, compared against the **published 4.3.2 `.mpk`** it was built from.

| Comparison | Elements matched by name | Orphans | Elements differing | Paths differing |
|---|---|---|---|---|
| installed 4.3.2 vs published 4.3.2 | 27 ↔ 27 | **0** | 10 of 27 | 15,066 |
| installed 4.3.2 vs published 4.3.2, **converted to 11.13 first** (`mx convert`) | 27 ↔ 27 | **0** | 7 of 27 | 15,041 |

Two things follow.

**Name+`$Type` is a sound join key.** Zero orphans in either direction — every element
in the installed module has exactly one counterpart in the package. This is what makes
a name-keyed merge feasible at all.

**A path-level comparison against the package is not a drift signal.** An untouched
module differs in 15,041 paths. Running Mendix's own conversion first (`mx convert`
accepts an `.mpk` and emits a converted one) removes only ~25 of them, so this is not
version drift. The differences are whole subtrees present in the project and absent in
the package:

```
/FormCall/…/Object/Properties[25]/Value/TextTemplate/$Type
   project = 'Forms$ClientTemplate'      package = <ABSENT>
/FormCall/…/Value/TextTemplate/Fallback/$Type
   project = 'Texts$Text'                package = <ABSENT>
/Autofocus
   project = 'DesktopOnly'               package = <ABSENT>   (this one IS conversion)
```

The installed copy has been transformed on the way in — by import, by version
conversion, and (for the widget-bearing pages) by reconciliation against the widget
packages present in the consuming project. It is not, and never was, a copy of the
`.mpk` payload.

A naive `mxcli marketplace diff` built on BSON comparison would therefore report every
module as heavily modified and be worse than useless. **This is the trap the proposal
exists to document.**

## Status update (2026-08-04) — the drift is entirely pluggable-widget BSON

Measured after the first draft, and it **supersedes the DESCRIBE-based design below**.

Attributing each differing path to the nearest enclosing `$Type` shows the 15,041
differences are not spread through the module at all:

| Module | Pages / pluggable widgets | Elements | Orphans | Drift inside `CustomWidgets$` | Outside |
|---|---|---|---|---|---|
| Administration 4.3.2 | 9 pages | 27 ↔ 27 | 0 | **15,041** | **0** |
| WebActions 2.11.0 | none | 13 ↔ 13 | 0 | **0** | 1 |

The two modules were chosen to make opposite predictions and both held. The single
WebActions difference is `Projects$ModuleSettings/PackageId`, a per-install GUID that
differs by construction.

So the model is:

> An installed marketplace module is **byte-identical** to its version-converted
> package, except (a) `PackageId`, and (b) everything inside a `CustomWidgets$`
> subtree — the pluggable-widget envelope, which is reconciled against the *consuming
> project's* widget packages on import.

Three consequences:

1. **Drift detection can be structural after all.** For every element outside a
   pluggable-widget subtree, a path-level comparison against the converted package is
   already exact. The DESCRIBE-based design below is heavier than necessary.
2. **Open question 2 dissolves.** DESCRIBE coverage no longer bounds the feature, and
   with it the "not comparable" bucket and its honesty problem — for non-widget
   elements. Only widget subtrees need special handling.
3. **Open question 3 is answered: yes.** The drift is the same phenomenon as
   mendixlabs/mxcli#716. The widget-aware comparison this needs is the same primitive
   [`PROPOSAL_widget_instance_reconciliation.md`](PROPOSAL_widget_instance_reconciliation.md)
   needs to decide whether a stored instance matches its installed package. Building
   it once serves both.

Revised design: compare structurally against the converted package; ignore
`PackageId`; and inside `CustomWidgets$` subtrees defer to the shared
widget-comparison primitive rather than comparing raw paths. The negative control in
the test plan is unchanged and still leads — it is what produced this result.

## Design

> Superseded by the status update above for the non-widget case; retained because the
> reasoning still applies inside widget subtrees, where raw comparison does not work.

### Compare semantically, not structurally

mxcli already has the normalisation this needs: `DESCRIBE` emits re-executable MDL for
a document, which by construction discards `$ID`s, storage envelopes, widget-internal
representation and the other artefacts §3 exposed. Two elements that describe to the
same MDL are the same element as far as an author is concerned.

So drift detection compares **DESCRIBE output**, not BSON:

1. Download the package for the version the project *claims* to have (`show modules`
   reports `AppStoreVersion`).
2. Import it into a scratch project at the consuming project's Mendix version, so it
   goes through the same conversion the installed copy did.
3. `DESCRIBE` every element of the module on both sides, key by name + `$Type`.
4. Report elements whose MDL differs — those are local modifications.

This is bounded by DESCRIBE coverage: document types with no DESCRIBE (or a lossy one)
must be reported as **unknown**, never as clean. Silently treating an
un-describable element as unmodified is the one failure mode that would make the tool
dangerous rather than merely incomplete.

### Then the upgrade becomes decidable

With drift known, the three cases separate cleanly:

| Installed module | Upgrade |
|---|---|
| unmodified | mechanical name-keyed merge, preserving in-project `$ID`s |
| modified, no collision with the new version | merge, reporting what was kept |
| modified, colliding with the new version | refuse, listing each conflicting element |

Only the first is in scope for a first implementation; the others need the merge
engine and are deliberately deferred.

## Proposed CLI

```bash
# What has been changed locally in this module since it was installed?
mxcli marketplace diff <id> -p app.mpr
mxcli marketplace diff DataWidgets -p app.mpr        # resolve by module name

# What would upgrading change? (adds the target-version comparison)
mxcli marketplace diff <id> -p app.mpr --to 3.11.3

# Machine-readable for CI ("fail the build if a marketplace module was edited")
mxcli marketplace diff <id> -p app.mpr --format json
```

Sketch of the output:

```
Administration — installed 4.3.2, latest 4.5.0

  Local modifications (2 of 27 elements):
    Forms$Page        Account_Overview     3 statements differ
    Microflows$Micro  NewAccount           1 statement differs

  Not comparable (1 element):
    Security$ModuleSecurity                no DESCRIBE support

  Upgrading to 4.5.0 would touch 14 elements, 2 of which you have modified:
    CONFLICT  Forms$Page  Account_Overview
    CONFLICT  Microflows$Microflow  NewAccount
```

No MDL syntax is added. This is a CLI-only, read-only command.

## Implementation Plan

Phase 1 is the whole of this proposal; phase 2 is named only to show where it leads.

### Phase 1 — `marketplace diff` (read-only)

| File | Change |
|------|--------|
| `cmd/mxcli/cmd_marketplace.go` | New `diff` subcommand: flags `--to`, `--format`, module-name resolution |
| `cmd/mxcli/marketplace/compare.go` *(new)* | Orchestration: resolve installed version → download → convert → describe both sides → report |
| `cmd/mxcli/marketplace/scratch.go` *(new)* | Build the scratch project at the consuming project's version; wraps `mx convert` on the `.mpk` |
| `mdl/executor/` (describe paths) | Expose a programmatic "describe this element" entry point; today DESCRIBE is reachable only as a statement |
| `mdl/backend/` | Interface method to enumerate a module's elements with name + `$Type` (the catalog has this; it needs a backend-level accessor) |
| `docs-site/src/` | User-facing page for the command |

### Phase 2 — `marketplace update` (deferred, not proposed here)

Name-keyed, `$ID`-preserving merge, gated on a clean `diff`. Needs a decision on
conflict presentation and on what to do with elements the new version deletes.

## Version Compatibility

Not version-gated. The command works against any project mxcli can open. It does
require an `mx` binary for the conversion step (already a dependency of
`docker check`/`build`, and auto-downloadable via `setup mxbuild`), and marketplace
credentials for the download (existing `MENDIX_PAT` / `mxcli auth` layer).

The one version-sensitive element is that the package must be converted **to the
consuming project's Mendix version** before comparison — comparing against an
unconverted package is measurably wrong (§3).

## Test Plan

The control from §3 is the primary test and it is fully reproducible:

- **Negative control (must report zero drift):** a blank project at version *N*, whose
  marketplace modules are untouched, diffed against their own published packages. If
  this reports modifications, the normalisation is wrong. This is the test that would
  have caught the naive BSON implementation.
- **Positive control (must report exactly one):** apply a single known edit through
  MDL (e.g. `alter page Administration.Account_Overview …`), re-run, assert that
  element and only that element is reported.
- **Coverage honesty:** assert that a document type with no DESCRIBE support is
  reported as *not comparable*, never as clean.
- Fixtures in `mdl-examples/bug-tests/` are not the right home; this needs an
  integration test under `-tags integration` because it shells out to `mx convert`
  and the marketplace API.

## Open Questions

1. **Scratch-project conversion cost.** Each diff runs `mx convert` on a package
   (~seconds). Acceptable for an explicit command; too slow to run implicitly inside
   `mxcli check`. Should the converted package be cached under `~/.mxcli/`?
2. ~~**DESCRIBE coverage is the real bound.**~~ **Resolved** by the status update:
   the comparison is structural outside widget subtrees, so DESCRIBE coverage does not
   bound it.
3. ~~**Is the widget-instance difference in §3 the same phenomenon as #716?**~~
   **Resolved: yes.** All 15,041 differing paths are inside `CustomWidgets$` subtrees
   and none outside. This proposal and
   [`PROPOSAL_widget_instance_reconciliation.md`](PROPOSAL_widget_instance_reconciliation.md)
   want the same widget-comparison primitive; it should be built once, for both.
4. **Modules with no recorded version.** `mx show-module-version` reports *"Module
   'DataWidgets' does not have a version"* while `mxcli show modules` reports
   `Marketplace v3.5.0` — they read different fields. mxcli has no writer for
   `AppStoreVersion`, so a hand-updated module cannot be recorded as such (FINDINGS
   #37). Should this proposal include `set module version`, or does that belong with
   phase 2?
5. **Where does the shared widget primitive live?** It is needed by `widget sync` (to
   decide whether a stored instance matches its package) and by this command (to
   compare a widget subtree without drowning in envelope noise). Neither proposal owns
   it. A `modelsdk/widgets` comparison entry point next to `AugmentTemplate` is the
   obvious home, but that decision should be made when the first of the two is built.
