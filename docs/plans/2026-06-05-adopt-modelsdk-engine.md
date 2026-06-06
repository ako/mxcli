# Implementation Plan — Adopt the modelsdk engine on `main`, behind a feature flag with dual-engine comparison

**Date:** 2026-06-05
**Status:** Draft plan
**Implements:** the "vendor the engine into `main`" path from
[`PROPOSAL_backend_strategy.md`](../11-proposals/PROPOSAL_backend_strategy.md) (Option 3 evolved
into a sequencing plan) — *keep `main` canonical and our product surface intact; lift the one
high-value, cleanly-separable asset (the roundtrip codec) under the existing `FullBackend`
abstraction.*
**Engine reference:** [`MODELSDK_BACKEND_PIPELINE.md`](../03-development/MODELSDK_BACKEND_PIPELINE.md)

---

## 1. Strategy in one paragraph

Vendor engalar's `modelsdk/` engine onto `main` as a **second implementation of the existing
`FullBackend` interface**, selected at the single factory seam
(`cmd/mxcli/main.go:205 SetBackendFactory`). Ship it **behind a feature flag** (`MXCLI_ENGINE`
/ `--engine`) with `legacy` (`sdk/mpr`) as the default. Validate the new engine with a
**dual-engine differential harness**: run the same MDL through both engines on a throwaway copy
and diff the normalized `.mxunit` BSON. Flip the default only when the diff is clean across the
corpus and real projects. **Defer the tree-wide `mongo-driver` v1→v2 migration to the cutover**
— the two engines coexist (v1 and v2 are different module paths) and we diff at the byte level,
which is driver-agnostic.

This is deliberately the *reversible* path: at every step both engines are present, the old one
is the source of truth, and the new one is shadow-validated before it ever owns a write.

## 2. Why the feature flag is the right backbone (answer to "does it make sense?")

Yes — and it does more than A/B switching:

- **Every real command becomes a differential test.** The curated `doctype-tests` corpus is
  ~90% shared but finite; running *actual* user scripts through both engines and diffing surface
  divergences the corpus never exercises.
- **It catches the bug class `mx check` misses** — field drift, `$Type` mistakes, pointer/order
  differences — by comparing against the *known-good* legacy output, not just "does Studio Pro
  accept it."
- **It removes the v2 migration from the critical path.** Because comparison is byte-level, the
  new engine can use `mongo-driver/v2` while the rest of the tree stays on v1 until cutover.
- **It makes cutover a one-line, reversible change** (flip the default factory; keep `legacy`
  reachable for one release as an escape hatch).
- **The tooling already exists** on engalar's branch (`cmd_bson_compare.go`, `cmd_bson_dump.go`)
  — port, don't invent.

### Engine modes

| `MXCLI_ENGINE` | Behaviour | Use |
|---|---|---|
| `legacy` (default) | `sdk/mpr` writes to disk. Current behaviour, byte-for-byte. | Production until cutover |
| `modelsdk` | new engine writes to disk. | The eventual default; opt-in early adopters |
| `compare` | `legacy` writes to disk (source of truth); `modelsdk` encodes in parallel on a copy; normalized BSON diffed; divergences logged (and failed in CI). | Development + CI gate |

> **Design note — run-both, not in-process dual backend.** `compare` mode runs the script
> through each engine on its own copy of the project and diffs the results. Do **not** build an
> in-process backend that fans each mutation to two live element trees — keeping two trees in
> lockstep mid-command is fragile and buys nothing the run-both approach doesn't. The executor
> stays single-backend; comparison is an outer harness.

### The normalization problem (load-bearing detail)

Generated `$ID`s and GUIDs differ on every run, so a raw byte diff is useless. The harness needs
a **canonicalizer** that, before diffing:

1. Collects the set of `$ID`/GUID binary values from each output and remaps them to stable
   ordinal tokens (`$ID#1`, `$ID#2`, …) in document-walk order — preserving *referential
   structure* (same ID used twice → same token) while masking the random value.
2. Masks known-volatile fields (timestamps, `dataStorageGuid` where non-deterministic).
3. Diffs the resulting canonical BSON (decoded → sorted-key JSON, or `bsoncore` element walk).

A residual diff after canonicalization is a **real** divergence. This canonicalizer is the same
primitive the golden-diff harness in the proposal needs, so build it once and share it.

## 3. Phases

Each phase has an explicit exit gate. Phases 1–3 leave `legacy` as the default the entire time.

