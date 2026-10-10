---
id: BENCH-002
category: Benchmark/Loop
tags: [benchmark, loop-efficiency, entity, association, enumeration, view-entity, pages, microflow, workflow, security, navigation, test]
timeout: 75m
---

# BENCH-002: Field service desk (larger agent-loop benchmark)

The second fixed brief for measuring an agent session end to end
(docs/11-proposals/PROPOSAL_agent_loop_efficiency.md, lever 6). BENCH-001 is a
seven-minute run; this one is roughly 2× its model surface — six entities,
one view entity, five associations, three enumerations, nine pages, five
microflows, a workflow and three module roles — so a change whose effect is
below BENCH-001's run-to-run noise has somewhere larger to show up.

It deliberately reaches the two doctypes BENCH-001 never touches. A
request-to-completion flow is what a Mendix **workflow** is for, and
reporting is what a **view entity** is for; modelling both with a status
enumeration and a microflow, as the first draft of this brief did, kept the
benchmark on the well-trodden half of the loop. Those are the doctypes whose
skills an agent has to go and read, which is the orientation surface the
benchmark exists to measure.

Same contract as BENCH-001: **keep the prompt unchanged**, because the only
thing that makes two runs comparable is that they asked for the same work. And
**run it from a terminal, not from inside an agent session** — `eval run` spawns
`claude -p --dangerously-skip-permissions` (hardcoded, `evalrunner/run.go`) and a
host that sandboxes its children refuses to create a permission-bypassed agent.

Why it is specified to this level of detail, when the brief it came from was a
paragraph: an *open* brief lets the agent choose its own scope, and scope is the
thing the cost is proportional to. Two runs of an open brief can differ by a
whole entity cluster, which swamps the effect of any loop change being tested —
the same confound that already made a 47-vs-61-call comparison unreadable. The
open version of this brief is worth running, but as a **design-quality** eval,
not as a cost A/B. Keep the two apart.

## Prompt
Build a field service and maintenance app in this Mendix project, in a new module called Service. The company builds and services semiconductor lithography systems installed at customer fabs; the app lets fab staff report system issues and lets service staff run the request-to-completion flow. Keep it simple: one happy path per process, no edge cases or exception handling.

Domain model:
- Fab: Name (required), City, Country.
- System: SerialNumber (required), Model, InstalledOn (date and time), Status (enumeration SystemStatus: Running, Degraded, Down; default Running).
- Engineer: Name (required), Email, Certified (boolean, default false).
- ServiceRequest: RequestNumber (autonumber), ReportedOn (date and time, defaults to now), Title (required), Description, Priority (enumeration Priority: Low, Medium, High; default Medium), Status (enumeration RequestStatus: New, Assigned, Completed; default New).
- WorkOrder: OrderNumber (autonumber), ScheduledOn (date and time), HoursSpent (decimal), TotalCost (decimal), Summary.
- PartUsed: Name (required), Quantity (integer), UnitCost (decimal).
- A system is installed at one fab, a service request is about one system, a work order belongs to one service request and is assigned to one engineer, and a part used belongs to one work order. Name associations the Mendix way: System_Fab, ServiceRequest_System, WorkOrder_ServiceRequest, WorkOrder_Engineer, PartUsed_WorkOrder.

Reporting:
- A view entity OpenRequestsPerFab with FabName (String(200)) and RequestCount (Integer): one row per fab, counting that fab's service requests. The rows come from the database, so this is a view entity, not a persistent entity that something fills.

Pages:
- An overview page and an edit page for Fab, System, Engineer, ServiceRequest and WorkOrder (Fab_Overview, Fab_NewEdit, and so on).
- The service request edit page shows the request's work orders.
- The work order edit page shows the parts used and lets the user add, edit and remove them.
- A read-only overview page OpenRequestsPerFab_Overview listing the view entity's rows.

Logic:
- ACT_Request_Assign: it creates a work order for the service request, sets the request's Status to Assigned and commits.
- ACT_Request_Complete: it sets the request's Status to Completed and commits.
- ACT_WorkOrder_Complete, called from a Complete button on the work order edit page: it sets the work order's TotalCost from its parts and commits.
- Put the parts cost calculation in its own sub-microflow, SUB_WorkOrder_TotalCost, so it can be tested. It returns the sum of quantity × unit cost over the work order's parts.

Workflow:
- A workflow WF_ServiceRequest whose parameter entity is ServiceRequest, driving the request from report to completion: a user task "Assign engineer" targeted at the ServiceEngineer module role, then a call to ACT_Request_Assign, then a user task "Carry out the service work" targeted at the same role, then a call to ACT_Request_Complete.
- A Start button on the service request edit page starts the workflow for that request.
- If a user task needs its own task page, create one.

