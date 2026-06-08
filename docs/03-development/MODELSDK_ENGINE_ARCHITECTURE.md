# modelsdk engine architecture & patterns

How the `modelsdk` write/read engine is structured, and the canonical patterns to
follow when extending it. Read this before adding a document type or a microflow
activity group. Decisions behind it: [ADR-0002](../13-decisions/0002-backend-abstraction.md)
(backend abstraction), [ADR-0004](../13-decisions/0004-full-codec-engine.md) (full codec),
[ADR-0005](../13-decisions/0005-semantic-model-interface-currency.md) (model is the interface currency).

## Layers

```
front-ends:  MDL executor ─┐
             fluent api/   ─┼─► SEMANTIC MODEL ──► backend interface (model-typed) ──┐
             (importers…)  ─┘   (sdk/microflows,                                      │
                                 sdk/domainmodel, model)                  ┌───────────┼────────────┐
                                                                      MPR backend  MCP/PED      (future fmt)
                                                                      gen+codec    JSON ops      adapter
                                                                          │
                                                                        BSON
```

- **The backend interface speaks the semantic model**, never gen/AST/BSON types. gen+codec
  (`modelsdk/gen`, `modelsdk/codec`) are the **MPR backend's internal storage adapter** — one of
  several (MPR, MCP/PED, a future format Mendix is considering). Do not leak gen into `mdl/backend/*.go`.
- The `modelsdk` MPR backend lives in `mdl/backend/modelsdk/`; selected via `MXCLI_ENGINE=modelsdk`
  (default is `legacy`). Parity against `legacy` is the correctness gate during migration.

## Write patterns

**Unit-write pattern (top-level documents: microflows, enums, constants, pages, …).**
A document is its own unit. `CreateX` → build gen → `assignXIDs` → `codec.Encoder.Encode` →
`writer.InsertUnit(unitID, containerID, "Documents", "<$Type>", contents)`. `DeleteX` →
`writer.DeleteUnit(id)`. `UpdateX` (CREATE OR REPLACE) → rebuild → `writer.UpdateRawUnit(id, contents)`.

**Child-element writes (entities/associations live inside the domain-model unit).**
Load the DM gen unit (`loadDomainModelGen`), mutate it (`AddEntities`, `RemoveEntities`, …), re-encode
the whole unit, `UpdateRawUnit`. Clean siblings pass through their original bytes (codec passthrough).

**CREATE = model→gen.** Write an `xToGen(model) *genX` converter mirroring the legacy serializer's
field set. Use engalar/dev's `flowbuilder_*_gen.go` / `*_gen.go` as the **reference** for which gen
setters to call (it targets the gen API directly) — but never adopt engalar's gen-typed interface
(see Harvest rule). Then `assignXIDs` walks the tree assigning fresh IDs to elements lacking one
(`assignID` is no-op on non-empty IDs; carry domainmodel IDs where references must resolve).

**Reads = gen→model.** Write `xFromGen(*genX) *model.X`. Make it **lossless** for any field that an
ALTER read-modify-write path round-trips (else ALTER silently drops data — see the entity history).

**ALTER.** Two sanctioned approaches (ADR-0005):
- **Mutator (preferred for fidelity-sensitive / large docs):** `OpenXForMutation(id) (XMutator, error)`
  loads the decoded gen unit and exposes semantic mutation ops; untouched sub-elements pass through
  byte-for-byte → clean git diffs. Template: `PageMutator` / `WorkflowMutator` in `mdl/backend/mutation.go`.
  **Microflow ALTER, when added, MUST use this pattern** (don't repeat entity's model round-trip).
- **Model round-trip (acceptable for small/simple docs):** `GetX` → mutate model → `UpdateX`. Used today
  for entities/enums. Correct but lossy-by-construction (needs a lossless read adapter) and churns IDs/
  order, so reserve it for docs where diff-fidelity doesn't matter.

## Codec mechanisms a converter author needs

- **`codec.RegisterTypeDefaults($Type, TypeDefaults{…})`** — fields Studio Pro emits on create that the
  gen constructor doesn't set. Knobs: `EmitGUID` (GUID = element $ID, binary), `MandatoryLists` (empty
  PartList emitted as `[marker]`), `NullFields` (emit BSON null), `ZeroGUIDFields` (all-zero GUID binary,
  e.g. an unset pointer), `FreshGUIDFields` (fresh random GUID binary, e.g. `StableId`).
- **`codec.RegisterListMarker(childType, n)`** — typed-array version marker. Default is **3**; some lists
  use **2** (index `IndexedAttribute`, sequence-flow `CaseValues`, change-action `ChangeActionItem`) or
  **1** (by-name lists like `AllowedModuleRoles`/`UrlSearchParameters`). Verify against real BSON.
- **Storage-name overrides.** The gen sometimes wires the SDK name, not the BSON storage name. Patch the
  gen `init` + `InitFromRaw` (both encode and decode) with a `STORAGE-NAME OVERRIDE` comment; the permanent
  fix is `internal/codegen/supplements.json` + regenerate. Known cases: `ErrorMessage`→`Message`,
  EventHandler `Event`→`Type` / `PassEventObject`→`SendInputParameter` / list `EventHandlers`→`Events`,
  microflow `ConcurrencyErrorMessage`→`ConcurrenyErrorMessage` (Mendix typo), `StableId` is binary-not-string
  (handled via `FreshGUIDFields`). gen MicroflowParameter was also missing fields (completed by hand).

## Harvest rule (engalar/dev)

engalar/dev has a complete, tested gen-native write suite, but built **AST-direct with a gen-typed
interface**. Harvest its `flowbuilder_*_gen.go` only as a **reference for `model→gen` construction**.
Never adopt its gen-typed backend interface or AST-direct executor — that would weld the contract to BSON
and break the MCP backend + the future format (ADR-0005). See `docs/11-proposals/ASSESSMENT_harvest_engalar_writes.md`.

## Recipe: add a document type or activity group

1. **Find the legacy serializer** for the type (`sdk/mpr/writer_*.go`) — the field set + ordering spec.
2. **Capture real BSON when unsure** — legacy can be wrong (e.g. the index `SortOrder` bug). Dump an
   on-disk `.mxunit` or use the MCP/PED probe (`cmd/mcpprobe`) to get authoritative keys/markers.
3. **Write `xToGen`** (+ `xFromGen` if reads/ALTER need it), registering any TypeDefaults / list markers.
4. **`assignXIDs`** walks new sub-elements.
5. **Add a parity test** in `mdl/enginecompare/` (`copyProject` → `Run(Legacy,…)` + `Run(ModelSDK,…)` →
   `XCanonBSON` → diff). Add an `XCanonBSON` dumper to `bsoncompare.go` for new top-level types.
6. **Iterate on the diff** until byte-identical. (Per-group this is fast: 1–2 iterations.)
7. **gofmt any hand-edited gen file** or `TestGeneratedCodeIsFormatted` fails.

## Verification truth

`legacy` is the parity baseline, but it is **not infallible** — it has had stale serializers (index
`SortOrder`). When a gen-vs-legacy disagreement appears, the tiebreaker is **real Studio-Pro BSON**
(on-disk dump or MCP capture), not whichever engine you trust. The gen has been wrong (EventHandler keys);
legacy has been wrong (indexes). Capture, don't guess.
