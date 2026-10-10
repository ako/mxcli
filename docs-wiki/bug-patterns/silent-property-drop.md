---
title: Properties That Parse but Never Persist
category: bug-pattern
last-synced: ced830e0
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
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
carries a drift test rather than a promise — and **the drift test must read the
source, not a second hand-typed list.** The first guard was a typed sample of the
describe vocabulary; when #813 taught the dataview builder and `describe` a
`ShowFooter`, neither list learned it, and `check` warned that a property it
wrote would be dropped (mendixlabs/mxcli#1346). A `go/parser` scan of the
builders' `Get*Prop` / `lookupPropCI` / `Properties["…"]` reads plus describe's
`"Key: "` strings found four more keys in the same state on its first run.

**Warn; do not reject.** Neither the pluggable nor the built-in vocabulary can be
proven complete, so an error would trade silent drops for false refusals. The
same reasoning appears wherever mxcli names a *specific* alternative — those
lists are deliberately explicit rather than inferred, because inferring them
would mean reimplementing the engine's dispatch and getting it wrong in the other
direction.

**Except where the engine's dispatch *is* the definition — then refuse.** The
action keywords are the case: `onClick:` and `OnChange:` are builtin names,
accepted on every widget, but the engine reads them only through a mapping
sourced from them. Whether a widget routes them is therefore a lookup, not an
inference, and a refusal cannot be false — though as a new rejection it still
waits for the `mdl 1;` header, and warns without it (ADR-0011). The Gallery held a hand-written
definition with no action mapping at all, so its row click passed `check`, was
dropped, and a re-execution that changed only the action reported "Unchanged"
(#842) — a year after the generator learnt the same mapping for the data grid
beside it. When a hand-written definition is the reason, audit every other
hand-written one: the same diff (template action keys minus mapped keys) found
the filters' `onChange` and the barcode scanner's `onDetect` in the same state.
And a fix that makes a property *persist* can surface a rule the widget itself
enforces on it — once the row click was written, the Gallery's own editor check
failed the build for the defaults mxcli writes (a selection on a single-click
trigger), so the fix had to carry that rule into `check` too.

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

**The sharper version: a check can be switched off by its own input.** Where a
family of rules hangs off one parsed fragment — a select clause, a resolved
definition, a decoded sub-document — a parse that yields nothing does not report
"I could not read this"; it reports nothing at all, which is the same output as
a clean file. The view-entity OQL scanner did this twice, two months apart and
in the same function: once because it compared cases wrongly, once because
Mendix's other clause order put the terminator behind the keyword it scanned
forward from. Both times a single visible false positive was the only hint,
while three real rules stopped running behind it. The durable fix is not the
next input shape, it is making *unreadable* a reported outcome distinct from
*nothing to report* — after which a third shape costs a diagnostic rather than a
blind spot.

**A routed key can still drop its value — by shape.** The key can be mapped and
the value still never arrive, when the grammar gives it a shape the property's
reader does not take. `dynamicBarColor: $currentObject/ColorHex` parses as a
variable-led *data source*, and every scalar reader stringified a non-string to
`""`, so a chart series' Expression property was written empty while `check`,
`exec` and `mx check` were all clean. The visitor cannot settle it — only the
widget's schema knows that key is Expression-typed — so the fix is to carry the
source text to the layer that knows the kind, and to make every scalar reader
answer "this shape does not fit" (MDL-WIDGET42, and an error at build) instead of
falling to `default: continue`. When a grammar alternative is chosen by the
*first token*, audit which other meanings that token starts.

**One property, two keys: a consumer that reads one drops the other.** The
visitor lowers `Visible:` by value shape — an expression to `VisibleIf`, a plain
value to `Visible` — and the two writers each read only one: page widgets read
`VisibleIf` (and once dropped `Visible: false`), DataGrid 2 columns read
`Visible` (and dropped every expression). Grep the consumers of the *key the
visitor writes*, not of the property name. And expect persisting a value to
surface a rule its absence hid: a column's visibility has no row object, so the
`$currentObject` examples that had always "worked" became CE0117 the moment they
were written, and the fix needed a check rule to go with it.

**Children drop the same way properties do.** A widget's body is distributed by
several passes that each skip what they do not recognise, so a child matching no
container, no slot and no catch-all is built and discarded exactly as an
unrouted property is. The compounding factor is that the *same slot* wears
different keywords on different widgets — the filters placeholder is `filter` on
a Gallery and `controlbar` on a Data Grid — so a form copied between two widgets
is both plausible and inert. That table is also the fix: because the engine
already maps keyword to property per widget, the diagnostic can name the
spelling *this* widget uses instead of listing everything it declares.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  aliases, rule IDs and widget-specific spellings
- [[describe-round-trip-gaps]] — the read-side half of the same failure
- [[widget-type-object-drift]] — when the property *is* written and the widget
  definition is what disagrees
