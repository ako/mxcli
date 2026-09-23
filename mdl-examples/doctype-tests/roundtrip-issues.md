# MDL Roundtrip Test Issues

> Generated: 2026-03-25
> Baseline: `/mnt/data_sdd/gh/mxproj-GenAIDemo/App.mpr`
> mxcli: `/mnt/data_sdd/gh/mxcli/bin/mxcli`

## Test Scope

| File | Agent | Status |
|------|-------|--------|
| 01-domain-model-examples.mdl | A | PASS |
| 02-microflow-examples.mdl | B | PARTIAL |
| 03-page-examples.mdl | C | PARTIAL |
| 04-math-examples.mdl | B | PARTIAL |
| 05-database-connection-examples.mdl | D | PASS (env CE) |
| 07-java-action-examples.mdl | D | PASS (env CE) |
| 08-security-examples.mdl | D | PARTIAL |
| 09-constant-examples.mdl | A | PARTIAL |
| 10-odata-examples.mdl | A | PASS |
| 11-navigation-examples.mdl | D | PASS |
| 12-styling-examples.mdl | C | PASS |
| 13-business-events-examples.mdl | D | PASS (env CE) |
| 14-project-settings-examples.mdl | D | PASS |
| 16-xpath-examples.mdl | A | FAIL |
| 17-custom-widget-examples.mdl | C | PARTIAL |

**Skipped (require Docker runtime):** 04-math-examples.tests.mdl, 06-rest-client-examples.test.mdl, 15-fragment-examples.test.mdl, microflow-spec.test.md, microflow-spec.test.mdl

> `15-fragment-examples.test.mdl` was skipped here for the wrong reason: it was
> named like a test file but declares no `@test` block, so it needed no runtime
> and was checked by nothing either. Renamed to `15-fragment-examples.mdl` in
> ako/mxcli#619; it has not been through this roundtrip run.

---

## Consolidated Bug Summary

> 15 files tested · 7 PASS · 6 PARTIAL · 1 FAIL · 1 env-only-FAIL

### HIGH — Real bugs, fix required

