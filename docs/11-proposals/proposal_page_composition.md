---
title: Page Composition and Partial Updates
status: proposed
---

# Page Composition and Partial Updates

## Status: Proposal

## Problem Statement

Large MDL page scripts become unwieldy to write, read, and maintain. Currently, the only way to create or modify a page is to specify the entire page structure in a single `create page` or `create or replace page` statement. This creates several issues:

1. **Unsafe editing of pages with unsupported widgets** - `create or replace page` rebuilds the entire page from MDL. Any widget type not yet supported by the MDL writer is **lost**. `describe page` renders unsupported widget types as comments (`-- pages$SomeType (name)`), so round-tripping a page silently drops content. With partial updates (ALTER PAGE), we can add, change, or remove specific widgets while leaving unsupported parts of the page untouched.
2. **No reusability** - Common widget patterns (save/cancel buttons, form layouts) must be copy-pasted
3. **All-or-nothing updates** - Changing a single property requires rewriting the entire page
4. **Large script files** - Complex pages result in hundreds of lines of MDL
5. **No incremental development** - Can't build pages piece by piece in REPL sessions

## Goals

1. **Safe partial editing** - Modify pages containing unsupported widget types without data loss
2. **Composability** - Break large pages into smaller, reusable MDL fragments
3. **Partial Updates** - Modify specific widgets or properties without replacing entire pages
4. **Incremental Creation** - Build pages step by step

## Design Principles

- Widget names are unique within a page (flat namespace, no nested paths needed)
- Fragments are MDL-level constructs (not Mendix Studio Pro snippets)
- Fragments are script-scoped (transient, not persisted)
- Operations should be atomic and validate against current page state
- Property assignments must validate against widget type capabilities
- Syntax should feel natural alongside existing MDL

---

## Relationship to Existing Features

This proposal complements existing features. `update widgets` (bulk property updates) and `alter styling on page` (partial styling updates) are already implemented and prove that partial page modification works.

| Feature | Status | Scope | Target | Use Case |
|---------|--------|-------|--------|----------|
| `update widgets set ... where ...` | **Implemented** | Project-wide or module | Multiple widgets by filter | "Disable labels on all comboboxes" |
| `alter styling on page ... widget ...` | **Implemented** | Single page | Widget styling by name | "Change CSS class on this container" |
| `alter page set ... on widget` | Proposed | Single page | Single widget by name | "Change this button's caption" |
| `alter page replace widget with {...}` | Proposed | Single page | Widget subtree | "Restructure this form section" |
| `define/use fragment` | Proposed | Script scope | Reusable widget groups | "Standard save/cancel buttons" |

**How they work together:**

```sql
-- Bulk update (existing): Change ALL combobox labels project-wide
update widgets
set 'showLabel' = false
where widgettype like '%combobox%'

-- Targeted update (proposed): Change ONE specific widget's property
alter page Module.CustomerEdit {
  set 'showLabel' = true on cbStatus  -- Exception to the rule above
}

-- Structural change (proposed): Replace entire section
alter page Module.CustomerEdit {
  replace footer1 with {
    use fragment NewFooterLayout
  }
}
```

---

## Part 1: Fragment Definition and Usage

### DEFINE FRAGMENT

Define reusable widget groups that can be inserted into pages:

```sql
-- Simple fragment
define fragment SaveCancelFooter as {
  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}

-- Fragment with layout structure
define fragment TwoColumnForm as {
  layoutgrid formGrid {
    row row1 {
      column colLeft (desktopwidth: 6) { }
      column colRight (desktopwidth: 6) { }
    }
  }
}

-- Fragment for consistent headings
define fragment PageHeader as {
  layoutgrid headerGrid {
    row headerRow {
      column headerCol (desktopwidth: 12) {
        dynamictext pageTitle (content: 'Page Title', rendermode: H2)
      }
    }
  }
}
```

### USE FRAGMENT

Insert a defined fragment at the current position:

