---
title: The SOAP request body — the half of a web service call MDL cannot say
status: draft
date: 2026-09-11
related:
  - PROPOSAL_mapping_coverage.md
  - PROPOSAL_first_class_expressions.md
  - https://github.com/ako/TestApp
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - docs/13-decisions/0005-semantic-model-interface-currency.md
---

# The SOAP request body

> Pinned against `ako/TestApp` (Mendix 11.14.0), whose `Clients` module holds
> three Studio Pro-authored SOAP calls covering both request shapes — two with
> operation arguments, one with a send mapping. Every storage claim below is read
> off those documents rather than inferred from the metamodel, and every error
> number was produced by running `mx check` on a project mxcli wrote.

## 1. Problem

`CALL WEB SERVICE` can say who to call and what to do with the answer. It cannot
say **what to send**, and it does not admit that.

Two clauses are involved, and they fail in opposite directions:

**`send mapping` parses, is accepted, and is discarded.** The clause has been in
the grammar since SOAP support landed. Neither engine writes it. Measured — one
statement, `mxcli exec`, then `mx check` on a project whose baseline is 0 errors:

```
create or replace microflow Clients.MxcliSoapSend ($Order : Clients.Order)
begin
  call web service Clients.OrderSoapClient
  operation SaveOrder
  send mapping Clients.SoapOrderExportMapping;
  return;
end;
```

```
Created microflow: Clients.MxcliSoapSend
[error] [CE0369] "Cannot use simple request body, as the operation's body is
        complex" at Call web service activity 'Call web service 'SaveOrder''
```

`SoapOrderExportMapping` appears **zero times** in the written document, on
`MXCLI_ENGINE=modelsdk` and on `legacy` alike. This is the #850 shape exactly: a
clause the parser accepts, the writer ignores, `exec` reports as success, and
mxbuild rejects several minutes later with an error naming neither the clause nor
the statement.

**Operation arguments have no syntax at all**, so the failure is one step
earlier: there is nothing to write. A call to an operation that takes parameters
is refused by mxbuild as **CE0178**, and MDL offers the author no way to fix it.
That was the last unresolved error in the four-round chain recorded in
`.claude/skills/fix-issue/findings/mdl-executor.jsonl` (2026-09-10):

```
StorageLoadException  →  CE0386  →  CE0243 + CE0366  →  CE0178
```

The first three are fixed. CE0178 is this proposal.

The two are one problem, which is the point of proposing them together: **they
are the two branches of a single stored property**, and today a statement can ask
for both and get neither.

### 1.1 Consequence: a SOAP call is currently unreadable as well as unwritable

Related, and worth fixing in the same pass. All three of TestApp's SOAP actions
carry **15 keys**; `webServiceActionRequiresRawBSON` treats **9** as
representable, so anything else forces the opaque fallback. mxcli's own writer
emits the same 15. Measured on both engines:

```
$Orders = call web service raw 'KAYAAAUkSUQAEAAAAACTWjMVYM9yRZNCZQlJkWTA…'
```

Every SOAP call in every project — Studio Pro's and mxcli's — describes as
base64. The structured `call web service …` form the describer can produce is
**unreachable**, which is how five dead resolvers lived in it undetected
(bfb30c7c). Six of the seven unsupported keys are fixed boilerplate mxcli already
writes unconditionally; the seventh is `RequestBodyHandling`, i.e. this proposal.
Landing it is what makes the structured form reachable, and DESCRIBE→exec a real
copy operation for SOAP.

## 2. What Mendix stores

`Microflows$CallWebServiceAction.RequestBodyHandling` is a polymorphic child.
`generated/metamodel` declares six variants (`Advanced`, `Binary`, `Custom`,
`FormData`, `Mapping`, `Simple`); the two a SOAP call uses are below. **They are
alternatives** — a call has one request body, not two.

### 2.1 Arguments — `Microflows$SimpleRequestHandling`

