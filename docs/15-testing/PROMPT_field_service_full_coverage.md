# Full-coverage test prompt — ASML Field Service & Maintenance

A single paste-ready prompt that drives an agent to build one Mendix app touching
**most of what mxcli can do**: every entity flavour, both flow flavours plus rules,
workflows, REST and OData in both directions, JSON structures and mappings, three
navigation profiles including an offline phone profile, a project-owned theme,
security at production level, and pages covering most of the widget vocabulary —
charts, Atlas building blocks (cards, forms, wizards, timeline), pluggable timeline
and tree widgets including an editable tree, and Vega sparklines inside a data grid.

It extends the original **field inspection** brief rather than replacing it: the
first paragraph is unchanged, so a run of this prompt is comparable with a run of
the original.

**What it is for.** Regression breadth. One run exercises ~60 MDL statement kinds and
~15 CLI subcommands, and the [coverage checklist](#coverage-checklist) at the bottom
turns the result into a score rather than an impression. It is deliberately *wide and
shallow* — "one happy path per process" is kept from the original brief, because depth
of business logic tests the model, not the tool.

**Expected cost.** A full run is long (several hours of agent time, one Mendix app,
one `npm` widget build). For a quick smoke test, cut sections 4 (integrations) and 9
(analytics) — the rest still stands on its own.

## The prompt

````text
Build a Field Service & Maintenance application for ASML, a company that builds and
services semiconductor lithography systems installed at customer fabs. The app helps
fab and service staff report system issues and run the core request-to-completion
service flow. Keep it simple: one happy path per process, no edge cases or exception
handling.

This build is about BREADTH. Every numbered section below names constructs I want to
see in the finished app. Where a section names a specific Mendix construct, use that
construct — not an equivalent that is easier to write. If something turns out not to
be authorable from MDL, say so explicitly in FINDINGS.md rather than silently
substituting something else.

Target Mendix version: 11.13.0 or later. Run `show features` before using anything
version-gated and tell me if the project version cannot support a section.

---

1. WHO USES IT

Four user roles: Administrator, ServiceManager, FieldEngineer, FabOperator.

  - FabOperator reports a problem with a system in their fab.
  - ServiceManager triages it and approves the work.
  - FieldEngineer does the visit, on a phone, often with no connection.
  - Administrator sees everything.

2. MODULES

Four modules, so that cross-module references and per-module roles are real:

  - ServiceCore   — customers, fabs, systems, requests, work orders, parts
  - FieldOps      — inspections and the offline phone experience
  - Integrations  — REST, OData, JSON structures, mappings
  - Analytics     — readings, aggregates, charts

3. WHAT IT KEEPS TRACK OF

Persistent entities (ServiceCore): Customer, Fab, LithoSystem, ServiceRequest,
WorkOrder, Part, PartUsage. Fab belongs to a Customer, LithoSystem sits in a Fab, a
ServiceRequest is raised against a LithoSystem, a WorkOrder fulfils a ServiceRequest
and is assigned to an Account.

Part carries a SELF-ASSOCIATION (a part is made of parts) so there is a real bill of
materials three levels deep — the editable tree in section 8 edits it in place.

Persistent entities (FieldOps): Inspection (one per WorkOrder), InspectionItem
(several per Inspection, one per subsystem checked), InspectionPhoto specialising
System.Image.

Persistent entity (Analytics): SystemReading — one row per system per day, holding
OverlayNm, UptimePct and WaferCount. Seed 90 days of these; the sparklines in section
8 read from them.

Non-persistent entities — I want all three of these, for three different reasons:
  - ServiceCore.RequestFilter — the dashboard's search/filter object
  - Integrations.PartsAvailability — the shape a REST response maps into
  - Analytics.ChartPayload — holds the JSON string a Vega chart binds to

View entity — Analytics.SystemMonthlyKPI, defined with OQL over SystemReading:
system, year, month, average overlay, average uptime, total wafers. This is what the
published OData feed in section 4 exposes and what one chart in section 9 reads.

Enumerations for Severity, RequestStatus, OrderStatus, Subsystem and SystemPlatform
(EUV, DUV_Immersion, DUV_Dry).

Also: a regular expression document for the system serial number
(`^(EUV|DUV)-[0-9]{4}-[A-Z]{2}$`), a validation rule binding it to
LithoSystem.SerialNumber, a range validation rule on InspectionItem.MeasuredValue,
and required + unique on ServiceRequest.RequestNumber.

4. INTEGRATIONS — BOTH DIRECTIONS, BOTH PROTOCOLS

  a. Consumed REST. A parts-availability API, called with the system serial number.
     Define the JSON structure from a sample payload, an import mapping onto
     Integrations.PartsAvailability, and a REST client operation that calls it. Use
     the mock-rest-apis skill to stand up something to call.

  b. Published REST. Integrations.ServiceIntake/v1 with a `requests` resource and a
     POST operation, so a fab's own MES can file an issue. The request body maps in
     through an import mapping; the response goes out through an export mapping.

  c. Consumed OData. An OData client plus external entities for a customer master
     held outside the app. Point it at a public OData v4 service if nothing better
     is reachable, and say in FINDINGS.md what you pointed it at.

  d. Published OData. Integrations.AnalyticsFeed/v1 publishing the VIEW ENTITY
     Analytics.SystemMonthlyKPI. One chart in section 9 consumes this by URL, so the
     two halves have to actually line up.

5. THE PROCESSES

Workflow — ServiceCore.ServiceRequestApproval, started when a request is critical:
a user task for ServiceManager, a decision on its outcome, a parallel split that
assigns an engineer and reserves parts at the same time, then a join and an end.
Happy path only.

Rule — ServiceCore.IsCriticalRequest, returning Boolean, called from a decision in
the microflow that creates the request. A rule, not a sub-microflow.

Microflows — at least: create a service request (with validation and a user-visible
message), turn an approved request into a work order, a datasource microflow feeding
one of the pages, a microflow that builds the Vega JSON payload for a chart, one that
calls the REST operation from 4a, and an after-startup microflow that seeds demo data
and RETURNS BOOLEAN.

Nanoflows — the offline phone experience runs on nanoflows only: start a visit, mark
an inspection item passed or failed, submit the inspection. Plus whatever the theme
switcher installs.

Scheduled event — a nightly rollup that recomputes each system's trend payload.

Queue — a parts-ordering queue.

6. NAVIGATION — THREE PROFILES, ONE OF THEM OFFLINE

  - Responsive: home is the dashboard, with a role-based home for FieldEngineer,
    a menu with icons and a sign-out item, and a login page.
  - Tablet: the same app with a shorter menu.
  - PhoneOffline: the field engineer's app. Home is the visit list. Three menu items.
    Every entity it needs gets an explicit sync mode — all of, where with an XPath
    (the engineer's own work orders), and never for the things it must not download.
    Set the partial-sync-error behaviour explicitly.

    An offline profile downloads nothing until each entity has a sync mode, and every
    page it can reach is restricted to nanoflows. Get both right.

Also create a standalone menu document and point a menu widget at it.

7. LOOK AND FEEL

Create a project-owned theme called `asml` from one of the built-in themes, seeded
with an ASML-ish palette (deep blue, a warm accent, light ground). Apply it as a
switchable set alongside one built-in theme with `--variant auto`, and install the
runtime theme switcher so I can flip it in the browser. Show me a screenshot in both
light and dark.

8. PAGES AND WIDGETS

I want most of the widget vocabulary represented. Concretely:

  a. ServiceCore.Dashboard — a page header building block, a row of four KPI CARDS
     built from the Atlas Card building blocks, a column chart (requests by
     severity), a line chart (uptime trend), and an Alert building block.

  b. ServiceCore.SystemOverview — a DataGrid2 with text, dropdown, date and number
     filters, sorting, row selection, and an action column. One column holds a
     SPARKLINE PER ROW: install the mendix-vega-charts skill pack, build its widget,
     and use the sparkline-cell spec (126x26) inside the column's content, bound to a
     per-system trend attribute that the section 5 rollup fills. Do not assemble the
     chart payload in the widget — the microflow emits rows, the spec draws them.

  c. ServiceCore.ServiceRequest_Overview — the older data grid, with search fields
     and control bar buttons, so both grid generations are covered.

  d. ServiceCore.ServiceRequest_NewEdit — an Atlas FORM building block plus textbox,
     textarea, date picker, dropdown over an enumeration, radio buttons, check box,
     combo box and a reference selector. Put the save/cancel footer in a FRAGMENT and
     reuse it on every edit page.

  e. ServiceCore.WorkOrder_MasterDetail — the Master_Detail building block, a list
     view, a data view, a tab container with three tab pages, and a group box.

  f. FieldOps.Inspection_Wizard — a Wizard building block with steps, a SNIPPET used
     on three different pages, an image upload, a dynamic image and a static image,
     and a gallery with a template.

  g. FieldOps.Phone_MyVisits and FieldOps.Phone_Inspection — the two offline pages.
     List item building blocks, nanoflow actions only.

  h. Analytics.ChartsGallery — three more Vega charts from the pack's specs (line
     time series, calendar heatmap, small-multiples table), each in a data view over
     the non-persistent ChartPayload, PLUS one chart whose data comes from the
     published OData feed BY URL rather than from an attribute.

  i. ServiceCore.RequestTimeline — the TIMELINE WIDGET (the pluggable one,
     com.mendix.widget.web.timeline.Timeline), fed by a datasource microflow over a
     request's history: reported, triaged, approved, visited, signed off. Put the
     Atlas Timeline BUILDING BLOCK on the same page underneath it, so the pair shows
     the difference between the live widget and the static composition — they are not
     the same thing and I want both.

  j. ServiceCore.SystemExplorer — TREES, using the TREE NODE widget
     (com.mendix.widget.web.treenode.TreeNode):

       - a read-only tree down Customer > Fab > LithoSystem, with a count and a
         status icon on each header;
       - an EDITABLE tree over the Part bill of materials from section 3, three
         levels deep, with a textbox and a number input inside each node's content
         so a part's quantity and label are edited in place and saved from one
         footer button.

     Tree Node nests rather than recurses — a level is a widget, not a loop — so fix
     the depth at three and tell me in FINDINGS.md what unbounded depth would need.

  k. One custom LAYOUT created from MDL (a full-width variant), used by at least one
     page, alongside the project-owned layout the app is scaffolded with and a popup
     layout for the edit pages.

9. SECURITY

Production security level. Module roles in all four modules mapped to the four user
roles. Entity access with XPath constraints — a FieldEngineer sees only their own
work orders, a FabOperator only their own fab's systems. Attribute-level access where
it matters. Page access and microflow access set per module role. Demo users for each
role so I can log in as each of them.

10. DATA

Seed enough that every page has something to show: 3 customers, 5 fabs, 12 systems,
90 days of readings per system, 20 requests, 15 work orders, 30 inspection items.

11. WHEN IS IT DONE

  - Every MDL script passes `mxcli check` including `--references`.
  - The project builds with zero errors.
  - `mxcli lint` and `mxcli report` run, with the numbers recorded in FINDINGS.md.
  - A `.test.mdl` suite of at least eight tests passes under `mxcli test --local`,
    including at least one using `@cleanup rollback`.
  - The app boots under `mxcli run --local` and I get screenshots of the dashboard,
    the grid with sparklines, the chart gallery, the wizard and the phone pages.
  - Re-running every script changes nothing: `git status` is clean afterwards.
  - The plan is recorded with `mxcli brain`, with requirements anchored at what
    implements them.

12. THINGS THAT WILL BITE YOU

Read the relevant skill before writing each part. Specifically:

  - `Type`, `ID`, `GUID`, `CreatedDate`, `ChangedDate`, `Owner` and `ChangedBy` are
    reserved by the platform for entity members. Quoting does not make them legal.
  - One list operation per activity — no nesting a FILTER inside a COUNT.
  - `retrieve ... limit 1` binds a single object, not a one-element list.
  - The after-startup microflow must return Boolean.
  - Offline pages can call nanoflows only.
  - Association access rules go on the FROM entity, never the TO entity.
  - Single quotes inside a Vega spec must be doubled in an MDL string.
  - Never hand-edit BSON to work around something. Report it instead.

Work in slices, commit after each one, and keep FINDINGS.md updated with anything
that surprised you or that mxcli could not do.
````

## Coverage checklist

Score a run by ticking what actually ended up in the model. A miss is a finding: either
the agent skipped it or mxcli could not author it, and those are different bugs.

| Area | Expected in the finished app |
|---|---|
| Entities | persistent, non-persistent (×3 uses), **view entity** (OQL), specialisation of `System.Image`, enumerations, associations across modules |
| Attribute constraints | regular expression document, regex validation rule, range validation rule, required, unique, calculated attribute |
| Flows | microflows, **nanoflows**, **rule** returning Boolean, workflow with user task + decision + parallel split, scheduled event, queue |
| Integration out | published REST service (resource + POST + path params), **published OData** over the view entity |
| Integration in | REST client operation, **consumed OData** + external entities |
| Mappings | JSON structure, **import mapping**, **export mapping** |
| Navigation | Responsive + Tablet + **PhoneOffline**, role-based home, menu icons, sign-out item, login page, **sync modes per entity** (`all` / `where [xpath]` / `never`), `on sync error`, standalone menu document |
| Pages | DataGrid2 + legacy datagrid, gallery, list view, data view, tab container, group box, master-detail, wizard, timeline, snippet, **fragment**, custom **layout**, popup layout |
| Widgets | textbox, textarea, date picker, dropdown, combo box, radio buttons, check box, reference selector, static + dynamic image, action/link buttons, filters (text/dropdown/date/number) |
| Pluggable widgets | **Timeline widget** (datasource-fed), **Tree Node** read-only hierarchy, **Tree Node editable tree** over the Part self-association, Vega chart widget |
| Charts | Studio Pro column + line chart, **Vega sparkline inside a grid column**, Vega line/heatmap/small-multiples, one Vega chart fed **by URL** from the published OData feed |
| Building blocks | Cards, Forms, Master_Detail, Wizard, Timeline (the static one, beside the widget), Alert, page header, list items |
| Theming | `theme create` from a palette, switchable set, `--variant auto`, switcher installed, dark verified |
| Security | production level, four user roles, module roles per module, XPath entity access, attribute access, page + microflow access, demo users |
| Tooling | `check --references`, build, `lint`, `report`, `test --local`, `run --local --screenshot`, `brain plan`, catalog refresh, idempotent re-run |

## Notes for whoever runs this

- **The sparkline column is the sharpest single test in here.** It needs the skill
  pack installed and its widget built (`npm ci && npm run build`, committed `.mpk`),
  a per-row string attribute filled by a microflow, the spec embedded in an MDL
  string with its single quotes doubled, and the whole thing placed inside a
  DataGrid2 column's content. Four layers, any of which can fail quietly — the grid
  renders fine with an empty cell.
- **The OData-by-URL chart is the second.** It only works if the published feed and
  the spec's `format.property` unwrapping agree, which ties section 4d to section 8h
  across the whole build.
- **The editable tree is the third.** A Tree Node whose content holds input widgets
  bound through a self-association is where a page can serialize to valid BSON, pass
  `mx check`, build clean and still render an empty node — the class of failure that
  `.claude/skills/verify-in-runtime.md` exists for. mxcli ships templates for both
  `timeline.json` and `treenode.json`, so CE0463 here means the template and the
  project's installed widget version disagree, not that the tree is unauthorable;
  `mxcli widget describe` reports what the project actually has.
- **Idempotence is checked, not assumed.** The "re-run changes nothing" gate is what
  catches a write path that bypasses `canon.Reconcile`; run it, don't take it on
  trust.
- **Keep the original brief's first paragraph verbatim** if you edit this prompt, so
  runs stay comparable against the original field-inspection app.