```sql
create page Module.CustomerEdit
(
  params: { $Customer: Module.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvCustomer (datasource: $Customer) {
    textbox txtName (label: 'Name', attribute: Name)
    textbox txtEmail (label: 'Email', attribute: Email)

    -- Insert the reusable footer
    use fragment SaveCancelFooter
  }
}
```

### Parameterized Fragments (Future)

Future enhancement - fragments that accept parameters:

```sql
define fragment FormField($label, $attr) as {
  textbox txt$attr (label: $label, attribute: $attr)
}

-- Usage
use fragment FormField('Customer Name', 'Name')
use fragment FormField('Email Address', 'Email')
```

### Fragment Naming and Prefixes

When using fragments multiple times, use a prefix to avoid name conflicts:

```sql
define fragment SaveCancelFooter as {
  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}

-- Use with prefix to create unique names
use fragment SaveCancelFooter as customer_   -- Creates customer_footer1, customer_btnSave, customer_btnCancel
use fragment SaveCancelFooter as order_      -- Creates order_footer1, order_btnSave, order_btnCancel

-- Without prefix (only use once per page)
use fragment SaveCancelFooter
```

### Fragment Scope

- Fragments are defined at script scope (available after definition until script ends)
- Fragments are transient (not persisted in MPR, only exist during script execution)
- Fragments can reference other fragments (but no circular references)
- Fragment names must be unique within a script

### SHOW FRAGMENTS

List all defined fragments in the current session:

```sql
define fragment SaveCancelFooter as { ... }
define fragment FormHeader as { ... }

show fragments;
-- Output:
-- SaveCancelFooter
-- FormHeader
```

### DESCRIBE FRAGMENT

Show the definition of a script-defined fragment:

```sql
describe fragment SaveCancelFooter;

-- Output:
-- DEFINE FRAGMENT SaveCancelFooter AS {
--   FOOTER footer1 {
--     ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
--     ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
--   }
-- }
```

---

## Part 2: DESCRIBE FRAGMENT FROM PAGE

Extract a widget subtree from an existing page as a fragment definition. This enables the **describe → edit → replace** workflow:

### Basic Syntax

```sql
describe fragment from page Module.PageName widget widgetName;
```

### Example Workflow

```sql
-- 1. Extract part of an existing page as a fragment
describe fragment from page Module.CustomerEdit widget dvCustomer;

-- Output (can be copied, modified, and used in ALTER PAGE):
-- {
--   DATAVIEW dvCustomer (DataSource: $Customer) {
--     TEXTBOX txtName (Label: 'Name', Attribute: Name)
--     TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
--     FOOTER footer1 {
--       ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES)
--       ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
--     }
--   }
-- }

-- 2. Copy the output, modify it, then replace
alter page Module.CustomerEdit {
  replace dvCustomer with {
    dataview dvCustomer (datasource: $Customer) {
      textbox txtName (label: 'Name', attribute: Name)
      textbox txtEmail (label: 'Email', attribute: Email)
      textbox txtPhone (label: 'Phone', attribute: Phone)  -- Added new field
      footer footer1 {
        actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)  -- Added style
        actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
      }
    }
  }
}
```

### Extract and Save as Reusable Fragment

```sql
-- Extract footer from one page
describe fragment from page Module.CustomerEdit widget footer1;

-- Output can be wrapped in DEFINE FRAGMENT for reuse:
define fragment StandardFooter as {
  footer footer1 {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
  }
}

-- Now use in other pages
create page Module.OrderEdit (...) {
  dataview dvOrder (datasource: $Order) {
    -- ... fields ...
    use fragment StandardFooter
  }
}
```

### Use Cases

1. **Modify complex widgets** - Extract, edit in your editor, replace
2. **Create fragment library** - Extract patterns from Studio Pro-designed pages
3. **Understand page structure** - Inspect specific sections without full DESCRIBE PAGE
4. **Refactoring** - Extract common patterns, convert to fragments

---

## Part 3: ALTER PAGE Statement

Modify existing pages without full replacement:

### Basic Syntax

```sql
alter page Module.PageName {
  -- One or more operations
}
```

### SET Property

