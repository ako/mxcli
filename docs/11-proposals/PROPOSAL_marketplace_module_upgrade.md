---
title: mxcli marketplace diff — detect local modification and plan a GUID-preserving module upgrade
status: draft
date: 2026-08-04
---

# Proposal: `mxcli marketplace diff` — detect local modification of an installed module

**Status:** Draft
**Date:** 2026-08-04 (initial), revised 2026-08-10 (Studio Pro update measured),
2026-08-11 (DESCRIBE coverage measured — §7; `GUID` = database identity measured — §8)

## Revision 2026-08-10 — what Studio Pro actually does, measured

The first draft reasoned about Studio Pro's Marketplace "Update" from its
outcomes. It has now been measured directly (§4), and two of this proposal's
load-bearing assumptions were wrong in ways that change the design:

- The update is a **replace that transplants the `GUID`**, not an
  ID-preserving merge. Every `$ID` in the module is renumbered.
- Renumbering does **not** break consumers. Cross-module references are
  qualified-name strings, so a module can be renumbered underneath its callers
  without touching them.
- A locally modified element is **silently destroyed**, with no merge and no
  conflict recorded in the model.

The upgrade this proposal is a precondition for is therefore
**GUID-preserving**, not `$ID`-preserving — a materially easier thing to build.
And the safety argument is stronger than first written: mxcli's refusal is not
a gap relative to Studio Pro, it is a safeguard Studio Pro does not offer.

Follows on from [`PROPOSAL_marketplace_modules.md`](PROPOSAL_marketplace_modules.md),
which ships discovery + download + install and explicitly parks the update path —
originally as *"a future ID-preserving merge"*, since corrected there to a
GUID-preserving replace on the evidence in §4. This proposal picks up that
remaining work and argues it should be approached back-to-front — ship the safety
question first, because it is separable, useful on its own, and a precondition for
the upgrade.

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
persistent-entity `$ID`s. Half of that is now confirmed and half is not: an update
does discard local edits (§4) and does renumber every `$ID` (§4) — but the
renumbering is harmless, because nothing outside the unit points at those `$ID`s.
The refusal is right for the first reason alone.

It is nonetheless **unconditional** — there is no way to tell mxcli a given case is
safe, and no way to find out whether it is.

The user cannot answer "is this safe?" because the question that actually decides it
— *has anyone modified this module since it was installed?* — has no tool. That is
the gap this proposal closes. Note the asymmetry §4 exposes: Studio Pro does not
answer that question either. It destroys the edit and proceeds.

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

The first draft drew a conclusion here that §4 has since refuted, and it is left in
place because the reasoning is the trap:

> ~~**Consequence: a literal replace is not merely risky, it is incorrect.** Replacing
> the installed module's units with the package's would renumber every entity in it,
> and every reference *from the consuming app* — an association to
> `Administration.Account`, a security rule, a microflow retrieve — points at the old
> `$ID`. The upgrade would break the callers, not just the database mapping.~~
>
> ~~An upgrade therefore has to be a **name-keyed merge into the existing module that
> preserves the in-project `$ID`s**.~~

The renumbering is real. The breakage is not: consumers reference a module's elements
**by qualified name**, not by `$ID`, and Studio Pro renumbers the whole module on every
update without touching them (§4). What an upgrade must preserve is the element
**name**, its **`GUID`**, and each unit's **internal** pointer consistency.

The genuinely hard part survives unchanged, and it is the reason this proposal ships
the diff first: a name-keyed upgrade is well-defined right up until the user has also
edited the element, at which point it cannot decide whose change wins. Studio Pro
resolves that by discarding the user's edit (§4).

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

### 4. What Studio Pro's update actually does (measured 2026-08-10)

§2 and §3 compare *packages* and *installed copies*. Neither observes the operation
itself. This section does: a project was snapshotted, its modules updated in Studio
Pro, and snapshotted again.

