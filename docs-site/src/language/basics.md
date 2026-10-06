# MDL Basics

MDL (Mendix Definition Language) is a SQL-like language for reading and modifying Mendix application projects. It provides a text-based alternative to the visual editors in Mendix Studio Pro.

## What MDL Looks Like

MDL uses familiar SQL-style syntax with Mendix-specific extensions. Here is a simple example that creates an entity with attributes:

```sql
CREATE PERSISTENT ENTITY Sales.Customer (
  CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
  Name: String(200) NOT NULL,
  Email: String(200) UNIQUE,
  IsActive: Boolean DEFAULT TRUE
)
INDEX (Name);
```

## Statement Termination

Every statement ends with a semicolon (`;`):

```sql
mdl 1;
CREATE MODULE OrderManagement;

CREATE PERSISTENT ENTITY Sales.Order (
  OrderId: AutoNumber NOT NULL UNIQUE,
  OrderDate: DateTime NOT NULL,
)
INDEX (OrderDate DESC);
```

A missing `;` is an error, and so is the Oracle SQL*Plus-style `/` on its own line. At the REPL and in `mxcli -c "…"` the end of the input ends the last statement, so a single command such as `list entities` needs no terminator there.

## Trailing Commas

A trailing comma is allowed in every bracketed list — attributes, enumeration values, parameters, property lists, `{ … }` blocks — under every language version. `()` is the only way to write an empty list: `(,)` and `(a,,)` are errors.

## Keyword Case

Keywords are case-insensitive, and lowercase is canonical: `describe` writes them in lowercase and `mxcli fmt` normalizes them to it. Names are not keywords, even when they are spelled like one — a module member `User`, an attribute `Title` or a property key `Folder:` keeps its case, and so does a CamelCase value such as `ButtonStyle: Success` or a type name such as `String(200)`. Expressions, XPath, OQL and SQL are stored as written, so `fmt` leaves their text alone.

## One Spelling per Keyword

Each keyword has one spelling, and a page action uses the words a microflow uses: `show page`, `save changes`, `cancel changes`, `close page`, `create object`, `delete`, `open link`, `sign out`, `complete task`, `call microflow M.F`, `call nanoflow M.F`. A validation message is `error message '…'` (after `not null`, `unique`, `required`, in a validation rule and after `on delete restrict`); a delete behaviour is the SQL referential action, `on delete cascade` / `restrict` / `set null`; a reference set's type is `ReferenceSet`; a REST call that returns nothing says `returns nothing`.

## One Verb per Job

`list` enumerates, `describe` shows one thing, and an `alter` adds and drops its children:

- `describe page M.P`, `describe app security`, `describe security matrix [in M]`, `describe structure …`, `describe context of X`;
- `list navigation`, `list navigation menu [profile]`, `list settings` (they print tables, so they are listings);
- `alter user role R drop module roles (…)`, `alter settings language drop '…'`, `alter settings workflows drop group '…'`;
- `alter entity E add|rename|modify|drop attribute …`;
- `call rest service get '…' …`, `describe widget type combobox`, `create fragment F as { … }`.

`show entity X` and `show association X` are not `mdl 1`: `describe entity X` prints the definition as MDL, and `list entities in M` / `list associations in M` print the summary columns. `show version`, `show status`, `show connections` and `show catalog status` report the session, not the model: they are session commands (R7), typed at the REPL.

## Activity Settings

A microflow or nanoflow activity says what it does in words, and lists its
settings in one property list ([ADR-0013](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0013-activity-settings-property-list.md)):

- **Words** carry the action and its data flow: the verb, the result variable
  (`$x =`), the main operand (the method and URL, the called document, the
  object), `returns …` and `on error …`.
- **One `( Key: value, … )` list after the main operand** carries what a Studio
  Pro user sets in the activity's properties dialog: headers, authentication,
  timeout, request body. `:` sets a setting, `=` binds a value, trailing commas
  are allowed, and an unknown key is an error.

```sql
$Html = call rest service get 'https://example.com' (
  Headers: ('Accept': 'text/html'),
  Timeout: 300,
) returns String;
```

The keys are named as the matching document property is (`Headers`,
`Authentication`, `Timeout`, as on a consumed REST service). An activity with
one or two settings that read naturally as words keeps its clauses —
`log info node 'App' 'text'`, `show message 'Saved' type Information`.

## Document Type Names

