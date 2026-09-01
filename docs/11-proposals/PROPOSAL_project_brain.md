---
title: Project brain — an opt-in store, in a user's Mendix project, for what mxcli cannot compute
status: draft
date: 2026-09-01
related:
  - .claude/skills/fix-issue.md
  - .claude/skills/maintain-wiki.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - PROPOSAL_ai_capability_dataset.md
---

# Project brain — an opt-in store, in a user's Mendix project, for what mxcli cannot compute

> Written in response to mendixlabs/mxcli#1017. The issue asks that the brief's
> assumptions be verified against the codebase and that conflicts be flagged
> rather than followed. §2 does that; four of the four assumptions needed
> amending, and one of them is wrong outright.

## 1. Problem

**Audience: users of mxcli building Mendix projects** — not mxcli's own
development. The store lives in the user's Mendix project, is maintained by that
developer and their agent, and is read by people who may never see mxcli's
source. This matters throughout: it sets the scale (tens of lines, not
hundreds of entries), the number of writers (one developer, not many parallel
sessions), and the review audience (a Mendix developer reading a pull request).

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

### 2.1 Can MDL read and write `Documentation`? — Yes, via doc comments, and it is the supported spelling

A `/** … */` comment before a statement becomes the object's `Documentation` and
is written into the model. Measured on Mendix 11.13, executing against a real
project and then searching the **stored bytes**, not a read-back:

```
create microflow … with a /** … */ header
  -> text present in mprcontents/d4/a4/….mxunit
create entity … with a /** … */ header
  -> text present in mprcontents/84/77/….mxunit
describe microflow …
  -> the comment comes back verbatim, above `create or modify microflow`
```

`create … comment 'text'` was removed **because this exists**, not because the
capability was missing — two spellings, one of which wrote nothing, is worse
than one. Reading the removal as evidence of a gap is the wrong inference, and
the earlier draft of this proposal made it.

The doc comment is wired at **28 sites** across `mdl/visitor/`, covering entity,
microflow, page, association, enumeration, workflow, scheduled event, queue,
regular expression, JSON structure, image collection, OData, REST, business
events and the agent-editor documents.

**But a rewrite destroys it.** Writing works; *surviving* does not. Measured on
the same project, checking the stored bytes after each step, with the untouched
object as the control:

```
                     microflow doc      entity doc
after create           PRESENT            PRESENT
after `create or replace` the microflow
                       ABSENT             PRESENT     <- control holds
after `create or modify` the entity
                       ABSENT             ABSENT
```

Each rewrite destroys **its own** object's documentation and leaves the other
alone. So a statement that says nothing about documentation — adding an
attribute, changing a flow — silently deletes whatever was promoted there.
`mx check` is clean throughout: a document with no documentation is valid.

This is the guard-don't-drop class, and it is **fatal to tier 1 as the brief
describes it**. "Knowledge attached to the object travels with the object and is
deleted with it" is true, and the unstated half is that it is also deleted by an
ordinary edit that has nothing to do with the knowledge. An agent that promotes a
decision into a microflow's documentation and later adds a parameter has thrown
the decision away, with every signal reporting success.

**Consequence for the design.** Tier 1 is the strongest idea in the brief and is
**blocked** until rewrites preserve documentation the statement does not restate.
That is a fix in mxcli's writers, not in the brain, and it should be a
precondition of phase 3 rather than a task inside it. Until then the brain's
preference order starts at tier 2.

A caveat on this measurement: it was made with a corrected test. The first
version chained `&& echo SURVIVED` to `head -1`, which exits 0 on empty input, so
it reported success regardless of what grep found — the same shape as the test
failures catalogued in §3.

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

## 3. Evidence from an analogous store — with the differences stated

mxcli maintains a store of a similar shape: the bug findings under
`.claude/skills/fix-issue/findings/`, digested into `docs-wiki/bug-patterns/`.

**It is not this feature and the audience is different.** That store is for
developing *mxcli itself* — a Go repository, many parallel agent sessions,
hundreds of entries accumulated over months, read by people working on the tool.
The brain proposed here is for a *user's Mendix project*: one developer and their
agent, a few dozen lines, read by someone who may never see mxcli's source. The
scale differs by two orders of magnitude and the number of concurrent writers by
more.

So this is an analogy, not a precedent. Three of its failures transfer, one does
not, and saying which is the point of including it.

**Transfers — unbounded growth destroys the artifact.** The findings began as a
table inside a skill file and reached 1.05 MB across 630 rows: past a context
window, past what GitHub's web editor will open. The instruction to read it
before diagnosing was unfollowable for months and nobody noticed, because an
unread file has no failure mode. This transfers *more* strongly here, not less:
a project brain is loaded into an agent's context every session, so its size is a
recurring tax rather than an occasional one. **The brief's caps are the single
most important thing in it**, and `promote` refusing when a cap would be exceeded
is the right enforcement point.

**Transfers — a curation step with no trigger stops.** Three digest pages were
written on one day and none was re-synced for three months while the corpus grew.
The step was on-demand and nothing demanded it. The brain has the same shape:
`staged.jsonl` fills automatically and `promote` is manual. What eventually
worked in the analogous case was printing the gap from a command that already
runs, rather than adding a report someone must remember to invoke.

**Transfers — self-reported claims go stale.** A README and a PR body both said a
check ran in CI when it did not; coverage figures written into prose were stale
within days. Anything the brain says about itself — its size, its staleness —
should be computed by `brain show` / `brain check`, never written into a
committed file.

**Does not transfer — silent duplicates from union merges.** `merge=union` let
256 duplicate findings reach the shared corpus unnoticed. That is a
many-parallel-writers problem; a single developer on one project has little
exposure to it. It justifies a cheap duplicate check on `staged.jsonl` and
nothing more, and the earlier draft over-weighted it.

## 4. Design

Adopt the brief as written, with the following amendments, each traceable to §2
or §3.

| # | Amendment | Because |
|---|---|---|
| A1 | `check` reports three anchor states — resolved, **not found**, **not indexable** — and fails only on the middle one | §2.2: the `objects` view is not a complete inventory |
| A2 | Anchors resolve at document *and* member granularity (`objects` + `attributes_data`) | §2.2: `@Mod.Entity.Attr` is the natural thing to write |
| A3 | Generated lint rules use a `brain_` filename prefix and a `BRAIN###` ID namespace | §2.3: `mxcli init` writes into the same directory |
| A4 | **Tier 1 is blocked** until a rewrite preserves documentation it does not restate. Until then the preference order starts at tier 2 | §2.1: measured — `create or replace` destroys the doc comment, `mx check` clean |
| A5 | `staged.jsonl` gets a cheap duplicate check | §3: cheap insurance, but a many-writers problem that mostly does not apply here |
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
4. **Who fixes documentation preservation?** §2.1 measures that `create or
   replace` / `create or modify` destroys an object's doc comment. That is an
   mxcli writer defect independent of this feature and worth its own issue; the
   brain merely cannot use tier 1 until it is fixed.
