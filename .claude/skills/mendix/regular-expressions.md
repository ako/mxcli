# Regular Expressions

## When to Use This Skill

Use this skill when the user wants to:
- Add a named validation pattern (email, phone, identifier, postcode)
- See or change the regexes a project already has
- Understand why a regex cannot be attached to an attribute from MDL

## What a Mendix regular expression is

A **document**, not a string on a rule. An attribute validation rule stores a
reference to it by qualified name, so one pattern is shared by every attribute
that validates against it. That is why it gets a `create` statement of its own.

```sql
list regular expressions;
list regular expressions in Val;
describe regular expression Val.EmailAddress;   -- re-executable MDL

create regular expression Val.EmailAddress (
  Expression: '\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+',
  Documentation: 'A, not too restrictive, email address regular expression'
);

drop regular expression Val.EmailAddress;
```

| Property | Meaning | Default |
|----------|---------|---------|
| `Expression` | the pattern — **required** | — |
| `Documentation` | free text | none |
| `ExportLevel` | `Hidden` or `Public` | `Hidden` |

## Writing the pattern

The pattern is an ordinary MDL string:

- **Backslashes are NOT escape characters.** Write `\d`, `\w`, `\.` exactly as
  Mendix should see them — do not double them.
- **A single quote is doubled**, like any MDL string: `'^it''s$'`.
- Commas, braces and pipes inside the pattern are fine — the whole thing is one
  quoted string.

## Go cannot check every legal pattern

Mendix validates with **.NET's** regex engine, which accepts constructs Go's RE2
does not — lookaround and backreferences most commonly. The Mendix Email
Connector itself ships `.*(?<!/)$`.

mxcli stores such a pattern unchanged and `describe` adds:

```
-- note: uses .NET regex syntax that Go cannot compile (e.g. lookaround); not verifiable here
```

That is a note, not an error. Do not "fix" a pattern because mxcli could not
compile it.

## You cannot bind a regex to an attribute from MDL yet

`create validation rule ... regex Attr ...` has grammar but **no
implementation** — it parses and silently does nothing (true of every validation
rule form). Create the regex document with mxcli, then attach it to the attribute
in Studio Pro.

Required and Unique rules *are* authorable, as attribute constraints:

```sql
create entity Val.Person (
  Email: String(200) not null error 'Email is required',
  Code:  String(20)  unique error 'Code must be unique'
);
```

Once an entity has a **RegEx or Range** rule, mxcli **refuses** to rewrite it
(`alter entity`, `create or replace entity`) rather than silently turning it into
a Required rule, which is what it used to do — the constraint would vanish and
the build would still pass. Change such an entity in Studio Pro.

## Finding out who uses one

```sql
show references to Val.EmailAddress;
```

lists the entities whose validation rules use that pattern — worth checking
before you change a shared regex.

```sql
select QualifiedName, Expression from CATALOG.REGULAR_EXPRESSIONS;
```

## Common Mistakes

| Mistake | Symptom | Fix |
|---------|---------|-----|
| Doubling backslashes | The stored pattern has `\\d` and matches nothing | Write `\d` once |
| Single quote left undoubled | Parse error | `'^it''s$'` |
| Omitting `Expression` | `has no Expression` | It is required |
| Treating the Go note as an error | A valid .NET pattern gets rewritten | The note means "not verified", not "invalid" |
| Expecting `create validation rule` to work | It reports nothing and writes nothing | Attach the regex in Studio Pro |

## Related

- `mxcli syntax regular-expression` — full syntax reference
- `mdl-entities.md` — attributes and validation
