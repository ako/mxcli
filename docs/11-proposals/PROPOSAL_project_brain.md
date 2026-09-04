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

**Consequence for the design — since fixed.** Tier 1 is the strongest idea in the
brief and was **blocked** until rewrites preserved documentation the statement
does not restate. That was a fix in mxcli's writers, not in the brain. It has
landed: every rewrite path now carries the stored value when the statement is
silent, the way it already carried folder, allowed module roles and element
identity, and a fixture asserts survival for **all 29 rewrite-capable document
types** (`documentation_preserved_test.go`, with an untouched-object control and
an empty-comment-clears case). A source-scanning guard fails when a new
rewrite-capable type appears without one. Tier 1 is therefore a phase-3 task
rather than a precondition of it.

Two mistakes made on the way there are worth carrying into the brain's own
tests. A control that does not *compile* is not a control — deleting the carry
block failed on unused variables instead of failing the test, so the condition
had to be stubbed to `if false` instead. And a type counted as done because the
carry was written, while the statement had a second update path the fixture never
exercised; "done" had to be redefined as **carried and covered**.

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
| A4 | ~~Tier 1 is blocked~~ **Resolved.** Rewrites now carry documentation the statement does not restate, on all 29 rewrite-capable document types | §2.1: was measured broken, now fixture-covered with a control |
| A5 | `staged.jsonl` gets a cheap duplicate check | §3: cheap insurance, but a many-writers problem that mostly does not apply here |
| A6 | `brain show`'s size figure and any coverage number are computed, never written into a committed file | §3: prose figures went stale within days |
| A7 | The gap that motivates curation is printed by a command that already runs, not only by `brain check` | §3: the on-demand digest went three months without a run |
| A8 | Records shard by **anchor scope** — `project.md` for cross-cutting, `modules/<Module>.md` for anchored ones — created on demand, with caps applying per shard | §4.1: one file makes a single project-wide budget, and every session pays for every module |
| A9 | The store lives at **`docs/brain/`**, with a `README.md` written by `init` | §4.2: a bare `docs/` gives no signal that the files are brain-managed, and §6 Q2 |

Everything else — the three tiers, the promote-only-through-a-human rule, the
non-goals — stands as written. The non-goals in particular should be treated as
load-bearing.

The storage layout and the CLI surface no longer stand as written: A8 and A9
change the first, and the second has to follow it. §4.1 and §4.2 give the
layout; §4.3 restates the surface against it and §4.4 the skill. Both are
restated in full rather than by reference, because a reader of this proposal
cannot see the brief.


### 4.1 Storage layout at scale

The brief's single decisions file assumes a project whose decisions fit one
budget. Mendix projects routinely run to 100+ modules, and one file has two
failure modes there — both of which the caps make *worse* rather than better:

- **The cap becomes a project-wide budget.** Recording a `Sales` decision
  competes with a `Finance` decision for the same allowance, so `promote` starts
  refusing on exactly the projects that most need the store.
- **Every session pays for every module.** The store is loaded into context each
  session; an agent working in one module carries the other ninety-nine.

Shard by **anchor scope**, which is derived rather than chosen:

```
docs/brain/
  README.md            written by `brain init` — what the folder is, how it is checked
  project.md           cross-cutting, no anchor: always loaded, the tightest cap
  modules/<Module>.md  anchored to `Module.*`: loaded when that module is in play
```

Three properties follow, and they are the reason to shard on this key rather
than by topic or by date:

1. **There is no index to maintain.** An anchor is `@Sales.Order.Status`; its
   module prefix *is* its file name. Routing is a string split — and A6 already
   forbids a written index, which would be a self-reported claim that goes stale.
2. **Misfiling becomes a check rather than a style note.** The `objects` view
   carries a `ModuleName` column (`mdl/catalog/tables.go:1028`), so `check` can
   assert that every anchor in `modules/Sales.md` resolves with
   `ModuleName = 'Sales'`. This check cannot exist for a single file: there is
   nothing for an entry to be inconsistent *with*.
3. **`check` can scope to a diff.** `--changed` reads git's changed-file list and
   validates only the affected shards — the cheap half of the answer to §6 Q1.

