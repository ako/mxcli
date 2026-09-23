---
title: First-class expressions for expression-typed MDL properties
status: draft
date: 2026-09-08
revised: 2026-09-23
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
in `mdl/backend/pagemutator/mutator.go` (`case "dynamicclasses"` in
`setRawWidgetPropertyMut`) as an ordinary string property; it
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

  An undelimited expression has to end somewhere, and inside a
  `(key: value, …)` property list that means the expression grammar must not
  consume the `,` or `)`. *(Revised 2026-09-23 after reading the grammar.)* This
  is already solved in the same property list, twice: `dataSourceExprV3`
  embeds a bare `expression` after `where` and is followed by `, sort by …` or
  the next property, and `propertyValueV3`'s array alternative is
  `LBRACKET expression (COMMA expression)* RBRACKET`. `expression` has no
  top-level `COMMA` or `RPAREN` production — commas occur only inside
  `LPAREN … RPAREN` of a call — so it stops at the property separator by
  construction. The remaining cost is ambiguity, not termination: a bare
  expression overlaps the other value forms (`'text'`, `42`, `true`,
  `Mod.Enum.Value` are all valid expressions). §6.2 resolves that by giving the
  expression alternative only to the named slots, never to the generic
  `IDENTIFIER COLON propertyValueV3`.

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

*(Revised 2026-09-23 against the grammar; the original list named two slots
that are not expression slots and missed three that are.)*

Ship in family order, XPath first because it is additive. The full slot
inventory, with the rule each lives in today, is §6.1.

0. **Close the silent drop first** (§6.2, slice 0). The syntax #750 proposes,
   `dynamicclasses: [ … ]`, is *already accepted* and throws the value away.
   This is a bug fix, not part of the feature, and should not wait for it.
