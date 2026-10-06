# ADR-0013: Microflow activity settings: words for the statement, one property list for the dialog

- **Status**: Proposed (direction approved by the maintainer; first applied to `call rest service`)
- **Date**: 2026-10-06
- **Related**: refines R2 and R3 of [ADR-0010](0010-mdl-canonical-syntax-rules.md); migrates through [ADR-0011](0011-mdl-language-versioning.md) aliases (MDL-DEPR720); [PROPOSAL_mdl_beta_syntax_freeze.md](../11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md) "HTTP concepts"

## Context

ADR-0010's R2 says `( Key: value, … )` holds the properties of document headers and of `{ }` children, and `begin … end` holds imperative flow. It says nothing about a microflow or nanoflow **activity**, which is a statement inside the flow and also an object with a properties dialog in Studio Pro. Every activity therefore grew keyword clauses for its settings, one grammar rule per setting:

```sql
$Html = call rest service get 'https://example.com'
    header 'Accept' = 'text/html'
    auth basic $User password $Password
    timeout 300
    returns String;
```

That worked while activities had one or two settings. It does not scale, and it has costs that grow with every new activity:

- **Two vocabularies for one concept.** A consumed REST service document already says `Headers: ('Accept': 'text/html')`, `Authentication: basic (Username: …, Password: …)` and `Timeout: 300`. The activity says `header 'Accept' = 'text/html'`, `auth basic … password …` and `timeout 300` for the same three settings. An author, human or LLM, must learn both, and `=` in the header clause contradicts R3 (`:` sets a model property).
- **Clauses are positional and order-sensitive.** The grammar fixes their order, so `timeout 30 header …` is a parse error that reads like a typo, and every new setting is a new keyword in the lexer.
- **Clauses are not strict.** R11 makes an unknown property key an error with a "did you mean" hint; there is no equivalent for a misspelt clause keyword, which surfaces as a generic parse error.
- **New activities are coming.** `send email` will have recipients, subject, body, attachments and more. Written as clauses it would add half a dozen keywords; another activity would add its own.

The proposal's own target for the REST activity is already a property list (`$Html = call rest service get '…' ( Headers: (…), Timeout: 300, ) returns string;`), and its "HTTP concepts" item asks for `Headers`, `Authentication` and `Timeout` "everywhere". The documents got them; the activity did not, because no rule said it should.

## Decision

A microflow or nanoflow activity is written as **words for what the statement does, and one property list for what its dialog sets**:

```
[$x =] <verb> <main operand> [( Key: value, … )] [returns …] [on error …];
```

- **Words.** The verb, the result variable (`$x =`), the activity's main operand (a REST call's method and URL, the called document, the object or list acted on), `returns …` and `on error …` stay words. They are the statement's meaning and its data flow, and they read as a sentence.
- **Property list.** The activity's dialog settings — what a Studio Pro user sets in the activity's properties dialog, such as headers, authentication, timeout, proxy and request body, and for email the recipients, subject and body — go in **one** `( Key: value, … )` list directly after the main operand. The list follows R2 (properties in `( )`), R3 (`:` sets a setting, `=` binds a runtime value, e.g. in `with ({1} = $x)`) and R11 (trailing commas allowed, unknown or repeated keys and misshapen values are errors).
- **Threshold.** An activity with one or two settings that read naturally as words may keep its clauses (`log info node 'App' …`, `show message '…' type Warning`, `download file $F show in browser`). **Every new activity, and every activity with several settings, uses the list.**
- **Key names.** A key reuses the name a document property already has for the same concept: `Headers`, `Authentication`, `Timeout`, `Body`. Values reuse the documents' value shapes: a header map is `('Name': expr, …)`, credentials are `basic (Username: expr, Password: expr)`, a template is `template '…' [with ({1} = …)]`.
- **Migration.** An existing activity that moves to the list keeps its clauses as a deprecated alias (ADR-0011) with the same meaning, rewritten by `mxcli fmt --upgrade`. Mixing the two forms in one statement is an error. `describe` emits only the list.

## Consequences

**Positive**

- One spelling per concept across documents and activities: a header is `'Name': value` in a consumed REST operation and in a REST call, and a fragment can be copied between them.
- Adding a setting to an activity is a new key, validated by the visitor, not a new keyword and grammar rule; a misspelt key gets R11's error with a suggestion.
- Order stops mattering inside the list, and every setting is optional by construction.
- New activities have a template to follow: `send email (To: …, Subject: …, Body: …)` needs no design discussion about clause words.
- The words that remain carry the statement's data flow, so a reader scanning a flow still sees what each activity does and where its result goes.

**Negative**

- **Two styles coexist.** Small activities keep clauses below the threshold, so an author still meets both shapes and must know which applies. The threshold is a judgement ("reads naturally as words"), not a mechanical rule, and will need review when an activity grows a third setting.
- **Migration effort.** Each migrated activity needs a grammar alternative, a visitor branch that builds the same AST, a deprecation entry with a structural `fmt --upgrade` rewrite, a describe change, and updated skills, docs and examples. `describe` output changes for every flow that uses the activity, which users who commit describe output will see as one diff.
- The grammar grows while both forms are accepted; the clause alternatives can only be removed under `mdl 2`.
- A setting's value can be any expression, so key/value-shape validation lives in the visitor rather than the grammar; the grammar accepts the union of value shapes and the visitor rejects the wrong one for a key.

**Neutral**

- R2 and R3 are unchanged; this ADR states how they apply to activities, which they did not cover. ADR-0010 is not superseded.
- `call rest service` is the first activity migrated (MDL-DEPR720). Others with several settings, such as `call web service` (operation, send/receive mapping, timeout), are candidates under the same rule when they are next touched.

## Alternatives considered

- **All clauses** (today's form, extended to every new activity). Rejected: it keeps two vocabularies for the same concepts, contradicts R3 in the header clause, adds a keyword per setting, and gives no strictness for misspelt settings. An activity like `send email` would need many new keywords.
- **All property lists** — every activity, including `log` and `show message`, puts every setting in a list, perhaps even the main operand (`log ( Level: info, Node: 'App', Message: '…' )`). Rejected: it makes the commonest, smallest statements noisier without removing any ambiguity, hides the data flow inside a list, and would churn every existing script for no gain in expressiveness. The threshold keeps the list where it pays.
- **A list per concern** (`headers (…) auth (…)`, one bracket per setting group). Rejected: it is clauses with brackets — still a keyword per setting and still positional — and it does not match the documents' single property list.
