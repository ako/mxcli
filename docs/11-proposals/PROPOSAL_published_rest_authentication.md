---
title: Authentication on a published REST service
status: implemented
date: 2026-10-07
related:
  - docs/13-decisions/0010-mdl-canonical-syntax-rules.md
  - show-describe-published-rest-services.md
---

# Authentication on a published REST service

> Written for mendixlabs/mxcli#1331. That issue also reported a parser crash on
> a non-string property value; the crash is fixed separately (ako/mxcli#1024)
> and is not part of this proposal.

## Problem Statement

`create published rest service` cannot say how a caller authenticates. Studio
Pro's service dialog has **Requires authentication** and, when on, the methods
**Username and password**, **Active session** and **Custom** (with an
authentication microflow). MDL has no spelling for any of them, so:

- a service mxcli creates is always unauthenticated, and the author has to
  finish it in Studio Pro;
- `describe` does not show a service's authentication, so reading a project
  through mxcli hides one of its most security-relevant settings;
- the reporter of #1331 fell back to publishing the API as OData, whose
  `authentication` clause works, to get custom authentication at all.

What exists today is a carry, not support: since ako/mxcli#571 the writer copies
the stored `AuthenticationTypes` and `AuthenticationMicroflow` back over its own
constants, so a rewrite does not turn a Studio Pro service's authentication
off. That protects existing services but means MDL can neither set nor read the
setting.

## BSON Structure

`Rest$PublishedRestService` (storage name equals qualified name). Two
top-level fields, both present on every service:

| Field | Shape | Notes |
|---|---|---|
| `AuthenticationTypes` | string list, **marker 1** | values of `Rest$AuthenticationType`: `Basic`, `Session`, `Microflow` (also `Guest`, `None` in the enum) |
| `AuthenticationMicroflow` | by-name reference, stored as a qualified-name string | `""` when none |

`AuthenticationType` (single) existed 7.11–7.13 and is deleted;
`AuthenticationTypes` since 7.13, `AuthenticationMicroflow` since 7.17
(`modelsdk/gen/rest/version.go`). Note the list marker differs from the
published OData service, which mxcli has written with 3 and Studio Pro with 1
(#743) — copying the OData writer is the wrong starting point.

Measured on ako/TestApp (`Services` module, Studio Pro 11.15.0-rc.4), one
service per dialog state:

| Service | Dialog | `AuthenticationTypes` | `AuthenticationMicroflow` | `AllowedRoles` |
|---|---|---|---|---|
| `OrdersRestApi` | all three methods | `[1, Basic, Session, Microflow]` | `Services.AuthMicroflow` | `Services.User` |
| `OrdersRestApi_MicroflowSession` | Custom ticked first, then Active session | `[1, Microflow, Session]` | `Services.AuthMicroflow` | `Services.User` |
| `OrdersRestApi_CustomOff` | Custom ticked, microflow chosen, Custom unticked | `[1, Basic, Session]` | `""` | `Services.User` |
| `OrdersRestApi_NoAuth` | Requires authentication = No | `[1]` | `""` | `[1]` (none added) |

What this settles:

1. **"Requires authentication = No" is the empty list.** There is no separate
   flag, and Studio Pro does not use `None` or `Guest` for it.
2. **Unticking Custom clears `AuthenticationMicroflow`.** No stale reference is
   left; mxcli must clear it too whenever `microflow` is not among the methods.
3. **The list keeps the order the methods were ticked.** It is not sorted.
   mxcli must write the methods in the order the statement gives them and
   `describe` must print the stored order; sorting either side would make an
   unchanged `describe` → `exec` rewrite the unit (ADR-0008).
4. **Roles are independent of authentication.** `AllowedRoles` is untouched by
   the authentication setting.

The authentication microflow in the sample takes `$HttpRequest:
System.HttpRequest` and returns `System.User`, empty meaning "not
authenticated".

## Proposed MDL Syntax

Authentication is a property of the service (ADR-0010 R9: everything but the
folder and documentation is a property), in the same list as `Path`:

```mdl
create or modify published rest service Shop.OrdersApi (
  Path: 'rest/orders/v1',
  Version: '1.0.0',
  ServiceName: 'Orders',
  Authentication: (basic, session, microflow Shop.AuthenticateRequest)
)
{
  resource 'orders' {
    get '{OrderId}' microflow Shop.GetOrder;
  }
};

alter published rest service Shop.OrdersApi set (Authentication: (session));
alter published rest service Shop.OrdersApi set (Authentication: none);
```

- `Authentication: none` is "Requires authentication = No".
- `Authentication: ( … )` lists the methods, at least one, in the order
  written: `basic` (username and password), `session` (active session),
  `microflow Module.Name` (custom). A list-valued property in parentheses is
  the shape `Parameters:` and `Headers:` already use.
- **Omitting the property keeps the stored setting** on `create or modify` and
  `alter`, as for every other property MDL does not state (`Excluded`, roles).
  A new service without it is created unauthenticated, as today.
- `alter … set ( … )` is new for this document type and takes create's keys
  (R3): `Path`, `Version`, `ServiceName`, `Authentication`. The existing
  `set Key = 'value'` form keeps working for the string properties.
- `describe` prints `Authentication: ( … )` when any method is stored, in the
  stored order, and omits it when none is (R12: the creation default is not
  printed). A stored value MDL cannot spell — `Guest`, `None`, an unknown
  method, or a microflow with no `Microflow` method — is not printed as a
  property but noted in a comment, so replaying the output keeps it.

### Why not the OData clause

The published OData service spells this as a trailing clause,
`authentication basic, session, microflow M.F`, after the property list. That
predates R9 and cannot be set by its `alter`. Following it here would add a
second non-property spelling and an `alter` gap; following R9 means the two
siblings differ until the OData clause is migrated. The migration is out of
scope here and noted under Open Questions.

## Validation

Measured with `mx check` on a fresh 11.14.0 project, one service per variant
(`mxcli new AuthProbe --version 11.14.0`, written by this change):

| Variant | `mx check` |
|---|---|
| microflow `($HttpRequest: System.HttpRequest) returns System.User` | clean |
| the same with the parameter named `$Req` | clean — matched by type |
| no parameters; `HttpRequest` + `HttpResponse`; `HttpResponse` only | clean |
| returns `Boolean` | **CE0334** "Authentication Microflow … should return a User" |
| a `String` parameter | **CE0336** "The microflow parameter ('Token') is not present in the custom authentication parameter list." |
| custom authentication, app security off | **CE6600** |
| authentication on, no allowed role | **CE0338** "At least one allowed role must be selected …" |
| no authentication, with an allowed role | clean |
| `(microflow M.F, session)` + role, security production | clean |

So, at `check --references` and `exec` time:

- **MDL-REST04** (error): the microflow does not exist, does not return
  `System.User`, or has a parameter that is not a `System.HttpRequest` or
  `System.HttpResponse`. A microflow the same script creates is resolved by name
  at check time; `exec` checks its signature once it exists.
- A repeated method (`(basic, basic)`) is a parse-time error; `()` does not parse.
- CE0338 and CE6600 depend on statements that come later (`grant`) or project
  state (app security), so they are documented, not predicted.

## Implementation Plan

| File | Change |
|------|--------|
| `mdl/grammar/domains/MDLService.g4` | `publishedRestProperty` gains `Key: none` and `Key: ( method, … )`; `publishedRestAuthMethod`; `alterPublishedRestServiceAction` gains `set ( property, … )` |
| `mdl/ast/ast_rest.go` | `PublishedRestAuthentication { Set bool; Methods []string; Microflow string }` on create, and on a new alter action |
| `mdl/visitor/visitor_rest.go`, `visitor_strict_properties.go` | build it; schema shape for `Authentication` |
| `model/types.go` | `PublishedRestService.AuthenticationTypes`, `.AuthenticationMicroflow` |
| `mdl/backend/modelsdk/integration_read.go` | read both fields |
| `mdl/backend/modelsdk/published_rest_write.go` | write both from the model; drop them from `publishedRestServiceUnauthored` |
| `mdl/executor/cmd_published_rest.go` | create / modify / alter: set, or carry the stored value when not stated; `describe` prints the property |
| `mdl/executor/validate.go` | MDL-REST04 at `check --references` |
| `docs-site/`, `mxcli syntax`, skill | document the property |

Moving the two fields from "carried by the writer" to "carried by the
executor" is the one risky step: every write path must then supply the stored
value itself. There are exactly three (`create`, `create or modify`, `alter`);
`grant`/`revoke` patch `AllowedRoles` only.

## Version Compatibility

Both fields exist on every version mxcli supports (10.0+), so no version gate.

## Test Plan

- Parser/visitor: each shape, the empty list, a duplicate, an unknown method.
- Backend: write each sample state and compare the encoded fields to the
  TestApp table above, marker and order included; read them back.
- Executor (MockBackend): omission carries the stored value on create-or-modify
  and alter; `none` clears methods and microflow; dropping `microflow` clears
  the microflow; `describe` → `exec` of each TestApp service writes nothing.
- `mx check` (11.14.0): a service per state built by mxcli, plus the
  microflow-signature variants, to pin MDL-REST04 to what Mendix accepts
  (results under Validation).
- `mdl-examples/doctype-tests/`: a published REST script exercising the property.

## Open Questions

1. ~~Migrate the published OData `authentication` clause to an `Authentication:`
   property~~ — done as a separate change: the property, `none`, `alter … set
   ( Authentication: … )`, and the clause as alias MDL-DEPR139 with an
   `fmt --upgrade` rewrite. The two siblings now agree.
2. ~~Whether a service with roles but no authentication is accepted~~ — it is
   (Validation table). The reverse, authentication with no role, is CE0338.
