# Idempotent Writes

Re-running an MDL script against a project that is already in sync leaves the
`.mpr` and `mprcontents/` files **byte-identical**. `git status` stays clean and
Studio Pro shows no version-control changes, because the write does not happen at
all.

This matters beyond tidiness: without it, `git diff` cannot answer "did this
script change anything", two people running the same script commit different
bytes, and a `.mxunit` merge conflict is not resolvable by hand.

## Why a write used to happen anyway

`create or replace` does not reconcile. It rebuilds the document from the MDL and
overwrites, and every sub-element in that rebuild is a new object with a freshly
random `$ID`. The stored bytes were a function of the script **and a random
source**, so no amount of care in the builders could have made them stable.

## What mxcli compares

Before writing a unit, mxcli compares the new document against the stored one in
a **canonical form**: every element `$ID` is replaced by its index in a
deterministic walk, so a difference in *which* UUIDs were minted is not a
difference. If the two are canonically equal, the write is skipped and the stored
document — with its existing IDs — stays exactly as it was.

Skipping is safer than writing, not merely cheaper. The stored IDs are the ones
every pointer inside that unit already agrees with, and nothing outside the unit
can observe them: cross-document references are by qualified name.

The comparison is biased toward writing. If anything cannot be decided, mxcli
writes. A redundant write costs you a diff; a wrongly skipped write would lose
your edit.

## Identity is preserved, not re-minted

A microflow carries a `StableId`, which Mendix declares as an identifier and
which the build turns into the operation id the browser uses to call that
microflow. mxcli carries the stored value onto the rebuilt document instead of
minting a new one, so re-running a script does not renumber operations in your
deployed model.

## A changed document only shows what changed

Skipping the write answers "nothing changed". When something *did* change, the
document is still rebuilt from scratch — so without further care, a one-line edit
lands as a wholesale replacement. Editing a single argument of a single
JavaScript action call used to re-mint **36 of a nanoflow's 37** element
identities; the same edit to a microflow re-minted 21 of 22. Studio Pro's changes
view and `git diff` both key on those identities, so a two-line change read as
"the whole document was replaced". It was also cumulative: changing a value and
changing it back produced a semantically identical document that shared none of
its element IDs with the original.

mxcli now matches the rebuilt document against the stored one element by element
— by shape, and by name where there is one — and puts the stored `$ID` back on
every element that still corresponds, rewriting each pointer to it in the same
pass. On the case above the diff is now the one changed line, all 37 identities
kept, and a change plus its revert returns to the original bytes exactly.

Inserting or deleting an activity mints IDs only for the elements that are
genuinely new; the rest of the flow keeps its identities rather than shifting
onto its neighbours'.

