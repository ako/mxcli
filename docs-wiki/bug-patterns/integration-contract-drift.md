---
title: Integration Documents and the Contract They Answer To
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - mdl/executor/cmd_odata.go
---

> **Do not duplicate**: the per-property fixes, EDM mappings and mapping-element
> shapes live in the findings; the MDL spellings live in the skills and
> `MDL_QUICK_REFERENCE.md`. This page describes what makes the area distinctive.

## What this is

Published OData services, consumed REST and OData clients, and import/export
mappings all describe a document whose correctness is defined **outside the
model** — by a schema, a `$metadata` document, or the EDM type system. Roughly
thirty executor findings live here, and they share a property that the rest of
the executor does not: for much of it, **neither `mxcli check` nor mxbuild is an
oracle**.

## How it fits

**The validation floor is lower than elsewhere.** A database connection's `type`
string is passed straight through, and mxbuild does not validate it either — so a
misspelled driver builds at 0 errors and simply does not connect. A mapping
naming a schema source that does not exist passes both checks. When neither tool
can tell you, the only remaining signal is a runtime failure, which is why
several findings in this class were first seen as an exception in a running app
rather than as an error anywhere.

**A silent downgrade that returns HTTP 200 is the worst available outcome.** A
consumed operation whose file body was written as literal text sent the string
`$Doc`, got a 200 back, and every signal a person or an agent would check said
success. Where mxcli cannot express a construct, refusing is strictly better than
writing a different one.

**Defaults are the recurring mechanism.** These documents carry many properties
Studio Pro sets and MDL does not mention — a service's `ServiceName` as distinct
from its document `Name`, a published attribute's `EdmType`, an association end's
`IsMany`, a mapping element's key flag, an activity's range and cardinality
pointers. Each omitted default is a build error or a runtime fault at some
distance from the statement, and several were invisible on one engine because its
`$Type` handling filled a gap the other left open.

**The type system is a real mapping, not a passthrough.** Mendix publishes
Integer as `Edm.Int64`; a name like `name` is not a system-managed attribute and
must not be disambiguated as if it were; `PublishAssociations` selects a
*representation* rather than a yes/no, and choosing "as an associated object id"
imposes a key requirement. Getting these wrong produces `CE5016`-family errors
across every exposed member at once, which reads as one large failure rather than
one small mapping bug.

**Modify paths lag create paths.** Re-running `create or modify` on a service
left the exposed entity sets untouched; on a client it left the cached contract
stale, so the tool kept reporting the old entity types after the backend had
changed. Where a create path fetches or derives something, check whether the
modify path does too — the asymmetry is easy to introduce and invisible until
someone edits an existing document.

**A rule that can be satisfied two ways will be satisfied the wrong way.** Two
OData advisories shared one predicate, so answering either concern silenced both:
adding an `HttpRequest` parameter for a key lookup also silenced the paging
warning, with no paging implemented.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  properties, EDM mappings and mapping-element shapes
- [[silent-property-drop]] — the same "written, then absent" shape inside pages
- `.claude/skills/verify-in-runtime.md` — for the failures that only appear when
  the request is actually made