From `Clients.GetOrders`:

```json
{
  "$Type": "Microflows$SimpleRequestHandling",
  "NullValueOption": "LeaveOutElement",
  "ParameterMappings": [2, {
    "$Type": "Microflows$WebServiceOperationSimpleParameterMapping",
    "Argument": "2",
    "IsChecked": true,
    "ParameterName": "",
    "ParameterPath": "http%3A//www.example.com/:GetOrder|OrderId"
  }]
}
```

`Argument` is a **Mendix expression string**, not a literal — `"2"` here because
the reference call passes a constant, but `$Order/OrderId` belongs in the same
field. `ParameterMappings` is a typed array with marker **2**.

`ParameterPath` is the load-bearing one, and it is **derivable**:

```
ParameterPath = escape(operation.RequestBodyElementName) + "|" + parameterName
```

The imported service document carries `RequestBodyElementName` per operation —
`"http://www.example.com/:GetOrder"` on `GetOrder` — in
`Description.Services[].Operations[]`, structured, alongside the `Name` the
CE0386 fix already reads. Escaping is per path segment: `:` becomes `%3A` inside
a segment, `/` is left alone, and the segment separators `:` and `|` are not
escaped. So the author supplies `OrderId` and mxcli builds the rest.

That the path is derivable is what makes a readable syntax possible at all; the
alternative is making users paste `http%3A//www.example.com/:GetOrder|OrderId`
into a script, which fails principle 1 on sight.

**The parameter names themselves are not in the model.** The operation document
stops at the body element name; `OrderId` lives in the WSDL's inline XSD, stored
as raw text in `Description.WsdlContentss[]`. mxcli does not parse WSDL and this
proposal does not add that. Consequence, stated plainly: a misspelled parameter
name cannot be caught by `mxcli check`, and reaches the author as CE0178 from
mxbuild. Section 6 says what could change that later.

### 2.2 Send mapping — `Microflows$MappingRequestHandling`

From `Clients.SaveOrder`:

```json
{
  "$Type": "Microflows$MappingRequestHandling",
  "ContentType": "Json",
  "MappingId": "Clients.SoapOrderExportMapping",
  "MappingVariableName": "NewSaveOrder"
}
```

Three facts, all consequential:

1. **`MappingVariableName` is the missing half of today's clause.** An export
   mapping maps *from an object*, and the call has to say which variable holds
   it. The current `send mapping X` cannot express it — which is a second reason
   the clause could not have been written correctly even if the writer tried.
2. **Both keys are already known-wrong in `modelsdk/gen`.** The key audit lists
   them: `Mapping`→`MappingId` and `MappingArgumentVariableName`→
   `MappingVariableName` (`modelsdk/gen/keyaudit_test.go:168-169`). Writing
   through the gen accessors produces a document mxbuild tolerates and Studio Pro
   cannot open. Two `STORAGE-NAME OVERRIDE` patches in `init<Type>` **and**
   `InitFromRaw`, per CLAUDE.md's two rules, and struck off the audit ledger.
3. **`ContentType` is `"Json"`** on this reference — on a SOAP call, whose
   request is XML, and whose *receive* side writes `"Xml"`. Surprising enough to
   be worth naming as a risk rather than a finding: it is one document, and the
   honest move is to write what Studio Pro wrote and get a second reference
   before treating it as the rule. See §6.

## 3. Proposed syntax

Reuse `callArgumentList` — the named-argument form every other call statement in
MDL already uses (`call microflow`, `call nanoflow`, `call java action`,
`call external action`, `execute database query`). No new spelling for a concept
MDL has spelled five times.

**Arguments attach to `operation`**, because they are the operation's parameters
and `ParameterPath` is built from the operation:

```
$Orders = call web service Clients.OrderSoapClient
  operation GetOrder (OrderId = $Customer/OrderId)
  receive mapping Clients.SoapOrdersImportMapping;
```

