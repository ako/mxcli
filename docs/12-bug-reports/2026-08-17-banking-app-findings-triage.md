# Triage: mxcli-banking FINDINGS.md

Source: <https://github.com/ako/mxcli-banking/blob/main/FINDINGS.md> (966 lines, 8 slices)
Reported against: mxcli `nightly-20260815-0dda3a76`, Mendix 11.13.0
Triaged against: `5c70014` (main), 2026-08-17

The banking app is a full CRUD/security/transfer app built end-to-end with MDL
and verified in a real browser. Its findings file is the most detailed external
report mxcli has had. This document separates the **mxcli defects** from the
Mendix-platform behaviour the report also (correctly) records, and pins a root
cause for each defect that reproduced here.

Every "reproduced" claim below was measured on this checkout with a real
project. Claims that could not be measured here say so, and say why.

---

## Verdict summary

| # | Finding | Verdict | Root cause pinned |
|---|---|---|---|
| 1 | `DROP ATTRIBUTE` orphans the validation rule (CE1613) | **Confirmed defect** | yes |
| 2 | Dropping the *last* attribute is a silent no-op | **New defect** (not in the report) | partially |
| 3 | Microflow "Apply entity access" not settable — datasource leak | **Confirmed gap** | yes |
| 4 | Lint CONV010 false-positives on every `ACT_` microflow | **Confirmed defect** | yes |
| 5 | Lint QUAL004 misses datasource/action/nav references | **Confirmed defect** | yes |
| 6 | `mxcli syntax` prints `NON_PERSISTENT`, parser wants `NON-PERSISTENT` | **Confirmed doc defect** | yes |
| 6b | Synced skill documents a *third*, non-parsing spelling | **New defect** (not in the report) | yes |
| 7 | ComboBox rejects `onChangeEvent` | **Confirmed gap** | yes |
| 8 | `mxcli exec` applies scripts `mxcli check` rejects | **Confirmed gap** | yes |
| 9 | `mxcli check` does not validate `ALTER SETTINGS MODEL` keys | **Confirmed** | yes |
| 10 | Optimistic locking not settable from MDL | **Fixed** — see below | yes (key identified) |
| 11 | `DROP USER ROLE` leaves demo users dangling | **Confirmed defect** | yes |
| 12 | `SHOW MESSAGE` clause-order error is misleading | **Confirmed UX defect** | yes |
| 13 | `ALTER MODULE/PAGE SET DOCUMENTATION` is a parse error | **Confirmed gap** | yes |
| 14 | OQL view entities are write-only in MDL | **Refuted** — measured on 11.13, every operation works | n/a |
| — | Everything else (Mendix platform behaviour) | **Correct, not our bug** | n/a |

---

## 1. `DROP ATTRIBUTE` orphans the validation rule — the cleanup is dead code

**Reported:** slice 1. `ALTER ENTITY … DROP ATTRIBUTE FullName` removed the
attribute and left the validation rule its `NOT NULL` had created, pointing at
an attribute that no longer exists → `CE1613`. Unrecoverable with mxcli alone:
there is no `DROP VALIDATION RULE`, and `DESCRIBE ENTITY` does not emit
validation rules, so the orphan is invisible to a describe→edit→exec round trip.
The reporter had to tear down and rebuild the entity and everything referencing
it.

**Status: confirmed, with a one-line root cause.** This is the highest-severity
item in the report and the cheapest to fix.

Minimal reproduction (this checkout, `sdk/mpr/testdata/v1-project`):

```
create module B4
create persistent entity B4.C ( Nm: String(100) not null error 'req', Age: Integer )
alter entity B4.C drop attribute Nm
```

Resulting BSON:

```json
"Attributes": ["Age"],
"ValidationRules": [
  { "$Type": "DomainModels$ValidationRule",
    "Attribute": "B4.C.Nm",
    "RuleInfo": { "$Type": "DomainModels$RequiredRuleInfo" } }
]
```

Note the executor prints no `Removed 1 validation rule(s)` line — the cleanup
it already has did not fire.

### Root cause

`mdl/executor/cmd_entities.go` (DROP ATTRIBUTE) removes rules by element ID:

```go
droppedID := entity.Attributes[idx].ID
for _, vr := range entity.ValidationRules {
    if vr.AttributeID != droppedID { keepRules = append(keepRules, vr) }
}
```

