# Changelog

All notable changes to mxcli will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

- **`run --local --watch` detects an inconsistent browser bundle instead of leaving it to look like an application bug** (ako/mxcli-maintenance-2) — after roughly ten hot-applies a page died with the runtime's generic error dialog while `runtime.log` filled with `404 - file not found for file: dist%2Fchunks%2F<hash>.js`. The model was clean, `mx check` was clean, and a plain restart fixed it, so several minutes went into debugging the wrong layer.

  mxcli already guarded this class — `ensureClientServed` re-bundles when the client is not served — but it probes **only `/dist/index.js`**, and in this state the entry point is served with a 200 while a chunk it imports is missing. The apply was therefore reported as successful. The check now walks the import graph from `index.js` and re-bundles when it references a chunk that is not on disk, naming the chunks.

  It follows only chunks that exist: an orphan left behind by an earlier bundle references old hashes, and scanning everything under `chunks/` would report a healthy bundle as broken. Since this gates an automatic re-bundle, the safety control is the one that matters — four real built bundles report clean, and deleting a single referenced chunk from a copy of one is detected.

- **`bootstrap-app` removes the stale `.ai-context/` before moving the generated project** (ako/mxcli-maintenance-2, reported twice) — the seed prompt's own `mxcli init --sync-skills` writes `.ai-context/` into the repo root, and `mxcli new` writes its own copy, so the skill's `mv` failed with `mv: cannot overwrite './.ai-context': Directory not empty`. Both copies come from the same binary, so the one in the way is always a stale duplicate. The `rm -f <AppName>/mxcli` note now also says it is required even when mxcli is already on PATH, since `new` hardlinks the binary into the project either way.
- **`mxcli check` warns when a microflow completes a user task it never assigned** (ako/mxcli-maintenance-2) — `set task outcome` on an unclaimed task fails at **runtime**, and quietly: the button appears to do nothing and the only trace is `Client: You can't complete this user task, it is not assigned to you` in the runtime log, while `mxcli check`, `mx check` and the build are all clean. The trap is that `targeting xpath` / `targeting microflow` decides who may *see* a task — it does not assign it, and there is no `assign task` statement. **MDL-WORKFLOW10** names the shape and shows the claim.

  A warning rather than an error, because a task can legitimately be claimed somewhere the rule cannot see — in a microflow this one calls, or earlier in the process. It walks the body in order, since a claim written *after* the outcome does not help, and matches the Assignees association on its member name so the qualified, bare and quoted spellings all count as a claim.

  The skills carried the shape that fails: `write-workflows`' "A common shape" paragraph showed `set task outcome` with no claim step, which the report singles out as the single most expensive paragraph in the skills. It now documents claiming, and the related trap that `WorkflowUserTask.Name` holds the task's *caption* rather than the activity name.

- **`manage-security` and `system-module` state the System-module ceiling** (ako/mxcli-maintenance-2) — no project module can widen access to `System.User`, `System.Workflow` or `System.WorkflowUserTask`, so any UI over them is Administrator-only. It fails silently: a combo box over `System.User` lists the current user only, a grid over `System.Workflow` renders empty, and both `mx check` and `mxcli lint` pass. The report calls this the constraint that shaped the app more than anything else, and it was discovered in a browser after a technician picker had been designed, built, tested and torn out. It is now stated where the role model is decided, with the three ways around it.
- **A multi-line `CALL WORKFLOW` parameter mapping no longer captures the newline** (ako/mxcli-maintenance-2) — writing the mapping across two lines produced the workflow context variable `"Request\n  "` and the build failed **CE0109** *"Undefined variable 'Request\n  '"*. The same call on one line was fine, and `CALL MICROFLOW` was fine at any width.

  That asymmetry is the diagnosis rather than a curiosity: a workflow's context is stored as a variable **name**, while a microflow's arguments stay **expressions** — and the visitor deliberately preserves an expression's trailing whitespace so a multi-line expression round-trips byte-for-byte, with tests pinning it. So the name is trimmed where it is built; trimming in the visitor, the obvious one-line fix, would have broken the round-trip the whitespace exists for. Verified end to end: the reported CE0109 becomes 0 errors on mxbuild 11.13.
- **`mxcli check` catches a microflow data source with no arguments** (ako/mxcli-maintenance-2) — `datasource: microflow M.F` on a microflow that declares parameters wrote a data source with no parameter mappings, and the page built to **CE1571** *"No argument has been selected for parameter 'Task'"* at the far end of an `mx check`. `mxcli check --references` passed, because the microflow itself resolves — nothing compared the signature against the arguments, though both are in the model.

  It is an **error**, and the measurement is what licenses that. Mendix's wording — "and no default is available" — suggests a default sometimes is, so it was worth checking: a page with no parameters *and* a page carrying a `$Task` parameter of the microflow parameter's exact entity type both produce CE1571. Studio Pro does not auto-map an object in scope, so a missing argument is never filled in.

  Signatures of flows the same script creates are recorded too, the way script-defined Java actions already are — one script writing a data source microflow and the page that binds it is the common shape, and without that the check would only ever fire on a flow that already existed. Three controls keep it honest: a parameterless microflow with no parens stays clean (the common case), an unresolvable flow is left to the reference check rather than reported as a missing argument, and the corrected form passes — verified against mxbuild, which reports 0 errors on it. An argument naming no parameter is reported as well, since it binds nothing and is otherwise silent.

  `mxcli syntax page.datasource` said "no parens when it takes no arguments", which reads as though Studio Pro's automatic parameter mapping applies. It does not; the topic now says so.
- **`editable: Never` now reaches the model from `CREATE PAGE`** (ako/mxcli-maintenance-2) — it did not. The property parsed, passed `mxcli check`, passed `mxcli exec` and passed `mx check`, and the field was editable in the browser: the writers hardcoded `Editable("Always")` for TextBox, TextArea, CheckBox, DatePicker and RadioButtons, and the semantic model had no field to carry the author's value. `describe page` emitted no editability either, so a describe/diff round-trip compared equal while the document was wrong.

  The control that isolates it is `ALTER PAGE … SET Editable = Never ON w`, which always wrote it correctly — the value was representable and only the CREATE path lost it. Editability now lives on `BaseWidget`, so one field covers all twelve widget types Mendix gives editability to, and `EDITABLE IF` still wins over a plain `editable:` because the conditional-settings element is what makes the enum `Conditional`. `describe page` emits the value whenever it is not `Always`.

  **Pluggable widgets are reported, not fixed.** No `.def.json` maps `editable`, and a stored `CustomWidgets$CustomWidget` carries no `Editable` key, so the value reaches the document nowhere — which is exactly why `editable: Never` persisted on the textboxes and vanished on the combobox beside them in one statement. `combobox … (Editable: …)` is now **MDL-WIDGET21**, a warning, instead of silence.

- **The documented `mx check` command no longer breaks once two mxbuild versions are cached** (ako/mxcli-maintenance-2) — `~/.mxcli/mxbuild/*/modeler/mx check app.mpr` expands to every cached version and `mx` reads the second path as an argument (`Verb '…' is not recognized`). Both binaries are fine; it is purely the command line, and the failure looks convincingly like a broken install — an earlier revision of that report recorded the wrong binary as broken because of it. The generated project docs now lead with `mxcli docker check -p <app>.mpr`, which resolves the version from the project, and the direct form names a version instead of globbing. Fixed across the generated `AGENTS.md` and ~15 documents, including three skills synced into user projects.

- **`create user role` on an existing role names the re-runnable form** (ako/mxcli-maintenance-2) — a blank Mendix app already ships an `Administrator` user role, so this fires on the first realistic security script, and because `exec` halts on the first error every later statement is skipped. `CREATE OR MODIFY USER ROLE` exists and is idempotent, but requires the parenthesised module-role list, so the bare `create or modify user role Administrator;` fails with `expecting '('` — which reads as an unsupported verb rather than a missing argument. The report filed it as missing MDL surface; the error now points straight at the form that works.

- **A parse error on a reserved word now tells you to quote it, when quoting is the fix** (ako/mxcli-maintenance §5) — `Title: String(100)` is accepted inside `create entity`, but `write (Title)` in a grant list is a parse error, and mxcli answered it with three renames: `Title_`, `_Title`, `MyTitle`. Quoting works — `write ("Title")` executes and the attribute is still called `Title` — and `QUOTED_IDENTIFIER` is in the expectation set the raw message already printed. The maintenance app renamed a stored attribute to `RequestTitle` on that advice, which is a schema change where quoting would have cost nothing.

  The hint now branches on whether the name is reserved by the **platform** rather than only by the parser, because the two have opposite remedies: a parser keyword is escaped by quoting and the model keeps the name, while a platform-reserved name is rejected after the quotes are stripped (CE7247), so advising quoting there yields something that parses and then fails the build. Measured against Mendix's own list: **38 of the 41** keywords mxcli hints on are rescued by quoting and exactly **three** — `Type`, `Default`, `Owner` — are not. The old advice was wrong for the 38 and right for the 3 by accident. The reserved-word lists moved to `mdl/types` so the validator and the parse-error hinting can share one copy; the test that pins the split carries a control that the quoted form actually parses, without which it would pass against advice that does not work.

- **`grant … on System.User` explains itself instead of leaking a missing file** (ako/mxcli-maintenance §4) — it failed with `load domain model 00000000-0000-0000-0000-000000000002: open …/mprcontents/00/00/….mxunit: no such file or directory`, which reads like a corrupt project. It is not: the System module's domain model is **not a stored unit** in any Mendix project — mxcli synthesises it so System entities can be referenced — so there is nothing to add an access rule to, ever. The grant is now refused up front by name, which also avoids the half-applied state a late failure leaves behind. The message says the limitation is universal, and names the consequence that is otherwise rediscovered the hard way: an XPath constraint that *traverses* System is equally unusable, so constraining `Administration.Account` by user role returns nothing.

- **`mxcli test` says why it found no tests** (ako/mxcli-maintenance §5) — a directory holding `workflow.mdl` reported `Found 0 test(s) in 1 file(s)` and then `no tests found in the provided files`. Discovery matches on the file **name** (`*.test.mdl` / `*.test.md`) and nothing said so, and the `1 file(s)` was the directory itself counted as a path — so it read as though a file had been opened and found wanting. The count is now of files actually read, and an empty suite names the `.mdl`/`.md` files skipped for their name, which turns a dead end into a rename. A genuinely empty directory gets the file-format explanation instead, rather than a claim that files were skipped.

- **New check `MDL-OFFLINE01`: a multi-step attribute path in a project with an offline navigation profile** — Mendix rejects an attribute bound across two or more associations on any page an offline profile can reach (**CE6206**), and the error lands far from its cause: the pages are valid until the *profile* is added, and mxcli writes such paths happily. Verified that a datagrid column bound `Req_Asset/Asset_Site/SiteName` stores an `IndirectEntityRef` with two steps and `mx check` reports 0 errors — correctly, because that project has no offline profile.

  The threshold is pinned by a control inside the reference page, which carried both shapes and had only one flagged: `MaintenanceRequest_Asset → AssetName` (one hop) is accepted, `… → Asset_Site → SiteName` (two) is not. The rule covers `Attribute:` bindings and template parameters — the reported case included a dynamic text, which binds through a content parameter rather than an attribute — and ignores XPath constraints, which are full of slashes and are not attribute paths.

  It is a **warning, not an error**, and deliberately so: deciding whether a page is reachable from an offline profile means walking the whole page graph, which Studio Pro does and `mxcli check` cannot (it does not build the catalog). The diagnostic names the offline profile and states the condition rather than asserting a reachability it has not established. Needs `--project`; a project with no offline profile pays one navigation read and sees nothing.
- **`mxcli new` no longer downgrades a Mendix 11.14 project that this machine can build** — the earlier fix lowered every 11.14 project from Java 25 to 21 because mxcli could only run 21. Once the JDK is resolved from the project that is no longer true, and lowering unconditionally downgraded projects *against* the platform: Mendix warns "Java versions below 25 are deprecated for deployment", and a Java 25 project builds without that warning on a machine that has a JDK 25. The step now asks the question that actually matters — can **this** machine build what the project asks for — and only lowers when the answer is no, naming the missing JDK and how to set the version back.

  Measured both ways with the same command: with a JDK 25 visible the project keeps `Java: 25`; without one it reports `Java 25 → 21: no JDK 25 on this machine, and the project would not build.` and stores 21. The message it used to print was also wrong on both counts — it claimed mxcli builds and runs on JDK 21 (it runs on whatever the project asks for) and cited the runtime bundles' class-file version as evidence 25 was unusable, which does not follow.
- **`CREATE OR REPLACE NAVIGATION` can now create a profile, not only replace one — offline profiles included** (ako/mxcli-maintenance §7) — `create or replace navigation Phone` failed with `navigation profile not found: Phone (available: Responsive)`, and nothing else in mxcli added one. Mendix routes a phone to a Phone **profile** on the User-Agent rather than on viewport width, so this put a whole class of app out of reach: phone-specific pages existed but no phone browser could land on them, and a layout naming the missing profile failed CE1613. A profile is now created when the name is one of Mendix's six web kinds — `Responsive`, `Phone`, `Tablet` and their `*Offline` twins — and re-running the statement updates it as before. An invented name such as `Mobile` is still an error, because a profile Mendix does not define can never route; Hybrid and native kinds are excluded for want of a reference document, and the legacy engine refuses rather than approximating.

  The shape is **pinned against Studio Pro 11.14 over PED**, and the measurement that matters was taken by adding a profile through Studio Pro's *own* model API and reading back what it materialised — not by copying a profile the reference project already had. The reference app's Phone and TabletOffline profiles both store `{Precaching: true, InstallPrompt: true}`, and an earlier draft of this change wrote exactly that for every Phone profile it created. They are the user's settings: the schema defaults `Precaching` to **false**, makes `ProgressiveWebAppSettings` optional, and a freshly created profile of any kind stores **null**. Copying the sibling would have turned precaching silently on, with `mx check` reporting 0 errors either way.

- **Creating an offline navigation profile now reports the pages it has just invalidated** (ako/mxcli-maintenance) — an offline profile restricts every page it can reach: an attribute may be bound across at most **one** association hop, and a longer path is **CE6206**. This bites at a distance. The pages were valid, the statement that broke them never mentioned them, and mxcli wrote multi-step paths happily, so `create or replace navigation TabletOffline` printed one cheerful line and the next build failed somewhere with no visible connection to it.

  The statement now scans the project's stored pages and snippets and names the bindings that exceed the limit. The scan keys on the stored `DomainModels$AttributeRef` rather than on a table of widget property names, which is what makes it cover pluggable widgets — a DataGrid2 column's binding sits several levels inside a `CustomWidgets$WidgetValue`, at a key path no property table would have listed. A data source that navigates an association is a different element and is deliberately not reported, since CE6206 does not reject it.

  It is a **warning, not a refusal**: whether a page is actually *reachable* from the new profile takes the whole page graph, which mxcli does not walk here. Measured end to end on an 11.13 project — mxbuild reports 0 errors while the flagged page is unreachable, and exactly the flagged column (`Req_Asset/Asset_Site/SiteName`, 2 of 2) as CE6206 once the profile's home page points at it. The sibling one-hop column in the same grid is accepted throughout, which is the threshold control, confirmed by mxbuild itself rather than by the rule that predicts it.


- **Java 25: mxcli builds and runs a project on the JDK the project asks for** — Mendix Studio Pro 11.14 supports Java 25 and its blank app is created with `JavaVersion = 25`, but mxcli resolved a JDK **21 unconditionally** and refused any other `JAVA_HOME`. An 11.14 project therefore could not be built (`error: release version 25 not supported`) and, if built elsewhere, could not boot (`UnsupportedClassVersionError … class file version 69.0` — compiled for 25, launched on 21). The JDK is now resolved from the project's own Settings > Model > JavaVersion, and the same release is used for the build and for the runtime.

  Measured end to end on a real 11.14 project left exactly as `mx create-project` wrote it: mxbuild builds it on a JDK 25 with **no deprecation warning**, and the runtime boots — `Runtime started; app serving at http://127.0.0.1:8080/`, with the client bundle served. The no-regression control is stronger than the happy path: a Java 21 project booted with `JAVA_HOME` deliberately pointing at the JDK 25 ignores it and launches on the JDK 21, verified per process rather than from the model setting.

  When the required JDK is missing the error now names the version, what was searched, and the second way out a bare "install a JDK" hides — that the requirement is a model setting the project can change. `mxcli run --help` no longer describes version-aware JDK selection as a follow-up.
