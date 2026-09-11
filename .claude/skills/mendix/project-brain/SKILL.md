---
name: project-brain
description: "Project-specific knowledge mxcli cannot compute — the requirements and slices being built from (a spec, a prototype, a conversation), why a pattern was chosen here, what is still undecided, which marketplace version broke what. Use when starting from requirements that live outside git, before designing something that looks like it was decided before, when you have had to correct the same thing twice, and when an mxbuild error is resolved by something non-obvious."
---

# Project brain

The brain holds what mxcli **cannot** compute about this project. Two halves:

- **Decisions** — why a pattern was chosen here, which marketplace version broke
  what, what a recurring mxbuild error means in *this* app.
- **Open questions** — what is *not* decided yet, so it is not silently
  forgotten and rediscovered expensively later.
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

**`mxcli brain brief` produces that set for you**, so the rule above does not
depend on judgement:

```
mxcli brain brief --slice 07-planning     # project + the modules that slice's
                                          # requirements anchor into + its plan
mxcli brain brief --module Sales          # project + Sales, no plan
```

The modules are *derived* from the slice's requirement anchors — you do not tell
it which modules the slice touches, because that is what you opened the brief to
find out. The pack goes to stdout and its size to stderr, so it pipes.

This matters most when each slice runs in its own session or sub-agent: the pack
is then re-read from a cold start every slice, and reading the whole store
instead is roughly three times the tokens.

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

**Never anchor a requirement at a bare module.** `@Sales` resolves the instant
the module exists — long before any of the work inside it — so the requirement
reports **built** with nothing done. `mxcli brain capture --requirement` refuses
it and names the alternative, because the failure is silent and flattering:
the plan shows progress that has not happened and nothing else disagrees.
Anchor at a document the slice actually creates. (Measured on a real project:
two requirements anchored at a module both read as built after slice 01.)

A module **role** is a fine anchor and resolves like any document — a security
requirement anchored at the roles it creates is measured correctly. Anchoring at
the documents whose access rules the roles govern works too, and says something
slightly different; either is legitimate.

A **theme** has no model element to anchor at (it is files under `theme/`, which
is the point of `mxcli theme`). Anchor the branding requirement at the branded
**layout** the slice adds — the model-side half of the same work.

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

## When to capture a decision

Capture is easy to postpone forever, so it needs a trigger rather than good
intentions. Two, and the first is the reliable one:

1. **You have had to correct the same thing twice.** The second correction is
   the signal: it will happen a third time to someone else. Capture what the
   right answer is and why, anchored at whatever you were working on.
2. **You chose between real alternatives** and the losing one would look
   reasonable to the next person. Record the choice *and* what ruled the other
   out — a decision without its reason gets re-litigated.

If you are unsure whether something qualifies, capture it. Staging costs
nothing and is reversible; a person decides what is worth committing.

## Recording what is NOT decided

An open question is a decision that has not been made yet. Record it rather
than carrying it in your head — the conversation ends, and the question is
expensive to rediscover.

```bash
mxcli brain capture "Do approvers see rejected orders?
The spec is silent. Affects the overview page and the access rules." \
  --open -a @Sales.Order -p app.mpr
```

A question's **anchors are not checked**. It may name something that does not
exist — often the question is precisely whether it should — so the staleness
rule that keeps decisions honest does not apply to it.

`--open` combines with `--slice`: a question about a slice's scope is filed with
that slice, and is counted apart from its requirements. An unanswered question
is not outstanding scope, so it never inflates the slice.

Answering it turns it into a decision, in place:

```bash
mxcli brain resolve <id> "Yes, for 30 days
Agreed with the product owner; drives the overview filter and the access rule."
```

The entry keeps its id and its position, and the question survives as the
answer's context. From that moment its anchors **are** checked, like any other
decision.

`mxcli brain check` and `mxcli lint` both report unanswered questions until
someone resolves one. That is deliberate: a question nobody answers is the one
kind of entry that gets more expensive the longer it sits.

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
| `mxcli brain staged [--since <id>] [--slice <n>] [--fail-if-empty]` | Lists the queue with the shard each entry would land in. `--since` is the slice boundary — see below |
| `mxcli brain promote <id> [--to <shard>]` | Writes it into its shard. The human step |
| `mxcli brain drop <id>` | Removes it from the queue or from its shard |
| `mxcli brain capture "<text>" --slice <name> [-a @Anchor]…` | Queues a **requirement** of that slice |
| `mxcli brain capture "<text>" --open [-a @Anchor]…` | Queues an **open question**; its anchors are not checked |
| `mxcli brain resolve <id> "<answer>"` | Answers a question, turning it into a decision in place |
| `mxcli brain plan [--slice <name>]` | The roadmap: each slice's requirements counted against the model |
| `mxcli brain brief --slice <name> \| --module <M>` | The reading pack: exactly the shards that work needs |
| `mxcli brain check [--changed]` | Anchors still resolve, entries in the right shard, plus slice progress |
| `mxcli brain show [<shard>]` | Entries, lines and headroom per shard |

## Renaming

`mxcli rename` updates the brain's anchors along with the model's own
cross-references, and says how many it touched. You do not have to fix them by
hand, and `--dry-run` previews the brain's share too.

This is done at the rename because it cannot be done afterwards. A decision's
anchor points backward, so a stale one is reported `NOT FOUND` — but a
requirement's points forward, so a stale one just counts as `PLANNED`, which is
exactly what a forward anchor failing is supposed to mean. Once the old name is
gone there is no way to tell "never built" from "built, then renamed".

If you rename an element some other way — in Studio Pro, or by hand — run
`mxcli brain check` afterwards and expect the plan's counts to have moved.

## What not to record

- Anything `show`, `describe` or the catalog answers — it will drift.
- Counts and sizes of anything, including the brain itself.
- **Status.** Whether a requirement is done is computed by `mxcli brain plan`.
  A "✅" written beside one is wrong as soon as anyone builds anything.
- Sprint chatter and task assignment. Requirements and their slices, yes; who is
  doing what this week, no — that belongs in an issue tracker.
- A restatement of Mendix documentation. Record what is true *here*.

## Handing a slice to another agent

Every command above takes `--json`, so a dispatcher can act on the answers
rather than read them. Two shapes are worth knowing.

**Give the agent its pack.** `mxcli brain brief --slice <name>` is one bounded
read instead of a directory the agent has to navigate.

**Check that it recorded something.** With one agent per slice the brain stops
being a record and becomes the *only* channel between slices — the next agent
has no memory of this one, so a capture that never happened is a decision lost
rather than a note lost. Note the boundary before dispatching and ask afterwards:

```
before=$(mxcli brain staged --json | jq -r .last_id)
# ... the slice's agent runs, and captures ...
mxcli brain staged --since "$before" --fail-if-empty --json
```

`--since` rather than `--slice` is deliberate: `capture --slice` is what makes an
entry a *requirement*, so a decision found while building a slice carries no
slice at all — and a slice's findings are mostly decisions. The queue is
append-only, so its own order is the honest boundary. `last_id` comes back even
when nothing matched, so a slice that recorded nothing still hands the next one
a boundary.