But `sdk/mpr/parser_domainmodel.go:641` (`parseValidationRule`) stores a
**qualified name** in that field, because Mendix stores the rule's attribute
reference as a `BY_NAME_REFERENCE` string, not an ID:

```go
} else if attrName, ok := attrRef.(string); ok {
    // Store qualified name as ID - will need to resolve later
    rule.AttributeID = model.ID(attrName)
}
```

The comment says "will need to resolve later" — it never is. `"B4.C.Nm"` is
never equal to a binary element ID, so **the cleanup can never match a rule read
from disk**, which is every rule Studio Pro or a prior mxcli run wrote. The
cleanup code has been in place since `5dc4a46` (2026-07-29) — before the nightly
the banking app used — and has been dead the whole time.

The writer already handles the dual meaning (`serializeValidationRule`,
`writer_domainmodel.go:1291` explicitly branches on `strings.Contains(attrIDStr, ".")`).
The executor does not.

**The same bug hits `MemberAccess`.** BSON stores those as qualified-name
strings too (verified in the same dump: `"Attribute": "NanoflowCommons.Geolocation.Timestamp"`),
and the DROP ATTRIBUTE handler filters them with the identical
`ma.AttributeID != droppedID` comparison. That is a plausible source of the
report's `CE0066 "Entity access is out of date"` as well.

### Fix

Compare on both spellings in the executor — or better, resolve the qualified
name to an element ID once at parse time so the model layer has one
representation. Index attributes should be checked for the same confusion.
Add the missing `DESCRIBE ENTITY` output for validation rules so an orphan is at
least *visible*, and consider `ALTER ENTITY … DROP VALIDATION RULE` for cleanup
of orphans already in the wild — without it, every project that has already hit
this needs Studio Pro.

---

## 2. Dropping the last attribute of an entity is a silent no-op — NEW

Not in the report; found while reproducing #1.

```
create module B6
create persistent entity B6.C ( Age: Integer, Nm: String(50) )
alter entity B6.C drop attribute Age   -> works
alter entity B6.C drop attribute Nm    -> "Dropped attribute 'Nm' from entity B6.C"
```

After the second drop the file hash is **unchanged** and
`describe entity B6.C` still shows `Nm: String(50)`. No validation rules are
involved; `MXCLI_ALWAYS_WRITE=1` does not change the outcome, so this is not
write elision — the model mutation itself never lands. The command reports
success either way.

Severity is lower than #1 (dropping every attribute is rare) but the failure
mode — reporting success while doing nothing — is the worst shape a write can
have, and it is the same shape as #11 and the `revoke` bug in
`revoke_orphaned_rules.md`. Worth a targeted test at the writer level.

---

## 3. Microflow "Apply entity access" is not settable — the datasource leak

**Reported:** slice 2, marked "the most important finding so far". A microflow
retrieve ignores entity access unless the microflow's *Apply entity access*
property is set. MDL has no syntax for it, so a datasource microflow that does
not spell out its own ownership constraint **retrieves every customer's rows**.
Access rules still blank the attributes at render time, so the page looks
correct while other people's objects have been loaded. Second-order damage
measured: `LIMIT 5` was applied *before* the access filter, so the user lost one
of their own rows to make room for rows they cannot read.

The report also (correctly) corrects its own slice 1 conclusion because of this,
and derives the right test rule: assert row *counts*, because "the other
customer's ID is not in the DOM" cannot distinguish "not retrieved" from
"retrieved and blanked".

**Status: confirmed.** Both engines hardcode the property to `false`:

- `mdl/backend/modelsdk/microflow_write.go:187` — `out.SetApplyEntityAccess(false)`
- `sdk/mpr/writer_microflow.go:96` — `{Key: "ApplyEntityAccess", Value: false}`

`microflowOption` in `mdl/grammar/domains/MDLMicroflow.g4:103` accepts only
`FOLDER` and `COMMENT`. The `APPLY` lexer token exists (`MDLLexer.g4:686`) but
is used only by settings rules. `MicroflowsMicroflow.ApplyEntityAccess` and
`MicroflowsRule.ApplyEntityAccess` are both present in `generated/metamodel`,
so nothing is missing at the metamodel layer.