Update widget properties by name:

```sql
alter page Module.CustomerEdit {
  -- Single property
  set caption = 'Update' on btnSave

  -- Multiple properties
  set (caption: 'Save Changes', buttonstyle: success) on btnSave

  -- Property on page itself
  set title = 'Edit Customer Record'

  -- Nested property for pluggable widgets (dot notation in quotes)
  set 'showLabel' = false on cbStatus
  set 'labelWidth' = 4 on cbStatus
}
```

### INSERT Widget

Add new widgets to existing containers:

```sql
alter page Module.CustomerEdit {
  -- Insert after a named widget
  insert after txtName {
    textbox txtMiddleName (label: 'Middle Name', attribute: MiddleName)
  }

  -- Insert before a named widget
  insert before btnSave {
    actionbutton btnValidate (caption: 'Validate', action: microflow Module.VAL_Customer)
  }

  -- Insert as first child of a container
  insert FIRST in dvCustomer {
    dynamictext formHeader (content: 'Customer Information', rendermode: H3)
  }

  -- Insert as last child of a container
  insert LAST in footer1 {
    linkbutton lnkHelp (caption: 'Help', action: show_page Module.HelpPage)
  }

  -- Insert fragment
  insert after txtEmail {
    use fragment AddressFields
  }
}
```

### DROP Widget

Remove widgets from a page:

```sql
alter page Module.CustomerEdit {
  -- Remove a single widget
  drop widget txtMiddleName

  -- Remove multiple widgets
  drop widget txtFax, txtPager

  -- Remove with IF EXISTS (no error if not found)
  drop widget if exists txtLegacyField
}
```

### REPLACE Widget

Swap a widget with new content:

```sql
alter page Module.CustomerEdit {
  -- Replace widget entirely
  replace btnCancel with {
    linkbutton lnkCancel (caption: 'Cancel', action: close_page)
  }

  -- Replace with fragment
  replace oldFooter with {
    use fragment SaveCancelFooter
  }
}
```

### MOVE Widget

Relocate a widget within the page:

```sql
alter page Module.CustomerEdit {
  -- Move to different position
  move txtPhone after txtEmail

  -- Move to different container
  move btnHelp LAST in footer1
}
```

---

## Part 4: Incremental Page Building

Build pages step by step, useful for REPL sessions:

### CREATE EMPTY PAGE

```sql
-- Create minimal page structure
create empty page Module.NewPage
(
  title: 'New Page',
  layout: Atlas_Core.Atlas_Default
)

-- Now build it up incrementally
alter page Module.NewPage {
  insert FIRST in Main {  -- Main is the layout placeholder
    layoutgrid mainGrid { }
  }
}

alter page Module.NewPage {
  insert FIRST in mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) { }
    }
  }
}

alter page Module.NewPage {
  insert FIRST in col1 {
    dynamictext heading (content: 'Welcome', rendermode: H2)
  }
}
```

---

## Part 5: Script Organization

### Multiple Statements in Sequence

```sql
-- fragments.mdl - Define reusable fragments
define fragment SaveCancelFooter as { ... }
define fragment FormHeader as { ... }
define fragment AddressFields as { ... }
/

-- customer-edit.mdl - Create the page
use script 'fragments.mdl'  -- Future: import fragments from other files

create page Module.CustomerEdit (...) {
  use fragment FormHeader
  dataview dvCustomer (datasource: $Customer) {
    textbox txtName (label: 'Name', attribute: Name)
    use fragment AddressFields
    use fragment SaveCancelFooter
  }
}
/

-- Later modifications
alter page Module.CustomerEdit {
  set title = 'Edit Customer v2'
  insert after txtName {
    textbox txtNickname (label: 'Nickname', attribute: Nickname)
  }
}
```

---

## Implementation Phases

### Phase 1: Core Fragment System
- `define fragment name as { ... }`
- `use fragment name [as prefix_]` within CREATE PAGE
- `show fragments` - list defined fragments
- `describe fragment name` - show fragment definition
- Fragment storage in executor context
- Fragment expansion during page building with prefix support

