---
title: Translations — preserve, describe, author, and auto-translate
status: partial
date: 2026-08-23
related:
  - PROPOSAL_catalog_integration.md
  - PROPOSAL_marketplace_module_upgrade.md
---

# Proposal: Translations — preserve, describe, author, and auto-translate

**Status:** Partial — all four slices shipped; see Implementation Status.
**Date:** 2026-08-23

A Mendix app ships its user-visible strings in every language it supports.
mxcli can *report* on those strings and *display* the right one, but it cannot
author a translation — and, more urgently, it **deletes translations it did not
write**. This proposal covers both halves, in that order.

## Problem Statement

### The bug: a rewrite drops every language but one

Measured on Mendix 11.13.0, describing `Administration.MyAccount` (a stock
marketplace page with Dutch translations) and re-executing that description:

```
                        before   after
en_US markers in unit      24      20
nl_NL markers in unit      20      13
"Mijn account"          present   GONE from the entire project
```

Project-wide, `SHOW LANGUAGES` went `nl_NL 17 → 16` from that one page.

The mechanism is `extractTextFromBson` (`sdk/mpr/parser_misc.go:637`), which
returns the first non-empty text and ignores `LanguageCode` entirely:

```go
for _, item := range extractBsonArray(raw["Items"]) {
    if transMap, ok := item.(map[string]any); ok {
        text := extractString(transMap["Text"])
        if text != "" {
            return text          // ← eight translations collapse to one string
        }
    }
}
```

Every writer then re-emits that single string as `en_US` — there are **75
hardcoded `"en_US"` sites** in non-test code.