**The send mapping gains the variable it maps from**, with `from` — the
preposition MDL already uses for a source:

```
call web service Clients.OrderSoapClient
  operation SaveOrder
  send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;
```

Grammar delta, confined to one rule:

```antlr
callWebServiceStatement
    : (VARIABLE EQUALS)? CALL WEB SERVICE
      (RAW STRING_LITERAL
      | webServiceReference
        (OPERATION webServiceReference (LPAREN callArgumentList? RPAREN)?)?
        (SEND MAPPING webServiceReference (FROM VARIABLE)?)?
        (RECEIVE MAPPING webServiceReference)?
        (TIMEOUT expression)?)
      onErrorClause?
    ;
```

`FROM` and `LPAREN`/`RPAREN` are existing tokens; no lexer change.

Against the design checklist: it reads as English (*call this service, operation
GetOrder with OrderId …*); it adds no verb; both clauses are optional, so every
script that parses today still parses; one argument is a one-line diff; and an
LLM that has seen `call microflow Mod.Flow (Name = $x)` generates this correctly
from the shape alone.

### 3.1 The clauses are mutually exclusive — and that is the rule to enforce

`RequestBodyHandling` holds one variant. So:

| Statement | Written |
|---|---|
| `operation X (a = …)` | `SimpleRequestHandling` with parameter mappings |
| `send mapping M from $v` | `MappingRequestHandling` |
| `operation X` alone | `SimpleRequestHandling`, empty — today's behaviour |
| both | **refused by `mxcli check`** |

The last row is the one worth having. Today both clauses can be written, neither
is stored, and the author learns something is wrong from CE0369 — an error about
a "simple request body" on a statement that asked for a mapping. The refusal
should name both clauses and say that a call sends either arguments or a mapping.

Per the repo's rule, `check` and `exec` must call the **same** function, so a
script cannot pass one and fail the other.

### 3.2 DESCRIBE

Round-trippable, per the layouts precedent — describe → edit → exec is how a SOAP
call gets copied. Once `RequestBodyHandling` is representable, remove it from the
`webServiceActionRequiresRawBSON` unsupported set along with the six boilerplate
keys, and a real call describes as the readable form instead of base64 (§1.1).

An argument whose `ParameterPath` does not decompose into a known operation's
body element name is the honest edge: emit the raw form for that action rather
than a structured line that would lose the path. Unknown-shape → raw, never
unknown-shape → dropped.

## 4. Alternatives considered

**`arguments (…)` as its own clause.** Symmetrical with `send mapping`, and
readable. Rejected: it introduces a second way to spell a call's arguments, when
five statements already use `(Name = value)`. Principle 2.

**Expose `ParameterPath` verbatim.** Honest about storage, and would survive a
WSDL whose parameters mxcli cannot enumerate. Rejected as the primary form: it
puts a percent-encoded URI in a script aimed at business analysts. Worth
reconsidering only as an escape hatch if a real WSDL turns up whose paths are not
`element|parameter` — none of TestApp's three are, but three is not many.

**Write the send mapping now and leave arguments for later.** Tempting, since the
mapping is four keys. Rejected because the mutual exclusivity in §3.1 is only
enforceable once both exist; implementing one alone means `check` can refuse a
combination it cannot yet offer an alternative to.

**Parse the WSDL to validate parameter names.** See §6 — deliberately out of
scope, not rejected.

## 5. Implementation sketch

Roughly the shape the CE0386/CE0243 fixes took, and mostly reusing their parts.

1. **Semantic model** (`sdk/microflows`): `WebServiceCallAction` gains
   `Arguments []WebServiceArgument{Name, Expression}` and `SendMappingVariable
   string`. Per ADR-0005 the backend interface speaks this, not gen or BSON.
2. **Operation lookup** (`mdl/executor/webservice_names.go`): one more reader
   beside `resolveWebServiceName`, returning the chosen operation's
   `RequestBodyElementName`. Same document, same walk, no new backend method —
   and the same discipline: **`""` when it cannot be established, never a guess**,
   since a fabricated path reproduces CE0178 with different text in it.
