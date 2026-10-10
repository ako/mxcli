---
title: The Report Is Not the Write
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/report_mutation.go
  - mdl/executor/cmd_security_defaults.go
  - docs/13-decisions/0008-identity-and-idempotence.md
  - docs-site/src/internals/idempotent-writes.md
---

> **Do not duplicate**: the elision rule and the evidence test are canonical in
> `report_mutation.go`'s comment and in
> [idempotent-writes](../../docs-site/src/internals/idempotent-writes.md);
> ADR-0008 is the decision. The individual handlers are in the findings. This
> page is about why a line of output is a correctness surface.

## What this is

`exec` prints one sentence per statement, and that sentence is the **only
window** a user has into what reached disk. They do not read `mprcontents/`
mtimes or unit md5s; they read "Created microflow: M.F" and believe it. Which
means the output is a claim about storage — made, in the defects of this class,
by code that cannot see storage.

Two mechanisms put the handler and the write on opposite sides of a wall.
Storage **elides** a write whose new content is semantically equal to what is
already there (ADR-0008), so `ctx.Backend.Update*` returning `nil` says nothing
about whether bytes moved. And some writes are **deferred** to the end of a
program run — access rules among them — so at statement time there is nothing to
look at yet. A `fmt.Fprintf` after the backend call is therefore not a report but
a guess, and it is wrong in whichever direction the statement happens to be in.

## How it fits

**The honest default is "Unchanged", and only evidence can produce it.** The
verb is downgraded on a positive measurement — unit writes were offered since
the last report and none of them landed — never on a handler's belief about its
own statement. A mutation that never reaches unit storage leaves no evidence
either way and is reported unqualified. Every hand-rolled sentence that bypasses
`ReportMutation` / `reportWrite` reintroduces the class, which is why the sweep
is a grep (`fmt.Fprintf` lines carrying *updated* / *set* / *added* that follow a
`ctx.Backend.Update*`) rather than a statement-by-statement audit: navigation,
workflows, security, settings and `move` were each found that way, each after an
earlier pass had been declared complete.

**The output is a contract, not decoration.** The re-run of a settled script is
the idempotency gate for the whole `create or modify` programme — "second run
reports Unchanged" is how a 66-script rehearsal is scored. One statement that
prints a write verb on its own authority voids the gate for the entire script,
so the property is not "most handlers are honest" but *no handler prints a write
verb by construction*. That is the difference between a cosmetic bug and a
blocked release.

**A counter in the wrong loop reports the opposite of what happened.** `UPDATE
WIDGETS` printed `Warning: Failed to set …` for every assignment and then
`Updated 2 widget(s)`, because the counter was incremented outside the
assignment loop and meant "this widget was found". The failures were already
being printed correctly one line above the lie, which is the tell: when the
detail and the summary disagree, the summary is the one computed from the wrong
thing. The fix is not a boolean but **naming the outcomes that actually occur** —
changed, matched-but-unwritable, in-catalog-but-not-in-document — because
rounding the third into either of the others is how a stale catalog reads as
success. And the **dry run had the same defect one step earlier**, which is the
worse half: the syntax help tells you to run it first, and it printed `Would
set …` without attempting anything. A preview that re-implements what a setter
accepts drifts from it in exactly the direction that hurts; run the assignments
against the discardable copy instead.

**A count that lines up with the errors is the diagnosis.** `RENAME ENTITY`
reported "Updated 3 reference(s) in 1 document(s)" and the build then failed with
exactly 3 CE1613s naming those 3 members. That arithmetic is not a coincidence
and not a gap in the scanner: the sweep found them and a later write to the same
unit put the stale names back. A sweep that reports work it then discards is
indistinguishable from one that never ran — except by that count, which is why
the number in the report is worth reading as evidence rather than noise.

**A post-write echo must select by the key the write used.** `GRANT` prints the
resulting rule, and it picked the first rule naming *any* of the granted roles
with the same XPath, while the backend upserts by the *exact* role set plus
XPath. So the echo described a broader shared rule whenever a role also sat in
one — precisely the case that additive grants made common. Looser is not safer
for a read-back: it reports a different object than the one that was written.

**An idempotent no-op has to sound like one.** `revoke` of a right already gone
printed "No access rules found matching …", which reads as a failed lookup in a
script log. Wording only, and worth fixing, because the whole value of the
Unchanged vocabulary is that a reader can tell a settled script from a broken
one at a glance.

**Silence about a default the tool applied is the same defect inverted.** A new
page in a module with no module roles is granted to an auto-created `<Module>.User`
role, and document grants are additive — so a later admin-only grant *adds*
rather than restricts, and the user's reasonable conclusion is that `grant`
misbehaved. Nothing was written wrongly; the defect was that the default was
applied without being said. **A default applied on the user's behalf has to be
visible at the moment it is applied**, or the next correct operation looks broken.

**Bound the severity before writing it up.** "Claims success after failing"
reads as corruption, and for the widget case it was not: the rebuilt document
was semantically identical, so elision skipped the write, no unit's mtime moved
and `mx check` stayed at 0. Saying so is part of the report — a reporting defect
and a data-loss defect get different queue positions, and conflating them spends
the reader's alarm on the wrong one.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the handlers,
  and which evidence each now reports through
- [[scripts-that-cannot-rerun]] — the programme whose gate this output is
- [[element-identity]] — why a write is elided at all
- [[misleading-diagnostics]] — the parse-time sibling: a hint that sends the
  reader somewhere the problem is not
- [[absent-edge-reads-as-absent-fact]] — the read-side twin, where a query's
  empty answer is phrased as a fact