### Phase 0 — Vendor + scaffolding + flag + harness  *(Effort: M, Risk: Low)*

- Vendor `modelsdk/` (`element`, `property`, `codec`, `gen/*`, `mpr`, `widgets`) and
  `mdl/backend/unitstore` onto `main`. Additive — `sdk/mpr` untouched. Brings `mongo-driver/v2`
  as a **coexisting** dependency.
- Vendor the codegen (`cmd/modelsdk-codegen`, `internal/codegen` deltas, `supplements.json`) so
  `gen/*` is regenerable, not just a frozen blob. **Fold in the `TypeVersionInfo` type-level
  bound fix** here (see §4).
- Add the flag seam: replace the hard-wired `SetBackendFactory` at `cmd/mxcli/main.go:205` with
  a selector reading `MXCLI_ENGINE` (env) and a hidden global `--engine` flag. `legacy` default.
- Port `cmd_bson_compare.go` / `cmd_bson_dump.go`; build the **canonicalizer** (§2) and a
  `make engine-diff` target that runs the `doctype-tests` corpus through both engines and reports
  divergences. Wire the `compare` mode.
- **Gate:** binary compiles with both engines linked; `MXCLI_ENGINE=modelsdk` reaches a stub
  backend; `make engine-diff` runs end-to-end (even if everything diverges initially).

### Phase 1 — Read-path parity  *(Effort: M, Risk: Low–Med)*

- Back the new `FullBackend`'s **read** methods with `modelsdk/mpr` + codec (reuse engalar's
  `mdl/backend/mpr/convert_reader.go` as reference).
- `compare` mode diffs the *decoded model* (entities, microflows, pages, …) read by each engine
  across the corpus **and a set of real MPRs** (v1 and v2 formats).
- **Gate:** reads are model-equivalent on every corpus project and the real-MPR set; any
  divergence is explained (benign) or fixed.

### Phase 2 — Write-path vertical slice: domain model  *(Effort: L, Risk: Med–High)*

This is the crux slice — the engine/backend seam (22 commits touch both `modelsdk/` and
`mdl/backend/`).

- Port engalar's `domainmodel_modelsdk.go`, `convert.go`, `factory.go`, `script_tx.go`, and the
  `unitstore` write path for the local backend's domain-model mutations. **Use his
  `mdl/backend/mpr` as a reference implementation, adapting imports — do not import his executor
  rework.** `FullBackend` is identical on both sides; the interface is the firewall.
- `compare` mode: run domain-model MDL (`create/alter entity`, associations, attributes,
  enumerations) through both engines; canonicalize; diff per `.mxunit`.
- Validate the new-engine output additionally with `mx check` and a Studio-Pro open on a sample.
- **Gate:** domain-model corpus produces canonically-identical BSON (or every diff is documented
  as a known, Studio-Pro-accepted improvement, e.g. roundtrip-preserved unknown fields). `mx
  check` clean.

### Phase 3 — Write-path coverage expansion  *(Effort: L, Risk: Med, parallelizable)*

Doctype by doctype, each independent, each gated by the same `compare` + `mx check` pass:
microflows → pages (`page_mutator.go`) → workflows → security → navigation → services/REST →
remaining. Pipeline-style: a doctype clears as soon as its diff is clean; no global barrier.

- **Widget parity is the special case.** Keep **our v0.12 widget serialization** (it lives on
  *our* side; his is "absent"); reconcile with his real-time `.mpk` registry. `compare` mode
  *sizes the gap precisely* instead of guessing from commit history — run it before committing
  widget effort.
- **Gate (per doctype):** clean canonical diff over its corpus slice + `mx check`; widget changes
  additionally Studio-Pro-validated (CE0463 class).

### Phase 4 — Cutover  *(Effort: S, Risk: Med)*

- Flip the default factory to `modelsdk`. Keep `MXCLI_ENGINE=legacy` reachable for **one release**
  as a documented escape hatch.
- Keep `compare` available; promote a periodic `compare` run over a project zoo to nightly CI.
- **Gate:** clean diff across the full corpus + real-project zoo for two consecutive nightly runs;
  no open Studio-Pro-rejection issues.

### Phase 5 — Cleanup  *(Effort: M, Risk: Low–Med)*

- Delete the `sdk/mpr` **write** path. Migrate the remaining tree from `mongo-driver` v1 → v2
  (~129 files, mechanical), drop the v1 dependency.
