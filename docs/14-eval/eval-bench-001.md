---
id: BENCH-001
category: Benchmark/Loop
tags: [benchmark, loop-efficiency, entity, association, pages, microflow, security, navigation, test]
timeout: 45m
---

# BENCH-001: Order desk (agent-loop benchmark)

The fixed brief for measuring an agent session end to end
(docs/11-proposals/PROPOSAL_agent_loop_efficiency.md, lever 6). Keep it
unchanged: its value is that runs before and after a change are comparable.
Run it with `mxcli eval run docs/14-eval/eval-bench-001.md --version 11.15.0`;
the run writes `diag session-report` for the transcript.

**Run it from a terminal, not from inside an agent session.** `eval run` spawns
`claude -p --dangerously-skip-permissions` (hardcoded, `evalrunner/run.go`), and
a host that sandboxes its children refuses to create a permission-bypassed agent
— measured on Claude Code on the web, where the denial reads "Create Unsafe
Agents". The credentials and a plain `claude -p` are fine there; it is the bypass
flag that is refused, and there is no flag to omit it. Supplying the child a
pre-approved settings file instead is the same bypass by another route, so the
honest answer is to run the benchmark where an unattended agent is allowed.

## Prompt
Build a small order desk app in this Mendix project, in a new module called Sales.

Domain model:
- Customer: Name (required), Email, Phone.
- Product: Name (required), Price (decimal), Active (boolean, default true).
- Order: OrderNumber (autonumber), OrderDate (date and time, defaults to now), Status (enumeration OrderStatus: Draft, Submitted, Cancelled; default Draft), Total (decimal).
- OrderLine: Quantity (integer), UnitPrice (decimal).
- An order belongs to one customer, an order line belongs to one order, and an order line refers to one product. Name associations the Mendix way: Order_Customer, OrderLine_Order, OrderLine_Product.

Pages:
- An overview page and an edit page for Customer, Product and Order (Customer_Overview, Customer_NewEdit, and so on).
- The order edit page shows the order's lines and lets the user add, edit and remove them.

Logic:
- A microflow ACT_Order_Submit, called from a Submit button on the order edit page. It refuses an order without a customer or without lines, or with a line whose quantity is not positive, and shows validation feedback for each problem. Otherwise it sets each line's UnitPrice from its product, sets Total to the sum of quantity × unit price, sets Status to Submitted and commits.
- Put the total calculation in its own sub-microflow so it can be tested.

Security:
- Two module roles in Sales: Administrator (full access) and SalesRep (can create and edit customers and orders, can only read products). Map them to the Administrator and User project roles.

Navigation:
- Menu items for the Customer, Product and Order overview pages.

Tests:
- A microflow test file in tests/ (a .test.mdl) that checks the total calculation for an order with two lines.

When you are done, the project must pass `mx check` with no errors, and the tests must pass.

## Expected Outcome
A Sales module with four entities and three associations, overview and edit pages for three entities, a validated submit microflow with a sub-microflow for the total, two module roles mapped to project roles, three menu items, and a passing microflow test.

## Checks
- entity_exists: "Sales.Customer"
- entity_exists: "Sales.Product"
- entity_exists: "Sales.Order"
- entity_exists: "Sales.OrderLine"
- entity_has_attribute: "Sales.Product.Price Decimal"
- entity_has_attribute: "Sales.OrderLine.Quantity Integer"
- entity_has_attribute: "Sales.Order.Status"
- association_exists: "Sales.Order_Customer"
- association_exists: "Sales.OrderLine_Order"
- association_exists: "Sales.OrderLine_Product"
- page_exists: "Sales.Customer_Overview"
- page_exists: "Sales.Product_Overview"
- page_exists: "Sales.Order_Overview"
- page_exists: "Sales.Order_NewEdit"
- microflow_exists: "Sales.ACT_Order_Submit"
- module_role_exists: "Sales.Administrator"
- module_role_exists: "Sales.SalesRep"
- navigation_has_item: true
- file_exists: "tests/*.test.mdl"
- tests_pass: "tests --local"
- mx_check_passes: true

## Acceptance Criteria
- Submitting an order without lines shows validation feedback and leaves it Draft
- Submitting a valid order sets Total and Status
- SalesRep cannot edit products
- The microflow test exercises the total sub-microflow
