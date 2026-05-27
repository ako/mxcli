---
title: Feature Request: Entity Access Rules in Starlark Lint API
status: proposed
---

# Feature Request: Entity Access Rules in Starlark Lint API

## Background

The `CATALOG.PERMISSIONS` table (added in a recent mxcli release) already contains full entity access rule data — including module role, access type (CREATE/READ/WRITE/DELETE/MEMBER_READ/MEMBER_WRITE), and XPath constraints. This data is queryable via catalog SQL but is not yet exposed to the Starlark lint rule API in `.claude/lint-rules/*.star`.

This gap prevents writing lint rules for the most security-critical class of Mendix misconfiguration: entity-level data access controls — specifically the pattern behind DIVD-2022-00019.

The raw data shape from the catalog is:

```
ModuleRoleName               | ElementName            | MemberName                       | AccessType   | XPathConstraint
-----------------------------+------------------------+----------------------------------+--------------+------------------
Administration.Administrator | Administration.Account |                                  | read         |
Administration.User          | Administration.Account |                                  | read         | [id='[%CurrentUser%]']
Administration.Administrator | Administration.Account | Administration.Account.FullName  | MEMBER_READ  |
Administration.User          | Administration.Account | Administration.Account.Email     | MEMBER_READ  |
Shop.ShopUser                | Shop.Customer          |                                  | read         |
Shop.ShopUser                | Shop.Payment           |                                  | read         |
```

---

## Requested API Additions

### 1. `permissions_for(entity_qualified_name)` → list of `entity_permission`

Returns all access rule records for a single entity. This is the primary function needed. Scoped per entity so rule authors can iterate alongside `entities()` without loading the full permission set upfront.

```python
for e in entities():
    for perm in permissions_for(e.qualified_name):
        if perm.access_type == "read" and perm.xpath_constraint == "":
            # unconstrained read
```

### 2. `entity_permission` object

Each record returned by `permissions_for()` should have:

| Property | Type | Example | Notes |
|----------|------|---------|-------|
| `module_role_name` | string | `"Shop.ShopUser"` | Qualified module role |
| `module_name` | string | `"Shop"` | Module of the role |
| `entity_name` | string | `"Shop.Customer"` | Qualified entity name |
| `access_type` | string | `"read"` | `create`, `read`, `write`, `delete`, `MEMBER_READ`, `MEMBER_WRITE` |
| `member_name` | string | `"Shop.Customer.Email"` | Populated for `MEMBER_READ`/`MEMBER_WRITE`, empty for entity-level |
| `xpath_constraint` | string | `"[%CurrentUser%/..."` | Empty string means unconstrained (all rows) |
| `is_constrained` | bool | `false` | Convenience: `true` when `xpath_constraint != ""` |

### 3. `user_roles()` → list of `user_role`

Returns all user roles in the project, with their module role assignments and anonymous flag. This is needed to trace which module roles are active for anonymous/guest users — the key check for DIVD-2022-00019.

```python
for ur in user_roles():
    if ur.is_anonymous:
        for mr in ur.module_roles:
            # check what this anonymous role can do
```

#### `user_role` object

| Property | Type | Example | Notes |
|----------|------|---------|-------|
| `name` | string | `"Anonymous"` | User role name |
| `is_anonymous` | bool | `true` | Whether this is the anonymous/guest user role |
| `module_roles` | list of string | `["Shop.ShopUser", "Main.User"]` | Qualified module role names assigned |

### 4. Extend `project_security` with `anonymous_user_role`

Add one property to the existing `project_security` object:

| Property | Type | Description |
|----------|------|-------------|
| `anonymous_user_role` | string or None | Name of the anonymous user role, or `none` if guest access is disabled |

---

## Rules This Unlocks

With these additions, the following security rules become writable in Starlark:

### Rule A: Unconstrained entity READ (direct DIVD-2022-00019 detection)