- Remove the `legacy` factory and (optionally) retire the flag to a kill-switch.
- Retire `sdk/widgets` in favour of `modelsdk/widgets` once parity is proven.

## 4. Cross-cutting work

| Item | Where | Notes |
|---|---|---|
| **`TypeVersionInfo` type-level bound fix** | `modelsdk/version` + codegen emitter | The one fidelity bug surfaced by the version analysis. Parser already captures type-level `Introduced`/`Deleted` (`JsVersionInfo`); the generated struct drops it. ~½ day. Do it during Phase-0 codegen vendoring. |
| **mongo-driver v1/v2 coexistence** | go.mod | Confirm both link in one binary (different module paths — expected to work). Validate in Phase 0 before committing to the strategy. |
| **Codegen source** | `cmd/modelsdk-codegen` | Inherit engalar's regex source short-term. The `audit`/`audit-keys` mode → **promote to CI gate** (catches unregistered `$Type`s / `ByIdRef` mismatches). The `mx dump-mpr` re-point and instantiate-and-reflect are **out of scope** here — separate, deferrable (see backend-strategy § Version handling). |
| **Version gating unchanged** | `sdk/versions/*.yaml` | Byte-identical on both branches; the engine swap does not touch it. No work. |

## 5. Decisions to confirm

1. **Vendor-as-reference vs. clean rewrite of the local backend.** Recommendation: **vendor
   engalar's `mdl/backend/mpr` modelsdk files and adapt imports** (faster, battle-tested), since
   `FullBackend` is identical — rather than rewriting the write path from scratch. Confirm.
2. **Flag surface.** `MXCLI_ENGINE` env + hidden `--engine` global. Confirm naming and whether
   `--engine` should be user-visible or hidden until Phase 4.
3. **Cutover default-flip release.** Which release carries the default flip, and how long `legacy`
   stays as an escape hatch (proposed: one release).
4. **Widget strategy at cutover.** Keep ours through cutover and retire `sdk/widgets` in Phase 5
   (recommended), vs. adopt his `modelsdk/widgets` earlier. `compare` mode informs this.

## 6. Effort & risk summary

| Phase | Effort | Risk | Default engine during phase |
|---|---|---|---|
| 0 — Vendor + flag + harness | M | Low | legacy |
| 1 — Read parity | M | Low–Med | legacy |
| 2 — Write slice (domain model) | L | **Med–High** | legacy |
| 3 — Write coverage expansion | L | Med | legacy |
| 4 — Cutover | S | Med | → modelsdk |
| 5 — Cleanup (delete sdk/mpr, v2 migration) | M | Low–Med | modelsdk |

**Long poles:** Phase 2 (the engine/backend seam) and Phase 3 widget parity. Everything before
cutover is reversible; the comparison harness is the safety net that makes the cutover decision
evidence-based rather than a leap.

## 7. First concrete steps

1. Phase 0 spike: vendor `modelsdk/` + `unitstore`, confirm **v1/v2 coexistence compiles**, wire
   the `MXCLI_ENGINE` selector at the factory seam, port `cmd_bson_compare.go`, build the
   canonicalizer, get `make engine-diff` running red-to-green on a single domain-model script.
2. If coexistence compiles and the harness runs, the rest is execution against gates. If it does
   **not** compile cleanly, escalate the v2 question to the front (a forced early tree-wide bump),
   which materially changes the Phase-0 estimate.

---

## 8. Phase-0 spike results (2026-06-05)

Ran the make-or-break parts of step 1 on the `modelsdk` branch. **The strategy's two central
premises hold; the entanglement is smaller and more contained than feared.**

### ✅ v1/v2 coexistence — PROVEN

- Added `go.mongodb.org/mongo-driver/v2 v2.6.0` alongside the existing `v1.17.9`. Both resolve in
  one module (distinct module paths).
- `go build ./modelsdk/... ./mdl/backend/unitstore/...` → **rc=0.** The vendored engine compiles
  on v2 in the same binary as our v1 tree. **The "defer the tree-wide v2 migration to cutover"
  premise is validated** — the v2 dependency is confined to `modelsdk/`.

### ✅ Dependency closure — small and clean

- `modelsdk/` reaches outside its subtree only into `model` and `mdl/types`.
- **All `model.*` symbols it needs already exist on `main`** — no `model` vendoring required.
- `mdl/types` is **bson-driver-agnostic** (`RawUnit.Contents` is `[]byte`, identical both branches),
  so it does *not* drag v2 into shared code.

