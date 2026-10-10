---
title: The Expression Translation Loses Meaning
category: bug-pattern
last-synced: a918689d
covers:
  - mdl/visitor
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor/
  - mdl/visitor/visitor_microflow_expression.go
  - mdl/visitor/visitor_helpers.go
  - mdl/visitor/visitor_xpath_quotes.go
---

> **Do not duplicate**: the Mendix expression rules themselves are
> [[platform-semantics-gaps]] and the skills; each translation's fix is in the
> findings. This page is about the step in between.

## What this is

An MDL expression is not a Mendix expression. The visitor parses one and emits
the other, and thirteen `mdl/visitor` findings are that translation changing
what the expression *means*. This is a different class from writing something
Mendix forbids: here the MDL is correct, the Mendix expression is well-formed,
and it says something else.

The worst of them is the quietest. An additive chain came back from the model
with its operators reordered, so **a microflow computed a different number than
its source said** — with `mxcli check`, `mx check` and the build all green, and
the corruption in the stored document rather than in the description.

## How it fits

**Three ways meaning is lost.**

*A literal changes type.* A decimal literal losing its fraction; an enumeration
value stored as a quoted string instead of a qualified name; a bare identifier
that needed a `$currentObject/` prefix and did not get one. Each produces an
expression Mendix parses and evaluates differently, or rejects with a generic
`CE0117` that names nothing.

*An operator changes.* `/` is division in most languages and member navigation in
Mendix, so the same characters mean two things depending on what is either side
of them. An additive chain rebuilt without walking its children in order swaps
operands.

*A function resolves to the wrong overload.* `contains` and `find` exist as both
string functions and list operations, and the visitor has to decide from the
argument shapes which one was meant — a string literal in the second position
means the string function, two bare variables are genuinely ambiguous at parse
time and get decided later from the declared type.

**Preserving raw source is the standard fix, and it carries hidden tokens with
it.** Where the rebuilt expression is lossy, the visitor keeps the original text.
The trap is that ANTLR's `GetText()` excludes hidden tokens while a source-interval
slice *includes* them — so any code reaching for original text inherits every
comment in that span, and a `--` comment between two operands ends up inside the
Mendix expression. Replacing a comment with whitespace rather than with nothing
matters for the same reason: `1 --c\n+ 2` must not become `1+ 2`.

It carries the *language's* string escapes too. Under `mdl 0` a backslash
escapes, so the tree holds a literal's unescaped value while the preserved text
still spells `\'` or `\\` — two storage paths for one literal, storing
different bytes. The same leak ran the other way in the renderer, which
pre-escaped values so that describe's verbatim output would re-read: the round
trip stayed green while the model held a backslash too many (#810, #820). The
rule that closed both: the store writes the Mendix spelling of the value on
every path, and only describe spells it for the reader's language.

The corollary is about re-reading: once text is in Mendix spelling — read from
the model, or already transcribed by the visitor — it is read with Mendix's
string rule whatever the script's header says. The XPath layout pass re-parsed
stored constraints with the mdl 0 lexer, so a multi-line `'C:\'` corrupted the
constraint under *both* languages (#825); asking "which language is the script"
would have fixed only one. Ask "whose spelling is this text".

**`GetText()` has the opposite defect from a source-interval slice, and it is
worse.** The slice carries hidden tokens; `GetText()` **concatenates visible
tokens with nothing between them**, so the whitespace that separates words
disappears: `Flag: true and false` was stored as the expression `trueandfalse`,
`if true then 'a' else 'b'` as `iftruethen'a'else'b'`. Which gives the blunt
rule — **`GetText()` on an expression context is always a bug** — and a grep that
finds them (`Expression().*GetText()`, `expr.GetText()`). The earlier fix for the
comment leak moved six microflow sites onto a shared helper and missed four more
that lived in page and REST code rather than microflow code, which is the usual
reason this class comes back: the sites are grouped by document type, not by
what they do.

**Precedence is part of the translation.** `/` shares a level with `*`, `div` and
`mod`, so a left-associative chain reads `$a/X * $b/Y` as `(($a/X) * $b) / Y` — a
division where the author wrote a member path, stored and evaluated as such with
every check green. The fix belongs where the AST is **built**, because that fixes
every consumer at once; an earlier attempt taught only the rule that had noticed
the mis-nesting to tolerate it, leaving the serializer and the iterator
qualification reading the wrong tree.

**Quoting is the other systematic leak.** The guidance to quote identifiers is
about the MDL *parser*; a quote that survives into the stored expression produces
`Mod."Entity".Attr`, which Mendix rejects. The tell is a **half-stripped** name in
the error message — one half went through the unquoting reader and the other did
not, which localises the bug to a `GetText()` call that should have been the
structured accessor.

**Position decides whether a quote is a name or a value.** The quote-stripping
that saves `Mod."Entity".Attr` is right for a *name* and wrong for a *value* — and
Mendix XPath never compares two members, so a quoted token after `=`, `!=`, `<`,
`<=`, `>` or `>=` is always a mis-quoted string. Left alone it stores a literal
that compares against itself: `where '["Status" = ''Accepted'']'` grants no rows,
with check, exec, lint and `mx check` all passing. Two consequences. The
normalisation lived in some XPath sinks and not others — the grant and the
workflow targeting had never been routed through it, both spellings each — so the
grep is for the normaliser's callers, not for the symptom. And because refusing
the quoted-value form fixes a silent wrong *write*, it applies under every
language version rather than waiting for a header.

**The same expression text means different things in different slots.** A widget
`Visible:` is a client expression and needs bare identifiers prefixed; a
datasource `where` is real XPath and must not be. An enum in the first stringifies
to a qualified name and in the second to a quoted string. A fix applied to
"expressions" rather than to a specific slot breaks the other one.

**Nothing downstream can catch this.** The output is a syntactically valid Mendix
expression, so `mx check` is satisfied, the build succeeds, and the only
observable difference is a value at runtime. Verification has to be the round
trip plus reading the stored bytes — `strings` on the `.mxunit` is what showed the
swapped operands.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  literals, operators and overloads
- [[platform-semantics-gaps]] — expressions Mendix forbids outright
- [[keyword-collisions]] — where the quoting leak starts
- [[two-meanings-one-script]] — the version boundary these spellings sit across
