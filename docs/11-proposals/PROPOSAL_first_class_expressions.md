---
title: First-class expressions for expression-typed MDL properties
status: draft
date: 2026-09-08
related:
  - https://github.com/mendixlabs/mxcli/issues/750
  - PROPOSAL_expression_type_checking.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
---

# First-class expressions for expression-typed MDL properties

> Written for `mendixlabs/mxcli#750`, which lists four design questions and
> defers them to a proposal. This answers them, and corrects the framing the
> problem is usually reported with.

## 1. The problem, and what it is *not*

A property that holds a Mendix expression but is declared as a quoted string
forces every quote inside it to be doubled:

```mdl
dynamicclasses: 'if $currentObject/Featured then ''is-featured'' else '''''
```

That `'''''` is `else ''` — an empty string — inside a doubled-quote string.
Counted across the shipped skills and examples: **23 runs of four or more
consecutive quotes**, twelve of them five-long.

The worst case is not additive but *multiplicative*, and it turns up wherever a
stored value already carries Mendix's own escaping. An offline sync constraint
stored as `contains(ActionValue, '''abc''')` re-emitted into a quoted MDL
string became:

```mdl
where '[ ( contains(ActionValue, ''''''abc'''''') ) ]'
```

Six. Correct, verified by round trip, and unreadable.

**It is not caused by expressions being stored as strings.** That framing is
natural and wrong, and `PROPOSAL_expression_type_checking.md` already corrected
it once, in its 2026-06-19 revision:

> It assumed our microflow expressions are stored as **raw strings**… In fact
> our visitor **already parses expressions into typed `mdl/ast` nodes**.

XPath is parsed too — `XPathPathExpr`, `XPathStep`, `AttributePathExpr`. And a
retrieve's constraint already round-trips first-class:

```
where Distance > 0 and contains($L/Name, 'abc');
```

So the machinery exists and is used. The defect is narrower and more tractable:
**specific properties are declared as generic quoted-string slots** while the
expression grammar sits unused beside them. `dynamicclasses` is handled by name
in `mdl/backend/pagemutator/mutator.go:2526` as an ordinary string property; it
never meets an expression rule at all.

## 2. There are two expression families, not one

This is the question #750 flags and the reason it has not moved:

> Note `[ … ]` today parses `xpathExpr` (XPath-flavored), whereas
> `dynamicclasses` is a full **Mendix microflow expression**.

The families are genuinely different languages:

| | Grammar | Shape | Used by |
|---|---|---|---|
| **XPath constraint** | `xpathConstraint: LBRACKET xpathExpr RBRACKET` | `[Amount > 0 and contains(Name, 'x')]` | `visible:`, `editable:`, `retrieve … where`, offline `sync … where` |
| **Mendix expression** | microflow `expression` | `if $x/F then 'a' else ''` | `dynamicclasses`, `DynamicCellClass`, page-variable defaults, calculated attributes |

**Recommendation: do not unify them.** One delimiter over two grammars means
either a parser that guesses which language it is reading, or an XPath rule
quietly extended until it accepts `if … then … else` — and a value that parses
under the wrong grammar produces a document that stores cleanly and fails at
build. The delimiter should follow the family.

A worked precedent for the XPath half already exists, added while this proposal
was being written: `sync … where` takes `[…]`, `DESCRIBE` emits it, and the
quoted form still parses. It took a grammar alternative and eight lines of
visitor. That is the shape of every XPath-family slot.

## 3. Answers to #750's four questions

### 3.1 Delimiter

- **XPath family: reuse `[ … ]`.** It already means "XPath constraint" in three
  places; a fourth is free, and readers already know it.
- **Expression family: reuse the microflow `expression` rule with no delimiter
  at all**, the way `if`/`while`/`return` already do — the property's `:` is the
  delimiter:

  ```mdl
  dynamicclasses: if $currentObject/Featured then 'is-featured' else ''
  ```

  Brackets here would be actively misleading: `[…]` reads as XPath everywhere
  else in MDL, and this is not XPath.

  The cost is real and worth stating: an undelimited expression has to end
  somewhere, and inside a `(key: value, …)` property list that means the
  expression grammar must not consume the `,` or `)`. This is the one place the
  work is more than additive, and it is why the XPath half should ship first.

### 3.2 Backward compatibility

Keep the quoted form parsing everywhere it parses today, permanently. It is not
deprecated: a quoted `'…'` remains the way to write a value that really is a
plain string, and the distinction is what lets a reader tell them apart.

Two properties make this safe rather than merely polite: the two forms must
produce an **identical** stored value, and there must be a test that says so.
The `sync … where` implementation has one; every slot converted needs its own.

### 3.3 DESCRIBE

Emit the first-class form. That is what makes the change worth doing — it is not
cosmetic, because a describer emitting the quoted form has to escape, and
escaping is where these defects live:

- `#1006` — `DESCRIBE WORKFLOW` emitted `TARGETING USERS XPATH` without doubling
  inner quotes, so its own output failed `mxcli check`.
- `#394` — `DESCRIBE ENUMERATION` emits unescaped single quotes in captions.
- `#642` — the quoted `where '<xpath>'` form mis-stored *every* constraint
  (CE0161) while the bracket form was correct.

Three bugs, one root: a describer that has to escape eventually will not.
Emitting a form that needs no escaping removes the class.

**One caveat measured on the offline-sync change.** A stored constraint carries
Studio Pro's own whitespace, and the first rewrite normalises it — describe →
exec → describe differs once, then is stable. That is `normalizeXPathTokens`,
which every retrieve constraint already goes through, so it is existing
behaviour rather than something the first-class form introduces. Worth stating
in any slice's acceptance criteria so it is not mistaken for a round-trip bug.

### 3.4 Scope

Ship in family order, XPath first because it is additive:

1. **XPath family** — audit for slots still taking a quoted constraint. `sync …
   where` is done; `targeting users xpath` (#1006's slot) is the obvious next.
2. **Expression family, single-value slots** — `dynamicclasses`,
   `DynamicCellClass`. Highest-value by the count in §1, and the ones #750 names.
3. **Expression family, inside property lists** — page-variable defaults,
   calculated attributes. Needs §3.1's termination question settled first.

## 4. What this unlocks

`PROPOSAL_expression_type_checking.md` needs its checker fed from parsed
expressions. A slot that stores an opaque string has nothing to feed it: today
`dynamicclasses` cannot be type-checked at all, because by the time it reaches
the executor it is a string that was never parsed. Converting a slot to the
first-class form is therefore the precondition for checking it, and the two
proposals compose rather than compete.

## 5. Open question

**Whitespace inside string literals is not safe to normalise, and at least one
code path did.** Folding a multi-line constraint with `strings.Fields` collapses
runs of whitespace *inside* quoted literals too, so `'two  spaces'` silently
becomes `'two spaces'` — a change to the value being matched on, in a place
nobody would look. Found and fixed in the offline-sync describer, where the fold
is now quote-aware.

Whether any other expression or XPath path folds or normalises whitespace
without tracking quote state is **unaudited**, and it is the kind of defect that
leaves no trace: the document stays valid and the build stays green.
