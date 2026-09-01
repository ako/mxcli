| Path | Category | Frames |
|------|----------|--------|
| `architecture/mdl-execution.md` | architecture | grammar → AST → visitor → executor → backend → MPR writer |
| `architecture/mpr-read-write.md` | architecture | MPR v1/v2, BSON round-trip, write safety |
| `architecture/widget-engine.md` | architecture | def.json, WidgetRegistry, V3 builders |
| `architecture/mcp-backend.md` | architecture | the MCP/PED backend: hybrid local-read / live-PED-write, dirty-set router, session tracking |
| `models/association-pointers.md` | mental-model | why `ParentPointer` = FROM, `ChildPointer` = TO |
| `models/element-identity.md` | mental-model | `$ID` vs `GUID` vs `StableId`; the unit as identity boundary |
| `models/ped-mutation-constraints.md` | mental-model | the counter-intuitive PED invariants: simplified constructors, leaf-only sets, acceptance is not validity |
| `models/storage-vs-qualified-names.md` | mental-model | BSON `$type` vs SDK qualified name |
| `models/version-gating.md` | mental-model | feature registry, `min_version`, `checkFeature()` |
| `rationale/mdl-as-sql.md` | rationale | why MDL is SQL-shaped, design principles (cites ADRs) |
| `rationale/backend-abstraction.md` | rationale | why the executor never imports `sdk/mpr` for writes (cites ADRs) |
| `positioning/vs-typescript-sdk.md` | positioning | gap analysis, intentional differences |
| `glossary.md` | glossary | Mendix ↔ mxcli ↔ BSON term bridge |
| `bug-patterns/bson-numeric-width.md` | bug-pattern | int32/int64 mismatches (links #583, #585 findings) |
| `bug-patterns/visitor-wiring-gaps.md` | bug-pattern | parsed-but-not-stored (links #393 finding) |
| `bug-patterns/widget-type-object-drift.md` | bug-pattern | CE0463 family |
| `bug-patterns/describe-round-trip-gaps.md` | bug-pattern | DESCRIBE as a second, unvalidated MDL implementation: won't-parse / drops / invents / destroys |
| `bug-patterns/unloadable-model-writes.md` | bug-pattern | writes that break LOAD rather than validation — no CE code, whole project down |
| `bug-patterns/silent-property-drop.md` | bug-pattern | a property parses, passes every check, and never reaches the model |
| `bug-patterns/check-mxbuild-drift.md` | bug-pattern | `mxcli check` as a model of mxbuild, drifting in both directions |
| `bug-patterns/platform-semantics-gaps.md` | bug-pattern | legal MDL, illegal Mendix — expression / XPath / variable-scope rules the grammar cannot carry |
| `bug-patterns/duplicate-resolver-drift.md` | bug-pattern | one question answered in two places, and the two disagree |
| `bug-patterns/rewrite-drops-unauthored-state.md` | bug-pattern | CREATE OR REPLACE losing what the statement did not mention |
| `bug-patterns/flow-graph-geometry.md` | bug-pattern | generated microflow coordinates and sequence-flow wiring |
| `bug-patterns/integration-contract-drift.md` | bug-pattern | OData / REST / mappings, where neither check nor mxbuild is an oracle |
| `bug-patterns/test-runner-cannot-fail.md` | bug-pattern | `mxcli test` reporting PASS for what did not hold, run or get evaluated |
| `bug-patterns/local-loop-silence.md` | bug-pattern | the warm loop failing quietly across processes mxcli does not own |
| `bug-patterns/styling-compiles-to-nothing.md` | bug-pattern | SCSS and tokens written correctly, compiled nowhere, checked by nothing |
| `bug-patterns/package-operations-damage.md` | bug-pattern | marketplace / `mx` operations that change the project and report success |
| `bug-patterns/cli-contract-defects.md` | bug-pattern | flags, paths, stdout and help — the class with no Mendix document in it |
| `bug-patterns/engine-divergence.md` | bug-pattern | two backend implementations, and a gap in one that is invisible from inside it |
| `bug-patterns/mutator-addressing.md` | bug-pattern | in-place edits to nodes the model does not name |
| `bug-patterns/access-rule-reconciliation.md` | bug-pattern | GRANT is a read-modify-write, and both directions of loss report success |
| `bug-patterns/capability-gap-as-parse-error.md` | bug-pattern | an unspelled capability presenting as `no viable alternative`, and the workarounds that follow |
| `bug-patterns/keyword-collisions.md` | bug-pattern | MDL's keyword set occupying positions where user data lives |
| `bug-patterns/scripts-that-cannot-rerun.md` | bug-pattern | statement-level idempotence, and why it is not write-level idempotence |
| `bug-patterns/expression-translation-drift.md` | bug-pattern | MDL expression to Mendix expression, where the translation changes the meaning |
| `bug-patterns/misleading-diagnostics.md` | bug-pattern | hints that fire on the wrong thing and send the reader somewhere the problem is not |