**Shards are created on demand, and their universe is far smaller than the module
count.** Marketplace modules do not carry decisions: Mendix's own guidance is not
to edit them, and mxcli already refuses to write a layout into one. The catalog
distinguishes them (`modules.Source`), so the universe is computable rather than
guessed. Measured on the sample project:

```
9 modules, 7 of them Marketplace (Atlas_Core, Administration, NanoflowCommons, …)
  -> 2 modules could ever own a shard
```

This is emphatically not the "one file per record" shape that turned the
analogous store into 600 files (§3). A shard holds many entries and is keyed by
something the project already has a name for; the file count tracks *modules the
team owns*, not decisions.

**Phase 2 introduces a third key.** An mxbuild error → resolution record is
anchored to a CE number, not to a module, and belongs in its own `errors.md`
rather than being forced into one of the other two. Naming the axis now —
*anchored to a module, anchored to an error, anchored to nothing* — is cheaper
than discovering it once entries exist.

### 4.2 Where the store lives

`docs/brain/`, not `docs/`.

The brief's argument for `docs/` is reviewability in a pull request. That is
right and is preserved unchanged. What a bare `docs/` does not give is any signal
that the files are brain-managed, and it walks straight into §6 Q2: a Mendix
project's `docs/` may already be the customer's, or Studio Pro's.

A clearly-named subfolder answers both at once. Dropping a `decisions.md` into
someone else's docs tree is a collision; adding a labelled folder beside their
files is not.

Two alternatives, and why not:

- **`brain/` at the repository root.** More discoverable, but a Mendix project
  root is already crowded (`mprcontents/`, `theme/`, `themesource/`,
  `javascriptsource/`, `resources/`, `deployment/`, `.mxcli/`, `.claude/`,
  `.ai-context/`), and discoverability is the skill's job — its `description` is
  what decides whether the store is ever consulted at all (§2.4).
- **An `mxcli-` prefix, as in `theme/mxcli-themes/`.** That prefix exists where
  mxcli *generates* files and must not clobber the user's. Brain entries are
  written by the developer and promoted through a human, so the prefix would
  signal tool ownership — the opposite of the intent. The `README.md` carries the
  ownership statement instead; because it describes mechanism rather than state,
  A6 still holds.

### 4.3 CLI surface, restated against the sharded layout

Seven verbs under `mxcli brain`. Sharding changes six of them and deliberately
leaves `capture` alone, so the surface is given in full rather than deferred to
the brief.

