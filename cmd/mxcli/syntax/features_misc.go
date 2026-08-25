// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	// ── CREATE modifiers ────────────────────────────────────────────────

	// OR MODIFY / OR REPLACE sit on the top-level createStatement rule, so they
	// apply to every CREATE uniformly. Individual entries documented them
	// unevenly — most said nothing, none mentioned OR REPLACE — which read as
	// "not supported here". Documented once, and referenced from the entries
	// where the question comes up, rather than repeated across all 27.
	Register(SyntaxFeature{
		Path:    "create-modifiers",
		Summary: "OR REPLACE / OR MODIFY — re-running a CREATE without dropping first",
		Keywords: []string{
			"or replace", "or modify", "create or replace", "create or modify",
			"idempotent", "upsert", "re-run", "already exists", "overwrite",
		},
		Syntax: "CREATE [OR REPLACE | OR MODIFY] <anything>;\n\n" +
			"-- Applies to every CREATE statement — entity, microflow, page, workflow,\n" +
			"-- REST client, security role, and the rest. Without a modifier, creating\n" +
			"-- something that already exists is an error.\n" +
			"--   OR REPLACE  discard the existing document and write a fresh one\n" +
			"--   OR MODIFY   update the existing document in place\n" +
			"-- Both reuse the existing element's ID, so references from other\n" +
			"-- documents survive.",
		Example: "CREATE OR REPLACE MICROFLOW MyModule.ACT_Recalculate ()\nBEGIN\n  RETURN;\nEND;\n\nCREATE OR MODIFY PERSISTENT ENTITY MyModule.Customer (\n  Name: String(200)\n);",
		SeeAlso: []string{"microflow", "domain-model.entity", "page", "document-folder"},
	})

	// The folder clause is the other cross-cutting CREATE modifier, and gets one
	// topic for the same reason OR MODIFY does: it applies to every document
	// type, so documenting it in all 27 places would guarantee 27 chances to
	// drift.
	Register(SyntaxFeature{
		Path:    "document-folder",
		Summary: "FOLDER — placing a document in a module folder as you create it",
		Keywords: []string{
			"folder", "folder clause", "place document", "module folder",
			"create in folder", "organise", "organize", "subfolder",
			"folder on create", "folder ignored", "document did not move",
		},
		Syntax: "-- Every document type takes a folder clause on CREATE. Where it goes\n" +
			"-- depends on the statement's shape:\n" +
			"--   Pages, snippets       Folder: 'path'   a property, inside the parentheses\n" +
			"--   Microflows, nanoflows FOLDER 'path'    a keyword, before BEGIN\n" +
			"--   Everything else       FOLDER 'path'    a keyword, after the qualified name\n" +
			"--\n" +
			"-- Missing folders in the path are created. Nested paths use '/'.\n" +
			"--\n" +
			"-- On CREATE OR MODIFY the clause MOVES an existing document. Omitting it\n" +
			"-- leaves placement alone — it never returns a document to the module\n" +
			"-- root — so adding a folder to an existing script is safe, and removing\n" +
			"-- one is a no-op. DESCRIBE emits the clause, so a description replays\n" +
			"-- into the same folder.\n" +
			"--\n" +
			"-- To move a document without rewriting it, use MOVE.",
		Example: "CREATE QUEUE MyModule.Q_Orders FOLDER 'Private/Queues' ( Parallelism: 3 );\n\n" +
			"CREATE IMPORT MAPPING MyModule.IMM_Order FOLDER 'Private/Import mappings'\n" +
			"  WITH JSON STRUCTURE MyModule.JSON_Order {\n" +
			"    CREATE MyModule.Order { Id = id }\n" +
			"  };\n\n" +
			"CREATE PAGE MyModule.OrderList\n" +
			"  (\n" +
			"    Title: 'Orders',\n" +
			"    Folder: 'Orders',\n" +
			"    Layout: Atlas_Core.Atlas_Default\n" +
			"  )\n" +
			"  {\n" +
			"    DYNAMICTEXT txtHeading (Content: 'Orders')\n" +
			"  };\n\n" +
			"CREATE OR MODIFY MICROFLOW MyModule.ACT_Sync ()\nFOLDER 'Private/Jobs'\nBEGIN\n  RETURN;\nEND;",
		SeeAlso: []string{"move", "folders", "create-modifiers"},
	})

	// ── Connection ──────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "connect",
		Summary: "Connect to a Mendix project (.mpr file) for the current session",
		Keywords: []string{
			"connect", "connect local", "connect project",
			"open project", "mpr", "connection",
		},
		Syntax: `CONNECT LOCAL '<path/to/app.mpr>';
CONNECT LOCAL '<path>' BRANCH '<branch>';

-- CLI flags (equivalent)
mxcli -p <path/to/app.mpr> -c "<statement>"`,
		Example: `CONNECT LOCAL '/projects/MyApp/MyApp.mpr';

-- Read-write session
CONNECT LOCAL '/projects/MyApp/MyApp.mpr';
CREATE ENTITY MyModule.Product ( Name: String(200) );
DISCONNECT;`,
		SeeAlso: []string{"disconnect", "status"},
	})

	Register(SyntaxFeature{
		Path:    "disconnect",
		Summary: "Close the current project connection",
		Keywords: []string{
			"disconnect", "close", "close connection", "close project",
		},
		Syntax:  "DISCONNECT;",
		Example: "DISCONNECT;",
		SeeAlso: []string{"connect", "status"},
	})

	Register(SyntaxFeature{
		Path:    "status",
		Summary: "Show the current connection status: project path, Mendix version, and module count",
		Keywords: []string{
			"status", "show status", "connection status",
			"project info", "version", "connected",
		},
		Syntax:  "STATUS;\nSHOW STATUS;",
		Example: "STATUS;\n-- Output: Connected to /projects/MyApp/MyApp.mpr (Mendix 10.24.0, 5 modules)",
		SeeAlso: []string{"connect", "disconnect"},
	})

	// ── Navigation ──────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "navigation",
		Summary: "Navigation profiles — home pages, menus, login pages per device type",
		Keywords: []string{
			"navigation", "nav", "profile", "responsive", "phone", "tablet",
			"home page", "menu", "login page",
		},
		Syntax:  "SHOW NAVIGATION;\nDESCRIBE NAVIGATION [profile];\nCREATE OR REPLACE NAVIGATION <profile> ...;",
		Example: "SHOW NAVIGATION;\nDESCRIBE NAVIGATION Responsive;",
		SeeAlso: []string{"navigation.show", "navigation.create", "navigation.alter"},
	})

	Register(SyntaxFeature{
		Path:    "navigation.show",
		Summary: "List navigation profiles, menus, and home page assignments",
		Keywords: []string{
			"show navigation", "describe navigation", "navigation menu",
			"navigation homes", "list profiles",
		},
		Syntax:  "SHOW NAVIGATION;\nSHOW NAVIGATION MENU;\nSHOW NAVIGATION MENU <profile>;\nSHOW NAVIGATION HOMES;\nDESCRIBE NAVIGATION;\nDESCRIBE NAVIGATION <profile>;",
		Example: "SHOW NAVIGATION;\nSHOW NAVIGATION MENU Responsive;\nDESCRIBE NAVIGATION Responsive;",
	})

	Register(SyntaxFeature{
		Path:    "navigation.create",
		Summary: "Create or replace a navigation profile with home pages, menus, and login page",
		Keywords: []string{
			"create navigation", "replace navigation", "home page",
			"login page", "not found page", "menu item", "menu icon",
		},
		Syntax: `CREATE OR REPLACE NAVIGATION <profile>
  HOME PAGE Module.Page
  [HOME PAGE Module.Page FOR Module.UserRole]
  [LOGIN PAGE Module.LoginPage]
  [NOT FOUND PAGE Module.Custom404]
  [MENU (
    MENU ITEM 'Label' PAGE Module.Page [ICON Module.IconCollection.Name];
    MENU 'Group' [ICON Module.IconCollection.Name] ( ... );
  )];

-- ICON is a qualified name into an ICON COLLECTION (Atlas_Core.Atlas,
-- Atlas_Core.Atlas_Filled, Atlas_Core.Atlas_Styling, or your own) -- a model
-- reference, not a string. Hyphenated Atlas names are double-quoted:
--   ICON Atlas_Core.Atlas."align-center"
-- Browse the available names with:
--   SHOW ICON COLLECTION  /  DESCRIBE ICON COLLECTION Module.Name`,
		Example: `CREATE OR REPLACE NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminDashboard FOR Administration.Administrator
  LOGIN PAGE Administration.Login
  MENU (
    MENU ITEM 'Home' PAGE MyModule.Home_Web ICON Atlas_Core.Atlas.home;
    MENU 'Orders' ICON Atlas_Core.Atlas."shopping-cart" (
      MENU ITEM 'All Orders' PAGE Orders.Order_Overview ICON Atlas_Core.Atlas."list-bullets";
      MENU ITEM 'New Order' PAGE Orders.Order_New ICON Atlas_Core.Atlas.add;
    );
  );`,
		SeeAlso: []string{"navigation.show"},
	})

	Register(SyntaxFeature{
		Path:    "navigation.alter",
		Summary: "Modify navigation via round-trip: DESCRIBE, edit, CREATE OR REPLACE",
		Keywords: []string{
			"alter navigation", "modify navigation", "update navigation",
			"round-trip", "edit menu",
		},
		Syntax:  "-- Round-trip workflow:\n-- 1. DESCRIBE NAVIGATION <profile>;\n-- 2. Copy output, modify\n-- 3. Paste as CREATE OR REPLACE NAVIGATION ...",
		Example: "-- Inspect current state\nDESCRIBE NAVIGATION Responsive;\n-- Copy output, modify, paste back as CREATE OR REPLACE",
		SeeAlso: []string{"navigation.create", "navigation.show"},
	})

	// ── Settings ────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "settings",
		Summary: "Project settings — model, configuration, constants, language, workflows",
		Keywords: []string{
			"settings", "project settings", "configuration",
			"startup", "shutdown", "hash algorithm", "java version",
		},
		Syntax:  "SHOW SETTINGS;\nDESCRIBE SETTINGS;\nDESCRIBE SETTINGS CONFIGURATION '<name>';   -- just one configuration\nALTER SETTINGS MODEL <key> = <value>;\nALTER SETTINGS CONFIGURATION '<name>' <key> = <value>;",
		Example: "SHOW SETTINGS;\nALTER SETTINGS MODEL AfterStartupMicroflow = 'Module.MF_Startup';",
		SeeAlso: []string{"settings.show", "settings.alter"},
	})

	Register(SyntaxFeature{
		Path:    "translations",
		Summary: "Bulk translation of every user-visible string, one file per language",
		Keywords: []string{
			"translations", "translate", "language", "languages", "i18n",
			"localisation", "localization", "multilingual", "nl_NL", "de_DE",
		},
		Syntax: `DESCRIBE TRANSLATIONS [IN <Module>] FOR <lang>;
CREATE [OR MODIFY|REPLACE] TRANSLATIONS [IN <Module>] FOR <lang> (
    '<source>' AS '<translation>',
    ...
);

Entries use AS, not a colon: a translation maps a user-provided name to
another name, the same shape CUSTOM NAME map uses.

The thing that exists is the LANGUAGE, so the three verbs read directly:

  CREATE             the language has none yet — errors if it does
  CREATE OR MODIFY   merge; a source the file does not name is left alone
  CREATE OR REPLACE  the file is authoritative; a translation whose source
                     it does not name is REMOVED, and the run says which

IN <Module> scopes both directions. Under OR REPLACE it also BOUNDS the
deletion, so per-module files do not wipe each other on every run.

Keyed on the source string, so one entry translates every occurrence.
DESCRIBE emits the CREATE form, and an untranslated string comes back with
an empty target — which is what makes the output an LLM prompt:

  mxcli -p app.mpr -c "describe translations for de_DE" > de_DE.mdl
  # fill in the right-hand side
  mxcli exec de_DE.mdl -p app.mpr

A key that matches nothing is REPORTED, not skipped: a source edited after
the file was written stops matching, and the run names the string it has
probably become.`,
		Example: `describe translations for nl_NL;

create or modify translations in Administration for nl_NL (
    'Save'            as 'Opslaan',
    'My Account'      as 'Mijn account',
);`,
		SeeAlso: []string{"settings", "settings.alter"},
	})

	Register(SyntaxFeature{
		Path:    "settings.show",
		Summary: "Show and describe project settings",
		Keywords: []string{
			"show settings", "describe settings", "list settings",
		},
		Syntax:  "SHOW SETTINGS;\nDESCRIBE SETTINGS;\nDESCRIBE SETTINGS CONFIGURATION '<name>';",
		Example: "SHOW SETTINGS;\nDESCRIBE SETTINGS;\nDESCRIBE SETTINGS CONFIGURATION 'Default';",
	})

	Register(SyntaxFeature{
		Path:    "settings.alter",
		Summary: "Modify project settings — model, configuration, constants, language, workflows",
		Keywords: []string{
			"alter settings", "modify settings", "change settings",
			"after startup", "before shutdown", "hash algorithm",
			"database type", "constant override", "language",
			"add language", "remove language", "enable language", "translations",
			"optimistic locking", "concurrency", "lost update",
		},
		Syntax: `ALTER SETTINGS MODEL <key> = <value>;
ALTER SETTINGS CONFIGURATION '<name>' <key> = <value>, ...;
ALTER SETTINGS CONSTANT '<qualifiedName>' VALUE '<value>' IN CONFIGURATION '<name>';
ALTER SETTINGS DROP CONSTANT '<qualifiedName>' IN CONFIGURATION '<name>';
ALTER SETTINGS LANGUAGE DefaultLanguageCode = '<code>';
ALTER SETTINGS LANGUAGE ADD '<code>' [(CheckCompleteness: true, CustomDateFormat: '<fmt>')];
ALTER SETTINGS LANGUAGE REMOVE '<code>';
ALTER SETTINGS WORKFLOWS UserEntity = '<qualifiedName>';
CREATE CONFIGURATION '<name>' [<key> = <value>, ...];
DROP CONFIGURATION '<name>';`,
		Example: `ALTER SETTINGS MODEL AfterStartupMicroflow = 'Module.MF_Startup';
ALTER SETTINGS MODEL HashAlgorithm = 'BCrypt';
ALTER SETTINGS MODEL EnableDataStorageOptimisticLocking = true;
ALTER SETTINGS CONFIGURATION 'Default'
  DatabaseType = 'PostgreSql',
  DatabaseUrl = 'localhost:5432',
  DatabaseName = 'mydb';
ALTER SETTINGS CONSTANT 'BusinessEvents.ServerUrl' VALUE 'kafka:9092'
  IN CONFIGURATION 'Default';
CREATE CONFIGURATION 'Production'
  DatabaseType = 'PostgreSql',
  HttpPortNumber = 8080;

-- LANGUAGE ADD/REMOVE change the ENABLED languages — the list under App
-- Settings > Languages, and the only languages a build emits anything for. A
-- translation written for a language that is not enabled is stored, passes every
-- check, and is discarded at build time.
ALTER SETTINGS LANGUAGE ADD 'de_DE';
ALTER SETTINGS LANGUAGE ADD 'ar_SD' (CheckCompleteness: true);
ALTER SETTINGS LANGUAGE REMOVE 'de_DE';

-- A language is identified by its CODE alone: Studio Pro's "Arabic, Sudan" is
-- derived from ar_SD for display and is not stored in the model.
-- Two refusals: the DEFAULT language cannot be removed (every missing
-- translation falls back on it — change DefaultLanguageCode first), and a
-- language that still carries translations is refused with the count, because
-- removing it would strip work the statement does not name. Say it on purpose
-- with: create or replace translations for <code> ( );

-- DatabaseType must be a Mendix database type:
--   Db2, Hsqldb, MySql, Oracle, PostgreSql, SapHana, SqlServer
-- (matched case-insensitively and stored in the spelling above).

-- MODEL accepts these keys (an unknown one is refused and lists them):
--   AfterStartupMicroflow, BeforeShutdownMicroflow, HealthCheckMicroflow,
--   HashAlgorithm, BcryptCost, JavaVersion, RoundingMode,
--   AllowUserMultipleSessions, ScheduledEventTimeZoneCode, DefaultTimeZoneCode,
--   FirstDayOfWeek, DecimalScale, EnableDataStorageOptimisticLocking,
--   UseDatabaseForeignKeyConstraints, UseOQLVersion2, SslCertificateAlgorithm
--
-- EnableDataStorageOptimisticLocking is Studio Pro's App Settings → Runtime →
-- "Optimistic locking": it makes a stale commit fail instead of silently
-- overwriting, which is the fix for a read-then-write race in a microflow.
-- FirstDayOfWeek is Default or Monday..Sunday; SslCertificateAlgorithm is
-- PKIX or SunX509. Both are matched case-insensitively.
--
-- Which of these a project stores depends on its Mendix version (a blank 9.24
-- project has 12, a blank 11.13 has 17). An ALTER naming one the project does
-- not store is refused rather than introducing it, and DESCRIBE SETTINGS emits
-- only the stored ones so its output always replays.`,
		SeeAlso: []string{"settings.show"},
	})

	// ── Queues ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "queue",
		Summary: "Task queues — bound concurrency for queued microflow calls",
		Keywords: []string{
			"queue", "queues", "task queue", "create queue", "drop queue",
			"describe queue", "show queues", "parallelism", "cluster wide",
			"background", "async microflow",
		},
		Syntax: `CREATE [OR MODIFY] QUEUE Module.Name [FOLDER 'path'] [( <property>: <value>, ... )];
SHOW QUEUES [IN <module>];
LIST QUEUES [IN <module>];
DESCRIBE QUEUE Module.Name;
DROP QUEUE Module.Name;

Properties:
  Parallelism   how many tasks run at once. This is an EXPRESSION, not a
                number — Mendix stores it as a string. A bare integer is the
                common case; quote anything else. Defaults to 1.
  ClusterWide   true = the limit applies across the cluster, false (default)
                = per runtime instance.
  Documentation free text.

Bind a call to a queue with the IN QUEUE clause on CALL MICROFLOW or
CALL JAVA ACTION (see: mxcli syntax microflow.call):

  CALL MICROFLOW Ops.ACT_Process(Order = $Order) IN QUEUE Ops.OrderProcessing;

A rewrite that does NOT restate a stored binding is refused, because it would
drop it silently. A retry policy on a queued call has no MDL spelling and is
also refused rather than reset — change those in Studio Pro.`,
		Example: `CREATE QUEUE Ops.OrderProcessing (
  Parallelism: 3,
  ClusterWide: true
);

-- Defaults: parallelism 1, per-instance.
CREATE QUEUE Ops.Mail;

-- An expression is legal wherever a number is.
CREATE OR MODIFY QUEUE Ops.OrderProcessing (
  Parallelism: '$MyModule.Workers',
  ClusterWide: true
);

-- Bind a call to it. A queued Java action must return Nothing (CE7038).
CREATE OR MODIFY MICROFLOW Ops.ACT_Enqueue ()
BEGIN
  CALL MICROFLOW Ops.ACT_Process(Order = $Order) IN QUEUE Ops.OrderProcessing;
END;

SHOW QUEUES IN Ops;
DESCRIBE QUEUE Ops.OrderProcessing;
DROP QUEUE Ops.Mail;`,
	})

	// ── Regular expressions ─────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "regular-expression",
		Summary: "Regular expressions — named patterns shared by attribute validation rules",
		Keywords: []string{
			"regular expression", "regular expressions", "regex", "pattern", "validation",
			"create regular expression", "drop regular expression", "describe regular expression",
			"show regular expressions", "email regex", "match",
		},
		Syntax: `CREATE [OR MODIFY] REGULAR EXPRESSION Module.Name [FOLDER 'path'] (
  Expression: '<pattern>',
  [Documentation: '<text>',]
  [ExportLevel: Hidden|Public,]
);

SHOW REGULAR EXPRESSIONS [IN <module>];
LIST REGULAR EXPRESSIONS [IN <module>];
DESCRIBE REGULAR EXPRESSION Module.Name;
DROP REGULAR EXPRESSION Module.Name;

A regular expression is a DOCUMENT, not a string on a rule: Mendix stores an
attribute validation rule's reference to it by qualified name, so one pattern is
shared by every attribute that validates against it.

Quoting: the pattern is a normal MDL string, so a single quote inside it is
doubled ('^it''s$'). Backslashes are NOT escape characters — write the regex
exactly as Mendix should see it.

Mendix validates with .NET's regex engine, which accepts constructs Go does not
(lookaround, backreferences). mxcli stores such a pattern unchanged and DESCRIBE
notes that it could not verify it — it does not call it invalid.

Bind a pattern to an attribute with CREATE VALIDATION RULE — see
'mxcli syntax validation-rule'.`,
		Example: `CREATE REGULAR EXPRESSION Val.EmailAddress (
  Expression: '\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+',
  Documentation: 'A, not too restrictive, email address regular expression'
);

CREATE REGULAR EXPRESSION Val.Identifier (
  Expression: '^[a-zA-Z_]+[a-zA-Z0-9_]*$'
);

-- .NET lookbehind: legal in Mendix, not verifiable by mxcli
CREATE REGULAR EXPRESSION Val.NoTrailingSlash ( Expression: '.*(?<!/)$' );

SHOW REGULAR EXPRESSIONS IN Val;
DESCRIBE REGULAR EXPRESSION Val.EmailAddress;
DROP REGULAR EXPRESSION Val.Identifier;

-- Which entities validate against a shared pattern
SHOW REFERENCES TO Val.EmailAddress;`,
	})

	// ── Validation rules ────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "validation-rule",
		Summary: "Validation rules — constrain an attribute with a pattern or a range",
		Keywords: []string{
			"validation rule", "validation rules", "validate", "constraint",
			"create validation rule", "regex rule", "range rule",
			"required", "unique", "not null", "feedback",
		},
		Syntax: `CREATE VALIDATION RULE FOR Module.Entity.Attribute
  REGEX Module.PatternName
  FEEDBACK '<message>';

CREATE VALIDATION RULE FOR Module.Entity.Attribute
  RANGE FROM <literal> TO <literal>
  FEEDBACK '<message>';

The bounds are inclusive and either may be omitted:
  RANGE FROM 1 TO 100   between 1 and 100
  RANGE FROM 1          1 or more
  RANGE TO 100          100 or less
Mendix has no strict < or >, so there is no exclusive form.

A validation rule is anonymous and lives on the ENTITY, keyed by the attribute
it constrains — so the statement names the attribute, not the rule. Re-running
it replaces the rule of the SAME type on that attribute and leaves the others
alone, so an attribute can carry a Required and a RegEx rule at once.

REGEX takes the qualified name of a REGULAR EXPRESSION document, never an inline
pattern — create the pattern first. A name that does not resolve is refused
here, because Mendix stores the reference by name and would otherwise report
CE0135 "No regular expression specified" at build time.

REQUIRED and UNIQUE rules are written as attribute constraints instead, on
CREATE ENTITY or ALTER ENTITY:
  ALTER ENTITY Shop.Product MODIFY ATTRIBUTE Email string(200)
    NOT NULL ERROR 'Email is required';
  ALTER ENTITY Shop.Product MODIFY ATTRIBUTE Code string(20)
    UNIQUE ERROR 'Code must be unique';`,
		Example: `CREATE REGULAR EXPRESSION Shop.EmailPattern (
  Expression: '^[^@\s]+@[^@\s]+\.[^@\s]+$'
);

CREATE VALIDATION RULE FOR Shop.Customer.Email
  REGEX Shop.EmailPattern
  FEEDBACK 'Enter a valid email address';

CREATE VALIDATION RULE FOR Shop.Booking.Guests
  RANGE FROM 1 TO 100
  FEEDBACK 'Between 1 and 100 guests are allowed';

CREATE VALIDATION RULE FOR Shop.Product.Price
  RANGE FROM 0
  FEEDBACK 'Price cannot be negative';`,
	})

	// ── Scheduled events ────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "scheduled-event",
		Summary: "Scheduled events — Mendix's cron: run a microflow on a repeating schedule",
		Keywords: []string{
			"scheduled event", "scheduled events", "schedule", "cron", "recurring",
			"create scheduled event", "drop scheduled event", "describe scheduled event",
			"repeat", "daily", "hourly", "weekly", "monthly", "yearly", "timer", "batch job",
		},
		Syntax: `CREATE [OR MODIFY] SCHEDULED EVENT Module.Name [FOLDER 'path'] ( <property>: <value>, ... );
SHOW SCHEDULED EVENTS [IN <module>];
LIST SCHEDULED EVENTS [IN <module>];
DESCRIBE SCHEDULED EVENT Module.Name;
DROP SCHEDULED EVENT Module.Name;

Always required:
  Microflow     the microflow to run, as a qualified name
  Repeat        which schedule to use (below)

Each Repeat takes ONLY its own fields; anything else is refused:
  Minutely          Multiplier
  Hourly            Multiplier, MinuteOffset
  Daily             HourOfDay, MinuteOfHour
  Weekly            Weekdays, HourOfDay, MinuteOfHour
  MonthlyByDate     Multiplier, MonthOffset, DayOfMonth, HourOfDay, MinuteOfHour
  MonthlyByWeekday  Multiplier, MonthOffset, DaySelector, Weekday, HourOfDay, MinuteOfHour
  YearlyByDate      Month, DayOfMonth, HourOfDay, MinuteOfHour
  YearlyByWeekday   Month, DaySelector, Weekday, HourOfDay, MinuteOfHour

  Weekdays is a quoted list: 'Monday, Friday'. DaySelector is First, Second,
  Third, Fourth or Last. Weekday is Sunday..Saturday. Month and DayOfMonth are
  numbers (1-12, 1-31). MonthOffset picks which month of a multi-month cycle
  fires (0-based).

Optional on any repeat:
  Enabled       true or false (default false)
  OnOverlap     DelayNext (default) or SkipNext — what happens when a run is
                still going when the next one is due. This is a scheduled
                event's own concurrency control; it does not use a task queue.
  TimeZone      UTC (default) or Server
  StartDateTime an RFC 3339 timestamp; the event does not run before it
  Documentation free text`,
		Example: `CREATE SCHEDULED EVENT Ops.NightlyCleanup (
  Microflow: Ops.SE_Cleanup,
  Repeat: Daily,
  HourOfDay: 4,
  MinuteOfHour: 0,
  TimeZone: Server,
  Enabled: true
);

CREATE SCHEDULED EVENT Ops.HourlyPing (
  Microflow: Ops.SE_Ping,
  Repeat: Hourly,
  Multiplier: 2,
  MinuteOffset: 23
);

CREATE SCHEDULED EVENT Ops.WeeklyReport (
  Microflow: Ops.SE_Report,
  Repeat: Weekly,
  Weekdays: 'Monday, Friday',
  HourOfDay: 9,
  MinuteOfHour: 30
);

CREATE SCHEDULED EVENT Ops.QuarterEnd (
  Microflow: Ops.SE_Close,
  Repeat: MonthlyByWeekday,
  Multiplier: 3,
  MonthOffset: 2,
  DaySelector: Last,
  Weekday: Friday,
  HourOfDay: 18
);

SHOW SCHEDULED EVENTS IN Ops;
DESCRIBE SCHEDULED EVENT Ops.NightlyCleanup;
DROP SCHEDULED EVENT Ops.HourlyPing;`,
		SeeAlso: []string{"queue"},
	})

	// ── Structure ───────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "structure",
		Summary: "SHOW STRUCTURE — compact project overview at configurable depth",
		Keywords: []string{
			"structure", "show structure", "project overview",
			"repo map", "module summary", "depth",
		},
		Syntax: "SHOW STRUCTURE [DEPTH 1|2|3] [IN <module>] [ALL];",
		Example: `-- Module counts only
SHOW STRUCTURE DEPTH 1;

-- Elements with signatures (default)
SHOW STRUCTURE;

-- Full types and parameter names
SHOW STRUCTURE DEPTH 3;

-- Focus on one module
SHOW STRUCTURE IN MyModule;

-- Include system modules
SHOW STRUCTURE DEPTH 1 ALL;`,
	})

	// ── Move ────────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "move",
		Summary: "MOVE command — relocate documents between folders and modules",
		Keywords: []string{
			"move", "relocate", "folder", "cross-module move",
			"move page", "move microflow", "move entity",
			"move folder", "drop folder",
			"move import mapping", "move export mapping", "move json structure",
			"move queue", "move workflow", "move menu", "move layout",
		},
		Syntax: `MOVE <doctype> Module.Name TO FOLDER 'Path';
-- doctype: every top-level document, spelled as DESCRIBE spells it —
--   PAGE | SNIPPET | BUILDING BLOCK | LAYOUT | MENU
--   MICROFLOW | NANOFLOW | WORKFLOW | QUEUE | SCHEDULED EVENT
--   ENUMERATION | CONSTANT | REGULAR EXPRESSION
--   JSON STRUCTURE | IMPORT MAPPING | EXPORT MAPPING
--   JAVA ACTION | JAVASCRIPT ACTION | DATABASE CONNECTION | DATA TRANSFORMER
--   IMAGE COLLECTION | ICON COLLECTION
--   REST CLIENT | PUBLISHED REST SERVICE | ODATA CLIENT | ODATA SERVICE
--   BUSINESS EVENT SERVICE
--   MODEL | AGENT | KNOWLEDGE BASE | CONSUMED MCP SERVICE
-- plus ENTITY (moves between domain models) and FOLDER (moves a folder).
MOVE <doctype> Module.Name TO TargetModule;
MOVE <doctype> OldModule.Name TO FOLDER 'Path' IN NewModule;
MOVE FOLDER Module.FolderName TO FOLDER 'Path';
DROP FOLDER 'Path' IN Module;`,
		Example: `-- Move page to a folder
MOVE PAGE MyModule.CustomerEdit TO FOLDER 'Customers';

-- Move microflow to nested folder
MOVE MICROFLOW MyModule.ACT_ProcessOrder TO FOLDER 'Orders/Processing';

-- Move entity to different module
MOVE ENTITY OldModule.Customer TO NewModule;

-- Many doctypes have no folder clause on CREATE, so MOVE is the only way to
-- place them
MOVE JAVA ACTION MyModule.ODataQuery TO FOLDER 'Support';
MOVE ODATA SERVICE MyModule.PublicApi TO FOLDER 'Api/Published';
MOVE IMPORT MAPPING MyModule.IMM_Order TO FOLDER 'Private/Import mappings';
MOVE JSON STRUCTURE MyModule.JSON_Order TO FOLDER 'Private/JSON structures';

-- A FOLDER clause on CREATE OR MODIFY moves an existing document too, so a
-- script can place a document without a separate MOVE. Every doctype accepts
-- one; on most it goes straight after the qualified name
CREATE OR MODIFY JSON STRUCTURE MyModule.JSON_Order
  FOLDER 'Private/JSON structures'
  SNIPPET '{"id": 1}';
CREATE QUEUE MyModule.Q_Orders FOLDER 'Private/Queues' ( Parallelism: 3 );
CREATE IMPORT MAPPING MyModule.IMM_Order FOLDER 'Private/Import mappings'
  WITH JSON STRUCTURE MyModule.JSON_Order { CREATE MyModule.Order { Id = id } };

-- Check impact before cross-module move
SHOW IMPACT OF OldModule.CustomerPage;
MOVE PAGE OldModule.CustomerPage TO NewModule;

-- Drop empty folder
DROP FOLDER 'OldFolder' IN Module;

-- Read the placement back
LIST FOLDERS IN MyModule;`,
		SeeAlso: []string{"folders"},
	})

	// ── Folders ─────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "folders",
		Summary: "LIST FOLDERS — the folder layout of a module, with what is in each folder",
		Keywords: []string{
			"folders", "list folders", "show folders", "layout",
			"folder tree", "where is this document", "unfiled",
		},
		Syntax: "LIST FOLDERS [IN <module>];",
		Example: `-- Layout of one module
LIST FOLDERS IN MyModule;

-- Every module in the project
LIST FOLDERS;

-- As rows, to diff against an intended layout
mxcli -p app.mpr --json -c "LIST FOLDERS IN MyModule"

-- Complements MOVE: MOVE places a document in a folder, LIST FOLDERS reads
-- the placement back. SHOW STRUCTURE is organised by document type at every
-- depth, so it never shows which folder a document sits in.
--
-- Empty folders are listed too (with [0]), and documents still at the module
-- root appear under "(module root)" — so the output is the whole layout and
-- can be diffed against an intended one.`,
		SeeAlso: []string{"move", "structure"},
	})

	// ── Search ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "search",
		Summary: "Full-text search across project strings and source definitions",
		Keywords: []string{
			"search", "full-text search", "find", "grep",
			"fts", "catalog strings", "catalog source",
		},
		Syntax: `SEARCH '<query>';

-- CLI
mxcli search -p app.mpr "<query>" [--format table|names|json] [-q]

-- Raw FTS queries
SELECT * FROM CATALOG.STRINGS WHERE strings MATCH '<query>';
SELECT * FROM CATALOG.SOURCE WHERE source MATCH '<query>';`,
		Example: `SEARCH 'validation';
SEARCH 'Customer';

-- CLI with piping
mxcli search -p app.mpr "validation" -q --format names

-- FTS5 operators
SEARCH 'word1 OR word2';
SEARCH '"exact phrase"';
SEARCH 'word*';`,
	})

	// ── Testing ────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "test",
		Summary: "Microflow testing — run .test.mdl or .test.md files against a Mendix project (local warm loop, or Docker)",
		Keywords: []string{
			"test", "testing", "microflow test", "nanoflow test",
			"test.mdl", "test.md", "junit", "docker",
			"@test", "@expect", "@throws", "@cleanup",
			"watch", "attach", "test endpoint", "warm",
		},
		Syntax: `mxcli test <file|dir> -p app.mpr [flags]

Flags:
  -l, --list          List tests without executing
  -j, --junit FILE    Write JUnit XML results
  -s, --skip-build    Skip the build (reuse existing deployment)
      --local         Run on mxcli's own runtime — no Docker daemon needed
  -w, --watch         With --local: keep the runtime warm and re-run on
                      every test or model change (Ctrl-C to stop)
      --attach        Run against an app already started with
                      'mxcli run --local --test-endpoint' — no boot at all
      --skip-app-startup
                      With --local, do not run the project's own
                      after-startup microflow (it runs by default)
      --legacy-runner With --local: use the old after-startup runner
      --require-assertions
                      Report a test that asserts nothing as an ERROR
  -v, --verbose       Show runtime log lines
  -t, --timeout DUR   Runtime startup timeout (default: 5m)

Annotations:
  @test <name>              Test name (required)
  @expect <condition>       A Mendix expression that must evaluate to true.
                            Any expression the engine accepts works:
                              $result = 'John Doe'
                              $product/Name != 'Widget'   (<> is accepted too)
                              length($result) = 81
                              find($result, '0') >= 0
                              substring($r, 0, 9) = substring($r, 9, 18)
                              find($r, '0') >= 0 and $count > 3
                            An assertion the runner cannot compile — unknown
                            function, wrong arity, or an expression that
                            yields a value rather than a condition — is an
                            ERROR against that test, never a pass. A failure
                            reports the observed value alongside the
                            expectation whenever the assertion pins its type.
  @throws 'message'         Expect error
  @verify <oql> <op> <lit>  Assert on the DATABASE after the microflow ran:
                              @verify select count(*) as n from Mod.Cell = 81
                              @verify select count(*) as n from Mod.Cell > 0
                            The query must return one row and one column, and
                            the test needs @cleanup none — rollback would undo
                            the writes before the query could see them. An
                            unevaluatable @verify is an ERROR, never a pass.
                            --local / --attach only.
  @cleanup rollback|none    What happens to the test's database writes.
                            rollback (the default) wraps the test in a
                            transaction and rolls it back, so nothing it
                            wrote survives — including when it throws.
                            none lets the writes commit. --local only:
                            the Docker path always commits. An unknown
                            value is a parse error, not a silent commit.

How --local runs tests: one microflow per test, invoked by name over a
token-guarded HTTP endpoint the app registers at boot. A test that throws
fails only itself, and results are returned rather than scraped from the log.
Docker still uses the older after-startup runner.

Boot also runs the project's own after-startup microflow, chained after the
endpoint registration, so tests see the app in the state it really boots into
and a suite behaves the same under --local and --attach.

Cost of a run:
  cold (--local)            ~30s   boots a runtime on its own ports + DB
  warm (--local --watch)    ~2s    runtime stays up between runs
  attached (--attach)       ~2s    no boot; uses the running app's database`,
		Example: `-- .test.mdl file format
/**
 * @test String concatenation
 * @expect $result = 'John Doe'
 * @expect length($result) = 8
 */
$result = CALL MICROFLOW MyModule.ConcatNames(
  FirstName = 'John', LastName = 'Doe'
);
/

-- Run tests
mxcli test tests/ -p app.mpr                      -- Docker
mxcli test tests/ -p app.mpr --local              -- no Docker daemon
mxcli test tests/ -p app.mpr --local --watch      -- warm loop, re-runs on change
mxcli test tests/ -p app.mpr --junit results.xml

-- Or attach to an app you already have running:
mxcli run  --local --test-endpoint -p app.mpr     -- terminal 1
mxcli test tests/ -p app.mpr --attach             -- terminal 2`,
	})

	// ── Errors ──────────────────────────────────────────────────────────

	Register(SyntaxFeature{
		Path:    "errors",
		Summary: "Common validation errors and how to fix them",
		Keywords: []string{
			"errors", "validation", "syntax error", "reference error",
			"reserved keyword", "module not found", "entity not found",
			"check", "troubleshooting",
		},
		Syntax: `mxcli check script.mdl                    -- Syntax + anti-pattern check
mxcli check script.mdl -p app.mpr --references  -- With reference validation`,
		Example: `-- Reserved keyword as identifier
-- Error:  mismatched input 'Title' expecting IDENTIFIER
-- Fix:    Use quoted identifiers: "Title"

-- Module not found
-- Error:  module not found: ModuleName
-- Fix:    CREATE MODULE ModuleName;

-- Missing module prefix on enumeration
-- Error:  enumeration reference 'X' is missing module prefix
-- Fix:    Use Enumeration(MyModule.Status)

-- Invalid association path in OQL (dot instead of slash)
-- Wrong:  WHERE l.Library.Loan_Member = m.ID
-- Right:  WHERE l/Library.Loan_Member = m.ID`,
		SeeAlso: []string{"errors.syntax", "errors.reference", "errors.execution"},
	})

	Register(SyntaxFeature{
		Path:    "errors.syntax",
		Summary: "Syntax errors — reserved keywords, invalid types, malformed enumerations",
		Keywords: []string{
			"syntax error", "reserved keyword", "invalid type",
			"malformed enumeration", "parse error", "mismatched input",
		},
		Syntax: "mxcli check script.mdl",
		Example: `-- Reserved keyword used as identifier
-- Error:  mismatched input 'Title' expecting IDENTIFIER
-- Fix:    Use quoted identifiers: "Title", "ComboBox"."Entity"
-- Alt:    Rename to avoid keyword: BookTitle, OrderStatus

-- Invalid data type
-- Error:  Unknown type parsed as enumeration reference
-- Fix:    Use correct type: DateTime (not DateAndTime)

-- Malformed enumeration
-- Error:  Invalid enumeration value: each value must have a name
-- Fix:    Use syntax: ValueName 'Caption'`,
	})

	Register(SyntaxFeature{
		Path:    "errors.reference",
		Summary: "Reference errors — missing modules, entities, enumerations",
		Keywords: []string{
			"reference error", "module not found", "entity not found",
			"enumeration not found", "missing module prefix",
		},
		Syntax: "mxcli check script.mdl -p app.mpr --references",
		Example: `-- Module not found
-- Error:  module not found: ModuleName
-- Fix:    CREATE MODULE ModuleName;

-- Enumeration not found
-- Error:  attribute 'X': enumeration not found: Module.EnumName
-- Fix:    Create the enumeration first, or check spelling

-- Missing module prefix on enumeration
-- Error:  enumeration reference 'X' is missing module prefix
-- Fix:    Use fully qualified name: Enumeration(MyModule.Status)`,
	})

	Register(SyntaxFeature{
		Path:    "module.jar-dependencies",
		Summary: "Manage Maven/JAR dependencies in a module's settings",
		Keywords: []string{
			"jar dependency", "maven", "jar dep", "module settings",
			"group", "artifact", "classpath", "exclusion",
		},
		Syntax: `LIST JAR DEPENDENCIES [IN <module>];
DESCRIBE JAR DEPENDENCY <module> '<group:artifact>';
ALTER MODULE <name>
  ADD JAR DEPENDENCY (
    group    = '<group>',
    artifact = '<artifact>',
    version  = '<version>',
    included = true|false,
  );
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' VERSION '<version>';
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' INCLUDED true|false;
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' ADD EXCLUSION '<group:artifact>';
ALTER MODULE <name> SET JAR DEPENDENCY '<group:artifact>' DROP EXCLUSION '<group:artifact>';
ALTER MODULE <name> DROP JAR DEPENDENCY '<group:artifact>';`,
		Example: `-- Add a new JAR dependency to a module
ALTER MODULE MyModule
  ADD JAR DEPENDENCY (
    group    = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version  = '1.1.3',
    included = true,
  );

-- Update the version
ALTER MODULE MyModule SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' VERSION '1.2.0';

-- Exclude a transitive dependency
ALTER MODULE MyModule SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' ADD EXCLUSION 'com.example:unwanted';

-- List all jar dependencies
LIST JAR DEPENDENCIES;
LIST JAR DEPENDENCIES IN MyModule;

-- Describe (outputs roundtrippable MDL)
DESCRIBE JAR DEPENDENCY MyModule 'org.duckdb:duckdb_jdbc';

-- Remove a dependency
ALTER MODULE MyModule DROP JAR DEPENDENCY 'org.duckdb:duckdb_jdbc';`,
	})

	Register(SyntaxFeature{
		Path:    "errors.execution",
		Summary: "Execution errors — entity exists, type mismatches, validation failures",
		Keywords: []string{
			"execution error", "entity already exists", "type mismatch",
			"boolean default", "view entity", "microflow validation",
		},
		Syntax: "mxcli check script.mdl -p app.mpr --references",
		Example: `-- Entity already exists
-- Error:  entity already exists: Module.Entity
-- Fix:    Use CREATE OR MODIFY ENTITY to update existing entities

-- Boolean without default
-- Note:   Boolean attributes auto-default to false

-- OQL invalid association path (dot vs slash)
-- Wrong:  WHERE l.Library.Loan_Member = m.ID
-- Right:  WHERE l/Library.Loan_Member = m.ID`,
	})
}
