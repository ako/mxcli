# Diff

mxcli provides two diff commands for comparing MDL scripts against project state and viewing local changes in MPR v2 projects.

## mxcli diff

Compares an MDL script against the current project state, showing what would change if the script were executed. This is a dry-run preview.

**Usage:**

```bash
mxcli diff -p app.mpr changes.mdl
```

This shows:
- Elements that would be created (new entities, microflows, pages)
- Elements that would be modified (changed attributes, altered properties)
- Elements that would be removed (DROP statements)

Use `mxcli diff` to review changes before applying them, especially when working with AI-generated scripts.

### What diff does not compare

`diff` compares entities, view entities, enumerations, associations, microflows
and nanoflows. Every other statement — `grant`, `create constant`, pages,
navigation, settings — is listed under **Not compared** after the summary:

```
Summary: 0 new, 0 modified, 1 unchanged

Not compared (2 statement(s)) — diff has no comparison for these,
so they are absent from the summary above, not unchanged:
  create constant x1
  grant microflow access x1
```

Read that list. The counts describe only the statements diff understands, so a
script made entirely of the others summarises as all zeros — which means "not
examined", not "no change". Those statements were previously skipped without a
word, so the summary looked like a clean bill of health for a script that would
add documents (#997).

### Both sides go through one renderer

The project side and the script side are rendered by the same describer
`describe microflow` uses, so an unmodified `describe` dump diffs as
**unchanged**. Before this, the script side had a renderer of its own that
covered 18 of 43 activity types and silently emitted nothing for the rest, so a
java-action call, a `download file` or a canvas annotation appeared as a
deletion in a script that changed nothing at all.

## mxcli diff-local

Compares local changes against a git reference for MPR v2 projects. MPR v2 (Mendix >= 10.18) stores documents as individual files in an `mprcontents/` folder, making git diff feasible.

**Usage:**

```bash
# Compare against HEAD (latest commit)
mxcli diff-local -p app.mpr --ref HEAD

# Compare against a specific commit
mxcli diff-local -p app.mpr --ref HEAD~1

# Compare against a branch
mxcli diff-local -p app.mpr --ref main

# Compare two arbitrary revisions (git range syntax)
mxcli diff-local -p app.mpr --ref main..feature-branch

# Three-dot range (changes since common ancestor)
mxcli diff-local -p app.mpr --ref main...feature-branch
```

### MPR v2 Requirement

`diff-local` only works with MPR v2 format (Mendix >= 10.18), where documents are stored as individual files. MPR v1 projects store everything in a single SQLite database, making file-level git diff impractical.

## Workflow

### Review Before Applying

```bash
# 1. Generate MDL changes
# (AI assistant creates changes.mdl)

# 2. Review what would change
mxcli diff -p app.mpr changes.mdl

# 3. If satisfied, apply
mxcli exec changes.mdl -p app.mpr
```

### Track Changes Over Time

```bash
# After making changes, see what changed since last commit
mxcli diff-local -p app.mpr --ref HEAD

# See changes since two commits ago
mxcli diff-local -p app.mpr --ref HEAD~2
```

### Compare Branches

```bash
# What changed between main and your feature branch
mxcli diff-local -p app.mpr --ref main..feature-branch

# Feed diff into an LLM for review
mxcli diff-local -p app.mpr --ref main..feature-branch > changes.diff
```
