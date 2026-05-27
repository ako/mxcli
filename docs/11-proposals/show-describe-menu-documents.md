---
title: SHOW/DESCRIBE Menu Documents
status: proposed
---

# Proposal: SHOW/DESCRIBE Menu Documents

## Overview

**Document type:** `Menus$MenuDocument`
**Prevalence:** 6 across test projects (2 Enquiries, 2 Evora, 2 Lato)
**Priority:** Low — small count, related to navigation; complements existing Navigation support

Menu Documents define standalone menu structures that can be assigned to navigation profiles. Each menu contains a tree of menu items with captions, icons, and actions (page links, microflow calls, or external URLs).

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | No | — |
| **Parser** | No | `parseNavMenuItem()` exists for navigation but not for standalone menus |
| **Reader** | No | — |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 3259 |

## BSON Structure (from test projects)

```
Menus$MenuDocument:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  ItemCollection: Menus$MenuItemCollection
    Items: []*Menus$MenuItem
      - caption: Texts$text (with translations)
      - AlternativeText: Texts$text
      - icon: Forms$IconCollectionIcon | Forms$GlyphIcon | null
      - action: Forms$* (polymorphic client action)
        - Forms$PageClientAction: PageSettings.Page (qualified name)
        - Forms$MicroflowClientAction: MicroflowSettings.Microflow (qualified name)
        - Forms$OpenLinkClientAction: Address (url)
        - Forms$NoAction
      - Items: []*MenuItem (recursive for sub-menus)
```

## Proposed MDL Syntax

### SHOW MENUS

```
show MENUS [in module]
```

| Qualified Name | Module | Name | Items | Depth |
|----------------|--------|------|-------|-------|

Where "Items" is the total count and "Depth" is the maximum nesting level.

### DESCRIBE MENU

```
describe menu Module.Name
```

Output format:

```
menu MyModule.MainMenu
{
  'Dashboard' -> page MyModule.Dashboard_Overview;
  'Customers' -> page MyModule.Customer_Overview
  {
    'New Customer' -> page MyModule.Customer_NewEdit;
    'Import' -> microflow MyModule.Customer_Import;
  };
  'Reports'
  {
    'Monthly Report' -> microflow MyModule.GenerateMonthlyReport;
    'Export Data' -> microflow MyModule.ExportAllData;
  };
  'Settings' -> page Administration.Account_Overview;
};
/
```

## Implementation Steps

### 1. Add Model Type (model/types.go)

```go
type MenuDocument struct {
    ContainerID model.ID
    Name        string
    documentation string
    Excluded    bool
    ExportLevel string
    Items       []*MenuItem
}

type MenuItem struct {
    caption    string
    ActionType string // "page", "microflow", "link", "none"
    ActionTarget string // qualified name or url
    Items      []*MenuItem // sub-items (recursive)
}
```

### 2. Add Parser (sdk/mpr/parser_misc.go or new file)

Parse `Menus$MenuDocument` BSON. Extract caption text (first translation or default language). Parse `action` polymorphic type to determine action type and target. Recursively parse sub-items.

Can potentially share code with existing navigation menu parsing (`parseNavMenuItem`).

### 3. Add Reader

```go
func (r *Reader) ListMenuDocuments() ([]*model.MenuDocument, error)
```

### 4. Add AST, Grammar, Visitor, Executor

Grammar tokens: `menu`, `MENUS`.

### 5. Add Autocomplete

```go
func (e *Executor) GetMenuNames(moduleFilter string) []string
```

## Complexity

**Medium** — recursive menu item tree with polymorphic actions and multi-language captions.

## Testing

- Verify against all 3 test projects
