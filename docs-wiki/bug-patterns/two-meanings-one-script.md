---
title: Two Meanings, One Script
category: bug-pattern
last-synced: a918689d
covers:
  - mdl/visitor
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor/
  - docs/13-decisions/0011-mdl-language-versioning.md
  - mdl/visitor/visitor_upgrade_fixes.go
  - mdl/upgrade/gated.go
  - mdl/visitor/visitor_silent_drops.go
  - mdl/visitor/visitor_quoted_expression.go
---

> **Do not duplicate**: the versioning decision and its two kinds of change are
> canonical in [ADR-0011](../../docs/13-decisions/0011-mdl-language-versioning.md);
> the registry of gated changes is `LanguageChanges()` and `gatedRewriters`; each
> instance is in the findings. This page is about what goes wrong while a script
> can mean two things.

## What this is

ADR-0011 lets MDL change what existing text *means* — `'it\'s'` stops being an
escape, `limit 1` stops binding an object, a quoted expression property stops
being the expression's text — without rewriting scripts users have committed.
The mechanism is a header: a script with `mdl 1;` gets the new meaning, a
headerless one keeps the old, and `fmt --upgrade` adds the header along with the
rewrites that preserve behaviour.

For as long as both meanings are live, **every component that reads MDL has to
agree on which one applies**, and the findings in this class are the places that
did not. They cluster into three failures: a gate consulted in the wrong layer, a
rewrite that cannot find what the gate recorded, and a value stored in one
language's spelling and read back in the other's.

## How it fits

**The gate lives in the visitor, so a refusal added in the executor cannot see
it.** A change of meaning shipped with its refusal of the old spelling as an
executor validator, ungated — and the validators run on the AST after the
visitor has already decided which meaning applies, so a perfectly valid
headerless script was refused for using the construct exactly as its own version
defines it. The rule that follows: **decide per version where the version is
known**, so the executor only ever sees the meaning of the script's own version.
The audit is cheap and worth running on any change of meaning — grep the commit
for new error-severity validators and ask what a headerless script using that
construct does now.

**A rewrite keyed on the parse tree and a gate keyed on the AST do not match
one-for-one.** The upgrade's rewrites are computed in the visitor because the
parse tree is the only place a construct's tokens and positions are still at
hand; the gate records an `ast.LanguageNote`. But the AST *normalises*: several
grammar rules collapse into one node, so a rewrite that recognises its construct
by parse-tree shape silently misses the alternatives that normalise into the same
note. `min`/`max`/`avg` parse through the OQL aggregate rule rather than the list
aggregate rule, and the upgrade refused the header on a form it could have
rewritten. **Enumerate the cases from the AST builder, not from the grammar rule
the report named.**

**Answer the upgrade's questions with the builder's own resolver.** Deciding
whether a header is safe often needs a type — is this operand a list? — and the
visitor has no project. One fix counted every `$x = call …` as unknown and
refused the header, although the called flow is declared in the same script. The
general form is [[duplicate-resolver-drift]] wearing an upgrade hat: re-deriving
a subset of what the builder knows produces a verdict the builder disagrees with.
And the "execute under both versions and compare" control is **asymmetric** —
some misreadings write the same model under either version, so only one
direction of the mistake is observable that way.

**Sort a "silently stored as X" refusal by what X was.** A single earlier change
refused `float`, `currency` and `date` together, on the grounds that all three
were silently stored as something else. But `float` and `currency` stored a
*String* — a wrong model, so refusing in every version is right — while `date`
stored the `DateTime` the author could only have meant. That makes `date` an
alias, which belongs in the deprecation registry: warn, rewrite under
`fmt --upgrade`, refuse only under the new version. Grouping them cost users a
working spelling. For the alias to be provable, the builder has to produce the
canonical AST, not the aliased one.

**Derive a compatibility heuristic's inputs from describe output, not from
hand-written examples.** Where a change of meaning needs a heuristic — "did the
author mean this quoted value as expression text?" — the largest body of
headerless scripts in existence is **mxcli's own describe output**, and it quoted
every stored expression whole. A heuristic tuned on hand-written scripts anchored
on the simple shapes and silently changed the meaning of every compound one.

**A value is stored in one spelling and must be read in that spelling, whatever
the script says.** This is the string-escape half of the class, and it is
documented at length in [[expression-translation-drift]]: the store writes the
Mendix spelling of the value on every path, and only describe spells it for the
reader's language; text already transcribed by the visitor, or read back from the
model, is read strictly. Two things are specific to the version boundary. A
construct that moves from "stored as written" to "interpreted value" has to drag
**every upgrade rewriter that branches on that distinction** with it — probe the
upgrade with escapes, not only with line breaks. And a pre-pass that scans raw
source for string boundaries must use **that source's** lexer rule: the comment
strip runs before the transcription to Mendix spelling, so under the old
language it still has to honour the old escape.

**A format with no header is a third state, not a default.** A `.test.mdl` takes
no language header at all, which makes "which meaning applies" a question with a
different answer again — see
[[generated-mdl-on-the-authors-lines]], where a header written into one became
content in the first chunk and ate a test.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each gated
  change, its rewrite, and the control that proved it
- [[expression-translation-drift]] — the string-spelling half, in depth
- [[visitor-wiring-gaps]] — where the two meanings are not the problem
- [[version-gating]] — the *other* version axis: Mendix's, not MDL's