Method, data and full analysis:
[`data/marketplace-upgrade/`](data/marketplace-upgrade/). Subject: `test1-app`,
Mendix 11.13.0, MPR v2, 369 units, `Administration` 4.3.2 → 4.5.0 and
`DataWidgets` 3.5.0 → 3.11.3. Snapshots are taken with
[`scripts/mprsnapshot`](../../scripts/mprsnapshot), which keys every element by
name and values it by `$ID` + `GUID` — the opposite normalisation from the drift
detection designed below, because here identity is the signal.

Comparing before/after in the *same* project is what makes this readable: both
sides have been through the same import and conversion, so the 15,041-path noise
floor of §3 does not arise.

**The module update is a replace that transplants the `GUID`.**

| `Administration`, matched by name | Count |
|---|---|
| Elements matched | 94 |
| `$ID` preserved | **0** |
| `$ID` renumbered | 94 |
| Elements carrying a `GUID` | 9 |
| `GUID` preserved | **9 (all)** |

At unit level the same: 0 of 27 `Administration` units and 0 of 10 `DataWidgets`
units kept their `$ID`, while 9 of each kept byte-identical content. `Account`
moved `562830a8…` → `1dee876e…` and kept `guid=b16e49ea…`.

`PROPOSAL_marketplace_modules.md` records this as *"an ID-preserving merge the
`mx` CLI does not expose"*. It is neither ID-preserving nor a merge.

**Consumers are untouched, because they reference by name.** `MyFirstModule`
generalizes *and* retrieves `Administration.Account`. Across the update it is
identical apart from the project-wide unit count in the header — all 7 units kept
`$ID` and content while `Account` was renumbered underneath them. Of 9,910 binary
`$ID` pointers in the project, **0** cross a unit boundary; cross-module
references are qualified-name strings
(`MaybeGeneralization/Generalization = "Administration.Account"`), and intra-unit
pointers are rewritten consistently.

**A locally modified element is destroyed silently.**
`Administration.AccountPasswordData.ExtraAttributeForTest` — added before the
snapshot precisely to test this — is gone. Discounting index-path artefacts, the
update's entire real element delta is:

```
- Administration/DomainModel/Entities/AccountPasswordData/Attributes/ExtraAttributeForTest
+ Administration/ModuleSecurity/ModuleRoles/EditOwnDetails
+ Administration/ModuleSecurity/ModuleRoles/EditOwnPassword
+ Administration/ModuleSecurity/ModuleRoles/ReadOthersEmail
+ Administration/ModuleSecurity/ModuleRoles/ReadOthersFullName
+ Administration/ModuleSecurity/ModuleRoles/ReadOwnDetails
```

Five genuine 4.5.0 additions, and one deletion that was the user's own work. No
merge, no conflict recorded in the model.

**Upgrading widget definitions is a different operation entirely.** Run
separately after the module update: all 124 elements kept their `$ID`, nothing was
added or removed, and exactly 7 units changed content — the `Administration` pages
carrying DataGrid2 widgets. The `DataWidgets` module itself was untouched (11 of 11
units byte-identical). So this rewrites widget **instances on pages**, in place,
preserving identity. It should not be conflated with the module update, and it
connects to [`PROPOSAL_widget_instance_reconciliation.md`](PROPOSAL_widget_instance_reconciliation.md).

**Two incidental findings.** `Administration/_Docs/v4.3.2` became `_Docs/v4.5.0` —
a cheaper installed-version oracle than the `AppStoreVersion` field that
`mx show-module-version` and `mxcli show modules` disagree about (open question 4).
And unnamed widget nodes keyed by list index produce 25 phantom add/removes when a
property list shifts `Properties[7]` → `Properties[6]`; a real `marketplace diff`
needs a stabler key for them than the index.

**Limits.** One project, one Mendix version, one update path. The role of `GUID` is
inferred, not observed: it is the one identity preserved across a full replace,
which is strong evidence it carries database mapping, but no DDL was examined.

