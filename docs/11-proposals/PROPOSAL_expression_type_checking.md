---
title: Expression Type Checking for mxcli check
status: draft
date: 2026-05-10
---

# Proposal: Expression Type Checking for mxcli check

**Status:** Draft
**Date:** 2026-05-10

## Problem Statement

`mxcli check` is currently a syntactic validator only. It catches grammar
errors but cannot catch type mismatches that produce silent failures or
CE errors in Studio Pro. Examples of bugs that pass `mxcli check` today:

```mdl
-- Silent wrong result: + is numeric add here, not string concat
declare $Label string = 'Order #' + $Order/OrderNumber;  -- OrderNumber is Integer

-- Silent empty results: enum attribute compared correctly in XPath (mxcli
-- normalises this now), but in an expression context the user writes:
if $Order/Status = 'Open' then ...   -- should be Module.OrderStatus.Open

-- Runtime CE0109: parseInteger expects a String argument
declare $N integer = parseInteger($Count);  -- $Count is Integer, not String
```

None of these are caught today. Mendix Studio Pro catches them at design time;
mxcli should too.

The goal is a **type checker** that runs as part of `mxcli check` and (for
expression-level checks) through the LSP diagnostics channel, without
requiring full project access for scope-local checks.

---

## Scope

This proposal covers **microflows and nanoflows**. Pages and security rules
involve different expression contexts and are out of scope for the initial
implementation.

Two tiers of checking:

| Tier | Requires project? | Examples |
|------|-------------------|---------|
| **Scope-local** | No | Variable type tracking, `+` overload mismatch, function argument count |
| **Catalog-backed** | Yes (`--references`) | Attribute type vs. comparison value, enum in expression vs. XPath, microflow return type |

---

## Second consumer: catalog `refs` expression edges

Type checking is not the only use of this machinery. The catalog cross-reference
graph (`CATALOG.REFS`, see [`PROPOSAL_graph_analysis.md`](PROPOSAL_graph_analysis.md))
currently captures structural edges (calls, CRUD, associations, widget datasource/
action, flow parameter/return types) but **not** the references buried *inside*
expressions and XPath constraints — an entity/association/attribute named in a
`retrieve … where [...]`, an enum compared in an `if`, a constant used in a
change-value. Those are the same things this checker must resolve to do its job:

- `AttributeAccessExpr` → catalog lookup gives the **entity + attribute** (and the
  association in a path) — i.e. the entity/attribute/association edges.
- `QualifiedNameExpr` (3-part) → `Enumeration{QN}` gives the **enum** edge.
- The scope/`populate.go` walker already tracks `$Var → entity type`, the same
  intra-flow resolution the refs builder hand-rolls today for change/delete.

So the catalog `refs` extractor should be wired as a **second consumer of
`InferType`/the resolver** — emitting a ref edge for each resolved reference —
rather than writing a parallel expression parser. This proposal is therefore a
**prerequisite for the expression-edge half of refs completeness**; building the
typesystem here unblocks both type checking *and* a materially richer model graph
(it also fills the enum/constant nodes that today have zero inbound edges). Worth
keeping the `Type`/`Checker` API usable headlessly (no LSP/lint coupling) so the
catalog builder can call it directly.

---

## Background: Mendix Type System

Mendix expressions use these types:

| Category | Types |
|----------|-------|
| Primitives | `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`, `Binary` |
| Object | `Module.EntityName` (or a generalization thereof) |
| List | `List of Module.EntityName` |
| Enumeration | `Module.EnumerationName` (the type); a specific value like `Module.OrderStatus.Open` has type `Module.OrderStatus` |
| Special | `empty` (null), `Boolean` (for `true` / `false` literals) |

### Operator Overloading

The `+` operator is overloaded in Mendix:

| Left | Right | Result | Notes |
|------|-------|--------|-------|
| `String` | `String` | `String` | Concatenation |
| `Integer` | `Integer` | `Integer` | Addition |
| `Long` | `Long` | `Long` | Addition |
| `Decimal` | `Decimal` | `Decimal` | Addition |
| `Integer` | `String` | **Error** | Must use `toString($n)` first |
| `String` | `Integer` | **Error** | Must use `toString($n)` first |
| `Decimal` | `Integer` | **Error** | Must use `toDecimal($n)` first |

All other arithmetic operators (`-`, `*`, `/`) require numeric operands.
Comparison operators (`=`, `!=`, `<`, `>`, `<=`, `>=`) require compatible
types (numeric ↔ numeric, String ↔ String, enum ↔ same enum).

