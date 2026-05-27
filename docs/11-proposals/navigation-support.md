---
title: Navigation Support in MDL
status: partial
---

# Proposal: Navigation Support in MDL

## Background

Every Mendix project has exactly one `navigation$NavigationDocument` — a project-level singleton containing navigation profiles (Responsive, Phone, Tablet, Native). Each profile defines:

- A **default home page** (page or microflow)
- **Role-based home pages** (override per user role)
- A **menu structure** (hierarchical menu items pointing to pages/microflows)
- A **login page** (optional custom login)
- **Offline entity configs** (sync mode per entity, for offline profiles)
- **Native settings** (OTA, encryption, transitions — native profiles only)

Navigation is currently the most significant gap in the MDL toolchain. The parser only extracts the document name — no profiles, home pages, menus, or role mappings are parsed. This means:

- `show context` cannot trace "which pages are entry points"
- The catalog `refs` table has no `home_page`, `menu_item`, or `login_page` ref kinds
- The architecture diagram proposal is blocked on knowing where users start

## Investigation Findings

### Storage structure (from QueryDemoApp.mpr)

The navigation document BSON contains a `Profiles` array. Each profile is either `navigation$NavigationProfile` (web) or `navigation$NativeNavigationProfile` (native). The test project has one Responsive profile with:

- **HomePage.Page**: `"Main.Home_Web"` (BY_NAME reference)
- **3 menu items**: Home → `Main.Home_Web`, Users → `Administration.Account_Overview`, OQL Pad → `OqlPad.OqlEditor`
- **No role-based home pages** (empty `HomeItems` array)
- **No custom login page** (empty `LoginPageSettings.Form`)

### Key storage name discrepancies

| SDK property | BSON storage name | Notes |
|---|---|---|
| `roleBasedHomePages` | `HomeItems` | Not `RoleBasedHomePages` |
| `menuItemCollection` | `menu` | Not `MenuItemCollection` |
| `pages$PageSettings` | `Forms$FormSettings` | Form/Page legacy naming |
| `PageSettings.page` | `Form` | Field also uses legacy name |

### All references navigation holds

| What | Reference type | Target |
|---|---|---|
| Default home page | BY_NAME | `pages$page` or `microflows$microflow` |
| Role-based home page | BY_NAME | `pages$page` or `microflows$microflow` + `security$UserRole` |
| Login page | BY_NAME | `pages$page` (via `Forms$FormSettings.Form`) |
| Menu item action | BY_NAME | `pages$page` (via `Forms$FormAction.FormSettings.Form`) |
| Not-found page | BY_NAME | `pages$page` or `microflows$microflow` (since 10.13.0) |
| App icon | BY_NAME | `Images$image` |
| Offline entity | BY_NAME | `DomainModels$entity` |
| Native home page | BY_NAME | `pages$page` or `microflows$nanoflow` |
| Native bottom bar | BY_NAME | `pages$page` |

## Proposed MDL Support

### Phase 1: Read-only (SHOW / DESCRIBE)

This is the most valuable phase — it unlocks catalog integration and architecture diagrams without needing to write navigation BSON.

#### SHOW NAVIGATION

Display a summary of all navigation profiles:

```sql
show navigation;
```

Output:
```
navigation Profiles:
  Responsive (Web)
    home: Main.Home_Web
    menu: 3 items
    login: (default)

  Phone (Mobile)
    home: Main.Home_Phone
    menu: 2 items
    login: Main.Login_Phone
    role-based homes:
      Customer → Main.Customer_Home
      Admin → Administration.Admin_Home
```

#### DESCRIBE NAVIGATION

Show full detail for a specific profile:

```sql
describe navigation Responsive;
```

Output:
```
Profile: Responsive
Kind: Responsive
home page: Main.Home_Web
login page: (default)
not found page: (not set)

role-Based home pages:
  (none)

menu:
  home                → Main.Home_Web
  users               → Administration.Account_Overview
  OQL Pad             → OqlPad.OqlEditor
```

For nested menus:
```
menu:
  Dashboard           → Main.Dashboard
  Orders              (submenu)
    ├─ all Orders     → Orders.Order_Overview
    ├─ New Order      → Orders.Order_New
    └─ Reports        (submenu)
         ├─ Monthly   → Reports.Monthly_Report
         └─ Yearly    → Reports.Yearly_Report
  settings            → Admin.Settings
```

#### SHOW NAVIGATION MENU

Show just the menu tree for a profile:

```sql
show navigation menu Responsive;
show navigation menu;              -- defaults to first Responsive profile
```

#### SHOW NAVIGATION HOMES

Show home page assignments across all profiles:

```sql
show navigation homes;
```

Output:
```
Profile      role        home page
Responsive   (default)   Main.Home_Web
Phone        (default)   Main.Home_Phone
Phone        Customer    Main.Customer_Home
Phone        Admin       Administration.Admin_Home
Native       (default)   Main.NativeHome
```

### Phase 2: Catalog integration

