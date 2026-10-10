---
id: BENCH-002
category: Benchmark/Loop
tags: [benchmark, loop-efficiency, entity, association, enumeration, pages, microflow, security, navigation, test]
timeout: 60m
---

# BENCH-002: Field service desk (larger agent-loop benchmark)

The second fixed brief for measuring an agent session end to end
(docs/11-proposals/PROPOSAL_agent_loop_efficiency.md, lever 6). BENCH-001 is a
seven-minute run; this one is roughly 1.8× its model surface — six entities,
five associations, three enumerations, eight pages, four microflows, three
module roles — so a change whose effect is below BENCH-001's run-to-run noise
has somewhere larger to show up.

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

Pages:
- An overview page and an edit page for Fab, System, Engineer, ServiceRequest and WorkOrder (Fab_Overview, Fab_NewEdit, and so on).
- The service request edit page shows the request's work orders.
- The work order edit page shows the parts used and lets the user add, edit and remove them.

Logic:
- ACT_Request_Assign, called from an Assign button on the service request edit page: it creates a work order for the request, sets the request's Status to Assigned and commits.
- ACT_WorkOrder_Complete, called from a Complete button on the work order edit page: it sets the work order's TotalCost from its parts, sets the parent request's Status to Completed and commits both.
- Put the parts cost calculation in its own sub-microflow, SUB_WorkOrder_TotalCost, so it can be tested. It returns the sum of quantity × unit cost over the work order's parts.

Security:
- Three module roles in Service: Administrator (full access), ServiceEngineer (can read fabs and systems, can create and edit work orders and parts, can edit service requests), FabUser (can create and edit service requests, read-only on everything else). Map Administrator to the Administrator project role and the other two to the User project role.

Navigation:
- Menu items for the System, ServiceRequest and WorkOrder overview pages.

Tests:
- A microflow test file in tests/ (a .test.mdl) that checks SUB_WorkOrder_TotalCost for a work order with two parts.

When you are done, the project must pass `mx check` with no errors, and the tests must pass.

## Expected Outcome
A Service module with six entities, five associations and three enumerations;
overview and edit pages for five entities, with work orders nested in the
request page and parts nested in the work order page; two action microflows and
one tested sub-microflow; three module roles mapped to project roles; three menu
items; and a passing microflow test.

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
- microflow_exists: "Service.ACT_Request_Assign"
- microflow_exists: "Service.ACT_WorkOrder_Complete"
- microflow_exists: "Service.SUB_WorkOrder_TotalCost"
- module_role_exists: "Service.Administrator"
- module_role_exists: "Service.ServiceEngineer"
- module_role_exists: "Service.FabUser"
- navigation_has_item: true
- file_exists: "tests/*.test.mdl"
- tests_pass: "tests --local"
- mx_check_passes: true

## Acceptance Criteria
- Assigning a request creates a work order and moves the request to Assigned
- Completing a work order sets TotalCost from its parts and the request to Completed
- FabUser can create a service request but cannot edit a work order
- The microflow test exercises SUB_WorkOrder_TotalCost, not the action microflow

## Note on the checks
`enumeration_exists` is not a check kind the runner has
(`evalrunner/checks.go`), so the three enumerations are covered indirectly by
`entity_has_attribute` on the attributes that use them. If a run passes those
while building plain strings instead of enumerations, that is a gap in the
instrument, not a pass — add the check kind rather than loosening the brief.
