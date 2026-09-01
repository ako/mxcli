---
description: Re-synthesize pages in docs-wiki/ from their canonical sources
argument-hint: [page]
---

# /mxcli-dev:wiki-sync — Re-synthesize Wiki Pages

Re-synthesize one or more pages in `docs-wiki/` — the agent-maintained
conceptual brain. The wiki frames and connects; it never re-states content
that has a canonical home elsewhere.

**Read `.claude/skills/maintain-wiki.md` first.** That skill defines the
page template, the synthesis rules, and the "do not duplicate" guardrails.
This command is the trigger; the skill is the contract.

## Arguments

- `<path>` — sync a single page (e.g. `architecture/mdl-execution.md`).
- `--all` — sync every page in `docs-wiki/`.
- `--stale` — sync only pages whose `last-synced:` SHA is older than HEAD.

For `bug-patterns/` specifically, run **`make digest-status`** first: it reports
how many findings have landed since the last bug-pattern sync and which areas
no page mentions. That is the scope question for this category — the pages
digest `.claude/skills/fix-issue/findings/*.jsonl`, and an area with many
findings and no page is an undigested failure class, not a missing file.

If invoked with no arguments, ask the user which page(s) to sync. List the
seed table from `maintain-wiki.md` as options.

## Process

### Phase 1: Confirm scope

1. Determine the page list (one path, `--all`, `--stale`, or user choice).
2. For each page, list the current `sources:` from its frontmatter. Confirm
   with the user: "These sources will be re-read. Add or remove any?"
3. If the user has a specific *reason* for the sync (a proposal landed, a
   refactor changed the shape of something, a new ADR), capture it in one
   line for the sync-log note.

### Phase 2: Read sources

For each page being synced:

1. Read **every file** in `sources:`. Do not synthesize from memory.
2. If a source no longer exists, drop it from the list and note in the
   sync log.
3. If a new source is clearly relevant (e.g. a new ADR for a rationale
   page), add it.

### Phase 3: Re-synthesize

Follow the page template in `maintain-wiki.md`. For each page:

1. Update the two body sections (*What this is*, *How it fits*) — concept-
   first prose, no procedure, no syntax reference, no state.
2. **Check every sentence against the "Do not duplicate" line.** If a
   sentence belongs in a canonical home (user manual, source code, skill,
   ADR, proposal), cut it and link instead.
3. Update `last-synced:` to `git rev-parse --short HEAD`.
4. Update `sources:` to reflect what was actually read this run.
5. Update the *See also* section with current `[[wiki-link]]` cross-refs.

### Phase 4: Append to SYNC_LOG.md

This is the last step and is non-optional.

```markdown
| <YYYY-MM-DD> | <page path> | <comma-sep sources read> | <one-line note> |
```

Rules from the skill:
- Append-only — never edit historical rows.
- The Sources column lists what was **actually read**, not what was relevant.
- Write the row even if the synthesis produced no diff (records the audit).

### Phase 5: Report to user

Show the user:
- Which pages were re-synthesized.
- What changed at a high level (one bullet per page).
- The new SYNC_LOG.md row(s).
- Whether any sources were dropped or added.

Do **not** commit. The user reviews the diff and commits when ready.

## Adding a new wiki page

If the user is requesting a page that doesn't yet exist in
`docs-wiki/`, first verify it belongs in the wiki at all:

1. Is the topic served by an existing page? → extend that one.
2. Is it really a procedure (skill), reference (manual), implementation
   detail (source), state (proposal frontmatter / GitHub), or
   decision (ADR)? → route there instead.
3. If it genuinely belongs in the wiki, add it to the seed table in
   `maintain-wiki.md` first, with its category, before creating the file.

The seed table is the wiki's table of contents — new pages outside it
should be the rare exception, not the default.

## Important reminders

- **Synthesis quality > breadth.** Better to sync one page well than ten
  pages superficially.
- **Link, don't restate.** Every time you write a sentence, ask: "Is this
  fact better expressed by linking to its canonical home?"
- **State stays out.** Proposal status, PR numbers, version numbers — link
  to GitHub or the proposal frontmatter; do not mirror.
- **The sync log row is part of the deliverable.** A sync without a log
  row is incomplete.