Add navigation references to the `refs` table so they participate in `show callers`, `show impact`, `show context`, and the architecture diagram.

#### New ref kinds

| RefKind | Source | Target | Meaning |
|---|---|---|---|
| `home_page` | `Navigation.Responsive` | `Main.Home_Web` | Default home page |
| `role_home` | `Navigation.Responsive` | `Main.Customer_Home` | Role-based home page |
| `menu_item` | `Navigation.Responsive` | `Orders.Order_Overview` | Menu item targets page |
| `login_page` | `Navigation.Responsive` | `Main.Login` | Custom login page |

The source name format `Navigation.<ProfileName>` is a synthetic qualified name for the profile (navigation profiles have `qualifiedNamePathDepth: 1` in the metamodel, so they do have qualified names).

#### Impact on existing commands

After catalog integration, these commands automatically pick up navigation:

```sql
-- "Which pages are entry points from navigation?"
show references to Main.Home_Web;
-- Output now includes: Navigation.Responsive → Main.Home_Web (home_page)

-- "What breaks if I rename this page?"
show impact of Main.Home_Web;
-- Output now includes: Navigation.Responsive (home_page)

-- "Full context for architecture diagram"
show context of Main.Home_Web depth 2;
-- Output now includes: Referenced by navigation: Responsive (home page), Responsive (menu item)
```

### Phase 3: Write support (CREATE OR REPLACE NAVIGATION) — IMPLEMENTED

Full replacement of navigation profiles using `create or replace navigation`. This follows the same describe→create-or-modify pattern used by other MDL commands. The output from `describe navigation` is directly executable.

#### Syntax

```sql
-- Full replacement: home page, login page, menu
create or replace navigation Responsive
  home page Main.Home_Web
  login page Administration.Login
  menu (
    menu item 'Home' page Main.Home_Web;
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
    );
  );

-- With role-based home page overrides
create or replace navigation Responsive
  home page Main.Home_Web
  home page Main.AdminHome for MyModule.Administrator
  login page Administration.Login;

-- Set not-found page
create or replace navigation Responsive
  home page Main.Home_Web
  not found page Main.Custom404;

-- Clear the menu (empty MENU block removes all items)
create or replace navigation Responsive
  home page Main.Home_Web
  menu ();

-- Use MICROFLOW instead of PAGE for home
create or replace navigation Responsive
  home microflow Main.ACT_ShowHome;
```

All clauses are optional — omitted clauses clear that section. Profile name is matched case-insensitively.

## Implementation Plan

### Phase 1: Parser + SHOW commands (read-only)

**Files to change:**

1. **`sdk/mpr/reader_types.go`** — Expand `NavigationDocument` with full type hierarchy:

```go
type NavigationDocument struct {
    model.BaseElement
    ContainerID model.ID
    Name        string
    Profiles    []NavigationProfile
}

type NavigationProfile struct {
    model.BaseElement
    Name                string
    Kind                string // "Responsive", "Phone", etc.
    HomePage            *HomePage
    RoleBasedHomePages  []RoleBasedHomePage
    LoginPage           string // qualified page name (or empty)
    NotFoundPage        string // qualified page name (or empty)
    MenuItems           []MenuItem
    OfflineEntityConfigs []OfflineEntityConfig
    // Native-only fields
    IsNative            bool
    NativeSettings      *NativeSettings // nil for web profiles
}

type HomePage struct {
    page      string // qualified name (BY_NAME)
    microflow string // qualified name (BY_NAME), alternative to page
}

type RoleBasedHomePage struct {
    UserRole  string // qualified name
    page      string
    microflow string
}

type MenuItem struct {
    caption  string
    page     string // target page qualified name
    Items    []MenuItem // sub-items (recursive)
}

type OfflineEntityConfig struct {
    entity     string // qualified entity name
    SyncMode   string // "all", "Constrained", "Never", etc.
    constraint string // xpath
}
```

2. **`sdk/mpr/parser_misc.go`** — Expand `parseNavigationDocument()` to walk the BSON structure:
   - Parse `Profiles` array (handle storageListType marker at index 0)
   - Dispatch on `$type`: `navigation$NavigationProfile` vs `navigation$NativeNavigationProfile`
   - Parse `HomePage` → extract `page` and `microflow` fields
   - Parse `HomeItems` (roleBasedHomePages) → extract `UserRole`, `page`, `microflow`
   - Parse `menu` → `Items` → recursively extract `caption`, `Action.FormSettings.Form`
   - Parse `LoginPageSettings` → extract `Form` field
   - Parse `NotFoundHomepage` → extract `page` and `microflow`
   - Parse `OfflineEntityConfigs` → extract `entity`, `SyncMode`, `constraint`

   Key BSON paths:
   ```
   Profiles[1+].$type                                    → profile type
   Profiles[1+].Kind                                     → profile kind enum
   Profiles[1+].Name                                     → profile name
   Profiles[1+].HomePage.Page                             → default home page
   Profiles[1+].HomePage.Microflow                        → default home microflow
   Profiles[1+].HomeItems[1+].UserRole                    → role name
   Profiles[1+].HomeItems[1+].Page                        → role home page
   Profiles[1+].Menu.Items[1+].Caption.Items[1+].Text     → menu caption (en_US)
   Profiles[1+].Menu.Items[1+].Action.FormSettings.Form   → menu target page
   Profiles[1+].Menu.Items[1+].Items[1+]...               → sub-menus (recursive)
   Profiles[1+].LoginPageSettings.Form                    → login page
   Profiles[1+].NotFoundHomepage.Page                     → 404 page
   ```