- **A new Mendix 11.14 project builds and boots as generated** (ako/mxcli-maintenance #1, #3) — it did not. Mendix 11.14's `mx create-project` writes `JavaVersion = 25`, while mxcli builds and launches the runtime on **JDK 21** and validates it, so `mxcli new --version 11.14.0` produced a project whose very first build failed with `error: release version 25 not supported`. Installing a JDK 25 does not fix it: the runtime is still launched on 21, so the failure moves to boot time as `UnsupportedClassVersionError … class file version 69.0`. Compiling for 25 and running on 21 cannot work in either direction, and 11.14's own runtime bundles are still Java 21 class files.

  `mxcli new` now aligns the project's Java version with what mxcli can actually build and run, before the first build — the step that would otherwise be the one to fail. It lowers **only** when the project asks for more than the runtime launcher uses, so it is a no-op on 11.13 and earlier and disappears by itself the day that launcher becomes version-aware, and it leaves a version it cannot parse alone rather than overwriting a setting the user can see. Mendix then warns that "Java versions below 25 are deprecated for deployment" — a deprecation notice on a project that builds and runs, rather than an error on one that does neither.

- **`mxcli check` no longer rejects Mendix's own apostrophe escape** (ako/mxcli-ledger #131) — an expression containing `''`, which is how the Mendix expression language escapes an apostrophe inside a string literal, was reported as `Unexpected token after expression … (possible missing space between keywords)`. Nothing was glued and no keyword was involved: exprcheck's lexer scanned to the next quote with no notion of an escape, so `'a''b'` lexed as two strings and the parser reported the second as a leftover token. Everything else in the toolchain already agreed the MDL was correct — `exec` writes it, `DESCRIBE` round-trips it byte-for-byte, `mx check` reports 0 errors, and CLAUDE.md documents the doubling rule.

  It reproduced **only with `-p`**, because exprcheck runs when a project is supplied. `make check-mdl` runs without one, so this repo's own MDL gate was blind to it — an `mdl-examples/` script alone would not have caught it, and the regression guard is a unit test on the lexer. Reported open across six consecutive builds.

- **`create or replace snippet` no longer destroys the snippet's translations** (ako/mxcli-ledger #143) — replaying one snippet file reset a translated label to its source language in all six languages of a project, silently: the run reported `Created snippet`, `mx check` reported 0 errors, and the app rendered in one language. The replace path was an unconditional **delete-then-create**, and nothing `canon.Reconcile` does can survive that — by the time the create runs there is no stored document left to transplant identities from or to compare against, so the translations went with the deleted unit along with anything else MDL cannot express. A replace is now an update on the stored unit's ID, the way pages have always done it, with only genuine duplicates deleted.

  The control was free and in the same run: a page and a snippet carrying the same shape of content, replayed together. Before the fix the page reported `Unchanged` and kept its Dutch while the snippet reported `Created` and lost it — which is what made this a snippet defect rather than a fact about replacing documents. The verb was the tell: a path that prints `Created` on a replace is not going through the write choke point, so it gets neither the identity transplant nor the unchanged-elision. That also fixes the second consequence the page path documents — a delete+add churns the file in git and crashes Studio Pro's RevStatusCache.
- **`mxcli run --local` could not start any Mendix 11.14 app** (ako/mxcli-ledger #146) — it died at `bundling web client: no rollup.config.mjs … (run a serve Deploy build first)` when a serve Deploy build had just run. mxcli bundles the browser client because mxbuild's serve target wrote the client source and a rollup config but never ran the bundler; **11.14 closes that gap upstream** and no longer emits a config, because there is nothing left to configure. The gate tested for the config, so it failed on the absence of a file whose purpose had been served — fatally, at both call sites, for every 11.14 app. Measured on a blank app, same build target: 11.13.0 `rollup.config.mjs` PRESENT / `dist/index.js` ABSENT, 11.14.0 exactly inverted. The step is now gated on the gap it closes — if the bundle is already there it has nothing to do — while still requiring the config when it is not, because then rollup is the only thing that can produce one.

- **`mxcli new` gives a project a layout it owns** — every generated app used to start exactly where the finding that opened this arc describes: on `Atlas_Core.Atlas_TopBar`, a layout in a Marketplace module that Mendix's own guidance says not to edit. A new project now gets `<YourModule>.App_Default` and its pages moved onto it, so the documented practice is the default rather than something to discover. `mxcli new --layout none` keeps Atlas's. The scaffold is **MDL, not Go** — `describe layout` gives it back and `create or replace layout` re-runs it.

  **It is not a copy of Atlas's, and the plan to make it one did not survive measurement.** Every Atlas layout a real app uses carries widgets MDL cannot spell: `Atlas_TopBar` has a `Forms$MenuBar`, a `Forms$SidebarToggleButton` and a pluggable image, so describe → exec produces a topbar with **no navigation and no logo**. Only the `FullPage` and `Popup` variants — the ones with no chrome at all — copy cleanly. The scaffold reproduces the *result* instead: same layout class, same region classes, navigation in the topbar, `Main` for page content. `Forms$MenuBar` is now authorable (`menubar name (profile: '…')` — the same four keys and the same MenuSource wrapper as a navigation tree) because without it the topbar has no menu; the sidebar toggle and the stock logo are the two things the scaffold does without, and it omits the sidebar rail entirely rather than shipping an always-open one it has no toggle for.

  **A layout's own CSS class turned out to be load-bearing, and `CREATE LAYOUT` could not set one.** Atlas scopes **24** of its layout rules to `.layout-atlas` and its variants, and every Atlas layout with chrome carries one (`layout-atlas layout-atlas-responsive-topbar` and friends; only `PopupLayout` is bare). A layout written without it builds cleanly, passes `mx check` at 0 errors — and renders with no topbar bar and no sidebar rail. `create layout` now takes `class:` and `style:`, and `describe layout` round-trips the class. This was invisible to every check mxcli has and was found by putting the generated app in a browser next to the one it replaces.

- **Layouts are editable and pages can be moved onto them: `ALTER LAYOUT`, `ALTER PAGES … SET LAYOUT`** — authoring a layout was half the job; a layout no page uses changes nothing, and a rewrite is the wrong tool for changing one you did not write.

  `ALTER PAGES [IN <module>] SET LAYOUT = X [WHERE LAYOUT = Y]` is the migration form — an app has one layout and many pages, so moving off Atlas_Default is one statement rather than forty. Pages in Marketplace modules are **skipped and named** rather than refused (a project-wide repoint that stopped dead on Administration's pages would be unusable), and a `WHERE LAYOUT` naming a layout that does not exist is an **error**, not a "0 pages" success for a typo.

  Both forms now **refuse a repoint that would leave a page bound to a placeholder the target layout does not declare**, and name `map (Old as New)` as the remedy. Without the check the rewrite produced `NewLayout.HeaderLeft` for a layout with no HeaderLeft; mxbuild does catch that (CE1613, measured on 11.13) but at the far end of a build, naming the page rather than the statement — and it matters more in bulk, where one page out of forty using an extra placeholder is exactly what nobody checks by eye. The suggested remedy is the syntax that actually parses: the grammar is `MAP (Old AS New)`, and the comment in the grammar file had said `MAP (Main -> Main)` for as long as the rule existed.

  `ALTER LAYOUT Module.Name { … }` takes the whole `ALTER PAGE` vocabulary — SET, INSERT, DROP, REPLACE — because a layout's widget tree *is* a page's with four extra element types. A scroll-container region has no name of its own, so it is addressed as `layoutContainer.top`, reusing the dotted widget reference the grammar already had rather than inventing a syntax for five fixed positions; only `INSERT INTO` takes one, since BEFORE/AFTER position a widget among siblings. Two finder gaps had to close first: the page finder looks for a `FormCall` a layout does not have, and nothing descended into a scroll container's five named slots — so every widget in every layout was unreachable, and so was anything inside a scroll container on a page.

  This is also the answer to a fidelity limit worth stating plainly: **describe → rename → exec is only as complete as what MDL can spell.** Measured on `Atlas_Core.Atlas_SideBar`, the copy loses both `Forms$SidebarToggleButton` widgets, and an `image` widget loses its image reference (CE0463 until `mxcli fix widgets`, then "No image selected"). Those widgets were already emitted as comments; the comment now says `-- NOT re-executable: mxcli cannot author this widget, so re-running this script would drop it`, because a bare `-- Forms$X (name)` line reads as informational. `ALTER LAYOUT` edits the stored document and leaves untouched what it was not asked about.

  The marketplace guard also got stricter in a way that matters: it keyed on `AppStoreGuid` alone, so a module carrying only `FromAppStore` read as project-owned. Both signals now count — a marketplace guard that quietly stops firing is worse than none, because the refusal is the whole point.

- **Layouts are authorable: `CREATE LAYOUT`** — the last document a page depends on that MDL could not write, which is why the topbar was out of reach. A layout is now `create [or replace] layout Module.Name (layouttype: 'Responsive') { scrollcontainer … }`, with four element types pages did not need: `scrollcontainer`, `region top|right|bottom|left|center`, `placeholder`, and `navigationtree`. Verified in a browser on a real 11.13 app — all three regions render in place and the page's own widget lands in the `Main` placeholder — not just against a clean `mx check`, which a layout can pass while rendering wrong.

  `DESCRIBE LAYOUT` now emits **re-executable MDL** rather than a commented tree, which is what makes the documented workflow work: describe an Atlas layout, change the qualified name, run it. That is the copy operation, so there is no `COPY DOCUMENT` verb and none is needed. Writing into `Atlas_Core` is **refused** — Mendix's own guidance is not to edit the supplied layouts, because a Marketplace update replaces the module and the edit is gone. Making that guard real also meant fixing a quieter bug: `FromAppStore` was only populated by `ListModules`, so a lookup by name read `false` for every module and any guard on it would have been inert.

  The document is pinned to the ten keys Studio Pro writes, measured identical across all 22 layouts Atlas ships on 11.13.0. `modelsdk/gen` offers a whole family of placeholder properties on `Layout` — `MainPlaceholderName` and six more — that `generated/metamodel` does not declare and no real layout carries; writing one gives a layout **mxbuild accepts at 0 errors and Studio Pro cannot open**, since it resolves every stored property against the type's list. Which placeholder is "main" is a naming convention instead (22 of 22 name one `Main`, and a page binds by qualified name anyway), so `layouttype` is the only header property and anything else is an error rather than an ignored key. The platform is inferred from the layout type — the web and native vocabularies are disjoint — so there is no `native:` flag to contradict it. Authoring is modelsdk-only; the legacy writer, whose layout serializer emitted a string `$ID`, no `Content` wrapper and a `LayoutType` on the wrong node, now refuses instead.
- **Captions written from MDL now land in the project's language, not `en_US`** (mendixlabs/mxcli#970) — on a project whose language is not en_US, every caption, title, label and content string mxcli wrote was stored under a hardcoded `en_US`. Studio Pro then rendered the widget as the empty-caption placeholder (`<>`) with a "no translation for this language" warning, while mxcli, `mx check` and mxbuild all reported success — measured on an nl_NL-only 11.13 project, where a created page's three texts all landed under en_US and `mx check` said `The app contains: 0 errors.`

  Mendix has no language-neutral text: every `Texts$Translation` carries a `LanguageCode`, so a writer must choose one, and mxcli chose the literal `en_US` at ~23 creation sites. The read side had already been fixed for the same reason (#702), so `describe` was reading the project's `DefaultLanguageCode` while `create` wrote en_US — and on a non-en_US project describe → exec therefore did not round-trip. Both halves now resolve the same language through one helper.

  The control came free from the reported configuration: Administration's own Studio Pro-authored pages in that same project store the same Dutch word under `nl_NL`, so the model was never the problem — only the writer's choice of language. Both engines were affected identically, which is what placed the defect above the storage layer.

  The fix has two halves, because the text is built at two different depths. The executor sites build a `model.Text` and can reach the project; the **writer leaves** build a `Texts$Text` from a bare Go string and have no context at all — a widget `Label`, a pluggable datagrid column caption, a navigation menu item and a menu-document item all land there, each measured storing en_US on an nl_NL project. Threading a language down to them would have changed **183 function signatures across five packages** (113 in `sdk/mpr` alone), nearly every widget writer in the codebase, to carry a value that is constant for the whole run — so they read one process-level `model.AuthoringLanguage()` instead, published by the same resolution point the read side already used. `ALTER PAGE … SET Caption` was never affected: it edits the stored translation in place and keeps its `LanguageCode`.

  An en_US project is unaffected — verified as a regression control, and an unset process still reports `en_US`, so anything that does not resolve a project behaves exactly as before. `DefaultLanguageCode` is by definition an enabled language, so this cannot produce the unenabled-language case #257 warns about.

- **`describe` no longer empties a loop body that carries an annotation** (#965) — a loop whose body held an `@annotation` was described as `loop $X in $L begin end loop;`: nothing inside. Silently, at exit 0, and the truncated script passed `check`, so the documented describe → exec round trip **deleted the loop body**. The model was never damaged — `bson dump` and `--format mermaid` both still showed the activity, mermaid because it iterates the object collection directly instead of traversing from a start node.

  `emitLoopBody` picks the body's starting node as "the object with no incoming sequence flow, leftmost then topmost". A `Microflows$Annotation` is an object in the collection but connects through `AnnotationFlows` and never through `Flows`, so its incoming count is permanently 0 and it was always a start candidate. When it won, traversal began at a node with no outgoing sequence flow and emitted nothing. Only loops were affected: the top level starts from its `StartEvent`.

  The defect is **positional**, which the report's "any statement with an `@annotation`" over-generalises: an annotation left of, above, or above-left of the first activity loses the body, while one to its right renders correctly *including* the annotation. A fixture that happens to place it on the right passes against the broken code, so the guard covers all four.

  It was also **not** a Studio Pro-only defect, which the first reading assumed from Studio Pro's above-left placement. mxcli's own builder emits a loop-body annotation at the same X as the activity and above it — measured activity `(150, 80)`, annotation `(150, -20)` — precisely the losing shape. mxcli was destroying loop bodies it had authored itself.

  #330 covers the same construct and did not catch it: its Go regression tests the AST → BSON *build* path, and its describe assertion is a manual procedure in a comment, so CI only ever syntax-checked the fixture. Nothing had ever *described* an annotated loop body. The durable guard is now a build → describe round trip over the builder's real positions, which asserts its own premise so it fails loudly if the layout ever stops exercising the defect.
- **Fixed: both parameterised theme-switcher actions threw on the first click** — `SetAppSkin` and `SetAppTheme` were dead on arrival. mxbuild lowers the first letter of a JavaScript action's parameter when it generates the wrapper (`Skin: String` reaches the body as `skin`), and the generated bodies used the modelled spelling, so the first click was `ReferenceError: Skin is not defined`. Nothing caught it earlier: `mx check` reports 0 errors because the action is well-formed and the body is opaque user code, `mxcli check` has nothing to say about JavaScript, and the generated file is rewritten on every build so it could not be patched in place. This killed exactly the half the multi-theme feature exists for — selecting a theme by name — while the no-parameter actions the help walks you through kept working, so the documented path worked and the feature did not. Verified against a real 11.13 build and by importing the generated module in a browser and calling it. The guard walks every parameter of every generated action. Reported by ako/mxcli-ledger (#135).

- **Fixed: a scaffolded theme kept its base theme's font licence filename** — two themes created from `signal` both shipped `OFL-signal.txt`, and `theme show` described their fonts under it. Nothing collided, because the content is identical — but it bites on the edit the scaffold exists to invite: a brand theme that changes its fonts to match its colours then ships IBM Plex's licence for fonts that are not IBM Plex, and the filename is what would have caught that. The licence is now renamed with the theme, the same treatment the mixin and the `@import` already got. Reported by ako/mxcli-ledger (#140).

- **Fixed: `mxcli theme create` reported paths that read as the app's own** — it printed `created theme/web/custom-variables.scss` and `created theme/web/main.scss`, which on a project with hand-written versions of both reads as having just overwritten them, from the one command whose purpose is to write into `theme/`. Nothing was touched; the paths were relative to the scaffold. Every path is now printed under the scaffold root, and `theme.json` is no longer mislabelled as living under `files/`. Reported by ako/mxcli-ledger (#139).

- **Several themes in one app, switched at runtime** — `mxcli theme apply signal ledger console` installs a switchable set. All of them compile into one stylesheet and the app picks one with a class on `<html>`: no rebuild, no reload, no server round trip. The first named renders by default. This is the CSS Zen Garden result for Mendix — the DOM Mendix renders never changes and neither does the model, yet brand, ground, ink, radius, type and card treatment all move on a class swap. Measured on a real 11.13 app, same page: signal `#0f6e6b`/4px/IBM Plex with shadowed cards, ledger `#1f3a5f`/2px/Source Sans with hairlines, console `#2dd4bf`/6px/Space Grotesk flat.

  It is cheap because a theme turns out to be almost entirely token values. The Atlas wiring, the recipe layer and the widget layer are **byte-identical in every theme** (measured: one hash across all three, 174 lines of recipes) and resolve every colour through `var(--mxt-*)`, so one copy serves the whole set — only the palette, the fonts and 3–8 lines of skin differ per theme. The recipe layer moved out of the three theme partials into a shared `_mxcli-recipes.scss`, which also closes a latent drift the old layout could not detect.

  The default theme's scope is `:root` **minus** every other skin's class rather than a bare `:root`: bare keeps matching once another class is set, so its rules would leak under every other theme and the winner would come down to specificity. Negation makes the scopes mutually exclusive — exactly one palette is ever live, and the outcome never depends on import order. A single installed theme is emitted exactly as before (bare `:root`, skin unscoped), so nothing changes for a project with one.

  With a set installed, `mxcli theme switcher install` also generates `SetAppSkin`, `CycleAppSkin`, `ApplyStoredSkin` and an `ACT_CycleSkin` nanoflow, built against the set actually installed so a cycle button can never offer a theme whose CSS is not in the page. Theme choice is stored separately from light/dark, so picking a theme keeps the variant. `theme apply` and `theme remove` now take several names; a bare `apply` refreshes the whole installed set in its installed order rather than promoting a different default.

- **Each theme vendors its own font licence** — all three wrote `theme/web/mxcli-fonts/OFL.txt`, with different SIL OFL texts (IBM Plex, Adobe Source, JetBrains Mono). Survivable for one theme, wrong for a set: installing three left a single licence covering three families, and every apply rewrote it, so the set never settled. Now `OFL-<theme>.txt`, with a test that fails if any two themes write different content to one verbatim path.

- **A project can carry its own themes: `mxcli theme create`** — the theme registry was embed-only, so a brand palette had to be hand-edited into a generated block, which the digest fence then refused to touch on the next `apply` — the project had silently taken the theme out of mxcli's hands to change one colour. `mxcli theme create <name> -p app.mpr` now scaffolds a theme the project owns into `theme/mxcli-themes/<name>/`, and from then on it is a theme like any other: `theme list -p` shows it marked `local`, `theme apply`/`remove`/`show` accept it, and a local theme named after a built-in shadows it.

  That folder is fixed by two constraints. It must be **committed** — a design-derived theme is source the team shares, which rules out `.mxcli/`, gitignored by `mxcli init`. And it must **not be compiled**: mxbuild's entry point is `theme/web/main.scss` and it does not glob `theme/`, verified against a real 11.13 build that nothing under it reaches `deployment/`.

  Scaffolding copies an existing theme, so the shared Atlas map and widget layer come across byte for byte and only the palette is yours to edit. The copy renames the identifiers built from the theme name (`@mixin mxcli-<name>-<alt>`, the `@import`) — two themes sharing a mixin name collide the moment both exist, and the symptom is a rule that compiles to nothing.

  `--from` takes either a theme name or a file. Given a file it reads `--mxt-*` declarations out of any CSS-shaped text — a stylesheet, an SCSS partial, or the `<style>` blocks of an HTML export — filing declarations inside a `prefers-color-scheme: dark` / `.theme-dark` / `[data-theme="dark"]` block into the dark palette and everything else into the light one. That contract is the whole agreement: inferring a palette from a mockup's inline styles is a guess, because two greys do not say which is the app ground and which is a hovered row. Tokens the design does not name keep the base theme's value, so a three-colour design still yields a complete palette. A `--mxt-*` name the base theme does not declare is **refused rather than written** — nothing reads it, so the theme would apply cleanly and render unchanged, indistinguishable from the design never having been applied. `mxcli theme show <name>` now prints the vocabulary a design has to target.

  Verified end to end rather than in Go alone: a design-seeded theme through `mxbuild --target=deploy`, then measured in a browser — `--brand-primary` resolves to the seeded colour under both `prefers-color-scheme` settings, with the seeded radius and the recipe classes intact.

- **A decision inside `while` keeps its false flow** (#893) — `while $C begin if X then break; end if; end while;` was written with only the decision's **true** edge, which Studio Pro rejects with CE0079 *"the 'false' condition value should be configured on an outgoing sequence flow"*. This is a writer defect rather than a check gap: `mxcli check` reported "Syntax OK" and exit 0, so the broken microflow reached the project and surfaced only when someone opened the Errors pane.

  The cause was a fix applied to one of two loop builders. A merge-less split defers its FALSE case to the next flow; `addLoopStatement` tracks that deferred case through the body and wires whatever is left over to a synthesized Continue event ("didn't break → next iteration"), and has since ledger #52. `addWhileStatement` mirrored neither half, so the two disagreed on the same body — measured `LOOP true=1 false=1` against `WHILE true=1 false=0`. Four shapes were affected, all of them a merge-less split: `break` last, `continue` last, either with a trailing statement, and either after a leading statement. (`return` inside a loop is refused by MDL062, so it is not a live shape.)

  The lasting guard is a **parity test** asserting the two builders agree on the same body — the assertion that would have caught the original fix landing in only one of them.

  Found while investigating #893, but **not** what it reported: case 3 claimed `IF` as the first statement of a `WHILE` body was written broken, and that shape builds correctly. It was found by probing the neighbourhood of a claim that did not reproduce. Cases 1, 2 and 6 of the same issue were already fixed by MDL061/062/063.

- **Enable, change and disable a project's languages from MDL** — `alter settings LANGUAGE add 'de_DE'`, `modify 'de_DE' (CheckCompleteness: true)`, `remove 'de_DE'`. The enabled list is what App Settings ▸ Languages shows and the only thing a build emits translations for, and nothing could change it: `alter settings LANGUAGE` carried `DefaultLanguageCode` only, so the answer to "translate this app" was always "open Studio Pro". The element is pinned against a Studio Pro-authored reference (11.13.0) — it is a `Texts$Language`, not `Settings$Language`; it carries **no** Description (Studio Pro derives "Arabic, Sudan" from `ar_SD` for display); Studio Pro appends rather than sorting; and `CheckCompleteness` is a real setting — it reports errors for texts with no translation in that language — stored `false` on a newly added language and on the default, which Mendix always checks regardless. Measured: the element mxcli writes is field-for-field identical to Studio Pro's, an untouched language keeps its stored `$ID`, and a re-run leaves the unit byte-identical.

  Removing a language is refused only for the **default** (everything falls back on it). A language that still carries translations is **reported, not refused**: removing it does not delete them — the enabled list and the translation data are independent, which a stock app proves by enabling one language while storing translations in eight.

  `DESCRIBE SETTINGS` now emits the whole settings block as statements that **replay**, which it did not before: the language list was a comment, and configurations were emitted as `alter settings configuration` — so a described project stopped at "configuration not found" on any target that lacked one. Both are upserts now (`add or modify`, `create or modify configuration`, the latter a prefix the grammar already accepted and the executor silently ignored), `DefaultLanguageCode` is emitted after the languages it is validated against, and every option is named because MODIFY touches only what it names. Measured both directions: replay onto the same project is byte-identical, replay onto a blank one reproduces the settings document field for field.

- **`--format json` and `--format sarif` put only the payload on stdout** (#904) — `mxcli lint --format json | jq .` failed with `Invalid numeric literal at line 1, column 10`, because stdout began `Connected to: …`, `Loading cached catalog…`, `✓ Catalog ready` and only then the JSON. The command's own help advertises `--format sarif > results.sarif`, which wrote a corrupt file for the same reason. The executor was constructed with `executor.New(os.Stdout)` regardless of format, so the commentary emitted during connect and catalog build shared the stream the payload was written to — nothing to do with the formatters, which were never reached until after the damage.

  The executor's writer is now chosen from the resolved format: stderr when stdout carries a machine-readable payload, stdout otherwise. It goes to **stderr rather than being discarded** — the progress is wanted on a terminal and in CI logs, it was simply on the wrong stream. Text mode is unchanged, which is asserted rather than assumed. `mxcli report` had the same defect and is fixed with it, with the extra condition that `-o <file>` frees stdout and sends progress back there.

- **Lint finds its rules above the project, and says so when it cannot** (#904) — a project whose author believed 29 convention and security rules were guarding it was running 4. Rule discovery was `filepath.Dir(<the .mpr>) + /.claude/lint-rules`, so the bundled Starlark rules were found only when `.claude/` sat beside the `.mpr`. With one `.claude/` at the repository root — the ordinary Claude Code layout, and what `mxcli init` produced for a solution repo — **zero** Starlark rules loaded, silently: measured at 19 rules instead of 48, same files, one directory apart. Discovery now walks up from the project and stops at the repository root; going further would let a stray `.claude/` in a home or temp directory supply rules to everything beneath it, which is worse than finding none because the run would look green under someone else's rules. `mxcli report` had the identical lookup and is fixed with it — that one emits a *score*, so a silently reduced rule set produced a falsely high one.

  The reported symptom was "only 4 of 29 rules load, silently". The diagnosis was off in a way worth recording: **CONV011-CONV014 are Go built-ins, not bundled rules**, and appear unconditionally, so "4 of 29" was really 0 of 29 — there was no stale-file problem to fix, and rule files that genuinely fail *were* already being reported.

  **`--rules/-r` now rejects an unknown id** instead of reporting a clean project. The flag works by disabling every rule it does not name, so an id matching nothing disabled all of them and printed "No issues found." — indistinguishable from success. This was broader than reported: an id that has never existed in any release behaved identically. The error names where rules were loaded from, or says that no directory was found. Matching is also case-insensitive now, because `-r conv009` was the same trap in miniature.

  **Rule-load failures moved to stderr and are returned to the caller.** They were printed with `fmt.Printf` — the same stream as `--format json`/`sarif` payloads — and discarded, so nothing could count them or exit on them. The directory-level error, previously dropped by `if err == nil`, is surfaced too: an unreadable rule directory used to be indistinguishable from a project with no custom rules. (The `--format json` residue noted here originally is fixed too — see the entry below.)

- **`CATALOG.SOURCE` covers nanoflows, rules, JSON structures and mappings** (#912) — the source index carried only five document types (entities, enumerations, microflows, snippets, pages), so `search '<text>'` could not reach anything a nanoflow, rule or mapping alone contained. Searching for a nanoflow's own name returned "No matches found" — while suggesting `refresh catalog source`, which the user had just run.

  Three of the missing types were **already being collected**. `buildSource` enumerated nanoflows, rules and workflows, but the executor's describe dispatch had no `rule` case and routed `"nanoflow"` to `describeMicroflow`, which searches `ListMicroflows()` only and so returned NotFound for every nanoflow. The worker loop discarded both errors — `if err == nil && text != "" { … }` — so the types contributed zero rows and said nothing about it. Measured on a 9-module project: 114 documents collected, 100 inserted, the 14 dropped being exactly the 13 nanoflows plus one rule. Both describes worked correctly when called directly, which is what made this a wiring defect rather than a missing feature.

  Fixed in three parts: dispatch nanoflows and rules to their own describe functions; return describe failures instead of dropping them, reported grouped by type (a whole type failing is a wiring bug, one document failing is a property of that document); and add the three types the report named — JSON structures, import mappings and export mappings — each of which already had a `Reader` method and a catalog table, with only the source text missing.

  The type names are now constants in one exported list and the dispatch is a table asserted against it, so a collector added without a dispatch case fails a test rather than silently indexing nothing. The guard pins each mapping by **function identity**: a presence-only check passes the nanoflow bug, because the wrong function is still a function. Measured after: 100 rows → 118 across all ten types, no describe failures reported. Still uncovered, and a larger piece of work: `DESCRIBE` supports 46 object types in total, so layouts, constants, Java actions, menus, scheduled events and the rest remain outside the index.

- **`check` reports the creation-order mistakes `exec` refuses** (#955) — a script naming a document a later statement creates passed `mxcli check --references` with "Check passed!" and then failed partway through `mxcli exec`, *after* earlier statements had been written, because exec is not transactional. The reporter's case was a microflow parameter typed to an entity created three statements further down; the module was created, then the run stopped.

  The reference validator collects every name in the script up front, so a forward reference is indistinguishable from a backward one — it is the same pass that deliberately skips references to things the script creates, just skipping more than it should. `exec` already computed the diagnosis (`… is defined later in this script — move its create statement before this one`), but only once a statement had failed. The new **MDL-ORDER01** says it first. It lives in the no-project tier, which is what makes `exec`'s own pre-flight refuse the script too: the reporter's repro now ends "Refusing to execute: 1 error(s) above. Nothing was written." instead of leaving a half-applied model.

  **It is not every forward reference, and the difference was measured rather than assumed.** Ten shapes fail at exec and are reported: a flow's parameter and return types, an entity attribute's enumeration, an association's endpoints, `CALL MICROFLOW`/`CALL NANOFLOW`, and a `GRANT` on an entity, microflow, nanoflow or page. Four execute correctly with the definition afterwards and are deliberately left alone — an entity's `EXTENDS`, `CALL JAVA ACTION`, `RETRIEVE … FROM`, and `CREATE <entity>` inside a body — because flagging them would reject scripts that work today. Both halves are pinned by bug-tests. Known residue: a later `CREATE OR MODIFY` fails exec the same way but asserts nothing about the document being absent, so it cannot be judged without `-p` and is not reported.

  Probed against every script in `mdl-examples/` (367): exactly one new finding, and it was real — a #675 fixture whose `GRANT` preceded the entity it names, now reordered.

- **`mxcli syntax <topic>` finds topics that are not the first word** (#955) — `mxcli syntax rule` answered "Unknown topic: rule" while both `microflow.rule` and `validation-rule` were registered. Lookup was anchored at the left and matched whole segments, so a topic was reachable only by its leading segment or via a hand-written alias; of the registry's 73 distinct leaf names, **60** had no spelling that found them, `call`, `column`, `decision`, `mapping`, `retrieve`, `targeting` and `user-task` among them. A query that matches no path from the left now matches any path *segment*, and all 60 resolve. This is why an alias each would have been the worse fix: `rule` names two unrelated topics and the fallback returns both, where an alias has to pick one.

- **`GRANT … --mcp` was unreachable, and so were the security `SHOW`s** (#900) — every `GRANT … ON <entity>` against a live Studio Pro aborted with `GetModuleSecurity: not supported by the MCP backend` before issuing a single PED call, even though #704 had shipped the write path and `mxcli mcp capabilities` reported entity access rules as available. The executor validates the referenced module role before it calls `AddEntityAccessRule`, and that read had no MCP implementation.

  Security **reads** now come from the local `.mpr`, like every other read in this hybrid backend. PED exposes no security document, but nothing about that makes the stored one unreadable — and the same omission had left `SHOW MODULE ROLES`, `SHOW USER ROLES` and `SHOW PROJECT SECURITY` failing over `--mcp` too, none of which writes anything at all. Serving them from disk is a favourable bargain: PED cannot author roles, so the only writer is Studio Pro itself, and a file stale by one unsaved role yields a clear "module role not found" rather than a wrong write — the roles are by-name references Studio Pro resolves on apply.

  The gap was structural rather than an oversight: the MCP backend delegates a read to the local reader only when one of *its own* write methods needs it, so a pre-check the executor makes before ever entering the backend is invisible from inside the package. `AddEntityAccessRule` was unit-tested in isolation and passed. The regression test drives the MDL statement instead, which is the only level at which this is visible. Verified live against Studio Pro 11.13 (PED 1.0.0): the pre-fix binary aborts on the read with no tool call, the fixed one writes the rule, and reading it back over `ped_read_document` shows the role, the `ReadOnly` default and a member access pointing at the attribute, with `ped_check_errors` clean.

- **`@start(x, y)` positions a microflow's start event** (#951) — written on the first statement, the one the start flows into, because the start has no statement of its own; the same placement `@merge` uses for the other implicit node. It is optional: omitted, the start is derived one spacing unit left of the first activity on that activity's centre line, as before. `DESCRIBE` emits it only for a start that is *not* at that derived spot, so an ordinary description does not grow a line restating its own arithmetic while one carrying a hand-placed start still round-trips exactly.

- **A rewritten microflow no longer strands its start event** (#951) — `CREATE OR MODIFY MICROFLOW` moved every activity to where the script asked and left the start where the *previous* layout had put it, joined to its own first activity by a long, mostly-empty diagonal across the canvas. Measured on a real project: activities at `360;340`, start at `40;200`.

  The cause was the fix for the opposite report. #884 was a describe→exec round-trip *moving* a hand-placed start (`145;200` came back as `100;200`), fixed by carrying the stored position over on every rewrite — which then pinned the start of every rewritten flow. Both reports are real, and neither is answerable without asking where the stored value came from. A start sitting at the derived spot is mxcli's own arithmetic handed back, carries no intent, and is now re-derived so it follows the activities; a start anywhere else was placed by a person and still survives. `@start(x, y)` states the position outright and beats both, which is the other half of what #951 reported — before it there was no way to move a start once one had been preserved.

  Neither `mx check` nor a successful build detects this, in either direction: the Mendix model carries no geometry rules, so a stranded start is a valid document that builds and runs and is merely drawn wrong — 0 errors on mxbuild 11.13.0 before and after.
- **`mxcli run --ensure-db` can start PostgreSQL without a working service manager (#823)** — on hosts that ship neither `service` nor Debian's `pg_ctlcluster` (e.g. Arch Linux), `--ensure-db` failed with `exec: "pg_ctlcluster": executable file not found in $PATH` even though the portable `initdb`/`pg_ctl`/`psql` tools were present. `startLocalPostgres` now falls back to a user-owned cluster under `~/.mxcli/postgres` when no service becomes ready, but never starts a competitor while another process owns the requested port. The cluster is idempotent and needs neither a `postgres` OS account nor passwordless `sudo`: its listen address, port, private socket directory, and `0700` socket permissions persist in PostgreSQL's own configuration; loopback TCP uses SCRAM while role/database provisioning uses local trust only through that private socket. Anyone who ran an earlier development revision of this fix must stop PostgreSQL, remove `~/.mxcli/postgres`, and rerun `--ensure-db`; reuse detects and refuses its insecure host-trust records. The retained system-cluster path is non-interactive, keeps `sudo -u postgres` on the peer-authenticated system Unix socket at the requested port, and creates a SCRAM password even on PostgreSQL versions whose default is MD5. Password-bearing SQL is sent over standard input instead of exposed in process arguments. The canonical endpoint—including bracketed IPv6—is also the one handed to the Mendix runtime.

- **A pluggable widget's click/change action survives a describe→exec round-trip** (#956) — the action landed in the `.mxunit` and built at 0 errors, then `describe page` omitted it, so re-executing mxcli's own output deleted the wiring. A widget's storage key is not MDL's name for the slot: Mendix's own widgets suffix theirs (BadgeButton `onClickEvent`, HeatMap `onClickAction`, Combobox `onChangeEvent`) and the writer has stripped that suffix since ledger #14, while the reader looked up the literal string `"onClick"`. It now resolves the stored key through the writer's own `actionSourceForKey`, so the two cannot drift apart again, and reads the change slot as well as the click slot — the generic pluggable path had no `OnChange` read at all, which silently dropped a Slider/RangeSlider/StarRating's only action.

- **`OnChange:` is accepted on widgets whose storage key is also `onChange`** (#956) — Slider, RangeSlider and StarRating were refused by **MDL-WIDGET01** with *"property `OnChange` is the widget's internal storage name … use `OnChange:` instead"*, so their only action slot could not be authored at all. `OnChange` was missing from `isBuiltinPropName` while `Action` was present, so MDL's own spelling never took the early exit the other dedicated keywords take and collided with the storage-name check.

  Neither of these is caught by a build: a page that lost its action is a valid document, and mxbuild 11.13.0 reports 0 errors before and after. Verified by describing the page back and by reading the stored unit.

- **Named pluggable-widget action slots** (#956) — every action-typed property of a pluggable widget is now authorable by the widget's own key, not just the two MDL names it: `createFileAction: microflow MyModule.ACT_CreateFile`, `onSelectionChange: show_page MyModule.Detail`. Previously only `onClick:` and `OnChange:` had a surface (matched after stripping one `Event`/`Action` suffix) and everything else got no mapping at all, so `mxcli check` reported MDL-WIDGET06 and the value was dropped. File Uploader 2.5.0 extracts to six action slots, none of which were authorable — the widget could be placed and not wired, which is what the issue reported.

  A named slot is a mapping with no `source`, addressed by its `propertyKey` — the shape object-list item mappings already used. `microflow`/`nanoflow` written on one parse as a *data source* (those forms overlap with `dataSourceExprV3`, and the datasource alternative has to keep winning or a chart series' `staticDataSource: microflow M.X` would become an action); the executor converts them using the widget definition, the same split `fragmentArgValue` already resolves. `DESCRIBE` emits named slots, so the round-trip closes.

- **A pluggable-widget property hidden by another property's value is refused before the build** (#956) — a slot can be conditional: DataGrid 2's `onSelectionChange` is hidden when `itemSelection` is `None`, and writing an action into a pruned slot fails the build with **CE0463**. **MDL-WIDGET10** could not fire on it, because a `selection` property carries no `defaultValue` in the `.mpk` and the rule refuses to guess an indeterminable condition. mxcli's own builder writes `None` for an omitted `Selection:` — read off the stored widget rather than assumed — so the condition is now determinable. The severity is also corrected for action slots: an action has no default value to differ from, so every hidden action was reported as a harmless "the value will be ignored" warning when the measured outcome is a failed build.

- **Rewriting a document no longer deletes its translations** (PROPOSAL_translations.md slice 1) — a Mendix text holds one `Texts$Translation` child per language and MDL carries one string, so a rebuilt document kept only the language the statement named. Measured on 11.13.0: describing `Administration.MyAccount` and re-executing that description took the project's Dutch translations from 335 to 328 and removed "Mijn account" entirely. `mx check` reports 0 errors before and after, because a model missing a translation is a valid model.

  The stored translations are now carried onto the rebuild by `canon.CarryTranslations`, in the same chain as the other things a rebuild would lose — identity, element `$ID`s, `PersistentId` — so both engines get it from one choke point. Texts are paired by containment path when the two documents hold texts at exactly the same paths (which also carries a text whose source the statement deliberately changed), and by source string otherwise; an ambiguous source carries nothing rather than guessing. A language the statement itself wrote is never overwritten.

- **Bulk translation: `DESCRIBE TRANSLATIONS` and `CREATE [OR MODIFY|REPLACE] TRANSLATIONS`** (PROPOSAL_translations.md slices 2–4) — every user-visible string of a project, as one MDL file per language: `create translations for nl_NL ( 'Save' as 'Opslaan', … )`. Entries use `as` rather than a colon because a translation maps a user-provided name to another name. Measured on a real project, a whole app is 411 distinct source strings, so one file per language is comfortably practical, and keying on the source string means `Save` is translated once for all ~40 places it occurs.

  The three verbs take the **language** as the thing that exists: bare `CREATE` refuses when it already has translations (the "add a language" statement), `OR MODIFY` merges, and `OR REPLACE` makes the file authoritative — a translation whose source the file does not name is removed, and the run names what it deleted. `IN <Module>` scopes both directions and, under `OR REPLACE`, **bounds the deletion**, without which per-module files would wipe each other on every run.

  `DESCRIBE` emits the `CREATE` form, and an untranslated string comes back with an empty target — so the auto-translate loop needs no separate export format: describe, fill in the right-hand side, execute. A dictionary key that matches nothing is **reported rather than skipped**, because a source edited after the file was written stops matching; where the translation identifies the moved source unambiguously the run names it, and where it does not, it says so instead of guessing.

  Writes patch the stored BSON directly rather than going through the document rebuild path, so a translation can be written into any document type — including ones mxcli cannot otherwise round-trip — and re-running a file writes nothing.

## [0.19.0] - 2026-08-21

Headline: **A statement mxcli accepts is now a statement mxcli honours.** This release closes a long list of clauses, annotations and document properties that parsed cleanly, reported success and were then dropped on the floor: a test's `@setup` and `@verify`, `DELETE_BEHAVIOR PREVENT`, a List View's specialization templates, a REST call's file-document response, a widget's `contentparams`, a workflow's boundary events, a decision's rule call. Alongside that, **rules** become the last microflow-family document type mxcli can both read and write, `mxcli check --references` **type-checks expressions** against the catalog, and skills can now ship **assets** — Java actions, Vega specs, MDL — rather than prose alone.

### Added

- **Rules are an authorable document type** — `LIST RULES`, `DESCRIBE RULE`, `CREATE RULE`, `DROP RULE` and `MOVE`, on top of a rule finally being *readable*. A `Microflows$Rule` was the one document type mxcli could reference but not read: `IsRule` answered a yes/no question about a name, and nothing could list a rule, show its signature or render its body, so a rule was invisible to every surface except the decision that called it. A rule is handled the way a nanoflow is — its own semantic type and its own listing, so `SHOW MICROFLOWS` still lists microflows only.
  - **A rule is a catalog object.** `CATALOG.RULES` sits beside `CATALOG.NANOFLOWS`, with a `RULE` row in `CATALOG.OBJECTS` and rule bodies indexed into `CATALOG.SOURCE`. The reference builder walks a rule's body, which had to land in the same change: without it, a microflow called only from inside a rule was reported dead by `show callers` and `GRAPH_DEAD_ASSETS` — worse than not knowing about rules at all.
  - **What a rule may not contain is refused at check time**, by the same `validateRule` the executor calls, so `check` and `exec` cannot disagree. The restrictions are measured rather than taken from the documentation: on mxbuild 11.13, a create action inside a rule is **CE0009**, and a void or String return is **CE0103 + CE0139**.
  - Authoring is modelsdk-only; the legacy engine refuses, as it does for menus. An mxcli-authored rule carries exactly Studio Pro's twelve keys — `ExportLevel: "Hidden"` and a bare `Flows` marker included, both of which `mx check` was happy to do without.

- **`mxcli check --references` type-checks expressions** — new `mdl/exprcheck` and `mdl/exprcatalog` packages. `if $obj/Status = 'Open'` — the shape people actually write — passed a check that caught the same mistake written as a create or change member. Two causes, both invisible: `$obj/Attr` typed to nothing, so every rule downstream of it stayed quiet; and even resolved, the rule keyed off an assignment *slot*, which a comparison does not have. Attribute paths now resolve to a real entity through a new `EntityScope` seam, and a multi-hop path resolves each association through the catalog's association index — an expression path, unlike an XPath one, does not spell its intermediate entity, which is the whole reason it needs a resolver. The same defect gets the same code and message wherever it is spotted, and nanoflows are checked too. Only an `error` severity fails the run, and the false-positive probe that gated that — all 21 microflows of a Mendix 11.13 app, most of them marketplace-authored and Studio Pro-validated, described back to MDL and re-checked — came back with **0 findings**.

- **Skill packs — a skill that carries assets, not just prose** — `mxcli skill list|add|remove|upgrade`. mxcli ships 65 skills and every one was a single Markdown file, which was never a design decision so much as the limit of the mechanism: the embed was `skills/*.md`, the sync was flat, and the write path joined on `filepath.Base`, so a nested pack was silently **flattened** — `references/install.md` and `specs/install.md` collided and one vanished with no error. Three packs are vendored: `mendix-vega-charts` (Vega-Lite spec templates plus a headless checker), `mendix-odata-pushdown` (Java actions applied through MDL, with `$select` pushed into the source query) and `mendix-bulk-oql-dml`. Packs are **opt-in and never touch the model** — `skill add` writes files and prints the MDL command for the user to run deliberately — and installing one prunes assets a newer version dropped, because a stale spec template is worse than a missing one.

- **A `FOLDER` clause on `CREATE`, for every document type** (#932) — import and export mappings had none, and neither did queues, scheduled events, regular expressions, workflows, menus, image collections, Java and JavaScript actions, database connections, data transformers or the four AI-agent documents. All of them could only be created at the module root, and `DESCRIBE` emitted no folder either, so describing a document Studio Pro had filed away and replaying it recreated it unfiled. The clause goes in one place — straight after the qualified name — rather than adding a third per-doctype convention to the two that already disagree. It also closed a trap: `createWorkflowStatement` read DISPLAY / DESCRIPTION / DUE DATE by counting string literals in order, so a folder path silently became the display name and shifted every later clause by one. The tokens are now labelled in the grammar and read by label.

- **`MOVE` accepts every top-level document type** (#932) — it listed nine; the other twenty-two were never unimplemented, merely unlisted, so `MOVE IMPORT MAPPING …` was a parse error while `MoveImportMapping` sat fully wired on the interface, both engines and the mock, called from nowhere. The doctype list is now a grammar rule rather than an inline alternation, which retires the hand-written negation of every doctype keyword that told `MOVE FOLDER` from a document move — a discriminator that could not have stayed in step with twenty-two more.

- **List View specialization templates** — a List View over a generalization can render a different body per specialization. MDL had no way to author one and, worse, no read path either. Measured on a Studio Pro-authored page carrying four templates: `describe` → `exec` left **0** templates, with `mx check` reporting 0 errors before and after — the document stays valid, it just holds less, which is why nothing downstream flagged it. A template is identified by the entity it renders rather than by a name (`template for Pages.Bus { … }`), `ALTER PAGE` can reach into one, and their contents are indexed in the catalog.

- **`IN QUEUE` on a call activity** — `CALL MICROFLOW Ops.ACT_Process(Order = $Order) IN QUEUE Ops.Work;`, and the same on `CALL JAVA ACTION`. Running on a task queue is a property of the call activity, not a separate construct, and MDL could express it on neither — so a queue could be created and never bound to anything. The clause takes the same position on both, after the argument list and before any `ON ERROR`, and `DESCRIBE` renders it back.

- **REST gains a binary request body and a file-document response**
  - `body binary $Doc/Contents` on a `REST CALL`. Binary POST had been reported as impossible, and for the *consumed operation* it is — `Rest$JsonBody`, `Rest$StringBody` and `Rest$ImplicitMappingBody` are the only three, which is why grepping the metamodel for `Rest$*Body` appears to settle it. Mendix models a binary request body on the **microflow activity** instead (`Microflows$BinaryRequestHandling`). The earlier, correct-about-the-wrong-document conclusion stays as **MDL-REST02**, refusing `body: file` — which had been sending the expression text.
  - Storing a response in a **file document** (#922) had no MDL form at all and both readers fell through to String, so `DESCRIBE` reported `returns String` and the round trip wrote that back. Measured on mxbuild 11.6.6: `ResultHandlingType` went `FileDocument` → `String` and `VariableType` went `ObjectType(MyModule.MyFile)` → `StringType`, with `mx check` unchanged — the activity silently retyped, with a valid model and a green build.

- **A published OData service can be published as GraphQL** — one boolean over the same resources. Every layer already had it except the one an author reaches, and since an unknown OData property is an error (MDL-ODATA01), a script asking for GraphQL was rejected outright. `create or modify` that does not mention it cannot turn GraphQL off on a service that has it, and an omitted value is left alone rather than opted in. Gated at Mendix 10.14, where it was introduced: writing a property a version's metamodel does not have is not a build error, it is a document Studio Pro cannot open.

- **`ALTER PROJECT SECURITY GUEST ACCESS ON|OFF`**, and `ON ROLE <UserRole>` — anonymous access was readable but not writable, so an app with a public area could not be built unattended: every headless run ended with a manual step in Studio Pro. Wired the way `DEMO USERS ON|OFF` already was — same AST node, same unit, same write choke point, both engines.

- **`@setup` and `@verify` do what they say in `mxcli test`** — both were parsed into the test case and read by nothing.
  - `@setup Mod.Seed` names a microflow called before the test's own statements. It is repeatable, and a file's header comment may declare one for every test in the file — which is what makes it worth more than `call microflow X;` at the top of each body, and is only readable because a header comment stopped being swallowed by the first test (#927). A header may carry nothing else: `@expect`, `@verify`, `@throws` and `@cleanup` describe one test's execution and are refused there by name rather than ignored.
  - `@verify` runs its OQL against the app after the microflow returns, keeping three outcomes apart: holds, does not hold (FAIL, with the value that came back), and cannot be evaluated (ERROR, counted with the failures). Previously no OQL — well-formed or not, true or not, against a real entity or an invented one — could make a `@verify` fail. `@cleanup rollback` is refused alongside it, since it would undo the test's writes before the query could see them and report a confident wrong answer, and the result must be one row and one column rather than a cell picked out of a table by guess.

- **Six more model settings, plus optimistic locking** — `FirstDayOfWeek`, `DecimalScale`, `DefaultTimeZoneCode`, `UseDatabaseForeignKeyConstraints`, `UseOQLVersion2` and `SslCertificateAlgorithm` on `ALTER SETTINGS MODEL`, with the two enum-typed ones canonicalised against `generated/metamodel` and a non-member refused as **MDL-SET03** rather than written through — an enum value the metamodel cannot resolve is what makes Studio Pro throw `Sequence contains no matching element`. Separately, `EnableDataStorageOptimisticLocking` was refused under every spelling with a bare "unknown model setting", which put a security-relevant runtime setting on the needs-Studio-Pro list; it was reported as the unclosable half of a read-then-write balance race in a transfer microflow.

- **Renaming an attribute rewrites the XPath constraints that name it** (#910) — XPath names an attribute as a bare step (`[Status = 'Open']`), so the qualified-name scan that fixes `Mod.Entity.Attr` references cannot see it, and a scan for the bare name would corrupt string literals, function names and other entities' identically-named attributes. Renaming therefore left every constraint dangling, which mxbuild reports as **CE0161**. A constraint's target entity is known structurally, the edit is textual and gated by the parse so that only a single identifier token changes, and the load-bearing safety check is a count invariant. `RENAME ATTRIBUTE` now updates ordinary cross-references too. New `mdl/xpathrefs` package.

- **Import mappings can reach a nested leaf without an entity per level** (#927) — `Attr = customer/contact/email` binds a value several levels below the object element it belongs to, which is the shape Studio Pro produces when you tick a nested leaf without ticking its parents: one entity, values pulled from several depths. Previously MDL had no way to write it, so every object level in a response became an entity whose only content was an association — one generated endpoint added 21 entities, almost all pass-throughs.

  Two shapes are refused rather than written, each measured on mxbuild 11.13 rather than assumed: an **export** mapping cannot collapse levels (**CE5015** — it has to produce the intermediate node; the same member in an import mapping builds at 0 errors, and the same export mapping with only top-level members builds at 0 errors), and an import member cannot cross a `0..*` element (**CE0256** "a schema element with wrong occurrence"). Both refusals name the build error they prevent.

- **`MPR011` — loop child containment** — a new lint rule flagging a microflow activity positioned outside the loop container that holds it. This is the only automated check for the condition: the Mendix model carries no geometry rules, so such a flow passes `mx check` with zero errors and builds and runs normally, it is just drawn wrong when opened. It catches the condition however it arrives — a project written by an older mxcli, hand-edited in Studio Pro, or a future layout regression. Nested loops are checked in their own coordinate space, and an unpositioned child at the origin is skipped rather than flagged.

- **`MDL-FLOW01` — a microflow whose branches cannot be described faithfully** (#923) — `DESCRIBE MICROFLOW` renders control flow as nested `IF/THEN/ELSE`, which only works when the graph is properly nested. A Mendix microflow is an arbitrary graph, and when a branch re-enters a sibling branch's path there is no nesting that means the same thing — the describer emitted one anyway, silently. On the reported graph the log activity ran on `¬c1 ∨ c2` and was described as `c1 ∧ c2`; with the reporter's actual expressions the original **always** logs and the description **never** does, so re-executing it produced the opposite program.

  The new rule reports such microflows, and `DESCRIBE` now emits a `-- WARNING:` comment naming the decision's canvas position and refusing the round trip, rather than handing back MDL that means something else. Detection is by post-dominance in the new `mdl/microflowgraph` package — deliberately independent of the describer's own merge search, since mxcli has two of those and they disagree on exactly these graphs (which is also why the regenerated diagram came back tangled: the emitted `@merge` lands before an activity that structurally follows it). Findings are split into *recombinable* (the branches share one suffix, so the conditions can usually be folded into one decision by hand) and *interleaved* (genuinely crossed — not expressible without duplicating an activity or adding a helper variable, so it is refused rather than rewritten).

  This is the detector only. Rendering these graphs faithfully needs a label form, designed in [PROPOSAL_structured_microflow_description.md](docs/11-proposals/PROPOSAL_structured_microflow_description.md) and gated on prevalence data this rule exists to collect. The rule is deliberately **not** part of `mxcli report`'s score: the model is valid and builds cleanly, and what fails is mxcli's ability to describe it.

- **`MDL-WIDGET20` — `editable:` on a widget that has no editability** (#928) — accepted on any widget and silently dropped, so a button bound this way passed check, passed the build, and stayed enabled: a silent functional failure rather than a caught error. It cannot be implemented as asked. Measured against `generated/metamodel`, exactly **11** Pages types carry `Editability`/`ConditionalEditabilitySettings` — ten input widgets plus DataView — and **none** of the fourteen button types does; a button has conditional *visibility*, not editability. So mxcli reports it instead, naming visibility as the alternative. Both spellings are caught: `editable: 'x'` and the bracket form `editable: [expr]`, which is the one that genuinely works on inputs and would be the more surprising silent drop. A test pins the list against the metamodel so it cannot drift.

- **`MDL-WIDGET21` — `contentparams` with no placeholder to consume them** — the residue of the fix above: parameters supplied where no property text carries a `{1}`-style placeholder have nothing to attach to and are dropped on write. Previously silent.

  Root cause of both reports is one thing: the allow-lists behind MDL-WIDGET01 and MDL-WIDGET07 are widget-type **agnostic**. `isBuiltinPropName` is a single flat list holding both `ContentParams` and `Editable`; it answers "is this a real MDL property name anywhere", and both validators read it as "is this valid on this widget".

- **Further check rules** — **MDL061/062/063** (#893) for three constructs that reported "Syntax OK", were written by `exec`, and surfaced only when a human opened Studio Pro's Errors pane: a `declare` with no value (CE0038), a `return` inside a loop (CE0068), and a duplicate variable name (CE0111). Each suggested fix — `= ''`, `break`, dropping the declare — was measured back to the project's 1-error baseline before going into the message. Also **MDL-MAP01** (a nested export-mapping member, caught without a project), **MDL-PAGEARG01**, **MDL-REST02**, **MDL-SET03**, and **MDL-SEC20**, which now warns unless the script enables security. And `mxcli exec` refuses a script whose own checks report an error, rather than running it anyway.

### Changed

- **A type split takes `when <Entity> then` branches, and `else` becomes `when (empty) then`** (#913) — the two multi-way splits are now one statement with two subjects. `else` was the worst of the three defects this fixes: it reads as a default branch and is not one, mapping to Mendix's `(empty)` outgoing flow, taken when the object is null. Measured on mxbuild 11.13, a split with one `case` and an `else` still fails **CE0090**, demanding a flow for every other subtype *and* for the base entity; cover every type while keeping the `else` and it is 0 errors. `case` also meant two things — subject introducer and branch keyword — and now means one. **MDL065** states the semantics and the CE0090 consequence rather than reading as a rename.

### Fixed

- **`DELETE_BEHAVIOR PREVENT` is stored as prevent, not as keep-references** (#901) — it parsed, printed "Modified association" and stored `DeleteMeButKeepReferences`, overwriting whatever the association had, which for the reporter was a cascade. The grammar was never at fault: `buildDeleteBehavior` called `CASCADE()` and nothing else, so `PREVENT`, `DELETE_IF_NO_REFERENCES` and `DELETE_AND_REFERENCES` all fell through to the zero value — a **legal** behaviour, so no layer below could tell it had been substituted.

- **Workflow rewrites stop losing what they do not mention**
  - `CREATE OR REPLACE WORKFLOW` deleted the workflow's boundary events (#948).
  - A same-name `REPLACE ACTIVITY` lost the activity's name (#944).
  - A workflow's `PersistentId` is now carried across a rebuild (#949), the way a microflow's `StableId` already was.
  - A workflow's references are validated (#943), and duplicate activity names are settled (#945).

- **The queued-call rewrite guard was a no-op on the legacy engine** — under `--engine legacy`, `CREATE OR REPLACE MICROFLOW` still silently destroyed a task-queue binding: the exact loss the guard exists to prevent, with the same misleading signature, since `mx check` goes quiet afterwards because the configuration its CE1613 referred to is gone. The cause is a shape difference rather than a logic error — the modelsdk reader yields `[]interface{}` for arrays and the legacy reader yields `bson.A`, a **named** slice type that `case []any:` does not match, so the walk never descended into `ObjectCollection.Objects`. The legacy parser also reads `QueueSettings` back now; it had been writing the binding correctly and never parsing it, so on that engine a describe → exec cycle dropped it.

- **A decision's rule call is written instead of dropped** — the condition went with it. Alongside: a rule call is recorded as a reference, a qualified call parses in `exprcheck` instead of having its parenthesis reported as junk, and a rule or microflow call written where a Mendix expression cannot have one is refused.

- **`GRANT` widens an access rule again instead of shrinking it** (#936) — re-granting a role on an entity rebuilt the rule from that one statement, so anything an earlier `GRANT` had allowed came back `None`: `(READ (Name, Email))` followed by `(READ (Phone))` left a rule reading `Phone` alone. Structural rights went the same way, which the report did not mention — `(CREATE, DELETE, READ *, WRITE *)` followed by a narrow re-grant lost create, delete and every write right. Nothing was said at any point: no error, no warning, no diff.

  The reported trigger was wrong in a way that matters for re-testing: `WHERE` is **not** required. The same loss reproduces with no constraint at all and with `READ *`, so a fix aimed at the constrained path would have satisfied the repro and left most of the bug. The legacy engine has merged additively since it shipped, so this was a regression in the codec engine — the default — rather than a longstanding gap, and it is why the documented contract ("GRANT is additive … never removes permissions") held on one engine and not the other. Rights merge on `None < ReadOnly < ReadWrite`, so a merge only ever widens; narrowing stays `REVOKE`'s job.

- **Two XPath constraints for one role are two access rules again** (#936) — the rule upsert matched on the module-role set alone and ignored `XPathConstraint`, so `GRANT … WHERE 'A'` followed by `GRANT … WHERE 'B'` folded the second onto the first and overwrote its constraint, destroying a rule with no warning. Mendix combines the rights of every rule naming a given module role ("Rules are additive", refguide/access-rules), making one rule per constraint the ordinary way to write row-level security — a pattern MDL could not previously express. The constraint is now part of the match key on **both** engines, with the empty constraint treated as a value rather than a wildcard, so a constrained and an unconstrained rule coexist and re-running a script stays idempotent. `mx check` accepts the result at 0 errors.

  Consequently the `Result:` line after a `GRANT` now reports the rule the statement actually touched, rather than the first rule mentioning that role.

- **Authoring a pluggable widget with an object list no longer raises CE0463** (#891) — an object-list item's *required* TextTemplate that the author left unset was written as `null`, so a freshly authored Accordion failed "the definition of this widget has changed" against the very package it was built from. The empty-ClientTemplate convention was a hardcoded table covering only DataGrid columns; required-ness now comes from the widget's own PropertyTypes, and the property is serialized with the widget's shipped translations — the same text `mx update-widgets` writes. Both weaker forms were measured and rejected: `null` is CE0463 and an empty template is CE4899. Optional TextTemplates keep their null, which is what Studio Pro stores.

- **`DESCRIBE PAGE` no longer renders an Accordion group empty** (#891) — an object-list item's child widgets (the group's `content` slot) were never read, and the emitter had no body to put them in, so a group holding a DataGrid2 described as a bare `group group1 (…)` and a describe→exec round-trip silently deleted the grid. Both halves are fixed, and the description now re-parses with the nested widgets intact. Applies to any pluggable widget's object-list items, not just Accordion.

- **`ALTER PAGE INSERT`/`REPLACE` no longer corrupts a page when a DataGrid2 column is named without its grid** (#891) — `REPLACE NextRunAt WITH { COLUMN … }` reported success while writing a layout container into the grid's column list, leaving a project Studio Pro and mxbuild could not **load** (`InvalidCastException: DivContainer → WidgetObject`). `DESCRIBE PAGE` skipped the malformed node, so REPLACE looked like a clean deletion and INSERT like a harmless no-op; both were corruption, and neither required the grid to be nested in a pluggable widget. A bare name that resolves to an object-list item is now refused, naming the qualified `grid.column` form — which always worked and is unaffected.

- **`DROP FOLDER` no longer orphans the documents inside it** (#892) — the command's contract ("the folder must be empty") existed only in a comment; nothing checked, so dropping a populated folder left every document pointing at a container that no longer existed. Nothing was deleted: the documents were **orphaned**, losing their module qualification (`FeedbackModule.IMM_PostResponse` → `.IMM_PostResponse`) so nothing could resolve them and mxbuild reported CE1613. Reproduced on a *stock* blank app, where `FeedbackModule/Private/Resources/Mappings` holds four documents. The drop is now refused, naming what is inside. The guard reads the type-agnostic unit list rather than the per-kind document lists, so it cannot inherit the blind spot that caused the bug, and it fails closed when contents cannot be determined.
- **`LIST FOLDERS` counts mappings, JSON structures, regular expressions and image collections** (#892) — these five kinds were missing from the per-kind listing, so a folder holding them rendered as `[0]`. That empty count is what made dropping the folder look safe.

- **A pluggable widget's text template no longer drops its `contentparams`** (#928) — `imageUrl: '{1}', contentparams: [{1} = PictureUrl]` on an Image widget stored a template with an **empty** parameter list, and mxbuild rejected it with `CE0720` ("place holder index 1 is greater than 0, the number of parameter(s)") on the **first write** — no describe round-trip needed. The engine took the parameters path only for mxcli's `{AttrName}` convenience spelling, so Mendix's own numeric `{1}` form had no route. Both spellings now reach the same stored shape; a `dynamictext` with identical syntax was the control that localised it to the pluggable path.

- **A `SHOW_PAGE` widget argument that is not the context object is refused instead of ignored** — `action: show_page Mod.Detail(Car: $Other)` inside a data view bound to `$Car` opened the page with **`$Car`**. mxcli stores a widget show-page action with an empty `ParameterMappings` array and lets Mendix infer the argument from the enclosing widget, which is deliberate and required (an explicit mapping is rejected as CE0115, #296) — but an argument naming anything else was then dropped on the floor by both engines. Nothing reported it: `mx check` gave **0 errors**, because an inferred mapping is a valid mapping, and `DESCRIBE` printed `(Car: $currentObject)`, so the description read as a diagnosis of a lost mapping rather than an accurate report of a model that never held one. The builder now refuses such an argument, and `mxcli check` flags it as **MDL-PAGEARG01** with no project needed. The guard fires only where it can prove the argument is discarded: `ALTER PAGE`'s `SET`/`INSERT` build an action against a stored page they never traverse, so the context object is unknown there and those statements are unaffected. Arguments that *do* name the context object — `$currentObject`, or the variable the enclosing data widget is bound to, which is the form the skills document — are unaffected. Found while verifying mxcli-formula1 §39, whose reported half (DESCRIBE omitting the inferred mapping) was already fixed.

- **`DESCRIBE` no longer mis-reads a mapping that binds a nested leaf** (#927) — value elements were printed as the last segment of their JsonPath alone, so a project holding `(Object)|customer|name` described as `CustomerName = name`. That is a description of a model that does not exist, and re-executing mxcli's own output failed with `"name" is not a member of the JSON structure at (Object)`. Members are now rendered relative to the enclosing object element, on both engines and for both mapping kinds. Nothing was ever corrupted — the #882 guard refused the bad re-execution — but the description was wrong.

- **A loop's box is sized from its contents, not from a statement count** (#884 problem 1) — `LOOP`/`WHILE` containers took their `Size` from a pre-pass over the AST run before the body was built, so it depended only on how many statements were inside. Two activities at x=150/310, at x=1500/2000 and at x=160/170 all produced `480;160`, and in the second case both children sat entirely outside their own container with `mx check` reporting nothing. The box is now derived from the real child bounding box after the body exists, which also makes an explicit `@position` on a loop child effective. Nested loops size bottom-up. Children are not moved: their positions round-trip through `DESCRIBE`, so the box grows to fit them rather than the contents being translated to fit the box.

- **A microflow's StartEvent no longer moves on a describe→exec round-trip** — the start has no MDL statement to annotate and `DESCRIBE` cannot emit its position, so the builder always derived one (first annotated activity minus one spacing unit). A Studio-Pro-authored flow whose start sat at `145;200` came back at `100;200` — the only coordinate in it that did not survive. The position is now carried over from the microflow being replaced, the way the folder and allowed module roles already are; a fresh `CREATE` still derives it.

- **The Mendix 10.24 nightly is green again** — `14-project-settings-examples.mdl` set `DecimalScale`, which 10.24 does not have, so `TestMxCheck_DoctypeScripts` failed on that matrix entry on both engines while passing on every 11.x. Measured against blank projects: 10.24 stores 11 model settings, 11.6.6 stores 12, and `DecimalScale` is the only difference — each of the other five settings in that statement is accepted on 10.24 on its own. mxcli's refusal was correct (Studio Pro will not open a model carrying a property its version does not define, and mxbuild does not catch it); the example simply was not version-gated, and because the refusal covers the whole statement, one unsupported setting took five portable ones down with it. It is now its own `-- @version: 11.0+` section.

  A new guard, `TestDoctypeScriptsParseAfterVersionFiltering`, filters every doctype script for each nightly matrix version and asserts the result still parses. It needs no mxbuild, so a mis-gated script fails in seconds on push rather than hours later in a single nightly job. Writing it surfaced the trap that makes this easy to get wrong: a `/** */` block is a *documentation* comment bound to the statement after it, so gating the statement while leaving its comment outside the section orphans the comment and the script stops parsing — reported at the *next* statement, tens of lines away, which reads like an unrelated syntax error.

- **A long tail of smaller fixes**
  - `DROP ATTRIBUTE` left orphaned validation rules behind (CE1613).
  - `SHOW ACCESS ON ENTITY` did not parse, and `SHOW ACCESS ON PAGE` described the page instead.
  - A grammar alternative with no visitor branch exited 0 in silence; it now reports.
  - Windows: `.exe` is appended to the java path and a per-user JDK is found. `run --local` picks an mxbuild the host can actually execute, and says so when none can.
  - `mxcli test` leaves the project byte-identical after a run; `--require-assertions` was a silent no-op on the default runner; an `@expect` the runner cannot evaluate now fails closed; and each test reports what it actually asserted.
  - `DESCRIBE`: a flow datasource's arguments are carried through, an XPath constraint is always emitted as a bracketed group, and a widget datasource is read and rendered in one place (#941).
  - Lint: `CONV010` and `QUAL004` stopped reporting correct code as violations, and `MDL003` no longer demands a `RETURN` the builder synthesizes.
  - Pages: `OnChange` is written on every input widget rather than only textbox; a pluggable widget's action is read back and the association storage key refused; a Mendix array can be created and is never written without its marker; the page keys Studio Pro writes and the marker its reader needs are written; and the hidden widget property that fails the build is flagged rather than the harmless ones (#931).
  - Mappings: an unauthored import range is `All`, not `First`, and `DESCRIBE` reproduces the script that made the mapping.
  - The calculated-attribute binding (`CALCULATED BY`) is written **and** read.
  - A database connection's SQL was replaced by a parameter default, with a lossy read on the default engine.
  - `canon` carries stored element `$ID`s onto a rewritten document; writes that land outside unit storage are counted rather than reported as skipped.
  - Excluded documents stay excluded, and the live twin is targeted.
  - An additive chain keeps its operators in the order they were written.
  - A building-block datasource override is rebound by widget type, not by one that happens to be present already.


## [0.18.0] - 2026-08-14

Headline: **mxcli can now maintain a project it did not author.** Marketplace modules install and update headlessly — carrying the GUIDs the database keys on, the role grants that live outside the module, and the MPR v2 format `mx module-import` silently collapses — and `marketplace diff` reports which elements were edited locally before an update replaces them. Alongside that, a write that changes nothing no longer touches the file, five more document types become authorable (task queues, scheduled events, regular expressions, validation rules, menus), and a long tail of activities that could be written but not read back stop disappearing from the describe → edit → re-exec loop. Separately, the Windows and macOS binaries stop shipping the embedded tunnel — 13.5 MB smaller, and no longer carrying the tunnelling stack that had Defender and enterprise EDR blocking mxcli on managed corporate endpoints.

### Added

- **Headless marketplace module install and update** — the lifecycle Studio Pro owned is now scriptable, built on measurements of what Studio Pro's own update does to stored BSON rather than on inference:
  - `mxcli marketplace update <content-id> -p app.mpr --to <version>` replaces a module's units with mxcli's own writer. It **refuses when the module has local edits**; `--save-edits` writes them out as re-executable MDL first and `--force` proceeds, so a destructive update becomes park → replace → replay.
  - **Element GUIDs are captured and transplanted** onto the replacement. Measured on a live PostgreSQL at 11.12.1, the runtime's identity map holds the model's GUIDs verbatim — so a replace that drops them destroys that module's data on the next deploy, silently, with a valid model and a green build.
  - **Module role grants are restored.** A user role's grant of a module role lives in the *project's* security document, not in the module, so removing the module takes it away and putting it back does not return it. Measured on a blank 11.12.1 app: dropping Administration left Administrator holding 2 module roles instead of 3, with nothing to complain about — the app builds and users quietly lose access.
  - **Every file a package ships is installed**, not just the model: widget binaries (a module reporting 3.11.3 while all ten binaries on disk were still 3.5.0 produced no build error), plus a module's own `themesource/` design properties. A module's bundled widgets no longer roll back newer copies already in the project.
  - `mxcli marketplace diff <content-id> -p app.mpr [--to <version>] [--json]` reports which elements of an installed module have been edited locally — the question Studio Pro's update never asks before discarding them — and which of those an upgrade would collide with. Comparison is on DESCRIBE output, not BSON: an *untouched* module differs from its own published package in ~15,000 BSON paths.
  - `mxcli marketplace install` uses that writer too, so **installing a new module works on MPR v2 projects**. The only previous path was `mx module-import`, which rewrites a v2 project as v1 — measured on a blank 11.12.1 app, one import turned a 69,632-byte `.mpr` plus 341 `.mxunit` files into a single 14 MB SQLite blob.
- **One precedence chain for constant values**, applied identically by `run --local`, `test --local` and `test --attach` — `--constant Module.Name=value` (repeatable) wins over everything and is written nowhere, so running once with a different API key no longer means committing it; below it `mxcli constant set/unset/list` keeps a machine-local store at `<project>/.mxcli/constants.json` (mode 0600, gitignored) for a secret that must not be committed, because Mendix's own private configuration value has been encrypted per user account since 10.9 and is unreachable headlessly; below that the configuration's shared overrides, then the constant's default. `mxcli constant list` shows which layer each value comes from (`--show-values` to print them), and `--apply` pushes a change into an already-running `mxcli run --local` — two admin calls, because `update_configuration` only stages and `reload_model` applies. A name the project does not declare is refused before anything boots, since the runtime silently ignores an unmatched entry.
- **Five more document types are authorable from MDL**, each wired grammar → AST → visitor → executor → backend on both engines, with `SHOW`/`DESCRIBE`, catalog entries, and the CLI/help surfaces:
  - **Task queues** — `create [or modify] queue`, `drop queue`, and a microflow call that binds to one. Previously a queue binding authored in Studio Pro was **deleted** by `create or replace microflow`, taking the `CE1613` that reported it along with it, so the loss was invisible from every angle.
  - **Scheduled events** — Mendix's cron, with all eight repeat variants. A field belonging to another variant is refused: a merged field set produces a document mxbuild accepts and Studio Pro cannot open.
  - **Regular expressions** — previously unsupported at every layer, so a pattern shared across a domain model could only be seen in Studio Pro.
  - **Validation rules** — `create validation rule for Module.Entity.Attribute` binding a regex or a range. The grammar existed and wrote nothing; underneath, `modelsdk/gen` bound the RegEx rule's reference to the wrong BSON key (`RegularExpression`, where Studio Pro stores `RegExIdentifier`).
  - **Menu documents** (`Menus$MenuDocument`) — `describe` / `create or modify` / `drop menu`, reusing the same menu-item grammar `CREATE NAVIGATION` uses.
- **Canvas layout annotations** — a scripted model can lay out its own diagram, not only its boxes: `@anchor(from: (0, 54), to: (100, 54))` places where an association's connector meets each entity (percentages, settled across 88 coordinate pairs in four Studio-Pro-authored sources); `@curve` sets a sequence flow's two bezier control vectors (Mendix stores no waypoints — the shape is the tangent handles); `@merge` positions the implicit merge node closing a split, which had no annotation of its own and routinely landed on top of a neighbouring activity. (#872, #884)
- **`ALTER PAGE … SET Action = microflow M.F ON <widget>`** — retarget a widget's action in place. The value position could not hold an action at all, so this did not parse. (#855)
- **New pre-build `check` coverage** — icon-collection references now resolve, so a wrong `icon:` is caught at `mxcli check` instead of as `CE1613` from the build; `MDL044` refuses the write instead of only reporting it, and stops flagging three real built-in expression functions (#828, #833); and every `mxcli syntax` example is now held to actually parsing — 26 of 120 entries documented MDL that did not, on the surface an agent consults first.
- **MCP capabilities are gated on a live probe** (ADR-0006) — tool presence is answered by `tools/list` and actually gates the feature, with the project's Mendix version consulted separately; a missing tool or an unanswered probe fails the feature closed and says which.
- **`DESCRIBE` reaches further** — building blocks and icon collections resolve by bare `describe Module.Name`; `DESCRIBE MODULE` emits the module's role list, the one part of module security no describe emitted; and a document nested in folders resolves its module through the folder chain instead of reporting "not found".
- **A `bootstrap-app` skill ships in the binary** — the empty-repo seed prompt shrinks from ~180 pasted lines to three steps, so fixing the procedure no longer requires everyone to re-copy a new prompt (it exists to be pasted from a phone).

### Changed

- **The embedded tunnel now ships in the Linux build only** (`mxcli run --hub`, `mxcli tunnel-hub`). The tunnel embeds [chisel](https://github.com/jpillora/chisel), a dual-use tunnelling tool that appears in threat intelligence as a pivoting component. It only ever runs inside a Linux container, but every platform linked it — so Microsoft Defender flagged the Windows binary (`Trojan:Script/Sabsik.EN.A!ml`) and enterprise EDR flags this class of payload harder still, blocking mxcli on the managed corporate endpoints most Mendix developers use. Both chisel imports now sit behind a one-interface, Linux-only seam; a CI guard (`scripts/check-tunnel-deps.sh`, `make check-tunnel-deps`) fails the build if chisel or its SSH/websocket/socks dependencies reappear in a windows/darwin dependency graph. **Windows and macOS release binaries are 13.5 MB smaller (-14.7%)** and contain no tunnelling code. The Linux build is unchanged. On other platforms the two commands remain registered and documented but fail with an actionable message. Note that this is a *different* problem from the `Wacatac.C!ml` report in [#185](https://github.com/mendixlabs/mxcli/issues/185), which was a genuine generic Go-binary false positive: here the capability really was in the binary, and code signing would not have addressed it. We did not obfuscate or repack anything — the fix is not shipping the capability where it is unused. ([#890](https://github.com/mendixlabs/mxcli/issues/890), [ADR-0009](docs/13-decisions/0009-tunnel-is-linux-only.md))
  - **Breaking, narrow:** `mxcli tunnel-hub` can no longer be hosted on Windows or macOS — move the hub to a Linux host. Developers on native Windows/macOS installs must run mxcli inside the project's devcontainer to use `--hub`.
  - `tunnelhub.ServerOptions.ChiselAddr` is renamed to `ControlAddr` (it addresses the platform-agnostic control server).
- **A write that changes nothing no longer happens** ([ADR-0008](docs/13-decisions/0008-identity-and-idempotence.md)) — re-running an MDL script against a project already in sync leaves the `.mpr` and `mprcontents/` byte-identical, so Studio Pro and git show no version-control changes. Comparison is on a canonical form with every element `$ID` normalised away, because a rebuild mints fresh random IDs and a byte comparison would skip nothing; `Microflows$Microflow.StableId` is carried from the stored document rather than re-minted, since the build derives every client-callable microflow's operation id from it. One policy in `modelsdk/canon`, called from both engines' write choke points. Measured on mxcli-sudoku (412 units: 386 identical, 26 volatile-only, 0 real differences) and confirmed for document-type coverage on the multi-app mxcli-formula1 solution. `MXCLI_ALWAYS_WRITE=1` disables elision (not identity preservation) for bisecting — and any claim of idempotence needs that control run alongside it. An `$ID` is never renumbered in place: the attempt that did (PR #125, reverted) left pointers dangling and made projects unopenable.
- **Go toolchain 1.26.5 → 1.26.6** for GO-2026-6218 (`net/url`), GO-2026-6090 (`crypto/tls`), GO-2026-6089 (`net/http`), GO-2026-6088 (`encoding/xml`), GO-2026-5972 (`encoding/asn1`) and GO-2026-5026 (`net/http`, via `golang.org/x/net/idna`). All six are standard-library advisories fixed in go1.26.6; no mxcli code changed. Bumped in `go.mod` and in all three workflows (`push-test`, `release`, `nightly`) together, so released binaries are not still linked against the vulnerable standard library.
- **Marketplace reference projects are cached** — `diff` and `update` each build a blank app of the consuming project's version with the published module imported, at the cost of a download plus two `mx` invocations, and `update` builds two. The result depends only on its inputs and was thrown away every run. Only the reference *model* is cached: of a 34 MB entry, `widgets/` (9.6 MB), `themesource/` (6.4 MB) and `theme-cache/` (2.1 MB) were never read.
- **Slash commands appear in the `/` menu** — none of the 16 command files carried the YAML `description` frontmatter a client lists them from, so they read as "not installed" despite always being invocable by full name. Both namespaces are fixed, and `mendix/` is embedded into `mxcli init` output.

### Fixed

- **Activities that could be written but not read back** — on the default `modelsdk` engine each read back with a nil action, so `DESCRIBE` rendered a placeholder and a describe → edit → exec cycle **deleted** the activity:
  - the eight workflow call actions (call workflow, get workflow data / workflows / activity records, open, lock and unlock workflow, workflow operation)
  - `EXECUTE DATABASE QUERY` (#863) — its storage `$Type` lives in the DatabaseConnector sub-metamodel, not `Microflows$`
  - `TRANSFORM JSON`, which was worse: the writer had no case for it either, so the enclosing activity was written with no action at all
  - an unsupported action now keeps its **error handlers** and is named in the output. An early return dropped the error branch of *every* activity that rendered as a comment, not just this one's. `SYNCHRONIZE` is now supported. (#863)
  - an import mapping's **custom Range** was unrepresentable — only `ConstantRange` was ever written or read, so a bounded import silently became unbounded (#881)
- **Pluggable widgets** —
  - authoring a property inside a conditionally shown group wrote a widget Mendix rejects with `CE0463`: a *visible* conditional property stores an empty `Forms$ClientTemplate` where a hidden one stores null, and the editor-config reader did not understand a ternary's else branch, so the gate was never seen. Filling visible templates without the gate merely inverted which case failed; both halves are measured both ways round. (#574, #104)
  - a page carrying a pluggable widget could not be described and re-applied — explicit properties went out through a raw `%s` so every string lost its quotes, and boolean values were skipped as "common defaults". Quoting is now decided by the declared property type, never the value's shape. (#104)
- **Constants written to a configuration never reached the running app** — `alter settings constant … in configuration 'Default'` executed, reported success and round-tripped through `describe settings`, but mxbuild writes each constant's *default* into `deployment/model/config.json` and that map is what `run --local` hands the runtime. An app ran for hours with an empty encryption key while the model said otherwise. `test --local` now resolves constants the same way `run --local` does, closing the gap where a test passed under `--attach` and failed under `--local` with nothing in either output to explain it.
- **Access rules** —
  - a `GRANT` naming an association inherited from a generalization was refused, which made the rule OpenAIConnector ships impossible to express in MDL (#758)
  - a `GRANT` on a specialization wrote a model Mendix rejects with `CE0066` "Entity access is out of date" — the grant collected the inherited associations and the reconcile that runs next deleted them again
  - dropping an association left behind every `MemberAccess` that named it, so Mendix rejected the model with `CE1613`
- **Validation rules were silently downgraded** — `alter entity … add attribute` on an entity carrying a RegEx or Range rule rewrote it as a **Required** rule: the reference gone, the field merely mandatory, and `mx check` silent, because a Required rule is perfectly valid. Both engines; the only evidence was the stored `$Type`.
- **Association line anchors were destroyed, not merely omitted** (#872) — `ParentConnection`/`ChildConnection` were hardcoded to `0;50`/`100;50` by both writers and read by neither parser, so any association write reset a hand-placed connector.
- **Pages** —
  - a `SNIPPETCALL` parameter satisfied by context now takes no mapping. There was no working spelling: omitting `params` was refused by mxcli, and supplying `$currentObject` was accepted and then failed the build with `CE0115`.
  - an association in an attribute-typed widget property is refused instead of writing a dangling `AttributeRef` (`CE1613`), and the drop-down filter's association mode is authorable (#830)
  - `ALTER PAGE … SET DataSource = $Param` works, matching what the identical retype through `REPLACE` could already do (#854)
  - page templates are no longer read as pages — `Forms$Page` is a prefix of `Forms$PageTemplate` and the type match was on the prefix, so `show modules` reported Atlas_Web_Content as having 46 pages when it has none, and both engines had been aligned on the wrong answer
- **JSON mappings** (#882) — a member now resolves by *either* of the two names a `JsonStructures$JsonElement` carries (the raw JSON key the runtime resolves by, and Mendix's derived exposed name that Studio Pro displays) instead of inventing paths; the JSON snippet's sample value is no longer copied onto mapping elements, where Studio Pro leaves it empty; and an unknown member is refused only when there is a schema to resolve against, so a mapping with no `with json structure` clause is no longer rejected outright.
- **A database connection's literal credentials wrote a project Mendix cannot open** (#854) — connection string, username and password are by-name references to a `Constant` and have no literal alternative, but the grammar offered a string-literal spelling and the executor passed it straight through, producing a project `mx check` could not even load. Literals are now refused.
- **`mx update-widgets` and `mx rename-design-properties` collapsed MPR v2 into v1** (#808) — each fixes something only Mendix can fix (`CE0463`, `CE6087` — the normal aftermath of a headless module install) and each rewrote the project into the single-file format while doing it: measured at 11.12.1, `rename-design-properties` took 1,865 `.mxunit` files to 0 and a 249,856-byte index to 39,895,040 bytes. Their repairs are now persisted without the collapse.
- **Catalog and lint** — scheduled events and queues are indexed, so a microflow run only by a scheduled event is no longer reported as dead from three directions at once (`show callers`, `CATALOG.GRAPH_DEAD_ASSETS`, `QUAL004`); and a page reference reached only from a widget action button is recorded, so `show callers` stops answering "(no callers found)" — a false negative that reads as "safe to delete" (#773).
- **Crashes and robustness** —
  - `log 'msg' with ()` panicked on a nil dereference, so `check` printed a stack trace instead of a diagnostic and every other command that parses the file died the same way
  - `mxcli check` segfaulted on `SQL DISCONNECT source;` — a line copied from mxcli's own `mxcli syntax sql` example. Keyword connection aliases are accepted and a bad one is reported.
  - `mxcli new --output-dir <deep path>` left a few hundred files and no `.mpr`: MxToolset refuses any destination path over 259 characters and aborts extraction part-way through, so the project is now created in a staging directory (#825)
  - the generated dev container installed `postgresql-client` only, so `run --local --ensure-db` failed on a freshly built `mxcli init` container that looked correctly provisioned — `psql` was on `PATH` with no server behind it
- **Marketplace client** — the versions endpoint is paginated, so an older version no longer reports as not published (Data Widgets has 131 releases; mxcli could see 10); a version the project's Mendix cannot import is refused up front, since the marketplace publishes against the newest patch within days and `install` with no `--version` routinely resolved to the one release that cannot be installed; and theme modules (Atlas_Core, Atlas_Web_Content, Conversational UI) can be diffed, `mx module-import`'s outright refusal being gated on a single BSON boolean that is cleared on a throwaway copy.
- **A java action parameter typed `Microflow` read back as a String** — so `MCPServer.AddTool`'s `ExecutingMicroflow`, and every other "register a callback" action, authored the callback in the wrong shape.
- **`check --references` reported "java action not found"** for an action the same script created a few statements earlier — the script's own declarations covered seven kinds of object but not java or JavaScript actions.
- **An association's target entity is qualified in expressions** (#854) — an association whose target lives in another module is a `DomainModels$CrossAssociation` and the lookup that inserts the entity step walked only the local domain model, ending in `CE0117` at build time with `exec` and `check --references` both silent.
- **Studio Pro 11.13 truncated MCP page reads** (#697) — `pg_read_page` gained a `depth` argument defaulting to 4, replacing anything deeper with `"..."`, and `ALTER PAGE` over MCP is read-modify-replace-whole-page, so the truncated read went back as the new page body and was rejected.
- **Agent documents are version-gated** — `sdk/versions/mendix-11.yaml` carried no agent area, so `show features` listed nothing and a pre-11.9 project got no error at all. This is the one version gate with no downstream safety net: agent-editor documents are stored as custom blobs that mxbuild does not validate.

## [0.17.0] - 2026-08-10

Headline: **A full Mendix build-and-test loop that fits on an iPad** — you can now design, build, run, observe, and debug a multi-app Mendix solution end-to-end from Claude Code on the web, on a phone or tablet, with no local IDE. Two capabilities make it possible: an **external browser preview** that reverse-tunnels a locally-running app out to a public URL from an egress-only container, and a **short agentic feedback loop** — a warm Docker-free runtime, sub-second microflow unit tests, live log/metric/trace observation, and a name-based microflow debugger — so an agent gets an answer in seconds instead of a build round-trip.

### Added

- **Build a Mendix app from Claude Code web, on an iPad or iPhone** — the app runs locally in the container and reverse-tunnels out over a single 443 connection to a static relay, so it is reachable in any browser at a public URL, verified live through the session's egress proxy. `mxcli run --hub <url>` boots the runtime with its `ApplicationRootUrl` set to the assigned URL (so the SPA and `originURI` work under the public origin) and retries forever; `mxcli tunnel-hub --domain <base>` is the **multi-tenant** relay that fronts many previews at per-subdomain hosts over one 443 with per-subdomain autocert, an availability overview **grouped by Claude Code session** (each session links back to its `claude.ai/code` conversation), and durable per-session endpoint history that survives restarts and reaping. A **SessionStart hook + bootstrap prompt** (`mxcli init`, `run --local --setup`, `docs-site/src/tools/bootstrap-prompt.md`) lets a fresh or reaped web session self-provision — cache mxbuild + runtime, ensure the local database — with no manual step, and the workflow covers **multi-app solutions**, not just a single app.
- **Hub GitHub authentication** (opt-in) — a **viewer plane** (GitHub OAuth + HMAC-signed SSO cookie, owner-checked previews, `--require-auth`) and a **registration plane** (durable hashed hub API keys presented as `X-Hub-Key`, minted from the hub's `/cli` browser page or `mxcli auth hub login --token <pat>`; `run --hub` reads `MXCLI_HUB_KEY` and degrades to local-only if registration fails). Append-only JSONL audit trail, no secrets. Absent flags leave today's open hub unchanged.
- **Docker-free microflow unit tests** (`mxcli test --local`) — tests run on mxcli's own warm runtime (no Docker daemon) on isolated ports and a `<project>_test` database, driving a **token-guarded test endpoint** — one microflow invoked per test over HTTP, so a throwing test fails only itself and results are returned rather than log-scraped. `--watch` keeps the runtime warm (~30s first run, then ~2s); `--attach` runs against an app already up under `run --local --test-endpoint` with no boot; `@cleanup` rolls back each test's writes.
- **The warm local dev loop** (`mxcli run --local [--watch] [--screenshot]`) — a Docker-free `mxbuild --serve` + standalone runtime with hot `reload_model` for behavioural changes and restart+DDL for structural ones, bundling the browser client so Mendix 11.x apps actually render. `--watch` keeps an incremental rollup bundler hot (~3–4s page re-bundle, skipped for model-only edits); `--ensure-db` provisions the local Postgres + app database; `--screenshot` captures a Playwright PNG each change, with `--screenshot-url` deep links and form-login via `--screenshot-user`/`--screenshot-password`.
- **Runtime observation — logs, metrics, and OpenTelemetry traces** — full visibility into the running app for an agent:
  - **Logs** — the runtime log is tee'd to a file for inspection, and `feat(log)` drives the runtime's per-node log levels from MDL.
  - **Metrics** — `run --local --metrics` registers a Prometheus Micrometer registry served at `/prometheus`; `--runtime-setting Key=Value` folds arbitrary runtime config (otlp/influx/statsd registries, span filters) into the single boot `update_configuration` call.
  - **Traces** — `--trace` attaches the bundled OpenTelemetry Java agent with sensible default span filters (unfiltered per-activity tracing is ~10× slower); `--trace-otlp <endpoint>` switches to the OTLP exporter for a collector and flame charts; `--trace-service` sets the service name.
  - An `analyze-runtime` skill ties logs, metrics, traces, and the catalog together.
- **Microflow/nanoflow debugger** (`mxcli debug`) — set breakpoints **by name** (activity resolved from the model), inspect paused flows and variables (`inspect --list` for list variables), and step over/into/out or continue against a `run --local` runtime; `run --local --debug` enables it at boot. Nanoflows are auto-detected, and nanoflow `LOG` output is rewritten to the runtime log's `Client_Nanoflow` node.
- **Default styling — generated apps that look designed on first boot** (`mxcli theme`, `mxcli new --theme`) — three themes ship in the binary: **signal** (the default: cool slate, one teal signal colour, 4px radius, 32px rows, IBM Plex), **ledger** (warm paper, hairline rules instead of card shadows, Source Serif over Source Sans) and **console** (dark-first, Space Grotesk over JetBrains Mono). A theme is files under `theme/` only — the model is never touched, so it hot-applies under `run --local --watch` and cannot affect a build. Atlas Core is untouched, so projects stay upgradable. Re-branding is one line (`--mxt-brand`); Atlas derives the whole colour ramp from it. Fonts are vendored (SIL OFL 1.1) rather than pulled from a CDN, so generated apps render correctly air-gapped. Generated regions are digest-fenced: a block carrying local edits is refused rather than overwritten. `mxcli theme list | show | apply | remove`; `--theme none` opts out. See `docs-site/src/tools/theme.md`.
- **Light/dark palettes and runtime theme switching** — every theme ships both palettes. `--variant auto` (the default) follows the operating system's `prefers-color-scheme` **before first paint** and honours a `theme-light` / `theme-dark` class on the root element; `--variant light|dark` bakes a single palette. Mendix ships the `:root.theme-dark` slot but nothing that applies it, so `mxcli theme switcher install` adds the JavaScript actions and a nanoflow for a toggle button — the one theme subcommand that writes to the model. Known limit: a reload falls back to the OS preference, because Mendix has no page on-load event and the usual substitute (a data view with a nanoflow data source) is not authorable on either engine yet.
- **Publish microflows and entities as OData** — `CREATE PUBLISHED REST SERVICE` gains the ability to publish a **microflow as an OData action** (with a named custom-authentication microflow), a `DESCRIBE` that round-trips it, and published-entity controls to turn `Countable`/`Skip`/`Top` on or off and to publish `Integer` as `Int64`. Generated external entities now follow the contract's `$top`/`$skip` capabilities and honour capability annotations.
- **`mxcli new` runs the first build** so a fresh clone does not go dirty the first time anyone builds it — the build settles the JS/Java action stubs MxBuild rewrites (48 tracked files in a blank 11.12 app). `mxcli theme apply` runs in the pipeline; `--skip-build` opts out.
- **New `check` heuristics** — caught at `mxcli check` before a build round-trip: `MDL053` (loop variable used outside its loop), `MDL054` (validation rules on a non-persistent entity), `MDL055` (XPath association traversal from a variable), `MDL056` (empty), a read microflow that cannot keep its OData resource's promises, a forward page reference without a project, and OData property names that are silently discarded. `MDL009` (false-positive) retired.
- **MDL language and authoring additions** — `SET` is now optional so `$Total = 5;` parses; multi-line strings and keyword widget names; `-` reads MDL from stdin (`exec`/`check`); `MENU ITEM … ICON` navigation icons; `MOVE JAVA ACTION` / `MOVE ODATA SERVICE`; `CREATE OR MODIFY MODULE ROLE`; `IF NOT EXISTS` / `IF EXISTS` on `ADD`/`DROP EVENT HANDLER`; declared JAR dependency resolution instead of silent skipping; a combobox binding an association via `Association:`; and dynamic-text parameter formatting via a `FORMAT` block.

### Changed

- **`QUAL002` sweeps every document type** (including Java actions and their parameters) and no longer reports the System module; every lint rule now excludes System, and `mxcli init` seeds a lint config that excludes it. Failed catalog queries in lint are reported rather than swallowed.
- **`mxcli new` pipeline** — now `mx create-project` → `mxcli theme apply` → `mxcli init` → first `mxbuild --target=deploy` → Linux mxcli binary, producing a ready-to-open, already-styled, already-built project.

### Fixed

- **Page and widget serialization / round-trip** — a batch of fidelity fixes: an association datasource's name is now qualified (an unqualified one wrote an unloadable page); cross-module association paths and inherited attributes resolve in page bindings; a parameterized-microflow datasource binds its arguments; DataGrid2 dynamic-text columns carry their `FORMAT` block and no longer trip CE0463; `ALTER PAGE` reaches widgets inside a `customContent` column and sets an action button's caption via `CaptionTemplate`; empty/literal `DYNAMICTEXT` content and `ListView` `PageSize` are honoured; `DownloadFileAction` writes on the modelsdk engine; an unresolved association `DestinationEntity` is never written.
- **Microflow / workflow correctness** — aggregates are kept whole now that `SET` is optional; the `InheritanceSplit` and its case values are written; the false branch of a conditional break/continue in a loop is wired; a dynamic query expression passes through unquoted; MDL comments are kept out of the Mendix expressions they sit in; `$workflowContext` is normalized in `DECISION` expressions; `DESCRIBE` no longer invents an `else` on a type split or an `on error rollback` clause; a standalone `annotation` (which wrote an unloadable model) is refused.
- **OData / integration** — `create-external-entities` no longer duplicates suffixed associations or renames an attribute called `name`; the modelsdk writer writes `AllowedModuleRoles`; a published service's role grants survive a modify; consumed REST operation mappings persist and read back (#843); `$metadata` is fetched with the client's own (constant-resolved) credentials.
- **Security** — a `GRANT` now covers every entity member, including both-owner associations and audit members; audit-member rights are rejected at check time (`MDL-SEC01`); a cross-module `GRANT` is reported before exec applies the script; demo users that cannot materialise are warned about.
- **`run --local` robustness** — the web client bundle is verified after boot, not before; stale ports are refused and the holding process is named; a relative project path resolves where MxBuild is called; the browser bundle is re-served after a structural `--watch` change; boot uses live-preview flags so `mxcli oql` can reach the app; the hub tunnel retries forever and re-registers on a heartbeat 404.
- **Theme** — `mxcli theme remove` with no name now reads the installed theme from the `mxcli:theme` markers instead of removing the built-in default and reporting a silent no-op; switching themes no longer orphans the previous theme's block in `_mxcli-atlas-map.scss`; the topbar language selector, filter-operator popovers, login page, and Data Grid 2 now follow the palette (the language selector went from 1.13:1 to 17.79:1 / 19.47:1 measured contrast).
- **JavaScript action sources were written to the wrong directory** — `CREATE JAVASCRIPT ACTION` wrote to `javascriptsource/<ModuleName>/actions/`, but Mendix reads a **lowercased** module directory. MxBuild found nothing there, generated a stub whose body throws `JavaScript action was not implemented`, and bundled that — so the action parsed, passed `mxcli check`, built cleanly, and threw the moment it ran. Only reproduced on a case-sensitive filesystem, which is why it went unnoticed on macOS and Windows.
- **Widget sync wrote duplicate GUIDs and corrupted the project** — reconciliation now removes stale properties and syncs attributes via `AugmentTemplate` so the widget `Type` is exact.

## [0.16.0] - 2026-07-12

Headline: **Pluggable chart authoring reaches round-trip fidelity**, plus in-place enum-caption editing, named layout placeholders, and a batch of new pre-build `check` heuristics. Charts gain widget-level datasource attributes, the `LINE`/`SCALECOLOR` object-list keywords, and a `DESCRIBE` that reconstructs them as executable MDL; workflows and widget-less pages now describe cleanly; view-entity OQL is validated before build; and several authoring mistakes are caught at `mxcli check` time instead of only by MxBuild.

### Added

- **Chart authoring expanded — object-lists, scale colours, and `DESCRIBE` round-trip** — building on the v0.15.0 chart-series work: widget-level datasource attributes for `PieChart`/`HeatMap` (item 1b), the `LINE` and `SCALECOLOR` object-list keywords, and a `DESCRIBE` that reconstructs pluggable-widget object-lists (series / line / scalecolor) as executable MDL, so an existing chart's syntax can be learned by describing it. Validated `LineChart` / `HeatMap` / `TimeSeries` / `BubbleChart` examples now run in the regular check-mdl suite.
- **`ALTER ENUMERATION … MODIFY VALUE <name> CAPTION '…'`** — edit an enumeration value's caption in place, without dropping and recreating the value.
- **Assign widgets to named layout placeholders** (#532) — a widget can be placed into a layout's named placeholder, not only the default content slot.
- **New authoring-time `check` heuristics** — caught at `mxcli check`, before a build round-trip:
  - `MDL043` — rejects an object/list-typed `declare` where a primitive variable is required.
  - `MDL032` — warns on a reserved OQL word used as a view-entity attribute name.
  - a derived-string view entity that MxBuild rejects with `CE6770` is now caught pre-build (reviving the OQL type-checker it depends on).
- **Workflow authoring skill (Bug 11a)** — added `.claude/skills/mendix/write-workflows.md` (synced to user projects and CLAUDE.md's "read first" list) documenting `CREATE` / `DROP` / `ALTER WORKFLOW`: the activity grammar (user task, multi-user task, decision, parallel split, jump, wait-for-timer/notification, boundary events), header options, and the two first-attempt gotchas (`PARAMETER $var: Entity`, body closes with `END WORKFLOW`). Workflow authoring already worked and built, but no skill documented it, so it read as read-only.
- **`DynamicClasses` documented across the styling surfaces** — the runtime-computed CSS-class property (a sibling of `Class`/`Style`) was wired and skill-documented but missing from every reference enumeration that lists its siblings. Added it to the `mxcli syntax page.styling` topic, `MDL_QUICK_REFERENCE.md` (styling table + ALTER PAGE `SET` properties), and the docs-site pages (`quick-reference`, `create-page`, `widget-types`, `alter-page`). Also demonstrated end-to-end in the `12-styling` doctype example (create-time, bulk `UPDATE WIDGETS`, and `ALTER PAGE ... SET DynamicClasses ON <container>`).

### Fixed

- **`DESCRIBE PAGE`/`DESCRIBE SNIPPET` of a widget-less page/snippet now re-parses (#626)** — an empty page described as `create page … ( … )` with no `{ }` body block, which the CREATE PAGE grammar rejects, so the DESCRIBE output failed to round-trip through `mxcli check`. Empty pages and snippets now emit an empty `{ }` body. Added an integration test (`TestRoundtripPage_DescribeReparses`) that actually re-parses DESCRIBE output (not just substring assertions); a full describe→check pass over the 57 `03-page-examples` pages is clean.
- **`DESCRIBE WORKFLOW` round-trips all activities as executable MDL (Bug 11b)** — on the default `modelsdk` engine, jump-to, wait-for-timer, and wait-for-notification activities (and the implicit start/end) decoded to `GenericWorkflowActivity` and rendered as non-executable `-- [Workflows$…]` comments, so `describe → exec` dropped them and their syntax couldn't be learned by describing an existing workflow. The reader now reconstructs these as typed activities (reading the jump target and timer delay from raw BSON), matching the legacy engine and the describe formatter, which already handled them. Round-trip verified: `describe → drop → exec → docker check` with zero workflow errors.
- **Containers now appear in the widget catalog** — `show widgets`, `update widgets`, and `CATALOG.widgets` queries dropped every `container` (`Forms$DivContainer` / `Pages$DivContainer`), so a `WHERE widgettype LIKE '%Container%'` filter silently matched nothing and containers couldn't be bulk-styled — even though they carry `Class` / `Style` / `DynamicClasses` / `DesignProperties` and can be clickable. The catalog now indexes user-authored containers and skips only the synthetic transparent `conditionalVisibilityWidget*` layout wrapper (matching how `DESCRIBE PAGE` already unwraps it). Other container types (LayoutGrid, TabContainer, …) were already indexed.
- **Widget action microflow parameter mappings persist (Bug 1)** — a widget action's microflow parameter mappings were dropped on write; they are now serialized and round-tripped.
- **Nanoflow client actions on widgets (Bug 2)** — the `modelsdk` engine now writes nanoflow client actions on widgets; previously only microflow actions serialized.
- **HeatMap scale colour persists via the `ColorValue` alias (Bug 10a class)** — a HeatMap's scale colour was not written; it now serializes through the `ColorValue` object-list alias.
- **`ALTER ENTITY … MODIFY ATTRIBUTE` constraints and `MOVE ENUMERATION` folder reads (Bug 12)** — the read paths for attribute constraints and an enumeration's folder are corrected so both operations round-trip.
- **View-entity OQL is read from the source document on the `modelsdk` engine (Mendix 11.x)** — so `DESCRIBE` and validation see the actual query text.
- **ELSIF arms preserved by lowering to nested `IfStmt` (#746)** — microflow expression `ELSIF` branches were dropped on round-trip; they are now retained.

## [0.15.0] - 2026-07-10

Headline: **A page-authoring fidelity wave on the `modelsdk` engine**, plus MCP pluggable-widget authoring (Phases 1–2), Playwright warm-session reuse, and new `check` heuristics for widget properties. A batch of numbered page bugs (DataView/DataGrid2/widget serialization and round-trip) are fixed, microflow round-trip gaps (#723) are closed, and several new authoring-time checks catch widget mistakes before they reach MxBuild.

### Added

- **`SHOW` / `DESCRIBE ICON COLLECTION`** — discover the icons in an icon collection (`CustomIcons$CustomIconCollection`, e.g. `Atlas_Core.Atlas_Filled`) so you can pick a valid name for a widget's `icon:` (icons have non-obvious names — it's `add`, not `plus`). `SHOW ICON COLLECTIONS` lists the collections (name, prefix, export level, icon count); `DESCRIBE ICON COLLECTION Module.Name` lists every icon with its ready-to-use `Module.Collection.IconName` reference. Read-only (icon collections ship with the theme/Atlas). Works on both engines. As a bonus, the `COLLECTION` keyword now accepts the plural, so `SHOW IMAGE COLLECTIONS` (and `ICON COLLECTIONS`) parse alongside the singular form.
- **Button icon (`icon:`) on action/link buttons** (#602) — `actionbutton`/`linkbutton` now accept `icon: 'Module.IconCollection.IconName'`, serialized as a `Forms$IconCollectionIcon` (the modern Atlas icon), so a link-mode button can carry a dedicated icon. Verified against a Studio-Pro-authored button; `describe` round-trips the `Icon:` clause and `mx check` is clean (an unknown icon name is correctly rejected with CE1613). Previously `icon:` was silently dropped (flagged by MDL-WIDGET07). Link render mode itself (`linkbutton`) already shipped.
- **REPL filesystem path completion for `EXECUTE SCRIPT`** — pressing Tab while typing the path argument of `execute script '<path>'` now completes against the filesystem (e.g. `execute script "mdl-`⇥ → `mdl-examples/`). Directories complete with a trailing `/` so you can keep tabbing to descend, hidden entries are offered only when the fragment starts with `.`, and completion works whether or not a project is connected (you often run a script to connect in the first place). Both single- and double-quoted paths are handled; keyword/object-name completion is unaffected.
- **Author Mendix Charts series via MDL** (Bug 9a) — SERIES chart types can now bind their data via MDL; object-list datasource sub-properties work, and the multi-widget docs are corrected.
- **DataGrid2 column binding to an associated attribute** (Bug 7) — a DataGrid2 column can now bind to an attribute reached over an association, not just a direct attribute of the grid's entity.
- **MCP pluggable-widget authoring** — the experimental MCP/PED backend can now author pluggable widgets against a running Studio Pro: Phase 1 accepts any registry-resolved pluggable widget via the shared `.def.json` registry; Phase 2 implements the expression, text-template, and action widget ops.
- **Playwright warm-session reuse and lifecycle control** — verify runs reuse a warm browser session across invocations, with new `open` / `status` / `close` session-lifecycle subcommands.
- **New authoring-time `check` heuristics for widgets** — `MDL-WIDGET07` warns on unrecognized built-in widget properties; `MDL-WIDGET08` flags invalid enum values on widget object-list sub-properties and rejects an association datasource on a `DataView`; `MDL-WIDGET09` rejects an invalid `DataView` database source.

### Changed

- **Go toolchain 1.26.4 → 1.26.5** for GO-2026-5856.

### Fixed

- **Page / widget serialization on the `modelsdk` engine** — a wave of numbered page bugs:
  - DataGrid2 column properties are now ordered by the widget template, fixing CE0463 on the modelsdk engine (Bug 6).
  - An association attribute is resolved correctly from a subclass context (Bug 3).
  - `DataView` "data from context over association" is supported, and an invalid association/database `DataView` source is now refused at both `check` and `exec` (Bug 5, `MDL-WIDGET09`).
  - Widget datasource `sort by … desc` is persisted and round-tripped by `DESCRIBE` (Bug 8).
  - `DynamicCellClass` is persisted and `dynamicclasses` is lowercased (Bug 10).
  - `DynamicText` contentparam over an association is persisted.
  - The `Visible` string/boolean conditional-visibility form and widget `DynamicClasses` expressions are persisted.
- **MDL parsing** — identifier quotes are stripped in expression contexts and in inline-bracket XPath (datasource `WHERE`).
- **OQL select-clause parser** was case-broken and is now case-insensitive (Bug 9b).
- **Microflow round-trip on the `modelsdk` engine** (#723) — execution flags (A1), flow-object box size (A2), and rule-based decisions (`IsRule`, A4) now read back correctly.
- **`docker` widget update** — the absolute `.mpr` path is passed to `mx update-widgets`, fixing a crash that left CE0463 unresolved.
- **MCP verify-on-timeout** — Studio Pro's `-32000` false failures are re-verified instead of reported as failures.
- **Executor** keeps the connection when a script reconnects internally.
- **Dependency bump** — `golang.org/x/crypto` → v0.52.0.

## [0.14.0] - 2026-07-06

Headline: **Mendix 11.12 support and `modelsdk`-engine parity.** This release makes mxcli's output load and build cleanly on Mendix 11.12 (strict `$ID`-first BSON ordering, and `CloseFormAction` / conditional-settings / number-filter serialization fixes), closes dozens of `DESCRIBE` and read-fidelity gaps on the now-default `modelsdk` engine, hardens OData import and publishing, and substantially expands the Starlark lint surface and the experimental MCP/PED backend. It also adds `CREATE` / `DROP JAVASCRIPT ACTION`, clickable containers, chart widgets, page-level CSS, and `linkbutton`.

### Added

- **`CREATE` / `DROP JAVASCRIPT ACTION`** — author JavaScript actions in MDL, mirroring `CREATE JAVA ACTION`, on both engines:

  ```sql
  create [or modify] javascript action Mod.Name(P: Type not null)
    returns Type
    [exposed as 'caption' in 'category']
    [platform Web|Native|Hybrid|All]   -- Web default
  as $$ <javascript> $$;
  drop javascript action Mod.Name;
  ```

  Each create writes the `JavaScriptActions$JavaScriptAction` unit plus `javascriptsource/<Module>/actions/<Name>.js` (BEGIN/END USER CODE markers), and the action is callable from nanoflows via `CALL JAVASCRIPT ACTION`. A JS action's BSON is structurally identical to a Java action but with `JavaScriptActions$` `$Type` names and a `Platform` field; the modelsdk engine encodes through the working Java gen path then rewrites the `$Type`s and injects `Platform`, so there's no generated-code divergence. `DESCRIBE` emits re-executable MDL. Verified end-to-end under both engines (`mx check` = 0). Ships with a synced user skill and a docs-site reference page.
- **Clickable `CONTAINER` — `OnClick:` / `Action:`** (#603) — a container's on-click action can now be set (`container c (OnClick: microflow Mod.Foo) { … }`, or the `Action:` alias), wired through both engines with a clean `DESCRIBE` roundtrip. Previously `OnClick:`/`Click:` errored in the parser and the one form that parsed (`Action:`) was silently dropped — the container always serialized `Forms$NoAction`. Non-clickable containers are unchanged (still `NoAction`).
- **`linkbutton` widget** — the documented `linkbutton (caption, action)` now builds (previously `exec` failed with "unsupported widget type: linkbutton" even though the grammar token and a `pages.LinkButton` stub existed). It serializes as a `Forms$ActionButton` with `RenderType: "Link"` — the modern toolbox "link button", not the legacy address-based `Forms$LinkButton` — so it reuses the proven action-button BSON with no CE0463 risk. Works in both `CREATE PAGE` and `ALTER PAGE INSERT`, and `DESCRIBE` round-trips the `linkbutton` keyword.
- **Chart widgets from bundled `.mpk` packages** (#679) — `ParseMPK` only read `WidgetFiles[0]`, so a package bundling several widgets registered only its first, and `Charts.mpk` (10 widgets, `AreaChart` first) left `BarChart`/`ColumnChart`/`PieChart`/`LineChart`/etc. invisible — `exec` failed "no definition for widget …" even after `widget init`. Every widget in a bundled package is now registered (`ParseMPKAll` / `ParseMPKWidget`); `widget extract`, `widget init`, and `refresh catalog` emit a def per bundled widget (`WidgetDefGeneratorVersion` 4→5, so existing projects regenerate). The SERIES chart types (Bar/Column/Area) are now authorable; a new `34-chart-widget-examples.mdl` documents them and the still-open gaps (per-series datasource binding, `LINE`/`SCALECOLOR` keywords).
- **Page-level CSS `Class` and `Style`** (#714) — `CREATE PAGE (Class: '…', Style: '…')` and `ALTER PAGE … SET (Class = …)` now write the page's `Forms$Appearance` class/style (previously rejected — `Class`/`Style` are reserved lexer tokens the generic header branch never matched). Wired through both engines with a `DESCRIBE` roundtrip.
- **Native `ListView` database datasource** — `listview (datasource: database from X where … sort by …)` now works on the default `modelsdk` engine (`Forms$ListViewXPathSource` with sort bar and search); previously only microflow sources were serialized, and a database source errored with "rerun with MXCLI_ENGINE=legacy". DataView/DataGrid2/Gallery database sources already worked.
- **`check --references` flags `System.owner` XPath refs on entities that don't store owner** (#641) — a retrieve/datasource constraint referencing `System.owner` / `changedBy` / `changedDate` / `createdDate` on an entity that doesn't store that member (which Studio Pro rejects with CE0161) is now caught at `mxcli check` time, with the exact `alter entity X add attribute owner: autoowner` fix hint. Fires against existing project entities (associations traversed via `/` are excluded, so a related entity's owner isn't false-flagged).
- **`check` heuristics for constructs MxBuild rejects** — several checks now catch at `mxcli check` time what previously only surfaced as a failed build round-trip:
  - `MDL041` — integer `div` (which yields Decimal) assigned to an `Integer`/`Long` target (CE0117). A rounding-function result (`round(...)`, `floor(...)`) into an Integer is deliberately not flagged. Wires the expression type-checker (`mdl/exprcheck`) into syntax-only `check` for the first time.
  - `MDL042` — `@caption` applied to a loop, which Mendix silently drops (for-loops have no caption); points to `@annotation`.
  - `MDL-WF01`/`WF02`/`WF03` and `MDL-BUTTON01` — workflow user-task-without-a-page (CE1834), single-outcome user task with a nested flow (CE1876), a decision/microflow outcome that isn't a valid enumeration identifier, and a DataGrid control-bar button passing the unbound `$currentObject` (CE1571). Workflows previously received no semantic validation at all. (Wave 1 of the MxBuild-gap-heuristics proposal.)
  - `MPR010` — an edit/new-form `DataView` containing input widgets but not wrapped in a layout grid (labels/inputs render misaligned). Available as both a `mxcli lint` rule and an authoring-time `mxcli check` warning; the built-in rule count is now 15.
- **Starlark lint expansion** — new query functions and flags for custom rules:
  - `constants()`, `scheduled_events()`, and `xpath_expressions()` query functions, plus `parse_xpath(expr)` to walk a parsed XPath/expression AST — enabling performance/security rules that inspect the parsed tree rather than raw strings.
  - `mxcli lint --rules/-r <IDs>` to isolate specific rules and `--modules/-m <names>` to scope to specific modules during rule development.
  - `mxcli lint` now honours `lint-config.yaml` (exclude modules, enabled flags, severity overrides, per-rule `options`) — previously only the REPL/script path did; adds a `get_option(key, default)` builtin.
  - Lint rules that need catalog depth self-declare it (#721): a rule using `refs_to`/`cycles`/… auto-upgrades the build to `full`/`communities` mode instead of silently returning empty results.
- **MCP/PED backend authoring surface** — the experimental backend for authoring against a running Studio Pro gained:
  - `GRANT` entity access rules over PED (#704) — access rules live on the domain-model document, so they're reachable even though the security documents are sealed (add/modify-only; a `GRANT` for a role that already has a rule, and `REVOKE`, are rejected honestly rather than faked).
  - `CREATE OR REPLACE NAVIGATION` web-profile authoring over PED (#699) — home page, login page, not-found page, and the menu tree.
  - Attribute default values over PED on Studio Pro 11.12+ (`ped_update_document` path-op on the `StoredValue`; project-version gated, since the PED server reports 1.0.0 on both 11.11 and 11.12).
  - Page-level `ALTER PAGE … SET (Title = …)` over MCP.
- **`docker init` detects a stale `docker-compose.yml`** — the compose template now carries a version stamp, and `docker up`/`down`/`logs`/`status`/`shell` warn when a project's compose predates the current template (so template fixes like the OQL live-preview flags reach projects that ran `init` before the fix), pointing at `mxcli docker init --force`.

### Changed

- **Nightly integration tests run against Mendix 11.12** (was 11.11), and the doctype `mx check` gate now runs every example through **both** engines (`modelsdk` default + `legacy`) so `modelsdk`-only serialization regressions are caught (#691). Marketplace-module test dependencies are now version-selected per script (e.g. External Database Connector 6.2.3 for ≤11.11, a slimmed 6.3.0 for 11.12+).
- **Dependency bump** — `actions/cache` 5 → 6.

### Fixed

#### Mendix 11.12 load & build compatibility

- **`$ID` must be the first property of every storage object** — Mendix 11.12's stricter streaming reader rejects a unit whose storage object doesn't lead with `$ID`, failing to load projects that ≤11.11 tolerated (`Expected '$ID' as the first property of a storage object, but got '$Type'/'Name'/…`). Fixed across three passes: `bson.M` (Go-map) storage objects converted to ordered `bson.D` (they only landed `$ID`-first by luck of map iteration); the already-ordered `bson.D` entity/attribute/access-rule/security serializers reordered to `$ID`-first; and every map-passthrough marshal site (settings raw-parts, ref-marking, domain-model `raw`) normalized via a non-sorting `HoistStorageID` that lifts `$ID`/`$Type` to the front without reordering the delicate widget maps a full sort would corrupt.
- **`CloseFormAction` page count wrote the wrong field name** — a CLOSE PAGE activity failed `mx check` on 11.12 with CE0117 (legacy engine); the count was written under `NumberOfPagesToClose` but the metamodel storage name is `NumberOfPages`. Now written correctly (the reader accepts both).
- **Conditional visibility/editability settings serialized `Attribute` as `null`** — a widget with a conditional setting failed to *load* on 11.12 (`StorageLoadException: '' is not a valid AttributeIdentifier`); Studio Pro writes the empty string `""` there. Now emits `""` not `null`.
- **`numberfilter` template had markerless empty arrays** — a `NUMBERFILTER` in a DataGrid column passed `check` but corrupted the `.mpr` on 11.12 (`WidgetProperty … does not contain a constructor with a parameter of type … WidgetValue`); its placeholder `Forms$ClientTemplate` blocks were authored with bare `[]` arrays missing the Mendix list marker int. Fixed in both engine copies of the template, with a guard test (`TestTemplates_NoMarkerlessEmptyArrays`) that rejects any markerless array in an embedded template.

#### `modelsdk` engine read & serialization parity

- **`DESCRIBE MICROFLOW` fidelity on the default engine** — a broad set of activities that rendered as `-- Empty action` or lost detail now round-trip, matching (and in a few cases exceeding) the legacy engine: loop `break`/`continue`; `EXPORT TO MAPPING` / `IMPORT FROM MAPPING` and single-object mapping cardinality; the retrieve `sort by` clause (read); inheritance-split variable and `@caption`; `download file`; legacy SOAP `call web service`; REST `body mapping from $var`; rule-based exclusive splits (were rendered as `if true`); the `NewCaseValue` branch case (a missing case dropped the entire then-body); `grant execute` on microflow/nanoflow allowed roles; and an unrecognized split case value that dropped a then-branch is now recovered by elimination.
- **Retrieve `sort by` columns dropped on write** (#727) — a database retrieve's sort clause was serialized to an empty list on the modelsdk engine (stored under the wrong key), so `DESCRIBE` emitted no sort. Now written under `Sortings`, mirroring the Sort list-operation.
- **`@annotation` on a microflow activity was silently dropped** on the modelsdk engine (no `Microflows$Annotation` case, and the linking `AnnotationFlow` was never emitted). Now serialized so notes round-trip.
- **External (OData) entities & associations serialized as plain persisted entities** (#718) — a regression from the default-engine switch: the modelsdk write adapters dropped external-entity serialization, so `CREATE EXTERNAL ENTITIES` produced plain entities and NPEs without their `from Service` link. The legacy serializer's logic is ported into the codec adapters (`Rest$OData*` gen types).
- **External OData string attributes lost `Length=0` (unlimited)** — the codec encoder only emits dirty properties, so an unlimited-length attribute omitted `Length` and Studio Pro applied its own default of 200 (20× CE6621). `SetLength` is now always called.
- **Caller-provided entity `$ID` was overwritten in `CreateEntity`** — the modelsdk `entityToGen` generated a fresh `$ID`, so an association wired to `entity.ID` (e.g. the primitive-collection NPE association in an external-entity import) dangled and the project failed to *open* (`KeyNotFoundException`). The caller's ID is now honoured.
- **Lossless OData reads + `call external action` write** — `ListConsumedODataServices` didn't read `HttpConfiguration` (a re-modify dropped the `ServiceUrl`, CE5111); `ListPublishedODataServices` surfaced only entity-set counts (an ALTER stripped the entity tree → NullReference on load); and a microflow `call external action` serialized with a nil action (CE0008). All fixed.
- **Page allowed roles not read** (#722) — on the modelsdk engine `SHOW ACCESS ON PAGE` and the `SHOW SECURITY MATRIX` page section reported "no module roles" for a role-restricted page (a security-audit hazard); `pageFromGen` never populated `Page.AllowedRoles`. This also caused `mxcli lint` to false-fire CE0557/MPR007 ("home page has no allowed roles") after a `GRANT VIEW ON PAGE` that had in fact persisted correctly (#696).
- **Cross-module associations invisible to `LIST`/`DESCRIBE`** — associations to an entity in another module live in the gen `CrossAssociations` collection, which `domainModelFromGen` didn't read; `DESCRIBE MODULE … WITH ALL` also skipped them on both engines. Now surfaced.
- **`System` module associations omitted** — the virtual System domain model built platform entities/attributes but not the platform associations (`UserRoles`, `Session_User`, `Workflow_*`, …), so they were missing from `SHOW`/`LIST ASSOCIATIONS` and `DESCRIBE MODULE System` on the modelsdk engine.

#### OData import & publishing

- **Published OData service missing `EdmType` (CE5016) and `IsMany` (CE5022)** — mxcli never wrote the exposed attribute's EDM type nor the exposed association end's multiplicity, so `mx check` flagged every exposed attribute and association on the modelsdk engine. Both are now derived and serialized (the legacy engine omitted them too, but its field order let the checker recompute — so it wasn't a canonical reference).
- **Re-importing external entities duplicated navigation associations** — running an import twice suffixed every nav association (`Friends`/`Friends2`/`Friends3`…); a triple TripPin import inflated 8 → 25. Existing associations are now recognized and skipped.
- **Consumed-OData Headers microflow written to the wrong slot** (#728, CE6808) — on Mendix 11.10+ the configuration and headers microflows are distinct fields (`ConfigurationEntityMicroflow` vs `HeaderListMicroflow`); mapping both to the configuration slot made Studio Pro demand a `ConsumedODataConfiguration` return type. They're now tracked separately, and the `configurationMicroflow` storage key is version-gated (introduced 10.12, renamed 11.10) so a setting no longer no-ops on 11.10+.
- **Unannotated OData entity capabilities** — an entity set without `Capabilities` annotations now defaults to read-only (Creatable/Updatable/Deletable = false), matching how Mendix reads the metadata; a `true` default disagreed with services like TripPin RESTier (26× CE6630). Only an explicit annotation turns a capability on.
- **OData primitive-collection NPEs on Mendix <11.0** — `Rest$ODataMappedPrimitiveCollectionValue` and friends are an 11.0 type that doesn't exist in the 10.x type cache, so writing them aborted the whole project load (`TypeCacheUnknownTypeException`). Pre-11 imports now omit primitive-collection properties (as Studio Pro does), keeping the rest of the external-entity import intact.

#### Microflows, retrieves & describe

- **Memory vs database retrieves conflated** (#726) — a retrieve-by-association (in-memory) and a database retrieve with an equivalent reverse-association XPath were indistinguishable, and re-running a script silently converted the memory retrieve into a database one. `from $var/Assoc` is now always an `AssociationRetrieveSource` and a database retrieve always renders as `from Entity where …` — except the one genuine case (a reverse `Reference` with owner `both` consumed as a list, which Mendix resolves to a single object) which stays a database source to avoid CE0100.
- **Quoted association/attribute names in a microflow `SET`/`Change` corrupted the `.mpr`** — `SET $x/Module."Assoc" = $y` (which the "always quote identifiers" guidance encourages) passed `check` and `exec` but carried the quotes into the member identifier, so Studio Pro failed to load (`StorageLoadException: … is not a valid AttributeIdentifier`). The target is now normalized (quotes stripped) at parse time, with a defense-in-depth guard that errors loudly on any future quote leak.
- **`DESCRIBE MICROFLOW` timed out on high-complexity flows** (#710) — the duplicate-output-variable check enumerated every execution path (O(2^branches)), so a McCabe-44 flow crossed the 300s timeout. Replaced with an O(V·E) reachability analysis giving the identical answer (a 120-diamond flow now completes instantly).
- **`describe` showed a non-default-language placeholder** (#702) — text extraction returned the first `Texts$Text` entry (often a placeholder in another language) and the page title hardcoded `en_US`, so a `describe`→`create` round-trip could overwrite the real caption. Translations are now selected by project default language → `en_US` → first non-empty.

#### Pages, MOVE, lint & MCP

- **Pop-up page `PopupWidth`/`PopupHeight` = 0 (auto-size) rejected** (#713) — `0` is Studio Pro's own default for an auto-sized pop-up, but mxcli rejected it as "must be positive" and both writers coerced `≤0` → 600, so an auto-size pop-up couldn't be authored. Only a negative value is now rejected.
- **`MOVE ENTITY` left a stale reference in inline view-entity OQL** — Mendix <11 stores a view entity's OQL both on the source document and inline on the entity; the modelsdk engine rewrote only the former, so `mx check` on 10.x aborted with CE0174. The inline copy is now rewritten too. Related: `MOVE ENTITY`/`MOVE ENUMERATION` no longer print spurious "could not update … no such file" warnings from trying to disk-load the virtual System domain model.
- **`MPR001` rejected the Mendix `ENUM_` naming prefix** (#715) — the enumeration naming rule rejected `ENUM_ShippingStatus` (which the Mendix best-practice `CONV004` rule *requires*) and its suggestion mangled the name; the two rules could not both be satisfied. `MPR001` now accepts the optional `ENUM_` prefix and preserves it in suggestions.
- **Chained XPath predicates mis-stripped in lint** — `[a = 1][b = 2]` was stripped to `a = 1][b = 2`; the bracket-matching now walks depth.
- **Java action `Enumeration` parameter/return types serialized as entity references** (#680) — `Enumeration(Module.Enum)` / `ENUM Module.Enum` on a Java or JavaScript action passed `check` but MxBuild rejected the model ("The selected entity … no longer exists"). The explicit-enum syntax now emits `CodeActions$EnumerationType`; a bare `Module.Name` still resolves as an entity (indistinguishable at parse time).
- **Enum `Value = 'Caption'` gave a cryptic parse error** — MDL enum values are `'Value ''Caption'''` with no `=`; the ANTLR error read like a quoting problem. A targeted hint now points at the `=`.
- **Duplicate identical widget-reference diagnostics** — a page referencing the same target from more than one widget (e.g. New + per-row Edit buttons to the same edit page) now yields a single diagnostic.
- **`mxcli oql` returned 0 rows against a docker-deployed runtime** — the OQL preview servlet only mounts when `mendix.running.locally.by.studiopro=true` is set, which a deployed `bin/start` doesn't; the compose command now sets it (and forces sane runtime ports), and the OQL client surfaces the "Action not found" preview error instead of swallowing it as an empty result.
- **MCP: context data view missing its entity (CE0488)** — a data view bound to a page parameter authored over MCP wrote only the source variable, not `entityRef`; both are now written. MCP page authoring was also migrated from the removed `pg_write_page` to `pg_patch_page` for Studio Pro 11.12 (#697).
- **`GetRawUnit` on v1 MPR files** (Mendix < 10.18) (#705) — the UUID string was passed directly to SQLite instead of being converted to a GUID blob, so every lookup failed with "no rows in result set".

#### Report & lint performance

- **`mxcli report --format json` hung for tens of minutes on large projects** (#720) — six lint rules called `GetMicroflow(id)` per microflow, and on the modelsdk backend each call re-decoded *every* microflow unit (O(N²), millions of parses on a 3259-microflow project). A per-run `FullMicroflow` cache loads all microflows once; report dropped from >40 min to ~5 s.

## [0.13.0] - 2026-06-20

Headline: **the roundtrip codec engine is now the default.** Reads and writes route through the new `modelsdk` codec engine — a Go-native, roundtrip-safe metamodel codec spanning 53 domains — replacing the legacy `sdk/mpr` write path. Legacy remains available as an explicit `--engine legacy` (or `MXCLI_ENGINE=legacy`) fallback for the few constructs the codec can't yet reproduce (e.g. SOAP), and refuses an op rather than dropping data where it can't. This release also lands an experimental **MCP/PED backend** for authoring against a running Studio Pro.

> **A big thank-you to [engalar](https://github.com/engalar).** The roundtrip codec engine and the expression type-checker that anchor this release are built on his contributions — his `modelsdk` codec work (the 53-domain, roundtrip-safe metamodel implementation) and `exprcheck` port were cherry-picked and adapted here. Much of what makes v0.13.0 possible is his. Thank you!

### Added

- **Experimental MCP/PED backend (`mxcli mcp`)** — author Mendix models against a *running* Studio Pro over the Model Edit Protocol (PED/MCP) transport, instead of writing the `.mpr` on disk. `mxcli mcp capabilities` reports what the connected Studio Pro version supports (a version-keyed capability registry), and CREATE/ALTER ops are gated on that model — unsupported constructs are refused with an actionable message rather than silently dropped. Covers entity create/update with NOT NULL / UNIQUE validation rules, ALTER ENTITY ADD ATTRIBUTE, ALTER STYLING design properties, page authoring (typed params, edit-button actions, design properties), folder placement, and business-event/workflow reads. Honoured in both `exec` and the interactive REPL (`--mcp`). Verified against Studio Pro 11.11.

- **Graph community detection & centrality (`refresh catalog communities`)** — a pure-Go (no CGO, no deps) graph engine over the refs graph: Leiden community detection, Tarjan cycles, topological layering, PageRank, and betweenness. `refresh catalog communities [resolution n]` computes them (in the full-refresh transaction) into new catalog tables/views — `communities` + `community_summary`, `graph_cycles`, `graph_layers`, `graph_centrality`, `graph_integration_surface` (cross-community edges → OData/REST/event mechanisms), `graph_module_dependencies` — and adds PageRank/betweenness columns to `graph_god_nodes`. Surfaced via `SHOW COMMUNITIES` / `SHOW COMMUNITY [MEMBERS] OF Module.Asset`, and exposed to Starlark lint rules (`community_of`, `layer_of`, `cycles`, `module_dependencies`, `centrality`, `god_nodes`, `integration_surface`, `refs_from`) so teams validate their own architecture guidelines. The native Leiden matches the `leidenalg` reference exactly (105 communities on Evora). Targets two refactoring journeys: spaghetti → layered/modular (cycles + layer sequence numbers) and monolith → multi-app (community cut → integration-contract list).
- **`mxcli graph-report` — architecture map from the dependency graph** — renders six analyses over new `CATALOG.graph_*` views: god nodes (degree centrality), cross-module coupling ("surprise edges"), module cohesion (intra/inter ratio), dead documents (no inbound edge), the reference-kind distribution, and entity hotspots (used by the most flows). Framework/marketplace modules are excluded by default (`--include-framework` to keep them); `--top N`, `--format markdown|json`, `-o file`. Each section is a thin `SELECT` over a `graph_*` view, so it's reproducible directly (`select * from CATALOG.graph_god_nodes`). Built on the now-substantially-complete `refs` graph; requires `refresh catalog full` (the command runs it). Also made `CATALOG.<name>` query translation generic (regex strip) so new catalog views work without a per-name allowlist.
- **Marketplace download & install** — the content API now returns a per-version `downloadUrl`, so the previously-parked install path is unblocked. `mxcli marketplace download <id> [--version X] [-o file]` fetches a content version's `.mpk` (two-step: MxToken-authed `303` on `marketplace.mendix.com` → public CDN, no token sent to the CDN). `mxcli marketplace install <id> -p app.mpr` is type-aware: widgets are copied into `widgets/`, new modules are imported via `mx module-import`, other types are downloaded with import instructions. Module **updates** are intentionally reported-not-applied — re-importing an existing module would discard local edits and change persistent-entity IDs (data loss); that path is left to Studio Pro pending an ID-preserving merge
- **Marketplace search caching** — the first `mxcli marketplace search` fetches the full catalog listing once and caches it under `~/.mxcli/marketplace-catalog-<profile>.json` (24h TTL, mode 0600); subsequent searches (any keyword) are served from the cache instantly. `--refresh` bypasses the cache and re-fetches. An interactive progress line ("Searching marketplace… N items scanned") shows during a fresh scan
- **`describe` auto-detects the document type** — the type is now optional for a qualified name: `mxcli describe MyModule.Customer` resolves the type itself (entity, microflow, page, snippet, enumeration, constant, java action, nanoflow, workflow, association incl. cross-module, …). Resolution prefers the catalog cache (O(1) lookup, no overhead vs. the explicit form) and falls back to a live project scan when the catalog is absent. An ambiguous name (e.g. an entity and a microflow sharing a name) is reported with its candidates. The explicit `describe <type> <name>` form is unchanged, and is still required for the forms that have no single qualified name (module, settings, navigation, module role)
- **Bare `describe Module.Name` works as MDL, not just as a CLI flag** — the auto-detect form is now part of the MDL grammar, so it parses and runs everywhere MDL does: the REPL, `exec` scripts, `check`, and the LSP (previously `describe Sales.Order` in the REPL was a parse error and only `mxcli describe Sales.Order` worked). The bare form resolves the type from the project's catalog `objects` index at execution time (built on demand, fresh — no staleness concern); all typed `describe <type> …` forms still take precedence, and an ambiguous or unknown name returns an actionable error
- **Pop-up page geometry** — `CREATE PAGE` and `ALTER PAGE` can now set a pop-up page's `width`, `height`, and `resizable` in the page header (#661). `DESCRIBE PAGE` round-trips them.
- **Compound (nested) design properties** — design properties on pages and snippets that nest (a group containing sub-properties) are now written and round-tripped by `DESCRIBE PAGE`/`DESCRIBE STYLING`, on both the codec engine and over MCP (#668)
- **Quoted identifiers in member lists and attribute refs** — names that collide with MDL reserved words can now be quoted in member lists and attribute references (#675), extending the reserved-word-quoting support to more positions (`DESCRIBE` emitters now quote reserved-word names in the remaining strict-identifier spots, #619)

### Changed

- **The codec engine (`modelsdk`) is the default; `sdk/mpr` is the explicit fallback** — all reads and writes now route through the roundtrip codec engine by default. The legacy path is reachable via `--engine legacy` or `MXCLI_ENGINE=legacy` for the constructs the codec can't yet reproduce (notably SOAP); where the codec path can't reproduce a construct it refuses the op rather than dropping data. This is the culmination of the Issue 7 parity effort that brought every document type — domain models, microflows, pages, workflows, security, REST/OData, agent-editor docs, settings, and more — to `mx check` parity on the codec path.

- **Catalog `objects` index includes associations** — the unified `objects` view now unions the `associations` table (`ObjectType = ASSOCIATION`), so it is a complete index for the cataloged document types and consumers no longer need a separate associations query. Catalog schema bumped to v3; cached `.mxcli/catalog.db` files rebuild automatically on the next `refresh catalog`.
- **Catalog indexes image collections, JavaScript actions, and data transformers** — these document types had no catalog table at all; they are now built (via the raw-unit surface, so no `CatalogReader`/backend change) into their own tables and unioned into `objects` (`IMAGE_COLLECTION`, `JAVASCRIPT_ACTION`, `DATA_TRANSFORMER`). `describe` auto-detect resolves image collections and data transformers by bare name. Catalog schema bumped to v4.
- **Catalog indexes agent-editor documents** — agents, AI models, knowledge bases, and consumed MCP services (one shared `CustomBlobDocuments$CustomBlobDocument` BSON wrapper, distinguished by `CustomDocumentType`) are now cataloged into their own tables and unioned into `objects` (`AGENT`, `AI_MODEL`, `KNOWLEDGE_BASE`, `CONSUMED_MCP_SERVICE`). The document name turned out to be a top-level wrapper field (not buried in the inner JSON blob), so this reads through the raw-unit surface with no `CatalogReader`/backend change, and `describe` auto-detect resolves all four by bare name. Catalog schema bumped to v5; this completes the `objects` index for the document types tracked in #658. (Verified against `test3-app`: 8 agent-editor docs across all four types.)

### Fixed

- **Page authoring fidelity** — several page constructs that were silently dropped or mis-stored are fixed: `DYNAMICTEXT` Attribute bindings are no longer dropped (#650); `ALTER PAGE` can set conditional `Visible`/`Editable` expressions without tripping CE0117 (#627); a ComboBox datasource property that was silently dropped is now caught at check time (#643); a quoted `where '<xpath>'` constraint is no longer mis-stored as CE0161 (#642); and gallery `DesktopColumns` + `class` are honoured on pluggable widgets.
- **`check` catches more page errors** — forward widget→page references (#674) and invalid static widget values (#672, #673) are now flagged at check time instead of surfacing later in Studio Pro.
- **`DROP MODULE` removes Java/JavaScript source directories** — dropping a module now also deletes its orphaned `javasource`/`javascriptsource` directories, on both engines.
- **Docker libSkiaSharp crash auto-handled** — `mxcli docker` auto-preloads the system libfreetype so the bundled `mx` no longer aborts with the `FT_Get_BDF_Property` symbol-lookup error, and reports a clear message when an M2EE call hits a stopped container.
- **`show context` now resolves its relationship sections** — the sections filtered the refs table on `TargetType`/`SourceType` using lowercase literals (`'entity'`, `'microflow'`, `'page'`) while those values are stored uppercase, so in case-sensitive SQLite "Entities Used", "Microflows Using This Entity", "Pages Displaying This Entity", "Related Entities" and the workflow context sections silently rendered empty. Now matched correctly. (`show callers|callees|references|impact` were unaffected and already pick up the expanded refs automatically.)
- **`catalog.refs` captures far more references** — the cross-reference index that powers `show callers|callees|references|impact` was missing whole categories (#663). Now added: nanoflow calls, consumed-REST-operation calls, and association-based retrieves from microflow actions; **nanoflows as reference sources** (previously only microflows were walked); **association references** (each association now links to both its FROM and TO entity — was an explicit `// Skipping for now` TODO); **page→layout references** (the emission was dead code gated behind an always-nil `LayoutCall`); **calculated-by** (entity→microflow for calculated attributes); **change/delete entity references** (resolved via lightweight intra-flow variable tracking); and **page- and snippet-widget references** — `datasource` (page/snippet→entity) and `action` (page/snippet→microflow/nanoflow), extracted from the existing raw-BSON widget walk and projected from `widgets_data` (new `MicroflowRef`/`NanoflowRef` columns; snippet widgets now populate the table too, with `ContainerType=SNIPPET`). On `MxGraphStudioDemo` the earlier slice took `associate` 0→104, `layout` 0→22; on `Evora-FactoryManagement` the full effort took `refs` from ~5.5k to 6,459. Re-run `refresh catalog full` to pick them up.
- **`catalog.activities` labels REST and other actions correctly instead of a generic `MicroflowAction`** — the `ActionType` column came from a hand-maintained type switch that only knew ~17 action types; every other parser-modelled action (REST call, REST operation call, web-service call, nanoflow call, JavaScript-action call, execute-database-query, transform-JSON, XML import/export, show-home-page, delete-object) silently collapsed into `ActionType = 'MicroflowAction'`, so e.g. `select … from CATALOG.activities where ActionType = 'RestCallAction'` returned nothing. The label is now derived from the concrete action type, so it stays correct for every action the parser models (including ones added later), and an action the parser doesn't model yet surfaces its real Mendix storage name rather than a generic bucket. On `MxGraphStudioDemo` the generic bucket dropped from 33 rows to 0, exposing RestCallAction/RestOperationCallAction/JavaScriptActionCallAction/NanoflowCallAction/DeleteObjectAction that were previously hidden. Re-run `refresh catalog full` to pick up the corrected labels.
- **`SHOW CATALOG TABLES` lists every catalog view** — the table list was hand-maintained and had drifted: the newly-cataloged document-type views (image collections, JavaScript actions, data transformers, agents, AI models, knowledge bases, consumed MCP services) and the pre-existing `navigation_profiles` view were all built and queryable but never shown. They are now listed, and a drift-guard test (`TestTables_CoversAllViews`) asserts every catalog VIEW appears in the list, so a future document type can't be silently omitted again.
- **`refresh catalog source` no longer O(N²) on large projects** — it resolved each document by re-reading and re-`bson.Unmarshal`ing *every* unit on *every* describe call, so a big app (#651: ~3.3k microflows, ~33k activities) took ~6 hours. The reader now builds a one-time `$Type + qualified-name → unit` index (decoding only the `Name` field, not the whole document), making `GetRawUnitByName` / `GetRawMicroflowByName` O(1); the shared backend means the index is built once across the parallel describe workers. The source phase also reports incremental progress every 2s instead of going silent for the whole build. GraphViewer's source build (993 microflows) dropped to ~3.5 min with live progress; cloud-portal-scale projects go from hours to minutes
- **Marketplace search now scans the whole catalog** — the Content API has no server-side search and caps `limit` at 100 per page, so `marketplace search` previously only filtered the first 100 items and silently missed matches further in (e.g. External Database Connector `219862`, Mendix Business Events `202649`). It now paginates via `offset`, fetching pages **concurrently** (first page alone so a common early match stays a single request; then bounded-parallel batches), and stops at `--limit` matches or end-of-catalog. Measured ~3m45s → ~44s on a slow link for a deep match; combined with the new cache, repeat searches are instant

## [0.12.0] - 2026-06-04

Headline: **one widget creation path.** The `datagrid`/`gallery`/`combobox`/`image` keywords and the `pluggablewidget '...'` form now build BSON through a single registry-driven engine, fed by widget definitions extracted from each project's installed `.mpk` files (`widget init`; auto-generated/refreshed on `exec`). The Mendix BSON *envelope* still comes from embedded `mendix-11.6` templates — full per-version, project-extracted templates remain tracked under #529.

### Added

- **Cross-version widget-envelope drift gate** — `make check-widget-versions` (script `scripts/check-widget-versions.sh`) runs a widget fixture through `exec` + `mx check` on multiple Mendix versions and fails if the CE0463 set differs between them (v0.12.0 Stream A). It drops each fixture's `create module` targets before exec so leftover/divergent reference-project state doesn't skew the comparison; the 11.10 libSkiaSharp crash is handled automatically via `scripts/mx-check.sh`. Fixture set: `03`, `30`, `31`, `32`. The gate surfaced one real 11.9→11.10 drift (textfilter `attrChoice`, #605, fixed above); after that fix all four fixtures pass with no cross-version drift

### Security

- **Go toolchain pinned to 1.26.4** — resolves two reachable standard-library vulnerabilities flagged by `govulncheck`: GO-2026-5039 (`net/textproto`, unescaped inputs in errors; reached via `mpk.ParseMPK`) and GO-2026-5037 (`crypto/x509`, inefficient candidate hostname parsing). `go.mod` now carries a `toolchain go1.26.4` directive and CI pins `go-version: '1.26.4'`, so every environment builds with the fixed stdlib

### Changed

- **ALTER STYLING design properties** — `ALTER STYLING` now writes design properties on pages and snippets, with correct value-type encoding (Option vs Custom; ToggleButtonGroup uses Option). `DESCRIBE STYLING` round-trips them. (#631)
- **Dependency bumps** — `chroma/v2` 2.24.1 → 2.26.1, `modernc.org/sqlite` 1.50.1 → 1.51.0, `mattn/go-runewidth` 0.0.23 → 0.0.24
- **DataGrid construction unified on the pluggable widget engine** — the `datagrid` MDL keyword now routes through the same registry-driven engine as the `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'` form, so both produce equivalent BSON. The hand-coded keyword-path builder (`datagrid_builder.go` `BuildDataGrid2Widget` + ~30 helpers, ~990 lines) is deleted. Engine gained the column conventions the keyword path applied implicitly: CONTROLBAR→filtersPlaceholder routing, per-column filter-widget routing (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`), object-list item property ordering, `Caption`/`Content` aliases with `CaptionParams`/`ContentParams` resolution, missing-Caption→attribute-name fallback, attribute-less columns default `sortable=false`, content-slot widgets auto-infer `ShowContentAs: customContent`, and the tooltip/exportValue empty-ClientTemplate conventions. (#529 Phase 4)
- **Catalog schema normalized** — every domain table (entities, microflows, pages, …) is now split into a `<name>_data` storage table plus a `<name>` view that joins `snapshots` to expose `ProjectName`, `SnapshotDate`, `SnapshotSource`, `SourceId`, `SourceBranch`, `SourceRevision`. Existing queries (`SELECT * FROM CATALOG.ENTITIES`, ad-hoc filters by `SnapshotSource`, the `objects` UNION view) keep working unchanged. Existing `.mxcli/catalog.db` files rebuild automatically on first open (schema version bumped to 2); cache metadata is cleared so the rebuild fires through `isCacheValid`. (#576)

### Fixed

- **DESCRIBE round-trips for pages and widgets** — `DESCRIBE` now emits re-executable MDL for several cases that previously broke a roundtrip: bare grant member names (#633), `microflow`/`nanoflow` (not `call_`) for widget actions (#634), an always-present java-action body (#637), quoted reserved-keyword DataGrid column names (#638), and widget-action microflow arguments as `Param: value` (#640)
- **Reserved-keyword names via quoting** — page/snippet parameter names (#114) and widget names (#619) that collide with MDL reserved words can now be expressed with quoting instead of being rejected
- **OQL against Mendix 11.11** — `mxcli oql` supports the new `/dev/preview_execute_oql` dev endpoint and surfaces its query errors (which arrive as HTTP 200 with an `{"error": ...}` body) instead of silently succeeding
- Filter widgets (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`) with an explicit `attributes: [...]` list now emit `attrChoice="linked"` instead of `"auto"` (#605). `"auto"` is correct only for a *bare* filter inside a DataGrid column (it binds to the column's attribute); a filter with an explicit attributes list (e.g. inside a Gallery `filter` block) needs `"linked"`. Mendix 11.10+ flags `attrChoice="auto"` alongside a populated attributes list as definition drift (CE0463); Mendix 11.9 tolerated it. This was real 11.9→11.10 envelope drift that the v0.12.0 Stream A gate missed because its fixtures only exercised column-bound filters
- DataGrid column `ColumnWidth: manual` is honoured again — the Stream B engine consolidation dropped the keyword path's `ColumnWidth` → schema `width` mapping, leaving width at its `autoFill` default. A `Size:` value is only valid when `width=manual`, so under autoFill Studio Pro / `mx check` flagged CE0463. Restored as a `width ← ColumnWidth` column alias (caught by the new cross-version gate as `dgDyn` in fixture 31)
- Pluggable widget property conditional visibility (#574) — a TextTemplate property hidden by the widget's `editorConfig.js` under the current configuration now emits `TextTemplate: null` instead of the template's populated default, eliminating CE0463 ("the definition of this widget has changed"). Phase 1 hand-authors rules for VideoPlayer (`videoUrl`/`posterUrl` hidden when `type=expression`) and Timeline (`title`/`description`/`timeIndication` hidden when `customVisualization=true`); rules live in each widget's `.def.json` as a `propertyVisibility[]` block and ride the `generatorVersion` auto-refresh
- `mxcli exec` now generates **missing** widget definitions, not just refreshes stale ones — a project that has `.mpk` widgets installed but was never `widget init`-ed (no `.mxcli/widgets/`) previously failed the first widget build with "unsupported widget type: datagrid". `exec` now extracts the defs from the installed `.mpk` files on demand (matching `refresh catalog`), so it works without a separate `widget init` step
- Stale project-local widget definitions self-heal — `.def.json` files carry a `generatorVersion` stamp, and `mxcli exec` re-extracts any definition generated by an older engine before building widgets. Projects whose `.mxcli/widgets/` was generated before the v0.12.0 widget changes no longer emit CE0463 ("widget definition changed") on the next run without a manual `widget init --force`
- DataGrid filter widgets (`textfilter`/`numberfilter`/`datefilter`/`dropdownfilter`) default `attrChoice` to `auto` instead of `linked`/`custom`, so a filter placed inside a column body binds to the column's attribute automatically rather than failing `mx check` with CE0642 ("Property 'Attribute' is required")

## [0.11.0] - 2026-05-21

### Added

- **Pluggable widget property validation** — `mxcli check` flags unknown widget property keys as `MDL-WIDGET01`; respects MDL builtin property names (e.g. `Label`, `Caption`, `DataSource`) so they aren't reported as typos
- **`mxcli check --post-migration`** — scans for legacy native widgets in pages/snippets and reports `MDL-WIDGET02` with qualified module.document names; version-gated via the legacy-widget catalog
- **LSP widget integration** — completion suggests widget property keys inside `(...)` blocks; hover shows property descriptions extracted from `.mpk`; widget property typos surface as real-time diagnostics
- **Widget definition workflow** — `widget init --force` re-extracts existing `.def.json` files; `widget init` and `refresh catalog` auto-refresh stale definitions; `mxcli init` now runs `widget init` so new projects pick up widget defs automatically
- **Catalog: widget tables** — `widget_definitions` and `widget_definition_properties` queryable via `SELECT ... FROM CATALOG.widget_definitions`
- **`ALTER` for agent-editor documents** — `ALTER AGENT/MODEL/KNOWLEDGEBASE/MCPSERVICE` (#464)
- **Skill docs include MDL keyword routing** — generated widget skill files document object-list and child-slot keywords driven from `.def.json`

### Fixed

- DataGrid2 column `tooltip` / `exportValue` / `dynamicText` TextTemplate now matches Studio Pro's per-column-kind convention (CE0463 on attribute-bound columns, #578)
- DataGrid2 column `CaptionParams` / `ContentParams` / `ShowContentAs` / `Content` roundtrip (#547); `$localVar` references in column captions emit `Forms$PageVariable.LocalVariable`
- Pluggable widget engine wrote `CustomWidgets$AttributeRef` (not a registered Mendix type); now emits `DomainModels$AttributeRef` with the fully-qualified path so `mx update-widgets` no longer fails with `TypeCacheUnknownTypeException` (#64)
- Object-list item TextTemplate slots emit `null` when unset (Accordion `groups`, Maps `markers`, AreaChart `series`, PopupMenu items) instead of placeholder ClientTemplate that triggered CE0463 (#548)
- Pluggable widget CE0463 on Mendix 11.9 — `FormattingInfo TimeFormat` + `Selection` PascalCase normalization
- DataView `FormOrientation` / `LabelWidth` now controllable from MDL (#554)
- `ALTER PAGE` fixes: `INSERT`/`REPLACE` serializes DataGrid2 columns correctly; `set Title` actually updates the title (#561); column `SET` is case-insensitive and supports TextTemplate captions (#560); column inserts use the grid's data source as entity context
- Master-detail page round-trip — Gallery `ItemSelectionMode` + DataView selection-source described correctly
- `DesktopWidth` / `TabletWidth` / `PhoneWidth`: `AutoFill` now actually sets `Weight: -1` instead of dropping the override
- Pluggable widget validator respects MDL builtin property names (no false positives on `Label:`, `Caption:`, `DataSource:`, etc.)
- `mxcli check` detects custom-content column INSERT issues before MxBuild
- `--references` no longer flags `DROP + CREATE` of the same name as a conflict
- Reject Mendix reserved words on non-persistent entity attributes (#552)
- Cached catalog applies the current schema on load (no more "no such table" after schema bump)
- Nightly CE0117 on Mendix 10.24.19 — drop redundant `toString()` on string parameter

### Changed

- Test infrastructure: `TestMain` runs `widget init` on the shared source project so per-test copies inherit `.def.json` files; integration tests now exercise pluggable widget fixtures end-to-end
- Robust cleanup for doctype/mx-check tests eliminates ENOTEMPTY flake on CI
- `modernc.org/sqlite` bumped from 1.50.0 to 1.50.1

### Known limitations

- Two CE0463 cases remain for widgets with property-conditional TextTemplate visibility (VideoPlayer with `type='expression'`, Timeline with `customVisualization='true'`). Root cause and proposal in `docs/11-proposals/PROPOSAL_widget_property_visibility.md`; tracked under #574
- `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'` form is less feature-complete than the `datagrid` keyword form (no CONTROLBAR/customContent/per-column filter routing). Tracked under #529 Phase 4

## [0.10.0] - 2026-05-12

### Added

- **Maven/JAR dependency management** — `CREATE/DROP/SHOW JAR DEPENDENCY` statements; `jar_dependencies` catalog table; skill and docs-site pages (MDL-JARDEP)
- **Object-list pluggable widget properties** — grammar keywords for object-list blocks, extraction to `.def.json` (Phase 1), and BSON serialization through the executor (Phase 1 Layer 3)
- **LEGACYDATAGRID grammar** — keyword dispatch table and `LEGACYDATAGRID` grammar rule (Phase 2 pluggable widget overhaul)
- **`AllowCreateChangeLocally` flag** — exposed on external OData entities (#534)
- **Catalog: contract_entities → external_entities link** — cross-reference between contract catalog and integration catalog
- **`not(expr)` grammar enforcement** — grammar now requires parenthesised form; bare `not expr` rejected with CE0117 diagnostic

### Fixed

- `mxcli fmt` exits 1 on unparseable input and pipes describe output correctly (#398)
- ALTER SNIPPET failing with "page not found" (#402)
- `SHOW CONTEXT OF` entity showing empty definition (#396)
- `CREATE ENTITY` rejects unknown attribute type names (#392)
- `CREATE ENUMERATION` rejects duplicate value names (#390)
- `DROP ENUMERATION` errors on ambiguous unqualified name (#391)
- `CREATE ASSOCIATION` rejects duplicates for cross-module associations (#389)
- `GRANT/REVOKE ON ENTITY` validates module roles (#399)
- Enum XPath comparisons stored as string literals instead of enum refs (#176)
- Catalog crash on duplicate OData contract entities/actions
- `CATALOG.JAR_DEPENDENCIES` missing from `Tables()` list
- Three nightly CI failures on Mendix 10.24
- DataGrid2 `WidgetObject` boolean defaults aligned with `PropertyType` schema
- `TextTemplate` translation defaults populated; `Editable=Always` set on filters
- Required `CustomWidget` envelope fields added to filter widgets
- `WidgetObject Properties` reordered to match `WidgetType PropertyTypes` order
- `AllowUpload` field added to `WidgetValueType` BSON (closes one CE0463 gap)
- Unique placeholder IDs for `TextTemplate` translations (#30)
- Two ALTER PAGE bugs caught in test feedback
- ComboBox CE0463 — guard auto-populate and null `selectAllButtonCaption`
- Grammar added as explicit dependency of `build`, `test`, and `release` targets

## [0.9.0] - 2026-05-08

### Added

- **Inheritance split and cast** — `CASE $var IS Module.SubType THEN ... END CASE` and `CAST $var AS Module.SubType` statements in microflow/nanoflow bodies; full BSON roundtrip with branch anchors, nested continuation cases, and merge emission (CE0079)
- **CREATE OR MODIFY** — Standardised `OR MODIFY` variant across all remaining document types so scripts are idempotent by default (#510)
- **MDL-DUPDEF** — `mxcli check` detects duplicate `CREATE` for the same qualified name and reports `MDL-DUPDEF`

### Fixed

- Catalog crash on duplicate business event channels (#533)
- `flowRefCollector` skipping EnumSplitStmt case and else bodies — impacted `show callers/callees` accuracy
- CE0079: inheritance split branches that continue after the split were missing their merge node
- Nested `traverseFlowUntilMerge` guard could cross an outer merge boundary (#528)
- Inheritance split: branch anchors, case order, nested continuation tails, and nodes outside cases all preserved
- List-typed Java action arguments not emitting the `empty` keyword (#521); broadened to cover all resolved `BasicParameterType` params
- REST mapping cardinality not roundtripping — `as list of` syntax now parsed and emitted (#519)
- Import mapping: `MinOccurs`/`MaxOccurs` not parsed on mapping elements; repeating Object root treated as list; `SingleObject` inferred when `JsonStructure` absent
- Microflow layout: spacing, branch heights, and loop containment improved
- `TEXTFILTER` inside `DATAGRID COLUMN` not wired to the column filter slot (#189)
- `SET $obj/Assoc` path target rejected and produced wrong BSON (#511)
- `SHOW WIDGETS WHERE … LIKE` silently degraded to equality match
- Reserved OData attribute names not renamed when importing entities (#526)
- Virtual `System.*` Java actions missing from `ListJavaActions` and catalog
- `ConcurrencyMode=Fixed` incorrectly marked as Creatable during OData import (#525)
- Reverse-Reference traversal through entity inheritance misclassified
- `mxcli check --references` reporting false positives on `System.*` references (#523)
- ANTLR4 version unpinned in CI caused flaky Maven Central lookup failures

### Changed

- Generated ANTLR parser removed from git; `make grammar` step added to CI (#514)
- `MDLParser.g4` split into domain-specific grammar files for maintainability (#515)

## [0.8.0] - 2026-05-05

### Added

- **CREATE/DROP NANOFLOW** — Full nanoflow authoring pipeline: grammar, AST, visitor, executor, BSON writer, CALL NANOFLOW statement, GRANT/REVOKE nanoflow access, and nanoflow ELK diagram support in VS Code preview
- **CALL JAVASCRIPT ACTION** — `call javascript action Module.ActionName(params)` fully supported in CREATE NANOFLOW/MICROFLOW bodies: grammar, parser, builder, serializer, and roundtrip
- **CASE/WHEN enum split** — Enum-value split statements with `CASE $var WHEN Module.Value THEN ... END CASE` syntax; replaces the earlier `split on enum` draft
- **CALL WEB SERVICE (SOAP)** — Legacy SOAP microflow call statement with unsupported-detail preservation as raw BSON
- **RENAME WORKFLOW / RENAME MODULE** — RENAME now covers workflows and modules with reference refactoring
- **Ellipsis placeholder expression** — `...` as a placeholder in microflow expressions
- **Add-to-list expressions** — `add expression to $list` syntax in microflow/nanoflow bodies
- **Free microflow annotations** — Unattached `@annotation` nodes in microflow bodies survive describe → exec round-trip
- **@anchor sequence flow annotation** — `@anchor(from: X, to: Y)` on microflow statements pins SequenceFlow attachment sides; split and loop forms supported; builder-default and layout-equivalent anchors suppressed from DESCRIBE output
- **OpenAPI import for REST clients** — `CREATE REST CLIENT` accepts `OpenAPI: 'path/or/url'` to auto-generate a consumed REST service from an OpenAPI 3.0 spec (#207)
- **DESCRIBE CONTRACT OPERATION FROM OPENAPI** — Preview OpenAPI-generated operations without writing to the project
- **mxcli catalog search** — Search Mendix Catalog for data sources and services (#213)
- **Local file metadata for OData clients** — `CREATE ODATA CLIENT` supports `file://` URLs and relative paths for `MetadataUrl` (#206)
- **CATALOG.ASSOCIATIONS table** — Query association metadata via `select ... from CATALOG.ASSOCIATIONS` (#419)
- **SET format = json** — Session-level `SET key = value` command; `SET format = json` applies to all subsequent output
- **Java action improvements** — DROP/RENAME updates source file references; `void` qualified name resolved as VoidType; explicit void returns parsed correctly
- **SHOW LANGUAGES** — Language listing with Languages array parsing and executor handler (#480)
- **VS Code extension** — LSP coverage extended to all document types (nanoflows, workflows, Java actions, JSON structures, import/export mappings)
- **LSP snippet completions** — `CREATE NANOFLOW`, `CALL MICROFLOW`, `CALL NANOFLOW`, `CALL JAVASCRIPT ACTION`, `CALL JAVA ACTION` snippets added
- **make check-mdl** — Fast doctype script syntax validation target; wired into CI
- **Nanoflow diff support** — `mxcli diff` detects and displays nanoflow changes
- **Nanoflow validation parity** — `mxcli check` runs full body validation on nanoflows via shared `validateFlowBody` helper

### Fixed

- SIGSEGV in `buildPublishedRestResourceDef` on malformed REST syntax (#429)
- nil panic in ALTER WORKFLOW when activity ref is missing or uses a keyword (#430)
- Single quotes not escaped in DESCRIBE ENTITY XPath output (#431)
- `diff-local` git-error propagation and regression tests (#424)
- DataGrid2 column name derivation for ALTER PAGE (#116)
- O(N²) `GetMicroflow`/`GetNanoflow` replaced with direct unit lookup (#397)
- `CALL MICROFLOW`/`CALL NANOFLOW` validates targets exist before writing model (#395)
- `mxcli new` exits 0 on download failure (#422)
- Reject obviously malformed `MetadataUrl` in CREATE ODATA CLIENT (#427)
- Rename commands reject collisions with existing names (#432)
- Exit codes and error messages for marketplace, eval list, widget init, TUI (#425)
- `connect`/`disconnect`/`status` registered in syntax registry (#441)
- `resolveSnippetRef` checks session cache before querying backend (#509)
- DESCRIBE WORKFLOW output was missing the `CREATE` keyword (#478)
- RENAME MODULE failed due to uppercase ObjectType comparison in visitor (#473)
- JSON structure qualified-name lookup through folder hierarchy (#508)
- Retry-style error handler tail now loops back to a merge before the source (#507)
- Cross-module associations preserved on CREATE object actions (#502)
- Negative annotation coordinates parsed correctly (#494)
- Multiple retrieve XPath predicates preserved (#500)
- Custom error handler routing, empty else branch preservation, and structured conditional emit (#366)
- Validation feedback targets preserved with fully-qualified association paths (#359)
- Mapping result range cardinality and explicit REST mapping output variables (#372)
- SNIPPETCALL on parameterised snippets no longer corrupts model
- SHOW_PAGE button actions no longer produce null `PageParameterMapping.Variable` (#295)
- `Forms$SnippetParameterMapping` used for snippet call parameter mappings
- Marketplace search applies client-side filtering (#479)
- Recursion depth limit added to EXECUTE SCRIPT (#472)
- `CATALOG.ASSOCIATIONS`/`CONSTANTS`/`OBJECTS` returning no rows (#419)

### Changed

- **MDL string literal escapes** — `\n`, `\r`, `\t`, `\\` inside single-quoted literals are now escape sequences. Scripts embedding raw backslash sequences must double the backslash.
- **CatalogDB/CatalogTx interfaces** — Catalog, Builder, and LintContext migrated to interface; SQLite implementation extracted to `catalogdb_sqlite.go`
- **LintReader interface** — `sdk/mpr` removed from linter and executor; all reads go through `LintReader`
- **Type-safe BSON helpers** — `bsonString`/`bsonBool` consolidated in `mdl/bsonutil` package

## [0.7.0] - 2026-04-21

### Added

- **Agent Editor** — CREATE/DROP Agent, Knowledge Base, Consumed MCP Service, and Model documents; read support for all four types; DESCRIBE MODULE WITH ALL includes agent-editor documents
- **Consumed REST Client v2** — Redesigned syntax with full mapping support, parameter support for SEND REST REQUEST, BODY JSON FROM clause roundtrip, and TRANSFORM microflow action (JSLT/XSLT, Mendix 11.9+)
- **Platform Authentication** — `mxcli auth login/logout/status/list` with PAT scheme for mendix.com; credentials stored at `~/.mxcli/auth.json` (mode 0600), MENDIX_PAT env override
- **Marketplace Browsing** — `mxcli marketplace search/info/versions` with `--min-mendix` compatibility filtering
- **Entity Event Handlers** — Full MDL support for before/after create/change/delete event handlers with entity parameter validation
- **System Attributes** — AutoOwner, AutoChangedBy, and other audit pseudo-types; ALTER ENTITY ADD/DROP ATTRIBUTE for system attributes
- **ALTER PUBLISHED REST SERVICE** — Full in-place modification of published REST services (#161)
- **GRANT/REVOKE ACCESS on PUBLISHED REST SERVICE** (#162)
- **GitHub Copilot support** — First-class Copilot integration in `mxcli init`
- **Unified --json output** — All commands support structured JSON output (#134); `mxcli check --format json/sarif` outputs structured results
- **OData TripPin bulk-import** — Executable bulk-import example with @Constant syntax for ServiceUrl
- **Backend Abstraction** — `ExecContext` with typed backend interfaces, dispatch registry replacing type-switch, mutation backends (`page_mutator`, `widget_builder`, `datagrid_builder`, `workflow_mutator`) decoupled from `sdk/mpr`
- **mdl/types package** — Shared types and utilities extracted from `sdk/mpr` (EDMX, AsyncAPI, ID, navigation, infrastructure, JSON utils)
- **bsonutil package** — BSON utility functions (IDToBsonBinary, BsonBinaryToID, NewIDBsonBinary)
- **Mock-based handler tests** — 189 tests across 33 files covering all executor command handlers
- **OperationRegistry extensibility** — Pluggable operation registry with ContainerSnippet constant

### Fixed

- REST client BASIC auth uses correct `Rest$ConstantValue` BSON key (#200)
- ConnectionIndex lost on roundtrip (int64 vs int32 type mismatch) (#204)
- OData: ByAssociation DataSource serialization for DataGrid 2, capability annotations for entity/association CRUD (#201), bulk-create NPEs for primitive collections, derived/abstract/contained entities, and navigation associations (#143)
- UUID v4 version/variant bits in `GenerateDeterministicID`; panic on invalid UUID in `IDToBsonBinary`
- Cascade-delete associations on DROP ENTITY and DROP ODATA CLIENT
- Reserved keywords now allowed as module names in CREATE MODULE
- Quoted identifiers accepted in CREATE MODULE
- Find, Filter, ListRange list operations parsed and rendered (#212)
- DESCRIBE REST CLIENT resolves constant credentials to literal values (#192)
- DESCRIBE microflow roundtrip issues; eliminate redundant Merge nodes when IF branch returns
- COLUMN name falls back to attribute + scope association lookup by module (#202)
- Schema-level external `<Annotations>` blocks parsed in OData $metadata
- OData ServiceUrl validated as constant reference
- Agent-editor commands conformed to backend abstraction

### Changed

- Executor fully decoupled from storage layer — all BSON writes go through mutation backends (PRs #225, #237, #238, #239)
- All executor handlers migrated to free functions using `ExecContext` (removed 233 unused wrapper methods)
- `show*` executor functions renamed to `list*` for consistency
- Type aliases added in `sdk/mpr` for backward compatibility after shared-type extraction

## [0.6.0] - 2026-04-09

### Added

- **RENAME** — Automatic reference refactoring when renaming entities, attributes, associations, and other elements
- **CREATE EXTERNAL ENTITIES** — Bulk import entities from OData contracts (#143)
- **@excluded Annotation** — Mark documents and microflow activities as excluded, with Excluded column in catalog and `[EXCLUDED]` indicator in LIST
- **LIST Alias** — LIST as alias for SHOW in MDL and CLI
- **ALTER WORKFLOW** — Full activity manipulation (INSERT, DROP, REPLACE) for workflow definitions
- **Primitive Page Parameters** — Support for String, Integer, and other primitive types in page parameters
- **DataGrid Column Targeting** — Addressable columns in ALTER PAGE via dotted refs (e.g., `DataGrid.ColumnName`)
- **diff-local --ref** — Accept git ranges directly via `--ref` for comparing arbitrary revisions
- **Virtual System Module** — Complete module listing including System module
- **PasswordPolicy.ValidatePassword** — Demo user password validation against project policy
- **Multiple XPath Predicates** — Support `[cond1][cond2]` in WHERE clauses
- **DESCRIBE Enhancements** — Missing types added to mxcli describe command, view entity Source object preservation
- **Proposals** — Bulk external action support from OData contracts, RENAME with reference refactoring

### Fixed

- INTO clause in CREATE EXTERNAL ENTITIES not routing to target module
- Mendix 11.9.0 integration test failures
- Demo user password updated to meet 12-char policy
- JSON number type inference and mxcli new locale duplicates
- BSON properties aligned with Mendix schema for mx diff compatibility
- View entity Source object ID preserved with CREATE OR MODIFY in DESCRIBE

### Changed

- Refactored large files: executor.go (4 files), init.go (3 files), tui/app.go (4 files), cmd_entities.go (3 files)
- Simplified diff-local to accept git ranges via `--ref` directly (removed `--base` flag)
- Pre-warmed name lookup maps to eliminate O(n²) BSON parsing in catalog source
- Updated CI to test against Mendix 11.9.0
- Documentation updates: LIST preferred over SHOW, execution modes, DataGrid column targeting, IMAGE datasource properties

## [0.5.0] - 2026-04-06

### Added

- **Import/Export Mappings** — CREATE/DESCRIBE/DROP IMPORT MAPPING and EXPORT MAPPING with JSON Structure integration, array mapping, and BSON roundtrip
- **IMPORT FROM MAPPING / EXPORT TO MAPPING** — Microflow actions for mapping-based data transformation
- **JSON Structure FOLDER** — FOLDER clause for organizing JSON Structures into folders
- **DESCRIBE NANOFLOW** — Display nanoflow activities, control flows, and return type
- **Pluggable Widget Engine v2** — Redesigned widget engine with 25+ new widget templates (accordion, maps, charts, timeline, etc.), filter widget migration, and `generateDefJSON` property mapping
- **WidgetDemo** — Baseline scripts and widget analysis tools for widget testing
- **mxcli new** — Create Mendix projects from scratch (downloads MxBuild, creates project, runs init, installs Linux mxcli binary)
- **setup mxcli** — Download platform-specific mxcli binary from GitHub releases
- **Podman Support** — Podman as Docker alternative with devcontainer configuration (#34)
- **Catalog Tables** — Import/export mapping catalog tables for project metadata queries
- **Project Tree** — Missing document types added to project tree and syntax highlighting
- **GRANT Additive** — GRANT is now additive with partial REVOKE for entity access
- **Version Pre-checks** — Executor commands validate Mendix version before BSON writes
- **SHOW FEATURES** — Display version registry feature availability
- **SHOW LANGUAGES** — Language listing and QUAL005 missing translations linter rule
- **Proposals** — Design proposals for i18n, workflow improvements, and multi-project tree view
- **BSON Tooling Guide** — Contributor documentation for BSON debugging workflow
- **CONTRIBUTING.md** — Rewritten with accurate project references

### Fixed

- CE1613 and Studio Pro crash from invalid CrossAssociation BSON (ParentConnection/ChildConnection fields) (#50)
- Import/export mapping BSON alignment with Studio Pro (JsonPath, ExposedName, ObjectHandling, array elements)
- Sort translation map iteration in all serializers for deterministic output
- Docker and diaglog tests cross-platform compatibility (macOS Unix socket paths)
- Roundtrip test stability with idempotency strategy
- Version gates for Mendix 10.24 nightly test failures and 11.0+-only MOVE commands
- Nanoflow BSON parsing for activities, flows, and return type
- mxcli new MPR filename detection from create-project
- Bun setup in nightly and release workflows for vscode-ext build
- Replace unreleased Mendix 11.9.0 with 11.8.0 in CI workflows

### Changed

- Redesigned import/export mapping syntax (v2) with comma separators
- Bumped dependencies: esbuild 0.28.0, typescript 6.0.2, sqlite 1.48.1, go-runewidth 0.0.22, @vscode/vsce 3.7.1
- Bumped CI actions: checkout v6, deploy-pages v5, upload-pages-artifact v4
- Bumped mdbook to v0.5.2 with musl for aarch64
- PR review checklist requires working MDL examples for syntax changes

## [0.4.0] - 2026-03-31

### Added

- **SEND REST REQUEST** — Microflow action for consumed REST services with full BSON serialization roundtrip
- **Pluggable Image Widget** — Full roundtrip support for `com.mendix.widget.web.image.Image` with Studio Pro-extracted templates
- **ALTER PAGE SET Url** — Change page URLs via MDL
- **ALTER PAGE SET Layout** — Switch page layout via MDL
- **ALTER ENTITY SET POSITION** — Set entity position in domain model diagrams
- **VISIBLE IF / EDITABLE IF** — Conditional visibility and editability with XPath expressions, plus TabletWidth/PhoneWidth properties
- **EXECUTE DATABASE QUERY** — Microflow action for static, dynamic, and parameterized SQL with runtime connection override
- **Contract Browsing** — SHOW/DESCRIBE CONTRACT ENTITIES/ACTIONS from cached OData $metadata, CONTRACT CHANNELS/MESSAGES from AsyncAPI
- **Integration Catalog** — 7 new catalog tables (rest_clients, rest_operations, published_rest_services, external_entities, external_actions, business_events, contract tables)
- **SHOW EXTERNAL ACTIONS / PUBLISHED REST SERVICES** — Integration pane commands
- **SHOW CONSTANT VALUES** — Display constant values and catalog tables
- **CREATE/DROP CONFIGURATION** — Configuration management with constant overrides
- **JavaScript Actions** — NDSL/MDL support for JavaScript action definitions
- **DROP/MOVE FOLDER** — Remove empty folders and reorganize project structure
- **GALLERY Columns** — DesktopColumns/TabletColumns/PhoneColumns properties
- **Forward-Reference Hints** — Helpful error messages when exec fails on later-defined objects
- **IMAGE FROM FILE** — Image collection syntax for file-based images
- **OpenSSF Baseline Level 1** — Security foundations and CodeQL fixes
- **Multi-Agent Merge Proposal** — Design proposal for parallel agent work on Mendix projects
- **Documentation Site** — mdBook-based site with tutorials, language reference, migration guide, and internals
- **Tool Integrations** — Added support for OpenCode, Mistral Vibe, and GitHub Copilot in `mxcli init`
- **TUI Enhancements** — Agent channel (Unix socket), UX improvements, auto-create module support
- **Custom Widget AIGC Skill** — Skill for AI-generated custom pluggable widgets
- **AI Issue Triage** — GitHub Actions workflow for automated issue classification
- **Daily Project Digest** — Scheduled workflow for project activity summaries

### Fixed

- Skip null TextTemplate in opTextTemplate to avoid CE0463 widget definition errors
- Set Editable to Conditional and fix Visible XPath expression serialization
- REST client BSON serialization field ordering and roundtrip correctness
- Image widget template extraction (imageObject defaults, Parameters version marker, Texts$Translation)
- Escape single quotes in page DESCRIBE output via `mdlQuote()`
- Resolve association/attribute and entity/enumeration ambiguity in MDL parser
- LSP diagnostics for editable `mendix-mdl://` documents
- Gallery CE0463 by re-extracting template and fixing augmentation
- DataGrid2 column name derivation from attribute or caption
- ComboBox association EntityRef via IndirectEntityRef with association path
- XPath tokens written unquoted to prevent CE0161
- Long type written as `DataTypes$LongType` instead of IntegerType
- Date as distinct type from DateTime throughout the pipeline
- MPR version detection using DB schema and `_FormatVersion` field
- Recurse into loop bodies when extracting catalog references
- CodeQL symlink path traversal alerts in tar extraction
- Multiple TUI data races and agent channel stability fixes

### Changed

- Bumped dependencies: pgx v5.9.1, zap v1.27.1, go-runewidth v0.0.21, cobra v1.10.2, mongo-driver v1.17.9, sqlite v1.48.0
- Refactored Visible/Editable syntax to `visible: [xpath]` and `editable: [xpath]`
- Used dedicated CWTest module in custom widget examples
- Always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Added scope & atomicity and documentation sections to PR review checklist

## [0.3.0] - 2026-03-26

### Added

- **TUI** — Interactive terminal UI (`mxcli tui`) with yazi-style Miller columns, BSON/MDL preview, search, tabs, command palette (`:` key), session restore (`-c`), and mouse support
- **Workflows** — Full CREATE/DESCRIBE WORKFLOW support with activities (UserTask, Decision, CallMicroflow, CallWorkflow, Jump, WaitForTimer, ParallelSplit, BoundaryEvent), BSON round-trip, and ANNOTATION statements
- **Consumed REST Clients** — SHOW/DESCRIBE/CREATE consumed REST services with BSON writer and mx check validation
- **Image Collections** — SHOW/DESCRIBE/CREATE/DROP IMAGE COLLECTION with BSON writer and Kitty/iTerm2/Sixel inline image rendering in TUI
- **WHILE Loops** — WHILE loop support in microflows with examples
- **ALTER PAGE Variables** — ALTER PAGE ADD/DROP VARIABLE support (Phase 3)
- **XPath** — Dedicated XPath expression grammar, catalog table population, and skills reference
- **BSON Tools** — `bson dump --format ndsl`, `bson compare` with smart array matching, `bson discover` for field coverage analysis
- **Documentation Site** — mdBook-based site with full language reference, tutorials, and internals documentation
- **Anti-pattern Detection** — `mxcli check` detects nested loops and empty list anti-patterns (issue #21)
- **CREATE OR MODIFY** — Additive upsert for USER ROLE and DEMO USER
- **AI PR Review** — GitHub Actions workflow using GitHub Models API for automated pull request review
- **RETRIEVE FROM $Variable** — Support for in-memory and NPE list association traversal (issue #22)
- **Constants** — Constant syntax help topic, LSP snippet, and CREATE OR MODIFY examples
- **UnknownElement Fallback** — Table-driven parser registries with graceful fallback for unrecognized BSON types (issue #19)

### Fixed

- MPR corruption from dangling GUIDs after attribute drop/add (#4)
- BSON field ordering loss in ALTER PAGE operations (#3)
- ALTER PAGE SET Attribute property support (issue #10)
- ALTER PAGE REPLACE deep GUID regeneration for stale $ID fields (issue #9)
- Quoted identifiers not resolved in page widget references (issue #8)
- DATAGRID placeholder ID leak during template augmentation (issue #6)
- COMBOBOX association EntityRef via IndirectEntityRef with association path
- Page/layout unit type mismatch (Forms$ vs Pages$ prefix)
- VIEW entity types, constant value BSON, and test error detection
- False positive OQL type inference for CASE expressions
- RETRIEVE using DatabaseRetrieveSource for reverse Reference association traversal
- RETURNS Void treated as void return type like Nothing
- ANNOTATION keyword added to annotationName grammar rule
- System entity types and RETURN keyword formatting in microflows
- 10 CodeQL security alerts
- XPath token quoting for `[%CurrentDateTime%]` (#1)
- DROP MODULE/ROLE cascade-removes module roles from user roles
- Security script CE0066 entity access out-of-date errors
- Slow integration tests with build tags and TestMain (issue #16)
- Docker run failing on fresh projects (issue #13)

### Changed

- Aligned `mxcli check` and `mxcli lint` reporting with shared Violation format (issue #10)
- Promoted BSON commands from debug-only to release build
- Auto-discover `.mpr` file when `-p` is omitted
- Moved `bson/` and `tui/` packages under `cmd/mxcli/` for better encapsulation
- Consolidated show-describe proposals into `docs/11-proposals/` with archive
- Documented association ParentPointer/ChildPointer semantics in CLAUDE.md
- Normalized CRLF to LF in bug reports via `.gitattributes`

## [0.2.0] - 2026-03-15

### Added

- **CI/CD** — GitHub Actions workflow for build, test, and lint on push; release workflow for tagged versions
- **Makefile Lint Targets** — `make lint`, `make lint-go` (fmt + vet), `make lint-ts` (tsc --noEmit)
- **Playwright Testing** — Browser name config support, port-offset fixes, project directory CWD for session discovery
- **VS Code Extension** — Project tree auto-refresh via file watchers, association cardinality label fix

### Fixed

- Enum truncation, DROP+CREATE cache invalidation, duplicate variable detection, subfolder enum resolution
- IMPORT FK column NULL fallback and entity attribute validation
- Docker exec using host port instead of container-internal port
- AGGREGATE syntax in skills docs
- Association cardinality labels in domain model diagrams
- 3 MDL bugs and standardized enum DEFAULT syntax

### Changed

- Default to always-quoted identifiers in MDL to prevent reserved keyword conflicts
- Communication Style section in generated CLAUDE.md for human-readable change descriptions
- Shortened mxcli startup warning to single line
- Chromium system dependencies added to devcontainer Dockerfile

## [0.1.0] - 2026-03-13

First public release.

### Added

- **MDL Language** — SQL-like syntax (Mendix Definition Language) for querying and modifying Mendix projects
- **Domain Model** — CREATE/ALTER/DROP ENTITY, CREATE ASSOCIATION, attribute types, indexes, validation rules
- **Microflows & Nanoflows** — 60+ activity types, loops, error handling, expressions, parameters
- **Pages** — 50+ widget types, CREATE/ALTER PAGE/SNIPPET, DataGrid, DataView, ListView, pluggable widgets
- **Page Variables** — `variables: { $name: type = 'expression' }` in page/snippet headers for column visibility and conditional logic
- **Security** — Module roles, entity access rules, GRANT/REVOKE, UPDATE SECURITY reconciliation
- **Navigation** — Navigation profiles, menu items, home pages, login pages
- **Enumerations** — CREATE/ALTER/DROP ENUMERATION with localized values
- **Business Events** — CREATE/DROP business event services
- **Project Settings** — SHOW/DESCRIBE/ALTER for runtime, language, and theme settings
- **Database Connections** — CREATE/DESCRIBE DATABASE CONNECTION for Database Connector module
- **Full-text Search** — SEARCH across all strings, messages, captions, labels, and MDL source
- **Code Navigation** — SHOW CALLERS/CALLEES/REFERENCES/IMPACT/CONTEXT for cross-reference analysis
- **Catalog Queries** — SQL-based querying of project metadata via CATALOG tables
- **Linting** — 15 built-in rules + 27 Starlark rules across MDL, SEC, QUAL, ARCH, DESIGN, CONV categories
- **Report** — Scored best practices report with category breakdown (`mxcli report`)
- **Testing** — `.test.mdl` / `.test.md` test files with Docker-based runtime validation
- **Diff** — Compare MDL scripts against project state, git diff for MPR v2 projects
- **External SQL** — Direct queries against PostgreSQL, Oracle, SQL Server with credential isolation
- **Data Import** — IMPORT FROM external DB into Mendix app PostgreSQL with batch insert and ID generation
- **Connector Generation** — Auto-generate Database Connector MDL from external schema discovery
- **OQL** — Query running Mendix runtime via admin API
- **Docker Build** — `mxcli docker build` with PAD patching
- **VS Code Extension** — Syntax highlighting, diagnostics, completion, hover, go-to-definition, symbols, folding
- **LSP Server** — `mxcli lsp --stdio` for editor integration
- **Multi-tool Init** — `mxcli init` with support for Claude Code, Cursor, Continue.dev, Windsurf, Aider
- **Dev Container** — `mxcli init` generates `.devcontainer/` configuration for sandboxed AI agent development
- **MPR v1/v2** — Automatic format detection, read/write support for both formats
- **Fluent API** — High-level Go API (`api/` package) for programmatic model manipulation
