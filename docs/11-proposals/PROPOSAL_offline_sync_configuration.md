---
title: Offline synchronization configuration — the one thing an offline profile needs that MDL cannot say
status: draft
date: 2026-09-07
related:
  - navigation-support.md
  - https://github.com/ako/TestApp
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - docs/13-decisions/0005-semantic-model-interface-currency.md
---

# Offline synchronization configuration

> Prompted by a CapTrack screenshot of Studio Pro's **Customize offline
> synchronization** dialog. mxcli can create the profile that dialog belongs to,
> and can read every row in it, and cannot write a single one.
>
> Pinned against `ako/TestApp`, whose `TabletOffline` profile configures seven
> entities across all six sync modes — so §2.1 onward is measured against a
> stored document rather than inferred from the UI.

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

### 2.1 Measured against a real document

`ako/TestApp` carries a `TabletOffline` profile with **seven** configured
entities, covering **all six** members of the sync enum — so the shape below is
observed, not derived:

| Entity | SyncMode | Constraint | CompatibilityMode |
|---|---|---|---|
| `Mappings.Customer` | `Never` | — | false |
| `Pages.Bus` | `All` | — | false |
| `Rules.BusinessRule` | `Online` | — | false |
| `Rules.RuleAction` | `Constrained` | `[\n  (\n    contains(ActionValue, '''abc''')\n  )\n]` | false |
| `Rules.RuleCategory` | `None` | — | false |
| `Rules.RuleExecutionLog` | `NoneAndPreserveData` | — | false |
| `System.Language` | `All` | — | false |

**Studio Pro writes exactly four properties**, not the six `modelsdk/gen`
declares:

```
CompatibilityMode   Constraint   Entity   SyncMode
```

`DownloadMode` and `ShouldDownload` occur **zero times** in the document. gen
declares them, and nothing on a web profile writes them — presumably native-only.
That shrinks the carry problem considerably: the semantic model drops **one** of
the four written properties, `CompatibilityMode`, not three of six.

Three further observations that change the design:

- **`System.Language` is configured.** A validation rule that refuses System or
  Marketplace entities here would reject a real document.
- **The constraint is stored multi-line**, with embedded newlines and Mendix's
  doubled-quote escaping (`'''abc'''` — a quoted literal inside a quoted XPath).
  MDL's own string escaping has to survive a round trip of that, which is the
  one part of the syntax with a non-obvious test.
- **The typed-array marker is `3`**, and `OfflineEntityConfigs` is present on
  every web profile including the online `Responsive` one, where it is the empty
  `[3]`.

### 2.2 Two properties the generated sources do not have

**`CompatibilityMode` is real.** It is written on all seven configs.
`generated/metamodel` declares three properties and is missing it; `gen` has it.
This is exactly the documented caveat on the arbiter rule — `generated/metamodel`
is a **snapshot of 11.6.0**, sound for what it contains and silent about
anything added later. Settled in gen's favour, by a document.

**`ThrowPartialSyncError` is in neither.** The screenshot's "Throw error when
server rejects objects during synchronization" is stored as a profile-level bool
of that name, and it occurs **zero times** in `modelsdk/gen` *and* zero times in
`generated/metamodel`. It is also on the online `Responsive` profile, so it
belongs to every web profile rather than to offline ones.

A property neither generated source knows about cannot be written through the
codec's typed accessors at all. It has to be an overlay onto the stored profile
document — which is what `mdl/settingsoverlay` already does, and which brings its
rules with it: write only a key the document already carries, and never invent
one. That is a constraint on the design, not a detail.

## 3. The trap this feature is walking into

The sync mode is an enumeration, and `generated/metamodel` declares six members:

```
All   Constrained   Never   None   NoneAndPreserveData   Online
```

Studio Pro's dropdown shows **captions** — the screenshot's three visible rows
read "Online", "All Objects" and "By XPath". Neither "All Objects" nor "By XPath"
is a member of the enumeration.

The reference document settles the mapping rather than leaving it to be guessed:
the entity whose row shows "By XPath" is stored as `Constrained`, and the one
showing "All Objects" as `All`. All six members occur in that one profile, so
the dialog exposes more captions than the screenshot happened to show.

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
test.** All six members occur in the reference document, so all six need words
and none is speculative:

| MDL | Stored |
|---|---|
| `ONLINE` | `Online` |
| `ALL` | `All` |
| `NEVER` | `Never` |
| `WHERE '<xpath>'` | `Constrained` |
| `NONE` | `None` |
| `NONE PRESERVE DATA` | `NoneAndPreserveData` |

`None` and `NoneAndPreserveData` differ in whether data already on the device
survives, which is why the second is a modifier on the first rather than an
unrelated word.

### 4.2 Writing

`ALTER NAVIGATION <profile> SYNC ( … )` replaces the whole list, the way a
`MENU (…)` block replaces the menu. Per-entity `ADD` / `DROP` verbs are
deliberately not proposed: the list is small, wholly visible in one describe,
and diff-friendly as a block.

The write is an **overlay on the stored element**, not a rebuild — §2.1. For an
entity already configured, `CompatibilityMode` is read from the stored config and
written back unchanged; it is the only written property MDL will not be able to
spell. A newly added entity gets `false`, which is what all seven reference
configs carry.

`DownloadMode` and `ShouldDownload` are **not written**, and the writer must not
start writing them. A property absent from every real document is one Studio Pro
fills in on load; emitting it is how a document mxbuild accepts becomes one
Studio Pro cannot open.

`ThrowPartialSyncError` is profile-level and unknown to both generated sources
(§2.2), so it is a separate raw-BSON overlay under the guard-don't-drop rules —
written only onto a document that already carries the key.

### 4.3 Catalog and references

`OfflineEntityCount` becomes rows: an entity synced by an offline profile is a
reference target, so `show references to CapTrack.Movement` names the profile.
This is the same argument that made a widget a reference target — "which
profiles sync this entity?" is the question an offline change asks, and it is
currently unanswerable.

## 5. The reference document

`ako/TestApp` is the reference, and its `TabletOffline` profile answers every
question this proposal originally listed as blocking. The four are closed:

1. **Which properties Studio Pro writes** — four: `Entity`, `SyncMode`,
   `Constraint`, `CompatibilityMode`. `DownloadMode` and `ShouldDownload` occur
   zero times and must not be written (§2.1).
2. **Whether `CompatibilityMode` exists on this version** — yes, on all seven
   configs. `gen` is right and `generated/metamodel` is a stale snapshot, exactly
   as its documented caveat allows (§2.2).
3. **The caption-to-key mapping** — confirmed against the stored values, with all
   six enum members present in one profile (§3).
4. **Where the throw-on-reject setting lives** — `ThrowPartialSyncError`, a
   profile-level bool on every web profile, and **absent from both generated
   sources** (§2.2).

An earlier scan of this machine reported five projects with offline configs and
was wrong: it matched the `OfflineEntityConfigs` key that every profile carries,
not the element. No local project has one; `ako/TestApp` had to be fetched.

What is still unmeasured, and does not block phase 1:

- **Native profiles.** `OfflineEntityConfigs` is declared on
  `NativeNavigationProfile` too, and `DownloadMode`/`ShouldDownload` are the
  obvious candidates for being written there. Nothing in this proposal touches
  native, and a native reference document is needed before it does.
- **`CompatibilityMode: true`.** All seven references are `false`, so the value
  is carried but the true case has never been seen. Carrying it is safe;
  authoring it is not proposed.

## 6. Phasing

1. **Carry all six properties on READ**, and add the round-trip test. No syntax,
   no writing. On its own this changes nothing a user sees, and it is the
   precondition for every later slice not being a silent downgrade.
2. **`SYNC` block in `CREATE`/`ALTER NAVIGATION`**, overlay write, the enum-key
   guard test, and `DESCRIBE` emitting real MDL instead of a comment — which
   closes the describe → exec round trip.
3. **Catalog rows and the reference edge.**

`ThrowPartialSyncError` belongs to slice 2 but is a separate mechanism — a
raw-BSON overlay rather than a codec write (§2.2) — so it can land after the
`SYNC` block without holding it up.

Native profiles are out of scope until a native reference document exists (§5).

## 7. Overlap

[`navigation-support.md`](navigation-support.md) is `status: partial` and listed
offline entity configs in its scope. Its reading half shipped; this proposal is
the authoring half and does not restate its findings. Nothing in
`mdl-examples/doctype-tests/` covers offline sync — `navigation-profiles.mdl`
and `11-navigation-examples.mdl` are home pages and menus only.
