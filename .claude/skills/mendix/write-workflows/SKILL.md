---
name: write-workflows
description: "Author Mendix workflows in MDL — user tasks, decisions, parallel splits, jumps, waits and boundary events, with CREATE, ALTER and DROP. Use when building a business process with human steps, timers or parallel branches."
---

# Mendix Workflows Skill

Guidance for **authoring** workflows in Mendix projects with MDL — not just
reading them. `CREATE WORKFLOW` / `DROP WORKFLOW` / `ALTER WORKFLOW` are fully
supported and build in Studio Pro. Workflows are **not** read-only in mxcli; do
not punt workflow creation to Studio Pro.

## When to Use This Skill

- Creating a business process: approvals, reviews, multi-step tasks with user
  interaction, timers, and parallel branches.
- Adding/removing/reordering activities in an existing workflow (`ALTER WORKFLOW`).
- Regenerating a workflow from `DESCRIBE WORKFLOW` output (round-trippable).

A workflow is a `Workflows$Workflow` unit driven by a **context entity**: the
persistent entity each workflow instance is about (the `Expense` being approved,
the `LeaveRequest` being reviewed). User tasks render a page bound to
`System.WorkflowUserTask`.

## Syntax — CREATE WORKFLOW

The header options are **order-sensitive** (parameter → display → description →
export level → overview page → due date), and the body **must** close with
`END WORKFLOW`.

```sql
create workflow Module.ApprovalFlow
  parameter $Context: Module.Request        -- REQUIRED: must be a $-variable + context entity
  display 'Request Approval'                 -- optional human-readable name
  description 'Approves incoming requests'   -- optional
  export level Hidden                        -- optional: Hidden | API (default Hidden)
  overview page Module.WF_Overview           -- optional admin overview page
begin
  -- activities here, each terminated with ;
end workflow;
```

**Two gotchas that trip up first attempts:**

- `PARAMETER` takes a **`$`-variable then a context entity**: `parameter $Context:
  Module.Entity`. `parameter Module.Entity` and `parameter name: Module.Entity`
  both fail (`expecting VARIABLE`).
- The body closer is `end workflow`, **not** `end`. `end;` fails (`missing
  WORKFLOW`).

**The context is always stored as `WorkflowContext`.** Whatever you name the
variable in the header, mxcli writes the parameter as `WorkflowContext`, so
`$WorkflowContext/Attribute` is the canonical way to reach it in an expression.
The name you declared (`$Context` above) and any casing of the canonical name
(`$workflowContext`) are rewritten to it on write — in decision conditions, user
task due dates and XPath targeting, wait-for-timer delays, and `with (…)`
parameter mappings. Anything else is an undefined variable and Mendix fails the
build with `CE0117 "Error(s) in expression."`.

`create or replace workflow …` and `create or modify workflow …` are supported.

## Activities

Every activity statement ends with `;`. Blocks `{ … }` nest a sub-flow.

```sql
create or replace workflow Module.ApprovalFlow
  parameter $Context: Module.Request
begin
  -- User task: renders a page, offers named outcomes (branches)
  user task Review 'Review the request'
    page Module.ReviewPage
    targeting users microflow Module.ACT_Reviewers   -- or: targeting users xpath '[Active = true()]'
    description 'Please review'
    outcomes
      'Approve' { call microflow Module.ACT_Process; }
      'Reject'  { call microflow Module.ACT_Notify; };

  -- Multi user task: same clauses, one task per targeted user
  multi user task GroupSignoff 'Group sign-off'
    page Module.ReviewPage
    outcomes 'Done' { };

  -- Call a microflow (server logic); optional parameter mapping + outcomes
  call microflow Module.ACT_Validate
    with (Module.ACT_Validate.Item = '$WorkflowContext');

  -- Decision: a boolean or enum exclusive split
  decision '$WorkflowContext/Total > 1000'
    outcomes
      true  -> { call microflow Module.ACT_Escalate; }
      false -> { call microflow Module.ACT_AutoApprove; };

  -- Parallel split: independent branches run concurrently
  parallel split
    path 1 { call microflow Module.ACT_Notify; }
    path 2 { call microflow Module.ACT_Log; };

  -- Wait for a timer, then continue (duration is a Mendix expression)
  wait for timer 'addHours([%CurrentDateTime%], 1)';

  -- Wait for an external notification (e.g. an event)
  wait for notification;

  -- Jump back to an earlier activity by name (a loop)
  jump to Review;

  -- Call a sub-workflow
  call workflow Module.SubProcess comment 'delegate';
end workflow;
```