Element IDs are *not* the database's identity — a `GUID` is, and that has always
been carried through. See [the note in
CLAUDE.md](https://github.com/ako/mxcli/blob/main/CLAUDE.md) on why the two must
not be confused.

## Both engines, every write path

The policy lives in one place (`modelsdk/canon`) and is applied at the single
write choke point of **both** the default `modelsdk` engine and the `legacy`
engine. Which engine ran is an `--engine` flag, and it must not be visible in
your diff.

## Turning it off

```bash
MXCLI_ALWAYS_WRITE=1 mxcli exec script.mdl -p app.mpr
```

Every write lands, whether or not anything changed. This exists for bisecting a
suspected elision bug ("does it still reproduce if nothing is skipped?") and is
not a supported option. It disables *skipping* only — identity is still
preserved, because a forced write that re-minted `StableId` would change the
deployed app rather than help you debug it.

## Verifying it yourself

Run your script twice against a settled project and compare the stored units:

```bash
find mprcontents -name '*.mxunit' | sort | xargs sha256sum > before.txt
mxcli exec script.mdl -p app.mpr
find mprcontents -name '*.mxunit' | sort | xargs sha256sum > after.txt
diff before.txt after.txt        # expect no output
```

Two cautions, both of which produce a meaningless zero:

- **Make sure the script is actually re-runnable.** A script containing
  `create module` or `create enumeration` fails on the second run and writes
  nothing, so the diff is empty for the wrong reason. Check the run's output.
- **Run the control** — and note that it is a control on *mtimes*, not on
  content. Since identities are carried, `MXCLI_ALWAYS_WRITE=1` produces the same
  **bytes**; what it changes is that the files are rewritten at all. Compare
  `stat -c %y` instead of `sha256sum` and confirm the timestamps *do* move. If
  they do not, nothing ran and the clean result proves nothing.

  ```bash
  find mprcontents -name '*.mxunit' | sort | xargs stat -c '%Y %n' > t-before.txt
  MXCLI_ALWAYS_WRITE=1 mxcli exec script.mdl -p app.mpr
  find mprcontents -name '*.mxunit' | sort | xargs stat -c '%Y %n' > t-after.txt
  diff t-before.txt t-after.txt    # expect output — writes landed
  ```

The console tells you the same thing, per document: a statement whose write was
skipped reports `Unchanged nanoflow: …` rather than `Replaced nanoflow: …`.

When a run skips **several**, they collapse into one line rather than one per
statement:

```
Modified entity: MyFirstModule.Od07
Created entity: MyFirstModule.OdNew1
39 documents already in sync (unchanged, not listed)
```

Every write that actually landed is still named individually — only `Unchanged`
is counted, because it is the one report that by construction says nothing
happened. A run with exactly one elision prints it in full, so nothing is ever
replaced by a count of one.

For a per-unit view of what would be skipped, `scripts/mprsnapshot -canon` emits
canonical digests keyed by unit id.

See [ADR-0008](https://github.com/ako/mxcli/blob/main/docs/13-decisions/0008-identity-and-idempotence.md)
for the decision and the measurements behind it.

## Writes Are Conditional, and an `$ID` Is Never Renumbered In Place

Storage does not write a unit whose new content is **semantically equal** to what
is stored ([ADR-0008](docs/13-decisions/0008-identity-and-idempotence.md)). The
comparison is on a canonical form — every element `$ID` replaced by its index in a
containment walk — because a rebuild mints a fresh random `$ID` per sub-element,
so comparing bytes would skip nothing. The policy lives in `modelsdk/canon`
(`Reconcile`) and is called at every write choke point in
`modelsdk/mpr/writer_core.go`: `updateUnit`, `WriteTransaction.WriteUnit`
(`codec.Store` reaches storage through this one) and — since ako/mxcli#556 —
`insertUnit`, for the case below.

**A delete followed by an insert is a write path too**, and it is the one that
hides. Several `create or modify` handlers are implemented as delete + create
under the preserved unit ID rather than as an update, and an insert has nothing
stored to reconcile against, so the rebuild's fresh `$ID`s went straight to disk:
`create or modify rest client` rewrote 9 element `$ID`s in a 1,128-byte unit on
every run, forever. `deleteUnit` now remembers what it removed and `insertUnit`
reconciles a re-insert against it (`carryIdentityFromRemovedUnit`). That carry
cannot *elide* — the row and the file are already gone — so a no-op recreate also
restores `_Transaction.LastTransactionID`, which both the delete and the insert
bumped; without it the `.mpr` still showed as modified after every `.mxunit` had
gone quiet. Prefer an in-place update where the handler can do one: the REST
client's own fix is to call `UpdateConsumedRestService` and keep delete+create
only for a folder move, which lives in the unit's row rather than its contents.

**The carry keys on the unit ID, so it does not reach a handler that re-mints
one** — and three handlers did, in one week: the REST client (#556), the view
entity's OQL document (#583) and the layout (ako/mxcli#600). All three took the
same fix, an in-place `UpdateRawUnit` rather than a replacement. The tell is
cheap and worth reaching for first: `ls` the `.mxunit` filenames across two
identical runs. A **changed filename** is delete+insert and the handler is
wrong; a **same filename with different bytes** is the codec or a missing carry
and `canon` is where to look. A replacement also silently reverts the unit's
ROW, which is how `create or replace layout` moved a foldered layout back to the
module root on every rewrite — there is no `FOLDER` clause on the statement, so
the rebuild always names the module root and only an insert applies it.

When something *has* changed, `Reconcile` still does not let the rebuild's fresh
`$ID`s reach disk: `canon.TransplantIDs` matches the incoming document against the
stored one element by element (by `$Type` and shape, by `Name` where there is one,
LCS-anchored within each list) and puts the **stored** `$ID` back on every element
that still corresponds. Without it a one-argument edit re-minted 36 of a nanoflow's
37 element identities and Studio Pro painted the whole document as changed (#910).
Its correctness bar is lower than it looks and worth knowing: a *wrong* match only
makes a diff bigger, because every reference is rewritten with the element — the
one real failure is two elements sharing an `$ID`, which `dropCollisions` guards.

Three rules follow, and each has already been violated once:

1. **Never rewrite an element `$ID` without rewriting every reference to it in the
   same pass.** Pointers are *primitive* properties holding an `element.ID`, not
   `ChildProperty`, so a containment walk traverses the whole document and never
   sees one. PR #125 renumbered IDs this way and made projects unopenable
   (`KeyNotFoundException` at `ResolvePostponedProperties`). A unit is rewritten
   wholesale or not at all. The transplant obeys this by substituting over *every*
   16-byte binary in the document rather than a maintained list of pointer
   properties — any occurrence of one of the document's element IDs is a reference
   by definition, the same insight the canonical form rests on.
2. **Adding a write path means wiring it to `canon.Reconcile`.** A new choke point
   that writes directly will silently churn while everything else is quiet — the
   worst kind of inconsistency, because the diff blames the wrong change.
3. **A new document type with an identity property needs a row in
   `canon.identityFields`.** It cannot be generated: Mendix's `IsIdentifier` lives
   in the modeler assemblies, not in the reflection data `generated/metamodel` is
   built from. `TestFreshGUIDFieldsHaveAnIdentityDecision` catches the common case
   (a property the codec mints fresh on every write) but cannot catch an identity property
   the codec does not mint. Establish the property's status the way `StableId` was
   — the method table is in ADR-0008.

Elision itself is type-agnostic and covers new document types for free, but it
assumes **no binary pointer crosses a unit boundary** (measured 0 of 9,910, not
enforced). A document type that references another *unit* by `$ID` rather than by
qualified name breaks that assumption and invalidates the argument in ADR-0008.

`MXCLI_ALWAYS_WRITE=1` forces every write to land, for bisecting. It does not
disable identity preservation. **Any test asserting "nothing changed" must include
a control** — otherwise the test passes against a build that never had the fix,
which is exactly how PR #125 shipped green. Note what the control can now be:
since identities are carried, a forced write of an in-sync unit produces the
**same bytes**, so "flip `MXCLI_ALWAYS_WRITE` and watch the content change" no
longer distinguishes anything (measured: same sha, mtime moves). Control on the
**rebuild** instead — encode the document twice and show the raw codec output
differs (`TestRebuildChurnsSubElementIDs`) — or, from the shell, on **mtimes**
rather than hashes.

The executor reports which of the two happened: a statement whose unit writes were
all elided prints `Unchanged nanoflow: …` instead of `Replaced nanoflow: …`
(`ExecContext.ReportMutation`, fed by each writer's `WriteStats`). The verb is only
downgraded on positive evidence — writes offered, none landed — so a mutation that
never touches unit storage is reported exactly as before.

**Several elisions in one run collapse into one line**, because that report is the
most repeated thing mxcli prints and an agent pays for it on every later model
call (a tool result is written into the conversation once and re-read by each one).
Measured on a settled 40-statement script: 41 lines / 1,604 B became 2 lines /
177 B. Two rules keep it honest, and each was arrived at by getting it wrong:

1. **Only `Unchanged` collapses.** It is the one verb that by construction reports
   an absence, so no line a reader would act on is ever replaced by a number —
   a mixed run still names every real write individually and counts only the rest.
   Collapsing on volume instead ("after N lines") would hide real writes in exactly
   the runs where they matter.
2. **The trigger is how many arrive, not which entry point ran.** A lone elision is
   printed verbatim, since "1 document already in sync" is worse than the line it
   replaces. Gating on "is this a script?" looked equivalent and is not: `-c`
   reaches `ExecuteProgram` too, because `executeMDL` prepends a `CONNECT`
   statement, so a one-liner collapsed to a count of one.

`mutationTally` (`mdl/executor/mutation_tally.go`), active only inside a program run.
