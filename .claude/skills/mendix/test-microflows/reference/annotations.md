# Test annotation reference

Supporting reference for [test-microflows](../SKILL.md).

## Annotations

| Tag | Purpose | Example |
|-----|---------|---------|
| `@test` | Test name (required) | `@test string concatenation` |
| `@expect` | Assert a Mendix condition | `@expect $result = 'John Doe'` |
| `@expect` | Assert an entity attribute | `@expect $product/Name = 'TestProduct'` |
| `@expect` | Assert with a built-in | `@expect length($result) = 81` |
| `@verify` | OQL post-condition on the database | `@verify select count(*) as n from Mod.E = 1` |
| `@throws` | Expect any error | `@throws` |
| `@throws` | Expect an error whose message contains this | `@throws 'validation failed'` |
| `@setup` | Microflow to run first | `@setup MyModule.ACT_SeedCustomers` |
| `@cleanup` | Rollback strategy | `@cleanup rollback` (default) or `@cleanup none` |

A tag is read only when it **opens its line** (after the javadoc `*` and its
indentation). Quoting one inside a sentence — ``a test with `@expect $x = 1`
asserts …`` — is documentation, not an annotation, so a doc comment can explain
itself without giving the test assertions nobody wrote.

### A test run leaves the project byte-identical

`mxcli test` injects an `MxTest` module, builds, runs, and takes the injection
back out. When cleanup succeeds the project file is restored **byte-for-byte**,
so `git status` is clean afterwards and a CI step of the form "run the tests,
then assert the tree is clean" holds.

This needs saying because restoring the *model* is not enough. Every unit write
stamps a fresh UUID into the `.mpr`'s `_Transaction` bookkeeping row, and the
inject/remove cycle relays SQLite's pages, so the file differs even once its
content matches again. Version control compares bytes, and a `.mpr` diff is
opaque — there is no cheap way to tell a bookkeeping GUID from a real model edit,
which is what made the spurious modification expensive rather than merely untidy.

The restore is declined, deliberately, when **cleanup failed** or when the
`mprcontents/` tree changed during the run. In both cases the project is not in
the state the snapshot describes, and putting the old file back would turn a
visible, harmless discrepancy into an invisible, misleading one.

### `@expect` — any Mendix condition, and nothing it cannot evaluate

An `@expect` is **a Mendix expression that must evaluate to true**, not a fixed
`$var = value` shape. Anything the Mendix expression engine accepts works:

```mdl
@expect $result = 'John Doe'                       -- equality
@expect $product/Name != 'Widget'                  -- inequality (<> also accepted)
@expect length($result) = 81                       -- built-in functions
@expect find($result, '0') >= 0                    -- any comparison operator
@expect substring($result, 0, 9) = substring($result, 9, 18)
@expect find($result, '0') >= 0 and $count > 3     -- and / or / not(...)
@expect $status = MyModule.Status.Open             -- enumeration values
@expect count($Customers) = 5                      -- how many rows a list holds
```

`<>` is accepted in the annotation and rewritten to `!=` on the way to the
model, because Mendix's expression engine rejects the `<>` spelling (CE0117).

**An assertion the runner cannot compile is an ERROR, never a pass.** Unknown
functions, wrong arity, unbalanced parentheses and expressions that evaluate to
a value rather than a condition are all rejected by name:

```
ERROR  a self-evident falsehood
       @expect randomInt($result) = 1: randomInt() is not a Mendix expression
       function at column 1 ("randomInt")
```

This is the one rule the annotation is built around. A test framework that
cannot evaluate an assertion has exactly one safe behaviour, and passing is not
it — an earlier version matched only `$var = <literal>` and silently discarded
every other line, so `@expect 1 = 2` reported PASS.

**A failure reports what came back**, not just what was wanted, whenever the
observed value's type is pinned down by the assertion itself:

```
FAIL  the board is 81 squares
      expected length($result) = 81, actual: 27
```

The value is omitted rather than guessed when neither side of the comparison
establishes a type (`@expect $a = $b`), because Mendix's expression engine is
typed and a wrong guess would break the build instead of the test.

#### `count($list)` — the one aggregate an assertion can make

Counting a list is not a Mendix *expression* function; it is an Aggregate list
**activity**, so it cannot appear in the decision that evaluates an assertion.
`@expect count($Scans) = 2` is nevertheless accepted: the count is lifted into
the activity you would otherwise write by hand, ahead of the decision, and the
condition compares its result.

```mdl
/**
 * @test the seed microflow writes five brands
 * @cleanup none
 * @expect count($Brands) = 5
 */
retrieve $Brands from eShop.CatalogBrand;
/
```

The other four aggregates (`sum`, `average`, `minimum`, `maximum`) aggregate an
**attribute** over the list, which an assertion has no way to supply, so they are
refused with that explanation. Call a microflow that returns the figure and
assert on its result:

```mdl
$Total = call microflow eShop.QRY_OrderTotal();
```