3. **Writers**: `RequestBodyHandling` stops being an unconditional
   `simpleRequestHandlingToGen()` and branches. Both engines
   (`mdl/backend/modelsdk/microflow_webservice_write.go`,
   `sdk/mpr/writer_microflow_actions.go`), key-for-key in the same order.
4. **`MappingRequestHandling` needs the two storage-name overrides** from §2.2
   before it can be written at all — both sides, `gofmt`, ledger struck.
5. **Reader**: `RequestBodyHandling` → arguments or send mapping. Note the read
   path currently looks for `RequestHandling` → `ExportMappingCall` → `Mapping`,
   a key **no TestApp document carries**, which is why `SendMappingID` was never
   populated from a real project either.
6. **Validation**: §3.1, one function, called by `check` and `exec`.
7. **Version gating**: none expected — SOAP calls predate the supported range —
   but confirm against `sdk/versions/mendix-{9,10,11}.yaml` before merging.

## 6. What this does not settle

Listed because each is a place where the next person will otherwise assume the
question was answered.

- **`ContentType` on the send mapping.** One reference says `"Json"` (§2.2).
  Write that; get a second Studio Pro-authored send mapping before calling it a
  rule. If a second says `"Xml"`, the property is probably following the
  *mapping's* source, not the call's protocol.
- **`ParameterName`** is `""` in both reference parameter mappings, and gen
  declares it beside `ParameterPath`. What fills it is unmeasured — plausibly
  RPC-style bindings, which neither TestApp operation uses. Write `""`.
- **`IsChecked`** is `true` in both. Presumably Studio Pro's per-parameter
  checkbox. There is no observed `false`, so no syntax is proposed for it; a
  stored `false` must be preserved on rewrite rather than normalised to `true`.
- **`Microflows$WebServiceOperationAdvancedParameterMapping`** — a per-parameter
  export mapping, a third shape between §2.1 and §2.2. No reference document.
  Out of scope; a stored one must make the action refuse a rewrite
  (guard-don't-drop), not silently become a simple mapping.
- **Nested parameters.** Both references pass one scalar. A WSDL with a nested
  complex parameter may produce a `ParameterPath` with more segments, which would
  change the derivation in §2.1. Unknown, and the reason §4 keeps the verbatim
  form as a possible escape hatch.
- **Validating parameter names against the WSDL.** Would need the XSD in
  `Description.WsdlContentss[]` parsed. Real value — it turns a CE0178 from
  mxbuild into a `check` error naming the parameter — but it is a separate piece
  of work with its own risk surface, and it is not needed for the write path to
  be correct.

## 7. Proving it

The bar is the one the CE0386 chain set: **measured against a real project, with
a control.**

- Unit, both engines: encode an action with arguments and one with a send
  mapping; assert the key sets and order against §2.1/§2.2 verbatim.
- Unit: `ParameterPath` derivation, including the escaping — a test asserting
  `http%3A//www.example.com/:GetOrder|OrderId` character for character, since a
  plausible-looking wrong escaping is exactly what mxbuild would accept and
  Studio Pro would not.
- Unit: the §3.1 refusal, and that `check` and `exec` reject the same script.
- `mdl-examples/doctype-tests/06b-soap-examples.mdl` extended with both forms.
- **Integration, against ako/TestApp**: rewrite `Clients.GetOrders` and
  `Clients.SaveOrder` from MDL and get **0 errors** from `mx check`. The control
  is the pair of numbers this proposal opened with — the same two statements
  today give CE0178 and CE0369. A test that only passes against fixed code has
  not been shown to detect anything.
- DESCRIBE round trip: describe both, re-exec into a copy, `mx check` clean —
  which also demonstrates §1.1, since neither describes as `raw` any more.
