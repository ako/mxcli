# Navigation Profiles

A navigation profile defines the navigation structure for a specific device type. Each profile has a default home page, optional role-specific home pages, an optional login page, and a menu.

## Profile Types

| Profile | MDL Name | Description |
|---------|----------|-------------|
| Responsive web | `Responsive` | Default browser navigation |
| Tablet web | `Tablet` | Tablet-optimized browser navigation |
| Phone web | `Phone` | Phone-optimized browser navigation |
| Responsive web offline | `ResponsiveOffline` | Offline-capable (PWA) |
| Tablet web offline | `TabletOffline` | Offline-capable (PWA) |
| Phone web offline | `PhoneOffline` | Offline-capable (PWA) |
| Native mobile | `NativePhone` | React Native mobile navigation |

The web kinds are a closed set and the profile is **created** if the project
does not have it yet. The offline kinds are the online names plus `Offline`.

## CREATE OR REPLACE NAVIGATION

Replaces an entire navigation profile:

```sql
CREATE OR REPLACE NAVIGATION <Profile>
  HOME PAGE <Module>.<Page>
  [HOME PAGE <Module>.<Page> FOR <UserRole>]
  [LOGIN PAGE <Module>.<Page>]
  [NOT FOUND PAGE <Module>.<Page>]
  [MENU (
    <menu-items>
  )]
```

### Full Example

```sql
CREATE OR REPLACE NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminHome FOR Administrator
  LOGIN PAGE Administration.Login
  NOT FOUND PAGE MyModule.Custom404
  MENU (
    MENU ITEM 'Home' PAGE MyModule.Home_Web;
    MENU ITEM 'Orders' PAGE Shop.Order_Overview;
    MENU 'Administration' (
      MENU ITEM 'Users' PAGE Administration.Account_Overview;
      MENU ITEM 'Settings' PAGE MyModule.Settings;
    );
  );
```

### Minimal Example

A profile only requires a home page:

```sql
CREATE OR REPLACE NAVIGATION Phone
  HOME PAGE MyModule.Home_Phone;
```

## Offline Synchronization

An offline profile downloads **nothing** until its entities have a sync mode.
Without a `sync` block the app builds, routes and installs as a PWA — and shows
an empty screen. This is the most common way an offline profile looks broken
while every check passes.

```sql
create or replace navigation PhoneOffline
  home page MyModule.Mobile_Dashboard
  sync (
    sync MyModule.Setting online;
    sync MyModule.Order all;
    sync MyModule.Trip where [Distance > 0];
    sync MyModule.Audit never;
    sync MyModule.Lookup none;
    sync MyModule.Draft none preserve data;
  );
```

| Mode | Meaning |
|------|---------|
| `online` | fetched from the server, never held on the device |
| `all` | every object downloaded |
| `where [xpath]` | only the objects the XPath selects |
| `never` | not synchronized |
| `none` | not downloaded; anything already on the device is dropped |
| `none preserve data` | not downloaded; what is on the device stays |

The words are the values Mendix stores, not Studio Pro's captions — its
"All Objects" is `all` and its "By XPath" is `where`. Copying a caption gives a
parse error rather than a broken document.

See [ALTER NAVIGATION](../reference/navigation/alter-navigation.md#offline-synchronization)
for the full reference.

## DESCRIBE NAVIGATION

View the current navigation profile in MDL syntax:

```sql
-- All profiles
DESCRIBE NAVIGATION;

-- Single profile
DESCRIBE NAVIGATION Responsive;
```

The output is round-trippable -- you can copy it, modify it, and execute it as a `CREATE OR REPLACE NAVIGATION` statement.

## See Also

- [Navigation and Settings](./navigation.md) -- overview of navigation and settings
- [Home Pages and Menus](./home-pages.md) -- details on home page and menu configuration
- [Project Settings](./project-settings.md) -- runtime and configuration settings
