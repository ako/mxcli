---
title: MDL Security Support
status: done
---

# Proposal: MDL Security Support

## Overview

This document proposes MDL syntax for managing Mendix security configuration. Security in Mendix spans three levels:

1. **Project Security** — Security level, admin credentials, application user roles, demo users
2. **Module Security** — Module roles (defined per module)
3. **Artifact Access** — Per-entity, per-microflow, and per-page access rules tied to module roles

## BSON Storage Reference

The following BSON types are involved:

| BSON `$type` | Containment | Purpose |
|---|---|---|
| `security$ProjectSecurity` | Project root | Security level, user roles, demo users, password policy |
| `security$UserRole` | Inside ProjectSecurity | Application-level user role combining module roles |
| `security$DemoUserImpl` | Inside ProjectSecurity | Demo user for testing |
| `security$ModuleSecurity` | Per module (`ModuleSecurity`) | Container for module roles |
| `security$ModuleRole` | Inside ModuleSecurity | Module-level role |
| `DomainModels$AccessRule` | Inside Entity | Entity access: create/delete, member rights, XPath |
| `DomainModels$MemberAccess` | Inside AccessRule | Per-attribute/association rights |
| `microflows$Microflow.AllowedModuleRoles` | Microflow field | Which module roles can execute |
| `microflows$Microflow.ApplyEntityAccess` | Microflow field | Whether entity access rules apply |
| `Forms$Page.AllowedModuleRoles` | Page field | Which module roles can view the page |

### Key BSON Structures

**Project Security:**
```json
{
  "$type": "security$ProjectSecurity",
  "SecurityLevel": "CheckEverything",  // CheckNothing | CheckFormsAndMicroflows | CheckEverything
  "AdminUserName": "MxAdmin",
  "AdminPassword": "1",
  "AdminUserRole": "Administrator",
  "CheckSecurity": true,
  "StrictMode": false,
  "StrictPageUrlCheck": true,
  "EnableDemoUsers": true,
  "EnableGuestAccess": false,
  "GuestUserRole": "",
  "UserRoles": [2, { "$type": "security$UserRole", "Name": "Administrator", "ModuleRoles": [1, "Administration.Administrator", "Shop.ShopUser", ...], "ManageAllRoles": true, ... }],
  "DemoUsers": [2, { "$type": "security$DemoUserImpl", "username": "demo_administrator", "password": "...", "UserRoles": [1, "Administrator"], "entity": "Administration.Account" }],
  "PasswordPolicySettings": { "MinimumLength": 1, "RequireDigit": false, "RequireMixedCase": false, "RequireSymbol": false }
}
```

**Module Security:**
```json
{
  "$type": "security$ModuleSecurity",
  "ModuleRoles": [3, { "$type": "security$ModuleRole", "Name": "ShopUser", "description": "" }]
}
```

