---
title: Workflow Improvements: ALTER WORKFLOW + Cross-References
status: proposed
date: 2026-04-03
---

# Workflow Improvements: ALTER WORKFLOW + Cross-References

**Date:** 2026-04-03
**Status:** Proposal
**Author:** @engalar

## Problem

Workflow support in mxcli has full CREATE/DESCRIBE/DROP/SHOW coverage with 13 activity types and BSON round-trip fidelity. Two significant gaps remain:

1. **No ALTER WORKFLOW** — any workflow change requires a full `create or modify` rebuild. For small edits (change a task page, add an outcome, insert an activity), this is disproportionate effort and error-prone.

2. **No cross-reference tracking** — the catalog `refs` table does not track workflow references. `show callers of Module.SomeMicroflow` will not show workflows that call it. Impact analysis is incomplete.

## Design

### Part 1: ALTER WORKFLOW

#### 1.1 Property Operations

Modify workflow-level and activity-level metadata without touching the flow graph:

```sql
-- Workflow-level properties
alter workflow Module.MyWorkflow
  set display 'New Display Name';

alter workflow Module.MyWorkflow
  set description 'Updated description';

alter workflow Module.MyWorkflow
  set export level api;

alter workflow Module.MyWorkflow
  set due date '[%CurrentDateTime%] + 7 * 24 * 60 * 60 * 1000';

alter workflow Module.MyWorkflow
  set overview page Module.NewOverviewPage;

alter workflow Module.MyWorkflow
  set parameter $WorkflowContext: Module.NewEntity;

-- Activity-level properties
alter workflow Module.MyWorkflow
  set activity userTask1 page Module.NewTaskPage;

alter workflow Module.MyWorkflow
  set activity userTask1 description 'Updated task description';

alter workflow Module.MyWorkflow
  set activity userTask1 targeting microflow Module.NewTargeting;

alter workflow Module.MyWorkflow
  set activity userTask1 targeting xpath '[Status = "Active"]';
```

**Implementation**: Uses `readPatchWrite` pattern from ALTER PAGE. Reads raw BSON as `bson.D`, modifies specific fields, writes back.

#### 1.2 Graph Operations

The workflow flow graph is stored as two flat arrays:
- `Flow.Activities`: all activity objects
- `Flow.SequenceFlows`: directed edges between activities

Each graph operation maps to a well-defined transformation on these arrays.

##### INSERT AFTER (linear position)

**Precondition**: Target activity has exactly one outgoing non-error edge.

```sql
alter workflow Module.MyWorkflow
  insert after activityName
    call microflow validate Module.Validate;
```

**BSON transformation**:
```
before: A ──edge1──→ B
Step 1: create activity C, append to Activities array
Step 2: set edge1.Dest = C
Step 3: create edge2: {Origin: C, Dest: B}
after:  A ──edge1──→ C ──edge2──→ B
```

**Error conditions**:
- Target has multiple outgoing edges → reject with "activity is a split point, use INSERT OUTCOME/PATH/BRANCH"
- Target is End → reject with "cannot insert after end activity"
- Target not found → error

##### INSERT OUTCOME ON UserTask

**Structure**: UserTask has an `outcomes` array. Each outcome maps to outgoing edges via `ConditionValue` matching the outcome `$ID`.

```sql
alter workflow Module.MyWorkflow
  insert outcome 'NeedMoreInfo' on userTask1 {
    call microflow requestInfo Module.RequestInfo
  };
```

**BSON transformation**:
```
Step 1: create new outcome {Name: "NeedMoreInfo", $ID: id3}, append to outcomes array
Step 2: create activities from the block, chained in sequence
Step 3: find merge point (see algorithm below)
Step 4: add edge {Origin: userTask1, Dest: firstNewActivity, ConditionValue: id3}
Step 5: add edge {Origin: lastNewActivity, Dest: mergePoint}
```

**Error conditions**:
- Duplicate outcome name → reject
- Empty block `{}` → create direct edge to merge point
- Merge point not found → error (malformed graph)

##### INSERT PATH ON ParallelSplit

Same as INSERT OUTCOME but without ConditionValue:

```sql
alter workflow Module.MyWorkflow
  insert path on parallelSplit1 {
    user task review3 'Third Review'
      page Module.ReviewPage
  };
```

