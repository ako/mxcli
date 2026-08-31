# Maintain Wiki Skill

Synthesize and update concept pages in `docs-wiki/` — the agent-maintained
"brain" that sits between raw docs, source code, skills, and the user manual.

The wiki's job is to **frame and connect**, never to re-state. If a fact has a
better home elsewhere, the wiki links to that home; it does not copy.

## What the wiki IS for

Six page categories. Anything outside these belongs somewhere else.

1. **Architectural narratives** — end-to-end pipelines as stories (e.g. MDL
   execution flow, MPR read/write, widget BSON serialisation).
2. **Mental models** — counter-intuitive invariants explained at concept level
   (e.g. association `ParentPointer`/`ChildPointer` inversion, storage names
   vs qualified names, version gating).
3. **Design rationale** — *why* the project is shaped this way (e.g. why MDL
   is SQL-shaped, why the executor must not import `sdk/mpr` for writes, why
   pure-Go SQLite).
4. **Project positioning** — how mxcli relates to its neighbours (TypeScript
   SDK, Mendix Studio Pro), what is intentionally not implemented.
5. **Glossary / vocabulary bridge** — Mendix ↔ mxcli ↔ BSON terminology.
6. **Bug-pattern taxonomy** — *categories* of recurring failure modes that
   link out to findings in `.claude/skills/fix-issue/findings/*.jsonl`.

## What the wiki is NOT for

If you find yourself writing one of these, stop and link to the canonical
home instead.

| Content type | Canonical home |
|--------------|----------------|
| How to use a command or MDL statement | `docs-site/` (user manual) |
| MDL syntax tables | `docs/01-project/MDL_QUICK_REFERENCE.md` |
| What a function does | source code |
| Step-by-step task procedure | `.claude/skills/<task>.md` |
| Specific bug fix recipe | `.claude/skills/fix-issue/findings/*.jsonl` |
| Proposal status, PR / issue numbers, roadmap | proposal frontmatter; GitHub |
| Latest design proposal | `docs/11-proposals/` |
| Architecture decision record | `docs/13-decisions/` (ADRs) |
| Changelog | `CHANGELOG.md` + git history |

**Rule**: if a value can change without anyone touching the wiki, it does not
belong in the wiki — only the synthesis around it does.

## Page template

Every page starts with this header. The "Do not duplicate" line is
load-bearing — it's how future syncs avoid re-stating canonical content.

```markdown
---
title: <concept name>
category: architecture | mental-model | rationale | positioning | glossary | bug-pattern
last-synced: <git short SHA at sync time>
sources:
  - docs/11-proposals/<file>.md
  - docs/13-decisions/<file>.md
  - mdl/executor/<file>.go
  - .claude/skills/<file>.md
---

> **Do not duplicate**: <list the canonical homes this page links to instead
> of re-stating, e.g. "syntax tables in MDL_QUICK_REFERENCE; symptom recipes
> in fix-issue skill">.

## What this is

<2-4 sentence concept summary — the framing a reader needs before clicking
into any of the sources.>

## How it fits

<The narrative or invariant. Concept-first prose, no procedure, no syntax
reference. Link out for specifics.>

## See also

- [Specific evidence file](path) — what it covers
- [[other-wiki-page]] — related concept
```

## Synthesis procedure

For each page being synced:

1. **Read every file in `sources:`.** If a source no longer exists, drop it
   from the list and note it in the sync log. Do not synthesize from memory.
2. **Re-read the "Do not duplicate" guardrail.** As you draft, check each
   sentence: would this sentence be more accurate in one of the linked
   canonical homes? If yes, cut it and link instead.
3. **Synthesize concept-first prose.** Two sections: *What this is* (framing)
   and *How it fits* (the model or narrative). Resist drifting into syntax,
   procedure, or status.
4. **Update `last-synced:`** to the current `git rev-parse --short HEAD`.
5. **Refresh `sources:`** to reflect what you actually read this run, not
   what the previous run read.
6. **Append a row to `docs-wiki/SYNC_LOG.md`** (see below). This is the last
   step and is non-optional.

## Sync log discipline

`docs-wiki/SYNC_LOG.md` is append-only and is the audit trail for every sync
run. It records *what triggered the resynth* and *what was read* — information
git does not capture, because the sources are upstream of the commit.

Format:

```markdown
| Date | Page | Sources read | Note |
|------|------|--------------|------|
| 2026-05-24 | architecture/mdl-execution.md | docs/11-proposals/p123.md, mdl/executor/cmd_pages.go | Reflect backend abstraction split |
```

Rules:

- **Append only.** Never edit historical rows. A re-sync is a new row.
- **The Sources column lists what was actually read**, not what was relevant.
  This is the audit trail; a reviewer can verify synthesis is grounded.
- **Write the row as the final step of every sync.** No exceptions. Same
  discipline as appending a finding under `fix-issue/findings/`.

## Seed topic pages

Start with these. Stubs are fine; the structure matters more than initial
content. Adding pages outside the seed list requires a stated reason it
isn't better served by an existing page or a different doc artifact.

| Path | Category | Frames |
|------|----------|--------|
| `architecture/mdl-execution.md` | architecture | grammar → AST → visitor → executor → backend → MPR writer |
| `architecture/mpr-read-write.md` | architecture | MPR v1/v2, BSON round-trip, write safety |
| `architecture/widget-engine.md` | architecture | def.json, WidgetRegistry, V3 builders |
| `models/association-pointers.md` | mental-model | why `ParentPointer` = FROM, `ChildPointer` = TO |
| `models/element-identity.md` | mental-model | `$ID` vs `GUID` vs `StableId`; the unit as identity boundary |
| `models/storage-vs-qualified-names.md` | mental-model | BSON `$type` vs SDK qualified name |
| `models/version-gating.md` | mental-model | feature registry, `min_version`, `checkFeature()` |
| `rationale/mdl-as-sql.md` | rationale | why MDL is SQL-shaped, design principles (cites ADRs) |
| `rationale/backend-abstraction.md` | rationale | why the executor never imports `sdk/mpr` for writes (cites ADRs) |
| `positioning/vs-typescript-sdk.md` | positioning | gap analysis, intentional differences |
| `glossary.md` | glossary | Mendix ↔ mxcli ↔ BSON term bridge |
| `bug-patterns/bson-numeric-width.md` | bug-pattern | int32/int64 mismatches (links #583, #585 findings) |
| `bug-patterns/visitor-wiring-gaps.md` | bug-pattern | parsed-but-not-stored (links #393 finding) |
| `bug-patterns/widget-type-object-drift.md` | bug-pattern | CE0463 family |

## Adding a new page

Before creating `docs-wiki/<new>.md`:

1. Confirm the topic does not fit an existing page.
2. Confirm it isn't better served by a skill (procedure), the user manual
   (how-to), source code (implementation), or proposal frontmatter (state).
3. Add it to the seed table above as part of the same sync run, with its
   category. The seed table is the wiki's table of contents.

## Final checklist

- [ ] Every claim in the page is grounded in a file listed in `sources:`
- [ ] No sentence duplicates content that lives in a canonical home
- [ ] `last-synced:` updated to current HEAD SHA
- [ ] `sources:` reflects what was actually read this run
- [ ] `SYNC_LOG.md` row appended
- [ ] Seed table updated if a new page was added
