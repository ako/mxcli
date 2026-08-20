# Security Statements

Statements for managing project security: module roles, user roles, entity access rules, microflow and page access, demo users, and project-level security settings.

Mendix security operates at two levels. **Module roles** define permissions within a single module (entity access, microflow execution, page visibility). **User roles** aggregate module roles into project-wide identities assigned to end users.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE MODULE ROLE](create-module-role.md) | Create a role within a module |
| [CREATE USER ROLE](create-user-role.md) | Create a project-level user role aggregating module roles |
| [GRANT](grant.md) | Grant entity, microflow, page, or nanoflow access to roles |
| [REVOKE](revoke.md) | Remove previously granted access |
| [CREATE DEMO USER](create-demo-user.md) | Create a demo user for development and testing |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Show project security | `SHOW PROJECT SECURITY` |
| Show module roles | `SHOW MODULE ROLES [IN module]` |
| Show user roles | `SHOW USER ROLES` |
| Show demo users | `SHOW DEMO USERS` |
| Show access on element | `SHOW ACCESS ON [ENTITY\|MICROFLOW\|PAGE\|NANOFLOW] module.Name` |
| Show security matrix | `SHOW SECURITY MATRIX [IN module]` |
| Alter project security level | `ALTER PROJECT SECURITY LEVEL OFF\|PROTOTYPE\|PRODUCTION` |
| Toggle demo users | `ALTER PROJECT SECURITY DEMO USERS ON\|OFF` |
| Drop module role | `DROP MODULE ROLE module.Role` |
| Drop user role | `DROP USER ROLE Name` |
| Drop demo user | `DROP DEMO USER 'username'` |
| Alter user role | `ALTER USER ROLE Name ADD\|REMOVE MODULE ROLES (module.Role, ...)` |
