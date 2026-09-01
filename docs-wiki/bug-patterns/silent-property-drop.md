---
title: Properties That Parse but Never Persist
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/validate_widgets.go
  - modelsdk/widgets/definitions/
---

> **Do not duplicate**: the per-widget property tables live in
> `sdk/widgets/templates/` and the generated `.def.json`; the individual aliases
> and rule IDs live in the findings. This page describes why the class exists.

## What this is

An MDL property is accepted, `mxcli check` passes, `exec` reports success,
`mx check` reports 0 errors — and the value never reached the model. The widget
renders, and does nothing. Twenty-two of the executor findings are this.

The outcome is indistinguishable from a typo, and that is the core of the
problem: `Contnet:` and a real-but-unrouted `DynamicCellClass:` failed in
exactly the same silent way, so nothing in the pipeline could tell the author
which one they had written.

## How it fits

**Parsing is not persisting.** A widget property crosses several independent
hops — grammar, AST, the executor's property switch, the widget definition's
mapping, the builder, the codec — and falling off any of them produces the same
nothing. The grammar cannot help: `Key: value` is generic by design, because the
parser has no idea which keys a given widget routes.

**The permissive allow-list is the usual mechanism.** Widget property names were
deliberately not rejected, so that `Label:` and `Class:` on an unfamiliar widget
would not produce false errors. That choice quietly converts every *unrouted*
name into a silent drop, and the cost stays invisible until a build error names a
property the author believed they had set.

**Three sources of truth compete, and the wrong one usually wins.** Pluggable
widget mappings are generated into `.def.json` from the `.mpk`, but a
hand-written built-in definition beats the generator — so fixing the generator
alone changes nothing for the widgets that have one. And the generated defs are
version-stamped and cached per project: a code-only alias addition is invisible
until `WidgetDefGeneratorVersion` is bumped and the stale defs regenerate.

**Where a vocabulary can be derived, derive it.** For pluggable widgets the
*known but unmapped* set falls out of two artifacts already present: every
property key the `.mpk` declares, minus everything the generated definition maps.
That needs no per-widget knowledge and cannot go stale, and it turns one silent
outcome into a three-way answer — an unrecognised key gets a "did you mean"
warning, a recognised-but-unmapped key gets "this will be dropped, set it in
Studio Pro", and a mapped key stays quiet.

**Where it cannot, enumerate generously and guard the list.** Built-in widgets
have no `.mpk` to subtract from, so their vocabulary is a hand-maintained union —
grammar keyword properties, every key the builders consume, and every property
`describe page` can emit. It is deliberately a union across *all* widget types
rather than per-type, because a per-type list would produce false warnings, and
the describe half is included so a describe → create round trip never warns about
its own output. A manually maintained list is a maintenance risk, which is why it
carries a drift test rather than a promise.

**Warn; do not reject.** Neither the pluggable nor the built-in vocabulary can be
proven complete, so an error would trade silent drops for false refusals. The
same reasoning appears wherever mxcli names a *specific* alternative — those
lists are deliberately explicit rather than inferred, because inferring them
would mean reimplementing the engine's dispatch and getting it wrong in the other
direction.

**Wire the read at the same time as the write.** DESCRIBE is how a dropped
property gets *found*, so a write fix without its describe counterpart leaves the
model holding a value that the tool reports as absent — the same lossy round trip
wearing the right answer. Several findings here are the follow-up half of a fix
that shipped write-only.

**A half-wired feature is worse than a missing one.** When the grammar already
accepts a form, scripts using it look correct and produce models without it.
Before assuming a spelling works, follow the whole chain: AST → model → both
writers → both readers → describe.

**And a check wired into nothing is invisible.** `exec` used to apply scripts
that `check` rejected outright; a validator that no pass calls reports no
violations and reads exactly like a clean project.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  aliases, rule IDs and widget-specific spellings
- [[describe-round-trip-gaps]] — the read-side half of the same failure
- [[widget-type-object-drift]] — when the property *is* written and the widget
  definition is what disagrees