**BSON transformation**: Same as INSERT OUTCOME, but edges have no ConditionValue. The merge target is a ParallelMerge node.

##### INSERT BRANCH ON Decision

```sql
alter workflow Module.MyWorkflow
  insert branch on decision1 condition '$ctx/Status = "Special"' {
    call microflow handleSpecial Module.HandleSpecial
  };
```

**BSON transformation**: Creates a new ConditionOutcome on the Decision, then adds edges and activities as in INSERT OUTCOME.

##### INSERT BOUNDARY EVENT

```sql
alter workflow Module.MyWorkflow
  insert boundary event interrupting timer '86400000' on userTask1 {
    call microflow escalate Module.Escalate
  };
```

##### DROP ACTIVITY (linear node)

**Precondition**: Target has exactly one incoming edge and one outgoing edge.

```sql
alter workflow Module.MyWorkflow
  drop activity callMf1;
```

**BSON transformation**:
```
before: A ──e1──→ C ──e2──→ B
Step 1: set e1.Dest = B
Step 2: delete e2 from SequenceFlows array
Step 3: delete C from Activities array
after:  A ──e1──→ B
```

**Error conditions**:
- Multiple in/out edges → reject with "activity is a split/merge point, use DROP OUTCOME/PATH instead"

##### DROP OUTCOME / PATH / BRANCH

```sql
alter workflow Module.MyWorkflow
  drop outcome 'NeedMoreInfo' on userTask1;

alter workflow Module.MyWorkflow
  drop path 'Third Review' on parallelSplit1;  -- by first activity caption

alter workflow Module.MyWorkflow
  drop branch 'Special' on decision1;
```

**BSON transformation**:
```
Step 1: find the outgoing edge for this outcome/path/branch
Step 2: Collect all activities on this path (BFS from edge.Dest to merge point)
Step 3: delete collected activities and their edges from arrays
Step 4: delete the outcome/branch entry from the split activity
```

**Path activity collection algorithm**:
```
collectPathActivities(startId, mergeId):
  queue = [startId]
  result = []
  while queue not empty:
    id = queue.pop()
    if id == mergeId: continue  // don't delete merge point
    result.append(id)
    for edge in outgoingEdges(id):
      queue.append(edge.Dest)
  return result
```

#### 1.3 Merge Point Discovery Algorithm

Several operations require finding the merge/convergence point of a split node. All branches from a split eventually converge at a single merge point (Mendix enforces structured workflows).

The algorithm must handle **nested splits** — a branch may contain a Decision or ParallelSplit whose outgoing edges must be traversed through their own merge point before continuing.

```
findMergePoint(splitActivityId, activities, sequenceFlows):
  // get all outgoing edges from the split
  outEdges = filter(sequenceFlows, edge.Origin == splitActivityId)
  if len(outEdges) == 0: error("no outgoing edges")

  // Follow each branch using depth-aware traversal
  branchPaths = []
  for each edge in outEdges:
    path = orderedList()
    current = edge.Dest
    while current != nil:
      path.add(current)
      outgoing = outgoingEdges(current)
      if len(outgoing) == 0:
        break  // end node
      else if len(outgoing) == 1:
        current = outgoing[0].Dest
      else:
        // current is a nested split — recursively find its merge point
        // and continue from after the merge
        nestedMerge = findMergePoint(current, activities, sequenceFlows)
        path.add(nestedMerge)
        nestedOut = outgoingEdges(nestedMerge)
        if len(nestedOut) == 1:
          current = nestedOut[0].Dest
        else:
          break  // nested merge is also a split (shouldn't happen in structured workflows)
    branchPaths.append(path)

  // find first common node across all branches (in traversal order)
  common = set(branchPaths[0])
  for path in branchPaths[1:]:
    common = common.intersect(set(path))

  // return the closest common node to the split (first in traversal order of any branch)
  for node in branchPaths[0]:
    if node in common:
      return node
  error("no merge point found — malformed workflow graph")
```

This recursion terminates because Mendix enforces structured (well-nested) workflows — each split has a matching merge, and cycles are not allowed.

#### 1.4 Activity Addressing

Activities are referenced by their **Caption** (display name). When duplicate captions exist:

- **Error by default**: If the caption matches multiple activities, the command fails with `"ambiguous activity name 'X' — matches N activities. use positional syntax: activity 'X' AT N (1-based)"`.
- **Positional disambiguation**: `set activity 'doValidation' AT 2 page Module.NewPage` targets the second activity named "doValidation" in flow order.

This is consistent with how `drop outcome` uses the outcome name (unique per UserTask).

**DESCRIBE output for ALTER support**: `describe workflow` will annotate each activity with its caption in a comment, making it easy to copy captions for ALTER commands:

```sql
-- DESCRIBE WORKFLOW Module.Onboarding output:
create workflow Module.Onboarding ...
  call microflow validate Module.ValidateInput    -- caption: "validate"
  user task review 'Manager Review'               -- caption: "review"
    page Module.ReviewPage
  ...
```

##### REPLACE ACTIVITY

Swap an activity in place, preserving incoming/outgoing edges:

```sql
alter workflow Module.MyWorkflow
  replace activity callMf1 with call microflow newValidate Module.NewValidate;
```

**BSON transformation**:
```
Step 1: Record all incoming/outgoing edges of old activity
Step 2: delete old activity from Activities array
Step 3: create new activity, append to Activities array
Step 4: Repoint all recorded edges to new activity ID
```

**Precondition**: Target has exactly one incoming and one outgoing edge (same as DROP ACTIVITY).

#### 1.5 Operations NOT Supported

These require `create or modify` to rebuild the workflow:
- Moving activities between branches
- Converting a linear activity into a Decision/ParallelSplit
- Merging or splitting branches
- Reordering activities within a branch

#### 1.6 Grammar

```antlr
alterWorkflowStatement
    : alter workflow qualifiedName alterWorkflowAction+
    ;

alterWorkflowAction
    : set workflowProperty                                          // workflow-level property
    | set activity activityRef activityProperty                     // activity-level property
    | insert after activityRef workflowActivity                     // linear insert
    | insert outcome STRING_LITERAL on activityRef workflowBlock    // user task outcome
    | insert path on activityRef workflowBlock                      // parallel path
    | insert branch on activityRef condition STRING_LITERAL workflowBlock  // decision branch
    | insert boundary event boundaryEventSpec on activityRef workflowBlock
    | drop activity activityRef                                     // linear delete
    | drop outcome STRING_LITERAL on activityRef                    // outcome delete
    | drop path STRING_LITERAL on activityRef                       // parallel path delete (by first activity caption)
    | drop branch STRING_LITERAL on activityRef                     // decision branch delete
    | drop boundary event on activityRef                            // boundary event delete
    | replace activity activityRef with workflowActivity            // swap activity in place
    ;

// activity reference — caption with optional positional disambiguation
activityRef
    : IDENTIFIER (AT INTEGER_LITERAL)?
    | STRING_LITERAL (AT INTEGER_LITERAL)?
    ;

workflowProperty
    : display STRING_LITERAL
    | description STRING_LITERAL
    | export level IDENTIFIER
    | due date STRING_LITERAL
    | overview page qualifiedName
    | parameter VARIABLE COLON qualifiedName
    ;

activityProperty
    : page qualifiedName
    | description STRING_LITERAL
    | targeting microflow qualifiedName
    | targeting xpath STRING_LITERAL
    | due date STRING_LITERAL
    ;

// workflowActivity: reuses the same activity syntax from create workflow
// (call microflow, user task, decision, parallel split, etc.)

workflowBlock
    : LBRACE workflowActivity* RBRACE
    ;
```

**Note**: `workflowActivity` reuses the existing CREATE WORKFLOW activity syntax defined in `MDLParser.g4` (`workflowActivityDef` rule). `LBRACE`/`RBRACE` reference existing lexer tokens.

### Part 2: Workflow Cross-References

#### 2.1 Reference Types

Extend `catalog/builder.go` `buildRefs()` to track workflow references in the `refs` table:

| SourceType | TargetType | RefKind | Scenario |
|------------|------------|---------|----------|
| workflow | microflow | `call` | CALL MICROFLOW activity |
| workflow | workflow | `call` | CALL WORKFLOW activity |
| workflow | entity | `uses` | Parameter entity |
| workflow | page | `uses` | UserTask PAGE, OverviewPage |
| workflow | microflow | `uses` | UserTask TARGETING MICROFLOW |

