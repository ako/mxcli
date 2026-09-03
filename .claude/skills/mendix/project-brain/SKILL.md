---
name: project-brain
description: "Project-specific knowledge mxcli cannot compute — the requirements and slices being built from (a spec, a prototype, a conversation), why a pattern was chosen here, which marketplace version broke what. Use when starting from requirements that live outside git, before designing something that looks like it was decided before, and when an mxbuild error is resolved by something non-obvious."
---

# Project brain

The brain holds what mxcli **cannot** compute about this project. Two halves:

- **Decisions** — why a pattern was chosen here, which marketplace version broke
  what, what a recurring mxbuild error means in *this* app.
- **The plan** — the requirements being built from and the slices they are
  grouped into, when the source is a specification document, a prototype or a
  conversation rather than GitHub issues.

The plan half matters because hours of work can otherwise leave no trace: a
Word document and a chat transcript are not in git, so a session that resumes
later has no idea what it was building towards, and neither does the next
person.

It lives in `docs/brain/`, is committed, and is reviewed in a pull request like
any other change.

## The rule that makes it work

**Anything mxcli can answer does not belong here.** Entities, microflows, pages,
bindings, references, callers, dead assets — all queryable. A note that
transcribes any of them is a note that will disagree with the project the moment
someone edits the model, and it will disagree silently.

Before writing anything down, ask whether a command answers it:

```bash
mxcli -p app.mpr -c "show entities"
mxcli -p app.mpr -c "show callers of MyModule.ACT_Thing"
mxcli -p app.mpr -c "describe microflow MyModule.ACT_Thing"
```

If one does, do not record it. Record only the **negative space** — the reason,
the constraint, the history that no query can reach.

## Reading it

```
docs/brain/
  project.md           cross-cutting decisions
  modules/<Module>.md  decisions anchored to one module
  plan/<slice>.md      requirements for one deliverable slice
```

**When building: read `project.md`, plus the shard for each module you are about
to touch.** That set is known before the work starts.

**When planning, or when picking up work: read the plan.** `mxcli brain plan`
first — it says which slices are outstanding — then the shard for the slice you
are working on.

**Never read the whole directory.** A large project has dozens of shards, and
reading them all reinstates exactly the context cost the split removed. If you
do not know which modules you are touching yet, read `project.md` and come back.

## Writing to it

An agent **captures**; a person **promotes**. Capturing is free and reversible;
promotion is the human's call about what is worth committing.

```bash
mxcli brain capture "Orders are committed by Finance, not Sales" \
  -a @Sales.Order -a @Finance.ACT_Post -p app.mpr
```

The first line becomes the entry's title and the rest becomes its body, so a
one-argument capture can still carry an explanation:

```bash
mxcli brain capture "Marketplace Administration 4.5.0 breaks the login flow
It changes Account's password-policy handling; we pinned 4.3.2 until the
custom login microflow is reworked." -a @Administration.Account -p app.mpr
```

Then leave it. `mxcli lint` reminds the developer that something is staged.

## Recording requirements and slices

When the source of truth is outside git — a specification document, a
prototype, a long conversation — record it as **requirements grouped into
slices** before building. Otherwise the work is invisible: not in an issue, not
in a commit message, and gone from the session that resumes tomorrow.

```bash
mxcli brain capture "Orders must be approvable by a manager" \
  --slice 02-approvals -a @Sales.ACT_Order_Approve -p app.mpr
```

`--slice` is the only signal needed. It files the entry in `plan/02-approvals.md`
and makes it a requirement rather than a decision.

**Slices are ordered by name**, so a numeric prefix is how a roadmap is
sequenced: `01-accounts`, `02-approvals`, `03-reporting`. That is your choice,
not something mxcli maintains.

### A requirement's anchor points forward

This is the difference that matters, and it is why requirements are not simply
more decisions:

| | Anchor points | An anchor that does not resolve means |
|---|---|---|
| decision | backward, at what exists | the decision is **stale** — check fails |
| requirement | forward, at what is intended | **not built yet** — normal, check passes |

So anchor a requirement at what you are *going to* build. `@Sales.ACT_Order_Approve`
before that microflow exists is correct, not a mistake.

