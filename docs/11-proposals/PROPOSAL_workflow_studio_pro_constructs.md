# Proposal: Workflow constructs only Studio Pro could author

**Status:** Phases 1–3 and 4a (event sub-processes, notification events) implemented; 4b (`notify workflow … target`) designed
**Date:** 2026-09-14

## Problem Statement

A team building Mendix workflows from MDL reported the constructs they still had
to add in Studio Pro after every scripted rewrite. Since #466 mxcli refuses a
rewrite that would lose them (guard-don't-drop, ADR-0005), which keeps the model
safe but leaves the work manual. This proposal makes them authorable, in the order
the team's workflows reference them:

| Phase | Construct | Stored as | Reference |
|-------|-----------|-----------|-----------|
| 1 | on-created microflow of a user task | `OnCreatedEvent` → `Workflows$MicroflowBasedEvent` | ako/TestApp `workflow.Workflow1` |
| 1 | workflow event handlers | `Workflow.OnWorkflowEvent` → `Workflows$WorkflowEventHandler` | ako/TestApp `workflow.Workflow1` |
| 2 | AI agent task | `Workflows$AIAgentTaskActivity` | ako/TestApp `workflow.Workflow1` |
| 3 | multi-user completion rules (majority, threshold, veto, microflow) | `CompletionCriteria` variants | **needed** |
| 4 | event sub-processes, notification events, and `notify workflow` targeting one | `Workflow.EventSubProcesses`, `Workflows$NotificationActivity`, `Workflows$*NotificationBoundaryEvent`, `NotifyWorkflowAction.NotifyTarget` | ako/TestApp `workflow.ZzMxcliExample_EventSubProcesses`, `workflow.ZzMxcliExample_Notify` |

## Phase 1 — handlers (implemented)

### BSON structure (ako/TestApp, Studio Pro 11.14.0, 0 errors)

```
SingleUserTaskActivity / MultiUserTaskActivity
  OnCreatedEvent: { $Type: "Workflows$MicroflowBasedEvent", Microflow: "workflow.UserTaskEventHandle" }
               or { $Type: "Workflows$NoEvent" }

Workflow
  OnWorkflowEvent: [2,
    { $Type: "Workflows$WorkflowEventHandler",
      Description: "OnAnyEvent",
      Documentation: "",
      EventTypes: [1, "WorkflowCompleted", "WorkflowInitiated", …],   // explicit list, marker 1
      MicroflowEventHandler: { $Type: "Workflows$MicroflowEventHandler", Microflow: "workflow.WorkflowEventHandle" } } ]
```

Studio Pro stores a handler's event types as an **explicit list even when every
box is ticked** — the handler named "OnAnyEvent" stores all 42. There is no "any"
flag.

Before this change both writers hard-coded `NoEvent` and an empty handler list,
and the legacy reader parsed `OnCreatedEvent` as a string (it is a document), so an
on-created microflow read back as none on that engine.

### MDL syntax

```sql
create workflow HR.LeaveApproval
  parameter $Request: HR.LeaveRequest
  on workflow events (UserTaskStarted, UserTaskEnded) microflow HR.ACT_AuditTask as 'Task audit'
  on any workflow event microflow HR.ACT_LogEvent as 'OnAnyEvent'
begin
  user task Review 'Review the request'
    page HR.ReviewPage
    targeting microflow HR.GetReviewers
    on created microflow HR.ACT_AssignReviewer
    outcomes 'Approve' { } 'Reject' { };
end workflow;
```

- Handler clauses follow the other header options, before `begin`, and repeat.
  `as '…'` is the handler's description — how Studio Pro lists handlers.
- `on created microflow` sits after the targeting clauses on both `user task`
  and `multi user task`.
- A named list is written in Studio Pro's order and de-duplicated; names match
  case-insensitively. `describe` emits `on any workflow event` exactly when the
  stored list equals the project version's full set, and otherwise the list (one
  type per line beyond three), so re-executing stores the same list.
- A handler's `Documentation` has no MDL spelling; a rewrite carries it from the
  stored handler with the same microflow and description.

### Measured rules (mxbuild 11.13.0, one task or handler per shape)

