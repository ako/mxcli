---
title: The Keyword Set Leaks Into User Data
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-grammar.jsonl
  - mdl/grammar/MDLLexer.g4
  - mdl/executor/identifier_quoting.go
---

> **Do not duplicate**: the quoting rules a user needs are in the skills and
> CLAUDE.md ("Quoting Escapes Parser Keywords, Not Platform-Reserved Member
> Names"); each collision's fix is in the findings. This page is about the shape.

## What this is

MDL has a large keyword vocabulary, and those keywords occupy positions where
user data also lives — a widget called `List`, an attribute called `Template`, an
XPath predicate calling `trim()`, a negative number, a string containing a
newline. Eight `mdl/grammar` findings are a collision between the two.

The distinguishing question is not whether a collision happens but **how it
fails**. A parse error is recoverable: the user sees it and quotes the name. The
same collision resolving to a *different valid parse* is not, and several of
these did exactly that.

## How it fits

**Silent is the dangerous half.** A widget conditional using a function whose
name is also a lexer keyword did not error — it was dropped, so the widget lost
its visibility rule and the page built cleanly. A describer emitting a
keyword-named widget bare produced output that failed only when someone re-ran
it. The loud failures in this class cost minutes; the quiet ones cost a
round trip.

**A grammar fix alone can be worse than the bug.** Accepting an unquoted negative
number in XPath made the parser succeed while the AST builder had no case for the
new alternative — so the constraint serialized as `[Amount > ]`, a dropped
operand instead of a loud error. **A grammar alternative and its visitor case are
one change**, and only a round-trip helper caught it; the write path used the raw
text and looked fine.

**Prove a relaxation with a control binary, not by reading.** Making a keyword
optional or a rule more permissive can change how unrelated input parses. The
method that works: stash the `.g4`, regenerate, build a control binary, and sweep
every example with both. Thirteen scripts failed with the change — the *same*
thirteen, all pre-existing.

**Derive the accepted set from the grammar, in a test.** Where a keyword list is
maintained on one side and derived on the other, the next widget package
re-opens the gap. A test that reads the accepted keywords straight out of the
`.g4` cannot drift from it, and can carry the fix instructions in its failure
message.

**A renderer that stringifies an enum outgrows its grammar silently.** An emitter
that formats a value by name will happily print the ninth member of an
enumeration that the grammar knows eight of. Switching on concrete types cannot
do that, which is why the sibling emitter never had the bug — and where
stringifying is unavoidable, the guard is a describe-then-parse loop over the
enumeration's full membership.

**Quoting is not one question.** Quoting escapes MDL's *parser* keywords. It does
not escape Mendix's platform-reserved member names, and it does not help in an
OQL alias. Three vocabularies stack, and a rule that conflates them produces
confident, wrong advice.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  collisions and their controls
- [[describe-round-trip-gaps]] — where an unquoted emit surfaces
- [[platform-semantics-gaps]] — the other two vocabularies quoting does not reach