### ⚠️ The one real entanglement — `mdl/types` java-action refactor

`modelsdk/` needs engalar's `mdl/types` (not just 4 additive files — `UnitPatch`/`RawUnitInfo`
extras live in *modified* existing files). Adopting his `mdl/types` wholesale, then building every
package that imports it (13, excluding the embed-noisy `cmd/mxcli`):

| Result | Packages |
|---|---|
| **build OK (10)** | all `modelsdk/*` engine pkgs, `mdl/backend`, `mdl/backend/mock`, `mdl/catalog`, `mdl/linter`(+rules), `mdl/bsonutil`, `sdk/mpr/version` |
| **break (3)** | `sdk/mpr`, `mdl/backend/mpr` (transitive via sdk/mpr), `mdl/executor` |

**All 3 failures share one root cause:** engalar consolidated the code-action types
(`CodeActionReturnType`, `JavaActionParameter`, `TypeParameter`, …) into `mdl/types` with a
different unexported marker method, while our legacy code still mixes them with the separate
`sdk/javaactions` package. Call sites: `sdk/mpr/parser_misc.go` and
`mdl/executor/cmd_javascript_actions.go`.

**Reconciliation is small and already conventional.** The fix is to make `sdk/javaactions`
re-export the `mdl/types` code-action types as aliases — exactly the CLAUDE.md rule *"new shared
types in `mdl/types/`; `sdk/mpr` re-exports as type aliases."* And `sdk/mpr` is deleted at cutover
anyway, so this is a transitional patch, not permanent debt.

### Pre-existing noise (not engine-related)

`go build ./...` also trips on make-managed generated files — the ANTLR parser (`make grammar`,
run during the spike) and `cmd/mxcli` `//go:embed changelog.md`. These are normal `make build`
steps, unrelated to the engine.

### Net & progress

The Phase-0 gate is **substantially met**: engine vendored, both drivers coexist, engine builds.
No forced early v2 migration.

**Phase-0 checklist (committed on the `modelsdk` branch):**

| Item | Status | Commit |
|---|---|---|
| Vendor `modelsdk/` + `unitstore` + read fixture; `mongo-driver/v2` coexists | ✅ done | `b1536ba7` |
| Java-action `mdl/types` reconciliation (alias re-export; fixes 3 pkgs) | ✅ done | `b1536ba7` |
| `MXCLI_ENGINE` / `--engine` selection seam (legacy wired; modelsdk/compare fail-fast) | ✅ done | `1e8ec679` |
| Vendor engalar codegen + `TypeVersionInfo` type-level bound fix | ⏳ todo | — |
| Comparison harness: port `cmd_bson_dump`/`cmd_bson_compare`, ID-canonicalizer, `make engine-diff` | ⏳ todo (run-both diff is Phase-2-gated on the modelsdk backend) | — |

`make build` green; engine, backend, and affected packages tested. Legacy path verified end-to-end
against `testdata/expr-checker/minimal.mpr` (SHOW ENTITIES / SHOW MODULES); the three engine guards
(`modelsdk`, `compare`, unknown value) all fail loud with exit 2.

### Phase 1 — read slice (started 2026-06-05)

| Item | Status | Commit |
|---|---|---|
| `mdl/backend/modelsdk` (package `modelsdkbackend`): FullBackend reading via the codec engine | ✅ first slice | `43cbb3b3` |
| Wire `MXCLI_ENGINE=modelsdk` → read backend (read-only warning; writes no-op via mock) | ✅ done | `43cbb3b3` |
| Modules read (`ListModules`/`GetModule*`) — diff-identical to legacy | ✅ done | `43cbb3b3` |
| Entities read (`ListDomainModels`/`GetDomainModel` + gen→`domainmodel` adapter) | ✅ done | `7dd42a1d` |
| Container-tree reads (`ListUnits`/`ListFolders`) for module/folder resolution | ✅ done | `f7b2a020` |
| Microflows read (`ListMicroflows`/`GetMicroflow` + flow-object/param conversion) | ✅ done | `f7b2a020` |
| Pages read (`ListPages`/`GetPage` + title/template handling) | ✅ done | `fb9664da` |
| Nanoflows read (`ListNanoflows`/`GetNanoflow`, reuses microflow helpers) | ✅ done | `24c4428d` |
| Read coverage beyond nanoflows (enums, constants, security, …) | ⏳ next | — |