| Shape | Verdict |
|-------|---------|
| on created `(WorkflowUserTask, Ctx)`, either order | 0 errors |
| on created `(WorkflowUserTask)`, `(Ctx)`, `()`, `(+ String)`, `(WorkflowUserTask, <specialization>)` | CE6683 |
| on created returning a value | CE5012 |
| handler `(WorkflowEvent, WorkflowRecord, WorkflowActivityRecord)`, any order | 0 errors |
| handler `(WorkflowEvent)`, `()`, `(+ String)`, `(WorkflowEvent, Ctx)` | CE6691 |
| handler with no microflow | CE0113 |
| handler listing all 42 types | 0 errors |
| handler listing an **invented** type | **0 errors** |
| handler with an empty type list | 0 errors |

The invented-type row decides the design: mxbuild does not check type names, so a
misspelt type would be written and never fire. mxcli checks them itself —
**MDL-WF12** without a project, and against the project's version with one.

### Event types per Mendix version

Measured as the type names present in each mxbuild's `Mendix.Modeler.*.dll`:

| Version | Types | Added |
|---------|-------|-------|
| 11.6.0 | 32 | workflow, activity, user-task and boundary-timer events |
| 11.10.0 | 36 | `AIAgentTaskStarted/Ended`, `(Non)InterruptingNotificationEventSubProcessStartExecuted` |
| 11.13.0, 11.14.0 | 42 | `NotificationStarted/Ended`, `(Non)InterruptingNotificationEventExecuted`, `(Non)InterruptingTimerEventSubProcessStartExecuted` |

- `on any workflow event` writes the set of the newest measured point not above
  the project version. Below 11.6.0 it is refused with a hint to name the types:
  no older mxbuild could be measured (Mendix publishes no arm64 10.24 build), and
  a guess would write types the version may not have.
- A named type is refused only where it is **measured absent** (at or below the
  last point before it appeared). Between that point and the one it was seen at,
  the answer is unknown and the author's choice stands.

### Guard changes

A rewrite now writes handlers and on-created microflows, so they move from
"cannot express" to "restated?" — the boundary-event rule: refused when the
statement declares fewer handlers, or fewer on-created microflows, than are
stored. `replace activity` is refused only when the replacement does not restate
the stored on-created microflow. Still refused outright: an event sub-process, a
non-default completion rule, and a handler with an empty type list (no MDL
spelling). (AI agent tasks were refused outright too until phase 2 made them
restatable.)

Running the end-to-end rewrite on both engines found that **none of the workflow
rewrite guards worked on the legacy engine**: its `GetRawUnit` returns arrays as
`primitive.A`, which the guards' `[]any` type switches never matched, so a rewrite
the modelsdk engine refused was written and deleted the handlers. The guards now
normalize the stored unit before walking it, and their tests decode fixtures the
way the legacy engine does.

### Version compatibility

`workflows.event_handlers` min 10.7.0 (`Workflow.onWorkflowEvent` in modelsdk gen)
gates the handler clauses. `onCreatedEvent` is 9.0.5, older than every supported
workflow version. The signature checks run on Mendix 11+, where they are measured.

### Files

| File | Change |
|------|--------|
| `mdl/grammar/domains/MDLWorkflow.g4` | `workflowEventHandlerClause`; `ON CREATED MICROFLOW` on both user-task alternatives |
| `mdl/ast/ast_workflow.go`, `mdl/visitor/visitor_workflow.go` | `EventHandlers`, `OnCreated` |
| `sdk/workflows/workflow.go` | `WorkflowEventHandler`, `Workflow.EventHandlers` |
| `sdk/mpr/writer_workflow.go`, `sdk/mpr/parser_workflow.go` | legacy write/read; `OnCreatedEvent` parse fix |
| `mdl/backend/modelsdk/workflow_write.go`, `workflow_read.go` | modelsdk write/read |
| `mdl/executor/workflow_event_types.go` | measured per-version type table |
| `mdl/executor/validate_workflow_handlers.go` | MDL-WF12, signatures (CE6683, CE5012, CE6691), version checks |
| `mdl/executor/cmd_workflows_write.go`, `cmd_workflows.go` | build and describe |
| `mdl/executor/validate_workflow_rewrite.go` | restated-handler guard |

### Not in phase 1

- `alter workflow` cannot yet add or change a handler or an on-created microflow
  (`set activity … on created microflow …` and header handler operations). A
  rewrite restating them is the path today.

## Phase 2 — AI agent task (implemented)

```sql
call agent microflow HR.InvokeAgent as aiAgentTask1 comment 'Classify the request'
  with (Request = '$WorkflowContext');
```

Stored as `Workflows$AIAgentTaskActivity` with `Caption`, `Name`, `Microflow`,
`BoundaryEvents` (marker 2), `Outcomes` (marker 3, a `VoidConditionOutcome` with its
`Flow`) and `ParameterMappings` (marker 2, `MicroflowCallParameterMapping`) — the
call-microflow shape under a different `$Type` (ako/TestApp, 11.14.0).

