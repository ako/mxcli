# Fix Issue Skill

A fast-path workflow for diagnosing and fixing bugs in mxcli. Each fix appends
to the symptom table below, so the next similar issue costs fewer reads.

## How to Use

1. Match the issue symptom to a row in the table — go straight to that file.
2. Follow the fix pattern for that row.
3. Write a failing test first, then implement.
4. After the fix: **add a new row** to the table if the symptom is not already covered.

---

## Symptom → Layer → File Table

| Symptom | Root cause layer | First file to open | Fix pattern |
|---------|-----------------|-------------------|-------------|
| `describe` shows `$var = list operation ...;` | Missing parser case | `sdk/mpr/parser_microflow.go` → `parseListOperation()` | Add `case "microflows$XxxType":` returning the correct struct |
| `describe` shows `$var = action ...;` | Missing formatter case | `mdl/executor/cmd_microflows_format_action.go` → `formatActionStatement()` | Add `case *microflows.XxxAction:` with `fmt.Sprintf` output |
| `describe` shows `$var = list operation %T;` (with type name) | Missing formatter case | `mdl/executor/cmd_microflows_format_action.go` → `formatListOperation()` | Add `case *microflows.XxxOperation:` before the `default` |
| Compile error: `undefined: microflows.XxxOperation` | Missing SDK struct | `sdk/microflows/microflows_actions.go` | Add struct + `func (XxxOperation) isListOperation() {}` marker |
| `TypeCacheUnknownTypeException` in Studio Pro | Wrong `$type` storage name in BSON write | `sdk/mpr/writer_microflow.go` | Check the storage name table in CLAUDE.md; verify against `reference/mendixmodellib/reflection-data/` |
| CE0066 "Entity access is out of date" | MemberAccess added to wrong entity | `sdk/mpr/writer_domainmodel.go` | MemberAccess must only be on the FROM entity (`ParentPointer`), not the TO entity — see CLAUDE.md association semantics |
| CE0463 "widget definition changed" | Object property structure doesn't match Type PropertyTypes | `sdk/widgets/templates/` | Re-extract template from Studio Pro; see `sdk/widgets/templates/README.md` |
| Parser returns `nil` for a known BSON type | Unhandled `default` in a `parseXxx()` switch | `sdk/mpr/parser_microflow.go` or `parser_page.go` | Find the switch by grepping for `default: return nil`; add the missing case |
| MDL check gives "unexpected token" on valid-looking syntax | Grammar missing rule or token | `mdl/grammar/MDLParser.g4` + `MDLLexer.g4` | Add rule/token, run `make grammar` |
| CE7054 "parameters updated" / CE7067 "does not support body entity" after `send rest request` | `addSendRestRequestAction` emitted wrong BSON: all params as query params, BodyVariable set for JSON bodies | `mdl/executor/cmd_microflows_builder_calls.go` → `addSendRestRequestAction` | Look up operation via `fb.restServices`; route path/query params with `buildRestParameterMappings`; suppress BodyVariable for JSON/TEMPLATE/FILE via `shouldSetBodyVariable` |
| `CREATE X` returns "already exists — use create or replace to overwrite" but OR REPLACE is not valid for that type | Error message in executor points to wrong keyword | `mdl/executor/cmd_<type>_*.go` — find the `NewAlreadyExistsMsg` call | Change hint from `or replace` to `or modify`; verify the AST stmt uses `CreateOrModify` not `CreateOrReplace` |
| `mx check` CE0126 "Missing value for parameter X" on `call java action ... ($Param = empty)` for typed (non-entity, non-microflow) parameters | Builder emitted `BasicCodeActionParameterValue.Argument: ""` instead of the literal `"empty"` keyword | `mdl/executor/cmd_microflows_builder_calls.go` → `addCallJavaActionAction` | Capture all resolved BasicParameterType params into `resolvedBasicParams`; when bound to MDL `empty`, emit `Argument: "empty"` so Studio Pro recognises an explicit empty literal rather than treating the slot as missing |
| `DESCRIBE microflow` puts shared activities inside an `if … then` block — they should appear after `end if;` | Nested guard split inside `traverseFlowUntilMerge` crosses the outer merge boundary | `mdl/executor/cmd_microflows_show_helpers.go` — guard path in `traverseFlowUntilMerge` (~line 854) | Add `if contID != mergeID` guard before the `isMerge` skip-through so the guard continuation never crosses the outer merge |
| MDL widget property `mxcli check`s clean but Studio Pro renders the default (e.g. `dataview ... (FormOrientation: Vertical)` always Horizontal) | V3 grammar generic-property branch parks the value in `w.Properties`, but the V3 builder never reads it and the writer never emits it; the widget struct has no field for it | `mdl/executor/cmd_pages_builder_v3_widgets.go` (`buildXxxV3`) + `sdk/mpr/writer_widgets_*.go` (`serializeXxx`) + `sdk/pages/pages_widgets_*.go` | Add field to `pages.Xxx`; read via `w.GetStringProp` / `w.GetIntProp`; write in `serializeXxx`. If the Studio Pro UI label differs from the BSON storage name (e.g. DataView "Form Orientation" → `LabelWidth: 0/N`), confirm by diffing a Studio Pro-saved page against the reflection-data defaults |
| `ALTER <DOC TYPE> ...` parse error "no viable alternative at input 'ALTER<TYPE>'" but `CREATE <DOC TYPE> ...` works | The grammar lists ALTER variants explicitly per document type; a new doc type was added to `createStatement` but not to `alterStatement` | `mdl/grammar/MDLParser.g4` `alterStatement` block + dedicated visitor + new `Alter*Stmt` AST type + `register_stubs.go` | Add ALTER rule (mirror the closest sibling — `ALTER ODATA CLIENT` for SET-only, `ALTER PUBLISHED REST SERVICE` for SET+ADD/DROP); regenerate grammar; add `Alter*Stmt` with `Changes map[string]string`; route via `exitAlterStatement` dispatcher; register handler. Add the new AST type to `registry_test.go`'s `allKnownStatements()` |
| CE7247 "The name 'X' is a reserved word." on non-persistent entity attributes (Owner/Type/Context/Id/CreatedDate/ChangedDate/ChangedBy) — mxcli accepts the MDL silently, Studio Pro rejects the project | `ValidateEntity` early-returned for NPEs; reserved-word check was not wired into the executor | `mdl/executor/cmd_enumerations.go` (`ValidateEntity`) and `mdl/executor/cmd_entities.go` (`execCreateEntity`) | Drop the `EntityPersistent` early-return; gate only `mendixSystemAttributeNames` (MDL020) to persistent; run `mendixReservedWords` (MDL021) for all kinds; call `ValidateEntity` from `execCreateEntity` before any backend write. Issue #552 |
| CE3637 "A data view cannot listen to the selection of Gallery 'X', because it is not available here." on a master-detail page generated from MDL | Gallery's `itemSelectionMode` pluggable-widget property was hardcoded to `clear` in the def.json; Mendix requires `toggle` for selection-listeners (DataViews bound via `DataSource: selection X`) to see the gallery's selection | `sdk/widgets/definitions/gallery.def.json` | Change the `itemSelectionMode` mapping from `value: "clear"` to `source: "ItemSelectionMode", default: "clear"` so MDL can write `gallery X (Selection: Single, ItemSelectionMode: toggle)`. The V3 engine's generic mapping path (`widget_engine.go` `resolveMapping` default case → `GetStringProp`) reads the value automatically — no grammar changes needed. Also add `ForceFullObjects: false` to `Forms$ListenTargetSource` serializer in `sdk/mpr/writer_widgets_display.go` |
| DESCRIBE drops `DataSource: selection X` for a DataView bound to a gallery/listview selection (master-detail pages) | `extractDataViewDataSource` only handled `Forms$MicroflowSource` / `Forms$NanoflowSource` / `Forms$DataViewSource` / `Forms$DatabaseSource`; `Forms$ListenTargetSource` fell through to `return nil` | `mdl/executor/cmd_pages_describe_parse.go` (`extractDataViewDataSource`) + `mdl/executor/cmd_pages_describe_output.go` (DataView case) + `mdl/executor/cmd_pages_describe.go` (rawDataSource doc-comment) | Add `case "Forms$ListenTargetSource":` returning `{Type: "selection", Reference: ds["ListenTarget"]}`; add `case "selection":` in the DataView output switch emitting `DataSource: selection <ref>` |
| CE0463 "widget definition changed" on every pluggable widget that contains a caption/template parameter (gallery, datagrid2 captions, dynamictext with ContentParams) on Mendix 11.9 — cascades into CE3637 on master-detail pages | `serializeClientTemplateParameter` emitted `Forms$FormattingInfo` with a `TimeFormat: "HoursMinutes"` field, but the FormattingInfo reflection schema only declares CustomDateFormat / DateFormat / DecimalPrecision / EnumFormat / GroupDigits — the extra field made Studio Pro mark the embedded WidgetType as drifted | `sdk/mpr/writer_widgets.go` `serializeClientTemplateParameter` + `mdl/backend/mpr/widget_builder.go` (mirror copy of the same FormattingInfo block) | Drop the `TimeFormat` entry from both writers. Verify by diffing your BSON against a Studio Pro-saved page's FormattingInfo block — if you see keys outside the reflection schema, that's the trigger |
| Pluggable widget `Selection` BSON value is lowercase (`single` / `multi` / `none`) but Studio Pro stores PascalCase — contributes to CE0463 drift on stricter widgets and looks wrong in diffs | MDL passes the user's typed value verbatim to `SetSelection`; the builder didn't normalise | `mdl/backend/mpr/widget_builder.go` `SetSelection` | Canonicalise via `canonicalSelectionValue` helper (lowercase-keyed switch → `Single` / `Multi` / `None`); unknown values pass through |
| Nightly `mx check` reports `CE0117 "Error(s) in expression." at Log message activity 'Log message (warning)'` on Mendix 10.24.19+ but not 10.24.16 or 11.x | Mendix 10.24.19 tightened expression validation: `toString(<string>)` is now a type error (toString expects a non-string input). An example called `toString($OrderNumber)` where `$OrderNumber` was already a string parameter | The offending `log warning ... with ({1} = toString($stringVar))` — find via `~/.mxcli/mxbuild/{ver}/modeler/mx check`, then bisect with `drop microflow ...` until CE0117 disappears | Remove the redundant `toString()` wrapper around already-string values. Only wrap non-string values (integers, decimals, dates, enums) in `toString()`. The Mendix 11.x parser is more lenient and lets this slide, but 10.24.19+ rejects it |
| Mendix can't resolve the microflow named in `CREATE ODATA CLIENT (ConfigurationMicroflow: microflow X.Y)` / `ErrorHandlingMicroflow:` — error names the literal string `"MICROFLOW X.Y"` as the missing microflow | Case-mismatched prefix strip: visitor emits uppercase `"MICROFLOW "` from `odataValueText`, but `extractMicroflowRef` only trimmed lowercase `"microflow "`, so the keyword survived into BSON | `mdl/executor/cmd_odata.go` → `extractMicroflowRef` | Use a case-insensitive strip: `if strings.EqualFold(ref[:10], "microflow ") { return ref[10:] }`. Whenever a value goes from a visitor that emits a keyword-prefixed form to an executor that strips it, the strip must match the case the visitor produces — grep visitor files for `"MICROFLOW " +`/`"ENTITY " +`/etc. when adding a new property. Issue #573 |
| `describe`/catalog reports a numeric BSON field (Length, MinOccurs, MaxOccurs, MaxLength, FractionDigits, TotalDigits, Interval, NumberOfPagesToClose, …) as `0` / `unlimited` even though Studio Pro shows a real value | BSON numeric width mismatch — Studio Pro writes the field as `int64`, but the parser asserted `raw["X"].(int32)` so the type assertion failed silently and the field defaulted to its zero value | `sdk/mpr/parser_*.go` — grep for the field name; the fix point is the narrow assertion | Replace narrow type assertions on BSON numeric fields with the existing `extractInt(raw["X"])` helper (`sdk/mpr/parser.go`). It handles int32/int64/int/float64. When a non-zero default must survive a missing field, gate with `if _, ok := raw["X"]; ok { … = extractInt(...) }`. Sweep `grep -n '\.(int32)' sdk/mpr/parser_*.go` and ignore matches whose comment says "marker" (BSON-array-prefix probes are intentional). Issues #583, #585 |
| Studio Pro shows a dropdown / property as its default value even though MDL set it explicitly (e.g. CREATE ODATA CLIENT with `ConfigurationMicroflow:` set, but the "Configuration source" dropdown reads "Constants only") | The BSON key mxcli writes isn't what Studio Pro reads — either the key name is wrong, or multiple dropdown options actually share a single field discriminated by something other than the key name (return type of a referenced microflow, sibling property, etc.) | The `serializeXxx` function in `sdk/mpr/writer_*.go` for the affected document type | (1) Ask the user to duplicate the offending object in Studio Pro and **explicitly pick each dropdown option** on the duplicate(s). An unconfigured duplicate just looks like "Constants only" and tells you nothing about which field the option uses. (2) Re-dump the duplicates from `mprcontents/**/*.mxunit` **after every Studio Pro change** — cached `/tmp/svc-*.json` files go stale the instant the user edits the project (see [[feedback-refresh-bson-dumps]]). (3) Diff against mxcli's output to find the renamed key. (4) Don't assume one-state-per-key. The OData "Configuration microflow" / "Headers microflow" case stores BOTH options in the single `ConfigurationMicroflow` BSON field — Studio Pro picks the dropdown label from the referenced microflow's return type. When a discriminator like that exists, have both MDL keywords write to the same model field. Issues #573, #587 and 2026-05-27 unify-config-microflow fix |
| `DESCRIBE` of a document type omits a property the user set in the CREATE (e.g. enum-level `/** doc */` vanishes after a roundtrip; `CREATE OR REPLACE` silently runs as plain CREATE for some types) — even though the AST struct has the field, the model has the field, the writer serialises it, and DESCRIBE prints it | Visitor never copied the parsed value into the AST struct — every layer below the visitor is wired but the visitor's `ExitCreateXxxStatement` is missing the assignment | `mdl/visitor/visitor_<type>.go` → `ExitCreateXxxStatement` | Diff the visitor against a known-good sibling (e.g. enumeration vs constant). Standard wiring: `stmt.Documentation = findDocCommentText(ctx)` for doc-comments; `if createStmt.OR() != nil && (createStmt.MODIFY() != nil \|\| createStmt.REPLACE() != nil) { stmt.CreateOrModify = true }` for OR MODIFY/REPLACE. When adding a new CREATE statement, grep `mdl/visitor/visitor_constant.go` and copy these two blocks verbatim. Issue #393 |
| `create [or modify] association ... to System.X` passes `mxcli check --references` and `mxcli diff` but fails at `mxcli exec` with `child entity not found: System.X` | Two divergent entity resolvers: the write path's `findEntity` resolved the owning module via `h.FindModuleID(dm.ID)`, but the virtual System domain model is not a real unit, so the hierarchy walk yielded an empty module name and System entities never matched. The validation path (`buildEntityQualifiedNames`) keyed on `dm.ContainerID` and worked, hence the check-passes/exec-fails split | `mdl/executor/oql_type_inference.go` → `findEntity` | Resolve the module from `dm.ContainerID` (the module ID `BuildSystemDomainModel` sets), not by walking up from the DM's own unit ID. When a symptom is "passes check/diff but fails exec," suspect two resolvers and make the write-path one match the validation-path one; add an `exec`-level test, not just a `check` test. Issue #610 |
| `declare $x list of T = empty;` (or any list-typed `declare`) passes `mxcli check` but Studio Pro rejects with CE0053 ("type not allowed") + CE0038 ("value required") | `declare` maps to a Create Variable activity, which Mendix forbids from producing a list — but the validator only flagged an empty list *used as a loop source* (MDL002), never the declaration itself | `mdl/executor/validate_microflow.go` → `walkBody` `*ast.DeclareStmt` case | Emit `MDL040` (SeverityError) for any `stmt.Type.Kind == ast.TypeListOf`, regardless of initializer. Lists must come from a microflow parameter, a `retrieve`, or `$x = create list of T;`. Also fix the synced skills that present declare-list as valid (`write-microflows.md`, `cheatsheet-variables.md`, `check-syntax.md`, `patterns-*`). Issue #607 |