> **Do NOT use `annotation '...'` in a workflow body.** It parses, but the
> annotation is written into the workflow's activity flow, which Mendix loads by
> constructing every child with a `Flow` parent — no annotation type takes one, so
> the resulting `.mpr` **cannot be loaded at all**: Studio Pro will not open the
> project and `mx check` fails before validating anything. `mxcli` now refuses the
> statement (MDL-WF04) at both check and exec time. Keep the note as an MDL comment
> (`-- ...`); workflow canvas annotations are not yet writable.

**Boundary events** attach a timer to a user task / call-microflow / wait:

```sql
create or replace workflow Module.WithBoundary
  parameter $Context: Module.Request
begin
  user task Review 'Review'
    page Module.ReviewPage
    outcomes 'Done' { }
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' {
      call microflow Module.ACT_Escalate;
    };
end workflow;
```

## DROP WORKFLOW

```sql
drop workflow Module.ApprovalFlow;
```

## ALTER WORKFLOW

In-place edits go through the workflow mutator — no full rewrite. Supports
`SET` properties, and `INSERT` / `DROP` / `REPLACE` of activities, outcomes,
parallel paths, decision conditions, and boundary events. Reference an activity
by its name (or an auto-named one by its caption in quotes).

Each operation is its **own statement** — there is no `{ … }` wrapper, and `SET`
uses no `=` (`set display 'X'`, not `set display = 'X'`):

```sql
alter workflow Module.ApprovalFlow set display 'Updated Approval';
alter workflow Module.ApprovalFlow set activity Review page Module.AltReviewPage;
alter workflow Module.ApprovalFlow insert after Review call microflow Module.ACT_Log;
alter workflow Module.ApprovalFlow replace activity ACT_Validate with call microflow Module.ACT_Process;
```

Consecutive `set`s may chain in one statement:
`alter workflow Module.ApprovalFlow set display 'X' set description 'Y';`

See `mdl-examples/doctype-tests/24-workflow-examples.mdl` for the full ALTER
surface (insert path, drop path, insert condition, boundary events).

## DESCRIBE round-trip

`DESCRIBE WORKFLOW Module.Name` emits **executable, re-runnable** MDL — user
tasks, decisions, splits, jump-to targets, wait activities and boundary events
all come back as statements (not comments). You can learn the exact syntax by
describing a Studio-Pro-authored workflow, and `describe → drop → exec`
reproduces a workflow that builds. (The implicit start/end activities are
omitted, as they are re-synthesised on create.)

**What DESCRIBE still cannot see: event sub-processes.** mxcli has no model for
them at all, so they do not appear in the output and cannot be written from MDL.
A workflow that has one can only be edited in Studio Pro or through
`ALTER WORKFLOW` (below) — never with `CREATE OR REPLACE`.

## Rewriting an existing workflow

`CREATE OR REPLACE|MODIFY WORKFLOW` **rebuilds the workflow from the statement**,
so anything the script does not restate is deleted — including each boundary
event's whole handler flow. This is the failure that costs real work: it is not
reported by `mx check` afterwards, because the result is a perfectly valid
workflow that simply no longer does what it did.

mxcli refuses the two cases where that would lose something:

- a stored **event sub-process** — MDL cannot express one, so the rewrite is
  refused outright;
- **more stored boundary events than the statement declares** — restate them and
  the rewrite proceeds, which is what `describe workflow` now emits for you.

The safe way to change one activity in a workflow carrying hand-placed structure
is `ALTER WORKFLOW`, which mutates in place and touches nothing else.

## Microflow statements for workflow tasks

These run **inside a microflow** (not in the workflow body) and drive a running
workflow / its tasks. They are easy to miss — there is no `complete task`:

- `set task outcome $Task 'Approve';` — completes a `System.WorkflowUserTask` with a
  named outcome. This is how a microflow (e.g. a task page's button) finishes a task
  and does the domain work; the outcome branches still record which one was chosen.
- `open user task $Task`, `notify workflow $Wf`, `lock workflow $Wf`, and
  `workflow operation abort|pause|restart|retry|continue $Wf` are also statements.

A common shape: the task page's buttons call a microflow that does the change and
then `set task outcome $Task '<Outcome>'`, leaving the workflow's outcome branch
bodies empty.

### Claim the task before completing it

**`set task outcome` on a task nobody has claimed fails at runtime**, and it fails
quietly — the button appears to do nothing and the only trace is in the runtime log:

```
ERROR - Client: You can't complete this user task, it is not assigned to you.
```

`mxcli check` and `mx check` both pass; the build is clean. `mxcli check` now warns
about it (**MDL-WORKFLOW10**), but the platform rule is worth knowing rather than
being told.

The trap is that **`targeting xpath` / `targeting microflow` decides who may SEE a
task — it does not assign it.** There is no `assign task` statement; claiming is a
plain write to the Assignees association, and it must come first:

```sql
create microflow Module.ACT_CompleteTask ( $Task: System.WorkflowUserTask )
begin
  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Plan';
end;
```

If the task is claimed somewhere else — earlier in the process, or in a microflow
this one calls — the warning does not apply.

Related: **`WorkflowUserTask.Name` holds the task's CAPTION, not the activity
name.** A task declared `user task "ReviewAndPlan" 'Review and plan'` stores
`Name = 'Review and plan'`, so routing an inbox on the activity name silently never
matches. Route on your own entity's status instead.

## System-module documents are read from the runtime, not the .mpr

`describe enumeration System.WorkflowUserTaskState` and `show enumerations in System`
return nothing — the System module's **enumerations** are not in the project file, so
mxcli cannot resolve them. Constrain on an attribute instead (`[EndTime = empty]`
selects open tasks) rather than naming a System enum value. System **entities** are
documented in `system-module`.

## Platform rules

- A user task needs a **task page** to be useful; without one Mendix flags the
  task (`CE1834`). Bind the page to `System.WorkflowUserTask`.
- A user task / decision with a single outcome and no activity can trip
  `CE1876` — give each branch a body or a distinct outcome.
- The context **Parameter entity must be persistent**.
- Write the context variable as **`$WorkflowContext`**, matching the parameter
  name exactly. Mendix expressions are case-sensitive on 11.9+, so a lowercase
  `$workflowContext` is an undefined variable and yields `CE0117`.

## Observing a running workflow

A workflow's characteristic failures are **runtime** failures — an instance that
starts and stops, a task that never reaches an inbox, a task page that renders
blank. None of them is visible to `mxcli check`, `mxcli lint` or `mx check`,
which all validate the model rather than the data the model no longer matches.
So do not stop at "it builds".

Everything needed is already a skill — read the one you need rather than
hand-rolling admin-API calls:

| To see | Read |
|---|---|
| Live instances and open tasks (OQL against the running app) | [`verify-with-oql`](../verify-with-oql/SKILL.md), [`write-oql-queries`](../write-oql-queries/SKILL.md) |
| The exception that stopped an instance | [`analyze-runtime`](../analyze-runtime/SKILL.md) — `run --local` tees the runtime log to `<projectDir>/.mxcli/runtime.log` |
| `System.Workflow` / `System.WorkflowUserTask` / `System.WorkflowDefinition` shapes | [`system-module`](../system-module/SKILL.md) |
| Driving a task end to end and asserting the result | [`test-app`](../test-app/SKILL.md), [`run-local`](../run-local/SKILL.md) |
| Raw admin API, incl. `POST /dev/preview_execute_oql` | [`runtime-admin-api`](../runtime-admin-api/SKILL.md) |

Two traps worth knowing before you start:

- **The declared return type is not what the runtime checks.** A workflow-called
  microflow whose end event returns a value while the microflow declares no
  return type fails at instance start with `Trying to compare
  VoidConditionValue$('') to BooleanValue('true')`. `mxcli check` catches this as
  **MDL004** — so do not skip it, and do not reach for `--no-check` to get past
  it. Read the message in the order it is written: the receiver is the stored
  outcome's condition, the argument is what the microflow actually returned.
- **A parked instance is not a failed one.** A wait or timer branch is supposed
  to sit there. Check the branch before calling it a hang.

## Validate before presenting

```bash
./bin/mxcli check script.mdl                      # syntax + activity grammar
./bin/mxcli check script.mdl -p app.mpr --references   # entity/page/microflow refs exist
```

Then `show workflows` (lists the workflow, its parameter entity, and activity
count) and, if Docker is available, `mxcli docker build -p app.mpr` for the full
Studio-Pro validation.