Document types are named as Studio Pro names them: `consumed rest service(s)`, `consumed odata service(s)`, `published odata service(s)`, `task queue(s)`, `alter app security …`, `alter settings runtime …`, `list image collections` / `list icon collections` / `list message definition collections`, `ai model(s)` (the agent editor's model document), and `create json structure M.J sample '…'`.

The older spellings of all three sections still parse in a script file with the same meaning, warn with an `MDL-DEPR*` code, and `mxcli fmt --upgrade` rewrites them; [Language Versions and Migration](versions.md) lists every one.

`consumed web service`, `published web service` and `xml schema` are reserved: MDL does not support these Studio Pro documents yet, and a statement that names one is refused with an error saying so.

`business event service` and `database connection` keep their names: a Mendix 10+ business event service document holds both the published and the subscribed operations, and `database connection` is the Database Connector's own name for the document.

## Language Version Header

A script starts with a header that names the MDL language version it is written in:

```sql
mdl 1;

create persistent entity Sales.Customer (
  Name: String(200)
);
```

- **`mdl 1`** is the language this manual documents. It was frozen at beta: a script headed `mdl 1;` means the same under every later mxcli release.
- The header is the **first** statement. It may be repeated later with the same version — descriptions concatenated into one file each start with it — but a header naming another version is an error, and so is a version this mxcli does not know.
- `describe` writes `mdl 1;` at the top of every description, so a description runs as it is, and `mxcli fmt --upgrade` adds it to an older script.
- At the REPL and in `mxcli -c "…"`, input without a header is read as `mdl 1`.
- A script **file** without a header is the older language, `mdl 0`, and keeps its meaning (constructs that mean something else under `mdl 1` warn). How it differs, and how to upgrade it, is on [Language Versions and Migration](versions.md).
- It is independent of the Mendix version your project targets.

What `mdl 1` makes strict:

- Every statement ends with `;`, and a `/` terminator line is an error.
- `''` is the only string escape; a backslash is an ordinary character, so `'C:\temp'` is that path.
- A text template (`log`, `show message`, `validation feedback`) written as one literal is the template text even when it spans lines, which is how a template holds a line break.
- In a REST client, published REST service, business event service, model, knowledge base, consumed MCP service or agent, an unknown property key is an error that names the key it most likely meant, and so is a value its key does not take (`Response: json from $X`).
- A `while` loop is `while <condition> begin … end while;`; leaving out `begin`, or the `while` after `end`, is an error.
- `show entity X` / `show association X` is an error: `describe entity X` prints the definition, `list entities in M` the summary columns.
- A session command — `connect`, `disconnect`, `use`, `set format = …`, `status`, `show version`, `show status`, `show connections`, `show catalog status`, `check`, `build`, `lint`, `debug`, `execute script`, `execute runtime`, `help`, `introspect api` — is an error in a script. Type it at the REPL, or use the command-line flag (`mxcli exec script.mdl -p app.mpr --json`).
- `DynamicClasses`, `DynamicCellClass` and an OData client's credential or header value take the expression bare; a quoted value is a Mendix string.

The manual, the skills, `mxcli syntax` and the example scripts are written in the canonical form, and CI holds them to it: `make check-conformance` parses every MDL block in them as `mdl 1` and fails on a deprecated spelling (`mxcli check --deprecations=error` does the same for a script of your own). The design is in [ADR-0011](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0011-mdl-language-versioning.md); `mxcli syntax language-header` has the details.

## Re-runnable Creates: `or modify` and `if not exists`

A plain `create` fails when the element already exists. Two guards make a script re-runnable, and they mean different things:

| Statement | Element absent | Element present |
|---|---|---|
| `create page M.P …` | created | error — the script stops |
| `create or modify page M.P …` | created | rewritten to match the statement (identity kept) |
| `create page if not exists M.P …` | created | **left untouched**, reported as skipped |

`if not exists` goes after the kind's keywords and before the name, on every `create` that names one element — `create microflow if not exists M.MF () …`, `create module if not exists M;`, `create user role if not exists Clerk (M.User);`, `create configuration if not exists 'Default' (…);`. It is not accepted on `annotation`, `index` (use `alter entity … add index if not exists`), `validation rule`, `navigation`, `translations` or `external entities`, which have no single named element to test. Writing `create or modify … if not exists` is refused as `MDL085`: the two guards contradict each other. `describe` never emits `if not exists`.

## Case Insensitivity

All MDL **keywords** are case-insensitive. The following are equivalent:

```sql
CREATE PERSISTENT ENTITY Sales.Customer ( ... );
create persistent entity Sales.Customer ( ... );
Create Persistent Entity Sales.Customer ( ... );
```

Identifiers (module names, entity names, attribute names) are case-sensitive and must match the Mendix model exactly.

## Statement Categories

MDL statements fall into several categories:

| Category | Examples |
|----------|----------|
| **Query** | `LIST ENTITIES`, `DESCRIBE ENTITY`, `SEARCH` |
| **Domain Model** | `CREATE ENTITY`, `CREATE ASSOCIATION`, `ALTER ENTITY` |
| **Enumerations** | `CREATE ENUMERATION`, `ALTER ENUMERATION` |
| **Microflows** | `CREATE MICROFLOW`, `DROP MICROFLOW` |
| **Pages** | `CREATE PAGE`, `ALTER PAGE`, `CREATE SNIPPET` |
| **Security** | `GRANT`, `REVOKE`, `CREATE USER ROLE` |
| **Navigation** | `CREATE OR REPLACE NAVIGATION` |
| **Connection** | `CONNECT LOCAL`, `DISCONNECT`, `STATUS` |

## Further Reading

- [Lexical Structure](./lexical-structure.md) -- keywords, literals, and tokens
- [Qualified Names](./qualified-names.md) -- how elements are referenced
- [Comments](./comments.md) -- comment syntax
- [Script Files](./script-files.md) -- running MDL from files
