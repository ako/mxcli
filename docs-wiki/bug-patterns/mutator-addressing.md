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
`CaptionTemplate`, not a `Caption`. Page-level property names are case-sensitive
while widget ones are matched lowercase. A column's value kind comes from the
schema — expression, primitive or text template — and writing a string where a
reference belongs is accepted by everything and visible to nothing.

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