**Entity Access Rule** (inside entity's `AccessRules` array):
```json
{
  "$type": "DomainModels$AccessRule",
  "AllowCreate": false,
  "AllowDelete": true,
  "AllowedModuleRoles": [1, "Shop.ShopUser"],
  "DefaultMemberAccessRights": "ReadWrite",
  "MemberAccesses": [3,
    { "$type": "DomainModels$MemberAccess", "AccessRights": "ReadWrite", "attribute": "Shop.Customer.FirstName", "association": "" },
    { "$type": "DomainModels$MemberAccess", "AccessRights": "ReadWrite", "association": "Shop.Order_Customer", "attribute": "" }
  ],
  "XPathConstraint": "",
  "XPathConstraintCaption": ""
}
```

**Microflow Access** (fields on `microflows$microflow`):
```json
{
  "AllowedModuleRoles": [1, "Shop.ShopUser"],
  "ApplyEntityAccess": false
}
```

**Page Access** (field on `Forms$page`):
```json
{
  "AllowedRoles": [1, "Shop.ShopUser"]
}
```

## Proposed MDL Syntax

### 1. Project Security Level

```sql
-- Set security level
alter project security level production;     -- CheckEverything
alter project security level prototype;      -- CheckFormsAndMicroflows
alter project security level off;            -- CheckNothing

-- Show current security level
show project security;
```

### 2. Module Roles

```sql
-- Create module roles
create module role Shop.ShopUser;
create module role Shop.ShopUser description 'Regular shop user with read access';
create module role Shop.Admin description 'Shop administrator';

-- Show module roles
show module roles;
show module roles in Shop;

-- Describe a module role (shows access rules across entities/microflows/pages)
describe module role Shop.ShopUser;

-- Drop a module role
drop module role Shop.Admin;
```

### 3. Application User Roles

```sql
-- Create user roles that combine module roles
create user role Administrator (
    Shop.ShopUser,
    Shop.Admin,
    Administration.Administrator,
    System.Administrator
)
manage all roles;

create user role user (
    Shop.ShopUser,
    Administration.User,
    System.User
);

create user role api (
    System.User,
    ShopSvc.Api
);

-- Show user roles
show user roles;
describe user role Administrator;

-- Modify user role
alter user role user add module roles (
    NewModule.NewRole
);

alter user role user remove module roles (
    OldModule.OldRole
);

-- Drop user role
drop user role api;
```

### 4. Entity Access Rules

Entity access rules define which module roles can create, read, write, and delete entity instances.

```sql
-- Grant access to an entity for a module role
grant Shop.ShopUser on Shop.Customer (
    create,
    delete,
    read *,           -- Read all attributes/associations
    write *           -- Write all attributes/associations
);

-- Fine-grained member access
grant Shop.ShopUser on Shop.Customer (
    read (FirstName, LastName, EmailAddress),
    write (FirstName, LastName),
    read association (Shop.Order_Customer)
);

-- With XPath constraint (row-level security)
grant Shop.ShopUser on Shop.Customer (
    read *,
    write (FirstName, LastName)
)
where '[%CurrentUser%/Shop.Customer_Account = System.owner]';

-- Multiple roles with same access
grant Shop.ShopUser, Shop.Admin on Shop.Order (
    create,
    delete,
    read *,
    write *
);

-- Revoke access
revoke Shop.ShopUser on Shop.Customer;

-- Show entity access
show access on Shop.Customer;
describe access on Shop.Customer;
```

### 5. Microflow Access

```sql
-- Grant module roles access to execute a microflow
grant execute on microflow Shop.ACT_CreateOrder to Shop.ShopUser;
grant execute on microflow Shop.ACT_CreateOrder to Shop.ShopUser, Shop.Admin;

-- Apply entity access (microflow respects entity-level security)
alter microflow Shop.ACT_CreateOrder apply entity access;
alter microflow Shop.ACT_CreateOrder NO entity access;

-- Revoke microflow access
revoke execute on microflow Shop.ACT_CreateOrder from Shop.ShopUser;

-- Show microflow access
show access on microflow Shop.ACT_CreateOrder;
```

### 6. Page Access

```sql
-- Grant module roles access to view a page
grant view on page Shop.CustomerOverview to Shop.ShopUser;
grant view on page Shop.CustomerOverview to Shop.ShopUser, Shop.Admin;

-- Revoke page access
revoke view on page Shop.CustomerOverview from Shop.ShopUser;

-- Show page access
show access on page Shop.CustomerOverview;
```

### 7. Demo Users

```sql
-- Create demo users (for development/testing)
create demo user 'demo_admin' password '1' (
    Administrator
);

create demo user 'demo_user' password '1' (
    user,
    api
);

-- Enable/disable demo users
alter project security demo users on;
alter project security demo users off;

-- Show demo users
show demo users;

-- Drop demo user
drop demo user 'demo_admin';
```

### 8. Inline Security in CREATE Statements

For convenience, security can be declared inline when creating entities, microflows, and pages:

```sql
-- Entity with inline access rules
create persistent entity Shop.Customer (
    FirstName: string(200),
    LastName: string(200),
    Email: string(200)
)
access (
    grant Shop.ShopUser (create, delete, read *, write *),
    grant Shop.Admin (create, delete, read *, write *)
        where '[%CurrentUser%/Shop.Customer_Account = System.owner]'
);

-- Microflow with inline access
create microflow Shop.ACT_CreateOrder ($Customer: Shop.Customer)
returns Shop.Order as $Order
access (Shop.ShopUser, Shop.Admin)
apply entity access
begin
    -- ...
end;

-- Page with inline access
create page Shop.CustomerOverview
(
    title: 'Customers',
    layout: Atlas_Core.Atlas_Default
)
access (Shop.ShopUser, Shop.Admin)
{
    -- widgets ...
};
```

### 9. Bulk Security Operations

```sql
-- Grant a role access to all microflows in a module
grant execute on all microflows in Shop to Shop.ShopUser;

-- Grant a role access to all pages in a module
grant view on all pages in Shop to Shop.ShopUser;

-- Grant default entity access for all entities in a module
grant Shop.ShopUser on all entities in Shop (
    read *
);
```

### 10. DESCRIBE and SHOW for Security Audit

```sql
-- Comprehensive security overview
show project security;

-- What can a module role do?
describe module role Shop.ShopUser;
-- Output:
--   Module Role: Shop.ShopUser
--   User Roles: Administrator, User
--   Entity Access:
--     Shop.Customer: CREATE, DELETE, READ *, WRITE *
--     Shop.Order: READ *, WRITE (Status)
--   Microflow Access:
--     Shop.ACT_CreateOrder: EXECUTE
--     Shop.ACT_DeleteOrder: EXECUTE
--   Page Access:
--     Shop.CustomerOverview: VIEW
--     Shop.OrderDetail: VIEW

-- What roles can access a specific entity?
show access on Shop.Customer;

-- Security matrix for a module
show security matrix in Shop;
```

## Implementation Phases

### Phase 1: Read Security (SHOW/DESCRIBE)

Read-only commands to inspect existing security configuration. Requires:

- Parse `security$ProjectSecurity` from MPR
- Parse `security$ModuleSecurity` from each module
- Read `AccessRules` from entities (already partially implemented in `parser_domainmodel.go`)
- Read `AllowedModuleRoles` from microflows
- Read `AllowedRoles` from pages
- New executor commands: `show project security`, `show module roles`, `show access on`, `describe module role`
- Add security data to the catalog for cross-referencing

**Existing code to build on:**
- `sdk/mpr/parser_domainmodel.go:parseAccessRule()` — already parses `AllowCreate`, `AllowDelete`, `XPathConstraint`, module role IDs
- `sdk/domainmodel/domainmodel.go` — `AccessRule` and `MemberAccess` structs defined
- `generated/metamodel/types.go` — `SecurityModuleRole`, `SecurityProjectSecurity`, etc. defined
- `generated/metamodel/enums.go` — `SecuritySecurityLevel` enum defined

**Implementation gaps:**
- No parsing for `security$ProjectSecurity` or `security$ModuleSecurity` units
- No parsing of `AllowedModuleRoles` on microflows (field exists in BSON but is not read)
- No parsing of `AllowedRoles` on pages
- Entity access rules are parsed but role IDs are not resolved to names
- Writer serializes `AccessRules: [3]` (empty) — access rules are lost on entity modification

### Phase 2: Module Roles & User Roles (CREATE/ALTER/DROP)

Write support for module and user role management:

- `create module role` — add role to `security$ModuleSecurity`
- `drop module role` — remove role (with impact check)
- `create user role` — add to `security$ProjectSecurity`
- `alter user role` — modify module role assignments
- `alter project security level` — change security level

### Phase 3: Access Rule Management (GRANT/REVOKE)

Write support for access control:

- `grant ... on entity` — add/modify `DomainModels$AccessRule` inside entity
- `revoke ... on entity` — remove access rule
- `grant execute on microflow` — set `AllowedModuleRoles` on microflow
- `grant view on page` — set `AllowedRoles` on page
- Inline `access (...)` in `create entity`, `create microflow`, `create page`
- Fix writer to preserve existing access rules when modifying entities

### Phase 4: Bulk Operations and Audit

- `grant ... on all microflows/pages/entities in module`
- `show security matrix`
- Catalog integration for security queries
- Lint rules for security (e.g., "microflow has no access rules", "entity accessible to all roles")

## Grammar Additions

New tokens for `MDLLexer.g4`:

```antlr
// security keywords
security: S E C U R I T Y;
role: R O L E;
roles: R O L E S;
grant: G R A N T;
revoke: R E V O K E;
execute: E X E C U T E;  // already exists
access: A C C E S S;
production: P R O D U C T I O N;
prototype: P R O T O T Y P E;
manage: M A N A G E;
demo: D E M O;
matrix: M A T R I X;
apply: A P P L Y;
```

New parser rules for `MDLParser.g4`:

```antlr
// security statements
securityStatement
    : createModuleRoleStmt
    | dropModuleRoleStmt
    | createUserRoleStmt
    | alterUserRoleStmt
    | dropUserRoleStmt
    | grantEntityAccessStmt
    | revokeEntityAccessStmt
    | grantMicroflowAccessStmt
    | revokeMicroflowAccessStmt
    | grantPageAccessStmt
    | revokePageAccessStmt
    | alterProjectSecurityStmt
    | showProjectSecurityStmt
    | showModuleRolesStmt
    | showAccessStmt
    | describeModuleRoleStmt
    | createDemoUserStmt
    | dropDemoUserStmt
    ;

createModuleRoleStmt
    : create module role qualifiedName (description STRING_LITERAL)?
    ;

grantEntityAccessStmt
    : grant moduleRoleList on qualifiedName LPAREN entityAccessRights RPAREN
      (where STRING_LITERAL)?
    ;

entityAccessRights
    : entityAccessRight (COMMA entityAccessRight)*
    ;

entityAccessRight
    : create
    | delete
    | read STAR
    | read LPAREN identifierList RPAREN
    | write STAR
    | write LPAREN identifierList RPAREN
    | read association LPAREN qualifiedNameList RPAREN
    ;
```

## Design Decisions

### SQL-style GRANT/REVOKE

The `grant`/`revoke` syntax was chosen because:
1. It's familiar to anyone who knows SQL security
2. It maps cleanly to the Mendix access model (roles, permissions, objects)
3. It reads naturally: "GRANT ShopUser ON Customer (READ *, WRITE *)"
4. It's the standard way to express authorization in declarative languages

### Inline ACCESS Clause

The inline `access (...)` clause on CREATE statements enables one-shot provisioning:
- Define and secure an entity in one statement
- No separate GRANT needed for new entities
- Consistent with how Studio Pro presents security alongside entity configuration

### Module Roles as First-Class Citizens

Module roles are the fundamental unit of access in Mendix:
- Entity access rules reference module roles
- Microflow access references module roles
- Page access references module roles
- User roles are composites of module roles

MDL reflects this by making `describe module role` the primary audit tool.

### XPath Constraints as Strings

Entity access XPath constraints are kept as string literals (`where '...'`) rather than parsed. This matches how Mendix stores them and avoids needing an XPath parser in MDL.

## Example: Complete Security Setup

```sql
-- 1. Set security level
alter project security level production;

-- 2. Create module roles
create module role Shop.Customer description 'Customer self-service access';
create module role Shop.Employee description 'Shop employee with full access';
create module role Shop.Manager description 'Shop manager with admin access';

-- 3. Create user roles
create user role Customer (
    Shop.Customer,
    System.User
);

create user role Employee (
    Shop.Customer,
    Shop.Employee,
    Administration.User,
    System.User
);

create user role Manager (
    Shop.Customer,
    Shop.Employee,
    Shop.Manager,
    Administration.Administrator,
    System.Administrator
)
manage all roles;

-- 4. Entity access
grant Shop.Customer on Shop.Order (
    create,
    read *,
    write (status, Notes)
)
where '[%CurrentUser%/Shop.Order_Customer = System.owner]';

grant Shop.Employee on Shop.Order (
    create,
    delete,
    read *,
    write *
);

grant Shop.Manager on Shop.Order (
    create,
    delete,
    read *,
    write *
);

-- 5. Microflow access
grant execute on microflow Shop.ACT_PlaceOrder to Shop.Customer, Shop.Employee;
grant execute on microflow Shop.ACT_CancelOrder to Shop.Employee, Shop.Manager;
grant execute on microflow Shop.ACT_DeleteOrder to Shop.Manager;

-- 6. Page access
grant view on page Shop.MyOrders to Shop.Customer;
grant view on page Shop.AllOrders to Shop.Employee, Shop.Manager;
grant view on page Shop.AdminDashboard to Shop.Manager;

-- 7. Demo users
create demo user 'demo_customer' password 'customer1' (Customer);
create demo user 'demo_employee' password 'employee1' (Employee);
create demo user 'demo_manager' password 'manager1' (Manager);
alter project security demo users on;
```