The refusal matters more than the convenience: before it, a count assertion was
dropped during parsing, and a test with no assertions left passes as long as its
body does not throw — so `@expect count($Brands) = 999` reported PASS against an
empty table (#927).

### `@setup` — the state a test needs before it runs

`@setup` names a **microflow** to call before the test's own statements. A
fixture in a Mendix app is a microflow, so there is nothing to declare:

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

Repeat it to compose fixtures; they run in the order written. Declare it **once
in the file's header comment** and every test in the file gets it, with the
file's fixtures running before a test's own:

```mdl
/**
 * Seeds every test below.
 * @setup eShop.ACT_SeedCatalog
 */
```

The header is the file's first doc comment when it carries no `@test`. It may
only carry `@setup` — `@expect`, `@verify`, `@throws` and `@cleanup` describe one
test's execution, so a header carrying one is refused by name rather than
silently ignored.

Two consequences worth knowing:

- **The setup runs inside the test's transaction.** Under the `@cleanup
  rollback` default it is undone with the test, so every test starts from the
  same state — which is what makes a fixture worth having. Under `@cleanup none`
  it persists like everything else that test writes.
- **A failing setup is an ERROR, not a FAIL**, naming the microflow. The test
  never ran, so it neither passed nor failed, and a suite full of assertion
  mismatches caused by one broken seed is exactly what this prevents.

`@setup` calls a microflow with no arguments; a fixture that needs arguments
gets a wrapper microflow. There is no `@teardown` — `@cleanup rollback` is the
teardown.

### `@verify` — asserting on what the microflow wrote

`@expect` can only see what a microflow **returned**. Most Mendix microflows are
side effects, so `@verify` is how you assert on the rows one left behind: an OQL
query, a comparison, and the value it must satisfy.

```mdl
/**
 * @test dealing a board writes 81 cells
 * @cleanup none
 * @expect $result = 'ok'
 * @verify select count(*) as n from Sudoku.Cell = 81
 * @verify select count(*) as n from Sudoku.Cell where Value = 0 > 0
 */
$result = CALL MICROFLOW Sudoku.ACT_DealGame();
/
```

The query runs against the app **after** the microflow returns, over the same
admin API `mxcli oql` uses. Three rules follow from that, and each is enforced
rather than left to trip you up:

- **`@cleanup none` is required.** `rollback` is the default, and it undoes the
  test's writes before the query could see them — so a `@verify` on a rollback
  test is **refused**, not run against the pre-test state.
- **The query must return exactly one row and one column.** Comparing a table to
  a literal would mean guessing which cell was meant. Aggregate it
  (`select count(*)`), or select one attribute of one row.
- **The expected value is a literal** — a number, a quoted string, `true`/`false`
  or `empty`. It is split off at the **last** comparison operator outside quotes
  and parentheses, so a `where Value = 5` in the query is left alone.
- **Every selected column needs a name.** Mendix's OQL rejects a bare
  `select count(*)` with *"All OQL select columns must have a name"*, so write
  `select count(*) as n`. That comes back as an ERROR, not a pass.

Operators: `=`, `!=` (`<>` accepted), `<`, `<=`, `>`, `>=`. Numbers compare
numerically even though the runtime returns them as strings.

**A `@verify` that cannot be evaluated is an ERROR, never a pass** — an unknown
entity, malformed OQL, a non-scalar result, or something that was never a query:

```
ERROR  dealing a board writes 81 cells
       @verify select count(*) as n from Sudoku.NoSuch = 1: OQL error: Unknown entity
```

and a false one fails with the value that came back:

```
FAIL  dealing a board writes 81 cells
      expected select count(*) as n from Sudoku.Cell = 81, actual: 27
```

`@verify` needs the test endpoint, so it runs under `--local` (the default) and
`--attach`. The Docker / `--legacy-runner` path **refuses** a suite using it:
its tests execute during boot, so there is no point at which to query the app.

### A test that asserts nothing says so

Every result line carries what the test actually checked, and a run that
contains a vacuous test calls it out:

```
  PASS  the board is 81 squares (6ms, 2 assertions)
  PASS  asserts nothing at all (4ms, no assertions)
------------------------------------------------------------
1 test(s) asserted nothing beyond "did not throw". Run with
--require-assertions to make that an error.
```

A test with no `@expect` and no `@throws` is a **smoke test** — it reports only
that the body did not throw. That is a legitimate thing to write, so it still
passes by default. What it may not do is look identical to a test with six
assertions: after `@expect` started failing closed, the cheapest way back to a
green suite is to delete the assertion, and that must not read as a repair.

`--require-assertions` turns every vacuous test into an ERROR, for a project
that has decided each test must assert. The JUnit report carries the count as a
`<property name="assertions">` per case, and `classname` now identifies the
source file, so a failure in a multi-file run says where it lives.

### `@cleanup` — what happens to a test's data

**`rollback` is the default**, so by default a test's database writes do not
survive it. The endpoint opens a transaction around the call and rolls it back
afterwards, including when the test throws.

```mdl
/**
 * @test creating an order does not leak
 * @expect $result = 'ok'
 */
$result = CALL MICROFLOW Sales.CreateOrder(Amount = 100);
/

/**
 * @test seed data the next test needs
 * @cleanup none
 */
$result = CALL MICROFLOW Sales.SeedCatalogue();
/
```

Use `@cleanup none` when the writes are the point — seeding a fixture, or
inspecting the result in the running app afterwards.

Two things worth knowing:

- **`--local` only.** Rollback needs the test endpoint, which owns the context
  the test runs in. The Docker / `--legacy-runner` path executes tests inside
  the after-startup action and has no such seam, so it always commits.
- **A rollback that fails is reported, loudly.** The run prints a `WARNING` per
  affected test and a summary line, because the alternative — data left behind
  while the suite still says PASS — is the failure mode this annotation exists
  to prevent. `--verbose` tags every test with `[rolled back]`, `[committed]` or
  `[ROLLBACK FAILED]`.

A misspelled strategy (`@cleanup rollbak`) is a **parse error**, not a silent
fallback to committing.

Rollback matters most under `--attach`, where the database is the one your dev
app is using.

---