| Command | Behaviour under A8/A9 |
|---|---|
| `brain init` | Creates `docs/brain/` with `README.md` and an empty `project.md`. `modules/` is created empty; shards appear on promotion. Adopts an existing `docs/` rather than claiming it, and **refuses** a `docs/brain/` it did not write (§6 Q2's residue) |
| `brain capture <text> [@anchor…]` | Appends to `.mxcli/brain/staged.jsonl`. **Not sharded** — staging is a queue, not a store, and it is gitignored (§2.4). Sharding a queue buys nothing and costs a routing decision made before a human has looked at the entry |
| `brain staged` | Lists the queue with the shard each entry *would* land in, so the routing is visible before it happens |
| `brain promote <id> [--to <shard>]` | The only writer of a committed file, and still human-invoked. Destination is **derived**: the module of the entry's first anchor, or `project.md` when it has none. `--to project` is the escape hatch for a fact that is cross-cutting despite carrying an anchor |
| `brain drop <id>` | Removes an entry from the queue or from its shard, and **deletes a shard that becomes empty** — otherwise the directory accumulates husks that read as "this module has decisions" |
| `brain check [--changed] [--ci]` | Per-shard. Reports A1's three anchor states, and **separately** whether an entry is misfiled — a second axis, not a fourth state: an anchor can resolve perfectly and still sit in the wrong shard. `--changed` reads git's changed-file list and checks only the affected shards |
| `brain show [<shard>]` | Per-shard size and cap headroom, **computed on every run** (A6). No shard's figure is ever written into a committed file, including the `README.md` |

Two rules the table compresses:

**The cap is per shard, and `promote` is where it bites.** A promotion that would
push its destination past the cap is refused, naming the shard and its current
occupancy. `project.md` carries the tightest cap of any shard, because it is the
only file loaded unconditionally.

**Misfiling is a check, with one deliberate relaxation.** `check` requires that
**at least one** of an entry's anchors resolves with `ModuleName` equal to its
shard. Additional anchors into other modules are *reported, not failed* — a fact
like "`Sales.Order` is committed by `Finance.ACT_Post`" is genuinely two-module,
and forcing it into `project.md` would grow the one file that must stay small.
An entry with **zero** anchors in its own shard is misfiled, and that is the
failure the check exists for.

**A7's host is `mxcli lint`, not `mxcli check`.** The staged-entry count and the
number of shards whose anchors no longer resolve have to surface from something
that already runs; `brain check` alone is a report nothing demands, which is how
the analogous store's curation step went three months without a run (§3).
`check` is the wrong host despite being the more frequently run of the two: it is
scoped to an **MDL script**, so a project-level staleness line has no business in
its output. `lint` already takes `-p app.mpr`, already runs in review, and is
already where the brain has a presence — A3's generated rules surface there.

### 4.4 The skill

Ship it at `.claude/skills/mendix/project-brain/SKILL.md`. The embed
(`//go:embed all:skills`) and `mxcli init` handle distribution, and the source of
truth is `.claude/skills/mendix/` — never the embed directory, which
`make sync-skills` rebuilds with `rsync --delete` (§2.4).

The `description` is the routing mechanism, so it is phrased around symptoms
rather than around the feature:

```yaml
---
name: project-brain
description: "Project-specific knowledge mxcli cannot compute — why a pattern was
  chosen here, which marketplace version broke what, what a recurring mxbuild error
  means in this app. Use before designing something that looks like it was decided
  before, and when an mxbuild error is resolved by something non-obvious."
---
```

Sharding gives the skill four instructions it would not otherwise need, and the
first is the one that matters:

1. **Read `project.md`, plus the shard for each module you are about to touch.**
   That set is known before the work starts. **Never read the whole directory** —
   the analogous store's failure was a file too large to read, and the sharded
   equivalent is an agent that reads every shard and reinstates the cost the
   sharding removed.
2. **Ask whether `mxcli` can answer it before writing anything down.** Entities,
   microflows, pages, bindings and references are all queryable; a store that
   transcribes them is a store that will disagree with the project (§1).
3. **`capture` during work; never `promote`.** Promotion is the human's step, and
   the skill should say so rather than leaving an agent to infer it from the
   command's absence in its instructions.
4. **Write the anchor, not the name.** `@Sales.Order.Status` is what makes an
   entry checkable and routable; the same fact written as prose is neither.

### 4.5 Two more record kinds, and the property that separates them

The brief and §4.1–4.4 describe one kind of record: a **decision**. Two more are
needed, and the reason is a single property rather than three separate
arguments — **what a failed anchor means.**

| Kind | Its anchor points | An anchor that does not resolve means | Checked? |
|---|---|---|---|
| decision | backward, at what exists | the decision is **stale** | yes — fails |
| requirement | forward, at what is intended | **not built yet** | counted, never fails |
| open question | at what is *under discussion* | nothing — the question is often whether it should exist | not at all |

The syntax is identical in all three. Only the direction differs, and that is
the whole lifecycle.

**This was measured before it was designed**, which is what settled it in one
command: recorded as an ordinary entry, a single not-yet-built requirement takes
`brain check` to exit 1. The same is true of a question. Requirements and
questions could not be more decisions without making the check useless.

**Requirements** (`plan/<slice>.md`, `capture --slice`) exist because the source
of truth is frequently outside git — a specification document, a prototype, a
conversation. None of that is an issue or a commit message, so hours of work can
end with nothing recording what they were for, and a resumed session has no idea
what it was building towards. This is the gap the analogous store never had,
because mxcli's own work is driven by GitHub issues.

The inversion then pays for the feature rather than merely accommodating it: a
requirement is **built** when its anchors resolve, so `brain plan` reports
progress *derived from the model*. Measured end to end — a slice at 0 built /
1 planned became 1 / 0 after creating the microflow its requirement named, with
the plan file untouched. There is no status column to maintain and none that can
be silently wrong, which is A6 applied to scope rather than to size. It is also
where this design departs from the AI-native SDLC playbook, which keeps a
`plan.md` and recommends a hook to enforce that the diff still matches it: that
is a self-reported artifact being policed, and it is only necessary when there is
no queryable model. There is one here.

**Open questions** (`--open`, `brain resolve`) are decisions not yet made. They
live beside the decisions they will join rather than in a file of their own,
because the moment you need to see one is while reading what that module already
decided. Resolution converts the entry **in place**, keeping its id and position
— an answered question is the same piece of knowledge as the question — and from
then on its anchors are checked like any other decision.

Two consequences worth stating, because both are the kind of thing that looks
like an oversight:

- **A question filed against a slice is not scope.** It is counted apart from
  the slice's requirements; counting an unanswered question as outstanding work
  would overstate what is left to do.
- **Requirements and questions are never misfiled.** A slice spans modules by
  design, and a question has no resolved anchor to compare a shard against.

Two amendments follow, in the table's numbering:

| # | Amendment | Because |
|---|---|---|
| A10 | Three record kinds, distinguished by anchor direction. Requirements live in `plan/<slice>.md`; questions live beside decisions and carry an `OPEN` marker | §4.5: measured — a requirement or question recorded as a decision takes `check` to exit 1 |
| A11 | Capture gets a **trigger**: a correction made twice, or a choice between real alternatives. `mxcli lint` reports unpromoted entries *and* unanswered questions | §4.5: the plan half fills at bootstrap while the decisions half fills only if someone remembers — A7 applied to the imbalance |

A11's trigger is taken from the AI-native SDLC playbook's rule for `CLAUDE.md`
("when Claude makes a mistake twice, the correction goes into `CLAUDE.md`"),
which is the one piece of that document this design was missing outright.

Its other contribution was to expose a defect: the playbook is explicit that
`CLAUDE.md` is read at the **start of a session**, and this proposal asserted the
same of `project.md` — using it to justify the tightest cap in the store — with
no mechanism behind it. A generated project mentioned the brain exactly once, in
a skills-table row, behind a symptom-triggered description. The generated
`CLAUDE.md` now names `docs/brain/project.md` directly. **A policy about what is
loaded is worth nothing without the thing that loads it**, and the same question
should be asked of any future claim of that shape here.

## 5. Phasing

Unchanged from the brief, with A4 inserted:

1. Storage, anchors, and the verbs of §4.3 — `init` / `capture` / `staged` /
   `promote` / `drop` / `check` / `show`, plus `plan` and `resolve` for the two
   further record kinds of §4.5 — and the skill of §4.4. Markdown destinations
   only.
2. The mxbuild error → resolution trigger.
3. **Documentation audit**, then promotion into model documentation and lint-rule
   generation. No longer gated on A4.

## 6. Open questions

1. **What does `brain check --ci` cost on a cold clone?** It needs a catalog, and
   catalog validity is an mtime comparison that a fresh checkout always fails.
   Unmeasured. If a full build is expensive, `--ci` may need a cheaper anchor
   index than the catalog. A8 narrows the question but does not close it:
   `--changed` checks only the shards a diff touches, yet the *first* of those
   still pays for the catalog build.
2. ~~**Is `docs/` the right home?**~~ **Answered — `docs/brain/` (A9, §4.2).** A
   labelled subfolder is safe to add to a `docs/` that is Studio Pro's or a
   customer's, which a bare `decisions.md` is not, and the folder name is what
   tells a reviewer the files are brain-managed. `init`'s adoption step still has
   to handle an existing `docs/brain/` that is *not* ours, which is now the only
   collision left.
3. **THEORY.md does not exist.** The issue says to read it and to update it if the
   working theory changes. There is no such file anywhere in the repository. Is it
   expected to be created, or was another document meant?
4. ~~**Documentation preservation is filed as `mendixlabs/mxcli#1018`.**~~
   **Fixed** — all 29 rewrite-capable document types carry documentation the
   statement does not restate, each with a fixture. The shape of the fix was the
   one predicted here: carry the stored value the way the rewrite paths already
   carry folder, allowed module roles and element identity. Upstream #1018 stays
   open until the fork syncs.
