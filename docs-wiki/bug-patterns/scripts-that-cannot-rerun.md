---
title: Scripts That Cannot Be Re-Run
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-grammar.jsonl
  - mdl/grammar/domains/MDLDomainModel.g4
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
---

> **Do not duplicate**: the `IF NOT EXISTS` / `create or modify` spellings live
> in `MDL_QUICK_REFERENCE.md`; write-level idempotence is ADR-0008 and
> [[element-identity]]. This page is about idempotence at the *statement* level,
> which is a different thing.

## What this is

mxcli's writes are idempotent — a unit whose content has not changed is not
written at all. Its *statements* are not, and the two get conflated. `create`
follows SQL convention and fails on an existing object; `alter entity … add
attribute` fails when the attribute is already there. Five `mdl/grammar` findings
are the consequence, and the compounding one is that `exec` halts on the first
error.

Put together: **a domain script that is 90% already applied applies none of the
remaining 10%.** The first `add attribute` that already exists ends the run, and
every later statement — including the new ones — is skipped. The user's reading is
usually that mxcli is broken, because re-running a script is the natural way to
converge a project on a description of it.

## How it fits

**The silent variant is worse than the error.** `alter entity … add index` did
not fail on a re-run: it reported "Added index" and appended a duplicate, and the
build then rejected the project. An error that stops the script is recoverable; a
success that corrupts is not, and the two live in the same statement family.

**Non-idempotence is a design decision, not a bug — but it has to be
discoverable.** MDL is SQL-shaped deliberately, and `CREATE TABLE` failing on an
existing table is the behaviour that shape implies. What was missing was the
signpost: the bare "already exists" error did not name `create or modify`, and
the idempotent spellings existed without being reachable from the failure.

**Two remedies, and they answer different questions.** `IF NOT EXISTS` /
`IF EXISTS` on a statement says *this one is optional*, which is what a
converging domain script wants. `exec --continue-on-error` says *attempt
everything and tell me what failed*, which is what a partially-applied script
wants — and it must still exit non-zero, or it trades a stopped run for a hidden
failure.

**Watch for the pair that cannot both be re-run.** `ADD EVENT HANDLER` errors
when the handler exists and `DROP EVENT HANDLER` errors when it does not, so
neither a plain script nor a defensive drop-then-add is re-runnable. A statement
family needs the guard on both halves or the workaround is unavailable too.

**A list rule without a separator reads as a value error.** Several parse
failures in this area were reported against the *value* in the second item of a
list — `add attribute A: integer default 9, add attribute B: …` — when the list
rule simply had no comma alternative. When a parse error names something that is
plainly valid in the first position, suspect the list before the value.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the guards, the
  flags and the bug-tests
- [[mdl-as-sql]] — why `create` is non-idempotent on purpose
- [[element-identity]] — write-level idempotence, which is unrelated and is often
  what people mean when they say mxcli is idempotent