### Phase 2: DESCRIBE FRAGMENT FROM PAGE
- `describe fragment from page Module.Page widget widgetName`
- Extract widget subtree as MDL fragment syntax
- Enables the describe → edit → replace workflow

### Phase 3: ALTER PAGE Basics
- `set property = value on widgetName`
- `set (prop1: val1, prop2: val2) on widgetName`
- `insert after/before widgetName { ... }`
- `drop widget widgetName`
- `replace widgetName with { ... }`
- Page loading, modification, and saving
- Operations work on raw BSON widget trees, preserving unsupported widget types
- Property validation against widget types

### Phase 4: Advanced ALTER Operations
- `insert FIRST/LAST in containerName`
- `move widgetName after/before target`
- `drop widget if exists`

### Phase 5: Future Enhancements
- Parameterized fragments: `define fragment Name($param) as { ... }`
- `use script 'file.mdl'` for file includes
- Fragment libraries
- Conditional fragments (`use fragment X if condition`)

### Already Implemented (not in scope)
- **Bulk Widget Updates** — `update widgets set ... where ...` with module and widget-type filtering, `dry run` support. Fully working in `cmd_widgets.go`.
- **ALTER STYLING ON PAGE** — Partial styling updates on individual widgets. Working in `cmd_styling.go`. Proves the partial page modification pattern.

---

## Grammar Changes

### New Tokens (MDLLexer.g4)

```antlr
define: D E F I N E;
fragment: F R A G M E N T;
insert: I N S E R T;
before: B E F O R E;
after: A F T E R;
FIRST: F I R S T;
LAST: L A S T;
```

The following tokens already exist in the lexer: `alter`, `move`, `replace`, `with`, `empty`.

### New Rules (MDLParser.g4)

```antlr
// fragment definition
defineFragmentStatement
    : define fragment IDENTIFIER as LBRACE widgetV3* RBRACE
    ;

// fragment usage (within widget children)
useFragmentStatement
    : use fragment IDENTIFIER (as IDENTIFIER)?  // Optional prefix
    ;

// show fragments list
showFragmentsStatement
    : show fragments
    ;

// describe fragment (script-defined or from page)
describeFragmentStatement
    : describe fragment IDENTIFIER
    | describe fragment from page qualifiedName widget IDENTIFIER
    ;

// alter page statement
alterPageStatement
    : alter page qualifiedName LBRACE alterOperation+ RBRACE
    ;

alterOperation
    : setPropertyOperation
    | insertOperation
    | dropWidgetOperation
    | replaceWidgetOperation
    | moveWidgetOperation
    ;

setPropertyOperation
    : set propertyAssignment on IDENTIFIER
    | set LPAREN propertyAssignmentList RPAREN on IDENTIFIER
    | set propertyAssignment  // page-level property
    ;

insertOperation
    : insert (after | before) IDENTIFIER LBRACE widgetV3+ RBRACE
    | insert (FIRST | LAST) in IDENTIFIER LBRACE widgetV3+ RBRACE
    ;

dropWidgetOperation
    : drop widget (if exists)? identifierList
    ;

replaceWidgetOperation
    : replace IDENTIFIER with LBRACE widgetV3+ RBRACE
    ;

moveWidgetOperation
    : move IDENTIFIER (after | before) IDENTIFIER
    | move IDENTIFIER (FIRST | LAST) in IDENTIFIER
    ;
```

---

## AST Types