Security:
- Three module roles in Service: Administrator (full access), ServiceEngineer (can read fabs and systems, can create and edit work orders and parts, can edit service requests), FabUser (can create and edit service requests, read-only on everything else). Map Administrator to the Administrator project role and the other two to the User project role.

Navigation:
- Menu items for the System, ServiceRequest and WorkOrder overview pages.

Tests:
- A microflow test file in tests/ (a .test.mdl) that checks SUB_WorkOrder_TotalCost for a work order with two parts.

When you are done, the project must pass `mx check` with no errors, and the tests must pass.

## Expected Outcome
A Service module with six persistent entities, one view entity, five
associations and three enumerations; overview and edit pages for five entities
plus a read-only page over the view, with work orders nested in the request page
and parts nested in the work order page; three action microflows and one tested
sub-microflow; a workflow over ServiceRequest with two role-targeted user tasks
driving the two action microflows; three module roles mapped to project roles;
three menu items; and a passing microflow test.

## Checks
- entity_exists: "Service.Fab"
- entity_exists: "Service.System"
- entity_exists: "Service.Engineer"
- entity_exists: "Service.ServiceRequest"
- entity_exists: "Service.WorkOrder"
- entity_exists: "Service.PartUsed"
- entity_has_attribute: "Service.System.SerialNumber"
- entity_has_attribute: "Service.System.Status"
- entity_has_attribute: "Service.ServiceRequest.Status"
- entity_has_attribute: "Service.ServiceRequest.Priority"
- entity_has_attribute: "Service.WorkOrder.HoursSpent Decimal"
- entity_has_attribute: "Service.WorkOrder.TotalCost Decimal"
- entity_has_attribute: "Service.PartUsed.Quantity Integer"
- entity_has_attribute: "Service.PartUsed.UnitCost Decimal"
- entity_exists: "Service.OpenRequestsPerFab"
- entity_has_type: "Service.OpenRequestsPerFab View"
- entity_has_attribute: "Service.OpenRequestsPerFab.RequestCount Integer"
- entity_has_attribute: "Service.OpenRequestsPerFab.FabName"
- association_exists: "Service.System_Fab"
- association_exists: "Service.ServiceRequest_System"
- association_exists: "Service.WorkOrder_ServiceRequest"
- association_exists: "Service.WorkOrder_Engineer"
- association_exists: "Service.PartUsed_WorkOrder"
- page_exists: "Service.Fab_Overview"
- page_exists: "Service.System_Overview"
- page_exists: "Service.Engineer_Overview"
- page_exists: "Service.ServiceRequest_Overview"
- page_exists: "Service.ServiceRequest_NewEdit"
- page_exists: "Service.WorkOrder_Overview"
- page_exists: "Service.WorkOrder_NewEdit"
- page_exists: "Service.OpenRequestsPerFab_Overview"
- microflow_exists: "Service.ACT_Request_Assign"
- microflow_exists: "Service.ACT_Request_Complete"
- microflow_exists: "Service.ACT_WorkOrder_Complete"
- microflow_exists: "Service.SUB_WorkOrder_TotalCost"
- workflow_exists: "Service.WF_ServiceRequest"
- module_role_exists: "Service.Administrator"
- module_role_exists: "Service.ServiceEngineer"
- module_role_exists: "Service.FabUser"
- navigation_has_item: true
- file_exists: "tests/*.test.mdl"
- tests_pass: "tests --local"
- mx_check_passes: true

## Acceptance Criteria
- Starting the workflow on a request produces an "Assign engineer" task for a ServiceEngineer
- Completing that task runs ACT_Request_Assign, so a work order exists and the request is Assigned
- Completing a work order sets TotalCost from its parts
- OpenRequestsPerFab returns one row per fab with that fab's request count
- FabUser can create a service request but cannot edit a work order
- The microflow test exercises SUB_WorkOrder_TotalCost, not the action microflow

## Note on the checks
`workflow_exists` and `entity_has_type` were added for this brief
(`evalrunner/checks.go`). Both close a hole rather than add convenience:
`microflow_exists` reads `SHOW MICROFLOWS` and never sees a workflow, so before
`workflow_exists` a workflow was scored only by `mx_check_passes` — "it
compiled", not "it is there"; and `entity_exists` matches on the qualified name
alone, so `entity_has_type: "… View"` is the only thing that fails a run which
builds an ordinary persistent entity where a view entity was asked for.

`enumeration_exists` is still not a check kind, so the three enumerations are
covered indirectly by `entity_has_attribute` on the attributes that use them. If
a run passes those while building plain strings instead, that is a gap in the
instrument, not a pass — add the check kind rather than loosening the brief.
