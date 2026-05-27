---
title: Workflow Support in mxcli
status: done
---

# Proposal: Workflow Support in mxcli

## Motivation

Workflows are a core Mendix feature (introduced in Mendix 9.0) for orchestrating multi-step business processes involving human tasks, automated actions, decisions, and parallel execution. Across our three test projects:

- **EnquiriesManagement**: 12 workflows (AgentCore: 2, BusinessProcesses: 5, WorkflowCommons: 5)
- **Evora-FactoryManagement**: 1 workflow (AltairIntegration: "WF Schedule technician appointment")
- **LatoProductInventory**: 0 workflows

Currently mxcli only *counts* workflows in `show modules` output. It cannot parse, describe, query, or create workflow documents.

## Current State

| Capability | Status |
|-----------|--------|
| Count workflows per module | Supported (`cmd_modules.go` line 293) |
| List workflows | Not supported |
| Describe/show workflow structure | Not supported |
| Catalog table for workflows | Not supported |
| Cross-references (callers/callees) | Not supported |
| MDL syntax for workflows | Not supported |
| Create/modify workflows | Not supported |

## Workflow BSON Structure (from reflection data 11.6.0)

### Top-Level Document: `workflows$workflow`

| Field | Storage Name | Type | Notes |
|-------|-------------|------|-------|
| Name | Name | String | Workflow name |
| Title | Title | String | Display title (deprecated, use WorkflowName) |
| Documentation | Documentation | String | Description |
| ExportLevel | ExportLevel | Enum (API/Hidden) | Visibility |
| Parameter | Parameter | PART (`workflows$parameter`) | Workflow context parameter |
| Flow | Flow | PART (`workflows$Flow`) | Main flow container |
| WorkflowName | WorkflowName | PART (`microflows$stringtemplate`) | Runtime name template |
| WorkflowDescription | WorkflowDescription | PART (`microflows$stringtemplate`) | Runtime description template |
| AdminPage | AdminPage | PART (`workflows$PageReference`) | Admin overview page |
| OnWorkflowEvent | OnWorkflowEvent | PART list | Event handlers |
| WorkflowMetaData | WorkflowMetaData | PART | Visual layout data |
| DueDate | DueDate | String | Due date expression |
| Excluded | Excluded | Boolean | Excluded from build |

### Activity Type Hierarchy

```
workflows$WorkflowActivity (abstract)
├── workflows$StartWorkflowActivity
├── workflows$EndWorkflowActivity
├── workflows$SingleUserTaskActivity
├── workflows$MultiUserTaskActivity
├── workflows$CallMicroflowTask
├── workflows$CallWorkflowActivity
├── workflows$ExclusiveSplitActivity
├── workflows$ParallelSplitActivity
├── workflows$MergeActivity
├── workflows$JumpToActivity
├── workflows$WaitForTimerActivity
├── workflows$WaitForNotificationActivity
├── workflows$EndOfParallelSplitPathActivity
└── workflows$EndOfBoundaryEventPathActivity
```

### Key Polymorphic Types

**User Targeting** (`workflows$UserTargeting`):
- `NoUserTargeting`, `MicroflowUserTargeting`, `MicroflowGroupTargeting`, `XPathUserTargeting`, `XPathGroupTargeting`

**Completion Criteria** (`workflows$UserTaskCompletionCriteria`):
- `ConsensusCompletionCriteria`, `MajorityCompletionCriteria`, `ThresholdCompletionCriteria`, `VetoCompletionCriteria`, `MicroflowCompletionCriteria`

**Condition Outcomes** (`workflows$ConditionOutcome`):
- `BooleanConditionOutcome`, `EnumerationValueConditionOutcome`, `VoidConditionOutcome`

**Boundary Events** (`workflows$BoundaryEvent`):
- `InterruptingTimerBoundaryEvent`, `NonInterruptingTimerBoundaryEvent`

### Real Example: FactoryManagement Workflow