```go
// ast/ast_page_fragments.go

type DefineFragmentStmt struct {
    Name    string
    widgets []*WidgetV3
}

func (s *DefineFragmentStmt) isStatement() {}

type UseFragmentStmt struct {
    FragmentName string
    Prefix       string // Optional prefix for widget names
}

type ShowFragmentsStmt struct{}

func (s *ShowFragmentsStmt) isStatement() {}

type DescribeFragmentStmt struct {
    FragmentName string        // for script-defined fragments
    PageName     QualifiedName // for describe fragment from page
    WidgetName   string        // widget to extract
    FromPage     bool          // true if from page syntax
}

func (s *DescribeFragmentStmt) isStatement() {}

type AlterPageStmt struct {
    PageName   QualifiedName
    Operations []AlterOperation
}

type AlterOperation interface {
    isAlterOperation()
}

type SetPropertyOp struct {
    WidgetName  string // empty for page-level
    properties  map[string]interface{}
}

type InsertOp struct {
    position    InsertPosition // after, before, FIRST, LAST
    TargetName  string
    widgets     []*WidgetV3
}

type DropWidgetOp struct {
    WidgetNames []string
    IfExists    bool
}

type ReplaceWidgetOp struct {
    WidgetName string
    NewWidgets []*WidgetV3
}

type MoveWidgetOp struct {
    WidgetName  string
    position    InsertPosition
    TargetName  string
}

type InsertPosition string
const (
    InsertAfter  InsertPosition = "after"
    InsertBefore InsertPosition = "before"
    InsertFirst  InsertPosition = "FIRST"
    InsertLast   InsertPosition = "LAST"
)
```

---

## Executor Changes

### Fragment Registry

```go
type Executor struct {
    // ... existing fields
    fragments map[string]*ast.DefineFragmentStmt
}

func (e *Executor) execDefineFragment(s *ast.DefineFragmentStmt) error {
    if _, exists := e.fragments[s.Name]; exists {
        return fmt.Errorf("fragment %s already defined", s.Name)
    }
    e.fragments[s.Name] = s
    return nil
}
```

### Fragment Expansion

During page building, when encountering `use fragment`:

```go
func (pb *pageBuilder) expandFragment(name string) ([]*ast.WidgetV3, error) {
    fragment, ok := pb.executor.fragments[name]
    if !ok {
        return nil, fmt.Errorf("fragment not found: %s", name)
    }
    // return copy of widgets to avoid mutation
    return cloneWidgets(fragment.Widgets), nil
}
```

### ALTER PAGE Execution

```go
func (e *Executor) execAlterPage(s *ast.AlterPageStmt) error {
    // 1. Load existing page
    page, err := e.reader.GetPage(pageID)
    if err != nil {
        return err
    }

    // 2. build widget index by name
    widgetIndex := buildWidgetIndex(page)

    // 3. apply operations in order
    for _, op := range s.Operations {
        if err := e.applyAlterOperation(page, widgetIndex, op); err != nil {
            return err
        }
    }

    // 4. Save modified page
    return e.writer.UpdatePage(page)
}
```

---

## Example: Complete Workflow

### Workflow A: Creating New Pages with Fragments

```sql
-- Step 1: Define reusable fragments
define fragment CrudButtons as {
  footer formFooter {
    actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
    actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    actionbutton btnDelete (caption: 'Delete', action: delete, buttonstyle: danger)
  }
}
/

-- Step 2: Create page using fragments
create page CRM.Customer_Edit
(
  params: { $Customer: CRM.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvCustomer (datasource: $Customer) {
    textbox txtName (label: 'Name', attribute: Name)
    textbox txtEmail (label: 'Email', attribute: Email)
    textbox txtPhone (label: 'Phone', attribute: Phone)
    use fragment CrudButtons
  }
}
/

-- Step 3: Simple modifications with ALTER
alter page CRM.Customer_Edit {
  insert after txtEmail {
    textbox txtWebsite (label: 'Website', attribute: Website)
  }
  set caption = 'Save Customer' on btnSave
  drop widget btnDelete
}
```

### Workflow B: Describe → Edit → Replace (Modifying Existing Pages)

This is the key workflow for modifying complex existing pages:

