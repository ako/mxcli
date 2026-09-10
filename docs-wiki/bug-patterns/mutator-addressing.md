---
title: Addressing Things the Model Does Not Name
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - mdl/backend/mpr/page_mutator.go
  - mdl/backend/modelsdk/page_write.go
---

> **Do not duplicate**: the ALTER PAGE / ALTER LAYOUT syntax lives in the skills
> and `MDL_QUICK_REFERENCE.md`; the per-property fixes live in the findings. This
> page is about the addressing problem underneath them.

## What this is

`ALTER PAGE`, `ALTER LAYOUT` and `ALTER WORKFLOW` edit a stored document in
place, which means naming a node inside it. Roughly thirteen `mdl/backend`
findings come from the fact that **many of the interesting nodes have no name**.
A DataGrid 2 column, an Accordion group, a pop-up menu item and a layout region
are all addressable in Studio Pro and store no `Name` at all.

The resulting failures have a distinctive signature: the command reports
success and nothing changes. Or it reports `widget "X" not found` for something
`DESCRIBE` plainly shows.

## How it fits

**A derived name is the only option, and it is not stable.** A column's
addressable name comes from its attribute or caption, so it moves when the
caption is edited. Persisting the name the author wrote is not available either:
there is no slot for it, and inventing one is the write-a-property-Studio-Pro-
does-not-declare hazard. What is left is to make the derivation *visible* — warn
at authoring time which name will be addressable, and on a lookup miss list the
addressable names rather than reporting a bare not-found.

**Ambiguity must be refused, not resolved.** Two columns deriving the same name
is a real state, and taking the first is a data hazard rather than a wart. The
same applies to a bare name that could mean either an object-list item or a
widget: discriminate on the resolved node's `$Type`, name the qualified form in
the error, and do not guess which grid was meant — guessing is what produced an
invalid document.

**A slot is not a name.** A layout region has neither, so it is addressed
positionally (`layoutContainer.top`), reusing the dotted reference that also
serves columns and disambiguated the same way. Only `INSERT INTO` accepts one:
BEFORE and AFTER position a widget among siblings, and treating them as INTO
would silently put widgets somewhere the script did not ask for.

**Property lookup is per-shape, and the shapes differ.** A button's text is a
`CaptionTemplate`, not a `Caption`. A column's value kind comes from the
schema — expression, primitive or text template — and writing a string where a
reference belongs is accepted by everything and visible to nothing.

**A key stored case-sensitively still has to be matched case-insensitively.**
CREATE has always resolved the author's spelling case-insensitively, so any
resolver on the ALTER side that does not is a verb the tool accepts on the way in
and rejects on the way back out — and DESCRIBE, which prints canonical capitalised
names, hands the author the spelling that fails. This has now been the cause
twice: first for first-class widget properties (`set class`), then for pluggable
ones (`set PageSize` on a grid CREATE had just written with `PageSize: 20`), where
the second fix was blocked for a month by the first one's comment asserting that
template keys must match exactly. The way to settle it is to **measure the
ambiguity rather than assume it**: relaxing the match is safe exactly when no
single lookup scope holds two keys differing only in case, which across every
shipped widget template is 0 of 1208 keys — and a test pins that as templates
are added.

**The names a mutation may use are not a list, so the pre-flight runs the
mutation.** The other half of the same story is that `check` could not see any
of this: the properties reference checking resolved were the ones a statement
*carried*, and `SET` carries no widget — it names one already stored, whose
vocabulary is partly a switch in the setter and partly the installed widget
package's own template keys. Nothing in this repo can state that vocabulary for
an arbitrary project, so the check does not try: it opens the document, runs the
real setter against a throwaway copy, and keeps the error. Two resolvers that
must agree are cheaper to make one resolver than to keep in step — the drift
here is silent in the direction that hurts, a pre-flight that passes what the
run then refuses. The cost is that the copy has to be a real copy, which is one
test, and that a target the script itself adds has to be recognised as
not-yet-stored rather than missing — by asking whether it resolves, never by
matching names, since a grid column is inserted under one name and addressed
under its derived one.

**Hand-built BSON drifts from codec-built BSON.** The mutator constructs
documents directly while CREATE goes through the codec, so the two encodings of
"the same" widget diverge — an empty-string value where the codec writes an
explicit key, a typed-array marker present in one and not the other. The reliable
method is to **diff the two encodings** with a BSON dump rather than eyeball the
hand-built one. Getting the marker wrong is not a smaller version of the same
bug: it turns a silent no-op into a project Studio Pro cannot open.

**Check whether the symptom reproduces outside the reported context.** One
"widget not found" turned out to be two independent defects, and the fix for the
reported nesting alone would not have made the reporter's command work.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  properties, shapes and refusals
- [[unloadable-model-writes]] — where a wrong typed-array marker ends up
- [[silent-property-drop]] — the executor-side twin of "reports success, writes
  nothing"