This is a security-relevant default. mxcli currently makes it impossible to
write a microflow datasource that is safe by construction — the only available
mitigation is the one the reporter used (hand-write the XPath constraint in
every retrieve), and nothing in mxcli tells you that.

### Fix

Add a microflow header option (`APPLY ENTITY ACCESS`), wire it through
AST/visitor/executor to both engines, emit it from `DESCRIBE MICROFLOW`, and —
separately — consider a lint rule for the actual defect: a microflow used as a
page datasource that retrieves a security-constrained entity with neither
`ApplyEntityAccess` nor an XPath constraint mentioning `[%CurrentUser%]`. That
rule would have caught this app's bug in slice 1 rather than slice 2.

---

## 4. CONV010 flags exactly what it documents as allowed

**Reported:** slice 3. 11 false positives out of 13 findings, burying 2 real
ones.

**Status: confirmed, verbatim.** `.claude/lint-rules/conv010_act_microflow_content.star`:

```python
ALLOWED_ACTIONS = ("ShowFormAction", "CloseFormAction", "ShowHomeFormAction",
                   "ShowMessageAction", "DownloadFileAction")
ALLOWED_ACTIVITY_TYPES = ("SubMicroflow", ...)
```

Those are the **BSON storage names** (per the storage-name table in CLAUDE.md).
The linter never sees them: `getMicroflowActionType`
(`mdl/catalog/builder_microflows.go:284`) derives `action_type` from the **Go
type name** —

```go
return strings.TrimPrefix(fmt.Sprintf("%T", action), "*microflows.")
```

— giving `ShowPageAction`, `ClosePageAction`, `MicroflowCallAction`. So the
allowlist matches nothing, and every `ACT_` microflow that shows a page, closes
a page, or calls a sub-microflow is flagged.

The reporter's fix (list both spellings) is right. Audited the other 28 bundled
rules for the same stale vocabulary: **CONV010 is the only one affected.**

A test that asserts each rule's action-name constants exist in the catalog's
vocabulary would prevent the whole class.

---

## 5. QUAL004 misses three kinds of reference, not one

**Reported:** slice 1, as a false positive on page datasources.

**Status: confirmed, and broader than reported.**
`.claude/lint-rules/orphaned_elements.star` counts a microflow as referenced
only when:

```python
if ref.ref_kind == "call" or ref.ref_kind == "schedule":
```

The reference builder emits nine other kinds (`builder_references.go:18-36`).
For microflows the ones that mean "this is an entry point" and are ignored:

- `datasource` — a page/widget microflow datasource (the reported case; the
  edge **is** emitted, `extractDataSourceRefs` handles `*pages.MicroflowSource`)
- `action` — a widget button calling a microflow
- `calculate` — a calculated attribute's microflow

The page half of the rule has the same shape: it counts only `show_page`, and
ignores `home_page`, `login_page` and `menu_item`. A page reachable only from
navigation is reported orphaned; the `ENTRY_PAGE_PATTERNS` list
(`Home`/`Login`/`Index`/`Dashboard`) masks this for exactly the pages most
likely to be navigation targets, which is why it has gone unnoticed.

The `schedule` kind was added for precisely this reason (see the comment in
`builder_references.go:485`); the other five were not.

---

## 6. `NON_PERSISTENT` in the syntax help does not parse

**Reported:** slice 2.

**Status: confirmed.** The lexer *token* is named `NON_PERSISTENT` but matches a
hyphen (`MDLLexer.g4:32`: `NON_PERSISTENT: N O N '-' P E R S I S T E N T;`).
`cmd/mxcli/syntax/features_domain_model.go` lines 27 and 40 print the token name
into user-facing syntax. Measured:

```
CREATE NON-PERSISTENT ENTITY Test.Foo ( Name: String(100) );   -> Syntax OK
CREATE NON_PERSISTENT ENTITY Test.Foo ( Name: String(100) );   -> no viable alternative
```

Two-line fix. Also present in `docs-site/src/language/lexical-structure.md`,
`docs/05-mdl-specification/01-language-reference.md` and
`docs/06-mdl-reference/grammar-reference.md`, where it is a token-name listing
and therefore arguably correct — but the syntax help is not.

### 6b. A synced skill documents a third spelling that also does not parse — NEW

Not in the report. `.claude/skills/mendix/rest-call-from-json.md` — which
`mxcli init` **ships into every user project** — uses a different form entirely,
five times:

```
create entity Module.MyRootObject (NON_PERSISTENT)
```

Measured: `mismatched input ')' expecting ':'`. So the skill mxcli hands to
agents in user projects teaches syntax that cannot parse. Worse than the syntax
help, because it is the thing an LLM reads first.

---

## 7. ComboBox rejects `onChangeEvent`, and the template already carries it

**Reported:** slice 2. `OnChange` works on `DATEPICKER`; every spelling is
MDL-WIDGET01 "has no property" on `COMBOBOX`, even though the generated widget
doc lists `onChangeEvent` and `onChangeDatabaseEvent`. The consequence is
silent: an unwired account picker does nothing, so the page had to be redesigned
around an explicit button.

**Status: confirmed, and the machinery is already there.**

- `sdk/widgets/definitions/combobox.def.json` declares four `knownProperties`,
  all `optionsSourceAssociation*`. Neither change event is among them.
- `sdk/widgets/templates/mendix-11.6/combobox.json` **does** contain both
  `onChangeEvent` and `onChangeDatabaseEvent`.

So the widget can carry the property and the doc generator can see it; only the
`.def.json` allowlist blocks it. The doc/def mismatch is itself worth a test:
a property the generated doc advertises should be a property MDL accepts.

---

## 8. `mxcli exec` has no pre-flight gate

**Reported:** slice 2. "A page with an invalid widget property was written to
the model anyway. Always run `check` before `exec`."

**Status: confirmed.** `cmd/mxcli/cmd_exec.go` has two flags (`--project`,
`--continue-on-error`) and no validation step. Parse errors do stop it — I
verified that — but the semantic checks `mxcli check` runs (MDL-WIDGET*, MDL0xx,
reference validation) are never invoked by `exec`.

That makes the documented workflow "always run check first" a convention nothing
enforces, on a tool whose whole point is unattended agent use. `exec` should run
the same semantic checks and refuse on errors, with `--no-check`/`--force` to
opt out.

---

## 9–10. Settings: unvalidated keys, and optimistic locking

**Reported:** slice 4.

**#9 confirmed:** `ALTER SETTINGS MODEL OptimisticLocking = true;` passes
`mxcli check` cleanly and fails only at `exec` with `unknown model setting`. The
grammar accepts any identifier there. The reporter's summary is the right one:
"check passed" means the text parses, not that the statement means anything.
`check` should validate settings keys against the same table `exec` uses.

**#10 fixed.** The report's correction is accurate — Mendix *does* ship optimistic
locking as an app setting, and it is exactly the mitigation for the read-then-write
balance race the app documents.

The stored property is `EnableDataStorageOptimisticLocking` on
`Settings$ModelSettings`, verified against Studio-Pro-created projects on **both**
9.24 and **11.13.0**. Unlike `JavaVersion` → `JavaMajorVersion` (which did rename
between 11.6 and 11.12, exactly as CLAUDE.md warns) this key did not move, so one
spelling is correct for every version mxcli supports.

Every layer except one was already in place — `model.ModelSettings` carried the
field, and both engines read and wrote it (`sdk/mpr/parser_settings.go:164`,
`writer_settings.go:102`, `mdl/backend/modelsdk/settings_{read,write}.go`). The
only thing missing was the executor's assignment case, so the property was
round-tripped faithfully on every write while being unreachable from MDL.

```
alter settings model EnableDataStorageOptimisticLocking = true;
```

Measured on the 11.13.0 project: the key lands in the BSON as
`"EnableDataStorageOptimisticLocking": true`, `JavaMajorVersion` is preserved
untouched beside it, `mx check` reports **0 errors**, re-running the statement
leaves the `.mpr` byte-identical (with `MXCLI_ALWAYS_WRITE=1` as the control run
that does churn), and a non-boolean value is refused by `mxcli check` (MDL-SET02)
and by the write.

The unknown-key error now lists the accepted keys. The report tried
`OptimisticLocking`, `UseOptimisticLocking` and `EnableOptimisticLocking` in turn
and got a bare `unknown model setting` each time; the first of those now answers
with the real name.

