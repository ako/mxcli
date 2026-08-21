# Test Annotations

Annotations describe what a test is and what must be true when it finishes. They
live in the javadoc comment above a test's statements, one per line, each opening
its line:

```mdl
/**
 * @test dealing a board writes 81 cells
 * @cleanup none
 * @expect $result = 'ok'
 * @verify select count(*) as n from Sudoku.Cell = 81
 */
$result = call microflow Sudoku.ACT_DealGame();
/
```

| Tag | Purpose |
|-----|---------|
| `@test <name>` | Names the test. Required — a doc comment without it is not a test. |
| `@expect <condition>` | A Mendix expression over the body's variables that must be true. Repeatable. |
| `@verify <oql> <op> <value>` | An OQL post-condition on the database. Repeatable. |
| `@throws '<message>'` | The body is expected to raise an error. |
| `@setup <Module.Microflow>` | A microflow to call before the body. Repeatable. |
| `@cleanup rollback\|none` | Whether the test's writes survive it. `rollback` is the default. |

A tag is read only when it **opens its line** (after the javadoc `*` and its
indentation). Quoting one inside a sentence — `` `@expect $x = 1` `` — is
documentation, not an assertion.

## `@test`

Names the test, and marks the doc comment as one. Everything between that comment
and the `/` that ends the block is the test's body.

A file may open with a header comment, in either spelling, and it is not part of
the first test. Exactly one `@test` may appear per block: the `/` is what ends a
test, and leaving it out is refused by name rather than silently running one of
the two tests it merges.

## `@expect`

Any Mendix expression that must evaluate to true — not a fixed `$var = value`
shape:

```mdl
@expect $result = 'John Doe'                    -- equality
@expect $product/Name != 'Widget'               -- inequality (<> is accepted too)
@expect length($result) = 81                    -- built-in functions
@expect find($result, '0') >= 0 and $count > 3  -- and / or / not(...)
@expect $status = MyModule.Status.Open          -- enumeration values
@expect count($Customers) = 5                   -- how many rows a list holds
```

`count($list)` is the one aggregate an assertion can make. Counting a list is a
Mendix Aggregate list **activity**, not an expression function, so mxcli lifts it
into that activity ahead of the decision that evaluates the condition. `sum`,
`average`, `minimum` and `maximum` aggregate an *attribute* over the list, which
an assertion has no way to supply — call a microflow that returns the figure and
assert on its result.

**An assertion the runner cannot compile is an ERROR, never a pass.** Unknown
functions, wrong arity, unbalanced parentheses, and expressions that evaluate to
a value rather than a condition are all rejected by name:

```
ERROR  a self-evident falsehood
       @expect randomInt($result) = 1: randomInt() is not a Mendix expression
       function at column 1 ("randomInt")
```

A failing assertion reports what came back, whenever the observed value's type is
pinned down by the assertion itself:

```
FAIL  the board is 81 squares
      expected length($result) = 81, actual: 27
```

`@expect` cannot be combined with `@throws`: a body expected to fail produces no
result to assert on, so the combination is refused rather than ignored.

## `@verify`

`@expect` only sees what a microflow returned. Most Mendix microflows are side
effects, so `@verify` asserts on the rows one left behind — an OQL query, a
comparison operator, and the value it must satisfy:

```mdl
@verify select count(*) as n from Sudoku.Cell = 81
@verify select count(*) as n from Sudoku.Cell where Value = 0 > 0
```

The query runs after the microflow returns, over the same admin API `mxcli oql`
uses. Three rules follow:

- The result must be **one row and one column** (`select count(*) as n …`, or one
  attribute of one row). OQL requires the column to be named, so write `as n`.
- `@cleanup rollback` — the default — is **refused** with `@verify`: the writes
  would be undone before the query could see them. Add `@cleanup none`.
- The **legacy after-startup runner refuses the whole suite**, because its tests
  run during boot with nothing to query yet. Use `--local` or `--attach`.

A query that cannot be evaluated is an ERROR, distinct from one that returns the
wrong value (FAIL).

## `@throws`

Marks a body that is expected to raise an error. The verdict starts as a failure
and only the error handler clears it, so a body that completes normally fails the
test.

```mdl
/**
 * @test rejects an empty order
 * @throws 'validation failed'
 */
$result = call microflow Sales.ACT_Submit(Order = $empty);
/
```

## `@setup`

Names a **microflow** to call before the test's own statements — a fixture in a
Mendix app is a microflow, so there is nothing to declare:

```mdl
/**
 * @test the seed microflow writes five brands
 * @setup eShop.ACT_SeedCatalog
 * @cleanup none
 * @expect count($Brands) = 5
 */
retrieve $Brands from eShop.CatalogBrand;
/
```

Repeat it to compose fixtures; they run in the order written. Declare it once in
the file's header comment and every test in the file gets it, the file's
fixtures first:

```mdl
/**
 * Seeds every test below.
 * @setup eShop.ACT_SeedCatalog
 */
```

A header may carry only `@setup`. `@expect`, `@verify`, `@throws` and `@cleanup`
describe one test's execution, so a header carrying one is refused by name.

The setup runs **inside the test's transaction**, so under the `@cleanup
rollback` default it is undone with the test and every test starts from the same
state. A failing setup is an **ERROR** naming the microflow, not a FAIL: the test
never ran, and a broken fixture should not read as a broken feature.

`@setup` calls a microflow with no arguments — a fixture that needs arguments
gets a wrapper microflow. There is no `@teardown`; `@cleanup rollback` is the
teardown.

## `@cleanup`

`rollback` (the default) wraps the test in a transaction that is rolled back when
it returns, so its writes do not survive it. `none` leaves them in place — needed
whenever a later test, or a `@verify`, has to see them. An unknown strategy is a
parse error.

## Related Pages

- [Test Formats](test-formats.md) — the `.test.mdl` and `.test.md` file layouts
- [Running Tests](running-tests.md) — `mxcli test`, `--local`, `--watch`, `--attach`
