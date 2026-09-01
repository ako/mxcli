---
title: Project brain — an opt-in store for what mxcli cannot compute
status: draft
date: 2026-09-01
related:
  - .claude/skills/fix-issue.md
  - .claude/skills/maintain-wiki.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - PROPOSAL_ai_capability_dataset.md
---

# Project brain — an opt-in store for what mxcli cannot compute

> Written in response to mendixlabs/mxcli#1017. The issue asks that the brief's
> assumptions be verified against the codebase and that conflicts be flagged
> rather than followed. §2 does that; four of the four assumptions needed
> amending, and one of them is wrong outright.

## 1. Problem

An agent working on a Mendix project accumulates knowledge it loses each
session: why a pattern was chosen, which marketplace version broke what, which
mxbuild error means what *here*. The usual answers — a hand-maintained
`CLAUDE.md`, a memory-bank tool — rot, and duplicate what mxcli can already
answer.

The governing principle from the brief is right and is what makes this
Mendix-specific rather than another memory tool: **store only the negative
space.** Anything derivable from the model must be answered by a command and
never written down. mxcli can already query entities, microflows, pages,
bindings and references; a store that transcribes any of that is a store that
will disagree with the project.

## 2. Assumptions verified

### 2.1 Can MDL read and write `Documentation`? — Partly, and unevenly

Yes for domain-model objects, through two spellings: a `/** … */` doc comment
before a statement (`findDocCommentText`, wired per-statement in
`mdl/visitor/`), and `ALTER ENTITY … SET DOCUMENTATION` / `SET COMMENT`
(`MDLDomainModel.g4:214`).

But the surface is uneven, and there is a recorded finding about exactly this:
`create … comment 'text'` was accepted and **wrote nothing** on `create entity`,
`enumeration`, `module`, `microflow`, `nanoflow`, `rule` and `association`. The
dead option was removed rather than wired, on the grounds that two spellings —
one of which lies — are worse than one. `COMMENT` remains live on constants,
JSON structures, image collections, database connections and workflows.

**Consequence for the design.** Tier 1 ("attach knowledge to the object it
concerns") is the right preference and is *not* uniformly available today. Phase
3 must open with a per-doctype audit of what can actually carry documentation
and round-trip it, and the proposal should not assume a microflow can be
annotated from MDL until that audit says so.

### 2.2 What does the catalog give us, and how fast? — Everything needed, and it is free

The `objects` view (`mdl/catalog/tables.go:1028`) unions **43 document types**
with a `QualifiedName` column — module, entity, association, microflow,
nanoflow, rule, page, snippet, layout, workflow, and so on. Resolving
`Sales.ACT_Order_Approve` is one equality query.

Measured against a real project's catalog (`/home/vscode/ord`, 382 objects,
1.6 MB SQLite):

```
resolve one anchor: 0.038 ms   (mean of 1000, no index on QualifiedName)
```

A hundred anchors is under 4 ms. **Speed is a non-issue and should not shape the
design.**

Two caveats that do:

- **The `objects` view indexes only describable types.** A recorded finding says
  it in as many words — "do not measure coverage from the catalog… enumerate raw
  unit `$Type`s instead". An anchor to a document type outside the view resolves
  as *missing*, which is a false staleness signal. `check` must distinguish
  "resolved", "not found", and **"cannot be resolved by this index"**, and only
  the middle one is a failure.
- **Member-level anchors need a second query.** `@Sales.Order` resolves through
  `objects`; `@Sales.Order.Status` does not — attributes live in
  `attributes_data` (`EntityQualifiedName` + `Name`, 254 rows in the sample
  project). Support both or document that anchors are document-scoped; do not
  let an attribute anchor silently fail.

**Staleness of the catalog itself** is an `.mpr` **mtime** comparison
(`cmd_catalog.go:296`). `brain check --ci` on a fresh clone gets fresh mtimes, so
CI rebuilds the catalog every run. That cost is unmeasured here and needs a
number before `--ci` is promised.

### 2.3 Does the Starlark engine support generated rules? — The question does not arise

Rules are **discovered from files**: `FindLintRulesDir` walks up for
`.claude/lint-rules/`, and every `*.star` in it is loaded. There is no compiled-in
registry to extend, so "a rule generated at runtime from a template" is just **a
generated file**. No engine work is needed.

One hazard the brief does not mention: `mxcli init` writes the bundled rules into
that same directory with `os.WriteFile` per file (`init.go:351`). It does **not**
wipe the directory, so a generated rule survives an upgrade — *unless its
filename collides with a shipped one*. Generated rules therefore need a reserved
prefix (`brain_*.star`) and a rule-ID namespace outside the shipped `ARCH` /
`CONV` / `QUAL` / `SEC` / `MDL` sets.

### 2.4 Release and skill mechanics — the brief's premise is wrong

**There is no goreleaser.** No `.goreleaser.yml` exists. Releases run
`make release` from `.github/workflows/release.yml`.