Nanoflows were a clean reuse — `nanoflowFromGen` shares `splitFlowObjects`/`dataTypeFromGen`
with microflows unchanged; `SHOW NANOFLOWS` was byte-identical on the first run (13 rows). The
per-document recipe is now well-established and the remaining doc types (enums, constants,
security, etc.) are mostly mechanical, with the cross-cutting infrastructure (container tree,
text registration, prefix-match handling) already solved.

**Two more reusable lessons from pages** (both will recur across remaining doc types):

1. **Strict typing vs legacy prefix-matching.** Legacy's `listUnitsByType("Forms$Page")` is
   *prefix-matched*, so it silently sweeps in `Forms$PageTemplate` (16 pages + 46 templates = 62).
   The modelsdk reader is strict-typed and returned only the 16. To match, `ListPages` explicitly
   reads `Forms$PageTemplate` too. **Watch for other prefix collisions** (the module `ModuleImpl`
   case was the same family). The modelsdk strictness is arguably *more correct*; we replicate the
   quirk for parity and can fix both engines later.
2. **Child-element gen packages must be registered.** Page titles came back empty because
   `Texts$Text` wasn't registered — the codec decoded the title child as bare `element.Base`, and
   the interface-based `textElementToModel` silently returned nil. Fix: blank-import the child's
   gen package (`modelsdk/gen/texts`). Any converter that reaches into nested element types must
   ensure those packages' `init()` registrations have run.

**Validated:** `SHOW PAGES` byte-identical to legacy across all 62 rows (16 pages + 46 templates),
titles included.

**Third discovery — renderers need the container tree, not just the doc converter.** Microflows
read fine (16 units) but `SHOW MICROFLOWS` initially dropped *every* row: the renderer resolves
each flow's module from its `ContainerID` via `ContainerHierarchy` (`FindModuleID`/
`BuildFolderPath`), which is built from `ListModules` + **`ListUnits`** + **`ListFolders`**.
Flows are nested in folders, so without those two reads folder→module resolution fails silently.
Lesson for the remaining read domains: implementing the doc converter is necessary but not
sufficient — the supporting container/metadata reads must be present too. (`ListUnits`/`ListFolders`
are now done, so later doc types inherit working module/folder resolution.)

**Validated:** `SHOW MICROFLOWS` byte-identical to legacy across all 16 microflows and every
column (params, actions, McCabe, returns). `SHOW MODULES` aggregate counts now match legacy for
the implemented doc types (entities, microflows); the rest converge as reads land.

**Two discoveries this slice:**

1. **We own the gen→`domainmodel` adapter — there is nothing to port.** engalar changed the
   `DomainModelBackend` interface to traffic in `*genDm` types (`GetDomainModelGen`, …) and
   **deleted `sdk/domainmodel`** entirely. Keeping main's executor and `domainmodel` types
   canonical means the translation is net-new and ours. It is the concrete, recurring cost of
   "vendor, don't adopt" — each read domain is *override + delegate to `mprread` + convert*.
   The entity converter (`mdl/backend/modelsdk/domainmodel.go`) is the template.
2. **System-module injection gap.** Legacy injects the whole System module from hardcoded
   `sdk/mpr/system_module.go` (`BuildSystemDomainModel`); the codec reader returns only real
   project units, so `SHOW ENTITIES` via modelsdk omits System.*. **All 8 non-System entities
   are byte-identical to legacy** (type, Extends, every count). Closing the gap = surface the
   same synthetic System module from the modelsdk backend (reuse/relocate `BuildSystemDomainModel`,
   which is coupled to the legacy `sdk/mpr` package today). Tracked, not yet done.

**Thesis validated.** The new backend embeds `*mock.MockBackend` (satisfies all 27 sub-interfaces;
un-overridden methods are safe zero/nil stubs) and overrides only connection + module reads,
delegating to `modelsdk/mpr.Reader`. `MXCLI_ENGINE=modelsdk show modules` returns a module list
**byte-identical to legacy** (10 modules on `testdata/expr-checker`) — reads flow through
`FullBackend` on the codec engine **without importing engalar's executor/backend rework**. This is
the central "vendor, don't adopt" bet, now demonstrated on a real read. Un-ported reads (entities,
etc.) fall through to empty stubs rather than crashing.

**Next read targets** (each: override the method, delegate to `mprread`/Reader, convert gen→our
types, diff vs legacy): domain models/entities → microflows → pages. These are where the
gen→`domainmodel`/`model` conversion cost (engalar's `convert_reader.go`) gets sized for real.
