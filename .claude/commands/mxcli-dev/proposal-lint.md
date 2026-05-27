# /mxcli-dev:proposal-lint — Audit and Clean Up Proposals

Audit `docs/11-proposals/` for status drift, missing frontmatter,
misplaced files, and convention violations.

**Read `.claude/skills/lint-proposals.md` first.** It defines the
frontmatter format, status vocabulary, and the seven lint rules. This
command is the trigger; the skill is the contract.

## Arguments

- (no args) — run the lint, produce a report, stop.
- `--apply` — only after the user has reviewed and approved the report:
  execute the proposed moves and frontmatter migrations.
- `--category <name>` — limit the run to one category (e.g.
  `frontmatter-missing`, `move-to-archive`). Useful for incremental
  cleanup.

## Process

### Phase 1: Walk and check

1. Walk `docs/11-proposals/` and `docs/11-proposals/archive/` (recursive).
2. Scan `/proposals/` at the repo root for stragglers.
3. For each file, run rules R1–R7 from the skill.
4. Collect findings into the eight categories defined in the skill.

### Phase 2: Report

Print a structured report:

```
## Proposal lint report

Files scanned: NN active, MM archived, K strays
Total findings: XX

### move-to-archive (N)
- `PROPOSAL_xxx.md` — status: done; propose: git mv → archive/PROPOSAL_xxx.md
- ...

### frontmatter-missing (N)
- `proposal-yyy.md` — inline status "Draft" found; propose: add YAML frontmatter
- ...

### readme-drift (N)
- `archive/PROPOSAL_zzz.md` listed in README under Active section
- `PROPOSAL_aaa.md` exists on disk but missing from README
- ...

(etc. for each category)
```

Stop after the report. Do **not** modify files.

### Phase 3: User direction

Ask the user:

- Which categories to apply, and in what order?
- Are there any flagged items to skip (e.g. a `done` proposal staying in
  active because work isn't fully finished, despite the inline status)?
- Should naming-convention violations (R7) be ignored for now? (Default
  recommendation: yes.)

### Phase 4: Apply (only with explicit approval)

For approved categories:

1. **Frontmatter migration**: prepend YAML frontmatter to each file,
   preserving the inline `**Status:**` line for one cycle.
2. **Moves**: use `git mv` so history is preserved. Update the README
   index to reflect new paths.
3. **Duplicate resolution**: keep one canonical location; for the root
   `/proposals/` stragglers, confirm with the user which to keep before
   removing.
4. **Supersession symmetry**: add the missing direction (`supersedes:` /
   `superseded-by:`) to frontmatter.

Run `mxcli` build/check if any moved file is referenced by build
scripts.

### Phase 5: Summary

Report what was applied vs deferred. Print a one-line entry suitable for
a commit message (e.g. `chore: lint proposals — move 5 to archive, add
frontmatter to 47`).

## Important reminders

- **The report is the primary deliverable.** Even with `--apply`, every
  run should produce a report first; apply is gated on user approval of
  that report.
- **`git mv` preserves history.** Never use `mv` + `rm` for moves.
- **Frontmatter migration is one-time.** After this cleanup, future
  runs should mostly find clean state — the lint becomes a sanity check,
  not a migration.
- **Don't touch proposal bodies.** Out of scope. If a proposal is wrong
  or stale, that's a separate concern from frontmatter/location.
