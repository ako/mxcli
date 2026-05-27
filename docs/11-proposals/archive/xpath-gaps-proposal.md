---
title: XPath Support Gap Analysis & Implementation Proposal
status: done
date: 2026-03-19
---

# XPath Support Gap Analysis & Implementation Proposal

**Date:** 2026-03-19
**Branch:** xpath
**Status:** All phases implemented

## 1. Background

Mendix uses XPath extensively for data retrieval constraints in microflows, page data sources, and entity access rules. Our MDL grammar and executor currently handle basic XPath expressions but lack support for several patterns that appear frequently in real-world Mendix projects.

This analysis is based on scanning **448 unique XPath expressions** across **823 occurrences** in three production-grade Mendix projects:
- EnquiriesManagement
- Evora-FactoryManagement
- LatoProductInventory

## 2. Current Implementation

### What works today

| Feature | Grammar | Visitor | Executor | Round-trip |
|---------|---------|---------|----------|------------|
| Simple attribute comparison `[attr = 'value']` | Yes | Yes | Yes | Yes |
| Variable reference `[attr = $var]` | Yes | Yes | Yes | Yes |
| Variable path `$Var/attr` | Yes | Yes | Yes | Yes |
| Boolean operators `and`, `or` | Yes | Yes | Yes | Yes |
| Comparison operators `=`, `!=`, `<`, `>`, `<=`, `>=` | Yes | Yes | Yes | Yes |
| Parenthesized grouping `(expr)` | Yes | Yes | Yes | Yes |
| Token quoting `'[%CurrentDateTime%]'` | Yes | Yes | Yes | Yes |
| Multiple chained predicates `[a][b]` | Yes | Yes | Yes | Yes |
| `empty` literal | Yes | Yes | Yes | Yes |
| Function calls (generic) | Yes | Yes | Partial | Partial |
| GRANT ... WHERE xpath | Yes | Yes | Yes | Yes |
| RETRIEVE ... WHERE xpath | Yes | Yes | Yes | Yes |
| Page DATABASE source WHERE | Yes | Yes | Yes | Yes |

### XPath contexts in BSON

| Context | Read | Write |
|---------|------|-------|
| `microflows$DatabaseRetrieveSource/XpathConstraint` | Yes | Yes |
| `DomainModels$AccessRule/XPathConstraint` | Yes | Yes |
| `Forms$ListViewXPathSource/XPathConstraint` | Partial | Partial |
| `CustomWidgets$CustomWidgetXPathSource/XPathConstraint` | No | No |
| `Forms$SelectorXPathSource/XPathConstraint` | No | No |

## 3. Gap Analysis

### Gap 1: Association Path Without Variable (bare paths)

**Frequency:** 190 occurrences (42%)

In XPath constraints, association paths often appear without a `$variable` prefix:

```xpath
[Module.Association/Module.Entity/attribute = $value]
[Module.Association = $object]
[not(Module.Association/Module.Entity)]
```

**Current behavior:** The grammar parses `Module.Association/Module.Entity` as a division operation between two qualified names, since `/` is also the division operator. The `tryBuildAttributePath()` visitor only recognizes paths starting with `$Variable`.

**Impact:** High - this is the most common gap.

**Fix:**
1. Add a new AST node `XPathPathExpr` representing a bare association path (no `$` prefix).
2. In `tryBuildAttributePath()`, also recognize `QualifiedNameExpr / IdentifierExpr` and `QualifiedNameExpr / QualifiedNameExpr` as path expressions.
3. Update `xpathExprToString()` and `expressionToXPath()` to serialize `XPathPathExpr`.

### Gap 2: Nested Predicates Within Paths

**Frequency:** 32 occurrences (7%)

XPath supports predicates inside path steps to filter intermediate associations:

```xpath
[Module.Assoc/Module.Entity[State = 'InProgress']/SubAssoc = '[%CurrentUser%]']
[System.Workflow[State != 'Incompatible' and State != 'Failed']]
```

**Current behavior:** The grammar's `xpathConstraint` rule only handles a single `[expression]` and does not allow brackets within a path step. Nested predicates fail to parse.

**Impact:** Medium - used in complex workflows and security rules.

**Fix:**
1. Introduce an `xpathExpression` grammar rule tree separate from the general-purpose `expression` tree. XPath has different syntax from Mendix expressions (bare identifiers, `/` is always path traversal, nested `[]` predicates).
2. The `xpathExpression` rule should support:
   - `pathStep ( '[' xpathExpression ']' )? ( '/' pathStep )*`
   - Where `pathStep` is an identifier or qualified name
3. Keep the existing `expression` rules untouched (they serve Mendix expressions in microflows, OQL, etc.).

