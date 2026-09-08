---
title: Offline synchronization configuration — the one thing an offline profile needs that MDL cannot say
status: draft
date: 2026-09-07
related:
  - navigation-support.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - docs/13-decisions/0005-semantic-model-interface-currency.md
---

# Offline synchronization configuration

> Prompted by a CapTrack screenshot of Studio Pro's **Customize offline
> synchronization** dialog: three entities, three different sync modes, one XPath
> constraint. mxcli can create the profile that dialog belongs to, and can read
> every row in it, and cannot write a single one.

## 1. Problem

`mxcli` can author an offline navigation profile — `PhoneOffline`,
`ResponsiveOffline` and `TabletOffline` are three of the six creatable kinds, and
`CREATE NAVIGATION PhoneOffline …` has worked since profile creation landed.

What it cannot author is the thing that makes an offline profile *do* anything.
A Mendix offline profile downloads nothing until each entity is given a
synchronization mode, and that configuration is unreachable from MDL. The result
is a profile that builds, routes and installs as a PWA, and shows an empty app.

`DESCRIBE NAVIGATION` is explicit about the gap, in the output itself:

```
-- Offline Entities (not yet modifiable):
-- SYNC CapTrack.Team MODE ByXPath where '[...]';
```

That comment is the whole feature request. It also makes describe → exec lossy
for any project that uses offline sync, which is the same failure that preceded
the layout and menu-icon work: a round trip that silently returns less than it
was given.

## 2. What exists today — measured

| | State |
|---|---|
| Creating an offline profile | **Works.** `CanonicalProfileKind` accepts `ResponsiveOffline`, `PhoneOffline`, `TabletOffline` |
| Reading the sync rows | **Works, both engines.** `sdk/mpr/parser_misc.go:550` and `mdl/backend/modelsdk/navigation_read.go:213` |
| `DESCRIBE NAVIGATION` | Emits the rows **as comments**, marked "not yet modifiable" |
| Authoring | **Absent.** No `offline` token exists in any `.g4` file; `navigationClause` is `HOME` / `LOGIN PAGE` / `NOT FOUND PAGE` / `MENU` |
| Writing | **Absent.** `navigation_profile_add.go:122` writes `OfflineEntityConfigs` as `bson.A{int32(3)}` — the empty typed-array marker |
| Catalog | Only `OfflineEntityCount`, a **count**. "Which entities sync offline?" is not queryable |
| Validation | `MDL-OFFLINE01` already exists, and warns when a page reachable from an offline profile binds an attribute across more than one association (CE6206) |

**An existing rewrite does not destroy the configuration**, and this was checked
first because it decides whether this is a feature or a rescue. Both engines
*patch* the stored profile rather than rebuilding it: `navPatchWebProfile` writes
`HomePage`, `HomeItems`, `LoginPageSettings`, `NotFoundHomepage` and the menu,
and never touches `OfflineEntityConfigs`; legacy goes through `readPatchWrite`.
A hand-configured sync setup therefore survives `CREATE OR REPLACE NAVIGATION`
today. This is a clean gap, not a data-loss defect — the opposite of what
`create or modify entity` was doing to access rules.

### 2.1 The read is lossy, and that is the hazard

`modelsdk/gen` declares **six** properties on `Navigation$OfflineEntityConfig`:

```
Entity  DownloadMode  ShouldDownload  SyncMode  Constraint  CompatibilityMode
```

`mdl/types.NavOfflineEntity` keeps **three** — `Entity`, `SyncMode`,
`Constraint`. `DownloadMode`, `ShouldDownload` and `CompatibilityMode` are read
and discarded. `CompatibilityMode` is the column carrying warning triangles in
the screenshot, so it is not hypothetical.

That matters the moment authoring exists. A writer that builds a config element
from the three fields the semantic model carries writes a document missing the
other three — which is precisely the class of defect that had
`create or modify entity` deleting access rules, and `create or modify entity`
is the more instructive precedent: the fix there was not to extend the carry
list but to **invert the direction**, starting from what is stored and
overwriting only what the statement declares.

So this proposal's first rule: **the read must carry all six properties before
the write carries any.** MDL will be able to spell three of them; the other
three must survive a rewrite untouched. The precedent is `ruleInfoFromGen` for
validation rules, where the payload is carried on READ specifically so a
rewrite can be refused or preserved rather than silently downgraded.

### 2.2 `generated/metamodel` and `gen` disagree, and the snapshot is why

`generated/metamodel.NavigationOfflineEntityConfig` declares **three**
properties — `Constraint`, `Entity`, `SyncMode`. CLAUDE.md makes
`generated/metamodel` the arbiter when the two disagree, with one caveat that
applies exactly here: it is a **snapshot of 11.6.0**, so it is sound for what it
contains and says nothing about properties introduced later.

`CompatibilityMode` appears in the Studio Pro UI of the version in the
screenshot. The likeliest reading is that it postdates the snapshot rather than
that `gen` invented it — but *likeliest* is not measured, and the rule for that
is to get a real document.

## 3. The trap this feature is walking into

The sync mode is an enumeration, and `generated/metamodel` declares six members:

```
All   Constrained   Never   None   NoneAndPreserveData   Online
```

Studio Pro's dropdown shows **captions**: "Online", "All Objects", "By XPath".
Neither "All Objects" nor "By XPath" is a member of the enumeration.

