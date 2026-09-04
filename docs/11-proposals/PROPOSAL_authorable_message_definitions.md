---
title: Authorable message definitions — the last mapping source MDL cannot write
status: implemented
date: 2026-09-01
related:
  - PROPOSAL_mapping_coverage.md
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
  - docs/13-decisions/0005-semantic-model-interface-currency.md
  - .claude/skills/mendix/json-structures-and-mappings/SKILL.md
---

# Authorable message definitions — the last mapping source MDL cannot write

## Problem Statement

A mapping is bound to one of four schema sources. MDL can create exactly one of
them.

| source | mappings in the corpus | authorable in MDL |
|--------|------------------------|-------------------|
| JSON structure | 250 (76.5%) | yes |
| **message definition** | **74 (22.6%)** | **no — read-only** |
| XML schema | 3 (0.9%) | no |
| imported web service (SOAP) | 0 in the corpus | no — and now [refused rather than dropped](https://github.com/ako/mxcli/issues/365) |

Measured across all 327 import/export mappings in the nine demo apps.

So a project built entirely through MDL can express three-quarters of the
mappings a real app has. The remaining quarter needs a document mxcli can read
and map over, but only a human in Studio Pro can create. `CREATE IMPORT MAPPING
… WITH MESSAGE DEFINITION` already works; there is simply no way to bring the
definition into existence.

This is the last of the four that is both **worth doing** and **doable**:

- **XML schema** holds an imported `.xsd` (`FilePath='C:\Users\…\Response.xsd'`).
  Authoring it means parsing XSD — imports, includes, complexTypes, substitution
  groups — for 0.9% of mappings.
- **Imported web service** holds a WSDL, with its schema entries inline. Same
  problem, larger.
- **Message definition** holds *nothing external*. It is a selection over the
  domain model. Every element names an entity, an attribute or an association.

## What the document actually is

Measured over all **36 collections / 56 definitions / 4,686 elements** in the
corpus, read through the unit table (not by grepping — see the note at the end).

Four element types, and every property present on every instance. There are no
optional keys to guess at:

```
MessageDefinitions$MessageDefinitionCollection   36     Name, Documentation, Excluded, ExportLevel, MessageDefinitions
  MessageDefinitions$EntityMessageDefinition     56     Name, Documentation, ExposedEntity
    MessageDefinitions$ExposedEntity             56     Entity, ExposedName, ExposedItemName, OriginalName, Path,
                                                        ElementType, MinOccurs, MaxOccurs, Nillable, PrimitiveType,
                                                        MaxLength, FractionDigits, TotalDigits, IsDefaultType,
                                                        Example, ErrorMessage, WarningMessage, Documentation, Children
      MessageDefinitions$ExposedAttribute        3697   (same, plus Attribute)
      MessageDefinitions$ExposedAssociation      933    (same, plus Association + Entity)
```

Collections are small in one dimension and not at all in the other. 28 of 36
hold a single definition, 4 hold two, 3 hold four, 1 holds eight — but the
definitions themselves are **deep**:

| nesting depth | elements | |
|---|---|---|
| 0 (the definition root) | 56 | |
| 1 | 356 | |
| 2 | 265 | |
| 3 | 142 | |
| 4 | 208 | |
| 5 | 477 | |
| 6 | 974 | |
| **7** | **2,208** | the maximum observed |

Object elements carry a mean of 4.7 members, and the tail is long — 93 have 15
members, one has 22.

That shape decides more of this proposal than anything else. A definition is not
a flat field list you would happily restate; a whole-document `CREATE OR MODIFY`
is a poor tool for "expose one more attribute", and any addressing scheme for
targeted edits has to reach seven levels down.

### Almost every property is a constant or is derived

Across all 4,686 elements, with no exceptions:

| property | value | |
|---|---|---|
| `MinOccurs` | `0` | constant |
| `Nillable` | `true` | constant |
| `IsDefaultType` | `false` | constant |
| `MaxLength`, `FractionDigits`, `TotalDigits` | `-1` | constant |
| `Example`, `ErrorMessage`, `WarningMessage`, `Documentation` | `""` | constant |
| `ElementType` | `Object` / `Value` | derived from the element kind |
| `PrimitiveType` | `Unknown` for objects, the attribute's type for values | derived |
| `OriginalName` | the entity / attribute / association's own name | derived |
| `Path` | the position in the tree (`Reference\|Content`) | derived |
| `MaxOccurs` | `1` or `-1` | derived — **see below** |
| `ExposedItemName` | `OriginalName` when `MaxOccurs = -1`, `""` otherwise | derived, 461/461 |

That leaves exactly four things an author chooses: the collection's name, each
definition's name, which entity it roots at, and which members to expose — plus
an optional rename per element.

### The one derivation that is not obvious

`MaxOccurs` on an exposed association is **not** a function of the association's
type. All 927 resolvable associations in the corpus are `Reference`, yet 526
carry `MaxOccurs = 1` and 401 carry `-1`.

It tracks the **direction of traversal**, with zero counter-examples:

| the element's holder is | traversal | `MaxOccurs` | count |
|---|---|---|---|
| the association's **FROM** entity | child → parent, following the FK | `1` | 496 |
| the association's **TO** entity | parent → children, in reverse | `-1` | 401 |

(30 further elements are cross-module and were not resolved by the census
script; all carry `MaxOccurs = 1`, consistent with FROM.)

This is the `ParentPointer` / `ChildPointer` inversion from CLAUDE.md wearing a
different hat, and it is the one place a plausible implementation goes silently
wrong: get it backwards and the definition exposes a list as a single object, or
a single object as a list. A build error is not guaranteed — the mapping over it
simply carries the wrong cardinality.

> **Correction (ako/mxcli-rest FINDINGS #60).** The heading above is right that
> `MaxOccurs` is not a function of the type *alone*; it is a function of the
> direction **and** the type. The census could not see the second half: every
> association in it is a `Reference`, so the corpus never varied the input the
> rule was declared independent of. A **`ReferenceSet` is a list in both
> directions**, and unlike the direction half this one does have a build error
> behind it — mxbuild reports CE6524 on the definition and CE0295 on any mapping
> element bound to it. Shipped rule:
>
> |                  | forward (holder is FROM) | reverse |
> |---|---|---|
> | `Reference`      | `1`  | `-1` |
> | `ReferenceSet`   | `-1` | `-1` |

**Design consequence: the statement names the target entity**, so direction is
explicit in the source text rather than inferred:

```sql
Sales.Order_Line/Sales.Line as 'Lines' ( ... )
```

This is also the shape import and export mappings already use for a nested
object (`Assoc/Module.Child`), so it is not a new idea to learn.

## Proposed syntax

```sql
create message definition collection Sales.MD_Order (
  definition Order for Sales.Order as 'Orders' (
    OrderId,
    OrderDate,
    Total,
    Sales.Order_Line/Sales.Line as 'Lines' (
      Sku,
      Quantity,
      Price
    ),
    Sales.Order_Customer/Sales.Customer (
      Name,
      Email
    )
  )
);
```

Following [ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md) and
`design-mdl-syntax`:

- `create` / `drop` / `describe` / `show`, not a custom verb.
- Qualified names everywhere; no implicit module.
- `as 'Name'` for a name-to-name mapping, matching `CUSTOM NAME MAP` and
  `ALTER ENTITY … RENAME … AS …`. (`:` is for property values; this is a rename.)
- A bare identifier is an **attribute**; a qualified `Assoc/Module.Entity` is an
  **association**. Same discriminator import and export mappings use.
- One member per line, trailing comma allowed, so adding a field is a one-line
  diff.

### Statement set

**Whole document:**

| statement | notes |
|---|---|
| `CREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION M.Name ( ... )` | `OR MODIFY` preserves the UUID — mappings reference the collection by qualified name, and a fresh document would break every `WITH MESSAGE DEFINITION` |
| `DROP MESSAGE DEFINITION COLLECTION M.Name` | refuse when a mapping still references it, naming the mappings |
| `DESCRIBE MESSAGE DEFINITION COLLECTION M.Name` | re-executable output; this is what makes describe → rename → exec the copy operation |
| `SHOW MESSAGE DEFINITION COLLECTIONS [IN Module]` | there is no listing today at all |

`FOLDER 'path'` on create, as every other document type takes.

**Targeted edits.** `CREATE OR MODIFY` alone is not enough, for the reason the
depth table shows: adding one attribute to a definition seven levels deep would
mean restating the whole document, which is precisely the diff-unfriendliness
[ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md) argues against, and is why
`ALTER ENTITY ADD ATTRIBUTE` exists rather than only `CREATE OR MODIFY ENTITY`.

Definitions within a collection — this is the 8 collections that hold more than
one:

```sql
alter message definition collection Sales.MD_Order
  add definition Line for Sales.Line as 'Lines' ( Sku, Quantity );

alter message definition collection Sales.MD_Order drop definition Line;
alter message definition collection Sales.MD_Order rename definition Line to OrderLine;
```

Members within a definition — this applies to all 36:

```sql
alter message definition Sales.MD_Order.Order add member Total;
alter message definition Sales.MD_Order.Order
  add member Sales.Order_Line/Sales.Line as 'Lines' ( Sku, Quantity );

alter message definition Sales.MD_Order.Order drop member Total;
alter message definition Sales.MD_Order.Order set member Total as 'GrandTotal';
```

Three deliberate choices here:

- **The definition is addressed as `Module.Collection.Definition`** — the same
  three-part reference `WITH MESSAGE DEFINITION` already takes. Nothing new to
  learn, and the two cannot drift apart.
- **`SET member … AS`, not `RENAME member … TO`.** `ALTER ENTITY RENAME
  ATTRIBUTE` renames the attribute *in the model* and rewrites every reference to
  it. This changes only the element's `ExposedName` and touches nothing else, so
  borrowing `RENAME` would promise something far larger than it does. `SET` is
  the established verb for changing a property, and `as` for a name-to-name
  mapping.
- **`IF NOT EXISTS` / `IF EXISTS`** on add and drop, so a definition script
  re-runs cleanly — the same treatment `ALTER ENTITY` gives attributes.

**Reaching a nested member.** Members live up to seven levels down, so the
address needs a path. It is written in **exposed names**, the names the document
itself carries:

```sql
alter message definition Sales.MD_Order.Order
  add member Price in Lines/Prices;

alter message definition Sales.MD_Order.Order
  drop member Sku in Lines;
```

`in <path>` rather than a `/`-joined member name, because `/` already means
"association to entity" inside a member (`Sales.Order_Line/Sales.Line`) and
overloading it in the same clause would be ambiguous to a reader even where the
grammar could tell them apart.

New members append. `describe` emits stored order, so the round trip is stable
either way; a positional form (`before` / `after`) is deliberately not proposed
until something needs it.

## The name of a repeating element, and why not to guess it

Studio Pro **pluralises** `ExposedName` for a repeating element while keeping
`ExposedItemName` at the singular. Across all 461 repeating elements:

| pattern | count | example |
|---|---|---|
| `ExposedName == OriginalName + "s"` | 386 | `Reference` → `References` |
| an English plural | 71 | `Factory` → `Factories` |
| unchanged (the original is already plural) | 4 | `Parts` → `Parts` |
| `ExposedItemName == OriginalName` | **461 / 461** | |

`ExposedItemName` is therefore free — it is always the original name. The plural
is not: reproducing it needs `-y → -ies`, an already-plural detector, and
whatever the next corpus turns up.

**Recommendation: do not implement English inflection.** Default `ExposedName` to
`OriginalName` and make `as 'Orders'` the way to get the plural — the same
conclusion reached for array-item naming in
[#272](https://github.com/ako/mxcli/issues/272), and for the same reason: a rule
fitted to one corpus is wrong on the next, and a name the author writes is
better than a name a heuristic guesses.

**The honest cost**, stated so nobody is surprised: an mxcli-written definition
that omits `as` will differ from a Studio-Pro-written one in `ExposedName` for
every repeating element. That is a describe-diff, not a build error — but it is
real, and it is the argument for making `as` prominent in the docs rather than a
footnote.

An alternative worth considering in review: **require** `as` on a repeating
element. It removes the silent divergence at the cost of some friction, and it
makes the cardinality visible at the point the author writes it.

## Scope

**In:** the statements above — whole-document and targeted; attributes,
associations in both directions, nesting to the depth the corpus shows;
`OR MODIFY`; folder placement; the `describe` round trip; a check-time rule for
members that do not exist on the entity, and for an `in` path that reaches
nothing (the shape `MDL-JSON01` / `MDL-JSON02` established).

**Out:**

- ~~**Inherited attributes.**~~ **Measured and IN scope.** 398 of 3,697 exposed
  attributes are inherited (10.8%) — e.g. `Email_Connector.Attachment` exposing
  `System.FileDocument.FileID`. Far too common to refuse, and the stored
  `Attribute` names the DECLARING entity, which `DeclaringMemberRef` already
  resolves for mappings.
- **`Documentation` / `ErrorMessage` / `WarningMessage`.** Empty in 4,707 of
  4,707. Add them when a document needs them, not before.
- ~~**`Example`.**~~ **In scope after all.** The corpus said empty in 4,686 of
  4,686 — but that corpus is marketplace modules. ako/TestApp's hand-authored
  definition sets one, so it is author-set and rare rather than unused, and
  hardcoding it empty would silently drop the one that exists. `example '...'`
  is now syntax.
- **Published message definitions** (the Business Events surface). A different
  document.
- **A wildcard member** (`definition Order for Sales.Order ( * )` to expose every
  attribute). Studio Pro's UI is a tree of checkboxes, so "tick everything" is a
  natural thing to want, and with a mean of 4.7 members and a tail out to 22 it
  would save real typing. But `*` appears nowhere else in MDL, and no measurement
  here says anyone wants it — proposing it now would be designing for a shape
  with no document behind it. Raised as an open question instead.

## Open questions for review

1. **Require `as` on a repeating element?** See above — it removes the silent
   divergence from Studio Pro at the cost of friction.
2. **A wildcard member.** Worth it, or speculative?
3. **Inherited attributes** — refuse, or block the proposal until measured?
4. **Legacy engine** — author there too, or modelsdk-only like rules, menus and
   layouts?

## What implementation changed

Five things the census had not shown, all found by round-tripping ako/TestApp's
hand-authored collection against its stored bytes:

1. **`Path` is a chain of ORIGINAL names**, not exposed ones, and an
   **association contributes two segments** — its own name, then the target
   entity's: `Order|OrderLine_Order|OrderLine|Amount`. Confirmed afterwards at
   4,707 of 4,707 elements once we knew to look for it.
2. **The typed-array marker is 2**, where the codec defaults to 3.
3. **Every element serializes `Children` even when empty** — a leaf attribute
   stores the bare `[2]`, the same `MandatoryLists` rule as a rule document's
   `Flows`.
4. **`PrimitiveType` is mapped, not passed through**: `Long → Integer`,
   `AutoNumber → Integer`, `Enumeration → String`, everything else identity.
   A pass-through gets 279 corpus elements wrong.
5. **`Example` is author-set** — see the scope note above.

The lesson is worth more than the list: **one hand-authored reference document
is worth more than a large census of marketplace modules.** A module author and
someone building an app by hand exercise different parts of the same document.

## Implementation outline

Full-stack, per the CLAUDE.md checklist:

1. **Grammar** — `createMessageDefinitionCollectionStatement`, plus `drop` /
   `describe` / `show`, and `alterMessageDefinitionCollectionStatement` /
   `alterMessageDefinitionStatement`. New keyword: none required; `MESSAGE` and
   `DEFINITION` already exist (`WITH MESSAGE DEFINITION`), `COLLECTION` is used
   by image and icon collections, and `IN` / `AS` / `SET` / `ADD` / `DROP` /
   `RENAME` are all in the lexer.
2. **AST** — `CreateMessageDefinitionCollectionStmt`, with a member tree that
   distinguishes attribute from association by the presence of a qualified name.
3. **Visitor** — bridge, as usual.
4. **Backend** — `CreateMessageDefinitionCollection` /
   `UpdateMessageDefinitionCollection` / `DeleteMessageDefinitionCollection` on
   `MappingBackend`, beside the existing `ListMessageDefinitionCollections`;
   implemented in both engines and stubbed in the mock.
5. **Executor** — thin handlers. The element tree is built from the domain model,
   which is where `MaxOccurs` and `PrimitiveType` are resolved. The ALTER path
   edits the stored document rather than rebuilding it, so an untouched
   definition is never round-tripped through the describer — the argument that
   made `ALTER LAYOUT` a capability rather than a convenience.
6. **`describe`** — re-executable, deterministic order.

The document is written through the codec on the modelsdk engine. Whether the
legacy engine should author it at all is an open question — recent document
types (rules, menus, layouts) are modelsdk-only and legacy refuses.

## Verification plan

The corpus is the oracle, and it is unusually good here: 36 real documents, all
uniform.

1. **Round-trip fixtures.** Pin three real collections in
   `mdl/executor/testdata/mapping-fixtures/` — one single-definition, one
   multi-definition, one with a reverse-direction association — and assert
   `describe` → re-exec → `canon.Equal`, the methodology used for scheduled
   events and rules.
2. **The cardinality control.** A definition exposing the *same* association in
   both directions, asserting `MaxOccurs` 1 and -1 respectively. Getting this
   backwards is the failure mode with no build error behind it.
3. **mxbuild.** 0 errors on a project holding an mxcli-written collection *and*
   an import mapping bound to it, against a base-project control.
4. **The constants.** One test asserting `MinOccurs`/`Nillable`/`MaxLength`/…
   against the measured values, so a future change that starts varying one of
   them fails loudly rather than drifting.
5. **ALTER leaves the rest alone.** Add one member to a pinned collection and
   assert every *other* element is byte-identical — the control that separates a
   targeted edit from a rebuild that happened to produce the same thing.

## A note on measuring this document type

The census figures above were read through `mprbson.units()` / the reader, not
by grepping the extracted project tree. Grep gives false negatives here: it can
only see units that are *files*, which is MPR v2 only. An MPR v1 project keeps
its units in the SQLite `Unit.Contents` blob, where the type string is not
greppable — which is exactly how an earlier count of XML schemas came back as
zero when the answer was three
([#259](https://github.com/ako/mxcli/issues/259) follow-up).