```python
RULE_ID = "SEC007"
RULE_NAME = "UnconstrainedAnonymousEntityRead"
description = "entity readable by anonymous users without row-level xpath constraint"
CATEGORY = "security"
SEVERITY = "error"

def check():
    sec = project_security()
    if sec == none or not sec.enable_guest_access:
        return []

    # find which module roles belong to the anonymous user role
    anon_module_roles = {}
    for ur in user_roles():
        if ur.is_anonymous:
            for mr in ur.module_roles:
                anon_module_roles[mr] = true

    violations = []
    for e in entities():
        if e.entity_type != "persistent" or e.is_external:
            continue
        for perm in permissions_for(e.qualified_name):
            if perm.access_type == "read" and not perm.is_constrained:
                if perm.module_role_name in anon_module_roles:
                    violations.append(violation(
                        message="entity '{}' is readable by anonymous users (via role '{}') with no xpath constraint — all rows exposed to unauthenticated users. (DIVD-2022-00019)".format(
                            e.qualified_name, perm.module_role_name
                        ),
                        location=location(
                            module=e.module_name,
                            document_type="entity",
                            document_name=e.qualified_name,
                        ),
                        suggestion="add an xpath constraint to the access rule for '{}', or remove the grant if this data should not be public.".format(
                            perm.module_role_name
                        ),
                    ))
    return violations
```

### Rule B: All authenticated roles with unconstrained PII reads

```python
RULE_ID = "SEC008"
RULE_NAME = "UnconstrainedPiiRead"
description = "roles can read all rows of entities containing PII without xpath scoping"
CATEGORY = "security"
SEVERITY = "warning"

PII_PATTERNS = ["email", "password", "creditcard", "dateofbirth", "ssn", "phonenumber", "iban"]

def check():
    violations = []
    for e in entities():
        if e.entity_type != "persistent" or e.is_external:
            continue

        # check if any attribute looks like PII
        pii_attrs = [a.name for a in attributes_for(e.qualified_name)
                     if any(p in a.name.lower() for p in PII_PATTERNS)]
        if len(pii_attrs) == 0:
            continue

        # check for any unconstrained read grant
        unconstrained_roles = [
            perm.module_role_name
            for perm in permissions_for(e.qualified_name)
            if perm.access_type == "read" and not perm.is_constrained
        ]
        if len(unconstrained_roles) > 0:
            violations.append(violation(
                message="entity '{}' contains PII attributes ({}) and is readable without xpath constraints by: {}".format(
                    e.qualified_name,
                    ", ".join(pii_attrs),
                    ", ".join(unconstrained_roles),
                ),
                location=location(
                    module=e.module_name,
                    document_type="entity",
                    document_name=e.qualified_name,
                ),
                suggestion="add xpath constraints to scope access to the current user's own data, e.g. [Sales.Order_Customer/Sales.Customer/id = '[%CurrentUser%]']",
            ))
    return violations
```

### Rule C: Attribute-level member permissions inconsistent with entity-level

```python
RULE_ID = "SEC009"
RULE_NAME = "MissingMemberReadRestriction"
description = "roles with entity read access should have explicit member-level read grants if attribute restriction is intended"
CATEGORY = "security"
SEVERITY = "info"

def check():
    violations = []
    for e in entities():
        if e.entity_type != "persistent" or e.is_external:
            continue

        entity_readers = set()
        member_readers = set()

        for perm in permissions_for(e.qualified_name):
            if perm.access_type == "read" and perm.member_name == "":
                entity_readers.add(perm.module_role_name)
            if perm.access_type == "MEMBER_READ":
                member_readers.add(perm.module_role_name)

        # roles with entity read but no MEMBER_READ entries are reading all attributes
        # (no attribute-level restriction). This may be intentional but worth flagging
        # on entities with many attributes or sensitive data.
        if len(entity_readers) > 0 and len(member_readers) == 0 and e.attribute_count > 10:
            violations.append(violation(
                message="entity '{}' ({} attributes) has no attribute-level access restrictions — all {} attributes readable by: {}".format(
                    e.qualified_name,
                    e.attribute_count,
                    e.attribute_count,
                    ", ".join(sorted(entity_readers)),
                ),
                location=location(
                    module=e.module_name,
                    document_type="entity",
                    document_name=e.qualified_name,
                ),
                suggestion="Consider using grant ... (read (Attr1, Attr2)) to restrict which attributes each role can access.",
            ))
    return violations
```

---

## Implementation Notes

- `permissions_for(qn)` should be backed by the same catalog data as `CATALOG.PERMISSIONS where ElementType = 'entity'`, filtered by `ElementName = qn`. It already exists in the catalog — this is a Starlark binding, not new data collection.
- `user_roles()` maps to `describe user role` output already available via MDL. The `is_anonymous` flag requires reading the project security anonymous user role setting from the MPR, which is already read for `project_security()`.
- `is_constrained` on `entity_permission` is a derived convenience property: `xpath_constraint != ""`. No new data needed.
- All three new additions follow the existing pattern of lazy-loaded, project-scoped data with no external dependencies.