`mx check` does not care: a model missing a translation is a valid model, and
mxbuild reports 0 errors before and after. This is the same silent-loss class as
[#901](https://github.com/mendixlabs/mxcli/issues/901) (delete behaviour) and
[#956](https://github.com/mendixlabs/mxcli/issues/956) (action slots), and it is
live against every marketplace module, all of which ship translations.

### The gap: no way to author one

There is no MDL syntax for a translation. `Title:`, `Caption:`, `Content:` and
`Label:` take one string and it goes to the default language.

Studio Pro's answer is *Language operations → export to Excel*, edit, import.
mxcli has no equivalent, so a project driven from MDL is monolingual by
construction.

### What already works

- **`SHOW LANGUAGES`** — translated-string counts per language, from the catalog.
- **`CATALOG.strings`** — every indexed string with its `Language`, queryable.
- **`ALTER SETTINGS LANGUAGE DefaultLanguageCode = 'en_US'`**.
- **Language-aware DESCRIBE** (`mdl/executor/describe_language.go`, issue #702) —
  picks the project's *default* language rather than whichever translation is
  stored first.

So reading for display is in reasonable shape. Preservation and authoring are not.

## BSON Structure

Verified by walking every `.mxunit` in a real 11.13.0 project (11 modules):

```json
{
  "$ID":   { "Subtype": 0, "Data": "YEEClXaA+0C8dvdDrVTrNw==" },
  "$Type": "Texts$Text",
  "Items": [
    3,
    { "$ID": …, "$Type": "Texts$Translation",
      "LanguageCode": "en_US",
      "Text": "Help us make your experience better and share your feedback with us!" },
    { "$ID": …, "$Type": "Texts$Translation",
      "LanguageCode": "nl_NL",
      "Text": "…" }
  ]
}
```

Measured facts, not assumptions:

| Fact | Measurement |
|------|-------------|
| `Items` is a versioned array with leading marker `3` | **3299 of 3299** texts |
| Children are `Texts$Translation{LanguageCode, Text}` | all |
| Texts carrying translations but no default language | **2** of 3299 |
| Texts with **no** items at all (unset captions) | **2089** of 3299 |

`Texts$Text` is a *leaf value* embedded wherever a caption lives — page titles,
widget captions, enum captions, log/validation messages, workflow task names,
menu items. It is never a document of its own. That is what makes a generic walk
possible: **one traversal covers every document type, present and future.**

### Corpus size

Same project:

```
Texts$Text elements            3299
  with a default-language source  1054     (2089 empty)
  DISTINCT source strings          411     ← the size of a translation file
  already multi-language           338
  per language: en_US=1054 nl_NL=330 de_DE=19 es_ES=19 pt_PT=19
                tr_TR=19 fr_FR=18 hi_IN=17 ar_DZ=4
```

**411 distinct strings for a whole app.** One file per language is comfortably
practical. Deduplication is a 2.6× reduction and, more importantly, means `Save`
is translated once and lands in all 40-odd places it occurs.

### `CATALOG.strings` cannot be the export source

The catalog's string index is built from **21 hand-listed contexts**
(`mdl/catalog/builder_strings.go`) — page title, page URL, documentation, enum
caption, workflow name/description, task name, REST paths, log/show/validation
messages. It does **not** index widget captions, which are the bulk of a page.

On `Administration.MyAccount`: **39 `Texts$Text` in the unit, 2 rows in the
catalog.** Export must do its own walk. (Widening the catalog is worth doing —
see Open Questions — but nothing here should be built on it.)

## Proposed MDL Syntax

Following `.claude/skills/design-mdl-syntax.md`: a translation entry maps a
*user-provided name to another name*, which is the `as` case, not the colon case
(the skill's own example is `CUSTOM NAME map ('kvkNummer' as 'ChamberOfCommerceNumber')`).

```mdl
-- translations/nl_NL.mdl
create translations for nl_NL (
    'Save'                as 'Opslaan',
    'Cancel'              as 'Annuleren',
    'My Account'          as 'Mijn account',
    'Change password'     as 'Wachtwoord veranderen',
);
```

Scoped to one module when a project is translated piecemeal:

```mdl
create translations in Administration for nl_NL (
    'My Account'          as 'Mijn account',
);
```

### `CREATE`, `CREATE OR MODIFY`, `CREATE OR REPLACE`

The three map onto MDL's established convention (`cmd_pages_create_v3.go:63` —
bare `CREATE` errors when the thing exists, `OR REPLACE` rebuilds wholesale,
`OR MODIFY` updates in place). The thing that exists here is **the language**:

| Statement | Meaning |
|-----------|---------|
| `create translations for de_DE (…)` | de_DE has no translations yet — make them. **Errors** if it already does. |
| `create or modify translations for nl_NL (…)` | Merge. Sources named are set; sources not named are left alone. |
| `create or replace translations for nl_NL (…)` | **The file is authoritative.** Any nl_NL translation whose source is not in the file is REMOVED. |

Bare `CREATE` is therefore the "add a new language" statement — one file, and it
refuses rather than silently colliding with an existing translation set.

`OR REPLACE` is what makes drift self-correcting. Take the `Save` → `Store`
rename above: under `OR MODIFY` the stale `Opslaan` stays attached to the
renamed text (the unmatched-key warning reports it, but the model stays wrong
until someone acts); under `OR REPLACE` that translation's source is not in the
file, so it is removed and the project's Dutch becomes exactly what the file
says. That is the semantics for "nl_NL.mdl is our Dutch translation, in version
control, reviewed in PRs" — file and project cannot silently diverge.

Two constraints follow, and both are load-bearing:

- **`OR REPLACE` deletes real work.** A translation made in Studio Pro and never
  added to the file is gone. Under guard-don't-drop
  ([ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md)) that
  cannot be quiet: the run reports the count and the strings it is about to
  remove, and `mxcli diff translations` is the preview.
- **Scope bounds the deletion.** `create or replace translations in Administration
  for nl_NL` removes nl_NL only from *Administration's* texts. Without that, a
  set of per-module files would each wipe the others' work on every run — which
  promotes `in <Module>` from a convenience to a requirement.

`or update` is deliberately not offered: `or modify` is MDL's established verb
and `.claude/skills/design-mdl-syntax.md` forbids inventing alternatives for
standard operations.

Inspect, and produce the file:

```mdl
describe translations for nl_NL;
describe translations in Administration for nl_NL;
```

`DESCRIBE` emits exactly the `CREATE` form, so the two round-trip. A language
with no translations yet describes as every source string with an **empty**
target:

```mdl
create translations for de_DE (
    'Save'                as '',
    'Cancel'              as '',
);
```

### Why this shape

- **Keyed on the source string, not an element id.** `$ID`s are renumbered by
  Studio Pro on every module update (94 of 94 in the measurement recorded in
  CLAUDE.md), so an id-keyed file rots the first time someone opens the project.
  Source-keying is also what gives deduplication, and it is how Mendix's own
  Excel flow behaves.
- **Upsert by nature.** A source string absent from the file is left alone; one
  present is set. Re-running is a no-op — see Implementation.
- **An empty target is "not translated yet", not "translate to empty".** It is
  skipped on write. This is what makes the describe-of-a-missing-language useful
  as an LLM prompt.

### The auto-translate loop

No separate export format is needed — it *is* the round-trip:

```bash
mxcli -p app.mpr -c "describe translations for de_DE" > de_DE.mdl
#   → 411 source strings, every target empty

#   hand de_DE.mdl to an LLM, fill in the right-hand side

mxcli exec de_DE.mdl -p app.mpr
```

The intermediate file is plain MDL: reviewable in a PR, diffable, and re-runnable.
A follow-up run after new strings are added describes only what is still empty if
`--untranslated` is passed (see Slice 4).

### Source drift — the failure mode source-keying has to answer for

A dictionary keyed on the source string cannot see the source move. The
scenario, and it is not exotic:

1. `create translations for nl_NL ('Save' as 'Opslaan')` runs.
2. Someone edits the English in Studio Pro: `Save` → `Store`. The `en_US` and
   `nl_NL` children of a `Texts$Text` are independent siblings, so this rewrites
   one child's `Text` and leaves the other untouched — the element is now
   `{en_US: 'Store', nl_NL: 'Opslaan'}`. That is Mendix's own behaviour, not
   something mxcli introduces.
3. The file is re-run. The walk reads the source as `Store`, finds no `Store`
   key, and **skips**. No write, no error. The Dutch now translates an English
   string that no longer exists.
4. `describe translations for nl_NL` emits `'Store' as 'Opslaan'` — the new
   source paired with the stale translation, presented as fact. Reviewing that
   output, by hand or with an LLM, gives no signal that anything is wrong.

**An unmatched key must therefore be reported, never skipped in silence.** The
evidence is available: the file's `'Save'` matches nothing, *and* a text reads
`'Store'` while carrying the `nl_NL` value the file assigns to `'Save'`.
Correlating backwards by the translation value identifies the moved source:

```
1 source string in the file matched nothing in the project:

  'Save' as 'Opslaan'
      No text has 'Save' as its en_US value.
      A text now reads 'Store' and carries nl_NL 'Opslaan' — the source was
      probably edited. Change the file to:   'Store' as 'Opslaan'
      and check the translation still fits.
```

Measured on the fixture project, that correlation is nearly always unambiguous:
**209 distinct (source, target) pairs across 191 distinct targets, with only 6
targets (3%) shared by more than one source** — and those are short generic words
(`"Knop"` ← 6 sources). So the hint is right ~97% of the time and honest about
the rest.

Three rules:

- **Unmatched keys warn by default**, and are an error under `--strict`. Silence
  is what lets drift compound across releases.
- **The correlation is never auto-applied.** `Save → Store` keeps the translation
  valid; `Save → Delete` does not. The tool reports; a person or a model decides.
- **`mxcli diff` is the home for the full report** — it already compares a script
  against project state. `mxcli diff translations nl_NL.mdl -p app.mpr` →
  matched / drifted / unmatched / untranslated.

**Why not key on the element instead.** That trades a detectable problem for an
undetectable one. A `Texts$Text` carries a `$ID` but no `GUID`, and Studio Pro
renumbers every `$ID` in a module on update (94 of 94, per CLAUDE.md). An
id-keyed file rots silently the first time anyone updates a marketplace module,
with no trace at all. Source-keying at least leaves evidence, and it is what
makes deduplication and hand-editing work. This is the translation-memory
problem, and TM tools answer it the same way: keep the dictionary source-keyed,
flag "source changed", do not guess.

## Implementation Status

All four slices below are shipped. What remains is one convenience flag and the
open questions at the end of this document; the feature itself is usable.

| Slice | State | Landed in |
|-------|-------|-----------|
| 1 — Preservation | done | #245 |
| 2 — The `Texts$Text` overlay | done | #250 |
| 3 — `DESCRIBE TRANSLATIONS` | done | #250 |
| 4 — `CREATE TRANSLATIONS` + drift report | done, except `--untranslated` | #250 |

Shipped beyond the original plan, because running the feature exposed the need:

- **Enabled-language statements** — `ALTER SETTINGS ADD/REMOVE/ADD OR MODIFY
  LANGUAGE`, which is what Open Question 2 turned into (#257 and follow-ups).
  `create translations` now *warns* when the language is not enabled rather than
  refusing or enabling it silently.
- **An out-of-scope report** (`mdl/translations/outofscope.go`) — a scoped run
  names the entries its own scope kept it from reaching. Ledger #137: `in Ledger`
  never reaches the project-level NAVIGATION, so the pages went Dutch and the
  sidebar did not, under a message that read as success.
- **A removal form** — an `OR REPLACE` naming nothing takes a language's
  translations back out, which is otherwise unexpressible.
- **A lint rule** (`mdl/linter/rules/missing_translations.go`), a skill
  (`.claude/skills/mendix/translations/SKILL.md`), and a manual page
  (`docs-site/src/language/translations.md`).

Still unbuilt: **`--untranslated`** on `DESCRIBE`, to emit only the empty targets.
It is a cost optimisation for the LLM loop on an already-translated project, not
a correctness gap — the full describe is what the loop uses today.

## Implementation Plan

Four slices, each shippable alone. **Slice 1 is the bug and should go first.**

### Slice 1 — Preservation (no new syntax)

A rewrite must carry every translation through. Two parts:

1. `extractTextFromBson` and its callers keep the whole `map[string]string`
   rather than the first string. `model.Text.Translations` is *already* a map,
   and `textToGen` (`mdl/backend/modelsdk/domainmodel_write.go:536`) *already*
   writes every entry sorted — so the write side of that path is done. Two
   readers already populate the full map (`modelsdk/domainmodel.go:437`,
   `modelsdk/page.go:200`); the legacy `sdk/mpr` collapse is the offender.
2. Where a writer builds `Translations: map[string]string{"en_US": s}` from a
   single MDL string, it must merge into the stored text rather than replace it
   — guard-don't-drop, [ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md).

**Control:** `SHOW LANGUAGES` before and after a describe→exec round-trip over a
marketplace module. Counts must be identical. Without the control the test passes
against a build that never had the fix.

### Slice 2 — The `Texts$Text` overlay (the write engine)

Import must **not** go through the document rebuild path — that is exactly where
translations get dropped and where all the fidelity risk lives. It is a targeted
BSON overlay, and every piece exists:

| Piece | Where |
|-------|-------|
| Read a unit's raw BSON | `modelsdk/mpr/reader.go:350` `GetRawUnitBytes` |
| Pure-BSON patch (precedent) | `modelsdk/mpr/nav_patch.go` `PatchNavigationProfile` |
| Write it back | `modelsdk/mpr/writer_core.go:189` `WriteTransaction.WriteUnit` |

`WriteUnit` runs `reconcileWithStored`, so **ADR-0008 idempotence and `$ID`
preservation come for free**: a unit whose translations did not change is not
written, and one that did keeps every element identity.

The walk is `$Type == "Texts$Text"` → read the default-language child → look the
source up in the dictionary → add or replace the `Texts$Translation` child for the
target language, preserving marker `3` and the existing children's order and
`$ID`s. Type-agnostic: pages, microflows, workflows, enums, widgets, and every
document type added later, with no per-type code.

### Slice 3 — `DESCRIBE TRANSLATIONS`

The same walk, read-only: collect every default-language string, deduplicate,
sort, emit the `CREATE` form with each known target filled in.

### Slice 4 — `CREATE TRANSLATIONS`

Grammar, AST, visitor, executor handler → the Slice 2 overlay. Plus:

- **the drift report** — unmatched keys warned (errored under `--strict`), with
  the back-correlated suggestion. This is not optional polish: without it the
  feature loses track of the project silently, which is the failure mode the
  whole design has to answer for.
- `--untranslated` on describe to emit only empty targets, which keeps the LLM
  loop cheap on a project that is already 90% translated.

### Files to modify/create

| File | Change |
|------|--------|
| `sdk/mpr/parser_misc.go` | `extractTextFromBson` → keep all translations (Slice 1) |
| `sdk/mpr/writer_*.go` | merge into stored text instead of replacing (Slice 1) |
| `modelsdk/mpr/text_patch.go` | **new** — the `Texts$Text` walk + patch (Slice 2) |
| `mdl/translations/` | **new** — dictionary type, dedup, describe rendering |
| `mdl/grammar/domains/MDLSettings.g4` | `createTranslationsStmt`, `describeTranslationsStmt` |
| `mdl/ast/ast_translations.go` | **new** — AST node |
| `mdl/visitor/visitor_translations.go` | **new** |
| `mdl/executor/cmd_translations.go` | **new** — handlers |
| `mdl/backend/` | `ListTexts` / `PatchTexts` on the backend interface + mock stub |
| `mdl/catalog/builder_strings.go` | widen coverage (optional, see Open Questions) |
| `cmd/mxcli/syntax/features_settings.go` | syntax topic |
| `.claude/skills/mendix/` | new skill: translating an app |

## Version Compatibility

None needed. `Texts$Text` with `Items` + `Texts$Translation` children is the
storage shape across every version mxcli supports; the leading marker `3` was
observed uniformly (3299 of 3299) on 11.13.0 and is the same value
`modelsdk/widgets/augment.go:908` already writes. No feature registry entry, no
`checkFeature()` gate.

## Test Plan

- `mdl-examples/bug-tests/` — a round-trip over a module with real translations,
  asserting `SHOW LANGUAGES` counts are unchanged (Slice 1).
- `mdl-examples/doctype-tests/` — `create translations` → `describe translations`
  round-trip, including a source string that occurs in several documents (proves
  dedup writes to all of them) and one with an empty target (proves it is skipped).
- Unit tests on the patch function: marker preserved, existing children's `$ID`s
  preserved, adding a language, replacing an existing one, leaving an unlisted
  source alone.
- **Source drift**: translate a string, edit the source in the stored document,
  re-run — the run must WARN with the back-correlated suggestion, not skip in
  silence. Control: the same run before the source edit reports nothing.
- **The three verbs**: bare `CREATE` refuses when the language already has
  translations; `OR MODIFY` leaves an unlisted source's translation alone;
  `OR REPLACE` removes it and says so. The MODIFY case is the control that
  proves REPLACE's deletion is the statement's doing and not a side effect.
- **Scoped REPLACE**: `in <Module>` must not touch another module's translations
  — two per-module files applied in sequence must both survive.
- **Idempotence with a control**: re-running an import reports no writes, and
  `MXCLI_ALWAYS_WRITE=1` reports writes — without the second line, "no writes" is
  equally consistent with a comparison that never ran.
- `mx check` 0 errors, and a Studio Pro open, on a project after an import.
  **`mx check` is not the oracle here** — a lost translation is a valid model.

## Open Questions

Two of the five are settled by shipped work; the resolutions are recorded here
rather than deleted, since each was decided by a measurement.

### Settled

2. **The enabled-language list.** ~~Does a translation for an unenabled language
   do anything?~~ **Measured**: a stock 11.13 app enables exactly one language
   (`en_US`) while its documents carry translations in nine, all from marketplace
   modules — so Studio Pro stores and keeps them, and `mx check` passes. But the
   app does not *serve* an unenabled language, which makes translating 411
   strings into a language nobody can select the quiet failure here. Resolved by
   doing neither of the two things this question offered: `CREATE TRANSLATIONS`
   does not enable the language (that is a settings change, and not this
   statement's business) and does not refuse (the translations are legitimately
   stored) — it **warns**, and `ALTER SETTINGS ADD LANGUAGE` is the fix it names.
   Note the trap the skill now documents: `show languages` lists languages that
   have *translations*, not enabled ones, so it reports 8 where 1 is enabled.


3. **Catalog coverage.** ~~`CATALOG.strings` misses widget captions; worth
   widening so `SHOW LANGUAGES` and `search` reflect reality.~~ **Done**, and
   wider than the question assumed. Measured on a stock project the index held
   **69 of 3265 texts and 8 of 9 languages** — a language present only on an
   unindexed site was *invisible*, not undercounted, so `SHOW LANGUAGES` named 8
   and `search` returned nothing for a caption `DESCRIBE TRANSLATIONS` had just
   listed. It also blinded the QUAL005 lint rule that shipped with slice 4, which
   discovers its language set from the same table.

   The resolution was not to add cases. The five extractors were hand-written per
   type, so a sixth site cost a sixth case; the index is now built from **this
   proposal's own walk** (`translations.SitesInUnit`), which is what makes the two
   subsystems structurally unable to disagree about what the project contains.
   `StringContext` names the site (`Forms$ActionButton.Caption`) and `ObjectType`
   is derived from the unit `$Type`, so a document type Mendix adds later is
   handled with no list to maintain. Atlas design templates are ~70% of the corpus
   and are indexed rather than excluded, because `CREATE TRANSLATIONS` writes them
   — `ObjectType` is how a consumer filters them out.

5. **Ordering inside `Items`.** ~~Does Studio Pro care, as it did for widget
   `PropertyTypes`?~~ **No** — the patch preserves existing order and appends,
   and projects patched this way open in Studio Pro and build clean. What *did*
   bite, and was not this, is the **`$ID` form**: a new `Texts$Translation` must
   carry a 16-byte `$ID` as its **first** property or the build fails with
   `Expected '$ID' as the first property of a storage object`. See
   `mdl-examples/bug-tests/translation-id-form.mdl`.

### Still open

1. **Homographs.** One source string needing different translations in different
   contexts. Unchanged, and now with usage behind it: `DESCRIBE` flags a
   conflicting source (3 on a stock app), and `in <Module>` resolves the common
   case. Mendix's own Excel export has the same limitation. Worth deciding
   explicitly rather than discovering, but nothing has forced the decision yet.

4. **The 2 texts with translations but no default language.** Skipped on export,
   as planned. What Studio Pro does with such a text is still unconfirmed, and
   the import still does not invent a default-language entry for them.
