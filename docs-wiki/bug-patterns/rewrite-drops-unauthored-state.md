---
title: Rewrites That Drop What They Did Not Author
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-executor.jsonl
  - docs/13-decisions/0005-semantic-model-interface-currency.md
  - docs/13-decisions/0008-identity-and-idempotence.md
---

> **Do not duplicate**: the guard-don't-drop decision is canonical in ADR-0005,
> identity and idempotence in ADR-0008 and CLAUDE.md, and the per-document
> recipes in the findings. This page describes the recurring failure.

## What this is

`CREATE OR REPLACE` and `CREATE OR MODIFY` rebuild a document from a statement
that describes only part of it. Everything the statement does not mention — a
queued call binding, a toolbox entry, translated captions, an index, a folder, an
attribute's identity — has to be carried across, and each property that is not is
lost silently. Thirteen executor findings are this, and they are unusually
expensive because the loss is invisible at every checkpoint: the run reports
success, `mx check` reports 0 errors, and the model is valid. It is simply
smaller than it was.

## How it fits

**A model is bigger than its MDL.** Mendix documents carry state MDL has no
spelling for, and a rebuild constructs the document from what the statement says.
Anything else defaults. That is why the safe posture is **guard, don't drop**: a
rewrite that would discard a construct mxcli cannot express should refuse the
statement rather than quietly produce a smaller document.

**The most expensive variant is identity, not content.** Re-minting an
attribute's ID on every rewrite produced a document that looked identical and
made the runtime's database synchroniser treat every column as new — rows
survived, values gone. Nothing in the model was wrong. This is the same concern
as `GUID` preservation and the reason `canon.Reconcile` exists; a codec that
mints fresh identities on rebuild is a data-loss bug wearing a clean `mx check`.

**Delete-then-create defeats every protection.** One replace path removed the
stored document before writing the new one, so nothing was left for identity
preservation or elision to reconcile against, and translated captions in every
language reset to the source language. Whatever carrying mechanism exists, a path
that deletes first opts out of it.

**Stamping every field on both paths is the usual mechanism.** Where a create and
an update share a field-application helper, the update overwrites settings the
user changed by hand with values derived from somewhere else. A field set on the
`OR MODIFY` path is also not evidence it is set on the `CREATE` path — the two
construct the element separately, so both need checking, by grepping the struct
literal rather than the field name.

**Partial statements are the honest hazard.** `create or modify entity` with a
subset of attributes drops the rest — 36 down to 2 in one report — which is
arguably what "modify to this shape" means. The remedy there was not refusal but
telling the truth loudly: diff the members, print what is being dropped, and
point at the incremental spelling. Where the statement *is* a full replace, the
user asked for it; where the loss is of something MDL cannot express at all, they
did not.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  properties and the carrying mechanism each needed
- [[element-identity]] — `$ID` versus `GUID` versus `StableId`
- [[mpr-read-write]] — where the write choke points are, and what elision assumes