The "WF Schedule technician appointment" workflow contains:
- 1 StartWorkflowActivity
- 5 EndWorkflowActivity nodes
- 9 SingleUserTaskActivity nodes (approval steps)
- 4 CallMicroflowTask nodes
- 4 JumpToActivity nodes
- 25 Flow connections
- 17 UserTaskOutcome definitions
- 9 PageReference objects (user task pages)
- 20 StringTemplate objects (task names/descriptions)

## Implementation Plan

### Phase 1: Read-Only Support (SHOW/DESCRIBE/Catalog)

**Goal**: Parse workflows, list them, describe their structure, and add to the catalog.

#### 1a. SDK Types (`sdk/workflows/`)

Create a new package with Go types for workflow documents:

```go
type workflow struct {
    model.BaseElement
    Name          string
    documentation string
    ExportLevel   string
    parameter     *parameter
    Flow          *Flow
    // ... other fields
}

type Flow struct {
    model.BaseElement
    Activities []WorkflowActivity // polymorphic
}

type WorkflowActivity interface {
    ActivityType() string
    ActivityName() string
    ActivityCaption() string
}
```

Activity types: `StartWorkflowActivity`, `EndWorkflowActivity`, `SingleUserTaskActivity`, `MultiUserTaskActivity`, `CallMicroflowTask`, `CallWorkflowActivity`, `ExclusiveSplitActivity`, `ParallelSplitActivity`, `MergeActivity`, `JumpToActivity`, `WaitForTimerActivity`, `WaitForNotificationActivity`

#### 1b. BSON Parser (`sdk/mpr/parser_workflow.go`)

Add workflow document parsing following the pattern of `parser_microflow.go`:
- Parse top-level Workflow fields
- Parse Flow and Activities array with polymorphic type dispatch
- Parse parameter mappings, outcomes, user targeting

#### 1c. Reader Methods (`sdk/mpr/reader_documents.go`)

```go
func (r *Reader) ListWorkflows() ([]*workflows.Workflow, error)
func (r *Reader) GetWorkflow(id string) (*workflows.Workflow, error)
```

#### 1d. SHOW/DESCRIBE Commands

- `show workflows [in module]` - List all workflows with activity counts
- `describe workflow Module.WorkflowName` - Full MDL-style output

Example describe output:
```sql
workflow AgentCore.WF_EnquiryProcessing
  parameter $WorkflowContext: AgentCore.Enquiry

  START start1;

  call microflow AgentCore.ACT_ClassifyEnquiry
    ($Enquiry = $WorkflowContext)
    on boolean DO
      true -> userTask1;
      false -> end1;
    end;

  user task userTask1 'Review Classification'
    page AgentCore.WF_ReviewClassification
    targeting microflow AgentCore.DS_GetReviewers
    outcomes
      'Approve' -> callMicroflow2;
      'Reject' -> end2;
    end;

  end end1;
  end end2;
end workflow;
```

#### 1e. Catalog Table (`mdl/catalog/`)

Add `CATALOG.WORKFLOWS` table:

| Column | Type | Description |
|--------|------|-------------|
| Id | TEXT | Workflow UUID |
| Name | TEXT | Workflow name |
| QualifiedName | TEXT | Module.WorkflowName |
| ModuleName | TEXT | Module name |
| Folder | TEXT | Folder path |
| Documentation | TEXT | Description |
| ExportLevel | TEXT | API or Hidden |
| ActivityCount | INTEGER | Total activities |
| UserTaskCount | INTEGER | User task count |
| MicroflowCallCount | INTEGER | Microflow call count |
| DecisionCount | INTEGER | Decision/split count |
| ParameterEntity | TEXT | Context entity QN |

Add to `objects` view UNION with `ObjectType = 'workflow'`.

#### 1f. Cross-References (refs table)

Add workflow-related reference types:
- `call_microflow` - CallMicroflowTask references a microflow
- `call_workflow` - CallWorkflowActivity references a sub-workflow
- `show_page` - UserTask references a task page
- `parameter` - Workflow parameter references an entity
- `user_targeting` - MicroflowUserTargeting references a microflow
- `admin_page` - AdminPage references a page

#### 1g. Source Generation