### Gap 3: `id` Pseudo-Attribute

**Frequency:** 37 occurrences (8%)

Mendix XPath uses `id` as a special pseudo-attribute to compare entity identity:

```xpath
[id = $currentUser]
[id != $existingObject]
[id = '[%CurrentUser%]']
```

**Current behavior:** `id` parses as an `IdentifierExpr` which works for serialization, but the executor/validator has no awareness that `id` is a special Mendix XPath keyword. No validation or special handling.

**Impact:** Low for parsing (it works), Medium for validation/catalog.

**Fix:**
1. No grammar change needed — `id` already parses as an identifier.
2. Add `id` to the catalog/validator as a recognized XPath pseudo-attribute.
3. Document that `id` refers to the Mendix object GUID.

### Gap 4: `not()` Function vs `not` Keyword

**Frequency:** 30 occurrences (6%)

XPath uses `not()` as a function, often for existence checks:

```xpath
[not(Module.Assoc/Module.Entity)]
[not(IsDraft)]
[not(contains(Name, 'demo'))]
```

**Current behavior:** The grammar has `not` as a prefix operator in `notExpression` and `not` is not in the `functionName` rule. However, since `HYPHENATED_ID` doesn't match `not` and `not` is a keyword, `not(...)` would be parsed as `not ( expression )` — a prefix-NOT with parenthesized expression, which actually works correctly for boolean expressions.

For existence checks like `not(Module.Assoc/Module.Entity)`, parsing fails because `Module.Assoc/Module.Entity` is not recognized as a boolean expression in the general expression grammar — it's a path, not a comparison.

**Impact:** Medium - existence checks are common in security rules.

**Fix:**
1. Add `not` to the `functionName` rule so `not(...)` can also parse as a function call.
2. In the XPath-specific context, treat a bare path as a boolean (existence check).
3. This overlaps with Gap 2 — a dedicated XPath expression grammar would handle this naturally.

### Gap 5: `true()` and `false()` Functions

**Frequency:** 14 occurrences (2%)

Mendix XPath supports both:
- `true` / `false` as bare boolean literals: `[Active = true]`
- `true()` / `false()` as XPath functions: `[Active = true()]`

**Current behavior:** `true` and `false` are in the `functionName` rule, so `true()` and `false()` parse as function calls. The literal forms `true`/`false` parse as `booleanLiteral`. Both work.

**Impact:** None — already works.

### Gap 6: `reversed()` Path Modifier

**Frequency:** 3 occurrences (<1%)

Used to traverse associations in reverse direction:

```xpath
[System.grantableRoles[reversed()]/System.UserRole/System.UserRoles = '[%CurrentUser%]']
[MxModelReflection.MxObjectType_SubClassOf_MxObjectType[reversed()] = $MxObjectType]
```

**Current behavior:** `[reversed()]` within a path step is not supported. The parser would fail on the nested `[]` with a function call inside.

**Impact:** Low frequency, but critical when needed.

**Fix:**
1. In the XPath path grammar (see Gap 2), allow `[reversed()]` as a special path modifier.
2. Add `reversed` to the function name list or handle as a keyword.
3. Serialize as `[reversed()]` in the output.

### Gap 7: `System.owner` Special Association

**Frequency:** 11 occurrences (2%)

```xpath
[System.owner = '[%CurrentUser%]']
```

**Current behavior:** Parses correctly as a qualified name comparison. No special handling needed.

**Impact:** None — already works.

### Gap 8: Date/Time Arithmetic in Tokens

**Frequency:** 2 occurrences (<1%)

```xpath
[TIMESTAMP > ('[%CurrentDateTime%] - 3 * [%DayLength%]')]
```

**Current behavior:** The string `'[%CurrentDateTime%] - 3 * [%DayLength%]'` is a string literal in XPath, so it parses fine. Mendix runtime evaluates it.

