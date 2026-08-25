---
title: Import/export mapping coverage — what real mappings use, and the MDL to express it
status: draft
date: 2026-08-24
---

# Proposal: closing the import/export mapping gap

**Status:** Draft
**Date:** 2026-08-24
**Measured against:** 327 mapping documents in 8 demo/marketplace apps
(`mx-test-projects/*.mpk`), Mendix 10.24–11.x

## 1. Summary

The mapping gaps tracked today (#248 array roots, #253 unbuildable
`find`/`find or create`, plus the unfiled "schema sources are never resolved")
are real, and this proposal does not restate them. What it adds is a **measured
denominator**: how much of the mapping surface real Mendix apps actually use,
and which missing pieces buy the most coverage.

Every `ImportMappings$ImportMapping` and `ExportMappings$ExportMapping` in the
eight demo apps was decoded from BSON and checked against what MDL can express
today:

| | |
|---|---|
| mapping documents | **327** (200 import, 127 export) |
| use **only** constructs MDL can express | **97 (30%)** |
| blocked by exactly one missing construct | 70 |
| blocked by two or more | 160 |

The other 70% is not exotic. It is concentrated in six constructs, and the
biggest single one — an array at the root of the schema, #248 — accounts for
122 documents on its own.

A second, sharper result: **`describe` of a real mapping is unreliable in both
directions.** All 327 were described and the output re-parsed:

- **132** produce MDL that does not parse — loud, recoverable.
- **112** produce MDL that parses cleanly but rebuilds a *different mapping* than
  the one stored — the schema source silently absent, a converter microflow
  gone, an array root flattened to an object root. This is the dangerous class:
  `describe → edit → exec` is currently a lossy round trip that reports success.
- **17** of the failures are mappings that use nothing on the unsupported list
  and *still* fail to parse — pure `describe`/grammar defects (§5).

Only **83 of 327 (25%)** survive `describe` → `check` both parseable and
faithful.

Two writer defects were also found, independent of any syntax gap, where mxcli
writes a value outside the metamodel's enum (§6).

## 2. Method, and how to reproduce it

Each `.mpk` is a zip containing a v1 `.mpr` (a single SQLite file). Every `Unit`
row's `Contents` blob was BSON-decoded and filtered to the two mapping
`$Type`s; the owning module comes from walking `ContainerID` up to
`Projects$ModuleImpl`. Feature classification is on the decoded document, not on
`describe` output, so it is independent of the bugs it is used to measure.

The round-trip measurement is separate: `mxcli describe {import,export} mapping
<qn>` for every mapping, then `mxcli check` on the result.

Both are mechanical and worth landing as a repo script — see §8.

## 3. What real mappings use that MDL cannot express

Counted by **document** (a document using a construct twice counts once).

| construct | docs | tracked as |
|---|---:|---|
| root is an array (`(Array)` / `(Array)\|(Object)`) | 122 | #248 |
| source is a **message definition** | 74 | new — no `with` clause exists |
| object handling **Custom** (a microflow finds the object) | 56 | new |
| value **converter microflow** | 39 | grammar exists, executor drops it |
| **wrapper** element (JSON array of primitives) | 34 | new |
| path through a `(Wrapper)`/`(Value)` marker | 34 | same as above |
| object element with **no entity** (export array container) | 33 | new |
| root is a **nested schema element**, not the document root | 13 | new |
| `find` + **ignore** if not found | 12 | new |
| mapping takes an **input parameter** | 10 | new |
| value element bound to no attribute (parameter feed) | 8 | new |
| `create` + **error** if it already exists | 6 | new |
| source is an **XML schema** | 3 | partially — `with xml schema` parses, nothing creates one |
| XML attribute binding (`IsXmlAttribute`) | 3 | new |
| `ObjectHandlingBackupAllowOverride` | 2 | new |
| `find` + **error** if not found | 1 | new |
| JSON member name that is not an MDL identifier | 5 | new (§5.4) |