Add workflows to `buildSource()` in `builder_source.go` so they appear in full-text search results.

### Phase 2: Context & Navigation

- Add workflows to `show context of` command
- Add workflows to `show callers of` / `show callees of`
- Add workflows to `show impact of`
- Add `detectElementType` support for workflows in `cmd_context.go`

### Phase 3: MDL Syntax (Future)

Define MDL grammar for creating/modifying workflows:

```sql
create workflow Module.WF_ProcessOrder
  parameter $context: Module.Order
begin
  START;

  call microflow Module.ACT_ValidateOrder ($Order = $context)
    on boolean
      true -> reviewTask;
      false -> endReject;

  user task reviewTask 'Review Order'
    page Module.WF_ReviewOrder
    targeting microflow Module.DS_GetReviewers
    outcomes
      'Approve' -> endApprove;
      'Reject' -> endReject;

  end endApprove;
  end endReject;
end;
```

This is complex and should be deferred until read-only support is stable.

## Files to Create/Modify

### New Files
| File | Description |
|------|-------------|
| `sdk/workflows/workflow.go` | Workflow, Flow, Parameter types |
| `sdk/workflows/activities.go` | All activity type definitions |
| `sdk/workflows/targeting.go` | UserTargeting types |
| `sdk/workflows/outcomes.go` | Outcome and criteria types |
| `sdk/mpr/parser_workflow.go` | BSON parsing for workflow documents |
| `mdl/catalog/builder_workflows.go` | Catalog builder for workflows |

### Modified Files
| File | Changes |
|------|---------|
| `sdk/mpr/reader_documents.go` | Add ListWorkflows(), GetWorkflow() |
| `mdl/catalog/tables.go` | Add workflows table schema |
| `mdl/catalog/builder.go` | Add buildWorkflows() to pipeline |
| `mdl/catalog/builder_references.go` | Add workflow cross-references |
| `mdl/catalog/builder_source.go` | Add workflows to source generation |
| `mdl/catalog/builder_strings.go` | Extract strings from workflows |
| `mdl/catalog/catalog.go` | Add to Tables() list |
| `mdl/executor/cmd_show.go` | Add SHOW WORKFLOWS command |
| `mdl/executor/cmd_describe.go` | Add DESCRIBE WORKFLOW command |
| `mdl/executor/cmd_context.go` | Add workflow detection |

## Effort Estimate

| Phase | Scope | Complexity |
|-------|-------|-----------|
| Phase 1a-1b | SDK types + BSON parser | High (14 activity types, many polymorphic types) |
| Phase 1c | Reader methods | Low (follows existing pattern) |
| Phase 1d | SHOW/DESCRIBE | Medium (workflow-specific formatting) |
| Phase 1e | Catalog table | Low (follows java_actions pattern) |
| Phase 1f | Cross-references | Medium (multiple reference types) |
| Phase 2 | Context/navigation | Low (extend existing commands) |
| Phase 3 | MDL create syntax | High (grammar + executor) |

## Verification

```bash
make build && make test

# Test with FactoryManagement project (has 1 workflow)
./bin/mxcli -p mx-test-projects/Evora-FactoryManagement/Evora-FactoryManagement.mpr \
  -c "show workflows"

./bin/mxcli -p mx-test-projects/Evora-FactoryManagement/Evora-FactoryManagement.mpr \
  -c "describe workflow AltairIntegration.WF_Schedule_technician_appointment"

# Test with EnquiriesManagement project (has 12 workflows)
./bin/mxcli -p mx-test-projects/EnquiriesManagement/EnquiriesManagement.mpr \
  -c "show workflows"

# catalog queries
./bin/mxcli -p mx-test-projects/EnquiriesManagement/EnquiriesManagement.mpr \
  -c "refresh catalog full force; select * from CATALOG.WORKFLOWS"

# cross-references
./bin/mxcli -p mx-test-projects/EnquiriesManagement/EnquiriesManagement.mpr \
  -c "refresh catalog full force; show callers of AgentCore.ACT_ClassifyEnquiry"
```
