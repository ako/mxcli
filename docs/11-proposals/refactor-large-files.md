---
title: Large Source Files
status: proposed
---

# Refactoring Proposal: Large Source Files

## Overview

This proposal addresses the refactoring of 6 large non-generated source files to improve maintainability and extensibility, especially for adding new widget types.

## Files to Refactor

| File | Lines | Priority |
|------|-------|----------|
| `mdl/visitor/visitor.go` | 3,471 | High |
| `sdk/mpr/writer.go` | 3,000 | High |
| `mdl/executor/cmd_pages.go` | 2,339 | High |
| `sdk/mpr/parser.go` | 2,013 | Medium |
| `sdk/pages/pages.go` | 1,217 | Medium |
| `mdl/ast/ast.go` | 1,154 | Medium |

Note: Generated files (mdl_parser.go, types.go, enums.go) cannot be refactored.

---

## 1. sdk/mpr/writer.go (3,000 lines)

**Current state:** Single file with CRUD operations, unit management, and serialization for all document types.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `writer.go` | Core struct, lifecycle, transaction support | ~150 |
| `writer_units.go` | Low-level unit insert/update/delete | ~200 |
| `writer_modules.go` | Module CRUD | ~150 |
| `writer_domainmodel.go` | Entity, attribute, association CRUD + serialization | ~500 |
| `writer_microflow.go` | Microflow/nanoflow CRUD + serialization | ~700 |
| `writer_pages.go` | Page, layout, snippet CRUD + serialization | ~350 |
| `writer_widgets.go` | Widget serialization dispatcher + all widget serializers | ~850 |
| `writer_enumeration.go` | Enumeration, constant CRUD + serialization | ~200 |

### Extensibility Benefit

New widgets only require adding a case to `writer_widgets.go`. Consider future registry pattern:

```go
// Future enhancement: widget serializer registry
var widgetSerializers = map[string]func(pages.Widget) bson.D{
    "Forms$container":    serializeContainer,
    "Forms$layoutgrid":   serializeLayoutGrid,
    "Forms$actionbutton": serializeActionButton,
    // New widgets registered here
}
```

---

## 2. mdl/visitor/visitor.go (3,471 lines)

**Current state:** Single file handling all MDL statement types with 40+ widget builders.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `visitor.go` | Core struct, interfaces | ~100 |
| `visitor_connection.go` | CONNECT, DISCONNECT | ~50 |
| `visitor_module.go` | CREATE MODULE | ~50 |
| `visitor_enumeration.go` | Enumeration statements | ~100 |
| `visitor_entity.go` | Entity + view entity statements | ~300 |
| `visitor_association.go` | Association statements | ~100 |
| `visitor_query.go` | SHOW, DESCRIBE statements | ~200 |
| `visitor_microflow.go` | CREATE MICROFLOW + statement builders | ~1,200 |
| `visitor_page.go` | CREATE PAGE/SNIPPET + widget builders | ~1,200 |
| `visitor_expression.go` | Expression parsing | ~400 |
| `visitor_helpers.go` | Utility functions | ~200 |

### Further Split for visitor_page.go (optional)

For maximum extensibility, page widgets can be further split:

| File | Widget Categories |
|------|-------------------|
| `visitor_page_widgets_container.go` | LayoutGrid, Container, GroupBox, TabContainer |
| `visitor_page_widgets_input.go` | TextBox, DatePicker, DropDown, CheckBox |
| `visitor_page_widgets_display.go` | DynamicText, StaticText, Label, Image |
| `visitor_page_widgets_data.go` | DataView, DataGrid, ListView, Gallery |
| `visitor_page_widgets_action.go` | ActionButton, LinkButton, DropDownButton |

---

## 3. mdl/executor/cmd_pages.go (2,339 lines)

**Current state:** All page-related commands in single file.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `cmd_pages.go` | Exports, shared helpers | ~100 |
| `cmd_pages_show.go` | SHOW PAGES command | ~200 |
| `cmd_pages_create.go` | CREATE PAGE, CREATE SNIPPET | ~400 |
| `cmd_pages_drop.go` | DROP PAGE, DROP SNIPPET | ~100 |
| `cmd_pages_describe.go` | DESCRIBE PAGE with widget tree | ~850 |
| `cmd_pages_widgets.go` | Widget extraction and MDL output | ~700 |

---

## 4. sdk/mpr/parser.go (2,013 lines)

**Current state:** Well-organized by document type but large.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `parser.go` | Core helpers, type extraction | ~200 |
| `parser_entity.go` | Entity, attribute parsing | ~300 |
| `parser_association.go` | Association parsing | ~150 |
| `parser_microflow.go` | Microflow objects, actions | ~800 |
| `parser_page.go` | Page parameters, widget properties | ~250 |
| `parser_text.go` | Text, enumeration, coordinates | ~200 |