### 3.1 Named examples

**Array root — `MendixSSO.AppRolesResponse` (ComposableFactory)**
Root at `(Array)|(Object)`. Reproduced from scratch on a copy of the CitrusGrove
project:

```
create json structure KrogerAPI.JSON_T_Arr snippet $$[{"locationId":"abc"}]$$;
create import mapping KrogerAPI.IM_T_Arr with json structure KrogerAPI.JSON_T_Arr {
  create KrogerAPI.Location { LocationId = locationId }
};
-- Error: "locationId" is not a member of the JSON structure at (Object),
--        which has no members there
```

**Nested root — `OpenAI_API.IM_OpenAI` (CitrusGrove)**
The mapping's root object element sits at `(Object)|choices|(Object)|message` —
Studio Pro let the author start the mapping deep inside the response. There is
no MDL for this at all, and `describe` prints it as if it were rooted at the
document root:

```
create or modify import mapping OpenAI_API.IM_OpenAI
  with json structure OpenAI_API.JSON_OpenAI_Response
{
  create OpenAI_API.Message { Content = content }   -- binds (Object)|content. Wrong.
};
```

That output parses and executes. It produces a mapping that resolves against
nothing at runtime.

**Message definition — `AgentCore.Email_Import` (EnquiriesManagement)**
`MessageDefinition = "AgentCore.MesDef_Email.Email"`. `describe` emits **no**
`with` clause whatsoever, and the result parses. 74 documents are affected;
`MendixSSO.AppRolesResponse` loses both its array root and its source in the
same output.

**Custom object handling — `AgentCommons.IM_Agent` / `EM_Agent`**
56 documents call a microflow to resolve the object instead of
creating/finding it:

```
CustomHandlerCall = Mappings$MappingMicroflowCallImpl {
  Microflow: "AgentCommons.AgentImport_GetSelf",
  ParameterMappings: [ { Parameter: "…AgentImport_GetSelf.AgentImport",
                         JsonValueElementPath: "(parent)", LevelOfParent: -1 } ]
}
```

Four parameter sources occur in the corpus: `(parent)`, `(parameter)` (the
mapping's own input object), a level-N ancestor (`LevelOfParent` 1 or 2, export
only), and an explicit JSON value path such as
`(Object)|embeddings|(Object)|_index`.

**Converter microflow — `FeedbackModule.IMM_PostResponse`**
`Converter = "FeedbackModule.ConvertUUIDToURL"` on the value element for `URL`.
The MDL grammar *already has* the spelling (`Attr = Module.MF(jsonField)`) and
the AST *already has* the fields (`Converter`, `ConverterParam` in
`ast_import_export_mapping.go:52`), but no executor or writer reads them —
every writer hardcodes `Converter: ""`. So the syntax parses, the mapping is
created, and the transform is silently absent.

**Wrapper — `KrogerAPI.IM_ProductList`**
`categories: ["Dairy","Milk"]` maps to one `Category` entity per string:

```
ObjectMappingElement et=Wrapper entity=KrogerAPI.Category assoc=KrogerAPI.Category_Product
                     path=(Object)|data|(Object)|categories|(Wrapper)
  ValueMappingElement  attr=KrogerAPI.Category.Value
                     path=(Object)|data|(Object)|categories|(Wrapper)|(Value)
```

`describe` renders this as `create …/KrogerAPI.Category = categories/(Wrapper) {
Value = (Value) }` — internal markers leaking into MDL, and unparseable.

**Export array container with no entity — `AgentCommons.EM_Agent`**
Every export array in the corpus (33 documents) has the shape *container with no
entity and no association* → *item with the entity and association*:

```
ObjectMappingElement et=Array  entity=-                assoc=-                  path=(Object)|Versions
  ObjectMappingElement et=Object entity=AgentCommons.Version assoc=…            path=(Object)|Versions|(Object)
```