### Enum Contexts

Enum values behave differently depending on context:

| Context | Correct form | Incorrect form |
|---------|-------------|----------------|
| Expression (IF, SET, DECLARE) | `Module.EnumName.Value` | `'Value'` (string) |
| XPath WHERE `[...]` | `'Value'` OR `Module.EnumName.Value` (mxcli converts) | — |
| CASE `when` branch | bare `Value` (no module prefix) | `'Value'` or qualified |

---

## Architecture

### 1. Type Representation

New package `mdl/types/typesystem` (distinct from `mdl/types` which holds
shared struct types):

```go
type Kind int
const (
    KindString Kind = iota
    KindInteger
    KindLong
    KindDecimal
    KindBoolean
    KindDateTime
    KindBinary
    KindObject      // holds QualifiedName
    KindList        // holds element QualifiedName
    KindEnumeration // holds QualifiedName of the enum
    KindEmpty
    KindUnknown     // type not yet resolved; suppress errors downstream
)

type Type struct {
    Kind          Kind
    QualifiedName string // for Object, List, Enumeration
}
```

`KindUnknown` is critical: when a variable's type cannot be resolved (e.g.,
no project available), downstream uses of that variable produce no false
positives.

### 2. Symbol Table

```go
// Scope tracks variable → Type bindings for one microflow or nanoflow.
type Scope struct {
    bindings map[string]Type  // "$VarName" → Type
    parent   *Scope           // for nested scopes (LOOP body, etc.)
}

func (s *Scope) Define(name string, t Type)
func (s *Scope) Lookup(name string) (Type, bool)
```

The scope is populated by a **first-pass walker** before type checking begins,
to handle forward references within a microflow (e.g., a variable used in a
LOOP that was declared earlier).

### 3. Type Inference Engine

```go
type Checker struct {
    scope    *Scope
    catalog  catalog.Reader  // nil if no project
    errors   []TypeError
}

func (c *Checker) InferType(expr ast.Expression) Type
func (c *Checker) CheckStatement(stmt ast.Statement)
func (c *Checker) CheckBinaryExpr(e *ast.BinaryExpr) Type
```

`InferType` walks the expression AST bottom-up:

| Expression node | Type rule |
|-----------------|-----------|
| `LiteralExpr{Kind: LiteralString}` | `String` |
| `LiteralExpr{Kind: LiteralInteger}` | `Integer` |
| `LiteralExpr{Kind: LiteralDecimal}` | `Decimal` |
| `LiteralExpr{Kind: LiteralBoolean}` | `Boolean` |
| `LiteralExpr{Kind: LiteralEmpty}` | `Empty` |
| `VariableExpr{Name: "$X"}` | look up in scope |
| `AttributeAccessExpr{Var: "$X", Path: "Attr"}` | look up entity type via catalog |
| `QualifiedNameExpr` (3-part) | `Enumeration{QN: "Module.EnumName"}` |
| `QualifiedNameExpr` (2-part) | `Unknown` (could be assoc reference) |
| `BinaryExpr{Op: "+"}` | see operator table above |
| `FunctionCallExpr{Name: "toString"}` | `String` |
| `FunctionCallExpr{Name: "parseInteger"}` | `Integer` |
| `FunctionCallExpr{Name: "length"}` | `Integer` |
| ... | built-in function return type table |

### 4. Populating the Symbol Table

The first-pass walker reads the microflow statement list and registers types:

| Statement | Binding added |
|-----------|--------------|
| `PARAMETER $Name: Type` | `$Name → resolveType(Type)` |
| `DECLARE $Name Type = expr` | `$Name → resolveType(Type)` (or `InferType(expr)` if no annotation) |
| `RETRIEVE $Name FROM Module.Entity` | `$Name → List{Module.Entity}` (or `Object{Module.Entity}` with `LIMIT 1`) |
| `CREATE OBJECT $Name FROM Module.Entity` | `$Name → Object{Module.Entity}` |
| `CALL $Name = Module.Microflow(...)` | `$Name → catalog.MicroflowReturnType(Module.Microflow)` |
| `LOOP $Item IN $List` | `$Item → element type of $List` |

### 5. Integration Points

#### mxcli check

Scope-local checks run unconditionally (no project needed).
Catalog-backed checks run when `--references` is supplied.

```
mdl/linter/rules/TC001_type_mismatch.go    -- binary operator type mismatch
mdl/linter/rules/TC002_string_concat.go    -- non-string operand in concat
mdl/linter/rules/TC003_enum_context.go     -- 'Value' string in expression context
mdl/linter/rules/TC004_function_args.go    -- wrong arg types for built-in functions
mdl/linter/rules/TC005_attribute_type.go   -- catalog-backed: attr type vs comparison
```

