---
title: Access Rules Are Reconciled, Not Appended
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-backend.jsonl
  - mdl/backend/modelsdk/domainmodel_security_write.go
---

> **Do not duplicate**: the GRANT syntax and the security workflow live in
> `.claude/skills/manage-security.md`; the CE numbers and per-member rules live
> in the findings. This page describes why this small class is graded high.

## What this is

`GRANT` looks additive and is not. A grant is stored as an *entity access rule*
carrying a complete picture of what a role may do, so writing one is a
read-modify-write over an existing rule — and five `mdl/backend` findings are
that reconciliation losing rights the statement never mentioned.

The class is small and worth its own page because of what it costs when wrong.
Every other silent loss in this wiki costs a feature; this one **quietly widens
or narrows who can read data**, and both directions are reported as success.

## How it fits

**A grant that mentions one attribute must not revoke the others.** The reported
symptom was attributes granted by an earlier statement coming back as `None`.
Structural rights — create, delete, the default member access — went with them,
which the report did not mention: **re-derive the blast radius rather than
inheriting it from the reporter**.

**The reported trigger is often not the trigger.** A constrained (`WHERE`) grant
was blamed, and the same loss reproduced with no `WHERE` and with `READ *`. A fix
scoped to the reported path would have passed the reporter's reproduction and
left most of the defect in place.

**When a value is written and then absent, instrument the later pass.** In the
most instructive of these the writer was innocent — it stored all three rules —
and a reconciliation running afterwards removed them. Starting at the writer is
the natural instinct and the wrong end.

**Preserve what cannot be checked.** Membership questions do not all resolve
locally: an association is qualified by the module that *declares* it, so a
specialization inheriting from a generalization in another module has members the
local walk cannot see. Dropping what the walk cannot confirm produced
`CE0066 "Entity access is out of date"` — the model claiming rights over members
it no longer lists. The safe default is to carry an unconfirmable member through
rather than to prune it.

**Under-reporting access is the read-side twin.** A restricted page reported as
having "no roles" is the same class seen from the query side, and it is the shape
most likely to be believed, because "no roles" reads like a finding rather than a
gap.

**Reconciliation has two directions and they are fixed separately.** Every
finding above is reconciliation *removing* something. The other direction —
failing to *add* a member the entity has gained — produced the same `CE0066`
from the opposite side, and stayed open through several fixes to this function
because each one was aimed at the loss. The ancestor set was being computed and
consumed by the association pass only; the attribute pass beside it walked the
entity's own attributes and no one noticed the asymmetry. **When a function
computes a set and uses it for one of two symmetric passes, check the other.**

**A count that drives a message is part of the contract.** The same defect made
`update security` print *"All entity access rules are up to date"* over a project
`mx check` rejects, because the reconcile reported 0 modified. That is worse than
an error: an error is investigated, a false success ends the investigation, and
the reporter reasonably concluded the command was broken rather than the model.
Assert the count a repair command returns, not only the state it leaves behind.

**Once the write path reconciles, an MDL script can no longer prove the repair
path.** The statement that used to break the model now fixes it as it writes, so
the script ends clean against a build that never had the fix. Break the project
with a *previous* binary and repair it with the new one — that is what finally
showed this command taking a real `CE0066` from 1 to 0.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the member
  walks, the CE numbers and the controls
- [[association-pointers]] — why a member belongs to the FROM entity, which
  decides where a MemberAccess may appear
- [[engine-divergence]] — where the "no roles" read gap came from