```sql
-- Step 1: Extract the section you want to modify
describe fragment from page CRM.Customer_Edit widget dvCustomer;

-- Output:
-- {
--   DATAVIEW dvCustomer (DataSource: $Customer) {
--     TEXTBOX txtName (Label: 'Name', Attribute: Name)
--     TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
--     TEXTBOX txtWebsite (Label: 'Website', Attribute: Website)
--     TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
--     FOOTER formFooter {
--       ACTIONBUTTON btnSave (Caption: 'Save Customer', Action: SAVE_CHANGES, ButtonStyle: Primary)
--       ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
--     }
--   }
-- }

-- Step 2: Copy output, edit in your editor, then replace
alter page CRM.Customer_Edit {
  replace dvCustomer with {
    dataview dvCustomer (datasource: $Customer) {
      -- Reorganized into two columns
      layoutgrid formGrid {
        row row1 {
          column colLeft (desktopwidth: 6) {
            textbox txtName (label: 'Name', attribute: Name)
            textbox txtEmail (label: 'Email', attribute: Email)
          }
          column colRight (desktopwidth: 6) {
            textbox txtPhone (label: 'Phone', attribute: Phone)
            textbox txtWebsite (label: 'Website', attribute: Website)
          }
        }
      }
      footer formFooter {
        actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
        actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
      }
    }
  }
}
```

### Workflow C: Extract Patterns from Studio Pro Pages

```sql
-- Extract a well-designed footer from a Studio Pro page
describe fragment from page Atlas_Core.ExamplePage widget footerActions;

-- Wrap in DEFINE FRAGMENT for reuse
define fragment StandardActions as {
  -- paste extracted content here
}

-- Use in your pages
create page Module.NewPage (...) {
  dataview dv (...) {
    -- ... fields ...
    use fragment StandardActions
  }
}
```

### Workflow D: Safe Editing of Pages with Unsupported Widgets

This is the primary motivation for this proposal. A page may contain widgets that MDL cannot yet describe or round-trip (e.g., newer pluggable widgets, specialized marketplace widgets). Today, `describe page` renders these as comments and `create or replace page` silently drops them. With ALTER PAGE, we can safely modify the known parts:

```sql
-- Page contains a mix of supported and unsupported widgets.
-- DESCRIBE PAGE shows:
--   DATAVIEW dvCustomer (...) {
--     TEXTBOX txtName (Label: 'Name', Attribute: Name)
--     -- CustomWidgets$SomeMarketplaceWidget (mpWidget1)    <-- unsupported, shown as comment
--     FOOTER footer1 { ... }
--   }

-- ALTER PAGE works on the raw BSON widget tree, so unsupported widgets are preserved:
alter page Module.CustomerEdit {
  insert after txtName {
    textbox txtEmail (label: 'Email', attribute: Email)
  }
  set caption = 'Update' on btnSave
}
-- mpWidget1 is untouched — it stays in the page exactly as it was.
```

---

## Design Decisions

1. **Raw BSON widget tree for ALTER PAGE**: ALTER PAGE operations must work on the raw BSON widget tree (not parsed/reconstructed widgets). This is what makes unsupported widget preservation possible — widgets that MDL cannot parse are kept as opaque BSON documents and passed through unchanged. Only the targeted widgets are modified. This follows the same approach proven by the existing `update widgets` and `alter styling` implementations.

2. **Fragment naming conflicts**: Use prefix syntax to avoid conflicts
   - `use fragment Name as prefix_` creates prefixed widget names
   - Without prefix, error if names conflict

3. **Validation**: ALTER operations validate property assignments against widget types
   - `set caption` only valid on widgets that have Caption property
   - Prevents creating invalid models

4. **Dry run**: Not needed for ALTER PAGE (keep it simple for now)

5. **Fragment scope**: Script-scoped (transient)
   - Fragments exist only during script execution
   - Use `describe fragment from page` to extract and recreate

---

## Open Questions

1. **Undo support**: Should ALTER operations be reversible?
   - Could generate inverse operations for rollback

2. **Nested fragments**: Should fragments be allowed to USE other fragments?
   - Adds complexity but increases reusability

---

## Success Criteria

1. Pages containing unsupported widget types can be safely modified without data loss
2. A 200-line page script can be broken into 5-6 smaller fragments
3. Single property changes don't require page rewrite
4. Common patterns (CRUD buttons, form layouts) defined once, used everywhere
5. REPL users can build pages incrementally
