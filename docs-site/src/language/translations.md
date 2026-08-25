# Translations

Every user-visible string in a Mendix app — page titles, widget captions, button
labels, validation messages, enum captions, menu items — is a text with one
translation per language. MDL works on them in bulk: **one file per language**.

## The loop

```bash
# 1. export — an untranslated string comes back with an EMPTY target
mxcli -p app.mpr -c "describe translations for de_DE" > de_DE.mdl

# 2. fill in the right-hand sides, by hand or by handing the file to an LLM

# 3. write them back
mxcli exec de_DE.mdl -p app.mpr
```

`DESCRIBE` emits the `CREATE` form, so the export format and the import format
are the same file and the empty targets are the prompt. A whole app is typically
a few hundred distinct source strings, so one file per language stays practical.

## Statements

```sql
DESCRIBE TRANSLATIONS [IN <Module>] FOR <lang>;

CREATE            TRANSLATIONS [IN <Module>] FOR <lang> ( '<source>' AS '<target>', ... );
CREATE OR MODIFY  TRANSLATIONS [IN <Module>] FOR <lang> ( ... );
CREATE OR REPLACE TRANSLATIONS [IN <Module>] FOR <lang> ( ... );
```

Entries use `AS`, not a colon: a translation maps a user-provided name to another
name.

| Verb | Meaning |
|------|---------|
| `CREATE` | the "add a language" form — refuses if that language already has translations in scope |
| `CREATE OR MODIFY` | merge; a source string the file does not name keeps what it has |
| `CREATE OR REPLACE` | the file is authoritative — a translation whose source it does not name is **removed**, and the run says which |

`IN <Module>` scopes both directions, and under `OR REPLACE` it **bounds the
deletion** — without it, per-module files would wipe each other on every run.

## A translation is only built if its language is enabled

This is the one thing worth knowing before translating anything.

A translation written for a language the project has **not enabled** is stored in
the model, passes `mx check`, is kept by Studio Pro — and is **discarded at build
time**. Measured with `mxbuild --target=deploy`: the string appears nowhere under
`deployment/`, and no `translations_<code>.properties` is produced at all.

So it is possible to translate four hundred strings into German, see every run
report success, and get no German in the app. `CREATE TRANSLATIONS` warns when
the language is not enabled. Enable it first — see
[Project Settings](project-settings.md).

A stock app invites the mistake: it enables **one** language while its marketplace
modules ship translations in **nine**, so "other languages already have
translations here" is true and misleading.

> `SHOW LANGUAGES` lists languages that **have translations**, which is a
> different list — a stock app reports eight while one is enabled. The enabled
> list is in `DESCRIBE SETTINGS`.

## Drift: a source string that was edited

The dictionary is keyed on the **source string**, so `Save` is translated once
for every place it occurs. The flip side is that editing a source after the file
was written stops it matching.

A key that matches nothing is **reported, not skipped** — and where the
translation identifies the moved source unambiguously, the run names the fix:

```
Warning: 1 source string(s) in the file matched nothing in the project.

  "Thingz" as "Grejer"
      No text has "Thingz" as its source. A text now reads "Things" and carries
      the sv_SE "Grejer" — the source was probably edited. Change the file to:
        "Things" as "Grejer"
```

## Notes

- **The source language** is the project's default: the left-hand column is what
  `DESCRIBE` shows for it. Translating *into* the source language is refused — it
  would overwrite the strings everything else is keyed on.
- **A rewrite keeps other languages.** Re-executing a page or microflow does not
  drop the translations MDL cannot express, so you need not re-import a language
  after editing a document.
- **To remove a language's translations**, make an empty file authoritative:
  `CREATE OR REPLACE TRANSLATIONS FOR de_DE ( );`