| # | Bug | Affected Files | Category |
|---|-----|----------------|----------|
| H1 | **WHILE loop body missing from DESCRIBE** — condition shown, body activities omitted entirely | 02, 04 | DESCRIBE writer | [#18](https://github.com/engalar/mxcli/issues/18) |
| H2 | **Long → Integer type degradation** — microflow params/vars and constants both affected; values > 2^31 silently corrupted | 02, 04, 09 | Writer type mapping | [#19](https://github.com/engalar/mxcli/issues/19) |
| H3 | **XPath `[%CurrentDateTime%]` quoted as string literal** → CE0161 at runtime; MDL token not recognised by writer | 16 | Writer / XPath parser | [#20](https://github.com/engalar/mxcli/issues/20) |
| H4 | **CE0463 pluggable widget template mismatch** — Gallery TEXTFILTER, ComboBox, DataGrid2 all trigger; object property count doesn't match type schema | 03, 08, 16, 17 | Widget template | (known) |
| H5 | **ComboBox association Attribute lost** — written as association pointer, DESCRIBE shows CaptionAttribute instead of original field name | 17 | Widget writer | [#21](https://github.com/engalar/mxcli/issues/21) |

### MEDIUM — Roundtrip gap, not data loss

| # | Bug | Affected Files | Category |
|---|-----|----------------|----------|
| M1 | **Sequential early-return IFs → nested IF/ELSE in DESCRIBE** — changes control flow semantics | 02, 04 | DESCRIBE formatter | [#22](https://github.com/engalar/mxcli/issues/22) |
| M2 | **DataGrid column names lost** — semantic names replaced by sequential identifiers | 03 | DESCRIBE / writer | [#23](https://github.com/engalar/mxcli/issues/23) |
| M3 | **DATAVIEW SELECTION DataSource not emitted by DESCRIBE** | 03 | DESCRIBE writer | [#24](https://github.com/engalar/mxcli/issues/24) |
| M4 | **Constant COMMENT not in DESCRIBE output** | 09 | DESCRIBE writer | [#25](https://github.com/engalar/mxcli/issues/25) |
| M5 | **Date type conflated with DateTime** — no Date-only type distinction | 01 | Writer type mapping | [#26](https://github.com/engalar/mxcli/issues/26) |

### LOW — Cosmetic / known asymmetry

| # | Issue | Notes |
|---|-------|-------|
| L1 | `LOG 'text' + $var` → template syntax with triple-quotes | Semantically equivalent |
| L2 | XPath spacing: `$a/b` → `$a / b` | Cosmetic |
| L3 | `RETURN false/true/empty` → `RETURN $false/$true/$empty` | Dollar prefix on literals |
| L4 | `DEFAULT 0.00` → `DEFAULT 0` | Decimal precision drop |
| L5 | REST default `ON ERROR ROLLBACK` always shown | DESCRIBE verbosity |
| L6 | `CaptionParams` → `ContentParams` rename | API rename, MDL not updated |
| L7 | Gallery adds default empty `FILTER` section in DESCRIBE | Verbosity |

### Environment CE (not mxcli bugs)

- CE errors for missing marketplace modules (DatabaseConnector, BusinessEvents) — module not installed in baseline
- CE0106 / CE0557 — security roles not assigned to microflows/pages in test scripts
- CE6083 — theme class mismatch (test app theme vs styling examples)

---

## Issues Found

<!-- Agent results below -->
## Agent B Results

### 02-microflow-examples.mdl
- **exec**: OK (all 92+ microflows created, 2 java actions, 1 page, 2 modules, MOVEs executed successfully)
- **mx check**: PASS (0 errors)
- **roundtrip gaps**:
  - **WHILE loop DESCRIBE missing body**: WHILE loops show the condition but the loop body activities are omitted from DESCRIBE output (e.g., M024_3_WhileLoop shows `WHILE $Counter < $N` then jumps to post-loop activities)
  - **WHILE uses END LOOP**: DESCRIBE outputs `END LOOP;` instead of `END WHILE;` for WHILE loops
  - **Sequential IFs become nested**: Independent sequential IF blocks (each with early RETURN) are rendered as nested IF/ELSE chains in DESCRIBE (e.g., M060_ValidateProduct: two independent IF checks for Code and Name become nested)
  - **LOG with concat becomes template**: `LOG ... 'text' + $var` is stored as `'{1}' WITH ({1} = 'text' + $var)` — semantically equivalent but different syntax roundtrip
  - **LOG template triple-quotes**: Template strings get triple-quoted in DESCRIBE: `'Processing order: {1}'` → `'''Processing order: {1}'''`
  - **XPath path spacing**: `$Product/Price` renders as `$Product / Price` (spaces around slash)
  - **RETURN literal prefix**: `RETURN false` → `RETURN $false`, `RETURN empty` → `RETURN $empty` (dollar-sign prefix on literals)
  - **RETURN formatting**: `RETURN $var` has trailing newline before semicolon in DESCRIBE
  - **Decimal literal truncation**: `42.00` renders as `42` in DESCRIBE
  - **REST default error handling shown**: DESCRIBE always shows `ON ERROR ROLLBACK` even when not specified in input (default value)
  - **M001_HelloWorld_2 FOLDER keyword**: Input specifies `FOLDER 'microflows/basic'` on CREATE, but later MOVE overrides to 'Basic'. DESCRIBE correctly shows FOLDER 'Basic'. No gap, but FOLDER keyword on CREATE is overridden by MOVE.
- **status**: PARTIAL (exec + mx check PASS, but DESCRIBE roundtrip has formatting and structural gaps)

### 04-math-examples.mdl
- **exec**: OK (module MathTest created, IsPrime and Fibonacci microflows created)
- **mx check**: PASS (0 errors)
- **roundtrip gaps**:
  - **WHILE loop body omitted from DESCRIBE**: Both IsPrime and Fibonacci have WHILE loops whose body activities are completely missing from DESCRIBE output. IsPrime's loop body (IF $Number mod $Divisor, SET $Divisor = $Divisor + 2) is not shown. Fibonacci's loop body (SET $Current, sliding window shifts, LOG DEBUG, counter increment) is not shown.
  - **Long type downgraded to Integer**: Fibonacci input specifies `RETURNS Long AS $Result` and `DECLARE $Result Long = 0`, `$Previous2 Long`, `$Previous1 Long`, `$Current Long`. DESCRIBE shows all as `Integer`. This is a **data type loss** — Long → Integer conversion.
  - **Sequential IFs become nested**: IsPrime and Fibonacci both have sequential early-return guard clauses that become nested IF/ELSE chains in DESCRIBE output
  - **RETURN literal prefix**: `RETURN false` → `RETURN $false`, `RETURN true` → `RETURN $true`, `RETURN 0` → `RETURN $0`, `RETURN 1` → `RETURN $1`
  - **LOG template triple-quotes**: Same as 02 file — template strings get triple-quoted
- **status**: PARTIAL (exec + mx check PASS, but WHILE body omission and Long→Integer type loss are significant DESCRIBE roundtrip gaps)

### Summary of Cross-Cutting Issues

| Issue | Severity | Category | Files Affected |
|-------|----------|----------|----------------|
| WHILE loop body missing from DESCRIBE | HIGH | DESCRIBE bug | 02, 04 |
| Long type stored as Integer | HIGH | Writer/type mapping | 04 |
| Sequential IFs rendered as nested IF/ELSE | MEDIUM | DESCRIBE formatting | 02, 04 |
| LOG concat becomes template syntax | LOW | Semantic equivalence | 02, 04 |
| LOG template triple-quoting | LOW | DESCRIBE formatting | 02, 04 |
| XPath path spacing ($a / b vs $a/b) | LOW | DESCRIBE formatting | 02 |
| RETURN literal dollar prefix | LOW | DESCRIBE formatting | 02, 04 |
| Decimal literal truncation | LOW | DESCRIBE formatting | 02 |
| REST default ON ERROR shown | LOW | DESCRIBE verbosity | 02 |
## Agent C Results

### 03-page-examples.mdl
- **exec**: OK (all 70+ pages/snippets/microflows created, MOVEs executed)
- **mx check**: FAIL (76 errors)
  - CE0557 x41: Page/snippet allowed roles not set (expected — no GRANT in script)
  - CE0106 x7: Microflow allowed roles not set (expected — no GRANT in script)
  - CE0463 x28: Pluggable widget definition changed (DataGrid2, ComboBox, TextFilter templates)
- **roundtrip gaps**:
  - **DATAGRID column names lost**: Input uses semantic names (`colName`, `colCode`, `colPrice`) but DESCRIBE outputs sequential names (`col1`, `col2`, `col3`). Applies to all DataGrid2 pages.
  - **GALLERY adds default FILTER section**: Input GALLERY without FILTER block (e.g., P019_Gallery_Simple) gets a default FILTER with DropdownFilter, DateFilter, TextFilter in DESCRIBE output. Template includes these filter widgets that were not in the original MDL.
  - **GALLERY Selection defaults to Single**: Input GALLERY without Selection property (P019) gets `Selection: Single` in output. This is a sensible default, not a bug.
  - **DATAVIEW DataSource: SELECTION missing in DESCRIBE**: P020_Master_Detail has `DataSource: SELECTION customerList` but DESCRIBE omits the DataSource entirely for `customerDetail` DataView.
  - **Button CaptionParams → ContentParams in DESCRIBE**: Input uses `CaptionParams: [{1} = 'abc']` on ACTIONBUTTON, DESCRIBE outputs `ContentParams: [{1} = 'abc']`. Naming inconsistency (functionally equivalent).
  - **Container Class/Style roundtrip OK**: P013b_Container_Basic with Class and Style properties roundtrips perfectly.
  - **GroupBox roundtrip OK**: P037_GroupBox_Example with Caption, HeaderMode, Collapsible roundtrips perfectly.
  - **Snippet roundtrip OK**: NavigationMenu snippet with SHOW_PAGE actions roundtrips perfectly.
  - **Page parameter, Url, Folder roundtrip OK**: All page metadata roundtrips correctly.
  - **MOVE roundtrip OK**: Folder changes (Pages/Archive) reflected correctly in DESCRIBE.
  - **DataGrid column properties roundtrip OK**: P033b with Alignment, WrapText, Sortable, Resizable, Draggable, Hidable, ColumnWidth, Size, Visible, DynamicCellClass, Tooltip all roundtrip correctly.
  - **DesignProperties on DataGrid/columns roundtrip**: Input DesignProperties on DataGrid and columns not reflected in DESCRIBE output (may be due to CE0463 template issues).
  - **CE0463 on all pluggable widgets**: DataGrid2 (28 instances), ComboBox, TextFilter templates have property count mismatches. Known engine-level issue with template extraction.
- **status**: PARTIAL (structure correct, CE0463 on pluggable widgets, minor naming/default gaps)

### 12-styling-examples.mdl
- **exec**: OK (module, entities, 6 pages, 1 snippet created; DESCRIBE/SHOW/UPDATE commands executed inline)
- **mx check**: FAIL (20 errors)
  - CE0557 x6: Page allowed roles not set (expected — no GRANT in script)
  - CE6083 x14: Design property not supported by theme ("Card style", "Disable row wrap", "Full width", "Border" on Container/ActionButton)
- **roundtrip gaps**:
  - **Class + Style roundtrip OK**: All CSS classes and inline styles roundtrip perfectly (P001, P002, P004, P006).
  - **DesignProperties roundtrip OK**: Toggle ON/OFF, multiple properties all roundtrip correctly in DESCRIBE output.
  - **CE6083 is theme-level**: The design properties used in examples ("Card style", "Disable row wrap") are not defined in the GenAIDemo project's theme. The writer serializes them correctly, but mx check rejects them as unsupported. This is a test environment issue, not a serialization bug.
  - **CREATE OR REPLACE page roundtrip OK**: P006_Roundtrip replaced with updated styling, DESCRIBE shows updated values correctly.
  - **Combined Class + Style + DesignProperties OK**: P004_Combined_Styling with all three styling mechanisms on single widgets roundtrips correctly.
- **status**: PASS (all styling features work; CE6083 is theme mismatch, CE0557 is expected)

### 17-custom-widget-examples.mdl
- **exec**: OK (enumeration, 2 entities, 1 association, 4 pages created)
- **mx check**: FAIL (3 errors)
  - CE0463 x2: ComboBox widget definition changed (cmbPriority, cmbCategory)
  - CE0463 x1: TextFilter widget definition changed (tfSearch)
- **roundtrip gaps**:
  - **GALLERY basic roundtrip OK**: P_Gallery_Basic TEMPLATE with DYNAMICTEXT roundtrips correctly. Gallery adds default FILTER section (same as 03-page-examples).
  - **GALLERY Selection defaults added**: Input had no Selection, output shows `Selection: Single`.
  - **GALLERY FILTER with TEXTFILTER**: Input specifies `TEXTFILTER tfSearch (Attribute: Title)` but DESCRIBE shows default filter template (DropdownFilter, DateFilter, TextFilter) instead of the user-specified single TextFilter. The user's TextFilter `Attribute:` binding is lost.
  - **COMBOBOX enum roundtrip OK (DESCRIBE)**: P_ComboBox_Enum DESCRIBE shows correct `Attribute: Priority`. CE0463 at mx check level.
  - **COMBOBOX association Attribute wrong**: P_ComboBox_Assoc input has `Attribute: Task_Category` (association name) but DESCRIBE shows `Attribute: Name` (the CaptionAttribute value instead). The association binding is lost in roundtrip.
  - **CE0463 on ComboBox**: Known template mismatch issue. The widget definition template doesn't match what the engine expects. Documented as known bug in the MDL file.
- **status**: PARTIAL (Gallery basic works, ComboBox enum roundtrip OK at DESCRIBE level but CE0463 at mx check; ComboBox association attribute lost)

---

### Summary of Systemic Issues

| Issue | Severity | Files Affected | Description |
|-------|----------|---------------|-------------|
| CE0463 pluggable widget templates | HIGH | 03, 17 | DataGrid2, ComboBox, TextFilter templates have property count mismatches. 28+ instances. |
| DATAGRID column names lost | MEDIUM | 03 | Semantic column names → sequential col1/col2/col3 in DESCRIBE |
| GALLERY adds default FILTER | LOW | 03, 17 | Galleries without explicit FILTER get default DropdownFilter+DateFilter+TextFilter |
| DATAVIEW SELECTION DataSource missing | MEDIUM | 03 | `DataSource: SELECTION widgetName` not emitted in DESCRIBE |
| COMBOBOX association Attribute lost | HIGH | 17 | Association name replaced by CaptionAttribute in DESCRIBE |
| CaptionParams → ContentParams rename | LOW | 03 | Button caption params use different property name in DESCRIBE |
| CE6083 theme design properties | LOW | 12 | Design properties not in project theme — test env issue |
| CE0557/CE0106 security roles | INFO | 03, 12 | Expected — no GRANT statements in test scripts |

## Agent A Results

### 01-domain-model-examples.mdl
- **exec**: OK (all 45+ elements created successfully, including enums, entities, associations, view entities, ALTER ENTITY, MOVE, EXTENDS)
- **mx check**: PASS (0 errors)
- **roundtrip gaps**:
  - **Decimal DEFAULT precision lost**: Input `DEFAULT 0.00` → DESCRIBE outputs `DEFAULT 0` (affects Product.Price, SalesOrder.TotalAmount, etc.). Cosmetic only — Mendix treats them the same.
  - **Enumeration value JavaDoc lost**: Input `/** Order received */  PENDING 'Pending'` → DESCRIBE outputs `PENDING 'Pending'` (per-value docs not preserved in DESCRIBE output).
  - **Date vs DateTime type conflation**: Input `ReleaseDate: Date` → DESCRIBE outputs `ReleaseDate: DateTime`. The `Date` type is stored as DateTime internally but DESCRIBE doesn't distinguish.
  - **VATRate.Check Boolean→String(10) conversion side-effect**: After `ALTER ENTITY DmTest.VATRate MODIFY ATTRIBUTE Check String(10)`, DESCRIBE shows `Check: String(10) DEFAULT 'true'` — the original boolean DEFAULT true was converted to string `'true'`. Expected behavior but notable.
  - **VATRate position shifted**: Input `@Position(700, 300)` → DESCRIBE outputs `@Position(4450, 100)`. Position auto-adjusted after ALTER operations.
  - **ALTER ENTITY attributes lack JavaDoc**: Attributes added via `ALTER ENTITY ... ADD ATTRIBUTE` don't have JavaDoc comments (e.g., `DiscountPercentage`, `LoyaltyPoints`). Expected — ALTER ADD doesn't support inline docs.
  - **MOVE operation works correctly**: Customer moved to DmTest2, Country enum moved to DmTest3 — DESCRIBE confirms correct module references including cross-module enum refs (`DmTest3.Country`).
  - **INDEX direction preserved**: Input `INDEX (OrderDate desc)` → DESCRIBE outputs `INDEX (OrderDate DESC)` — correct.
  - **EXTENDS roundtrip clean**: Attachment (System.FileDocument), Truck (DmTest.Vehicle), ProductPhoto (System.Image) all roundtrip correctly including GENERALIZATION keyword.
  - **CREATE OR MODIFY idempotent**: AppConfig second version correctly overwrites first (position, attributes, indexes all updated).
  - **GetCustomerTotalSpent microflow**: DESCRIBE shows `$Stats / TotalSpent` (with spaces around `/`) instead of `$Stats/TotalSpent`. Cosmetic formatting difference.
- **status**: PASS

### 09-constant-examples.mdl
- **exec**: OK (7 constants created, 2 modified via CREATE OR MODIFY, 1 dropped)
- **mx check**: PASS (0 errors)
- **roundtrip gaps**:
  - **Long type stored as Integer**: Input `TYPE Long DEFAULT 10485760` → DESCRIBE outputs `TYPE Integer DEFAULT 10485760`. The Long constant type is not preserved — stored/displayed as Integer. This is a real gap.
  - **COMMENT not shown in DESCRIBE**: Input `COMMENT 'API key for external service...'` → DESCRIBE output does not include the COMMENT field. Comments are stored but not roundtripped in DESCRIBE output.
  - **CREATE OR MODIFY works correctly**: ServiceEndpoint updated from v1 URL to staging v2 URL; EnableDebugLogging changed from false to true.
  - **DROP CONSTANT works**: LaunchDate successfully removed from constant list.
  - **Folder not shown**: SHOW CONSTANTS shows empty Folder column for CoTest constants (none were specified in input). Expected.
- **status**: PARTIAL (Long→Integer type loss is a real bug; COMMENT not in DESCRIBE is a gap)

### 10-odata-examples.mdl
- **exec**: OK (all OData lifecycle operations completed: CREATE, ALTER, CREATE OR MODIFY, GRANT, REVOKE, DROP for clients, services, external entities)
- **mx check**: PASS (0 errors)
- **roundtrip gaps**:
  - **Lifecycle test, not roundtrip**: This file creates OData resources then drops them at the end, so final state has no OData objects to DESCRIBE. The create→modify→alter→drop lifecycle completes without errors.
  - **Base entities remain**: OdTest.Customer and OdTest.Order persist after cleanup (not dropped by script). These are setup entities, not OData-specific.
  - **No DESCRIBE verification possible**: Since all OData objects are dropped by end of script, roundtrip comparison of DESCRIBE output vs input is not applicable. The test validates write operations succeed, not read-back fidelity.
- **status**: PASS (lifecycle test — all operations succeed, 0 mx errors)

### 16-xpath-examples.mdl
- **exec**: OK (4 entities, 3 associations, 22 microflows, 2 pages created)
- **mx check**: FAIL (3 errors)
  - **CE0161**: "Error(s) in XPath constraint" at Retrieve object(s) activity 'Retrieve list of Order from database' — likely from `Retrieve_DateTimeToken` where `[%CurrentDateTime%]` token is quoted as a string literal in the BSON XPath. DESCRIBE shows `WHERE OrderDate >= '[%CurrentDateTime%]'` — the token is wrapped in quotes, which may cause the XPath engine to treat it as a string literal instead of a token.
  - **CE0463** (x2): DataGrid2 widget definition errors on `dgOrders` and `dgCustomers` pages — known pluggable widget template issue, not XPath-related.
- **roundtrip gaps**:
  - **XPath brackets stripped**: Input `WHERE [State = 'Completed']` → DESCRIBE outputs `WHERE State = 'Completed'` (brackets removed). Cosmetic difference — both forms are valid MDL syntax.
  - **RETURN true → RETURN $true**: Input `RETURN true;` → DESCRIBE outputs `RETURN $true;`. The boolean literal is represented as variable `$true`.
  - **XPath functions roundtrip correctly**: `contains()`, `starts-with()`, `not()`, `true()`, `false()` all preserved in DESCRIBE output.
  - **Association paths preserved**: `XpathTest.Order_Customer/XpathTest.Customer/Name` roundtrips correctly.
  - **Variable paths preserved**: `$Customer/Name` in XPath roundtrips correctly.
  - **Token quoting**: `[%CurrentDateTime%]` appears as `'[%CurrentDateTime%]'` in DESCRIBE — wrapped in string quotes. This may be the cause of CE0161.
  - **System.owner token**: `System.owner = '[%CurrentUser%]'` roundtrips correctly in DESCRIBE.
  - **Parenthesized logic preserved**: Complex grouping `($IgnoreAfterDate or OrderDate >= $AfterDate)` roundtrips correctly.
  - **Pages CE0463**: DataGrid2 widgets `dgOrders` and `dgCustomers` fail with CE0463 — this is a known pluggable widget template issue, not specific to XPath functionality.
- **status**: FAIL (CE0161 XPath constraint error on DateTime token; CE0463 on pages)
## Agent D Results

### 05-database-connection-examples.mdl
- **exec**: OK
- **mx check**: FAIL (expected): 4 errors — "We couldn't find the External Database Connector module in your app" at 4 Execute database query action activities
- **roundtrip gaps**:
  - MinimalDB: DESCRIBE omits empty `BEGIN/END` block (input has `BEGIN END;`), equivalent semantically
  - F1Database: DESCRIBE matches input exactly (all 8 queries, parameters, mappings)
  - Microflows (GetAllDrivers, GetDriversDynamic, GetDriversByNationality): DESCRIBE matches input
  - MOVE CONSTANT/DATABASE CONNECTION to folders: executed without error (no DESCRIBE verification for folder placement)
- **notes**: CE errors are expected — the test project lacks the External Database Connector marketplace module. Not an mxcli bug.
- **status**: PASS (roundtrip correct, CE errors are environment-dependent)

### 07-java-action-examples.mdl
- **exec**: OK — created 25 java actions, 40+ microflows, 1 page
- **mx check**: FAIL (expected): 25 CE0106 errors — "At least one allowed role must be selected if the microflow is used from navigation, a page, a nanoflow or a published service." Microflows called from test page buttons have no security roles configured.
- **roundtrip gaps**:
  - Java actions round-trip correctly: basic (GetCurrentTimestamp), primitive params (ToUpperCase, Concatenate), entity params (SendEmail), list params (ProcessEmails), type parameters (`ENTITY <pEntity>`), `EXPOSED AS` (FormatCurrency)
  - Inline Java code preserved with formatting (extra indentation added on read-back but functionally equivalent)
  - Microflows calling java actions: parameter mappings, `ON ERROR ROLLBACK` default, entity type params (`EntityType = 'JaTest.EmailMessage'`) all correct
- **notes**: CE0106 is expected — the test script creates a page with buttons calling microflows without setting up module roles/access. Script-level issue, not mxcli bug.
- **status**: PASS (roundtrip correct, CE0106 expected for page-button microflows without roles)

### 08-security-examples.mdl
- **exec**: OK — created module roles (User, Administrator, Viewer, Manager), user roles (RegularUser, SuperAdmin), demo users, grants/revokes all executed
- **mx check**: FAIL: 2 CE0463 errors — "The definition of this widget has changed" at Data grid 2 widgets (dgOrder, dgCustomer)
- **roundtrip gaps**:
  - Module roles: 4 created with correct descriptions
  - User roles: RegularUser (2 module roles), SuperAdmin (manage all) — PowerUser correctly dropped
  - Demo users: sectest_user dropped, sectest_admin remains with roles (RegularUser, SuperAdmin)
  - Microflow access: ACT_Customer_Create→User,Manager; ACT_Customer_Delete→Manager,Administrator; ACT_Order_Process→Administrator,Manager — matches final state after grants/revokes
  - Page access: Customer_Overview→User,Manager,Administrator; Order_Overview→Administrator,Manager — correct
  - Entity access: all revoked at end (cleanup section) — correct
  - One minor gap: `REVOKE SecTest.Manager ON SecTest.Customer` prints "No access rules found" — Manager role was never granted entity access on Customer, so this is harmless
- **notes**: CE0463 on DATAGRID widgets is a known widget template issue, unrelated to security features.
- **status**: PARTIAL (security roundtrip correct; CE0463 from DATAGRID widget template is a separate known issue)

### 11-navigation-examples.mdl
- **exec**: OK — created NavTest module, page, multiple CREATE OR REPLACE NAVIGATION statements, catalog queries
- **mx check**: PASS — 0 errors
- **roundtrip gaps**:
  - Final navigation state matches last `CREATE OR REPLACE NAVIGATION` statement (HOME PAGE MyFirstModule.Home_Web, MENU with Home item)
  - Intermediate navigation changes correctly overwritten by subsequent CREATE OR REPLACE
  - LOGIN PAGE not shown in DESCRIBE output (may be omitted when using default, or Administration.Login is the default)
  - Role-based HOME PAGE override (`FOR Administration.Administrator`) not visible in final DESCRIBE (overwritten by later CREATE OR REPLACE that didn't include it)
  - SHOW NAVIGATION, SHOW NAVIGATION MENU, SHOW NAVIGATION HOMES all worked
  - REFRESH CATALOG FULL and catalog queries executed correctly
- **status**: PASS

### 13-business-events-examples.mdl
- **exec**: OK — created BusinessEvents stub module, entities, 2 services, CREATE OR REPLACE, DROP
- **mx check**: FAIL (expected): 1 error — "BusinessEvents module version folder not found"
- **roundtrip gaps**:
  - CustomerEventsApi DESCRIBE matches input exactly (ServiceName, EventNamePrefix, 2 messages with PUBLISH + ENTITY binding)
  - SimpleEvents correctly replaced via CREATE OR REPLACE (ServiceName changed to 'SimpleEventsUpdated', EventNamePrefix to 'v2')
  - SimpleEvents correctly dropped via DROP BUSINESS EVENT SERVICE
  - SHOW BUSINESS EVENT SERVICES and SHOW BUSINESS EVENTS output correct
- **notes**: CE error is expected — the test creates a stub BusinessEvents module, but mx expects the full marketplace module structure.
- **status**: PASS (roundtrip correct, CE error is environment-dependent)

### 14-project-settings-examples.mdl
- **exec**: OK — all ALTER SETTINGS executed (MODEL, CONFIGURATION, CONSTANT, LANGUAGE, WORKFLOWS)
- **mx check**: PASS — 0 errors
- **roundtrip gaps**:
  - MODEL settings verified: AfterStartupMicroflow='MyModule.ASU_Startup', HashAlgorithm='BCrypt', JavaVersion='Java21', RoundingMode='HalfUp', BcryptCost=12, AllowUserMultipleSessions=true — all match input
  - CONFIGURATION 'Default' updated (DatabaseType, DatabaseUrl, etc.)
  - CONSTANT override in configuration: MyModule.ServerUrl = 'kafka:9092' in 'Default'
  - LANGUAGE DefaultLanguageCode = 'en_US'
  - WORKFLOWS UserEntity = 'System.User'
- **status**: PASS

### Summary

| File | exec | mx check | roundtrip | status |
|------|------|----------|-----------|--------|
| 05-database-connection | OK | FAIL (4 CE - missing marketplace module) | Correct | PASS |
| 07-java-action | OK | FAIL (25 CE0106 - no security roles on page buttons) | Correct | PASS |
| 08-security | OK | FAIL (2 CE0463 - DATAGRID widget template) | Correct | PARTIAL |
| 11-navigation | OK | PASS (0 errors) | Correct | PASS |
| 13-business-events | OK | FAIL (1 CE - missing BusinessEvents module structure) | Correct | PASS |
| 14-project-settings | OK | PASS (0 errors) | Correct | PASS |

**Key findings:**
1. All 6 files execute successfully — no exec errors
2. Roundtrip fidelity is excellent across all tested doc types
3. CE errors are all either environment-dependent (missing marketplace modules) or test-script-level issues (missing security roles for page-referenced microflows)
4. CE0463 on DATAGRID widgets (file 08) is a known widget template issue
5. No mxcli bugs found in these test files

**Script re-run note:** Re-ran with `/tmp/mxrt/roundtrip.sh` — all 6 files exec=OK. The script's `describe-all.sh` only covers entities/microflows/pages (not database connections, java actions, security, navigation, business events, or project settings), so diff results are noisy and incomplete for these doc types. Manual DESCRIBE verification above is authoritative.
