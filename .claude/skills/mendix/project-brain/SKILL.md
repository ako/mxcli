---
name: project-brain
description: "Project-specific knowledge mxcli cannot compute — why a pattern was chosen here, which marketplace version broke what, what a recurring mxbuild error means in this app. Use before designing something that looks like it was decided before, and when an mxbuild error is resolved by something non-obvious."
---

# Project brain

The brain holds what mxcli **cannot** compute about this project: why a pattern
was chosen here, which marketplace version broke what, what a recurring mxbuild
error means in *this* app.

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
```

**Read `project.md`, plus the shard for each module you are about to touch.**
That set is known before the work starts.

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
| `mxcli brain check [--changed]` | Anchors still resolve, entries in the right shard |
| `mxcli brain show [<shard>]` | Entries, lines and headroom per shard |

## What not to record

- Anything `show`, `describe` or the catalog answers — it will drift.
- Counts and sizes of anything, including the brain itself.
- Task state ("next we should…"). This is a store of what is true, not a to-do.
- A restatement of Mendix documentation. Record what is true *here*.