**The sibling settings are now exposed too**, in the same shape: `FirstDayOfWeek`,
`DecimalScale`, `DefaultTimeZoneCode`, `UseDatabaseForeignKeyConstraints`,
`UseOQLVersion2` and `SslCertificateAlgorithm`. The two enumeration-typed ones are
canonicalised against `generated/metamodel` and a non-member is refused
(MDL-SET03) rather than written through — an enum value the metamodel cannot
resolve is what makes Studio Pro throw "Sequence contains no matching element".

`UseSystemContextForBackgroundTasks` is deliberately **read-only**. `mx check` on
11.13 rejects a project holding `true` with **CE9436** *"The project setting
'System context tasks' is not supported anymore"* — its only legal value on a
current version is its default, so an ALTER could only ever produce a project that
does not build. mxcli preserves whatever is stored and offers no way to change it.
Found by running `mx check` after the change; nothing else would have caught it.

Still on the "needs Studio Pro" list and **not** addressed here: strict mode
(SEC005), which the report also flags and which weakens XPath constraint
enforcement (CVE-2023-23835).

### 10b. NEW, found while doing the above: writes leaked properties across versions

Not in the report. Exposing the sibling settings meant checking whether the
overlay was safe, and it was not.

**A blank Mendix 9.24 project stores 12 model settings; a blank 11.13 stores 17.**
Both engines' overlays wrote all of their known properties unconditionally, so on
a 9.24 project:

```
alter settings model BcryptCost = 11
```

…also introduced `DecimalScale = 0` and `UseDatabaseForeignKeyConstraints = false`,
neither of which that version stores. Both were wrong as values too (11.13 defaults
them to 8 and `true`), so one unrelated one-line statement silently changed two
other settings. And a property the type may not define is the #759 shape: Studio Pro
resolves every stored property against the type's property list and throws
"Sequence contains no matching element", while mxbuild loads it happily — so the
build is not a safety net.

Fixed by making the overlay **presence-gated** (`settingsoverlay.SetModelSettings`,
now shared by both engines rather than duplicated, as `Configurations` already was),
with the executor refusing an ALTER naming a property the stored document does not
carry so the gate never becomes a silent no-op, and `DESCRIBE SETTINGS` emitting
only stored properties so its output still replays on both versions.

Measured: a 9.24 project keeps exactly its 12 keys through a write; an 11.13
project takes all six new settings with `mx check` at 0 errors; both projects'
describe output replays; re-running is byte-identical with `MXCLI_ALWAYS_WRITE=1`
as the control. The regression test reproduces the exact symptom when the guard is
stubbed out.

Same list, not investigated here: strict mode (SEC005), which the report also
flags as Studio-Pro-only and which weakens XPath constraint enforcement
(CVE-2023-23835). Worth checking whether it is equally reachable.

---

## 11. `DROP USER ROLE` does not follow inbound references

**Reported:** slice 1. `DROP USER ROLE User` succeeded and left the blank app's
`demo_user` referencing it → `CE1613`.

**Status: confirmed.** `execDropUserRole` (`mdl/executor/cmd_security_write.go:288`)
checks the role exists, calls `RemoveUserRole`, prints success. There is no
demo-user check, no navigation-profile check, and — unlike DROP ATTRIBUTE — not
even a warning.

Same family as #1: mxcli drops the thing you named without following what points
at it. The report's framing is right, and it is worth deciding this once as a
policy (cascade, refuse, or warn) rather than per-command.

---

## 12. The `SHOW MESSAGE` clause-order error sends you to the wrong place

**Reported:** slice 4.

**Status: confirmed.** Measured:

```
SHOW MESSAGE 'Ref {1}.' OBJECTS [$x] TYPE Information;
  -> mismatched input 'TYPE' expecting ';'
     'Type' is a reserved keyword in MDL. Use a different name like:
       - Type_  (add underscore suffix) ...
```

The reserved-keyword hint fires on the token text regardless of context, so a
pure clause-ordering problem is reported as an identifier clash that does not
exist. Cheap fix: suppress the keyword hint when the offending token is a
keyword in a valid position for the statement, or add the ordering to the
message.

---

## 13. Module and page documentation are unreachable

**Reported:** slice 1 — `ALTER PAGE … SET DOCUMENTATION` and
`ALTER MODULE … SET DOCUMENTATION` are both parse errors, a `/** */` docblock
before `CREATE PAGE` is not picked up (entities and microflows do take one), so
the two QUAL002 lint findings this raises are not fixable with mxcli.

