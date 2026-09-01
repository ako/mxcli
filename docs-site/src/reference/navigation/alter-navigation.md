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
| `NativePhone` | Native mobile application |

### Menu Items

Menu items form a hierarchy. Top-level items appear in the main navigation bar. Nested submenus are created with the `MENU 'label' ( ... )` syntax.

Each `MENU ITEM` specifies a label and a target page. Menu items are terminated with semicolons.

## Parameters

`profile`
:   The navigation profile type: `Responsive`, `Tablet`, `Phone`, or `NativePhone`.

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