Because the shape is identical, the semantic model is `CallMicroflowTask` with an
`IsAgent` flag rather than a new type: every walker, validator, catalog edge,
activity-name rule and ALTER path that handles a call microflow handles an agent
task unchanged, and only the `$Type`, describe and the rewrite guard branch on it.

**Measured** (mxbuild 11.13.0) by writing each shape as a call-microflow activity
and switching only the `$Type`, so the two columns differ in nothing else:

| Agent microflow | call microflow | AI agent task |
|---|---|---|
| `(Ctx)`, mapped | 0 errors | 0 errors |
| `()` — no parameters | 0 errors | **CE1590** "Missing parameter" |
| `(Ctx, String)`, both mapped | 0 errors | 0 errors |
| `(System.Workflow)` | 0 errors | 0 errors |
| returns Boolean, true/false outcomes | 0 errors | 0 errors |
| returns an enumeration, value outcomes | 0 errors | 0 errors |
| interrupting timer boundary event | 0 errors | 0 errors |

So `check` refuses an agent microflow with no parameters, and nothing else new.

**Confirmed in Studio Pro 11.14 over MCP** (ped_get_schema, then a created
document checked with `ped_check_errors` once its error list settled): the agent
task, event handler and on-created shapes mxcli writes are accepted; an on-created
microflow with the handler signature and a handler with the on-created signature
report the CE6683 / CE6691 messages mxbuild reports; an agent task without
parameter mappings is flagged; and an invented event type is refused at create by
the schema's enum of the same 42 names. Details in
`docs/03-development/PED_MCP_CAPABILITIES.md`.

**Version.** `AIAgentTaskActivity` is absent from the 11.6 mxbuild and present
from 11.10; the writer already records that Mendix 11.9 split
`MicroflowBasedActivity` into `CallMicroflowActivity` + `AIAgentTaskActivity`.
`workflows.ai_agent_task` min 11.9.0 gates CREATE and the activities ALTER adds.

**Guard.** A rewrite declaring fewer agent tasks than are stored is refused
(restate with `call agent microflow`, which describe emits); it was refused
outright before, since describe printed an agent task only as a comment.

**Engines.** Written and read by the codec engine, the only one since #468 retired
the legacy backend. The `sdk/mpr` serializer, which remains for the tools that still
hold it, carries the flag through (same shape, agent `$Type`) rather than silently
turning an agent task into a call microflow. The MCP backend sends it as
`Workflows$AIAgentTaskActivity`, verified against Studio Pro 11.14.

## Phase 3 — completion rules (implemented)

```sql
multi user task Vote 'Vote on the request'
  page HR.VotePage
  participants 80 percent
  decide by threshold 60 percent fallback 'Reject'
  await all users
  outcomes 'Approve' { } 'Reject' { };
```

`decide by consensus|majority more than half|majority most chosen|threshold <n>
percent|votes [fallback '<outcome>']`, `decide by veto '<outcome>'`, `decide by
microflow M`; `participants all|<n>|<n> percent`; `await all users`. Each clause
omitted is what a rebuild has always written — all participants, consensus falling
back to the first outcome, not waiting — so existing describe output is unchanged.

### Reference documents

The examples were built in Studio Pro 11.14 over MCP, checked clean, saved and
committed to ako/TestApp (`workflow.ZzMxcliExample_Completion2`, now the reader
fixture `mdl/backend/modelsdk/testdata/TestApp.ZzMxcliExample_Completion2.bson`).
Stored:

| MDL | CompletionCriteria | TargetUserInput |
|-----|--------------------|-----------------|
| `consensus fallback 'Fast'` | `ConsensusCompletionCriteria{FallbackOutcomePointer}` | |
| `majority more than half fallback 'Fast'` | `MajorityCompletionCriteria{CompletionType: Absolute, FallbackOutcomePointer}` | |
| `majority most chosen fallback 'Fast'` | `…{CompletionType: Relative, …}` | |
| `threshold 60 percent fallback 'Fiurious'` | `ThresholdCompletionCriteria{CompletionType: Relative, FallbackOutcomePointer, Threshold: 60}` | |
| `threshold 2 votes …` | `…{CompletionType: Absolute, …, Threshold: 2}` | |
| `veto 'Fiurious'` | `VetoCompletionCriteria{VetoOutcomePointer}` | |
| `microflow M` | `MicroflowCompletionCriteria{Microflow: "M"}` | |
| `participants 80 percent` / `participants 3` / omitted | | `PercentageAmountUserInput{Percentage}` / `AbsoluteAmountUserInput{Amount}` / `AllUserInput` |

