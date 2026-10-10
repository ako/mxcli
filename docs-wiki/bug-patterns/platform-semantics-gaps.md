---
title: Legal MDL, Illegal Mendix
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/validate_microflow_ce_gaps.go
  - mdl/executor/validate_microflow_loop_scope.go
  - mdl/executor/cmd_entities_system_members.go
  - modelsdk/meta/system_module.go
---

> **Do not duplicate**: each rule's predicate, CE number and measured fix live in
> the findings and in `mdl/executor/validate_microflow*.go`; the MDL syntax lives
> in `MDL_QUICK_REFERENCE.md`. This page describes why the category exists at all.

## What this is

MDL is deliberately SQL-shaped and permissive: a statement is well-formed if it
parses. Mendix's *semantics* are considerably narrower, and the narrowing has no
syntactic marker — the same statement is well-formed MDL and an illegal model.
This is the largest remaining class of executor finding after the read/write
ones, and it is not a defect in any single component. It is the seam between two
languages.

## How it fits

**Four sub-languages, each with rules the MDL grammar cannot carry.**

*Mendix expressions.* An association path is not a value — reaching an object
over an association has to be materialised with a `retrieve` before it can be
passed as an argument. Word operators must be lowercase; an uppercase `AND` in a
condition builds as `CE0117`. `dateTime()` takes numeric constants, not
variables. Each of these is a plain expression as far as MDL is concerned.

*XPath.* Mendix's dialect reaches at most **one** hop off a variable; `= empty`
tests attributes, not associations; there is no `id` member to constrain on. A
constraint is a string to MDL, so nothing about its shape flags any of them.

*Microflow variable model.* A loop iterator is scoped to the **whole microflow**,
not to its loop — so two loops reusing `$R` is a duplicate name (`CE0111`), while
a reference to `$item` *after* `end loop` is out of scope (`CE0108`). Both halves
are counter-intuitive in opposite directions, and both read as ordinary code.
`$x = call M.F()` is a variable *creation* each time, so the natural "try A else
try B" fallback chain is also a duplicate.

*Entity and document rules.* `not null` and `unique` are validation *rules* in
Mendix's model, which is why they are rejected on a non-persistent entity
(`CE0070`). A `declare` maps to a Create Variable activity, which may not produce
a list. An `else` on a type split is the `(empty)` flow — a null object — not a
default branch, so it never catches an unlisted subtype.

**A fifth sub-language: the platform's own module.** `System` is a module with a
domain model and **no stored unit** — it is synthesized in code — so every *read*
of it resolves and every *write* reaches for a container that does not exist.
`CREATE ENUMERATION System.X` reported success and wrote an orphaned unit
parented to a synthetic id. The general lesson is uncomfortable and worth
stating plainly: **making a synthesized element readable makes it writable, so
audit every write verb in the same change.** The read fix was one append;
enumerating what it exposed took three passes and kept growing — create, alter,
drop, and `MOVE` in *both* directions, since guarding only the source still lets
a user element be moved *into* System. Any per-module write sweep has the same
hole: an access reconcile of the TO end's module tried to open a zero-GUID unit
for it.

The inverse holds for references *into* System: they can never appear in a
project listing, because the project does not store them. A resolver that
refuses what it cannot find there is wrong, and the honest behaviour is **not to
judge** — mxbuild resolves the name, and its member lists are not in a readable
form anywhere in mxcli.

**System members are flags on the root of the generalization chain, and two of
them are associations.** `owner` and `changedBy` are associations to
`System.User`, addressable only as `System.owner` / `System.changedBy`, and no
domain model lists them; an entity carries them as `HasOwner` / `HasChangedBy`
on the root's `NoGeneralization`. Everything surprising follows from that: the
names are lower camel case and the case is significant, so `sort by CreatedDate`
— the spelling DESCRIBE prints for the pseudo-type — is CE1613; a declaration on
a *specialization* has exactly two meanings, redundant (the root stores it) or
impossible (it does not, and for a System root nothing can change it); and a
retrieve over `$x/System.owner` has no storable shape, because the association
is not an element there is an id for. The stored flags are **measurable** rather
than guessable — each root's `NoGeneralization` in `deployment/model/model.mdp`
carries them, which is the same place the System attribute *lengths* come from
(they are not in the Model SDK at all).

**None of this is discoverable from the MDL side.** The author writes something
that reads correctly, `mxcli check` has no rule for it, `exec` writes it, and the
build names a CE code against a construct the author believed was ordinary. The
gap is closed one rule at a time, and each rule is a claim about the *platform*
that has to be measured against a real mxbuild rather than reasoned from the
error text.

**The rules divide by how confidently they can be enforced**, and that decides
where they are wired. Rules that predict a *platform* constraint — the XPath and
expression ones — are safe to enforce unconditionally, because the constraint
does not depend on anything mxcli generates. Rules that predict what the
*builder* emits are not: building one such change turned up four shapes where the
AST reads "broken" and the build disagrees, two of them in mxcli's own shipped
examples. Those stay off the unconditional write barrier and leave `--no-check`
as an escape hatch.

**Not every report in this class is a defect.** Several were measured and closed
as correct behaviour: quoting an identifier escapes MDL's parser keywords and not
Mendix's *platform*-reserved member names; `else` on a type split really is the
empty flow; `create` really is non-idempotent, in the SQL sense the language is
modelled on. What those cases needed was a better error message or a
documentation fix, not a code change — and telling the two apart is what the
measurement is for.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each rule, its
  CE number, and the mxbuild run that established it
- [[check-mxbuild-drift]] — what happens when one of these rules is wrong
- [[mdl-as-sql]] — why the language is permissive by design
- [[binding-context]] — the sibling class where the name is legal and resolved
  against the wrong object