Skills ship by embedding: `//go:embed all:skills` over `cmd/mxcli/skills/`
(`skills_content.go:26`), which `make sync-skills` mirrors from
`.claude/skills/mendix/` with **`rsync --delete`**. So shipping a skill means
adding `.claude/skills/mendix/<name>/SKILL.md` and nothing else — and editing the
embed directory directly is always wrong, because the next sync deletes it. The
`all:` prefix is load-bearing (a plain `go:embed` skips `_`-prefixed files).

`.mxcli/` **is** gitignored by `mxcli init` (`constant_gitignore.go`), so the
brief's split between committed docs and tool-owned state holds as written.

## 3. Evidence from mxcli's own attempt at this

mxcli already runs a store of exactly this shape — the bug findings under
`.claude/skills/fix-issue/findings/`, digested into `docs-wiki/bug-patterns/`.
It has been running for months and has failed in four ways that this design
should be built against, because every one of them is reachable from the brief
as written.

**It grew until it could not be read.** The findings began as a Markdown table
inside a skill file and reached **1.05 MB across 630 rows** — past a context
window, past what GitHub's web editor will open, and past what the digest step
could consume. The instruction to "read it before diagnosing" was unfollowable
for months and nobody noticed, because an unread file has no failure mode.
*→ The brief's caps are the single most important thing in it. Keep them, and
enforce them in `promote` as it says.*

**The digest nothing triggered stopped.** Three pattern pages were written on one
day in May and none was re-synced for three months while the corpus grew 200×.
The sync was on-demand and no step demanded it.
*→ A trigger has to fire where work already happens. What eventually worked was
printing the gap from a command that already runs on every fix, not adding a
report someone must remember.*

**Append-only plus union merge produced silent duplicates.** `merge=union` was
added so parallel fixes would not conflict. It cannot distinguish a genuine
parallel append from a re-append of the same content, and nothing compared lines:
`main` currently holds **885 records of which 629 are distinct** — the executor
shard is essentially the same 247 findings twice.
*→ `staged.jsonl` and the committed `decisions.md` are both append-only stores
with the same exposure. Whatever check they get must compare entries, not just
validate each one.*

**Claims about the mechanism went stale, including "it runs in CI".** A README
and a PR body both said the findings check ran in CI. It did not. Coverage
percentages written into prose were stale within days.
*→ Anything the brain reports about itself should be computed by a command, not
written into a file.*

None of this argues against the feature. It argues that the parts of the brief
that look like restraint — caps, no auto-promotion, human-in-the-loop `promote` —
are the parts that carry the design, and the parts that look like plumbing are
where it will fail.

## 4. Design

Adopt the brief as written, with the following amendments, each traceable to §2
or §3.

| # | Amendment | Because |
|---|---|---|
| A1 | `check` reports three anchor states — resolved, **not found**, **not indexable** — and fails only on the middle one | §2.2: the `objects` view is not a complete inventory |
| A2 | Anchors resolve at document *and* member granularity (`objects` + `attributes_data`) | §2.2: `@Mod.Entity.Attr` is the natural thing to write |
| A3 | Generated lint rules use a `brain_` filename prefix and a `BRAIN###` ID namespace | §2.3: `mxcli init` writes into the same directory |
| A4 | Phase 3 opens with a per-doctype documentation audit; no promotion to model documentation before it | §2.1: `create … comment` was a dead option on seven doctypes |
| A5 | `staged.jsonl` and every committed store get a **duplicate check**, not only a shape check | §3: 256 duplicate findings reached `main` unnoticed |
| A6 | `brain show`'s size figure and any coverage number are computed, never written into a committed file | §3: prose figures went stale within days |
| A7 | The gap that motivates curation is printed by a command that already runs, not only by `brain check` | §3: the on-demand digest went three months without a run |

Everything else — the three tiers, the storage layout, the CLI surface, the
promote-only-through-a-human rule, the non-goals — stands as written. The
non-goals in particular should be treated as load-bearing.

**The skill.** Ship it under `.claude/skills/mendix/project-brain/SKILL.md`; the
embed and `mxcli init` handle the rest (§2.4). The brief is right that the
description decides whether it is ever used and should be phrased around
symptoms. Worth adding: mxcli's own skills are synced with `rsync --delete`, so
the source of truth is `.claude/skills/mendix/`, never the embed directory.

## 5. Phasing

Unchanged from the brief, with A4 inserted:

1. Storage, anchors, `init` / `capture` / `staged` / `promote` / `drop` /
   `check` / `show`, plus the skill. Markdown destinations only.
2. The mxbuild error → resolution trigger.
3. **Documentation audit**, then promotion into model documentation and lint-rule
   generation.

## 6. Open questions

1. **What does `brain check --ci` cost on a cold clone?** It needs a catalog, and
   catalog validity is an mtime comparison that a fresh checkout always fails.
   Unmeasured. If a full build is expensive, `--ci` may need a cheaper anchor
   index than the catalog.
2. **Is `docs/` the right home?** The brief argues for reviewability in PR diffs,
   which is correct. But a Mendix project's `docs/` may already be Studio Pro's
   or a customer's. `init`'s adoption step should cover "there is a `docs/` and it
   is not ours".
3. **THEORY.md does not exist.** The issue says to read it and to update it if the
   working theory changes. There is no such file anywhere in the repository. Is it
   expected to be created, or was another document meant?
