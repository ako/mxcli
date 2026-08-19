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

For a per-unit view of what would be skipped, `scripts/mprsnapshot -canon` emits
canonical digests keyed by unit id.

See [ADR-0008](https://github.com/ako/mxcli/blob/main/docs/13-decisions/0008-identity-and-idempotence.md)
for the decision and the measurements behind it.