MDL requires an entity on both levels and writes one on both levels (§6.2).

## 4. Coverage if each gap is closed

Greedy: at each step, the construct that unblocks the most additional documents.

| after adding | docs fully expressible | |
|---|---:|---:|
| *(today)* | 97 | 30% |
| + array root (#248) | 123 | 38% |
| + message-definition source | 177 | 54% |
| + converter microflow | 200 | 61% |
| + custom object handling | 222 | 68% |
| + entity-less object element | 248 | 76% |
| + nested root | 261 | 80% |
| + find/create × error/ignore | 276 | 84% |
| + quoted member names | 280 | 86% |
| + mapping input parameter | 283 | 87% |
| + the XML/wrapper long tail | 327 | 100% |

Mappings whose **only** blocker is a single construct — the cheapest wins:
array root **26**, converter microflow **18**, custom handling **11**, nested
root **9**.

Reproduce with `scripts/mapping-census/census.py mx-test-projects/*.mpk`; the
classifier is the same one this section was computed from.

The two large items (array root, message definitions) are also the two that
today fail **silently** in `describe`, which raises their priority above their
raw count.

## 5. `describe` defects that are not expressiveness gaps

These need no new syntax; `describe` emits shapes the grammar has never
accepted, or drops content.

### 5.1 An object element cannot address a nested member path

`describe` of `KrogerAPI.IM_ProductList` emits:

```
create KrogerAPI.Pagination_ProductRoot/KrogerAPI.Pagination = meta/pagination { … }
                                                               ^ parse error
```

`importMappingChild`'s object form takes a single `identifierOrKeyword` on the
right of `=`, not a `jsonMemberPath`. #927 gave *value* elements multi-segment
paths; object elements never got them, and Studio Pro produces them routinely
(an object mapped several levels down with nothing mapped in between).

### 5.2 An array/wrapper element is printed as an empty value binding

`printExportMappingElement` (`cmd_export_mappings.go:136`) branches on
`elem.Kind == "Object"`. An element parsed with `Kind == "Array"` or
`"Wrapper"` falls into the value branch, so `AgentCommons.EM_Agent` describes as:

```
    Versions = ,
    Variables = ,
    …
    TestCases = ,
```

— an empty right-hand side **and the entire subtree dropped** (all of Version,
Tool, MCP, KnowledgeBase, TestCase). It does not parse, which is the only reason
this has not silently destroyed a mapping.

### 5.3 Three describe-only spellings the grammar never accepted

`describe` emits `create . = x`, `create ./Module.Entity = x` (import) and
`. as x`, `./Module.Entity as x` (export) for entity-less / association-less
elements. All four are parse errors — verified directly. Either the grammar
gains them or `describe` must stop emitting them.

### 5.4 JSON member names that are not identifiers

`jsonMemberPath` is `identifierOrKeyword (SLASH identifierOrKeyword)*`. There is
no quoted form, and `/` is the path separator, so a member named
`https%3A//sws.siemens.com/sam/claims/tenantId` (TcConnector.IM_DecodedJWT),
`sws.samauth.ten`, or `$type` (43 occurrences, OneHarness/CapitalConnector)
cannot be written or described. 5 documents.

## 6. Two writer defects: values outside the metamodel enum

Neither is a missing feature; both are wrong bytes written by the current code.

### 6.1 `ObjectHandlingBackup` is written as `Find` / `Parameter`

`ImportMappingsObjectHandlingBackup` and `ExportMappingsObjectHandlingBackup`
are `{Create, Error, Ignore}` (`generated/metamodel/enums.go:621`, `:540`).
`mapping_write.go:155` sets `objectHandlingBackup := objectHandling`, so:

| MDL | ObjectHandling | ObjectHandlingBackup | legal? |
|---|---|---|---|
| `create` | Create | Create | yes |
| `find or create` | Find | Create | yes |
| `find` | Find | **Find** | **no** |
| export root (implicit) | Parameter | **Parameter** | **no** |
| export nested (implicit) | Find | **Find** | **no** |

Measured by executing MDL against a copy of CitrusGrove and re-reading the BSON:

```
find KrogerAPI.Location { LocationId = locationId key }
→ ObjectHandling='Find'  ObjectHandlingBackup='Find'

KrogerAPI.Location { locationId = LocationId }        (export root)
→ ObjectHandling='Parameter'  ObjectHandlingBackup='Parameter'
```

Across 1,261 object elements in the corpus, `ObjectHandlingBackup` is
`Create` (683), `Error` (562) or `Ignore` (20) — **never** `Find` or
`Parameter`. Every export element in every demo app uses `Error`.

Both the legacy (`sdk/mpr/writer_import_mapping.go:122`,
`writer_export_mapping.go:136`) and modelsdk (`mapping_write.go:155`, `:336`)
paths do this, so it is engine-independent.

Studio Pro's own metamodel confirms the enum: PED's `element` schema for
`ImportMappings$ImportObjectMappingElement` declares
`objectHandlingBackup: 'Create' | 'Ignore' | 'Error' = "Create"` (§7a).

**mxbuild does not catch it.** Measured on 11.13.0 against a copy of the
`ped-verify` project carrying two mxcli-written mappings — `ZZ_Probe_Ok`
(`find or create` → `Find`/`Create`, legal) and `ZZ_Probe_OffEnum` (`find` →
`Find`/**`Find`**, off-enum), both confirmed on disk by decoding the `.mxunit`:

```
The app contains: 0 errors.
```

With a positive control, so the 0 is not vacuous — adding a third mapping with a
keyless `find` makes the same checker fire:

```
[error] [CE0250] "Object element must have a key defined if object handling is
set to 'Search for an object'." at Object mapping element 'Root'
The app contains: 1 errors.
```

So mxbuild's mapping checker is live and still passes the off-enum value.

**Studio Pro tolerates it too.** The same three mappings were written into the
project Studio Pro 11.13.0 had open (`mx-test-projects/ped-verify`), reloaded,
and checked with PED's own `ped_check_errors`:

| mapping | on disk | `ped_check_errors` |
|---|---|---|
| `ZZ_Probe_Ok` | `Find` / `Create` | No errors found |
| `ZZ_Probe_OffEnum` | `Find` / **`Find`** | **No errors found** |
| `ZZ_Probe_NoKey` (control) | `Find` / `Find`, no key | `Object element must have a key defined if object handling is set to 'Search for an object'. (at locations: /rootMappingElements/0)` |

The control is what makes the two clean results mean something: the checker
**does** inspect object handling on the root mapping element — it flags the
missing key at `/rootMappingElements/0`, the very element carrying the off-enum
backup — and says nothing about the backup value. And the project **opened**:
no `System.InvalidOperationException`, PED answers about the document normally.

So this is **not** the CLAUDE.md failure mode where an unknown property makes
Studio Pro refuse to open. Severity is lower than feared, and the fix drops from
"data-loss risk" to "wrong bytes that no tool complains about".

**One thing remains untested:** whether Studio Pro *normalizes* `Find` to the
declared default `Create` on load. If it does, the value is silently discarded
rather than accepted — the same practical outcome, a different mechanism.
Measuring it needs a save, and a save only rewrites units Studio Pro considers
changed, so it needs a deliberate edit to that mapping in the UI. Disk was
re-read after the reload and is still byte-for-byte what mxcli wrote, so Studio
Pro had not rewritten anything of its own accord.

It should still be fixed — an off-enum value is wrong, `find` has no way to
express the Error/Ignore continuations users actually want (§7.4), and nothing
in this measurement covers what a *future* Studio Pro does with it. It should be checked by opening a written project before this is
prioritised, but the fix is unambiguous either way: export always `Error`;
import `find` needs the backup chosen by the author (§7.4).

### 6.2 Export arrays are written with the wrong shape

mxcli writes the array container **with** entity and association:

```
ObjectMappingElement et=Array  entity=KrogerAPI.Product assoc=KrogerAPI.Product_ProductRoot
  ObjectMappingElement et=Object entity=KrogerAPI.Product assoc=KrogerAPI.Product_ProductRoot
```

Studio Pro writes the container bare (33 of 33 documents, §3.1). The MDL that
produces this also forces the author to repeat `Assoc/Entity` on both levels,
which is redundant given the container should carry neither.

## 7. Proposed syntax

Design constraints from `.claude/skills/design-mdl-syntax.md` and ADR-0003:
standard verbs, qualified names, one example teaches the variants, adding a
property is a one-line diff.

### 7.1 Array root — no new syntax (#248)

The schema decides; Studio Pro does not ask either. Root resolution becomes:
look up `(Object)`, and if the structure has no such element, look up
`(Array)|(Object)`. One change in `buildImportMappingElementModel`
(`cmd_import_mappings.go:356`) and its export twin
(`cmd_export_mappings.go:293`), which today hardcode `lookupPath = "(Object)"`.

This unblocks `returns mapping … as list of` and the `first`/`offset` ranges,
and lets `bug-tests/519-rest-mapping-as-list-of.mdl` execute.

### 7.2 Nested root — an `at` clause on the source

```
create import mapping OpenAI_API.IM_OpenAI
  with json structure OpenAI_API.JSON_OpenAI_Response at choices/message
{
  create OpenAI_API.Message { Content = content }
};
```

Absent = the structure's own root (§7.1). The path uses the existing
`jsonMemberPath` spelling, so `choices/message` steps through the array without
naming `(Object)`. `describe` emits it whenever the root element's `JsonPath`
is not the structure root.

### 7.3 Schema sources — resolve them, and add the two missing ones

```
with json structure Module.Name          -- today
with xml schema Module.Name              -- parses today, never resolved
with message definition Module.Doc.Message   -- new
```

Three changes, in order of value:

1. **Resolve the reference.** Today `with json structure M.NoSuchThing` is
   written verbatim → CE1613, *and* it disables member validation for the whole
   mapping because `jsonSchemaIndex.resolvable()` returns false, so a typo in
   the structure name suppresses every member error too. The refusal must name
   what would have worked, like the #882 member refusal does. This also fixes
   `null values Banana`, which is written through unvalidated today (verified).
2. **`with message definition`** — 74 documents. The definition must already
   exist in the project, exactly as `with xml schema` intends to work.
3. `create message definition` / `create xml schema` are a separate proposal;
   the `with` clause is useful before them (marketplace modules ship the
   definitions).

### 7.4 Object handling — say what happens when the object is not found

```
create Module.Entity { … }                      -- unchanged
find Module.Entity or create { … }              -- ObjectHandling=Find, Backup=Create
find Module.Entity or ignore { … }              -- Backup=Ignore   (12 docs)
find Module.Entity or error { … }               -- Backup=Error    (1 doc)
create Module.Entity or error { … }             -- Backup=Error    (6 docs)
```

`find or create` stays as an alias for `find … or create` (it is what the
existing grammar spells and what the tests use). Bare `find` currently writes an
illegal enum; it should become an error naming the three continuations rather
than silently picking one — the corpus has no dominant default (Create 2,
Error 6, Ignore 18).

`ObjectHandlingBackupAllowOverride` (2 docs) is a suffix on the same clause:
`or create overridable`.

### 7.5 Custom object handling — `by <microflow>(...)`

```
find MxGenAIConnector.KnowledgeBaseChunk
     by GenAICommons.Chunk_FindByIndex ( Index: embeddings/_index )
     = embeddings
{
  Value = value
}
```

Parameter sources, one keyword each, covering all four shapes in the corpus:

| MDL | stored |
|---|---|
| `Param: parent` | `JsonValueElementPath="(parent)"`, `LevelOfParent=-1` |
| `Param: parameter` | `"(parameter)"`, `-1` — the mapping's own input object |
| `Param: parent(2)` | `"-"`, `LevelOfParent=2` — export only |
| `Param: some/json/path` | the value element's JsonPath, `-1` |

`by` reads as English on both sides ("find the chunk by
Chunk_FindByIndex") and does not collide with `find or …`.

### 7.6 Value converter — wire up what already parses

```
URL = uuid via FeedbackModule.ConvertUUIDToURL
```

The grammar's existing form `URL = FeedbackModule.ConvertUUIDToURL(uuid)` is
already parsed into `Converter`/`ConverterParam`. Prefer wiring **that** form
end-to-end first — it is a model field, two writer lines and a describe branch,
with no grammar change — and consider the `via` spelling separately if the
call-shaped form reads badly next to `Attr = a/b/c`. Note the stored document
has only `Converter` on the value element: the microflow receives the element's
own value, so `ConverterParam` must equal the member the element already binds
(a mismatch should be refused, not silently ignored).

### 7.7 Primitive arrays (wrapper) — `[]` on the member

```
create KrogerAPI.Category_Product/KrogerAPI.Category = categories[] {
  Value = value
}
```

`categories[]` selects the `(Wrapper)` level; the reserved member `value` binds
`(Value)`. This keeps `(Wrapper)`/`(Value)` out of MDL — they are storage
markers, not names an author should type. (Alternative considered:
`= categories/(Wrapper)` with `Value = (Value)`, i.e. what `describe` already
emits. Rejected — it exposes internal markers and reads as a member named
`(Wrapper)`.)

### 7.8 Entity-less object element — `group`

```
group as Versions {                                     -- export array container
  AgentCommons.Agent_Version/AgentCommons.Version as VersionsItem { … }
}
```

Replaces the `describe`-only `.` / `./Entity` forms (§5.3) with a word. On the
import side the same construct is `group = versions { … }`. This is also what
lets §6.2 be fixed without the author repeating `Assoc/Entity`.

### 7.9 Mapping input parameter

```
create import mapping MxGenAIConnector.IM_CohereEmbed_Response
  with json structure …
  parameter GenAICommons.ChunkCollection
{ … }
```

Feeds `ParameterType` (`DataTypes$ObjectType` + `Entity`), and is what
`Param: parameter` in §7.5 refers to. 10 documents.

### 7.10 Quoted member names

```
"https%3A//sws.siemens.com/sam/claims/tenantId" = TenantId
Type = "$type"
```

A `STRING_LITERAL` alternative in `jsonMemberPath`'s segment rule. Quotes are
stripped and the segment is used verbatim — no path splitting inside a quoted
segment.

## 7a. What Studio Pro's PED MCP server exposes (probed live, 2026-08-24)

Probed through a host `socat` on 7790 → Studio Pro's MCP on `localhost:7782`,
with `cmd/mcpprobe`. Studio Pro **11.13.0**; the tool surface matches
`mdl/backend/mcp/testdata/tools-11.13.json` exactly, minus the federated
`mcp_mendix-marketplace_*` entry that `PED_MCP_CAPABILITIES.md` already calls a
sample rather than a constant — 17 tools, no change to record there.

Mappings sit in the **schema-visible, document-inaccessible** bucket — the same
place the security documents are:

| tool | mappings |
|---|---|
| `ped_get_schema` (`element` and `constructor`) | **works** — full schemas for both document types and every element type |
| `ped_list_folder` | **works** — lists them by name and `ImportMappings$ImportMapping` / `ExportMappings$ExportMapping` |
| `ped_check_errors` | **works** — accepts the type, returned "No errors found" |
| `ped_read_document` | `Unknown document type 'ImportMappings$ImportMapping'.` |
| `ped_find_document` | `Unknown document type` |
| `ped_update_document` | `Document type ImportMappings$ImportMapping is not supported.` |

Controls: the same calls against `Microflows$Microflow` and
`JsonStructures$JsonStructure` succeed, and `ped_update_document` on a
*nonexistent microflow* returns "not found" rather than "not supported", so the
probe reaches document resolution. The constructor schema is `{ $Type, name }`
only — even if create were whitelisted it would produce an empty shell with no
way to populate it.

**So there is no MCP route to a mapping today**, and mxcli's own MCP backend
agrees: all ten mapping methods are `unsupportedBackend` stubs
(`mdl/backend/mcp/unsupported_gen.go`). `JsonStructures$JsonStructure` *is* fully
readable and writable (`jsonSnippet` is in its constructor), which is worth
knowing — the mapping's source document is reachable even though the mapping is
not.

### What the schema settles

The `element` schema is Studio Pro's own metamodel, and it confirms two things
this proposal had to infer:

```
objectHandling:       'Parameter' | 'Create' | 'Find' | 'Custom'   = "Create"
objectHandlingBackup: 'Create' | 'Ignore' | 'Error'                = "Create"
nullValueOption:      'SendAsNil' | 'LeaveOutElement'              = "LeaveOutElement"
```

`Find` and `Parameter` are **not** legal `objectHandlingBackup` values (§6.1),
and `null values Banana` is off-enum (§7.3). This is a stronger source than the
corpus inference — it is the platform declaring the domain — though it still does
not say what Studio Pro *does* when it loads an off-enum value.

It also confirms every construct §7 proposes is a real, first-class property:

| §7 item | PED property |
|---|---|
| custom object handling | `mappingMicroflowCall?: Element<'Mappings$MappingMicroflowCall'>` (BSON: `CustomHandlerCall`) |
| converter microflow | `converter?: Reference<'Microflows$Microflow','qualified-name'>` |
| wrapper / arrays | `elementType: … 'NamedArray' \| 'Array' \| 'Wrapper'` |
| mapping input parameter | `parameterType: ChooseAbstractType<DataTypes$*>` |
| message-definition source | `messageDefinition?: Reference<'MessageDefinitions$MessageDefinition','qualified-name'>` |
| XML attribute / content | `isXmlAttribute`, `isContent`, `xmlPrimitiveType` |
| custom-handler parameters | `Mappings$MappingMicroflowParameter { parameter, levelOfParent, jsonValueElementPath, xmlValueElementPath }` |

Two naming notes for whoever implements this. PED speaks the **SDK** names
(`ImportMappings$ImportObjectMappingElement`, `mappingMicroflowCall`) while the
BSON uses the storage names (`ImportMappings$ObjectMappingElement`,
`CustomHandlerCall`) — the split CLAUDE.md documents, now visible on both sides
at once. And `Mappings$ElementPath` reads as an **opaque element** (`{ $Type }`,
no properties), so PED would not expose the `(Object)|a|b` paths even if the
document tools accepted mappings.

### Consequence for this proposal

The MPR backend stays the only route, so nothing in §7 changes. But two things
follow:

1. **`ped_check_errors` is usable as an oracle right now** — it accepts a mapping
   by name without needing to read it, so a mapping mxcli writes into an open
   project can be error-checked by Studio Pro itself. That is a better signal
   than `mx check` for exactly the cases (§6.1) where mxbuild is known to be
   permissive.
2. `PED_MCP_CAPABILITIES.md` should gain a mappings row and the new tool surface,
   and the "determine support with `ped_read_document`, not `ped_find_document`"
   note now has a second confirmed instance.

## 8. Testing, and why none of this was caught

`make check-mdl` runs `mxcli check` with **no project**, so it validates syntax
and nothing else. Every defect above needs either a project or a decoded
document to see. Three additions, cheapest first:

1. **A round-trip test over a fixture project.** For every mapping: describe →
   re-execute into a copy → compare the decoded documents. This is the test that
   would have caught all of §5 and the silent-loss class in §1. The demo `.mpk`s
   are far too large for CI (2.5 GB), but a curated fixture carrying one mapping
   per shape in this document is small.
2. **An enum guard.** Assert that every written `ObjectHandling`,
   `ObjectHandlingBackup` and `NullValueOption` is in the corresponding
   `generated/metamodel` enum. §6.1 and `null values Banana` both fall out of one
   test.
3. **`check --references` must resolve the schema source** (§7.3), so item 3 of
   the current gap list stops being invisible to CI.

Land the corpus scripts (BSON decode + feature classify) under `scripts/` so the
denominator can be recomputed when a marketplace module changes shape.

## 9. Tracker actions

Filed 2026-08-24 against `ako/mxcli`.

| issue | §  | docs blocked |
|---|---|---:|
| [#248](https://github.com/ako/mxcli/issues/248) (updated) — array root, export half now confirmed | §7.1 | 122 |
| [#259](https://github.com/ako/mxcli/issues/259) — schema sources never resolved; a typo also disables member validation; `null values` unvalidated | §7.3 | — |
| [#260](https://github.com/ako/mxcli/issues/260) — DESCRIBE does not round-trip (4 defects + the 112 silent-loss set) | §5, §8 | 244 |
| [#261](https://github.com/ako/mxcli/issues/261) — `ObjectHandlingBackup` off-enum; `find … or error/ignore` unauthorable | §6.1, §7.4 | 19 |
| [#262](https://github.com/ako/mxcli/issues/262) — export array container written with entity + association | §6.2, §7.8 | 33 |
| [#263](https://github.com/ako/mxcli/issues/263) — `with message definition` | §7.3 | 74 |
| [#264](https://github.com/ako/mxcli/issues/264) — custom object handling (`by <microflow>(…)`) | §7.5 | 56 |
| [#265](https://github.com/ako/mxcli/issues/265) — mapping input parameter | §7.9 | 10 |
| [#266](https://github.com/ako/mxcli/issues/266) — converter microflow (already parses) | §7.6 | 39 |
| [#267](https://github.com/ako/mxcli/issues/267) — nested schema root (`at a/b`) | §7.2 | 13 |
| [#268](https://github.com/ako/mxcli/issues/268) — primitive-array wrapper (`member[]`) | §7.7 | 34 |
| [#272](https://github.com/ako/mxcli/issues/272) — `create json structure` builds different element metadata than Studio Pro | — | — |
| [#277](https://github.com/ako/mxcli/issues/277) — export value elements omit `IsKey`, hardcode `MaxLength 0` | — | — |
| *(unfiled — GitHub rate limit)* rebuilt mappings carry a fixed property set: `MessageDefinition2` dropped, `MappingSourceReference` added, export `MinOccurs` hardcoded. Body drafted; see the session scratchpad | — | — |

[#253](https://github.com/ako/mxcli/issues/253) is unchanged and still open.

#272 was found by the round-trip fixture rather than the census: a real Studio Pro
mapping using nothing outside MDL's range failed to round-trip until its JSON
structure was transplanted verbatim instead of regenerated from its snippet.
The census cannot see it, because it reads stored documents and never rebuilds one.

Not filed separately, deliberately: the quoted-member-name gap (§5.4, 5 docs) is
inside #260; the XML-schema and XML-attribute long tail (§3, 3 docs each) waits on
a `create xml schema` proposal; the CI round-trip test (§8) is the acceptance
criterion of #260 rather than its own ticket.

## 10. Latent in the repo

Unchanged from the current gap list, restated so it is not lost:
`doctype-tests/21`'s `IMM_UpsertPet` is `find or create` over a non-persistent
entity (CE0251) that never reaches a build because PART 5 drops it;
`360-import-mapping-result-type.mdl` and
`369-rest-mapping-result-cardinality.mdl` are in the `check-mdl` SKIP list
under #571.
