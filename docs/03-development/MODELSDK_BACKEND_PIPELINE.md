# modelsdk Backend Pipeline — how the roundtrip BSON engine works

**Status:** Reference (development) · **Date:** 2026-06-02
**Companion to:** [`docs/11-proposals/PROPOSAL_backend_strategy.md`](../11-proposals/PROPOSAL_backend_strategy.md)
**Subject:** the `modelsdk/` engine on `engalar/dev` that the backend-strategy proposal recommends adopting as the base.

---

## Why this document exists

The [backend-strategy proposal](../11-proposals/PROPOSAL_backend_strategy.md) recommends
replacing the hand-written `sdk/mpr` write path with engalar's `modelsdk/` package
because it produces **more reliable BSON** — specifically, it is *roundtrip-safe*:
fields mxcli does not understand survive a read/modify/write cycle byte-for-byte,
which is the bug class behind most `TypeCacheUnknownTypeException` / `CE0463`-style
"document changed" failures when reopening a project in Studio Pro.

This document explains the **development pipeline** of that engine end-to-end:

1. [How it uses `modelsdk`](#1-how-it-uses-modelsdk) — the three-layer engine.
2. [How it extracts BSON templates](#2-how-it-extracts-bson-templates) — the two
   distinct template sources (generated *type* schema vs. extracted *instance* BSON).
3. [How it uses those templates](#3-how-it-uses-those-templates) — constructing new
   documents and widgets.
4. [How it keeps non-modified values](#4-how-it-keeps-non-modified-values) — the
   dirty bitmap + clean-field passthrough encoder, the load-bearing reliability win.

All file paths below are on the `engalar/dev` branch unless noted otherwise.

---

## The pipeline at a glance

```
                        ┌──────────────────────────────────────────────┐
   .mpr / mprcontents   │            modelsdk ENGINE                     │
   (BSON units)         │                                                │
        │               │  ┌── codec.Decoder ──┐                         │
        │  raw bytes     │ │  $Type → factory   │   element tree         │
        └──────────────▶ │  │  (TypeRegistry)   ├──▶ Base + Property[T]  │
                        │  └───────────────────┘    (lazy-decoded)       │
   modelsdk/mpr.Reader  │            │                     │             │
   modelsdk/codec.Store │            │ raw bytes kept       │ mutate via  │
                        │            │ on every element     │ setters     │
                        │            ▼                     ▼ (marks dirty)│
                        │  ┌── codec.Encoder ──────────────────────────┐ │
                        │  │  clean element  → passthrough raw bytes    │ │
                        │  │  dirty element  → rebuild, clean FIELDS    │ │
                        │  │                   still pass through raw   │ │
                        │  └────────────────────────────────────────────┘│
                        │            │ new BSON bytes                     │
                        └────────────┼────────────────────────────────────┘
                                     ▼
                  unitstore.BufferedUnitStore  (in-memory pending set)
                                     │ Flush() — one batched transaction
                                     ▼
                  unitstore.UnitPersistence → .mpr SQLite / mprcontents

      Templates feed the "new element" path (no raw bytes yet):
        modelsdk/gen/*  ── generated TYPE schema (fields, defaults, versions)
        widgets/templates/*.json + .mpk ── extracted INSTANCE BSON shells
```

The crucial architectural point from the proposal: **`modelsdk` is the *engine* under
the local backend, not a new backend.** `mdl/backend/mpr` is rebuilt on top of it
behind the `unitstore` persistence seam; the `FullBackend` abstraction is unchanged.

---

## 1. How it uses `modelsdk`

`modelsdk` (`modelsdk/doc.go`) is a three-layer Go package — *"a replacement for the
hand-written `sdk/` package"* — built so that every read/modify/write goes through a
type registry and a dirty-tracked element tree.

### Layer 1 — `element` (identity + raw bytes + dirty bitmap)

`modelsdk/element/element.go` defines `Element` and the embedded `Base`:

- **Identity:** `ID()`, `TypeName()`, `Container()`, `Unit()`.
- **`Raw() bson.Raw`** — every decoded element keeps its **original BSON bytes**.
  This is what makes roundtrip safety possible (§4).
- **Dirty bitmap:** `dirty []uint64` (one bit per property) plus a `childDirty` flag.
  `MarkDirty(bit)` sets the bit *and* walks the container chain calling
  `MarkChildDirty()`, so a deep edit propagates "something below me changed" up to the
  unit root. `IsDirty()` / `IsChildDirty()` let the encoder cheaply skip clean subtrees.

### Layer 2 — `property` (lazy-decoded, per-field dirty)

`modelsdk/property/*` holds the typed fields. Every property type embeds
`propertyBase` (carrying the owner `*Base` and its dirty bit):

| Property type | Holds | File |
|---|---|---|
| `Primitive[T]` | scalar (string/bool/int32/float64…), **lazy** — decodes from raw on first `Get()` | `property/primitive.go` |
| `Enum` | enumeration value | `property/enum.go` |
| `Part[T]` | single contained child `Element` | `property/part.go` |
| `PartList[T]` | ordered list of child `Element`s | `property/part.go` |
| `ByNameRef` / reference | cross-element references | `property/reference.go` |

Two things matter here:

- **Lazy decode.** `Primitive[T].Get()` does not decode until first access
  (`loaded` flag), and decoding is a single `raw.LookupErr(key)` — the whole 25 KB
  document is never eagerly materialised.
- **Mutation marks dirty.** `Set()` / `Append()` / `Remove()` / `InsertAt()` call
  `markDirty()`, which flips the owner's dirty bit and propagates up the chain.
  `*FromDecode` variants (`SetFromDecode`, `AppendFromDecode`) populate during decode
  *without* marking dirty — so freshly read documents start 100 % clean.

### Layer 3 — `codec` (registry dispatch + encode/decode)

- **`TypeRegistry`** (`codec/registry.go`): maps the BSON `$Type` string →
  factory `func() element.Element`, and a reverse `Go type → canonical $Type`.
  Storage-name aliases (e.g. `ShowFormAction` ↔ `ShowPageAction`) are handled by
  **dual registration at codegen time**, not at runtime — the generated `gen/*/types.go`
  `init()` functions register both names. `DefaultRegistry` is the package-global.
- **`Decoder`** (`codec/decoder.go`): `Decode(raw)` reads `$Type`, looks up the
  factory, constructs the element, sets id/type/**raw**, and calls the optional
  `InitFromRaw(raw)` so the element wires its lazy properties to the raw bytes.
  **Unknown `$Type` → a bare `element.Base` that still carries the raw bytes**, so even
  a completely unrecognised document round-trips losslessly.
- **`Encoder`** (`codec/encoder.go`): the reverse, covered in detail in §4.

The read API is `modelsdk/mpr` (`Reader`/`Writer`) wrapped by `codec.Store`
(`codec/store.go`), which exposes unit-level BSON to the codec via the
`UnitReader`/`UnitWriter` interfaces (`codec/interfaces.go`).

### The lifecycle in one paragraph

Open the MPR → `codec.Store` lists units → `Decoder.Decode` turns each unit's raw BSON
into an `element.Base`-rooted tree whose typed properties are lazily backed by that raw
BSON → callers mutate through typed setters, which flip dirty bits and propagate up →
`Encoder.Encode` serializes, passing clean material through untouched → bytes go to the
`BufferedUnitStore`, flushed to disk in one batch at each checkpoint.

---

## 2. How it extracts BSON templates

"Template" means two different things in this codebase. The engine uses **both**, for
**different parts** of a document. Keeping them distinct is essential.

### 2a. Generated *type* templates — the metamodel (`modelsdk/gen/*`)

These are not BSON blobs; they are **Go struct definitions that know each type's field
set, BSON storage keys, default values, and version gating**. They are the schema the
encoder builds *new* elements from.

**Source & extraction** (`cmd/modelsdk-codegen/main.go`):

1. Input is the public `mendixmodelsdk` npm package's `src/gen/*.js` (same source our
   `internal/codegen` uses). Parsed by `internal/codegen/dtsparser/jsparser.go`.
2. `internal/codegen/supplements.json` is the single source of truth for codegen config:
   storage aliases, property-key overrides, force-concrete types, edge-kind overrides,
   id-ref scope, and **extra properties / extra types** that the npm SDK omits.
3. Phase 1 builds a cross-domain property map so inherited base-class properties
   (e.g. `projects.Document.Name`) reach types in other domains.
4. Phase 2 emits, per domain, four files:

   | File | Contents |
   |---|---|
   | `types.go` | struct + getters/setters + `InitFromRaw` + registry `init()` |
   | `enums.go` | enumeration aliases & constants |
   | `refs.go` | cross-domain reference metadata |
   | `version.go` | per-property introduced/deleted/public version info (version gating) |

Result: **53 domains** of generated, roundtrip-aware types. Regenerate with:

```bash
npm install mendixmodelsdk --prefix reference/mendixmodelsdk
go run ./cmd/modelsdk-codegen
```

> **Known debt (carried from the proposal):** `jsparser.go` is still regex-based, and
> the npm SDK structurally omits newer Studio Pro doc types (e.g. Agent Editor → hand-
> coded). The proposal's "re-point codegen at PED/mxunit" workstream addresses this.
> An `audit` / `audit-keys` mode (same `main.go`) scans real MPRs for unregistered
> `$Type`s and `ByIdRef` key mismatches — promote it to a CI gate.

### 2b. Extracted *instance* templates — widget BSON shells (`widgets/templates/*`)

Pluggable widgets (DataGrid2, ComboBox, Gallery, custom marketplace widgets) cannot be
generated from the metamodel — their structure lives in the widget's own `.mpk`. So the
engine **extracts a known-good BSON shell from a real project** and replays it.

There are two extraction routes:

**Route 1 — offline extraction into embedded JSON** (`cmd/mxcli/cmd_extract_templates.go`):

```bash
mxcli extract-templates -p app.mpr -o modelsdk/widgets/templates/mendix-11.6/
```

It opens a real project, calls `reader.ListAllCustomWidgetTypes()`, and writes each
widget's `Type` (PropertyTypes schema) **and** `Object` (default WidgetObject) — the two
fields that *both* must be present, or Studio Pro raises `CE0463` — to a JSON file. These
JSONs are embedded in the binary via `go:embed`. This is the canonical "extract a golden
BSON template from Studio Pro output" workflow.

**Route 2 — runtime derivation from `.mpk`** (MPK template derivation,
`docs/superpowers/specs/2026-05-08-mpk-template-derivation-design.md`):

When a widget has *no* embedded template, `widgets/loader.go`'s
`getOrGenerateTemplate(widgetID, projectPath)` falls back to:

```
embedded cache  →  session cache  →  FindMPK(projectPath, widgetID)
                                       → ParseMPK → GenerateFromMPK → cache
```

`GenerateFromMPK` builds the `CustomWidgetType` + `WidgetObject` shells and fills the
PropertyTypes/Properties from the widget's XML property defs (`xmlTypeToBSONType` covers
all 17 XML property types). System properties (Label/Visibility/Editability) are
deliberately **not** added — Studio Pro injects those on open.

**Route 3 — golden fixtures for tests** (`modelsdk/mpr/testdata/*.mxunit`): real
Studio-Pro-authored unit BSON used by the golden-diff harness to prove the engine is
behaviour-preserving (see §5 of the proposal).

---

## 3. How it uses those templates

### Using *type* templates: constructing a new element

When MDL creates a new document (e.g. `create entity`), the backend asks the generated
factory for a fresh element — it has typed properties but **no raw bytes**. The encoder
detects `raw == nil` and takes the *new-element branch* (`encoder.go::buildDoc`):

```go
if raw == nil {
    doc := bson.D{
        {Key: "$ID",   Value: idToBinarySubtype0(elem.ID())}, // fresh UUID if empty
        {Key: "$Type", Value: elem.TypeName()},               // canonical storage name
    }
    // append only the properties that were actually set (dirty), in
    // stable Properties() order
}
```

So a new document is the `$ID` + `$Type` from the registry, plus exactly the fields the
caller set — defaults and version gating come from the generated `version.go`/`types.go`.

### Using *instance* templates: placing a widget

`GetTemplateBSON` / `GetTemplateFullBSON(widgetID, idGenerator, projectPath)` resolves a
template (embedded → session cache → MPK derivation) and then **remaps every placeholder
`$ID` to a real UUID** (`collectIDs`) before the shell is spliced into the page's widget
tree. Because the shell came from genuine Studio Pro output (or the widget's own `.mpk`),
its `$Type`/PropertyTypes/Object structure already matches what Studio Pro expects — the
engine never *invents* widget BSON, it *replays* a verified shell.

---

## 4. How it keeps non-modified values

This is the reliability core — the reason to adopt the engine. There are **two levels**
of preservation, and they compose.

### Level 1 — clean elements pass through verbatim

`Encoder.Encode` (`codec/encoder.go`):

```go
func (e *Encoder) Encode(elem element.Element) ([]byte, error) {
    raw := elem.Raw()
    if raw != nil && !elem.IsDirty() {
        return []byte(raw), nil   // ← byte-for-byte original, zero re-encoding
    }
    return e.buildDoc(elem)
}
```

If you never touched a document, you write back exactly the bytes Studio Pro wrote.
Nothing can drift, because nothing is re-serialized.

### Level 2 — dirty elements rebuild, but clean *fields* still pass through

When an element *is* dirty, `buildDoc` does **not** re-encode the whole document. It
iterates the **original raw bytes** field-by-field and decides per field:

```go
rawElems, _ := bsoncore.Document(raw).Elements()
for _, re := range rawElems {
    if findRebuild(re.KeyBytes()) < 0 {
        // CLEAN field → copy the raw bytes through unchanged (zero-alloc,
        // verbatim) as a bson.RawValue
        doc = append(doc, bson.E{Key: ..., Value: bson.RawValue{...}})
        continue
    }
    // DIRTY field → encode the new value
    doc = append(doc, bson.E{Key: ..., Value: encodeEntry(...)})
}
// then append any genuinely-new fields that weren't in the raw bytes
```

Key consequences:

- **Fields mxcli has never heard of are preserved**, because they are simply "not in the
  rebuild set" → copied through as raw bytes. This is what eliminates the
  `TypeCacheUnknownType` / unknown-field bug class. The generated metamodel does **not**
  need 100 % field coverage for output to be correct — partial coverage is safe.
- **Field order is preserved** (we walk the original document's order; new fields append
  at the end).
- Child trees recurse the same way. `encodeEntry` handles `Part`/`PartList`:
  a `PartList` where the list itself wasn't reordered re-encodes only the *dirty children*
  and passes **clean children through as raw bytes** (`arr = append(arr, bson.Raw(child.Raw()))`).

### How "dirty" is decided (the three branches)

`buildDoc` uses the dirty bitmap to stay cheap:

| Element state | Branch | Cost |
|---|---|---|
| `!IsDirty()` | passthrough raw bytes | zero |
| dirty scalars only, `!IsChildDirty()` | rebuild, skip O(N) child scans | low |
| `IsChildDirty()` | rebuild + scan dirty children, recurse | proportional to edits |

The `childDirty` propagation (set in `element.go::MarkChildDirty`) means a scalar-only
edit on a 200-widget page never scans the widget list — it knows no child is dirty.

### Net guarantee

> **read → (edit a few fields) → write** produces a document identical to the original
> except for the fields you changed, down to byte order of untouched fields and the full
> content of fields mxcli does not model. Reopening in Studio Pro sees only your intended
> diff.

---

## 5. Persistence: the `unitstore` seam

Encoded bytes don't go straight to disk. `mdl/backend/unitstore` is the swappable I/O
seam the proposal calls "Option A":

- **`BufferedUnitStore`** (`unitstore/buffered.go`) holds all writes in an in-memory
  `pending` map. `Read` checks `pending → loaded cache → disk` (lazy). `Write` is pure
  memory. `Flush()` batches every pending unit into **one** `BatchStore` transaction —
  eliminating per-statement SQLite fsync overhead during large imports. `Discard()`
  drops pending writes (rollback).
- **`UnitPersistence`** (`unitstore/interfaces.go`) is the storage abstraction:
  `Load`, `BatchStore`, `BatchHash` (SHA-256 per unit, for `@cache:` markers). The MPR
  implementation is production; a stub is used in tests.

This seam is also where the **MCP backend diverges**: it is operation-based
(`ped_update_document`) and so does *not* implement `UnitPersistence` — it sits one layer
up as its own `FullBackend`. See the [backend-strategy proposal](../11-proposals/PROPOSAL_backend_strategy.md#architecture-the-two-axes-the-load-bearing-idea).

---

## 6. How the pieces map to the backend abstraction

```
FullBackend (mdl/backend)            ← unchanged interface (ADR-0002)
  └── localBackend
        └── modelsdk engine          ← §1 (element / property / codec)
              ├── codec.Store ──▶ modelsdk/mpr Reader/Writer
              └── output ──▶ unitstore.BufferedUnitStore ──▶ UnitPersistence ──▶ disk
        templates feed the new-element path:
              gen/*  (type schema, §2a/§3) · widgets/templates + .mpk (instance, §2b/§3)
```

The executor never sees BSON: it calls `ctx.Backend.*`; the local backend turns that into
element-tree mutations; the codec turns those into bytes; `unitstore` persists them.

---

## 7. Validation & testing the pipeline

From the proposal's testing approach, the pipeline-specific gates are:

1. **Golden-diff spine.** Run the shared `doctype-tests` corpus through the old
   (`sdk/mpr`) and new (`modelsdk`) engines and diff the resulting `.mxunit` BSON; proves
   the engine swap is behaviour-preserving and sizes the widget-serialization gap.
2. **Roundtrip tests** (`modelsdk/codec/roundtrip_test.go`, `modelsdk/dirty_chain_test.go`)
   assert decode→encode of an unmodified unit returns identical bytes, and that a targeted
   edit changes only the intended field.
3. **Codegen audit** (`cmd/modelsdk-codegen --audit / --audit-keys`) — scan real MPRs for
   unregistered `$Type`s and `ByIdRef` key mismatches; promote from test-only to CI gate.
4. **`mx check` / Studio Pro** — every new doctype example passes `mx check`; widget
   output is validated by opening in Studio Pro (the `CE0463` class).
5. **MCP as a BSON oracle** — diff local-engine output against Studio-Pro-authored BSON
   for the same operation, catching field/`$type`/pointer mistakes `mx check` misses.

---

## 8. Open issues carried from the proposal

- **Codegen source debt** — regex `.js` parser + npm SDK omits new doc types; the fix is
  re-pointing extraction at an authoritative PED/mxunit source (L effort, deferrable).
- **Widget v0.12 parity** — our v0.12 serialization is absent on his branch; run the
  golden-diff before committing the port. His real-time `.mpk` registry supersedes our
  `generatorVersion` staleness fix.
- **Storage-name correctness** — still depends on the alias table being right (see
  `CLAUDE.md` "BSON Storage Names" and the codegen `storageAliases`); the `audit` mode is
  the safety net.

---

## See also

- [`PROPOSAL_backend_strategy.md`](../11-proposals/PROPOSAL_backend_strategy.md) — the adoption decision and work breakdown
- [`PROPOSAL_mcp_backend.md`](../11-proposals/PROPOSAL_mcp_backend.md) — the live-Studio-Pro target backend
- [`UNIFIED_SCHEMA_REGISTRY.md`](../11-proposals/UNIFIED_SCHEMA_REGISTRY.md) — overlap with the codec's type/reference registry
- [ADR-0002: Backend Abstraction Layer](../13-decisions/0002-backend-abstraction.md) — the `FullBackend` contract this preserves
- [`PAGE_BSON_SERIALIZATION.md`](PAGE_BSON_SERIALIZATION.md), [`WIDGET_BSON_VERSION_COMPATIBILITY.md`](WIDGET_BSON_VERSION_COMPATIBILITY.md) — the BSON-fidelity concerns this engine is designed to solve
