# Navigation Statements

Statements for configuring application navigation profiles, home pages, login pages, and menu structures.

Mendix applications can have multiple navigation profiles (Responsive, Tablet, Phone, NativePhone) that each define a home page, optional login page, optional 404 page, and a hierarchical menu tree. Navigation profiles control which page the user sees first and how they navigate through the application.

## Statements

| Statement | Description |
|-----------|-------------|
| [ALTER NAVIGATION](alter-navigation.md) | Create or replace a navigation profile with home pages, login page, and menus |
| [SHOW NAVIGATION](show-navigation.md) | Display navigation profiles, menus, and home page assignments |
| [CREATE MENU](menu.md) | Create, describe and drop standalone menu documents (`Menus$MenuDocument`) |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Describe navigation | `DESCRIBE NAVIGATION [profile]` |