### Progress is derived, never written

```bash
mxcli brain plan -p app.mpr
```

```
SLICE           BUILT  PLANNED   UNANCHORED
01-accounts         1        0
02-approvals        0        1            1

1 of 3 requirements built, across 2 slice(s).
```

A requirement is **built** when its anchors resolve against the model. Nothing
in the file says "done" — building the thing is what moves the number.

**Never write a status into a requirement**, and never keep a checklist beside
it. A hand-maintained status is wrong the moment someone builds something, and
nothing will tell you.

A requirement with **no anchor** is counted separately as unanchored: it cannot
be measured. That is a prompt to anchor it once you know what will implement it,
not an error.

### Slices have a generous cap, and that is the slicing discipline

A slice holds source material, so its budget is much larger than a decision
shard's — and it is not loaded every session. But it is still a budget: **a
slice too long to read is a slice that should be split.**

## Write the anchor, not the name

`@Sales.Order.Status` is what makes an entry **routable** (its module decides
the file) and **checkable** (`mxcli brain check` verifies it still resolves).
The same fact written as prose — "the Status attribute on the Sales order
entity" — is neither.

| Anchor | Names |
|---|---|
| `@Sales` | a module |
| `@Sales.Order` | a document: entity, microflow, page, workflow, … |
| `@Sales.Order.Status` | a member: an attribute |

An entry's **first** anchor decides its shard. An entry with no anchor is
cross-cutting and goes to `project.md`.

An entry may anchor into more than one module — "`Sales.Order` is committed by
`Finance.ACT_Post`" genuinely spans two — and that is fine as long as one anchor
belongs to the shard it is filed in.

## Checking it

```bash
mxcli brain check -p app.mpr            # every shard
mxcli brain check --changed -p app.mpr  # only shards this branch touched
```

Two independent things are checked, and only some outcomes are failures:

| Outcome | Meaning | Fails? |
|---|---|---|
| resolved | the anchor names something that is there | no |
| **not found** | the anchor names nothing — the entry is stale | **yes** |
| not indexable | the target exists but its document type is not in the catalog's index | no |
| **misfiled** | no anchor belongs to the shard the entry sits in | **yes** |

"Not indexable" is not a problem to fix. Treating it as missing would demand
edits to entries that are perfectly current.

Misfiling is a **separate axis**, not a fourth anchor state: every anchor can
resolve and the entry still be in the wrong file.

## Size

Each shard has a line budget, and `promote` refuses rather than letting a shard
grow past it. `project.md` is the tightest — it is the only file loaded every
session.

```bash
mxcli brain show -p app.mpr
```

Sizes are computed on every run and deliberately not written down anywhere. A
figure in prose is stale the next time anyone promotes.

If a promotion is refused, the answer is to condense or drop, not to raise the
cap: the cap is what stops the store becoming a file nobody reads.

## Commands

| Command | Does |
|---|---|
| `mxcli brain init -p app.mpr` | Creates `docs/brain/`. Refuses a `docs/brain/` it did not write |
| `mxcli brain capture "<text>" [-a @Anchor]…` | Queues an entry. Never commits |
| `mxcli brain staged` | Lists the queue with the shard each entry would land in |
| `mxcli brain promote <id> [--to <shard>]` | Writes it into its shard. The human step |
| `mxcli brain drop <id>` | Removes it from the queue or from its shard |
| `mxcli brain capture "<text>" --slice <name> [-a @Anchor]…` | Queues a **requirement** of that slice |
| `mxcli brain plan` | The roadmap: each slice's requirements counted against the model |
| `mxcli brain check [--changed]` | Anchors still resolve, entries in the right shard, plus slice progress |
| `mxcli brain show [<shard>]` | Entries, lines and headroom per shard |

## What not to record

- Anything `show`, `describe` or the catalog answers — it will drift.
- Counts and sizes of anything, including the brain itself.
- **Status.** Whether a requirement is done is computed by `mxcli brain plan`.
  A "✅" written beside one is wrong as soon as anyone builds anything.
- Sprint chatter and task assignment. Requirements and their slices, yes; who is
  doing what this week, no — that belongs in an issue tracker.
- A restatement of Mendix documentation. Record what is true *here*.