Both pointers hold the **`$ID` of one of the task's own outcomes** (not its
`PersistentId`); `AwaitAllUsers` is a bool on the task.

### Measured (mxbuild 11.13.0, one task per shape)

| Shape | Verdict |
|-------|---------|
| consensus / majority (both) / threshold without fallback | CE1866 |
| veto without veto outcome | CE1867 |
| microflow rule without microflow | CE0113 |
| decision microflow returning Boolean | CE5012 |
| decision microflow with no, context or WorkflowUserTask parameters | 0 errors |
| threshold 0 or 101 percent, 0 or 5 votes; participants 0 or 150 percent, 0 | 0 errors |

`check` refuses the first two rows and an outcome name that matches none
(MDL-WF13, no project), and CE5012 when the script declares the microflow's return
type. The numbers are not range-checked: the build accepts them, so a refusal
would be a guess.

### Guard

A rewrite writes what the statement says, so each omitted clause resets a stored
value. Refused when the statement declares fewer non-default rules, non-`all`
participant counts or `await all users` than are stored — the last two were not
guarded before this and a rewrite reset them silently. `replace activity` is
refused when the replacement does not restate them.

### MCP

The multi-user task constructor takes `completionCriteria`, `majorityType`
(MoreThanHalf|MostChosen), `thresholdType` (Percentage|AbsoluteNumber),
`thresholdValue`, `fallbackOutcome` / `vetoOutcome` (outcome names),
`participiantInput` (sic) / `participiantValue` and `awaitAllUsers`. Two quirks,
both measured: PED's default is consensus with **no** fallback (CE1866), so mxcli
always sends one; and the constructor **drops the fallback of a more-than-half
majority**, so the backend reads the outcome's `$ID` afterwards and sets it.

## Phase 4a — event sub-processes and notification events (implemented)

### Reference documents (ako/TestApp, Studio Pro 11.14.0, built over MCP and saved)

`workflow.ZzMxcliExample_EventSubProcesses` holds one sub-process per start kind,
a notification activity, and both notification boundary events on a user task:

| Construct | Stored as |
|---|---|
| sub-process | `EventSubProcesses` (marker 2) → `Workflows$EventSubProcess{Annotation, Caption, Flow, Name, PersistentId}` |
| its start | the flow's first activity: `Workflows$(Non)Interrupting(Notification\|Timer)EventSubProcessStartActivity`; a timer adds `FirstExecutionTime` |
| its end | a closing `Workflows$EndWorkflowActivity`, as in the main flow |
| notification activity | `Workflows$NotificationActivity{Annotation, Caption, Name, PersistentId, RelativeMiddlePoint, Size}` |
| notification boundary event | `Workflows$(Non)InterruptingNotificationBoundaryEvent{Annotation, Caption, Flow, Name, PersistentId}` |

gen has no type for the timer starts, the notification activity or the
notification boundary events; the reader takes them off the raw document, and the
saved document is its fixture.

### MDL syntax

```sql
begin
  notification DocumentsReceived comment 'Documents received';
  user task Review 'Review' page HR.ReviewPage outcomes 'Approve' { } 'Reject' { }
    boundary event interrupting notification Withdrawn 'Withdrawn' { end workflow; };

  event subprocess ESP_Cancel 'Cancel request'
    on interrupting notification espCancelStart 'Cancel received' { … };
  event subprocess ESP_Reminder 'Daily reminder'
    on non interrupting timer 'addDays([%CurrentDateTime%], 1)' as espReminderStart { … };
end workflow;
```

Sub-processes follow the main body because they are stored beside the flow, not in
it. The body's End is implicit, like the main flow's.

### Measured (mxbuild 11.13.0, Studio Pro-saved shapes, one construct per workflow)

| Shape | Verdict |
|---|---|
| each of the four starts, body ending in End | 0 errors |
| body with activities; body ending in a jump within the sub-process (also to its start) | 0 errors |
| body with no end | **CE0105** — the builder appends the End |
| jump into another sub-process, or between one and the main flow | **CE6682** — MDL-WF05 |
| timer start with no first execution time | **CE0126** — MDL-WF14 |
| start event named like another activity | **CE0495** — names deduplicated across flows |
| boundary-path end marker inside a sub-process | CE6692 (not authorable) |
| notification boundary event on a user task, multi-user task, call microflow, wait for notification or wait for timer | 0 errors |
| interrupting notification path ending in the end-of-path marker | 0 errors |
| interrupting notification path with no end | **CE0105** — the builder appends the marker |
| two interrupting boundary events on one activity | **CE6697** — MDL-WF15 |