1. **XPath family** — `targeting users|groups xpath` (#1006's slot). `sync …
   where` is done.
2. **Expression family, widget slots** — `DynamicClasses`, `DynamicCellClass`,
   and every pluggable-widget property whose schema type is `expression`.
   Highest-value by the count in §1, and the ones #750 names.
3. **Expression family, other statements** — page/snippet variable defaults,
   workflow `due date`, `ALTER PAGE SET DynamicClasses = …`.

Two of #750's targets are **not** expression slots and drop out:

- **Calculated attributes.** MDL writes them `calculated by Mod.Microflow`
  (`MDLDomainModel.g4`, `attributeConstraint`) — Mendix computes the value with
  a microflow, and no expression is stored. Attribute `default` already takes
  `literal | expression`.
- **REST/OData "filter and mapping expressions".** No such slot exists in
  `MDLService.g4`: REST `Path:` / `Body: template` are `{param}` text templates
  with their own escaping, and OData/REST mappings bind attributes by name. If a
  real expression slot turns up there it joins slice 3; nothing is designed for
  it speculatively.

## 4. What this unlocks

`PROPOSAL_expression_type_checking.md` needs its checker fed from parsed
expressions. A slot that stores an opaque string has nothing to feed it: today
`dynamicclasses` cannot be type-checked at all, because by the time it reaches
the executor it is a string that was never parsed. Converting a slot to the
first-class form is therefore the precondition for checking it, and the two
proposals compose rather than compete.

## 5. Open questions

1. **Whitespace inside string literals is not safe to normalise, and at least
   one code path did.** Folding a multi-line constraint with `strings.Fields`
   collapses runs of whitespace *inside* quoted literals too, so
   `'two  spaces'` silently becomes `'two spaces'` — a change to the value being
   matched on, in a place nobody would look. Found and fixed in the offline-sync
   describer, where the fold is now quote-aware. Whether any other expression or
   XPath path folds or normalises whitespace without tracking quote state is
   **unaudited**, and it is the kind of defect that leaves no trace: the document
   stays valid and the build stays green.

   A sibling found while writing §6: `buildPropertyValueV3`'s array branch and
   `buildMicroflowArgV3` both use ANTLR `expr.GetText()`, which concatenates
   tokens *without* the hidden-channel whitespace — `if $x then 'a' else ''`
   becomes `if$xthen'a'else''`. Literals survive (one token each); keywords and
   operators fuse. Every new slot must go through `buildExpression` →
   `expressionToString`, never `GetText()`. Whether the microflow-argument path
   is live-broken for `if`-expressions is untested.

2. **Does `expressionToString` round-trip Studio Pro's spelling?** A stored
   `if $currentObject/Featured then 'x' else ''` re-emitted through
   parse → `expressionToString` may differ in whitespace, parenthesisation or
   keyword case. That is harmless to Mendix, but it is a *write* on the next
   `exec` of unchanged `describe` output, which ADR-0008's idempotence rule
   treats as churn. Needs one measurement per slot (§7, T4) before slice 2
   merges; if it churns, store the source text of the parsed span
   (the token-stream text between `GetStart()` and `GetStop()`, which keeps
   whitespace) rather than the re-rendered AST.

3. **`describe` fallback when a stored value does not parse.** A project can
   hold an expression our grammar does not accept (a newer function, a
   Studio-Pro-only construct). `describe` must then emit the quoted form rather
   than bare text that its own output cannot re-parse. Proposed rule: emit bare
   only if the stored string parses as `expression` *and* re-renders to itself;
   otherwise quote. Confirm this output variance is acceptable.

4. **Pluggable expression properties are resolved by schema, not by name.**
   Slice 2 either (a) lets any generic property take a bare expression and
   rejects it at check time when the widget schema says the slot is not
   `expression`-typed, or (b) adds the bare form only for the named properties
   and leaves pluggables quoted. (a) is recommended — it follows the precedent
   of the datasource/action generic branches (#956), where the grammar admits
   the form and the executor decides by the declared kind — but it is the one
   place this change widens a generic rule, so it wants a maintainer decision.

## 6. Implementation plan

### 6.1 Slot inventory

| Slot | Family | Grammar today | Describer today |
|---|---|---|---|
| widget `DynamicClasses:` | expression | generic `IDENTIFIER COLON propertyValueV3` (`MDLPage.g4`, `widgetPropertyV3`) | `cmd_pages_describe_output.go` — `mdlQuote(w.DynamicClasses)` |
| datagrid column `DynamicCellClass:` | expression | generic, aliased to schema key `columnClass` (`mdl/types/widget_item_aliases.go`) | `cmd_pages_describe_output.go` — `mdlQuote(col.DynamicCellClass)` |
| pluggable property of schema type `expression` | expression | generic | `cmd_pages_describe_pluggable.go` |
| `ALTER PAGE SET DynamicClasses = … ON w` | expression | `alterPageAssignment` (`MDLParser.g4`) | n/a |
| page/snippet `Variables: { $v: T = '…' }` | expression | `variableDeclaration: VARIABLE COLON dataType EQUALS STRING_LITERAL` | `cmd_pages_describe.go` — `mdlQuote(defaultVal)` |
| workflow / user-task `due date '…'` | expression | `DUE DATE_TYPE STRING_LITERAL` (`MDLWorkflow.g4`) | `cmd_workflows.go` — `mdlQuoted(DueDate)` |
| user task `targeting users/groups xpath '…'` | XPath | `TARGETING … XPATH STRING_LITERAL` | `cmd_workflows.go` — `mdlQuoted(us.XPath)` |
| offline `sync … where` | XPath | `WHERE (xpathConstraint \| STRING_LITERAL)` | **done** |
| widget `Visible:` / `Editable:` | XPath-shaped | `xpathConstraint` | **done** |

Explicitly not yet in: workflow `timer '…'`, `decide by veto '…'`,
`fallback '…'` and `description '…'`. Verify the stored kind of each against the
reflection data before adding one — a timer delay and a veto outcome are not
obviously expressions, and adding a slot on a guess is how #750's target list
went wrong.

### 6.2 Slices

Each slice is one PR (CLAUDE.md "Scope & atomicity").

**Slice 0 — bug: `[ … ]` on an expression slot is accepted and dropped.**

Measured on `d34c6803`:

```mdl
container c1 (dynamicclasses: [ if $currentObject/Featured then 'is-featured' else '' ]) { }
```

`mxcli check` → `Syntax OK … Check passed!`. The value parses as
`propertyValueV3`'s array alternative, `buildPropertyValueV3` returns a
`[]string`, and both readers discard it: `WidgetV3.GetDynamicClasses` →
`GetStringProp` returns only a `string`, and `datagrid_column.go` (`columnClass`)
guards with `if sv, isStr := v.(string)`. By reading, the widget is written with
no dynamic class — the #999 shape (checks clean, exec succeeds, value vanishes).
Confirm on a project with `exec` + `describe` before fixing.

Fix: a check-time violation (next free `MDL-WIDGET` id) when an expression-typed
property (`expressionWidgetProps` in `validate_widgets.go`, plus
`DynamicCellClass`) holds a non-string value, naming the fix. Test first in
`mdl/executor/`; prove it by reverting the check. Append a finding to
`.claude/skills/fix-issue/findings/<pages area>.jsonl`.

**Slice 1 — XPath family: `targeting … xpath [ … ]`.**

| File | Change |
|---|---|
| `mdl/grammar/domains/MDLWorkflow.g4` | `TARGETING (USERS \| GROUPS)? XPATH (xpathConstraint \| STRING_LITERAL)` |
| `mdl/visitor/` (workflow visitor) | bracket branch → `normalizeXPathTokens(buildXPathString(…))`, as `sync … where` does |
| `mdl/executor/cmd_workflows.go` | emit `xpath [ … ]` instead of `mdlQuoted(us.XPath)`, users and groups |
| `mdl-examples/doctype-tests/24-workflow-examples.mdl` | bracket form beside the quoted form |

**Slice 2 — expression family, widget slots.**

| File | Change |
|---|---|
| `mdl/grammar/domains/MDLPage.g4` | `widgetPropertyV3`: add `(IDENTIFIER \| keyword) COLON expression` **after** every existing generic branch, so `'text'`, numbers, booleans, qualified names and `[ … ]` keep their current parse and only what those reject (`if …`, `$v/Attr + …`, calls) reaches it. `make grammar`; watch for new ambiguity reports. |
| `mdl/visitor/visitor_page_v3.go` | new branch storing an `ast.ExpressionValue` built with `buildExpression(ctx)` under the property name — never `GetText()` (§5.1) |
| `mdl/ast/ast_page_v3.go` | `ExpressionValue` type; `GetStringProp` renders it, so every existing reader (`GetDynamicClasses`, column props) gets the same string it gets today |
| `mdl/executor/cmd_microflows_helpers.go` | move `expressionToString` to where `mdl/ast` can use it without importing `executor` |
| `mdl/backend/widgetobj/datagrid_column.go` | accept `ExpressionValue` for `DynamicCellClass` |
| `mdl/executor/validate_widgets.go` | bare expression on a property whose resolved schema type is not `expression` → violation (§5.4 option a) |
| `mdl/executor/cmd_pages_describe_output.go`, `cmd_pages_describe_pluggable.go` | emit bare when §5.3's rule allows, else `mdlQuote` |
| `.claude/skills/mendix/` (page, datagrid, styling skills) | rewrite the §1 quote runs in the bare form; `make sync-skills` |

**Slice 3 — expression family, other statements.**

| File | Change |
|---|---|
| `mdl/grammar/domains/MDLPage.g4` | `variableDeclaration: VARIABLE COLON dataType EQUALS (STRING_LITERAL \| expression)` — a lone `STRING_LITERAL` keeps its legacy meaning (§6.3) |
| `mdl/grammar/domains/MDLWorkflow.g4` | `DUE DATE_TYPE (STRING_LITERAL \| expression)` in workflow and user-task clauses |
| `mdl/grammar/MDLParser.g4` | `alterPageAssignment`: expression alternative, validated against the property's type |
| matching visitors, `cmd_pages_describe.go`, `cmd_workflows.go` | as slice 2 |

### 6.3 The one semantic trap: a quoted value keeps meaning "expression text"

For every slot above, `'…'` today means "the Mendix expression, quoted". After
the change a `STRING_LITERAL` in these slots still means that — not a Mendix
string literal:

```mdl
dynamicclasses: 'if $x then ''a'' else '''''   -- quoted expression (legacy)
dynamicclasses: if $x then 'a' else ''          -- bare expression (new), same bytes
dynamicclasses: 'a'                             -- quoted expression: the text a (!)
```

The last stores `a` before and after this change, which Mendix reads as an identifier, not the class `a`.
That is today's behaviour; the plan does not make it worse, and the
skills showing the bare form is what steers people off it. Recorded here so no
reviewer "fixes" the grammar by making a `STRING_LITERAL` a string value — that
would silently re-interpret every existing script. The ordering in slice 2
(`propertyValueV3` before `expression`) is what enforces it.

## 7. Test plan

Every test follows CLAUDE.md "Working Rules": written first, proven by reverting
the fix, and any "nothing changed" assertion carries a control.

| # | Layer | Test |
|---|---|---|
| T1 | parser | `mdl/visitor` table test per slot: bare form parses to `ExpressionValue`; `'text'`, `42`, `true`, `Mod.E.V`, `[a, b]` keep their current AST shape (guards slice 2's ordering) |
| T2 | parser | `(dynamicclasses: if $a then 'x' else '', class: 'c')` — the expression stops at `,` and `class` is still its own property |
| T3 | encoding | per slot, quoted and bare forms produce byte-identical stored strings (the §3.2 guarantee); control: a different expression does not |
| T4 | idempotence | `describe` → `exec` → `describe` is stable and the second `exec` writes nothing (`canon.Reconcile` elides); control: an edited expression does write. Settles §5.2 |
| T5 | describe | a stored value that does not parse is emitted quoted, and that output re-parses (§5.3) |
| T6 | check | slice 0: `[ … ]` on `DynamicClasses` / `DynamicCellClass` is a violation; control: the quoted expression on the same widget is clean |
| T7 | integration (`-tags integration`) | `03-page-examples.mdl`, `24-workflow-examples.mdl` gain bare-form cases; `mxcli docker check` passes with no CE0117 |
| T8 | runtime (`.claude/skills/verify-in-runtime.md`) | a page with a bare `dynamicclasses` renders the class on a featured object and not on another — the only test that proves the value reached the client |

## 8. BSON and version compatibility

**No BSON change.** Every slot already stores its expression as a string
(`Forms$Appearance.DynamicClasses`, the pluggable property's `Expression` field
built in `widgetobj/builder.go`, the page variable default, the workflow
`DueDate`, the user task's XPath). The feature changes only how MDL spells that
string and how `describe` prints it; the bytes written are identical by T3. No
new `$Type`, no storage-name question, no `canon.identityFields` row, and no
version gate: nothing about the encoding moves.

## 9. Acceptance

- Slice 0 merged independently, as a bug fix.
- For each converted slot: T1–T5 green; `describe` emits the bare/bracket form;
  the quoted form still parses and stores the same bytes.
- `make build && make test && make lint` pass; the §1 count of four-plus quote
  runs in the skills is re-run and is zero for converted slots.
