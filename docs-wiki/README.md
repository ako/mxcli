# Concept Wiki

Agent-maintained "brain" — synthesized concept pages that frame and connect the
other documentation artifacts. **Never restate** content that has a canonical
home; always link to it.

## Reading order

If you're new to mxcli internals, read in this order:

1. `architecture/mdl-execution.md` — how an MDL statement becomes a write
2. `architecture/mpr-read-write.md` — how MPR files are read and modified
3. `architecture/widget-engine.md` — pluggable widget serialisation
4. `models/` — counter-intuitive invariants you'll trip over
5. `rationale/` — *why* the codebase is shaped this way
6. `glossary.md` — terminology bridge across Mendix / mxcli / BSON

**Diagnosing a bug?** Start at [`bug-patterns/`](bug-patterns/) — the failure *classes*,
digested from the symptom table in
[`.claude/skills/fix-issue.md`](../.claude/skills/fix-issue.md). Grep that table for the
specific instance afterwards; it is ~1 MB and is not meant to be read whole. Pattern
coverage is partial, so a miss there means the finding has not been digested yet.

## How this wiki is maintained

- **On-demand only**, via `/mxcli-dev:wiki-sync`.
- Rules and template: [`.claude/skills/maintain-wiki.md`](../.claude/skills/maintain-wiki.md).
- Audit trail of every sync: [`SYNC_LOG.md`](SYNC_LOG.md).
- New pages outside the seed list need to be added to the skill's seed table first.

## What this wiki is NOT

| Looking for... | Go to |
|---------------|-------|
| How to use a command | `docs-site/` |
| MDL syntax tables | `docs/01-project/MDL_QUICK_REFERENCE.md` |
| What a function does | source code |
| Step-by-step procedure | `.claude/skills/<task>.md` |
| Specific bug recipe | `.claude/skills/fix-issue.md` |
| Proposal status / PR # / roadmap | proposal frontmatter, GitHub |
| Architectural decision history | `docs/13-decisions/` (ADRs) |
| Latest design proposal | `docs/11-proposals/` |