3. **`sdk/mpr/reader.go`** — Add `GetNavigation() (*NavigationDocument, error)` convenience method that returns the parsed singleton.

4. **`mdl/executor/cmd_describe.go`** (or new `cmd_navigation.go`) — Implement:
   - `execShowNavigation()` — summary of all profiles
   - `execDescribeNavigation(profileName)` — full detail for one profile
   - `execShowNavigationMenu(profileName)` — menu tree
   - `execShowNavigationHomes()` — home page matrix

5. **`mdl/grammar/MDLLexer.g4` + `MDLParser.g4`** — Add tokens and rules:
   ```antlr
   // Lexer tokens
   navigation: N A V I G A T I O N;
   homes: H O M E S;
   menu: M E N U;

   // Parser rules
   showNavigationStmt: show navigation (menu qualifiedName? | homes)?;
   describeNavigationStmt: describe navigation qualifiedName?;
   ```

6. **`mdl/visitor/`** — AST node types and visitor rules for the new statements.

7. **`mdl/ast/`** — Add `ShowNavigationStmt`, `DescribeNavigationStmt` AST nodes.

### Phase 2: Catalog integration

**Files to change:**

1. **`mdl/catalog/builder_references.go`** — Add `buildNavigationReferences()`:
   - Call `reader.GetNavigation()`
   - For each profile, insert refs:
     - `home_page`: profile → home page
     - `role_home`: profile → role-based home pages
     - `menu_item`: profile → each menu item's target page (recursive)
     - `login_page`: profile → login page (if set)
   - Source name format: `Navigation.<ProfileName>` (e.g., `Navigation.Responsive`)

2. **`mdl/catalog/tables.go`** — Optionally add a `navigation_profiles` table:
   ```sql
   create table if not exists navigation_profiles (
       Id text primary key,
       Name text,
       Kind text,
       IsNative integer,
       HomePage text,
       LoginPage text,
       MenuItemCount integer
   );
   ```
   This is optional — the `refs` table alone may be sufficient.

3. **`mdl/catalog/builder.go`** — Call `buildNavigationReferences()` during full catalog build.

### Phase 3: Write support — IMPLEMENTED

Implemented in `sdk/mpr/writer_navigation.go` with full BSON serialization:
- `Texts$text` → `Texts$Translation` objects for menu captions
- `Forms$FormAction` → `Forms$FormSettings` wrapping for menu page actions
- `Forms$MicroflowAction` → `Forms$MicroflowSettings` for menu microflow actions
- `Forms$NoAction` for sub-menu containers
- All embedded objects include `$ID` (binary UUID)
- `FormSettings` includes required `ParameterMappings` and `TitleOverride` fields
- `MenuItem` includes required `AlternativeText` and `icon` fields
- Supports both web profiles (`navigation$NavigationProfile`) and native profiles (`navigation$NativeNavigationProfile`)

## Complexity Estimate

| Phase | Files | Lines | Effort |
|---|---|---|---|
| Phase 1 (parser + SHOW) | 7 files | ~400 lines | Medium — mostly BSON path walking | **DONE** |
| Phase 2 (catalog refs) | 3 files | ~100 lines | Small — follows existing ref pattern | **DONE** |
| Phase 3 (CREATE OR REPLACE) | 8 files | ~600+ lines | Large — BSON serialization + grammar | **DONE** |

**All phases implemented.** Navigation support includes read (SHOW/DESCRIBE), catalog integration (refs table), and write (CREATE OR REPLACE NAVIGATION).

## Verification

After Phase 1+2, verify with:

```bash
# Parse navigation correctly
./bin/mxcli -p QueryDemoApp.mpr -c "show navigation"
# Expected: Responsive profile, home: Main.Home_Web, 3 menu items

# catalog has navigation refs
./bin/mxcli -p QueryDemoApp.mpr -c "refresh catalog full; select * from CATALOG.REFS where RefKind like '%home%' or RefKind like '%menu%'"
# Expected: home_page and menu_item refs

# impact analysis includes navigation
./bin/mxcli -p QueryDemoApp.mpr -c "refresh catalog full; show impact of Main.Home_Web"
# Expected: includes "Navigation.Responsive (home_page, menu_item)"

# context assembly includes entry points
./bin/mxcli -p QueryDemoApp.mpr -c "refresh catalog full; show context of Main.Home_Web"
# Expected: "navigation entry point: Responsive (home page, menu item 'Home')"
```