---

## 5. sdk/pages/pages.go (1,217 lines)

**Current state:** All widget types in single file, good interface design.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `pages.go` | Page, Layout, Snippet, Template types | ~200 |
| `pages_parameters.go` | PageParameter, SnippetParameter, LayoutCall | ~100 |
| `pages_widgets.go` | Widget interface, BaseWidget | ~50 |
| `pages_widgets_container.go` | LayoutGrid, Container, GroupBox, TabContainer | ~200 |
| `pages_widgets_data.go` | DataView, DataGrid, ListView, TemplateGrid | ~200 |
| `pages_datasources.go` | DataSource interface + implementations | ~100 |
| `pages_widgets_input.go` | TextBox, DatePicker, DropDown, etc. | ~250 |
| `pages_widgets_action.go` | ActionButton, ClientAction types | ~300 |
| `pages_widgets_display.go` | Text, DynamicText, Label, Image | ~150 |
| `pages_widgets_advanced.go` | CustomWidget, Gallery, SnippetCall | ~200 |

---

## 6. mdl/ast/ast.go (1,154 lines)

**Current state:** All AST types in single file.

### Proposed Split

| New File | Contents | Lines |
|----------|----------|-------|
| `ast.go` | Core interfaces, Position, QualifiedName, DataType | ~150 |
| `ast_domainmodel.go` | Entity, attribute, association statements | ~250 |
| `ast_enumeration.go` | Enumeration statements | ~80 |
| `ast_microflow.go` | Microflow statements + expressions | ~400 |
| `ast_page.go` | Page, snippet statements | ~100 |
| `ast_page_widgets.go` | Widget interface + 40+ widget types | ~350 |
| `ast_commands.go` | Connection, query, session statements | ~150 |

---

## Implementation Order

### Phase 1: Core SDK Files (enables easier widget additions)
1. `sdk/mpr/writer.go` → Split serialization by type
2. `sdk/pages/pages.go` → Split widget types by category

### Phase 2: Executor Files
3. `mdl/executor/cmd_pages.go` → Split by command type

### Phase 3: Visitor and AST
4. `mdl/visitor/visitor.go` → Split by statement type
5. `mdl/ast/ast.go` → Split by domain

### Phase 4: Parser (lowest priority)
6. `sdk/mpr/parser.go` → Split by document type

---

## Extensibility Pattern for Widgets

After refactoring, adding a new widget type requires changes in these files:

| Layer | File | Change |
|-------|------|--------|
| Types | `sdk/pages/pages_widgets_*.go` | Add struct definition |
| AST | `mdl/ast/ast_page_widgets.go` | Add AST node |
| Parser | `sdk/mpr/parser_page.go` | Add parse function |
| Writer | `sdk/mpr/writer_widgets.go` | Add serialize function |
| Visitor | `mdl/visitor/visitor_page.go` | Add builder function |
| Executor | `mdl/executor/cmd_pages_widgets.go` | Add describe/output |

Each file is focused and ~200-400 lines, making changes localized and reviewable.

---

## Verification

After each split:
1. `go build ./...` must pass
2. `go test ./...` must pass
3. No import cycles introduced
4. Existing functionality unchanged

### Test Framework

A comprehensive test framework exists in `mdl/executor/roundtrip_test.go`:

```bash
# run semantic roundtrip tests (fast, ~5s)
go test -v ./mdl/executor/... -run "Roundtrip" -timeout 60s

# run mx check integration tests (slower, ~25s)
go test -v ./mdl/executor/... -run "MxCheck" -timeout 120s

# run all tests
go test -v ./mdl/executor/... -run "Roundtrip|MxCheck" -timeout 120s
```

**Test Types:**

| Test | What it validates |
|------|-------------------|
| `TestRoundtripEntity_*` | Entity creation with attributes, constraints, indexes |
| `TestRoundtripEnumeration` | Enumeration values and captions |
| `TestRoundtripPage_*` | Page creation with widgets (compact and verbose syntax) |
| `TestMxCheck_*` | Studio Pro validation via `mx check` command |

**Key Features:**
- Auto-cleanup of test artifacts
- Semantic comparison (property presence, not string equality)
- MxCheck tests distinguish new errors from pre-existing issues
- Benchmark test for performance regression detection

### Pre-Refactoring Checklist

Before starting any refactoring:

1. Run full test suite to establish baseline
2. Note any pre-existing failures
3. Create a checkpoint commit

After each refactoring step:

1. Run `go build ./...`
2. Run `go test ./mdl/executor/... -run "Roundtrip|MxCheck"`
3. Verify no new failures introduced
4. Commit the split