**Status: confirmed** (`ALTER MODULE B6 SET DOCUMENTATION 'hello'` →
`no viable alternative at input 'SETDOCUMENTATION'`). Note the shape of the
complaint: mxcli's own linter raises a finding mxcli gives you no way to fix.

---

## 14. OQL view entities — REPRODUCED AND REFUTED

**Update, same day.** mxbuild 11.13.0 is now cached, so this was tested properly on
a real Mendix 11.13 project rather than reasoned about. **The finding does not
reproduce. Every operation it reports as impossible works on current main**, and
the resulting project builds clean.

Test: a module with two persistent entities and an association, a view entity over
a join with `sum()`/`GROUP BY`, a module role granted read on it, and a microflow
returning a list of it.

| Reported as | Measured on 11.13 / main |
|---|---|
| `SHOW ENTITIES IN <mod>` does not list it | **Lists it**, `Type: View` |
| `GRANT … ON <view>` → `entity not found` | **Granted**, `read *` |
| `RETURNS LIST OF <view>` → `entity not found for return type` | **Microflow created** |
| `--` comments in the OQL body fail the parse | **Parses**, and `exec` creates the view |
| `CREATE OR MODIFY` does not prune dropped members (CE6770) | **Prunes**; add-then-remove cycle → 0 errors |
| — | `DESCRIBE ENTITY` round-trips the view and its OQL |
| — | **`mx check`: 0 errors** with the whole chain wired up |

No fix landed in the window that would explain it: the only comment-handling fix
(`cd2ac98`, comments inside expressions) is from 2026-08-09, six days *before* the
reporter's nightly, and the two view-entity commits dated 2026-08-17 are OData
pushdown work that does not touch these paths.

The likeliest explanation is in the reporter's own notes. They record that a
"0 errors" build was reported for **a view that had been dropped and never
recreated** — and if the view entity is not in the model, then `entity not found`
from `GRANT` and from `RETURNS LIST OF`, plus its absence from `SHOW ENTITIES`, are
all *correct* answers rather than three separate bugs. One missing document
explains the whole finding. That cannot be proven from here, but it fits every
symptom, and nothing else does.

**This matters beyond the triage:** the conclusion "a view entity cannot feed a
page written with mxcli" is what killed the reporter's dashboard design and pushed
it onto N+1 non-persistent `DashboardCard` objects. It is worth revisiting — as is
`DS_AdminSummary`'s six-queries-in-six-counts, which the report itself calls the
strongest remaining case for a view.

### What is real in this area

- **A view entity requires `UseOQLVersion2 = true`.** Otherwise the build fails
  **CE6779** *"View Entity 'X' is only allowed in the domain model when 'OQL
  version 2' is set to 'Yes' in the runtime settings."* Found by accident: the
  earlier settings work had flipped it off on the test project. Until finding #10's
  change, **that setting was not reachable from MDL at all** — so a project with it
  off could not be made to accept a view entity without Studio Pro. Worth stating
  in the skill, because the error names the runtime setting rather than anything
  the author wrote.
- **`cast()` cannot narrow a length.** The report says `cast()` "is not in the MDL
  grammar"; that is not quite right. `cast(c.Name as String)` parses;
  `cast(c.Name as String(200))` does not. So the underlying limitation is real — a
  pass-through `String(unlimited)` column still cannot be narrowed — but the gap is
  a missing length argument on an existing function, not a missing function.
- **Associations to a view entity** are still unauthorable from MDL (CE6771 for an
  explicit one; Mendix's own route is to select the associated object's `.ID`).
  Untested here, and independent of the above.

## 14b. Original triage note (superseded by the measurement above)

**Reported:** slice 6, as the headline finding that killed the dashboard design.
mxcli can CREATE a view entity and Mendix accepts it (0 errors), after which MDL
cannot reference it at all: `GRANT` → `entity not found`,
`RETURNS LIST OF` → `entity not found for return type`, `SHOW ENTITIES` does not
list it. Plus three quieter traps: `--` comments inside the OQL body fail the
parse (and `exec` then does not run, which reads as success if you grep for a
success line); `CREATE OR MODIFY` does not prune members the OQL no longer
produces (CE6770 until DROP + recreate); pass-through columns inherit the source
length and `cast()` is not in the grammar.

