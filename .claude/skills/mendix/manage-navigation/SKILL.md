---
name: manage-navigation
description: "Inspect and change navigation profiles in MDL — home pages, menus, login and not-found pages, and role-based routing. Use when asked to change where the app opens, restructure the menu, or route roles to different home pages."
---

# Navigation Management Skill

This skill covers inspecting and modifying Mendix navigation profiles via MDL: home pages, menu items, login pages, role-based routing, and navigation catalog queries.

## When to Use This Skill

Use when the user asks to:
- View or change navigation home pages
- View or modify the navigation menu structure
- Set login or not-found pages
- Configure role-based home page routing
- Discover which pages are navigation entry points
- Set up navigation for a new project

## Navigation Concepts

- **Navigation Profiles** — Every Mendix project has navigation profiles: Responsive, Phone, Tablet, and optionally Native. Each profile has its own home page, menu, and login page. The web kinds are a **closed set** and `create or replace navigation` will create one that does not exist yet:

  | Online | Offline twin |
  |---|---|
  | `Responsive` | `ResponsiveOffline` |
  | `Phone` | `PhoneOffline` |
  | `Tablet` | `TabletOffline` |

  An invented name (`Mobile`, `Desktop`) is an error, not a new profile: Mendix routes on **User-Agent** to its own kinds, so a profile the platform does not define can never be reached. Native profiles are a different document type and are not creatable from MDL.
- **Offline Profiles** — An offline profile makes the app work without a connection (a PWA with a local database that syncs). It is not a different document — same properties as its online twin — but it **constrains every page it can reach**, see below.
- **Home Page** — The default page shown after login. Can be a PAGE or MICROFLOW.
- **Role-Based Home Pages** — Override the default home page per user role (e.g., admins see a dashboard, users see a task list).
- **Menu Items** — Hierarchical menu tree. Each item has a caption and optionally targets a PAGE or MICROFLOW. Sub-menus nest with `menu 'caption' (...)`.
- **Menu Documents** — A *separate* document type (`Menus$MenuDocument`) holding a reusable menu that a menu widget points at, e.g. Atlas_Core's `Phone_Menu`. Not the same thing as a profile's menu, though both are built from the same items, so the item syntax is identical. Managed with `create/describe/drop menu` — see below.
- **Login Page** — Custom login page (optional; Mendix provides a default).
- **Not-Found Page** — Custom 404 page (optional).

## Show Commands (Read-Only)

```sql
-- Summary of all navigation profiles (home pages, menu counts)
show navigation;

-- Full MDL description of a profile (round-trippable output)
describe navigation Responsive;
describe navigation;              -- all profiles

-- Menu tree for a specific profile
show navigation menu Responsive;
show navigation menu;             -- all profiles

-- Home page assignments across all profiles and roles
show navigation homes;
```

## CREATE OR REPLACE NAVIGATION (Full Replacement)

This command fully replaces a navigation profile's configuration. All clauses are optional — omitted clauses clear that section. The output from `describe navigation` can be pasted back directly.

### Basic: Set Home and Login Page

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login;
```

### Role-Based Home Pages

Add `for Module.Role` to override the home page for specific user roles:

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  home page MyModule.AdminDashboard for Administration.Administrator
  home page MyModule.CustomerPortal for MyModule.Customer
  login page Administration.Login;
```

### Full Menu Tree

The `menu (...)` block replaces the entire menu. Use `menu item` for leaf items and `menu 'caption' (...)` for sub-menus:

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  menu (
    menu item 'Home' page MyModule.Home_Web;
    menu 'Orders' (
      menu item 'All Orders' page Orders.Order_Overview;
      menu item 'New Order' page Orders.Order_New;
    );
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
      menu item 'Run Report' microflow Reports.ACT_GenerateReport;
    );
  );
```

### Menu Icons

Both `menu item` and `menu 'caption' (...)` take an optional `icon`. It is a
**qualified name** into an **icon collection** — a model reference, written like
every other reference in MDL, not a string:

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  menu (
    menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;
    menu 'Orders' icon Atlas_Core.Atlas."shopping-cart" (
      menu item 'All Orders' page Orders.Order_Overview
        icon Atlas_Core.Atlas."list-bullets";
    );
  );
```

