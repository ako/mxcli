# Project Settings

Project settings control application runtime behavior, server configurations, language settings, and workflow configuration. MDL provides commands to inspect and modify all settings categories.

## Inspecting Settings

```sql
-- Overview of all settings
SHOW SETTINGS;

-- Full MDL output (round-trippable)
DESCRIBE SETTINGS;
```

## ALTER SETTINGS

Settings are organized into categories. Each `ALTER SETTINGS` command targets one category.

### Model Settings

Runtime-level settings such as the after-startup microflow, hashing algorithm, and Java version:

```sql
ALTER SETTINGS MODEL <Key> = <Value>;
```

Examples:

```sql
ALTER SETTINGS MODEL AfterStartupMicroflow = 'MyModule.ACT_Startup';
ALTER SETTINGS MODEL HashAlgorithm = 'BCrypt';
ALTER SETTINGS MODEL JavaVersion = '17';
```

Mendix renamed the Java version property between versions — up to 11.6 it is stored
as `JavaVersion` = `Java21`, from 11.12 as `JavaMajorVersion` = `21`. Write either
spelling (`'17'` or `'Java17'`): mxcli stores the value in whichever dialect the
project already uses.

### Configuration Settings

Server configuration settings like database type, URL, and HTTP port. Each configuration is identified by name (commonly `'default'`):

```sql
ALTER SETTINGS CONFIGURATION '<Name>' <Key> = <Value>;
```

Examples:

```sql
ALTER SETTINGS CONFIGURATION 'default' DatabaseType = 'POSTGRESQL';
ALTER SETTINGS CONFIGURATION 'default' DatabaseUrl = 'jdbc:postgresql://localhost:5432/myapp';
ALTER SETTINGS CONFIGURATION 'default' HttpPortNumber = '8080';
```

### Constant Overrides

Override a constant value within a specific configuration:

```sql
ALTER SETTINGS CONSTANT '<ConstantName>' VALUE '<value>' IN CONFIGURATION '<cfg>';
```

Example:

```sql
ALTER SETTINGS CONSTANT 'MyModule.ApiBaseUrl' VALUE 'https://staging.example.com' IN CONFIGURATION 'default';
```

### Language Settings

A project's **enabled languages** are the only ones a build emits translations
for. A translation written for any other language is stored in the model, passes
`mx check`, and is silently discarded at build time — so enabling the language is
the step that makes translating an app do anything.

```sql
ALTER SETTINGS LANGUAGE <Key> = <Value>;
ALTER SETTINGS LANGUAGE ADD [OR MODIFY] '<code>' [( <option>: <value>, ... )];
ALTER SETTINGS LANGUAGE MODIFY '<code>' ( <option>: <value>, ... );
ALTER SETTINGS LANGUAGE REMOVE '<code>';
```

```sql
-- enable a language; the defaults match Studio Pro's Add Language dialog
ALTER SETTINGS LANGUAGE ADD 'de_DE';

-- with options
ALTER SETTINGS LANGUAGE ADD 'ar_SD' (CheckCompleteness: true, CustomDateFormat: 'yyyy-MM-dd');

-- change one that is already enabled (only the options named are touched)
ALTER SETTINGS LANGUAGE MODIFY 'de_DE' (CheckCompleteness: true);

-- make it the default (it must already be enabled)
ALTER SETTINGS LANGUAGE DefaultLanguageCode = 'de_DE';

-- disable it
ALTER SETTINGS LANGUAGE REMOVE 'de_DE';
```

A language is identified by its **code** alone — Studio Pro's "Arabic, Sudan" is
derived from `ar_SD` for display and is not stored in the model.

| option | meaning |
|--------|---------|
| `CheckCompleteness` | report errors and warnings for texts that have no translation in this language, instead of letting them fall back to the default silently. The **default language is always checked** whatever this says. |
| `CustomDateFormat`, `CustomTimeFormat`, `CustomDateTimeFormat` | override Mendix's date/time formatting for this language. Empty means default formatting. |

`ADD OR MODIFY` is the upsert — it enables a language that is not there and
changes one that is. It is what `DESCRIBE SETTINGS` emits, so a described project
replays onto one that already has some of its languages.

Two things to know about removal:

- The **default language cannot be removed** — every missing translation falls
  back on it. Change `DefaultLanguageCode` first.
- Removing a language **does not delete its translations**. They stay in the
  model and stop being built; the run reports how many source strings are
  affected. To remove them, use
  `CREATE OR REPLACE TRANSLATIONS FOR '<code>' ( );`.

#### Set the default language *before* authoring content

The default language is not only a fallback — it is **the language a new caption
is written in**. `Caption: 'Opslaan'` has to be stored under some language code
(Mendix has no language-neutral text), and mxcli uses the project's
`DefaultLanguageCode`.

So the order of these two statements changes the result:

```sql
-- right: the page's texts are stored as nl_NL
ALTER SETTINGS LANGUAGE ADD 'nl_NL';
ALTER SETTINGS LANGUAGE DefaultLanguageCode = 'nl_NL';
CREATE PAGE MyModule.Opslaan ( Title: 'Opslaanpagina', ... ) { ... }

-- wrong: the page is authored while en_US is still the default, so its texts
-- are stored as en_US and stay there
CREATE PAGE MyModule.Opslaan ( Title: 'Opslaanpagina', ... ) { ... }
ALTER SETTINGS LANGUAGE DefaultLanguageCode = 'nl_NL';
```

Changing `DefaultLanguageCode` **does not move text that already exists** — it
would be rewriting content you authored. Nothing reports the mismatch either:
`mx check` gives 0 errors both ways, and the symptom only appears in Studio Pro,
as the empty-caption placeholder plus a "no translation for this language"
warning.

**If you get the order wrong, re-run the `CREATE` statements.** With the new
default in place the texts are stored under it; the old language's copy stays
alongside as a harmless extra translation.

```sql
-- recovery: same script, run again after the default is correct
CREATE OR MODIFY PAGE MyModule.Opslaan ( Title: 'Opslaanpagina', ... ) { ... }
```

`CREATE TRANSLATIONS FOR '<the default>'` is **not** the way out — it is refused,
because the default is the source language every other translation is keyed on.

> `SHOW LANGUAGES` lists languages that have **translations**, which is not the
> same list — a stock app reports eight while one is enabled. For the enabled
> list use `DESCRIBE SETTINGS`.

### Workflow Settings

Configure workflow behavior such as the user entity and task parallelism:

```sql
ALTER SETTINGS WORKFLOWS <Key> = <Value>;
```

Examples:

```sql
ALTER SETTINGS WORKFLOWS UserEntity = 'Administration.Account';
ALTER SETTINGS WORKFLOWS DefaultTaskParallelism = '5';
```

## See Also

- [Navigation and Settings](./navigation.md) -- overview of navigation and settings
- [Navigation Profiles](./navigation-profiles.md) -- navigation profile configuration
- [Workflows](./workflows.md) -- workflow definitions