### Versions (each construct loaded alone into a blank project)

| | 11.10 | 11.11 | 11.12 | 11.13 | Gate |
|---|---|---|---|---|---|
| notification-started sub-process | loads | loads | loads | loads | `event_subprocesses` 11.8 (gen) |
| notification activity / boundary events | unknown type | loads | loads | loads | `notification_events` 11.11 |
| timer-started sub-process | unknown type | unknown type | unknown type | loads | `timer_event_subprocesses` 11.13 |

### Guard

Event sub-processes and notification activities are restate-or-refuse, like
boundary events; only a sub-process with no start event is refused outright. Every
counter walks the sub-process bodies, and a sub-process's closing End is not
counted as a stored `end workflow`. A rewrite from an older describe used to
delete notification activities silently — measured, 1 → 0 with the previous build.
`alter workflow … insert boundary event` refuses the notification kinds: the op
carries no name.

### MCP

The workflow constructor takes `eventSubProcesses`, each flow sent whole. The
interrupting notification boundary event's constructor requires
`isInsideOfParallelSplit`, set from the event's position. A rewrite removes the
stored sub-processes and adds the statement's.

Measured live against Studio Pro 11.14 (`mxcli exec --mcp`, then `ped_check_errors`
and `ped_read_document`):

| Shape | Result |
|---|---|
| notification and timer sub-processes, a jump inside one, a notification activity, an interrupting notification event on a wait ending in `end workflow` | stored as sent, "No errors found." |
| boundary events on a **single** user task — timer or notification — at create, and at an `add` of the task in an update | **dropped**, "No errors found." (a multi-user task keeps them) |
| a notification event added afterwards at `<task>/boundaryEvents` | stored |
| interrupting notification event inside a parallel split, path ending in `jump to` | stored as sent |
| the same path ending in the end-of-path marker | refused: the constructor appends a jump with no target after it |
| interrupting **timer** event, path ending in the marker / `end workflow` / `jump to` | End appended after the marker (refused) / stored / **jump replaced by an End**; every one "Missing value for parameter 'Timer'" |

The interrupting constructors normalize the path's terminator — End outside a
split, jump inside one, removing the other — and the interrupting timer
constructor has no `firstExecutionTime` at all. mxbuild 11.13 builds every one of
these paths, so the rules are Studio Pro's. For notification events the MCP
backend refuses each shape the constructor would rewrite, naming the ending it
needs, and re-adds a single user task's notification events after every create
and update. The timer-event losses predate this phase and are tracked separately.

## Phase 4b — notify targets

`NotifyWorkflowAction.NotifyTarget` (11.7.0) names what a microflow notifies:
`Workflows$(Non)InterruptingNotificationEventSubProcessStartActivityTarget`,
`NotifyNotificationActivityTarget`, `NotifyWaitForNotificationActivityTarget`
(each `Activity: "Module.Workflow.name"`) and `NotifyNotificationBoundaryEventTarget`
(`BoundaryEvent: …`). Reference: ako/TestApp `workflow.ZzMxcliExample_Notify`.
Syntax: `$Notified = notify workflow $Workflow target HR.Leave.espCancelStart;`.
Two bugs to fix with it: the reader takes the output variable from gen's
`VariableName` key where Studio Pro stores `OutputVariableName`, and a rewrite drops
the target.

## Test plan (phase 1)

- Unit: visitor (clause positions with and without targeting), legacy
  serialize/parse, modelsdk write→read round trip with the raw unit's markers,
  the per-version type table, signature rows as mock-backed `check` cases, the
  version checks, MDL-WF12, describe → re-parse → rebuild, the guard's
  restated/unrestated cases.
- Controls: each fix stubbed in turn makes its test fail.
- Doctype: `24-workflow-examples.mdl` part F builds under `mx check` on both
  engines.
- End to end: exec on an 11.13 app, `mx check`, describe → re-exec → identical
  describe; describe of TestApp's `workflow.Workflow1`.

## Open questions

- Whether a 10.x project knows every type in the 11.6 set. Named types are let
  through there; `any` is refused.
- A stored type newer than the table (a future Mendix) is refused by MDL-WF12 on
  re-execute until the table gains that version's point.
