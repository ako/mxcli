---
title: Element Identity — $ID, GUID, StableId
category: mental-model
last-synced: 9ab9afa6
sources:
  - docs/13-decisions/0008-identity-and-idempotence.md
  - modelsdk/canon/identity.go
  - CLAUDE.md
  - .claude/skills/fix-issue/findings/
---

> **Do not duplicate**: the rules an implementer must follow (CLAUDE.md is canonical), the decision and its measurements (see [ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md)), the dangling-reference fix recipe (symptom table in `.claude/skills/fix-issue.md`), or the identity-field table (read `modelsdk/canon/identity.go`).

## What this is

A Mendix model element can carry three different identifiers, and they are not interchangeable — they have different scopes and different lifetimes. `$ID` is the element's storage identity, private to the unit that contains it. `GUID` is a second, durable identity that survives operations which renumber `$ID`. And a microflow carries a `StableId`, an identity Mendix declares explicitly as such and derives client-facing names from.

The intuition to unlearn is that an `$ID` is *the* identity of a thing. It is closer to a pointer target — meaningful inside one document, meaningless outside it, and replaceable so long as everything pointing at it is replaced in the same breath.

## How it fits

**The unit is the identity boundary.** Nothing outside a unit observes its `$ID`s: cross-document references are qualified-name strings, not pointers. This is measured rather than assumed, and it is the load-bearing fact behind everything else — including Studio Pro's own behaviour, which renumbers an entire module's `$ID`s during a marketplace update while consumers of that module carry on unaffected.

**Inside a unit, the same IDs are load-bearing.** Pointers reference them, and Mendix's loader resolves those references *after* reading the document, so an ID that no longer has an owner is not a stale value — it is a hard failure that makes the project unopenable. What makes this trap subtle is that pointers are not child elements: they are primitive properties that happen to hold an element ID, so a walk over containment traverses the whole document and never sees one. A rewrite driven by such a walk renumbers the targets and silently leaves the pointers behind. Document types differ in how badly they are hurt — a page's widget tree is containment, a microflow's is a graph — which means "the pages still load" is not evidence that an ID-rewriting approach is safe.

Put together: an `$ID` is internal wiring. Preserving a unit wholesale is safe, replacing a unit wholesale is safe, and *editing IDs in place* is the operation that is not — it requires rewriting every reference in the same pass, which mxcli cannot currently express because it cannot enumerate which properties are references.

**`GUID` and `StableId` are the identities that must survive.** They exist precisely because `$ID` does not: they are what lets something outside the document — a consumer of an upgraded module, or the generated web client — keep referring to the same element after its storage identity has moved. Studio Pro's module update transplants them onto the incoming elements by name rather than trying to preserve `$ID`s, which is the same shape mxcli follows. Regenerating one of them on every write is therefore not harmless churn, even though nothing rejects it: a `StableId` determines the operation name the browser uses to invoke that microflow, so re-minting it renames operations in the deployed model for a microflow nobody edited.

The practical asymmetry to remember: identity that outlives a rebuild has to be *carried*, and identity that is private to the document can be re-minted freely — but only wholesale, never selectively. Both halves are wired at the write choke point described in [[architecture/mpr-read-write]].

## See also

- [ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md) — the decision, the measurements, and how `StableId`'s role was established from Mendix's own binaries
- [modelsdk/canon/identity.go](../../modelsdk/canon/identity.go) — which properties are treated as identity, and why the table is hand-maintained
- [[architecture/mpr-read-write]] — where identity is carried and where writes are elided
- [[models/association-pointers]] — another place Mendix's pointer naming is counter-intuitive
- [[models/storage-vs-qualified-names]] — the other two-names-for-one-thing trap