**Status: not yet reproduced.** View entities require Mendix 10.18+ and the only
projects in this checkout were 9.24 at triage time. An 11.13.0 project now exists
for this (`mxcli new --version 11.13.0`, mxbuild cached at
`/root/.mxcli/mxbuild/11.13.0`), so the reproduction is no longer blocked — it
just has not been run.

Reading the code does *not* obviously support the claim — the read path parses
`Source` (`parser_domainmodel.go:105`), `isViewEntity` is used in validation,
and several executor paths handle `DomainModels$OqlViewEntitySource` on entities
obtained from the reader — so a view entity ought to appear in `dm.Entities` and
therefore in `SHOW ENTITIES`. Something version- or engine-specific is going on.

**This is the top item to reproduce next**, on a real 11.13 project. The report
is careful and its other claims all held up, so I would not discount it.

The associated finding — that Mendix view entities *can* have associations (you
select the associated object's `.ID`) and MDL cannot declare one — is worth
recording as a feature gap independent of the above.

---

## What is correct and is not our bug

A large fraction of the report is Mendix platform behaviour, accurately
diagnosed. Recording it here so nobody re-opens it as an mxcli issue, and
because several items belong in mxcli's *skills* even though they are not
defects:

- `mxcli check` passing does not mean `mx check` passes — the report's single
  most emphasised operational lesson. Correct by design (MDL syntax + mxcli's
  own rules vs. the Mendix model), but the skills should say it as plainly as
  the report does.
- `Administration.Account` already defines `FullName`/`Email` (CE0069); a user
  entity may not carry Required/Unique rules (CE7247); every user role needs a
  System module role (CE0156); the stock `User` role is not inert and costs one
  CE2729 per widget on the default home page.
- Database datasources **do** apply entity access; microflow datasources do
  not. The report states this pair better than our docs do.
- Row-level security makes associated objects unreadable, not just unlisted →
  you must denormalise across the security boundary. Four attributes in the app
  exist for this reason, plus the counter-example (a plain String caption on
  readable reference data needs nothing).
- `HEAD()`, `COUNT()`, `SUM()` are list activities, not expression functions
  (CE0117). `id` is an XPath pseudo-attribute, not a member.
- `RETRIEVE … LIMIT 1` returns an object, not a list.
- A microflow is one transaction — the whole atomicity story, for free.
- Mendix commits an input to the model on **blur**; Mendix serves page URLs
  under `/p/`; the trial licence caps concurrent sessions and Playwright pages
  leak them (and `/logout` is a 404 — `mx.logout()` is the working call).
- `Show Message` is blocking; a microflow-datasource grid does not refresh
  after a delete.
- No forward references, in either direction, between microflows and pages.
  mxcli's hint is good; the report's complaint is that
  `--continue-on-error` leaves the model partially updated, which is fair.

The two test-methodology rules the report derives are worth lifting into
`.claude/skills/verify-in-runtime.md` more or less verbatim:

> A check that asserts something is ABSENT needs a sibling check that the same
> thing is PRESENT for someone.

> A literal about *seeded* data is fine; a literal about data that any other
> suite mutates is a time bomb.

Both were earned the hard way: the first found the `/p/` prefix after four
security assertions had been passing for the wrong reason, and the second found
four stale tests during a regression run.

---

## Suggested order of work

1. **#1** — validation-rule / member-access cleanup keyed on the wrong field.
   Data-corrupting, one-line root cause, already has (dead) code.
2. **#3** — `APPLY ENTITY ACCESS`. Security-relevant, currently unexpressible.
3. **#8** — `exec` pre-flight gate. Cheap, and it is the backstop that would
   have softened several other findings.
4. **#4, #5** — the two lint rules. Cheap, and false positives are actively
   harmful: CONV010's 11 false positives buried 2 real findings.
5. **#6/#6b** — three documented spellings of `NON-PERSISTENT`, two of which do
   not parse, one of which ships to every user project.
6. **#2, #9, #11, #12, #13** — individually small.

**Done since this document was written:**

- **#10** — optimistic locking and its five sibling settings, plus the
  cross-version property leak (10b) found while checking that the additions were
  safe.
- **#14** — reproduced on a real Mendix 11.13 project and **refuted**; the
  dashboard design it blocked is worth revisiting.