**Hyphenated names are double-quoted** (`Atlas_Core.Atlas."align-center"`) —
a hyphen does not lex as an identifier, so the segment is quoted exactly as a
keyword-colliding name would be. Plain names need no quotes, including ones that
happen to be MDL keywords (`home`, `user`, `add`). Atlas_Core ships three
collections: `Atlas` (outline), `Atlas_Filled`, and `Atlas_Styling`. List what
is actually available in your project rather than guessing a name:

```sql
show icon collection;
describe icon collection Atlas_Core.Atlas;
```

**Only the icon-collection form is writable.** Studio Pro can also attach a
*glyph* icon (a numeric character code) or an *image* icon (pointing into an
image collection). Those are different elements with different fields, so MDL
does not write them — and `describe navigation` reports them as a comment rather
than emitting an `icon` clause that would silently convert one into the other on
replay:

```
menu item 'Close' page MyModule.Close;
-- icon System.Images.Close (Forms$ImageIcon) is not reproducible by CREATE NAVIGATION; set it in Studio Pro
```

### Clear the Menu

An empty `menu ()` block removes all menu items:

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  menu ();
```

### Not-Found Page

```sql
create or replace navigation Responsive
  home page MyModule.Home_Web
  not found page MyModule.Custom404;
```

### Microflow as Home Page

Use `home microflow` instead of `home page` to run a microflow on login:

```sql
create or replace navigation Responsive
  home microflow MyModule.ACT_ShowHome;
```

## Round-Trip Workflow

The DESCRIBE output is directly executable. Use this pattern to inspect, modify, and re-apply:

```sql
-- Step 1: Inspect current state
describe navigation Responsive;

-- Step 2: Copy the output, modify as needed, paste back
create or replace navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  menu (
    menu item 'Home' page MyModule.Home_Web;
    menu item 'New Feature' page MyModule.NewFeature;
  );

-- Step 3: Verify
describe navigation Responsive;
```

## Catalog Queries

After `refresh catalog full`, navigation references appear in the `REFS` table:

```sql
refresh catalog full;

-- Find all pages that are navigation entry points
select SourceName, TargetName, RefKind
from CATALOG.REFS
where RefKind in ('home_page', 'menu_item', 'login_page');

-- What references point to a specific page?
show references to MyModule.Home_Web;

-- Impact analysis: what breaks if I change this page?
show impact of MyModule.Home_Web;

-- Full context for a page (includes navigation references)
show context of MyModule.Home_Web;
```

## Common Patterns

### New Project Setup

Set up navigation for a freshly created project:

```sql
-- Create home page
create page MyModule.Home_Web
(
  title: 'Home',
  layout: Atlas_Core.Atlas_Default
)
{
  container ctnMain {
    dynamictext txtWelcome (content: 'Welcome!')
  }
}

-- Configure navigation
create or replace navigation Responsive
  home page MyModule.Home_Web
  menu (
    menu item 'Home' page MyModule.Home_Web;
  );
```

### Adding a New Page to Navigation

After creating a new page, add it to the menu:

```sql
-- First inspect current menu
describe navigation Responsive;

-- Then re-apply with the new item added (copy existing + add new)
create or replace navigation Responsive
  home page MyModule.Home_Web
  login page Administration.Login
  menu (
    menu item 'Home' page MyModule.Home_Web;
    menu item 'Customers' page MyModule.Customer_Overview;  -- new
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
    );
  );
```

## Menu Documents (standalone, reusable)

A profile menu lives *inside* a navigation profile and is edited with
`create or replace navigation`. A **menu document** is its own document, and a
menu widget on a page points at it. Atlas_Core ships `Phone_Menu` and
`Tablet_Menu`.

Tell them apart by which command reads them:

```sql
show navigation menu;                    -- the menu inside each profile
describe menu Atlas_Core.Phone_Menu;     -- a standalone menu document
```

Menu documents use the same item syntax as the profile `menu (...)` block:

```sql
create or modify menu MyModule.Main_Menu (
  menu item 'Home' page MyModule.Home_Web icon Atlas_Core.Atlas.home;
  menu item 'Run' microflow MyModule.DoThing;
  menu 'Admin' (
    menu item 'Accounts' page Administration.Account_Overview;
  );
  menu item 'Plain';
);

