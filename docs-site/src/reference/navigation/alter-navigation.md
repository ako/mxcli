# ALTER NAVIGATION

## Synopsis

```sql
CREATE OR REPLACE NAVIGATION profile
    HOME PAGE module.PageName
    [ HOME PAGE module.PageName FOR UserRole ]
    [ LOGIN PAGE module.PageName ]
    [ NOT FOUND PAGE module.PageName ]
    [ MENU (
        menu_items
    ) ]
    [ SYNC (
        sync_rules
    ) ]
```

## Description

Creates or replaces a navigation profile. Each profile defines the home page, optional role-specific home pages, optional login page, optional 404 page, and a hierarchical menu structure.

The statement fully replaces the existing profile configuration. To modify only part of a profile's navigation, use DESCRIBE NAVIGATION to export the current configuration, edit the MDL, and re-execute.

### Profile Types

Mendix supports the following navigation profile types:

| Profile | Description |
|---------|-------------|
| `Responsive` | Web browser (desktop and mobile responsive) |
| `Tablet` | Tablet-optimized web |
| `Phone` | Phone-optimized web |
| `ResponsiveOffline` | Responsive web, offline-capable (PWA) |
| `TabletOffline` | Tablet web, offline-capable (PWA) |
| `PhoneOffline` | Phone web, offline-capable (PWA) |
| `NativePhone` | Native mobile application |

The web kinds are a closed set, and the profile is **created** if the project
does not have it yet. The three offline kinds are the online names plus
`Offline`, and each one needs a [`SYNC` block](#offline-synchronization) to
download anything.

### Menu Items

Menu items form a hierarchy. Top-level items appear in the main navigation bar. Nested submenus are created with the `MENU 'label' ( ... )` syntax.

Each `MENU ITEM` specifies a label and a target page. Menu items are terminated with semicolons.

## Parameters

`profile`
:   The navigation profile type — see [Profile Types](#profile-types). One of
    `Responsive`, `Tablet`, `Phone`, `ResponsiveOffline`, `TabletOffline`,
    `PhoneOffline` or `NativePhone`.

`HOME PAGE module.PageName`
:   The default home page for the profile. Required. The page must already exist.

`HOME PAGE module.PageName FOR UserRole`
:   Optional role-specific home page. Users with this role see a different home page than the default. Multiple role-specific home pages can be specified.

    The role is a **user role**, written as a bare name (`FOR Administrator`). User roles are project-level and have no module part. A module-qualified name here — a module role, which often shares the name — produces a project Mendix cannot load: `StorageLoadException: … is not a valid UserRoleIdentifier`, raised before checking runs. `mxcli check --references` refuses it.

`LOGIN PAGE module.PageName`
:   Optional custom login page. If omitted, the default system login page is used.

`NOT FOUND PAGE module.PageName`
:   Optional custom 404 page shown when a requested page is not found.

`MENU ( menu_items )`
:   Optional menu structure. Contains `MENU ITEM` and nested `MENU` entries.

`MENU ITEM 'label' PAGE module.PageName`
:   A leaf menu item that navigates to a page.

`MENU 'label' ( ... )`
:   A submenu containing nested menu items and/or further submenus.

### Offline Synchronization

`SYNC ( ... )` configures which entities an offline profile downloads. **An
offline profile downloads nothing without it** — the app builds, routes and
installs as a PWA, and shows an empty screen.

```sql
SYNC (
    SYNC Sales.Setting ONLINE;
    SYNC Sales.Order ALL;
    SYNC Sales.Trip WHERE [Distance > 0];
    SYNC Sales.Audit NEVER;
    SYNC Sales.Lookup NONE;
    SYNC Sales.Draft NONE PRESERVE DATA;
)
```

| Mode | Meaning |
|------|---------|
| `ONLINE` | Fetched from the server; never held on the device |
| `ALL` | Every object downloaded |
| `WHERE [ xpath ]` | Only the objects the XPath selects |
| `NEVER` | Not synchronized |
| `NONE` | Not downloaded; anything already on the device is dropped |
| `NONE PRESERVE DATA` | Not downloaded; what is on the device stays |

These are the values Mendix stores, **not** the captions Studio Pro shows in
its *Customize offline synchronization* dialog: its "All Objects" is `ALL` and
its "By XPath" is `WHERE`. A caption is refused rather than written.

`WHERE` implies the constrained mode rather than naming it, so a constraint
without a mode and a mode without a constraint are both unspellable. The XPath
goes in **brackets** and is taken verbatim — nothing inside is escaped. A
quoted `WHERE 'xpath'` still parses, but every quote inside it must be doubled.

The block replaces the stored list, the way `MENU` replaces the menu. Omitting
it leaves the stored configuration alone.

An entity's *compatibility mode* flag has no MDL syntax. It is read, preserved
across a rewrite, and reported by `DESCRIBE NAVIGATION` — never silently
dropped.

## Examples

Minimal navigation with just a home page:

```sql
CREATE OR REPLACE NAVIGATION Responsive
    HOME PAGE MyModule.Home_Web;
```

Full navigation with role-specific homes and menus:

```sql
CREATE OR REPLACE NAVIGATION Responsive
    HOME PAGE MyModule.Home_Web
    HOME PAGE MyModule.AdminHome FOR Administrator
    LOGIN PAGE Administration.Login
    NOT FOUND PAGE MyModule.Custom404
    MENU (
        MENU ITEM 'Home' PAGE MyModule.Home_Web;
        MENU 'Admin' (
            MENU ITEM 'Users' PAGE Administration.Account_Overview;
            MENU ITEM 'Settings' PAGE MyModule.Settings;
        );
        MENU ITEM 'About' PAGE MyModule.About;
    );
```

Native mobile navigation:

```sql
CREATE OR REPLACE NAVIGATION NativePhone
    HOME PAGE Mobile.Dashboard
    MENU (
        MENU ITEM 'Home' PAGE Mobile.Dashboard;
        MENU ITEM 'Tasks' PAGE Mobile.TaskList;
        MENU ITEM 'Profile' PAGE Mobile.UserProfile;
    );
```

## See Also

[SHOW NAVIGATION](show-navigation.md)
