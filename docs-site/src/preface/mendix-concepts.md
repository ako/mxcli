# Mendix Concepts for Newcomers

This page explains the core Mendix concepts you'll encounter when using mxcli. If you're familiar with web frameworks, relational databases, or backend development, most of these will map to things you already know.

## The Big Picture

A **Mendix application** is a model-driven app. Instead of writing code in files, developers build applications visually in **Studio Pro** (the Mendix IDE). The entire application model -- data structures, logic, UI, security -- is stored in a single binary file called an **MPR** (Mendix Project Resource).

mxcli lets you read and modify that MPR file using text commands, without opening Studio Pro.

## Modules

A **module** is the top-level organizational unit, like a package in Go or a namespace in C#. Each module has its own:

- Domain model (data structures)
- Microflows and nanoflows (logic)
- Pages (UI)
- Enumerations (constant sets)
- Security settings

A typical project has a few custom modules (`Sales`, `Admin`, `Integration`) plus system modules provided by the platform (`System`, `Administration`).

```sql
SHOW MODULES;
```

## Domain Model

The **domain model** is the data layer of a module. If you know relational databases, the mapping is straightforward:

| Mendix Concept | Relational Equivalent | MDL Syntax |
|----------------|----------------------|------------|
| Entity | Table | `CREATE ENTITY` |
| Attribute | Column | Inside entity definition |
| Association | Foreign key / Join table | `CREATE ASSOCIATION` |
| Generalization | Table inheritance | `EXTENDS` |
| Enumeration | Enum / Check constraint | `CREATE ENUMERATION` |

### Entities

An **entity** defines a data type. There are several kinds:

- **Persistent** -- stored in the database (the default and most common)
- **Non-persistent** -- exists only in memory during a user session, useful for form state or temporary calculations
- **View** -- backed by an OQL query, like a database view

```sql
CREATE PERSISTENT ENTITY Sales.Customer (
    Name: String(200) NOT NULL,
    Email: String(200),
    IsActive: Boolean DEFAULT true
);
```

### Associations

An **association** is a relationship between two entities. Think of it as a foreign key.

- **Reference** -- many-to-one or one-to-one (a foreign key column on the "from" entity)
- **ReferenceSet** -- many-to-many (a join table under the hood)

```sql
CREATE ASSOCIATION Sales.Order_Customer
    FROM Sales.Order TO Sales.Customer
    TYPE Reference;
```

### Generalization

Entities can inherit from other entities using **generalization** (the `EXTENDS` keyword). The child entity gets all parent attributes. This is commonly used with system entities like `System.Image` (for file uploads) or `System.User` (for user accounts).

```sql
CREATE PERSISTENT ENTITY Sales.ProductImage EXTENDS System.Image (
    Caption: String(200)
);
```

## Microflows and Nanoflows

**Microflows** are the server-side logic of a Mendix app. They're visual flowcharts in Studio Pro, but in MDL they read like imperative code with activities:

- **Retrieve** data from the database
- **Create**, **Change**, **Commit**, **Delete** objects
- **Call** other microflows or external services
- **If/Else** branching, **Loop** iteration
- **Show Page**, **Log Message**, **Validation Feedback**

```sql
CREATE MICROFLOW Sales.CreateOrder(
    DECLARE $Customer: Sales.Customer
)
RETURN Boolean
BEGIN
    CREATE $Order: Sales.Order (
        OrderDate = [%CurrentDateTime%],
        Status = 'Draft'
    );
    CHANGE $Order (
        Sales.Order_Customer = $Customer
    );
    COMMIT $Order;
    RETURN true;
END;
```

**Nanoflows** are the client-side equivalent. They run in the browser (or native mobile app) and are useful for offline-capable logic and low-latency UI interactions. They have the same syntax but fewer available activities (no database transactions, no direct service calls).

## Pages

**Pages** define the user interface. A page has:

- A **layout** -- the outer frame (header, sidebar, footer) shared across pages
- **Widgets** -- the UI components inside the page

Widgets are nested in a tree. Common widget types:

| Widget | Purpose | Analogy |
|--------|---------|---------|
| DataView | Displays one object | A form |
| DataGrid | Displays a list as a table | An HTML table with sorting/search |
| ListView | Displays a list with custom layout | A repeating template |
| TextBox | Text input bound to an attribute | An `<input>` field |
| Button | Triggers an action | A `<button>` |
| Container | Groups other widgets | A `<div>` |
| LayoutGrid | Responsive column layout | CSS grid / Bootstrap row |

```sql
CREATE PAGE Sales.CustomerOverview
(
    Title: 'Customers',
    Layout: Atlas_Core.Atlas_Default
)
{
    DATAGRID dgCustomers (DataSource: DATABASE Sales.Customer) {
        COLUMN colName (Attribute: Name, Caption: 'Name')
        COLUMN colEmail (Attribute: Email, Caption: 'Email')
        COLUMN colActive (Attribute: IsActive, Caption: 'Active')
    }
}
```

### Snippets

A **snippet** is a reusable page fragment. You define it once and embed it in multiple pages using a SnippetCall. Think of it as a component or partial template.

## Security

Mendix uses a role-based access control model:

1. **Module roles** are defined per module (e.g., `Sales.Admin`, `Sales.User`)
2. **User roles** are defined at the project level and aggregate module roles
3. **Access rules** control what each module role can do with entities (CREATE, READ, WRITE, DELETE) and which microflows/pages they can access

```sql
CREATE MODULE ROLE Sales.Manager;

GRANT CREATE, READ, WRITE ON Sales.Customer TO Sales.Manager;
GRANT EXECUTE ON MICROFLOW Sales.CreateOrder TO Sales.Manager;
GRANT VIEW ON PAGE Sales.CustomerOverview TO Sales.Manager;
```

## Navigation

**Navigation profiles** define how users move through the app. There are profiles for responsive web, tablet, phone, and native mobile. Each profile has:

- A **home page** (the landing page after login)
- A **menu** with items that link to pages or microflows

## Workflows

**Workflows** model long-running processes with human tasks. Think of them as state machines for approval flows, onboarding processes, or multi-step procedures. A workflow has:

- **User tasks** -- steps that require human action
- **Decisions** -- branching based on conditions
- **Parallel splits** -- concurrent paths

Workflows complement microflows: microflows handle immediate logic, workflows handle processes that span hours or days.

## How It All Fits Together

```
Project
├── Module: Sales
│   ├── Domain Model
│   │   ├── Entity: Customer (Name, Email, IsActive)
│   │   ├── Entity: Order (OrderDate, Status)
│   │   └── Association: Order_Customer
│   ├── Microflows
│   │   ├── CreateOrder
│   │   └── ApproveOrder
│   ├── Pages
│   │   ├── CustomerOverview
│   │   └── OrderEdit
│   ├── Enumerations
│   │   └── OrderStatus (Draft, Active, Closed)
│   └── Security
│       ├── Module Role: Manager
│       └── Module Role: Viewer
├── Module: Administration
│   └── ...
└── Navigation
    └── Responsive profile → Home: Sales.CustomerOverview
```

In mxcli, you can explore this structure with:

```sql
SHOW STRUCTURE DEPTH 2;
```

## What's Next

- [Part I: Tutorial](../tutorial/setup.md) -- hands-on walkthrough
- [Part II: The MDL Language](../language/basics.md) -- complete language guide
- [Glossary](../appendixes/glossary.md) -- alphabetical reference of all terms