---

## TDD Protocol

Always follow this order — never implement before the test exists:

```
Step 1: write a failing unit test (parser test or formatter test)
Step 2: Confirm it fails to compile or fails at runtime
Step 3: Implement the minimum code to make it pass
Step 4: run: /c/users/Ylber.Sadiku/go/go/bin/go test ./mdl/executor/... ./sdk/mpr/...
Step 5: add the symptom row to the table above if not already present
```

Parser tests go in `sdk/mpr/parser_<domain>_test.go`.
Formatter tests go in `mdl/executor/cmd_<domain>_format_<area>_test.go`.

---

## Issue #212 — Reference Fix (seeding example)

**Symptom:** `describe microflow` showed `$var = list operation ...;` for
`microflows$find`, `microflows$filter`, `microflows$ListRange`.

**Root cause:** `parseListOperation()` in `sdk/mpr/parser_microflow.go` had no
cases for these three BSON types — they fell to `default: return nil`.

**Files changed:**
| File | Change |
|------|--------|
| `sdk/microflows/microflows_actions.go` | Added `FindByAttributeOperation`, `FilterByAttributeOperation`, `RangeOperation` |
| `sdk/mpr/parser_microflow.go` | Added 3 parser cases |
| `mdl/executor/cmd_microflows_format_action.go` | Added 3 formatter cases |
| `mdl/executor/cmd_microflows_format_listop_test.go` | Added 4 formatter tests |
| `sdk/mpr/parser_listoperation_test.go` | New file, 4 parser tests |

**Key insight:** `microflows$ListRange` stores offset/limit inside a nested
`CustomRange` map — must cast `raw["CustomRange"].(map[string]any)` before
extracting `OffsetExpression`/`LimitExpression`.

---

## After Every Fix — Checklist

- [ ] Failing test written before implementation
- [ ] `go test ./mdl/executor/... ./sdk/mpr/...` passes
- [ ] New symptom row added to the table above (if not already covered)
- [ ] PR title: `fix: <one-line description matching the symptom>`
