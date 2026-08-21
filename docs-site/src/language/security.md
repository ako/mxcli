# Security

Mendix applications use a layered security model with three levels: project security settings, role definitions, and access rules. MDL provides complete control over all three layers.

## Security Levels

The project security level determines how strictly the runtime enforces access rules.

| Level | MDL Keyword | Description |
|-------|-------------|-------------|
| Off | `OFF` | No security enforcement (development only) |
| Prototype | `PROTOTYPE` | Security enforced but incomplete configurations allowed |
| Production | `PRODUCTION` | Full enforcement, all access rules must be complete |

```sql
ALTER PROJECT SECURITY LEVEL PRODUCTION;
```

## Security Architecture

Security in Mendix is organized in layers:

1. **Module Roles** -- defined per module, they represent permissions within that module (e.g., `Shop.Admin`, `Shop.Viewer`)
2. **User Roles** -- project-level roles that aggregate module roles across modules (e.g., `AppAdmin` combines `Shop.Admin` and `System.Administrator`)
3. **Entity Access** -- CRUD permissions and XPath constraints per entity per module role
4. **Document Access** -- execute/view permissions on microflows, nanoflows, and pages per module role
5. **Demo Users** -- test accounts with assigned user roles for development

## Inspecting Security

MDL provides several commands for viewing the current security configuration:

```sql
-- Project-wide settings
SHOW PROJECT SECURITY;

-- Roles
SHOW MODULE ROLES;
SHOW MODULE ROLES IN Shop;
SHOW USER ROLES;

-- Access rules
SHOW ACCESS ON MICROFLOW Shop.ACT_ProcessOrder;
SHOW ACCESS ON PAGE Shop.Order_Edit;
SHOW ACCESS ON ENTITY Shop.Customer;
SHOW ACCESS ON Shop.Customer;         -- a bare name means the entity

-- Full matrix
SHOW SECURITY MATRIX;
SHOW SECURITY MATRIX IN Shop;

-- Demo users
SHOW DEMO USERS;
```

## Modifying Project Security

Toggle the security level and demo user visibility:

```sql
ALTER PROJECT SECURITY LEVEL PRODUCTION;
ALTER PROJECT SECURITY DEMO USERS ON;
ALTER PROJECT SECURITY DEMO USERS OFF;
```

## Guest (Anonymous) Access

Guest access is Studio Pro's "Anonymous users" setting: it lets people use part of
the app without signing in. It is a flag plus a user role, and the role carries
the weight -- **whatever it can read is public**.

```sql
-- The role anonymous visitors are given. System.User is what lets an
-- unauthenticated session exist at all.
CREATE USER ROLE Anonymous (Shop.Viewer, System.User);

ALTER PROJECT SECURITY GUEST ACCESS ON ROLE Anonymous;

-- Grant exactly what should be public, and nothing else.
GRANT Anonymous ON Shop.Product (read *);
```

Turning it off keeps the stored role, so switching it back on needs no `ROLE`
clause:

```sql
ALTER PROJECT SECURITY GUEST ACCESS OFF;
ALTER PROJECT SECURITY GUEST ACCESS ON;
```

### The role is mandatory

Mendix fails the build with **CE0133** -- *"No user role for anonymous users
selected even though the feature anonymous users is enabled"* -- when guest access
is on and no role is set. `GUEST ACCESS ON` is therefore refused unless a role is
given in the statement or already stored in the project:

```
Error: GUEST ACCESS ON requires a role: no anonymous user role is configured,
and Mendix rejects anonymous access without one (CE0133).
```

### mxcli validates the role; Mendix does not

An anonymous role that does not exist builds with **zero** extra errors in
Mendix -- the app is valid, and anonymous visitors simply get nothing. That makes
a typo invisible until someone opens the public page, so mxcli checks the name
against the project's user roles and refuses an unknown one:

```
Error: user role not found: Anonymus (project user roles: Administrator, User).
```

### Review what you made public

Lint rule **SEC004** flags a project with guest access enabled, because any
unconstrained `read *` granted to the anonymous role is readable by the whole
internet ([DIVD-2022-00019](https://csirt.divd.nl/cases/DIVD-2022-00019/)). Add an
XPath constraint to each anonymous grant, or do not grant it.

## See Also

- [Module Roles and User Roles](./roles.md) -- defining roles
- [Entity Access](./entity-access.md) -- CRUD permissions per entity
- [Document Access](./document-access.md) -- microflow, page, and nanoflow permissions
- [GRANT / REVOKE](./grant-revoke.md) -- granting and revoking permissions
- [Demo Users](./demo-users.md) -- creating test accounts
