# Test Formats

`mxcli test` reads two file formats: `.test.mdl` for plain MDL tests, and
`.test.md` for tests embedded in documentation. Both describe the same thing — a
named test, some MDL to run against the app, and what must be true afterwards —
and both use the same [annotations](test-annotations.md).

A test's body calls into the app. It is not a script that builds a project: the
statements run against a booted runtime, and the assertions are about what they
returned or wrote.

## `.test.mdl`

A test is a javadoc comment followed by its statements. A line containing just
`/` ends it, the way it ends any MDL block:

```mdl
/**
 * @test concatenating a name
 * @expect $result = 'John Doe'
 */
$result = call microflow MyModule.ConcatNames(
  FirstName = 'John', LastName = 'Doe'
);
/

/**
 * @test the seed microflow writes five brands
 * @cleanup none
 * @expect count($Brands) = 5
 */
retrieve $Brands from eShop.CatalogBrand;
/
```

Two rules about layout are worth knowing, because getting them wrong used to be
silent:

- **A file may open with a header comment** — a `/** … */` block or `--` lines —
  and it is not part of the first test. The test's own doc comment is the last
  one above its statements. A `/** … */` header may carry
  [`@setup`](test-annotations.md#setup), which then applies to every test in the
  file.
- **One `@test` per block.** Because the `/` is what ends a test, omitting it
  merges two tests into one. That is refused by name rather than resolved, since
  either resolution runs one of the two and drops the other.

## `.test.md`

The same tests, inside `mdl-test` fenced code blocks, with prose around them. One
block is one test; everything outside the fences is documentation and is ignored.

````markdown
# Customer module

## Names are concatenated in display order

Given a first and a last name, `ConcatNames` returns them separated by a space:

```mdl-test
/**
 * @test concatenating a name
 * @expect $result = 'John Doe'
 */
$result = call microflow MyModule.ConcatNames(
  FirstName = 'John', LastName = 'Doe'
);
```
````

Use it when the reasoning around a test is worth as much as the test — a
specification that stays honest because it runs.

## File organisation

`mxcli test` takes a file or a directory. A directory run picks up every
`*.test.mdl` and `*.test.md` in it (not recursively) and reports them as one
suite.

```
tests/
├── domain-model.test.mdl
├── microflows.test.mdl
├── security.test.mdl
└── ordering.test.md
```

Names are yours to choose; the `.test.mdl` / `.test.md` suffix is what makes a
file a test file.

## Related Pages

- [Test Annotations](test-annotations.md) — `@test`, `@expect`, `@verify`, `@throws`, `@cleanup`
- [Running Tests](running-tests.md) — `mxcli test`, `--local`, `--watch`, `--attach`
