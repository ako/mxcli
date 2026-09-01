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

The page list lives in [`maintain-wiki/pages.md`](maintain-wiki/pages.md) — the
wiki's table of contents. Stubs are fine; the structure matters more than initial
content. Adding a page outside it requires a stated reason it isn't better served
by an existing page or a different doc artifact.

It is a **table and nothing else**, in its own file, with `merge=union` in
`.gitattributes`. That is deliberate: every wiki sync appends a row, so two
parallel syncs would otherwise collide on one line and the work would have to be
serialised — which is exactly what happened to the five bug-pattern PRs, and how
13 pages went missing when the stack merged into itself. Union is safe here and
is NOT safe on this file, which is prose: union keeps both sides of an edited
prose line silently.

## Adding a new page

Before creating `docs-wiki/<new>.md`:

1. Confirm the topic does not fit an existing page.
2. Confirm it isn't better served by a skill (procedure), the user manual
   (how-to), source code (implementation), or proposal frontmatter (state).
3. Add a row to [`maintain-wiki/pages.md`](maintain-wiki/pages.md) as part of
   the same sync run, with its category. That file is the wiki's table of
   contents.

## Final checklist

- [ ] Every claim in the page is grounded in a file listed in `sources:`
- [ ] No sentence duplicates content that lives in a canonical home
- [ ] `last-synced:` updated to current HEAD SHA
- [ ] `sources:` reflects what was actually read this run
- [ ] `SYNC_LOG.md` row appended
- [ ] `maintain-wiki/pages.md` updated if a new page was added
