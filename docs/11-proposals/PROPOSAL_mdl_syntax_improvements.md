---
title: MDL Syntax and Grammar Improvements
status: draft
date: 2026-01-20
---

# Proposal: MDL Syntax and Grammar Improvements

**Date:** 2026-01-20
**Author:** GitHub Copilot

## 1. Introduction

This document proposes a set of improvements to the Mendix Definition Language (MDL) syntax and grammar. The goal is to enhance the language's **readability**, **consistency**, and **token efficiency**, making it more intuitive for citizen developers and more effective for processing by LLMs.

The analysis is based on the existing capabilities demonstrated in `mdl-examples/doctype-tests/02-microflow-examples.mdl`.

## 2. Key Areas for Improvement

### 2.1. Variable Declaration and Assignment

**Current Syntax:**
The language uses three different ways to handle variable assignments:
- `declare $VarName type = initial_value;` (Declaration)
- `set $VarName = new_value;` (Re-assignment)
- `$VarName = create ...;` or `$VarName = call ...;` (Assignment from expression)

**Problem:**
This inconsistency increases the learning curve and adds unnecessary verbosity (e.g., the `set` keyword is redundant).

**Proposal: Introduce Go-style Assignment Operators**
Adopt a more concise and consistent approach:
- **`:=` (Declare and Assign):** Use for first-time declaration and initialization within a scope.
- **`=` (Assign):** Use for re-assigning a new value to an already declared variable.

**Example:**
```mdl
// before
declare $Counter integer = 0;
set $Counter = $Counter + 1;
$NewProduct = create MfTest.Product(...);

// after
$Counter := 0;
$Counter = $Counter + 1;
$NewProduct := create MfTest.Product(...);
```
This change would make the `declare` and `set` keywords obsolete, significantly cleaning up the syntax.

### 2.2. Control Flow Block Syntax

**Current Syntax:**
MDL uses `BEGIN...END` and `THEN...END if` to delineate code blocks.
- `if $condition then ... end if;`
- `loop $item in $list begin ... end loop;`

**Problem:**
This syntax, while explicit, is verbose and less common in modern programming languages. It consumes more tokens and can be harder to read for developers accustomed to C-style syntax.

**Proposal: Adopt C-style Brace Syntax `{}`**
Use curly braces to define blocks for all control flow statements.

**Example:**
```mdl
// before
if $Product/IsActive then
  set $ActiveCount = $ActiveCount + 1;
  log info node 'Test' 'Found active product';
end if;

// after
if ($Product/IsActive) {
  $ActiveCount = $ActiveCount + 1;
  log info node 'Test' 'Found active product';
}
```

### 2.3. Fluent APIs for List and Aggregate Operations

**Current Syntax:**
List operations are function-based, and aggregate functions have a unique syntax.
- `$ActiveProducts = filter($ProductList, $IteratorProduct/IsActive = true);`
- `$AveragePrice = average($ProductList.Price);`

**Problem:**
Chaining multiple operations is clumsy and hard to read. The syntax for aggregation (`$List.Attribute`) is inconsistent with other function calls.

**Proposal: Introduce Fluent (Pipelined) Syntax**
Allow for method-chaining on list variables. This is a highly readable and expressive pattern.

**Example:**
```mdl
// before
$ActiveProducts = filter($ProductList, $IteratorProduct/IsActive = true);
$SortedProducts = sort($ActiveProducts, Price desc);
$AverageActivePrice = average($SortedProducts.Price);

// after
$AverageActivePrice := $ProductList
  .filter($p -> $p/IsActive)
  .sort(Price desc)
  .average($p -> $p/Price);
```
This syntax is more intuitive, token-efficient, and powerful for complex data manipulation. It uses lambda-style expressions (`$p -> ...`) for clarity.

### 2.4. `change` Statement Readability

**Current Syntax:**
The `change` statement modifies multiple attributes in a flat list.
`change $Product (Name = $NewName, ModifiedDate = [%CurrentDateTime%]);`

**Problem:**
For objects with many attributes, this can become a long, hard-to-read line.

**Proposal: Introduce `with` block for `change`**
Allow a block syntax for grouping attribute changes, improving readability.

**Example:**
```mdl
// before
change $Product (DailyAverage = $DailyAverage, LastCalculated = [%CurrentDateTime%]);

// after
change $Product with {
  DailyAverage = $DailyAverage,
  LastCalculated = [%CurrentDateTime%]
};
```

### 2.5. Unify Function and Action Call Syntax

**Current Syntax:**
- `call microflow MfTest.M001_HelloWorld()`
- `call java action CustomActivities.ExecuteOQLStatement(...)`
- `count($ProductList)`

**Problem:**
The `call` keyword is verbose and inconsistent with built-in function calls like `count`.

**Proposal: Standardize All Calls**
Remove the `call` keyword and treat microflows and Java actions as regular callable functions. The system can distinguish them by their path.

**Example:**
```mdl
// before
$Result = call microflow MfTest.M003_StringOperations(FirstName = 'Hello', LastName = 'World!');
$OqlResult = call java action CustomActivities.ExecuteOQLStatement(...);

// after
$Result := MfTest.M003_StringOperations(FirstName: 'Hello', LastName: 'World!');
$OqlResult := CustomActivities.ExecuteOQLStatement(...);
```
Using named parameters with colons (`:`) could further improve clarity, distinguishing them from positional parameters.

## 3. Summary of Benefits

- **Improved Readability:** The proposed syntax is closer to modern programming languages, making it more familiar to a wider range of developers.
- **Increased Consistency:** Rules for variable assignment and function calls are unified, reducing cognitive load.
- **Enhanced Token Efficiency:** Removing redundant keywords (`declare`, `set`, `call`, `then`, `begin`/`end`) and using braces makes the code more compact for LLM processing.
- **Greater Expressiveness:** Fluent APIs for list manipulation allow for more complex logic to be expressed clearly and concisely.

Adopting these changes would represent a significant evolution for MDL, making it a more powerful and user-friendly language for Mendix development.