Each rule receives the parsed AST + a `TypeContext` (scope + optional catalog).
Rules return `[]linter.Finding` with line/column, message, and suggested fix.

#### LSP diagnostics

The type checker runs on every file save via the LSP `textDocument/diagnostic`
handler in `cmd/mxcli/lsp.go`. Scope-local checks are fast enough to run
inline; catalog-backed checks require the project to be open.

---

## Implementation Plan

### Phase 1 — Type infrastructure (no project needed)

1. `mdl/typesystem/` package: `Type`, `Kind`, built-in function return-type table
2. `mdl/typesystem/scope.go`: `Scope`, `Define`, `Lookup`
3. `mdl/typesystem/checker.go`: `Checker`, `InferType` for literals + variables + binary ops
4. `mdl/typesystem/populate.go`: first-pass walker that populates scope from DECLARE/PARAMETER/RETRIEVE/CREATE/CALL statements

**Deliverable**: `TC001` (`+` with String + non-String) and `TC004` (built-in
function argument count) working in `mxcli check` with no project.

### Phase 2 — Catalog integration

5. Extend `Checker` to accept `catalog.Reader`; implement attribute type lookup via catalog
6. Resolve microflow return types via catalog for CALL results
7. `TC002` (enum qualified name in XPath vs. expression mismatch)
8. `TC005` (attribute type vs. comparison value type in WHERE clauses)

**Deliverable**: `mxcli check --references` catches enum-string mismatches and
wrong-type WHERE predicates.

### Phase 3 — LSP wiring

9. Wire the checker into the LSP `workspace/diagnostic` push and
   `textDocument/diagnostic` pull handlers
10. Emit `DiagnosticSeverity.Warning` for type mismatches (not Error, to avoid
    blocking users on partially-typed scripts)
11. `TC003`: inline hint "use `toString($x)` before concatenating"

### Files to create / modify

| File | Change |
|------|--------|
| `mdl/typesystem/types.go` | New — `Type`, `Kind`, built-in function table |
| `mdl/typesystem/scope.go` | New — `Scope` |
| `mdl/typesystem/checker.go` | New — `Checker`, `InferType`, `CheckStatement` |
| `mdl/typesystem/populate.go` | New — first-pass scope population walker |
| `mdl/linter/rules/TC001_type_mismatch.go` | New — `+` overload mismatch |
| `mdl/linter/rules/TC002_string_concat.go` | New — non-string in concat |
| `mdl/linter/rules/TC003_enum_context.go` | New — enum context mismatch |
| `mdl/linter/rules/TC004_function_args.go` | New — built-in function arg types |
| `mdl/linter/rules/TC005_attribute_type.go` | New — catalog-backed attr type check |
| `cmd/mxcli/lsp.go` | Extend diagnostic handler to run type checker |

---

## Version Compatibility

Type checking is a static analysis pass on MDL source — it reads the AST and
optionally the catalog. It does not write BSON and has no minimum Mendix
version dependency. No version-gating required.

---

## Test Plan

### Unit tests for the type system

`mdl/typesystem/*_test.go`:
- `InferType` for each literal kind
- `InferType` for variable lookup (hit + miss)
- `CheckBinaryExpr` for each `+` combination in the operator table
- Scope nesting and shadowing

### Linter rule tests

`mdl/linter/rules/TC*_test.go` — each rule tested with:
- A snippet that triggers the finding (assert finding emitted)
- A snippet that is correct (assert no finding)

### MDL example scripts

`mdl-examples/bug-tests/type-mismatch-string-concat.mdl` — regression for the
`'Order #' + $IntegerVar` case
`mdl-examples/bug-tests/type-mismatch-enum-expression.mdl` — regression for
`$Var/Status = 'Open'` in IF context

---

## Open Questions

1. **Mendix coercion rules**: Confirmed — the runtime does **not** promote
   numeric types to `String` in any context, including `+` concatenation.
   Mixed-type `+` (e.g. `String + Integer`) is a hard error; the checker
   should flag it as `Error` severity with a `toString()` hint.

2. **`empty` compatibility**: `empty` is assignable to any nullable type in
   Mendix. The type checker should treat `empty` as compatible with everything
   to avoid false positives on `if $X = empty then` patterns.

