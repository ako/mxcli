# Built-in Rules

mxcli ships with built-in rules implemented in Go. These rules are fast and always available.

The **lint rules** below run with `mxcli lint`. There is also a separate group of **check-time rules** that run with `mxcli check` — see [Check-time Rules](#check-time-rules-mxcli-check) at the bottom of this page.

## MDL Rules

| Rule | Description |
|------|-------------|
| **MDL001** | Naming conventions -- Checks entity, attribute, and microflow naming patterns |
| **MDL002** | Empty microflows -- Detects microflows with no activities |
| **MDL003** | Domain model size -- Warns when a module has too many entities |
| **MDL004** | Validation feedback -- Checks for proper validation feedback usage |
| **MDL005** | Image source -- Validates image widget source configuration |
| **MDL006** | Empty containers -- Detects container widgets with no children |
| **MDL007** | Page navigation security -- Checks that pages called from microflows have appropriate access rules |

## Quality Rules

| Rule | Description |
|------|-------------|
| **QUAL006** | Required caption without the default language -- A tab page caption (page, snippet or layout) with no text in the project's `DefaultLanguageCode`. MxBuild refuses it with CE4899 "Empty caption. [German, Germany]". Typically left behind by changing the default language after the pages were written. Error. Measured on Mendix 11.14: of the caption kinds tried (page title, group box, buttons, column header, label, dynamic text, title, enumeration value, menu item, message) only the tab page caption is required; page templates and building blocks are not checked |

## Security Rules

| Rule | Description |
|------|-------------|
| **SEC001** | Entity access rules -- Ensures persistent entities have access rules defined |
| **SEC002** | Password policy -- Checks for secure password configuration |
| **SEC003** | Demo users -- Warns about demo users in production security level |

## Convention Rules

| Rule | Description |
|------|-------------|
| **CONV011** | No commit in loop -- Detects COMMIT statements, and CREATE / CHANGE with a COMMIT clause, inside LOOP blocks (performance anti-pattern) |
| **CONV012** | Exclusive split captions -- Checks that decision branches have meaningful captions |
| **CONV013** | Error handling on external calls -- Ensures external service calls have error handling |
| **CONV014** | No continue error handling -- Warns against using CONTINUE error handling without logging |
| **MDL-FLOW01** | Un-describable branch structure -- Decision branches that re-enter each other's paths, so `DESCRIBE MICROFLOW` cannot render them as nested `IF`s without changing what they mean |
| **MDL-MAP04** | First over an object-rooted import mapping -- An `import from mapping` or `rest call … returns mapping` activity already in the model stores Studio Pro's *First* (`ForceSingleOccurrence` or `Range.SingleObject`) over a mapping that returns one object. `mx check` reports 0 errors; the activity throws `key not found: Path(QName(None,),None,)` when it runs. `mxcli check` refuses the same statement in a script under the same ID; this finds activities written before that check existed, or with `exec --no-check`. A list-rooted mapping's *First* is legitimate and is not reported |

## Running Built-in Rules

Built-in rules run automatically with `mxcli lint`:

```bash
# Run all rules (built-in + Starlark)
mxcli lint -p app.mpr

# List all rules to see which are built-in
mxcli lint -p app.mpr --list-rules
```

## Excluding Modules

System and marketplace modules often trigger false positives. Exclude them:

```bash
mxcli lint -p app.mpr --exclude System --exclude Administration
```

## Check-time Rules (`mxcli check`)

These rules run with `mxcli check` (and the LSP, for real-time diagnostics) rather than `mxcli lint`. They cover pluggable-widget authoring and typed project-settings values.

| Rule | Where it fires | Description |
|------|----------------|-------------|
| **MDL-WIDGET01** | `mxcli check` + LSP | Unknown property key on a pluggable widget. The property is not in the widget's `.def.json`. Catches typos like `optionsSourcType` (missing `e`) before MxBuild does. Suggests the nearest known key. |
| **MDL-WIDGET02** | `mxcli check --post-migration` | Legacy native widget found on a project that has a pluggable replacement available. Reports each occurrence with the qualified document name, widget instance name, and the recommended pluggable widget. |
| **MDL-SET01** | `mxcli check` + LSP | Non-integer value for an Integer-typed project setting (`HttpPortNumber`, `ServerPortNumber`, `BcryptCost`, `DefaultTaskParallelism`, `WorkflowEngineParallelism`). These used to be skipped silently while the statement still reported success. |
| **MDL-I18N01** | `mxcli check -p` | A script that changes `DefaultLanguageCode` leaves a tab page caption — stored in the project, or created by the script before the change — with no text in the new default. MxBuild reports CE4899 "Empty caption". See [Error Messages](../appendixes/error-messages.md#mdl-i18n01-a-tab-page-caption-without-the-default-language-ce4899). |
| **MDL-SET02** | `mxcli check` + LSP | Value other than `true` / `false` for a Boolean-typed project setting (`AllowUserMultipleSessions`). Anything else was silently stored as `false`. |

Run `mxcli check --help` for usage. See [Error Messages → MDL-WIDGET01 / MDL-WIDGET02](../appendixes/error-messages.md#mdl-widget01-unknown-pluggable-widget-property) for cause-and-solution detail.
