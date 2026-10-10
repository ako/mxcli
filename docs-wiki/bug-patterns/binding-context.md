---
title: Which Object Is This Name Read Against?
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/widget_attribute_scope.go
  - mdl/executor/cmd_pages_input_binding_context.go
  - mdl/executor/cmd_pages_parameter_binding.go
  - mdl/executor/validate_page_button_context.go
  - docs-wiki/architecture/widget-engine.md
---

> **Do not duplicate**: the one binding rule and its three scope kinds are
> canonical in `mdl/executor/widget_attribute_scope.go`'s own comment; the
> `$Param.Attr` spelling is in `MDL_QUICK_REFERENCE.md` and the pages skills;
> the per-widget fixes are in the findings; the engine's build order is
> [[widget-engine]]. This page is about why the question keeps getting answered
> wrong.

## What this is

`Attribute: FullName` does not denote anything on its own. It denotes an
attribute **of some object**, and which object is decided by where the widget
sits, what its template declares, and what the enclosing container retrieves.
MDL lets an author write the short form everywhere — that is the point of it —
so mxcli has to supply the missing half, at four layers that each keep their own
idea of "the current object": the visitor (which spelling is this?), the builder
(what do I qualify it with?), the writer (is this path storable?) and `check`
(is this legal?).

The failure is never a crash. A name resolved against the wrong object produces
a *valid model that reads the wrong data*, and the signal arrives from mxbuild
as **CE1613 "the selected attribute no longer exists"** — naming a
fully-qualified attribute the author never typed. When the wrong object happens
to have an attribute of that name, there is no signal at all.

## How it fits

**One slot cannot hold a per-property answer.** The page builder carried a
single `entityContext` string, and the question is per property: a pluggable
widget's template states the link in `widget.xml` (`dataSource="…"` per
property), so two datasources mean two answers inside one widget. With one slot
the engine answered "whichever datasource mapping ran last" — for a widget's own
properties and for the enclosing container's alike. A pass that runs *after* all
the datasource mappings is where a shared "current entity" is guaranteed to be
wrong, and the re-apply pass was exactly that, silently overwriting the
correctly-linked values the mapping pass had just written.

The remedy generalises past widgets: **the schema already carried the answer and
nobody read it.** The loader had the per-property link; a hand-written copy
between two spellings of one Go struct dropped the field, so the engine
reconstructed the link from `.def.json` mapping order. When a context defect is
fixed at widget level, look for the second copy — object-list items run the same
pre-resolve/resolve shape in their own loop, and the chart series that failed
next was that twin.

**"Qualify with the context entity" is not the same as "find the declaring
entity".** Concatenating `entity + "." + name` is correct only when the entity
declares the attribute itself; on a specialization the inherited member is
stored against the entity that *declares* it, so the concatenation produces a
name no entity has. Each hand-qualified path is its own instance, which makes
the grep (`entity + "." +`) more useful than the report. The same shape appears
one branch over: a writer that tells an attribute from an association **by dot
count** qualifies a bare association name into the attribute slot, because a
dotless name in MDL can be either and only a lookup can say which.

**A two-valued "do we know the context?" flag hides the state that is the bug.**
known/unknown collapses *there is no context object* into *we cannot say*, and a
guard's own stand-down rule then protects the defect it was written to catch.
Three states are needed — known-and-present, known-absent, unknown — because the
three get different treatment: known-absent refuses the bare name *and* the
qualified one; unknown refuses only what the writer provably nulls. That
distinction is load-bearing rather than tidy: an excluded marketplace page whose
data-source flow is missing has no resolvable context, and DESCRIBE writes
qualified names there that must keep building.

**Scope is a property of the position, not of the slot the report used.** A data
container's widget-name variable (`$dvGate`) exists only for the containers
nested *below* it; in its own context the object is `$currentObject`. The report
named one slot — an action argument — and the rule is the same for a nested data
source's arguments, a visibility expression, dynamic classes and a dynamic text,
because all of them are evaluated in the widget's enclosing context. One probe
page with ten buttons through `mx check` settled six shapes in one build and the
literal reading of the issue would have missed four. The inverse case is just as
easy to get wrong in the other direction: a control bar takes its context from
*above* its grid, so a nested grid's control bar does have an object and
refusing it was a false positive on a project that builds clean.

**Studio Pro has a binding MDL could not spell, and the gap reads as a
refusal.** A widget outside every data container still binds — to a page or
snippet **parameter**, stored as an `AttributeRef` beside a `Forms$PageVariable`
naming the parameter. DESCRIBE printed the attribute bare, so `exec` either
refused it (there is no enclosing object) or wrote a binding with no source
variable: a different document. The cure for a refusal that fires on
Studio Pro-authored content is usually a **spelling for the stored shape**, not
a looser guard — and the survey that finds them is over the slot combinations
the corpus actually stores, not over the ones the report mentions.

**Both halves or neither.** A describer that emits a path the builder cannot
consume turns silent corruption into a build error, and a builder taught a form
the describer still shortens re-binds the widget on the next round trip. The
build-level control is what settles these: same script, same mxbuild, old binary
→ CE1613, new binary → 0 errors, with the probe entity deliberately lacking an
attribute of that name so a wrong binding cannot pass by coincidence.

**A guard on one property name is not a guard on a builder with several
callers.** The association-path data source was refused by name on the widget's
`DataSource:` key while the engine reached the same builder from named
datasource keys and object-list item properties. Put a form only one writer may
store behind an entry point only that writer's builder calls, so the next call
site is refused by default rather than admitted by omission.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-widget
  instances; `grep -rl CE1613 .claude/skills/fix-issue/findings/` reaches most of
  this class
- [[widget-engine]] — the build order the per-datasource scope has to survive
- [[duplicate-resolver-drift]] — why `check` and `exec` share one scope function
  instead of each deriving it
- [[describe-round-trip-gaps]] — the read half: a qualified name shortened to a
  bare one the reader cannot re-derive
- [[platform-semantics-gaps]] — the neighbouring class where the name resolves
  and Mendix still refuses it