3. **Generalization / inheritance**: If an entity `Dog` specialises `Animal`,
   is `$Dog` usable where `Animal` is expected? Mendix allows this. The checker
   needs to walk the generalization chain — requires catalog.

4. **Nanoflow restrictions**: Nanoflows disallow certain activity types (e.g.,
   database RETRIEVE). Should those restrictions live in the type checker or
   remain in the existing `validate.go`?

5. **Severity level**: Should type mismatches be `Warning` or `Error` in the
   linter output? Starting as `Warning` avoids blocking CI pipelines while the
   rules are being validated against real projects.

6. **Decimal/Integer coercion in arithmetic**: Confirmed no implicit promotion.
   Mixed `Decimal + Integer` is a type error; the user must call `toDecimal()`
   explicitly. The checker should flag this as `Error` severity.

7. **Built-in function table completeness**: The initial implementation will
   cover the ~40 documented built-in functions. User-defined Java actions and
   microflow calls are covered by catalog lookup; the table only needs to cover
   the built-ins.

---

## Relation to engalar's `exprcheck` — port, don't rebuild

The engalar fork already carries `mdl/exprcheck/`, a **working and more complete
implementation of this proposal's Phase 1–2**, architected the same way. This
section recasts the proposal accordingly: **the recommended path is to port and
adapt `exprcheck`, not build `mdl/typesystem` from scratch.**

Decisive detail this draft missed: our microflow/nanoflow expressions are stored
as **raw strings** (`Expression string` in `ast_microflow.go`) — the typed
`LiteralExpr`/`BinaryExpr`/`FunctionCallExpr` nodes in `ast_expression.go` are not
produced for them. So type-checking *requires* an expression parser, which this
draft assumed away (it planned to "walk the existing expression AST"). `exprcheck`
already includes that parser (`lexer.go`/`parser.go`/`recovery.go`).

`exprcheck` mirrors this proposal's two-tier design exactly: a
`Context{Scope, Catalog, Slots}` where `Catalog`/`Slots` are **optional** —
nil → syntax-only (our Tier 1, scope-local), non-nil → semantic (our Tier 2,
catalog-backed).

| This proposal | `exprcheck` | Status |
|---------------|-------------|--------|
| `Type`/`Kind` | `TypeKind` + `inferKind` (all 15 AST nodes) | built |
| `Scope` symbol table | `Scope` interface + `adapter_scope.go` | built |
| `InferType` engine | `inferKind` | built |
| `+` overload / arithmetic / comparison | `E004` mixed-`+` + operand checks | built |
| built-in function return table (~40) | `func_checker.go` funcTable (full DateTime/String API) | built, **more complete** |
| enum / context rules | `slot_resolver` + `slot_to_context` (`SlotResolver`) | built, systematised |
| two tiers (scope-local / catalog) | `Context{Scope, Catalog, Slots}` (nil = syntax-only) | **same design, built** |
| error codes `TC001–TC005` | `E001–E012` (hints registry + formatter) | built, **more codes** |
| *(unstated)* expression parsing | own lexer/parser/recovery | built — this draft missed the need |
| Tier-2 catalog-backed attr/microflow-return | `CatalogReader` seam; `AttributePathExpr` partly `KindUnknown` | seam done, depth partial |
| Phase 3 LSP diagnostics | not wired (validate/check path only) | to-do |

**Portability:** self-contained — `exprcheck`'s only mxcli deps are `mdl/ast` and
`mdl/linter` (both already on our branch); no dependency on engalar's other
refactors (`mprrepos`/`mxgraph`). Cherry-pickable like the modelsdk engine.

**Work remaining after a port** (this becomes the real implementation plan,
superseding "Phase 1 — Type infrastructure" above, which `exprcheck` already
delivers):

1. Provide our-catalog-backed `CatalogReader` + `SlotResolver` implementations and
   finish Tier-2 depth (attribute types, microflow return types) — the
   `AttributePathExpr → KindUnknown` gap.
2. Wire the `exprcheck` adapters into **our** `mxcli check` / `validate` path.
3. **Phase 3 LSP** — still to-do; the `Context`-based design makes it
   straightforward (run with `Scope` only for the inline, project-less path).
4. Decide the cosmetics: keep `exprcheck`'s `E0xx` codes (faithful port) vs remap
   to this proposal's `TC0xx`; keep the `mdl/exprcheck` package name vs the
   proposed `mdl/typesystem`.

**Process note:** two proposals in a row (this one and the graph-analysis
proposal) turned out to be substantially pre-built on the engalar fork — **check
the fork before greenfielding** future proposals.