**Note**: The reverse direction (microflow → workflow via CallWorkflowAction) should already be tracked if microflow refs are complete. Verify and add if missing.

#### 2.2 Implementation

In `catalog/builder_workflows.go`, extend the existing `buildWorkflows()` function to also emit `refs` rows:

```go
func (b *Builder) buildWorkflowRefs(wf *workflows.Workflow, moduleName string) {
    qn := moduleName + "." + wf.Name

    // parameter entity reference
    if wf.Parameter != nil && wf.Parameter.Entity != "" {
        b.insertRef(qn, wf.Parameter.Entity, "uses", "workflow", "entity")
    }

    // overview page reference
    if wf.OverviewPage != "" {
        b.insertRef(qn, wf.OverviewPage, "uses", "workflow", "page")
    }

    // Recursively scan activities
    b.buildActivityRefs(qn, wf.Flow.Activities)
}

func (b *Builder) buildActivityRefs(workflowQN string, activities []workflows.Activity) {
    for _, a := range activities {
        switch act := a.(type) {
        case *workflows.CallMicroflowTask:
            b.insertRef(workflowQN, act.MicroflowQN, "call", "workflow", "microflow")
        case *workflows.CallWorkflowTask:
            b.insertRef(workflowQN, act.WorkflowQN, "call", "workflow", "workflow")
        case *workflows.UserTask:
            if act.Page != "" {
                b.insertRef(workflowQN, act.Page, "uses", "workflow", "page")
            }
            if act.TargetingMicroflow != "" {
                b.insertRef(workflowQN, act.TargetingMicroflow, "uses", "workflow", "microflow")
            }
            // Recurse into outcome sub-flows
            for _, outcome := range act.Outcomes {
                b.buildActivityRefs(workflowQN, outcome.Flow.Activities)
            }
        }
        // Recurse into boundary event sub-flows
        for _, be := range a.BoundaryEvents() {
            b.buildActivityRefs(workflowQN, be.Flow.Activities)
        }
    }
}
```

#### 2.3 Effect on Existing Commands

No new commands needed. Existing commands automatically pick up workflow refs:

```sql
-- Shows workflows that call this microflow
show callers of Module.SomeMicroflow;

-- Shows all microflows/workflows/pages referenced by this workflow
show callees of Module.MyWorkflow;

-- Shows workflows that reference this entity
show references to Module.SomeEntity;

-- Includes workflows in impact analysis
show impact of Module.SomeEntity;
```

## Implementation Phases

| Phase | Scope | Complexity | Dependencies |
|-------|-------|------------|--------------|
| **P1** | Workflow Cross-References in catalog builder | Low | None — independent, quick win |
| **P2** | ALTER WORKFLOW SET (properties only) | Low | Grammar + executor |
| **P3** | ALTER WORKFLOW SET ACTIVITY (activity properties) | Low | P2 |
| **P4** | INSERT AFTER / DROP ACTIVITY (linear graph ops) | Medium | P2 + merge point algorithm |
| **P5** | INSERT/DROP OUTCOME, PATH, BRANCH (split graph ops) | High | P4 + merge point discovery |
| **P6** | INSERT/DROP BOUNDARY EVENT | Medium | P4 |

**Recommended order**: P1 first (quick, independent), then P2→P3→P4→P5→P6.

## Testing Strategy

- **Cross-References**: Add workflow entries to existing `buildRefs` test suite. Verify `show callers/callees` output includes workflows after `refresh catalog full`.
- **ALTER SET**: Roundtrip test — `describe` → `alter set` → `describe` → compare.
- **Graph ops**: Create workflow via `create workflow`, apply ALTER operations, `mx check` to validate, `describe` to verify structure.
- **Edge cases**: INSERT on split nodes (must reject), DROP on merge nodes (must reject), duplicate outcome names, empty blocks.

## Compatibility

- **Backward compatible**: No existing syntax changes.
- **Forward compatible**: ALTER WORKFLOW statements fail gracefully on older mxcli versions with a parse error.
- **Studio Pro**: All BSON output must pass `mx check` validation.