**Impact:** None — already works (it's just a string literal).

### Gap 9: `contains()` and `starts-with()` Functions

**Frequency:** 20 occurrences (4%)

```xpath
[contains(Name, $SearchStr)]
[starts-with(Assoc/Account/FullName, $search)]
```

**Current behavior:**
- `contains()` is in the `functionName` rule — works.
- `starts-with()` uses `HYPHENATED_ID` — works.
- However, the first argument is often a bare attribute name or path (Gap 1 applies).

**Impact:** Low — the function call itself parses; the path argument is the real issue (Gap 1).

### Gap 10: CustomWidget and ListView XPath Sources

**Frequency:** 57 occurrences (12%)

Custom widgets (DataGrid2, Gallery, etc.) and ListView use XPath constraints via `CustomWidgetXPathSource` and `ListViewXPathSource` BSON types.

**Current behavior:**
- `ListViewXPathSource` is partially supported in page parsing.
- `CustomWidgetXPathSource` is not handled — the XPath is embedded in pluggable widget property objects.

**Impact:** Medium — affects modern widget support.

**Fix:**
1. Add `CustomWidgetXPathSource` parsing in `parser_page.go`.
2. Add `ListViewXPathSource` writing in `writer_widgets.go`.
3. For pluggable widgets, the XPath is inside the widget's property data — may need template-level support.

## 4. Prioritized Implementation Plan

### Phase 1: Core XPath Path Expressions (High Priority)

**Goal:** Support the full XPath path syntax that covers 80%+ of real-world expressions.

#### 1a. Bare Association Paths (Gap 1)

Add `XPathPathExpr` AST node and extend path detection in the visitor:

```go
// XPathPathExpr represents a bare xpath path: Module.Assoc/Module.Entity/attr
type XPathPathExpr struct {
    Steps []XPathStep
}

type XPathStep struct {
    Name      string       // Qualified name or identifier
    Predicate expression   // Optional nested predicate [expr]
}
```

**Files to modify:**
- `mdl/ast/ast_expression.go` — add `XPathPathExpr`, `XPathStep`
- `mdl/visitor/visitor_microflow_expression.go` — extend `tryBuildAttributePath()` to recognize `QualifiedNameExpr / ...` patterns
- `mdl/visitor/visitor_page_v3.go` — update `xpathExprToString()`
- `mdl/executor/cmd_microflows_helpers.go` — update `expressionToXPath()`

**Estimated scope:** ~100 lines changed

#### 1b. Nested Predicates in Paths (Gap 2)

This requires a grammar-level change. Two options:

**Option A: Dedicated XPath grammar rules** (recommended)

Add XPath-specific expression rules that are only used within `xpathConstraint`:

```antlr
xpathConstraint
    : LBRACKET xpathOrExpr RBRACKET
    ;

xpathOrExpr
    : xpathAndExpr (or xpathAndExpr)*
    ;

xpathAndExpr
    : xpathNotExpr (and xpathNotExpr)*
    ;

xpathNotExpr
    : not? xpathComparison
    ;

xpathComparison
    : xpathPath (comparisonOperator xpathValue)?
    | xpathFunctionCall
    | LPAREN xpathOrExpr RPAREN
    ;

xpathPath
    : xpathStep (SLASH xpathStep)*
    ;

xpathStep
    : (qualifiedName | IDENTIFIER | VARIABLE)
      (LBRACKET xpathOrExpr RBRACKET)?    // Nested predicate
    ;

xpathValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | VARIABLE (SLASH xpathStep)*
    | MENDIX_TOKEN
    | empty
    | xpathFunctionCall
    ;

xpathFunctionCall
    : (IDENTIFIER | HYPHENATED_ID | not | true | false | contains)
      LPAREN xpathArgList? RPAREN
    ;

xpathArgList
    : xpathOrExpr (COMMA xpathOrExpr)*
    ;
```

**Pros:** Clean separation from Mendix expression grammar. No ambiguity between `/` (path) and `/` (division). Handles nested predicates naturally.

**Cons:** Duplicates some expression rules. Needs a new visitor tree.

**Option B: Reuse expression grammar with context flag**

Mark the expression parser as "in XPath mode" so `/` is always path traversal and `[...]` inside paths becomes a predicate.

**Pros:** Less grammar duplication.
**Cons:** Complex, error-prone, hard to maintain.

**Recommendation:** Option A. XPath is a fundamentally different language from Mendix expressions. A dedicated sub-grammar of ~40 lines is cleaner than hacking the shared expression grammar.

**Files to modify:**
- `mdl/grammar/MDLParser.g4` — add xpath expression rules
- `mdl/grammar/MDLLexer.g4` — no changes needed
- `mdl/visitor/visitor_xpath.go` — new file, builds XPath AST from xpath grammar rules
- `mdl/ast/ast_expression.go` — add `XPathPathExpr`, `XPathStep`
- Regenerate parser: `make grammar`

**Estimated scope:** ~200 lines new, ~50 lines modified

#### 1c. `reversed()` Modifier (Gap 6)

Falls out naturally from Phase 1b — `reversed()` is just a function call inside a nested predicate: `[reversed()]`.

No additional work needed if Phase 1b is implemented.

### Phase 2: Existence Checks and `not()` (Medium Priority)

**Goal:** Support `not(path)` and bare path existence checks.

In XPath, a bare path like `Module.Assoc/Module.Entity` is truthy if the association is populated. This is used in:
- `[not(Module.Assoc/Module.Entity)]` — entity has no associated object
- `[Module.Assoc/Module.Entity]` — entity has an associated object

With the Phase 1 xpath grammar, these work naturally:
- `xpathComparison` allows `xpathPath` without a comparison operator (existence check)
- `xpathNotExpr` handles `not? xpathComparison`
- `not()` as a function call in `xpathFunctionCall`

**Files to modify:**
- Covered by Phase 1b grammar changes
- `mdl/visitor/visitor_xpath.go` — handle existence check (path without comparison)

**Estimated scope:** ~30 lines

### Phase 3: BSON Source Types (Medium Priority)

**Goal:** Read/write XPath for all BSON source types.

#### 3a. `CustomWidgetXPathSource`

Pluggable widgets store XPath in their property data under the `$type: CustomWidgets$CustomWidgetXPathSource` BSON type. This needs:

1. Recognition in the BSON parser
2. Extraction of `XPathConstraint` field
3. Round-trip serialization

#### 3b. `ListViewXPathSource` and `SelectorXPathSource`

Similar to above — add full read/write support.

**Files to modify:**
- `sdk/mpr/parser_page.go` — add source type parsing
- `sdk/mpr/writer_widgets.go` — add source type serialization
- `sdk/pages/pages_datasources.go` — add source types if not present

**Estimated scope:** ~150 lines

### Phase 4: Catalog & Validation (Lower Priority)

**Goal:** Populate the `xpath_expressions` catalog table and validate XPath references.

The catalog table schema already exists (`mdl/catalog/tables.go`). Implementation needs:

1. XPath expression extraction during catalog refresh
2. Entity/association reference resolution
3. Storage in the `xpath_expressions` table
4. Cross-reference queries (SHOW CALLERS/REFERENCES) for XPath

**Files to modify:**
- `mdl/catalog/builder.go` — extract XPath during catalog build
- `mdl/catalog/queries.go` — query xpath_expressions table

**Estimated scope:** ~200 lines

## 5. Feature Coverage Matrix

After implementation, coverage of the 448 real-world XPath patterns:

| Feature | Occurrences | Phase | Current | After |
|---------|-------------|-------|---------|-------|
| Variable reference `$var` | 301 (67%) | - | Yes | Yes |
| Association paths (bare) | 190 (42%) | 1a | No | Yes |
| String comparison | 145 (32%) | - | Yes | Yes |
| Chained predicates `[][]]` | 132 (29%) | - | Yes | Yes |
| `and` / `or` | 153 (33%) | - | Yes | Yes |
| `!=` | 69 (15%) | - | Yes | Yes |
| `empty` comparison | 61 (13%) | - | Yes | Yes |
| Multi-hop paths | 51 (11%) | 1a | No | Yes |
| Parenthesized grouping | 43 (9%) | - | Yes | Yes |
| `[%CurrentUser%]` token | 42 (9%) | - | Yes | Yes |
| `id` pseudo-attribute | 37 (8%) | - | Yes | Yes |
| `<=`, `>=`, `<`, `>` | 126 (28%) | - | Yes | Yes |
| Nested predicates | 32 (7%) | 1b | No | Yes |
| `not()` / existence | 30 (6%) | 2 | Partial | Yes |
| `contains()` | 14 (3%) | 1a* | Partial | Yes |
| `true()` / `false()` | 14 (2%) | - | Yes | Yes |
| `System.owner` | 11 (2%) | - | Yes | Yes |
| `[%CurrentDateTime%]` | 7 (1%) | - | Yes | Yes |
| `starts-with()` | 6 (1%) | 1a* | Partial | Yes |
| `reversed()` | 3 (<1%) | 1b | No | Yes |
| Date arithmetic in tokens | 2 (<1%) | - | Yes | Yes |

*Functions work but their path arguments need Gap 1 fix.

**Estimated total coverage after Phase 2: ~98%** (up from ~55% today)

## 6. Testing Strategy

1. **Unit tests:** Parse and round-trip each XPath pattern category
2. **Integration tests:** Extract XPath from the 3 test projects, parse through MDL, write back, verify `mx check` passes
3. **Regression tests:** Existing security/microflow round-trip tests must continue to pass

## 7. Risks

| Risk | Mitigation |
|------|------------|
| Dedicated XPath grammar conflicts with existing expression rules | XPath rules are only referenced from `xpathConstraint`, completely isolated |
| `/` ambiguity between path and division in shared contexts | Only use xpath grammar in xpath contexts; keep existing expression grammar for Mendix expressions |
| Nested predicates with complex expressions | Limit nesting depth in tests; Mendix projects rarely go beyond 2 levels |
| ANTLR4 left-recursion issues | XPath grammar uses iterative rules (`xpathStep (SLASH xpathStep)*`) not recursive |