drop menu MyModule.Main_Menu;
```

`describe menu` emits a re-executable `create or modify` statement, so
describe → edit → exec is the normal editing loop.

**`or modify` replaces the whole item list.** An omitted item is a removed item,
exactly as with `create or replace navigation`. The document's identity and
export level are preserved, so menu widgets pointing at it keep working.

### Gotchas

- **A menu item cannot open a page that takes a required parameter.** There is
  nowhere to supply the argument, and Mendix reports **CE1571** ("No argument has
  been selected for parameter …") against `Menu item`. Point the item at a
  parameterless page, or call a microflow that opens the page.
- **Only icon-collection icons round-trip.** A glyph icon (numeric code) or an
  image icon cannot be written by MDL; `describe` flags those on their own
  comment line rather than dropping them silently, so re-running the output
  loses that icon visibly.
- **Authoring needs the default engine.** Under `MXCLI_ENGINE=legacy`,
  create/modify/drop refuse rather than writing a differently-shaped document.

## Offline Profiles

An offline profile is created the same way as any other:

```sql
create or replace navigation TabletOffline
  home page Maintenance.Request_Overview
  menu (
    menu item 'Requests' page Maintenance.Request_Overview;
  );
```

Two things about it are worth knowing before you do.

**It breaks pages the statement never mentions.** Mendix restricts every page an
offline profile can reach: an attribute may be bound across **at most one
association hop**. A longer path is **CE6206** — *"Attribute paths with multiple
steps cannot be used on pages that are accessible through an offline-based
navigation."*

```
Request_Asset/AssetName                one hop    fine
Request_Asset/Asset_Site/SiteName      two hops   CE6206
```

Those pages were valid before. Adding the profile is what invalidated them, and
the build fails somewhere with no obvious connection to the statement you ran.
mxcli reports them when it creates the profile:

```
Navigation profile 'TabletOffline' created.

Warning: TabletOffline is an offline profile, and 3 documents in this project bind
an attribute across more than one association.
Mendix rejects a multi-step attribute path with CE6206 on any page reachable from
an offline profile — one hop is allowed, two are not.
  page Maintenance.Request_Overview: Maintenance.Site.SiteName (2 steps)
  ...
```

It is a **warning**, not a refusal: whether a page is actually *reachable* from
the profile takes the whole page graph, which mxcli does not walk. The fix is
either to keep the page off the offline profile, or to bring the value one hop
closer — add an attribute to the intermediate entity and keep it in step.

**The synchronization config is derived, not authored.** `offlineEntityConfigs`
starts empty and Studio Pro fills it from the entities the reachable pages use;
only rows that differ from the defaults (`syncMode: Online`, no constraint) are
stored. MDL does not author per-entity sync modes — set those in Studio Pro.

## Checklist

- [ ] Profile name is one of Mendix's web kinds (`Responsive`/`Phone`/`Tablet`, or their `*Offline` twins) — an invented name is an error, not a new profile
- [ ] For an **offline** profile, no page it can reach binds an attribute across more than one association (CE6206)
- [ ] All PAGE/MICROFLOW targets are fully qualified (`Module.Name`)
- [ ] Role references in `for` clauses are fully qualified (`Module.Role`)
- [ ] Every `menu item` and `menu 'caption' (...)` ends with `;`
- [ ] Sub-menu items are wrapped in `menu 'caption' ( ... );`
- [ ] `icon` is a qualified name (not a string); hyphenated segments are double-quoted
- [ ] The icon exists — check with `describe icon collection Module.Name`, do not guess
- [ ] Use `describe navigation` to verify changes after applying
- [ ] For a **menu document**, confirm you want `create menu` and not a profile menu — `show navigation menu` vs `describe menu` tells them apart
- [ ] No menu item targets a page with required parameters (CE1571)