### 5. "Internally consistent" is a hard constraint, and mxcli cannot yet honour it

§4 licenses renumbering with one condition — *"`$ID`s may be renumbered freely as
long as each unit stays internally consistent, because nothing outside the unit
points at them."* That clause is doing more work than its length suggests, and it
has now been tested from the other direction.

An unrelated change ([#125](https://github.com/ako/mxcli/pull/125), reverted)
attempted to make re-running an MDL script byte-stable by carrying stored element
`$ID`s onto a rebuilt document — structurally the same operation an upgrade
performs, at a smaller scale. It rewrote `$ID`s and left the pointers that
reference them untouched, which makes the project unopenable:

```
ERROR: System.AggregateException: (The given key '553f4a64-…' was not present in the dictionary.)
 ---> System.Collections.Generic.KeyNotFoundException
   at Mendix.Modeler.Storage.Operations.StreamingBsonUnitReader.ResolvePostponedProperties()
```

Measured in review, on a real MPR v2 app (Mendix 11.13, 413 `.mxunit` files):
microflows and nanoflows corrupt, pages and navigation survive. Searching the tree
for the GUID the loader rejects finds it once, as a `SequenceFlow.OriginPointer`
aimed at an element that no longer exists — 6 of 6 flow pointers resolved before,
0 after. `mx check` on the parent commit is clean, so the failure isolates to that
change.

Two things follow for this proposal.

**The document types that survive are not evidence of safety.** A page's widget
tree is containment; a microflow's is a graph. The §4 snapshots make the risk
concrete: `DataWidgets` — the module that motivated the field report — contains
**0** `Microflows$Microflow`, while `Administration` contains **8**. So an
upgrade could pass a manual smoke test on `DataWidgets` and corrupt
`Administration`, and the two are routinely updated together.

**mxcli's write layer cannot currently guarantee the condition**, which is the
part worth knowing before Phase 2 is scheduled. Pointers are not child elements —
`Microflows$SequenceFlow.OriginPointer` is a *primitive* property holding an
`element.ID` (see `InitFromRaw` in `modelsdk/gen/microflows/types.go`), so a walk
over `ChildProperty`/`ChildListProperty` traverses the whole document and never
sees a single reference. There is no way, today, to enumerate the reference-valued
properties of an element generically. Any operation that renumbers within a unit
needs that first, and the gap is invisible until a project fails to load.

An import-and-transplant upgrade may sidestep this entirely — if the new unit is
written wholesale from the package, its internal pointers are already consistent
and nothing is renumbered in place. That is the strongest argument yet for
preferring import-and-transplant over anything element-level, and it is worth
stating as a design constraint rather than an implementation detail: **an upgrade
should never rewrite an `$ID` inside a unit it is otherwise preserving.**

### 6. The unnamed-element key problem is shared, and larger than the phantom diffs

§4's closing observation — unnamed widget nodes keyed by list index produce 25
phantom add/removes when `Properties[7]` shifts to `Properties[6]` — is the same
obstacle #125 hit, and its size is now known from the other side.

Matching by name where a name exists, and by position only where the two lists are
the same length, left **980 of 988 element IDs in a single page unmatched**. Only
the top-level chain (`Page`, `PageParameter`, `ObjectType`, `LayoutCall`,
`FormCallArgument`) carries a name to match on. What dominates a page is unnamed
and unmatched: `Texts$Text` ×152, `Forms$Appearance` ×116,
`Forms$ClientTemplate` ×74, `Texts$Translation` ×68.

So the phantom add/removes are not an edge case to paper over in the differ — they
are the majority of a page document. A `marketplace diff` that reports honestly on
a page-heavy module (which `DataWidgets` is) needs a stable key for unnamed
elements, and no such key exists today. The two candidates are a structural path
(stable until a list shifts, which is exactly when it matters) and a content
digest of the subtree (stable under moves, ambiguous between genuinely identical
siblings).

This is the one piece of work both this proposal and script idempotence need, and
neither can be finished well without it.

> **Superseded in part (2026-08-11).** Script idempotence shipped *without* a stable
> key for unnamed elements: `modelsdk/canon` sidesteps element matching entirely by
> normalising each `$ID` to its index in a containment walk, so it never has to pair
> two elements up (ADR-0008). The claim that neither could be finished without this
> key held for the BSON-structural diff §4 was reasoning about; it does **not** bind
> the design below, which compares DESCRIBE output rather than BSON structure and so
> never keys an unnamed widget at all. The key problem is real but is now scoped to
> anything wanting an element-level *structural* diff — not to `marketplace diff`.

### 7. DESCRIBE coverage is sufficient, and was measured (2026-08-11)

The design below is bounded by DESCRIBE coverage, and its honesty rule — an
un-describable element must be reported **unknown**, never clean — is only affordable
if the unknown bucket is small. That was unmeasured, so the whole proposal rested on
an assumption. It has now been measured against `testdata/expr-checker/minimal.mpr`,
which carries seven real marketplace modules with recorded `AppStoreVersion`s
(Administration 4.3.2, Atlas_Core 4.1.3, Atlas_Web_Content 4.1.0, DataWidgets 3.5.0,
FeedbackModule 4.0.2, NanoflowCommons 6.0.0, WebActions 2.11.0).

**Method.** The denominator is the set of *named units read from the MPR itself* —
369 units, 251 named documents after excluding 80 folders and the unnamed
module-level units. It deliberately does **not** come from the catalog: the `objects`
view indexes only describable types and `show modules` only has columns for types
mxcli models, so either source reports 100% coverage by construction. Each document
was then actually described, and the outcome recorded.

**Result — 247 of 251 (98%) describable.**

| | Docs | |
|---|---|---|
| Auto-detected by bare `describe Module.Name` | 204 → **247** | 81% → **98%** |
| Reachable only by naming the type explicitly | 43 → **0** | |
| Not describable at all | 4 | 2% |

The 43 in the middle row were building blocks (40) and icon collections (3): both had
working explicit handlers, but building blocks were never joined into the catalog's
`objects` view and icon collections had no catalog table at all, so bare DESCRIBE
reported them as not found. Fixed here — the second number in each row is post-fix,
verified end-to-end on the same 251 documents, with no new name ambiguity. A drift
test (`TestDescribeAutoCoversCatalogObjectTypes`) now fails when a type reaches the
view without a describe kind.

The remaining 4 are two genuine defects, both out of scope for this proposal:

- **Import/export mapping describe is broken.** The catalog indexes both, but
  `describe import mapping FeedbackModule.IMM_PostResponse` errors `not found`, and
  naming the type explicitly does not help.
- **Menu documents have no DESCRIBE at all** (2 in Atlas_Core). No grammar, no handler.

**Two gaps this measurement also exposed, which the differ must handle.**

- **Page templates were conflated with pages** (closed 2026-08-11). All 46
  `Forms$PageTemplate` units reported `ObjectType = PAGE` and described as
  `create or modify page`, so `show modules` reported Atlas_Web_Content as having
  46 pages when it has zero.

  This was recorded here as "harmless for describe-to-describe comparison, since
  both sides conflate identically". That was wrong, and the error was in the same
  family as the security one above: the conflation was checked, the *content* was
  not. A page template describes with an **empty body** — its widgets hang off
  `LayoutCall`, and the page describe path reads `FormCall` — so those 46
  elements compared on nothing but name, folder and CSS class. The differ was
  reporting them unchanged without having looked inside them, which is precisely
  the false negative the honesty rule exists to prevent, and it was hiding behind
  a bug filed as cosmetic.

  Cause: `listUnitsByType` matched on a type **prefix**, and `Forms$Page` is a
  prefix of `Forms$PageTemplate`. Both engines now match exactly, templates are
  indexed as their own `PAGE_TEMPLATE` catalog type, and — having no DESCRIBE
  handler — they are reported **unknown**, which is the truthful version of what
  the differ was already doing.
- **Module roles were invisible** (closed 2026-08-11). This was originally recorded
  here as "module security is invisible", which was wrong and briefly made the
  security hole look like the largest risk in the proposal. Re-measured: three of the
  four parts of module security were **already** in the describe surface —
  entity access rules in `DESCRIBE ENTITY` (`grant <role> on <entity> (...) where
  '<xpath>'`), page access in `DESCRIBE PAGE` (`grant view on page ...`), and
  microflow access in `DESCRIBE MICROFLOW` (`grant execute on microflow ...`). Only
  the module's **role list** was missing, because it lives in the module's own
  `Security$ModuleSecurity` unit and belongs to no document. `DESCRIBE MODULE` now
  emits it, sorted for stable comparison.

  The original error came from grepping describe output for `role|access|allowed` —
  a pattern that cannot match `grant view on page P to Administration.Administrator;`.
  Absence of evidence from a search is not evidence of absence: check the emitter, or
  grep for the statement you expect to see rather than for words describing it.

Folders need no separate coverage: folder membership is captured inside each
document's describe (`Folder: 'Phone/PageTemplates/Form'`), so a document moved
between folders by an upgrade is visible.

**What this does not establish.** It measures *invocation success and non-trivial
output* — not round-trip fidelity. A describe can succeed and silently drop a
property, which is exactly what #812, #111 and #57/#58 were. So 98% is an **upper
bound on what a differ can compare**, not evidence that the comparison is faithful;
establishing that needs describe → execute → re-describe round-tripping, which is a
separate and much larger exercise. Also: one project, and all seven modules are
Mendix-authored — a third-party module may use document types absent here. Building
blocks and icon collections describe read-only ("cannot be created via MDL"), which
is fine for diff but blocks a Phase 2 *replace* of those two types.

## Design

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
| unmodified | mechanical name-keyed replace, preserving `GUID`s |
| modified, no collision with the new version | replace + re-apply the local edits, reporting what was kept |
| modified, colliding with the new version | refuse, listing each conflicting element |

Only the first is in scope for a first implementation; the others need the merge
engine and are deliberately deferred.

§4 makes the first row substantially cheaper than the original draft assumed. An
unmodified module does not need element-level merging at all: import the new
package's units, and carry the `GUID` across for each element matched by name.
`$ID`s may be renumbered freely as long as each unit stays internally consistent,
because nothing outside the unit points at them. That is what Studio Pro does.

The difference mxcli should offer is **not** a better merge — it is that rows two
and three are distinguished from row one *before* anything is written. Studio Pro
treats all three as row one.

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

### 8. What `GUID` is for: the database keys on it (measured 2026-08-11)

§4 established that Studio Pro's update renumbers every `$ID` and preserves every
`GUID`. That made `GUID` the *only* candidate carrier of database identity, but
the proposal was careful to call it inference. It is now measured.

**Method.** A blank Mendix 11.12.1 app with `Administration` 4.3.2, booted with
`mxcli run --local` against a local PostgreSQL, so the runtime creates and then
re-synchronises a real schema. The lever is one that Studio Pro does not expose
and mxcli does: rewrite a single BSON value in the stored model and boot again.
Nothing points *at* a `GUID` — it is not a pointer target — so changing it is a
safe one-value edit, unlike renumbering an `$ID` (ADR-0008).

**The runtime writes its own identity map.** `mendixsystem$entity` and
`mendixsystem$attribute` record, per element, the id the database knows it by:

```
mendixsystem$entity     b16e49ea-91df-4caa-aed8-6ba4c4e133c5  Administration.Account  administration$account
mendixsystem$attribute  aac00d66-7cc1-4def-a8d6-8b81fa1f5477  FullName
```

Those are the model's own `GUID`s. `Account` stores
`ea 49 6e b1 df 91 aa 4c ae d8 6b a4 c4 e1 33 c5`, which is
`b16e49ea-91df-4caa-aed8-6ba4c4e133c5` once the .NET field order is undone — and
`FullName`'s decodes to `aac00d66-…` likewise. The mapping is byte-identical, not
merely correlated. It is also the same `b16e49ea…` recorded in §4 on a different
project at a different Mendix version, because the `GUID` is a property of the
*published module*, stable across every project that installs it.

**Changing only the `GUID` destroys the data.**

| Run | Model change | `mendixsystem$entity.id` | `administration$account` |
|---|---|---|---|
| 1 | — (baseline) | `b16e49ea-…` | 1 row inserted |
| 2 | `GUID` → `a0a1a2…` | `a3a2a1a0-…` | **0 rows** |
| 3 | `GUID` restored | `b16e49ea-…` | table recreated, empty |
| 4 | none (control) | `b16e49ea-…` | **row survives** |

Run 2 changed nothing else — same entity name, same table name, same attributes —
and the runtime treated it as a different entity. Run 4 is the control that makes
run 2 readable: an unchanged reboot preserves the row, so the loss was caused by
the identity change and not by restarting.

**What this settles, and what it does not.** Studio Pro's update preserves exactly
the identity the database keys on, so `$ID` renumbering — all 94 of them — is
irrelevant to data safety. "A `GUID`-preserving replace is data-safe" is now a
measured claim at the level of entity and attribute identity.

It does **not** say the upgrade is harmless. An element the new version deletes
still loses its column or table, which is a schema decision rather than an
identity failure, and §4's destroyed local edit is untouched by any of this. Nor
was the generated DDL read statement-by-statement: the runtime logs the count
(596 commands cold, 38 on the identity change) but not the text at INFO. The
outcome was measured instead, which is the stronger evidence for the question
asked.

## Implementation Plan

Phase 1 is the whole of this proposal; phase 2 is named only to show where it leads.

### Phase 1 — `marketplace diff` (read-only)

§7 measured the risk this phase actually carries — DESCRIBE coverage — at 98% of the
documents in a seven-module marketplace project, so the design below is viable as
written. Three prerequisites fall out of that measurement, in priority order:

1. ~~**Report module security as unknown** (or close the gap).~~ **Closed.** The gap
   was narrower than first recorded — only the module role list, not module security
   as a whole; see the correction above. `DESCRIBE MODULE` now emits roles.
2. ~~**Fix import/export mapping describe.**~~ **Closed** — the cause was
   `moduleNameFor` reading a unit's direct container instead of walking to the
   enclosing module, so every foldered document missed.
3. ~~**Distinguish page templates from pages**, or accept and document the
   conflation.~~ **Closed 2026-08-11**, and it was not "the least severe" as
   recorded — see the corrected finding above. `Forms$Page` being a prefix of
   `Forms$PageTemplate` fed 46 templates into a prefix-matched page query; they
   then described as pages with an empty body, so the differ judged them
   unchanged without reading them. Templates are now their own catalog type and
   report as unknown.

Also closed since: the bare-DESCRIBE auto-detect gap (43 documents), and menu
documents, which had no DESCRIBE at all and now have full CRUD. Bare-DESCRIBE
coverage over the fixture is 251/251.

**Phase 1 is therefore unblocked.**

| File | Change |
|------|--------|
| `cmd/mxcli/cmd_marketplace_diff.go` *(new)* | The `diff` subcommand: flags `--to`, `--module`, `--json`; module + version resolution |
| `cmd/mxcli/marketplace/snapshot.go` *(new)* | Enumerate a module's elements from the catalog and capture DESCRIBE output for each |
| `cmd/mxcli/marketplace/compare.go` *(new)* | Match two snapshots by name+type and classify each element |
| `cmd/mxcli/marketplace/scratch.go` *(new)* | Build the reference project at the consuming project's version and import the `.mpk` into it |
| `cmd/mxcli/marketplace/report.go` *(new)* | Human and JSON rendering, including the honesty rule |
| `internal/marketplace/client.go` | Paginate `Versions` (see below) |
| `docs-site/src/guides/marketplace.md` | User-facing documentation for the command |

Two things the plan above got wrong, both found by running it:

- **The scratch project is built with `mx create-project` + `mx module-import`,
  not `mx convert` on the `.mpk`.** A blank project is not empty — it already
  ships Administration, Atlas_Core, DataWidgets and friends — so the template's
  copy is dropped through mxcli's own `DROP MODULE` before the package is
  imported, or `module-import` refuses the name (exit 47).
- **No new backend or executor entry point was needed.** The catalog's `objects`
  view already enumerates a module's elements with name + type, and DESCRIBE is
  reachable programmatically by executing an `ast.DescribeStmt` against an
  executor writing to a buffer. Adding an interface method would have been a
  second way to do the same thing.

**How the module and its version are identified.** Each installed module records
`AppStoreGuid`, and that GUID is the marketplace **version** UUID — a blank
11.12.1 project carries `2059615c-…` for Administration, which is exactly
content 23513's version 4.3.2, and `225ac9cf-…` for DataWidgets, content
116540's version 3.5.0. Matching on it identifies both the module and the exact
release with no network call and no guessing at the listing name (content 23513
is listed as "Administration module" and installs `Administration`).

Matching on the version *number* instead looks equivalent and is not: that same
blank project has Atlas_Web_Content at 4.1.0 and Administration's content has
also published a 4.1.0, so a number match selects two modules and cannot tell
them apart. This was not reasoned out — it was the first real run of the
command, which refused rather than guessing.

**A latent bug this surfaced.** `/v1/content/{id}/versions` pages: it returns 10
versions unpaged and caps `limit` at 20. `Client.Versions` asked once, so
`marketplace versions` showed exactly ten of everything and an older installed
version looked unpublished — Data Widgets has 131 releases and mxcli could see
10. Fixed by walking pages until one comes back short; `marketplace
versions`/`download`/`install` all benefit.

### Phase 2 — `marketplace update` (deferred, not proposed here)

Name-keyed, **`GUID`-preserving** replace, gated on a clean `diff`. Needs a
decision on conflict presentation and on what to do with elements the new version
deletes.

§4 narrows this considerably: the mechanism is import-and-transplant-`GUID`s
rather than an element-level merge engine, and the `$ID`s need no special
handling. The open design work is almost entirely about the *modified* cases —
what to show the user, and whether re-applying a local edit onto a replaced
element is something mxcli should attempt at all or merely describe.

§5 adds a constraint on *how* the replace is implemented: import the unit
wholesale, never renumber an `$ID` inside a unit being preserved. Re-applying a
local edit onto a replaced element is the case that would tempt an in-place
rewrite, and mxcli cannot currently keep intra-unit pointers consistent when it
does that.

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
- **The recorded update (new):** the `01-before` → `02-after` pair in
  [`data/marketplace-upgrade/`](data/marketplace-upgrade/) is a captured Studio Pro
  update with a known answer — one destroyed local edit, five added module roles.
  A `marketplace diff` run against `02-after` must report
  `ExtraAttributeForTest` as locally modified in `01-before` and gone afterwards.
  This is the only fixture in the plan that does not need Studio Pro to re-run.
- **Renumbering is not drift:** every `$ID` in the module changes on every update,
  so a diff implementation that keys on `$ID` reports 100% drift. Assert that the
  `01-before` → `02-after` pair reports drift only for the elements that actually
  changed, not for all 94.
- Fixtures in `mdl-examples/bug-tests/` are not the right home; this needs an
  integration test under `-tags integration` because it shells out to `mx convert`
  and the marketplace API.

### What has run (2026-08-11)

Both controls pass, against real marketplace content rather than a fixture.

- **Negative control.** `marketplace diff 23513 -p <blank 11.12.1 project>`:
  *"No local modifications: 21 of 21 elements verified unchanged."* The
  reference is downloaded, imported and described from scratch each run, so this
  also demonstrates the build is reproducible — `TestPackageProject_ReferenceIsReproducibleAndDiffable`
  asserts the same thing on two independently built references.
- **Positive control.** One added attribute
  (`alter entity Administration.Account add attribute LocalNote: String(100)`) →
  *"Locally modified (1 of 21 elements): changed ENTITY Account"*, and nothing
  else.
- **Upgrade impact.** `--to 4.5.0` reports five elements touched by the author,
  one of which (`ENTITY Account`) collides with the local edit. The control for
  *that* is `--to 4.3.2` — upgrading to the version already installed — which
  reports nothing touched, so the five are real author changes and not noise
  from the reference-building path.
- **Coverage honesty** is unit-tested rather than measured, because the fixture
  module has no un-describable element: `TestDiffResult_UnknownIsNeverACleanBillOfHealth`
  asserts an unknown element never renders as a clean verification, and fails
  with exactly the dangerous output ("No local modifications: 1 of 2 elements
  verified unchanged") when the branch that distinguishes the two is removed.

Not yet run: the recorded `01-before` → `02-after` Studio Pro update pair. The
renumbering assertion it exists to make is already covered in principle — the
comparison never reads an `$ID` — but the fixture remains the end-to-end proof.

## Open Questions

1. **Scratch-project conversion cost.** Each diff runs `mx convert` on a package
   (~seconds). Acceptable for an explicit command; too slow to run implicitly inside
   `mxcli check`. Should the converted package be cached under `~/.mxcli/`?
2. **DESCRIBE coverage is the real bound.** Which document types in a typical
   marketplace module lack DESCRIBE today? `Security$ModuleSecurity` and
   `Projects$ModuleSettings` appeared in the §3 control and need checking. The answer
   sizes the "not comparable" bucket and therefore the feature's honesty.
3. **Is the widget-instance difference in §3 the same phenomenon as #716?**
   *Partly answered by §4.* Upgrading widget definitions is a separate, explicitly
   invoked operation that rewrites widget instances on pages in place, preserving
   every `$ID`. So the §3 subtree differences are plausibly reconciliation
   artefacts of the same kind — but §4 measured the *definition upgrade*, not the
   *import*, and those need not behave alike. Still open, and still shared with
   [`PROPOSAL_widget_instance_reconciliation.md`](PROPOSAL_widget_instance_reconciliation.md).
4. **Modules with no recorded version.** `mx show-module-version` reports *"Module
   'DataWidgets' does not have a version"* while `mxcli show modules` reports
   `Marketplace v3.5.0` — they read different fields. mxcli has no writer for
   `AppStoreVersion`, so a hand-updated module cannot be recorded as such (FINDINGS
   #37). Should this proposal include `set module version`, or does that belong with
   phase 2?
   *§4 adds a third source:* a `_Docs/v<version>` unit inside the module, which
   tracked the update correctly (`v4.3.2` → `v4.5.0`). It is a read-only oracle and
   does not remove the need for a writer, but `diff` can use it to cross-check the
   version a project claims.
5. ~~**What is `GUID` actually for?**~~ **Answered by measurement 2026-08-11 — see
   §8.** It is the database's identity for an entity and for each of its
   attributes. Changing only an entity's `GUID`, with its name, table name and
   attributes untouched, destroys its data.
6. **Does Studio Pro warn before discarding a local edit?** *(new)* §4 shows the
   edit is gone from the stored model, but a snapshot cannot see a dialog. This
   changes how the proposal should describe the status quo: "Studio Pro silently
   destroys local edits" and "Studio Pro warns and then destroys them" support
   different claims about what mxcli is adding.
