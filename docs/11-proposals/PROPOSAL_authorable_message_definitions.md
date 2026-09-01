---
title: Authorable message definitions — the last mapping source MDL cannot write
status: draft
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

Collections are small: 28 of 36 hold a single definition, 4 hold two, 3 hold
four, 1 holds eight.

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

| statement | notes |
|---|---|
| `CREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION M.Name ( ... )` | `OR MODIFY` preserves the UUID — mappings reference the collection by qualified name, and a fresh document would break every `WITH MESSAGE DEFINITION` |
| `DROP MESSAGE DEFINITION COLLECTION M.Name` | refuse when a mapping still references it, naming the mappings |
| `DESCRIBE MESSAGE DEFINITION COLLECTION M.Name` | re-executable output; this is what makes describe → rename → exec the copy operation |
| `SHOW MESSAGE DEFINITION COLLECTIONS [IN Module]` | there is no listing today at all |

`FOLDER 'path'` on create, as every other document type takes.

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

**In:** the four statements above; attributes, associations in both directions,
nested definitions to arbitrary depth; `OR MODIFY`; folder placement; the
`describe` round trip; a check-time rule for members that do not exist on the
entity (the shape `MDL-JSON01` established).

**Out:**

- **Inherited attributes.** An entity mapped with `EXTENDS` can expose its
  parent's attributes; whether the corpus contains any has not been measured.
  Determine before implementing, and refuse rather than guess.
- **`Documentation` / `Example` / `ErrorMessage` / `WarningMessage`.** Empty in
  4,686 of 4,686. Add them when a document needs them, not before.
- **Published message definitions** (the Business Events surface). A different
  document.

## Implementation outline

Full-stack, per the CLAUDE.md checklist:

1. **Grammar** — `createMessageDefinitionCollectionStatement`, plus `drop` /
   `describe` / `show`. New keyword: none required; `MESSAGE` and `DEFINITION`
   already exist (`WITH MESSAGE DEFINITION`), and `COLLECTION` is used by image
   and icon collections.
2. **AST** — `CreateMessageDefinitionCollectionStmt`, with a member tree that
   distinguishes attribute from association by the presence of a qualified name.
3. **Visitor** — bridge, as usual.
4. **Backend** — `CreateMessageDefinitionCollection` /
   `UpdateMessageDefinitionCollection` / `DeleteMessageDefinitionCollection` on
   `MappingBackend`, beside the existing `ListMessageDefinitionCollections`;
   implemented in both engines and stubbed in the mock.
5. **Executor** — thin handler; the element tree is built from the domain model,
   which is where `MaxOccurs` and `PrimitiveType` are resolved.
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

## A note on measuring this document type

The census figures above were read through `mprbson.units()` / the reader, not
by grepping the extracted project tree. Grep gives false negatives here: it can
only see units that are *files*, which is MPR v2 only. An MPR v1 project keeps
its units in the SQLite `Unit.Contents` blob, where the type string is not
greppable — which is exactly how an earlier count of XML schemas came back as
zero when the answer was three
([#259](https://github.com/ako/mxcli/issues/259) follow-up).
