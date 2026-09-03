# Project Brain

`mxcli brain` records the project knowledge mxcli **cannot** compute: why a
pattern was chosen here, which marketplace version broke what, what a recurring
mxbuild error means in *this* app.

It is opt-in. A project without `docs/brain/` never hears about it.

## The rule

**Anything mxcli can answer does not belong in the brain.** Entities,
microflows, pages, bindings, references and callers are all queryable. A note
that transcribes any of them will disagree with the project the moment someone
edits the model — and it will disagree silently, because nothing checks prose.

The brain stores only the negative space: the reason, the constraint, the
history that no query can reach.

## Layout

```
docs/brain/
  README.md            what the folder is
  project.md           cross-cutting decisions
  modules/Sales.md     decisions anchored to the Sales module
  modules/Finance.md   …
```

Committed, and reviewed in a pull request like any other change.

The split is not cosmetic. A single file would make the size cap a project-wide
budget — recording a `Sales` decision would compete with a `Finance` one — and
every session would load every module's decisions. With one file per module, a
session loads `project.md` plus the shards for the modules it is touching.

## Anchors

An entry's anchors are what make it routable and checkable.

| Anchor | Names |
|---|---|
| `@Sales` | a module |
| `@Sales.Order` | a document: entity, microflow, page, workflow, … |
| `@Sales.Order.Status` | a member: an attribute |

The **first** anchor decides the file: `@Sales.Order` puts the entry in
`modules/Sales.md`. An entry with no anchor is cross-cutting and goes to
`project.md`.

There is no index to maintain — the module prefix *is* the file name.

## Workflow

An agent captures; a person promotes.

```bash
mxcli brain init -p app.mpr

mxcli brain capture "Orders are committed by Finance, not Sales" \
  -a @Sales.Order -a @Finance.ACT_Post -p app.mpr

mxcli brain staged -p app.mpr      # review the queue
mxcli brain promote a1b2c3 -p app.mpr
```

Captures go to `.mxcli/brain/staged.jsonl`, which `mxcli init` git-ignores — so
nothing reaches a pull request until someone has looked at it. `mxcli lint`
prints a one-line reminder when the queue is not empty.

The queue is deliberately not sharded: routing it would force the file decision
before a human has looked at the entry, which is the decision promotion exists
to make.

## Checking

```bash
mxcli brain check -p app.mpr             # every shard
mxcli brain check --changed -p app.mpr   # only shards this branch touched
```

Two independent things are checked.

**Does each anchor still resolve?** Three outcomes, one of which is a failure:

| Outcome | Meaning | Fails |
|---|---|---|
| resolved | the anchor names something that is there | no |
| **not found** | the anchor names nothing — the entry is stale | **yes** |
| not indexable | the target exists, but its document type is not in the catalog's index | no |

The third state matters. The catalog's `objects` view covers the describable
document types, not all of them. Without this distinction an anchor to, say, a
scheduled event would read as *missing*, and `check` would demand edits to
entries that are perfectly current. mxcli separates the two with a
type-agnostic unit lookup that cannot miss a kind, because it never asks what
kind anything is.

**Is each entry in the right shard?** A separate axis, not a fourth anchor
state: every anchor can resolve and the entry still be in the wrong file. At
least one anchor must belong to the shard the entry sits in; anchors into other
modules are reported but do not fail, because a fact like "`Sales.Order` is
committed by `Finance.ACT_Post`" genuinely spans two modules.

Misfiling is only decided when something resolved. An entry whose anchors are
all *not indexable* is left alone — there is no evidence about where it belongs.

## Size

```bash
mxcli brain show -p app.mpr
```

```
SHARD              ENTRIES    LINES     HEADROOM
project.md               3       28      92/120
Administration.md        2       16     224/240
```

Each shard has a line budget and `promote` refuses rather than exceeding it,
naming the shard and its occupancy. `project.md` is the tightest: it is the only
file loaded every session, so every line in it is charged to every session.

When a promotion is refused the answer is to condense or drop, not to raise the
cap. The cap is what stops the store becoming a file nobody reads.

Sizes are computed on every run and are deliberately not written into any
committed file, including the store's own `README.md` — a figure in prose is
stale the next time anyone promotes.

## Commands

| Command | Does |
|---|---|
| `brain init` | Creates `docs/brain/`. Refuses a `docs/brain/` it did not write |
| `brain capture "<text>" [-a @Anchor]…` | Queues an entry. Never commits |
| `brain staged` | Lists the queue with the shard each entry would land in |
| `brain promote <id> [--to <shard>]` | Writes it into its shard |
| `brain drop <id>` | Removes it from the queue or from its shard |
| `brain check [--changed]` | Anchors resolve, entries filed correctly |
| `brain show [<shard>]` | Entries, lines and headroom per shard |

Dropping the last entry from a module shard deletes the file, so the directory
does not accumulate husks that read as "this module has decisions".