This is the same defect that shipped as `mendixlabs/mxcli#1035` two days ago —
`gallery.def.json` stored `pagingPosition: "below"` because "Below grid" was the
caption of the member `bottom`, and mxbuild rejected every gallery with
`CE0463`. The lesson generalises: **a value read off a Studio Pro screen is a
caption until proven otherwise**, and the six-member enum here does not map
one-to-one onto the three captions the dialog shows, so the mapping cannot even
be guessed from the UI.

The guard that closed #1035 is the model to copy:
`TestDefJSONEnumLiteralsAreDeclaredMPKKeys` asserts every literal mxcli writes is
a key the package declares. The equivalent here asserts every MDL sync word maps
to a member of `NavigationSyncMode`, and it must fail when the mapping table
gains a caption.

## 4. Design

### 4.1 Syntax

A per-entity statement, matching `navMenuItemDef`'s shape rather than the
`( key: value )` property form — this is a list of rules, not a property bag,
and the menu block is the established precedent for a list inside a profile.

```sql
CREATE NAVIGATION PhoneOffline
  HOME PAGE CapTrack.Mobile_Dashboard
  MENU (
    MENU ITEM 'Overview' PAGE CapTrack.Mobile_Dashboard ICON Atlas_Core.Atlas.home;
  )
  SYNC (
    SYNC CapTrack.Employee ONLINE;
    SYNC CapTrack.Movement ALL;
    SYNC CapTrack.Team WHERE '[Name = ''abc'']';
    SYNC CapTrack.Audit NEVER;
  );
```

Reading as English is ADR-0003's bar, and `SYNC CapTrack.Movement ALL` clears it
where `MODE Constrained` — the spelling the current describe comment invents —
does not.

Two decisions inside that:

**`WHERE` implies `Constrained`.** A constrained entity without a constraint and
a constrained constraint without the mode are both nonsense, so deriving the
mode from the clause makes the invalid pair unspellable rather than diagnosable.
The cost is that `Constrained` has no bare word, which is correct: there is
nothing to say.

**Every mode word maps to one enum member, and the mapping is a table with a
test.** `ONLINE`→`Online`, `ALL`→`All`, `NEVER`→`Never`, `WHERE`→`Constrained`.
`None` and `NoneAndPreserveData` need words too — and need a reference document
before they get them, because the difference between them is data retention on
the device and the dialog in the screenshot does not obviously expose either.

### 4.2 Writing

`ALTER NAVIGATION <profile> SYNC ( … )` replaces the whole list, the way a
`MENU (…)` block replaces the menu. Per-entity `ADD` / `DROP` verbs are
deliberately not proposed: the list is small, wholly visible in one describe,
and diff-friendly as a block.

The write is an **overlay on the stored element**, not a rebuild — §2.1. For an
entity already configured, `DownloadMode`, `ShouldDownload` and
`CompatibilityMode` are read from the stored config and written back unchanged.
For a newly added entity they take the codec's declared defaults, which is the
one case where a reference document is load-bearing: a default guessed wrong is
invisible until a device syncs.

### 4.3 Catalog and references

`OfflineEntityCount` becomes rows: an entity synced by an offline profile is a
reference target, so `show references to CapTrack.Movement` names the profile.
This is the same argument that made a widget a reference target — "which
profiles sync this entity?" is the question an offline change asks, and it is
currently unanswerable.

## 5. What needs a reference document before implementation

There is **no local project with a populated offline entity config.** Every
project on this machine carries the empty `OfflineEntityConfigs` key, which
every navigation profile has; none carries a `Navigation$OfflineEntityConfig`
element. An earlier scan of this reported five hits and was wrong — it matched
the key name, not the element.

So the shape must come from a real document, and CapTrack in the screenshot is
one: three entities, three modes, one XPath constraint, and visible
compatibility-mode state.

What a dump of that document settles, none of which should be guessed:

1. **Which of the six properties Studio Pro actually writes**, and their
   defaults — particularly whether `ShouldDownload` and `DownloadMode` are
   written at all on a web profile or are native-only.
2. **Whether `CompatibilityMode` really exists on this version**, closing §2.2.
3. **What the caption-to-key mapping is**, confirming "All Objects"→`All` and
   "By XPath"→`Constrained` rather than assuming it.
4. **Where "Throw error when server rejects objects during synchronization"
   lives.** It is on neither `gen`'s `NavigationProfile` nor the metamodel's, so
   it is either a later property or is not stored on the profile at all. It is
   in the screenshot, so it is stored somewhere.

## 6. Phasing

1. **Carry all six properties on READ**, and add the round-trip test. No syntax,
   no writing. On its own this changes nothing a user sees, and it is the
   precondition for every later slice not being a silent downgrade.
2. **`SYNC` block in `CREATE`/`ALTER NAVIGATION`**, overlay write, the enum-key
   guard test, and `DESCRIBE` emitting real MDL instead of a comment — which
   closes the describe → exec round trip.
3. **Catalog rows and the reference edge.**

`None` / `NoneAndPreserveData` words, and the throw-on-reject setting, are held
back to whichever slice the reference document lands in.

## 7. Overlap

[`navigation-support.md`](navigation-support.md) is `status: partial` and listed
offline entity configs in its scope. Its reading half shipped; this proposal is
the authoring half and does not restate its findings. Nothing in
`mdl-examples/doctype-tests/` covers offline sync — `navigation-profiles.mdl`
and `11-navigation-examples.mdl` are home pages and menus only.
