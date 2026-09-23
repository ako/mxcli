// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "domain-model",
		Summary: "Domain model: entities, attributes, associations, enumerations, constants",
		Keywords: []string{
			"domain model", "entity", "attribute", "association",
			"enumeration", "constant", "data model", "schema",
		},
		Syntax:  "CREATE PERSISTENT ENTITY Module.Name (...);\nCREATE ASSOCIATION Module.Name FROM ... TO ...;\nCREATE ENUMERATION Module.Name (...);\nCREATE CONSTANT Module.Name TYPE ... DEFAULT ...;",
		Example: "CREATE PERSISTENT ENTITY Shop.Customer (\n  Name: String(100) NOT NULL\n);",
		SeeAlso: []string{"domain-model.entity", "domain-model.association", "domain-model.enumeration", "domain-model.constant"},
	})

	// --- Entity ---

	Register(SyntaxFeature{
		Path:    "domain-model.annotation",
		Summary: "Canvas notes on a domain model — the boxes that explain the diagram",
		Keywords: []string{
			"annotation", "annotations", "note", "canvas", "section", "comment box",
			"create annotation", "drop annotation", "show annotations",
		},
		Syntax: "SHOW ANNOTATIONS [IN Module];\n\n" +
			"CREATE [OR MODIFY] ANNOTATION IN Module (\n" +
			"  Caption: 'text'  |  $$multi\nline$$,\n" +
			"  [Position: (x, y),]\n" +
			"  [Width: n]\n" +
			");\n\n" +
			"DROP ANNOTATION 'first line of the caption' IN Module;\n" +
			"DROP ANNOTATION AT (x, y) IN Module;\n\n" +
			"An annotation has no name. It is addressed by its POSITION when the\n" +
			"statement gives one — the identity to prefer, since it survives an edit to\n" +
			"the wording — and otherwise by the FIRST LINE of its caption. Give a note a\n" +
			"position if you intend to re-run the script; without one, rewording creates\n" +
			"a second note instead of updating the first.\n\n" +
			"An omitted Position or Width leaves the stored value alone. A new note gets\n" +
			"Studio Pro's defaults: (60, 240) and width 440.\n\n" +
			"THERE IS NO HEIGHT, AND NO COLOUR. DomainModels$Annotation stores exactly\n" +
			"Caption, ExportLevel, Location and Width. A note auto-sizes to its caption,\n" +
			"so height follows from the wrapping and WIDTH is the only lever: narrower\n" +
			"wraps to more lines and is TALLER, wider is SHORTER. To keep a note clear of\n" +
			"what sits below it, move those with ALTER ENTITY … SET POSITION (x, y).\n" +
			"Likewise a \"coloured section box\" is this element in Studio Pro's own\n" +
			"styling — the model has nowhere to keep a colour.",
		Example: "CREATE ANNOTATION IN Sales (\n" +
			"  Caption: $$Orders\nEverything about an order lives here.$$,\n" +
			"  Position: (60, 40),\n" +
			"  Width: 400\n" +
			");\n\n" +
			"-- Reword and re-run: the position identifies the note, so this updates it\n" +
			"CREATE OR MODIFY ANNOTATION IN Sales (\n" +
			"  Caption: 'Orders and invoices',\n" +
			"  Position: (60, 40)\n" +
			");\n\n" +
			"SHOW ANNOTATIONS IN Sales;\n" +
			"DROP ANNOTATION AT (60, 40) IN Sales;",
		SeeAlso: []string{"domain-model", "domain-model.entity"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity",
		Summary: "Entity creation: persistent, non-persistent, generalization, event handlers",
		Keywords: []string{
			"entity", "create entity", "persistent", "non-persistent",
			"generalization", "extends", "event handler", "attribute",
		},
		Syntax:  "CREATE PERSISTENT ENTITY Module.Name (\n  Attr: Type [constraints],\n  ...\n) [INDEX (attr1)];\n\n-- Documentation is the /** … */ doc comment before the statement.\n-- (A `COMMENT 'text'` option existed, set nothing, and has been removed.)\n\nCREATE NON-PERSISTENT ENTITY Module.Name (...);\n\nCREATE PERSISTENT ENTITY Module.Name EXTENDS Module.Parent (...);",
		Example: "/** Stores customer information. */\nCREATE PERSISTENT ENTITY MyModule.Customer (\n  Name: String(100) NOT NULL ERROR 'Name is required',\n  Email: String(200) UNIQUE,\n  Balance: Decimal DEFAULT 0,\n  IsActive: Boolean DEFAULT true,\n  Status: Enumeration(MyModule.CustomerType)\n)\nINDEX (Email);",
		SeeAlso: []string{"domain-model.entity.create", "domain-model.entity.alter", "domain-model.entity.attributes"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity.create",
		Summary: "CREATE ENTITY with all options: persistence, generalization, indexes, events",
		Keywords: []string{
			"create entity", "new entity", "persistent entity",
			"non-persistent", "extends", "generalization",
			"index", "event handler", "before commit", "after commit",
		},
		Syntax:  "CREATE PERSISTENT ENTITY Module.Name (\n  Attr: Type [NOT NULL [ERROR 'msg']] [UNIQUE [ERROR 'msg']] [DEFAULT val],\n  ...\n)\n[INDEX (attr1, attr2)]\n[ON BEFORE|AFTER CREATE|COMMIT|DELETE|ROLLBACK CALL Module.MF [RAISE ERROR]];\n\n-- Documentation: the /** … */ doc comment before the statement, or\n-- ALTER ENTITY Module.Name SET COMMENT 'text' on an existing entity.\n\nCREATE NON-PERSISTENT ENTITY Module.Name (...);\nCREATE PERSISTENT ENTITY Module.Name EXTENDS Module.Parent (...);\n\n-- INDEX goes AFTER the closing parenthesis, never inside the attribute list.\n-- On an existing entity, either spelling works:\nALTER ENTITY Module.Name ADD INDEX [IF NOT EXISTS] [name] [ON] (attr1 [ASC|DESC], ...);\nCREATE INDEX IdxName ON Module.Name (attr1 [ASC|DESC], ...);\n\n-- Re-runnable script: IF NOT EXISTS skips instead of erroring, and leaves an\n-- existing element untouched (unlike OR MODIFY, which rebuilds it from the\n-- statement and drops any attribute the statement omits).\nCREATE ENTITY IF NOT EXISTS Module.Name (...);\nALTER ENTITY Module.Name ADD ATTRIBUTE IF NOT EXISTS Attr: Type;\nALTER ENTITY Module.Name DROP INDEX IF EXISTS (attr1, ...);",
		Example: "-- Persistent with constraints and index\nCREATE PERSISTENT ENTITY Shop.Order (\n  OrderNumber: String(20) NOT NULL,\n  Total: Decimal DEFAULT 0,\n  CreatedAt: DateTime\n)\nINDEX (OrderNumber)\nON BEFORE COMMIT CALL Shop.ValidateOrder($currentObject) RAISE ERROR;\n\n-- With generalization\nCREATE PERSISTENT ENTITY Shop.ProductImage EXTENDS System.Image (\n  Caption: String(200)\n);",
		SeeAlso: []string{"domain-model.entity.alter", "domain-model.entity.attributes"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity.alter",
		Summary: "ALTER ENTITY: add/rename/modify/drop attributes, indexes, documentation, event handlers; ALTER ENTITIES for the bulk add",
		Keywords: []string{
			"alter entity", "modify entity", "add attribute",
			"drop attribute", "rename attribute", "add index",
			"event handler", "documentation",
			"if not exists", "if exists", "idempotent",
			"alter entities", "bulk", "every entity", "all entities", "where persistent",
		},
		Syntax:  "ALTER ENTITY Module.Name ADD ATTRIBUTE [IF NOT EXISTS] AttrName: Type [constraints];\nALTER ENTITY Module.Name DROP ATTRIBUTE [IF EXISTS] AttrName;\nALTER ENTITY Module.Name RENAME ATTRIBUTE OldName TO NewName;\nALTER ENTITY Module.Name MODIFY ATTRIBUTE AttrName Type [DEFAULT val];\nALTER ENTITY Module.Name DROP DEFAULT ON ATTRIBUTE AttrName;\nALTER ENTITY Module.Name ADD INDEX [name] [ON] (attr1, attr2);\nALTER ENTITY Module.Name SET DOCUMENTATION 'text';\nALTER ENTITY Module.Name SET POSITION (x, y);\nALTER ENTITY Module.Name ADD EVENT HANDLER ON BEFORE COMMIT CALL Module.MF RAISE ERROR;\nALTER ENTITIES [IN Module] ADD ATTRIBUTE [IF NOT EXISTS] AttrName: Type [, ...]\n  [WHERE PERSISTENT | WHERE NON-PERSISTENT];\n\nALTER ENTITIES is the bulk form: one statement applied to every entity in a\nmodule instead of one statement per entity. Only ADD ATTRIBUTE is offered --\nDROP and RENAME aimed at a set are destructive by a typo, and SET POSITION on\nevery entity is meaningless. Pair it with IF NOT EXISTS so the script re-runs.\n\nWHERE filters by persistence, using the same words CREATE ENTITY uses. A VIEW\nentity matches NEITHER: its rows come from an OQL query, so it is not the\npersistent/non-persistent distinction this filter means.\n\nWITHOUT IN, the sweep covers the whole project but SKIPS System and every\nMarketplace module, reporting which -- an upgrade replaces those modules and\nwould take the attribute with it. Naming a module with IN is taken as meaning\nit, so a deliberate edit there is still possible.\n\nSET POSITION places the entity in the domain-model editor, and CREATE ENTITY\ntakes the same thing as an @Position(x, y) annotation. Both are the box's\nCENTRE, not its top-left corner. An entity created without one takes the next\nslot in a wrapping grid, which is a default rather than a layout: to arrange a\nwhole module from its association graph, run 'mxcli layout -p app.mpr'\n(--dry-run first; it replaces positions you set by hand).\n\nMODIFY ATTRIBUTE always takes a type — restate it even when you are only\nchanging the default. There is no 'MODIFY ATTRIBUTE X SET DEFAULT v' form:\nSET would be read as the type name. Use DROP DEFAULT to clear one.\n\nIF NOT EXISTS / IF EXISTS make the add/drop a no-op (skipped, not an error)\nwhen the attribute is already present / already gone — so a domain script\nre-runs cleanly. For a whole script, 'mxcli exec --continue-on-error' reports\neach failed statement and keeps going instead of halting at the first.\n\nRENAME ATTRIBUTE also rewrites every reference to the attribute: the stored\nqualified names (microflow create/change members, page widgets, the entity's own\nvalidation and access rules) AND the bare steps inside XPath constraints, which\nare resolved to their owning entity first so another entity's identically-named\nattribute is left alone. A constraint that cannot be resolved is reported and\nleft unchanged, never guessed at. Uses inside microflow expressions ($obj/Attr)\nare free text and are NOT rewritten; mxbuild reports those as CE0117.",
		Example: "ALTER ENTITY Shop.Customer ADD ATTRIBUTE Phone: String(20);\nALTER ENTITY Shop.Customer ADD ATTRIBUTE IF NOT EXISTS Phone: String(20);  -- re-runnable\nALTER ENTITY Shop.Customer DROP ATTRIBUTE IF EXISTS OldField;              -- re-runnable\nALTER ENTITY Shop.Customer RENAME ATTRIBUTE Email TO EmailAddress;\nALTER ENTITY Shop.Customer MODIFY ATTRIBUTE Phone String(30) DEFAULT '';  -- type restated\nALTER ENTITY Shop.Customer DROP DEFAULT ON ATTRIBUTE Phone;               -- clear a default\nALTER ENTITY Shop.Customer ADD INDEX ON (EmailAddress);\nALTER ENTITY Shop.Customer\n  ADD EVENT HANDLER ON BEFORE COMMIT CALL Shop.Validate($currentObject) RAISE ERROR;\n\n-- give every persistent entity in a module an audit trail, in one statement\nALTER ENTITIES IN Shop\n  ADD ATTRIBUTE IF NOT EXISTS CreatedDate: AutoCreatedDate,\n  ADD ATTRIBUTE IF NOT EXISTS ChangedDate: AutoChangedDate\n  WHERE PERSISTENT;",
		SeeAlso: []string{"domain-model.entity.create", "domain-model.entity.attributes"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity.show",
		Summary: "List and describe entities in the project",
		Keywords: []string{
			"show entities", "list entities", "describe entity",
			"show attributes", "entity details",
		},
		Syntax:  "SHOW ENTITIES;\nSHOW ENTITIES IN <module>;\nDESCRIBE ENTITY Module.Name;",
		Example: "SHOW ENTITIES IN Shop;\nDESCRIBE ENTITY Shop.Customer;",
		SeeAlso: []string{"domain-model.entity.create"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity.attributes",
		Summary: "Attribute data types, constraints, system attributes, and calculated attributes",
		Keywords: []string{
			"attribute", "data type", "string", "integer", "decimal",
			"boolean", "datetime", "autonumber", "binary", "hashedstring",
			"not null", "unique", "default", "calculated",
			"auto owner", "auto changed by", "system attribute",
		},
		Syntax:  "-- Data types\nString(n)  Integer  Long  Decimal  Boolean  DateTime  Date\nAutoNumber  Binary  HashedString  Enumeration(Module.Name)\n\n-- System attributes (auditing)\nAutoOwner  AutoChangedBy  AutoCreatedDate  AutoChangedDate\n\n-- Constraints\nNOT NULL [ERROR 'msg']  UNIQUE [ERROR 'msg']  DEFAULT value\nCALCULATED BY Module.Microflow",
		Example: "CREATE PERSISTENT ENTITY MyModule.AuditedEntity (\n  Name: String(100) NOT NULL,\n  Age: Integer DEFAULT 0,\n  Price: Decimal,\n  IsActive: Boolean DEFAULT true,\n  Status: Enumeration(MyModule.Status),\n  FullName: String(200) CALCULATED BY MyModule.CalcFullName,\n  Owner: AutoOwner,\n  ChangedBy: AutoChangedBy,\n  CreatedDate: AutoCreatedDate,\n  ChangedDate: AutoChangedDate\n);",
		SeeAlso: []string{"domain-model.entity.create", "domain-model.types"},
	})

	// --- View entity ---
	//
	// Filed under domain-model, not at the top level, because a view entity IS
	// a domain-model document: it sits on the canvas, declares attributes,
	// pages bind to it, and its associations are derived from its own OQL. The
	// top-level `oql` topic is a different thing entirely — running a query
	// against a live runtime — and a sibling `view-entity` beside it would
	// invite exactly that confusion.

	Register(SyntaxFeature{
		Path:    "domain-model.view-entity",
		Summary: "VIEW ENTITY — an entity whose rows come from an OQL query the database runs",
		Keywords: []string{
			"view entity", "view-entity", "oql view", "aggregation", "aggregate",
			"group by", "sum", "count", "report", "dashboard", "totals",
			"create view entity", "query performance", "read model",
		},
		MinVersion: "10.18.0",
		Syntax: "CREATE VIEW ENTITY [IF NOT EXISTS] Module.Name (\n" +
			"  Attr: Type,\n" +
			"  ...\n" +
			") AS ( <oql> );\n\n" +
			"REACH FOR THIS INSTEAD OF A MICROFLOW whenever the answer is a total, a\n" +
			"count per group, a figure on a dashboard, or anything joined across\n" +
			"entities. The database does the work and returns rows a page binds to\n" +
			"directly; the microflow version retrieves every object into memory to\n" +
			"produce one number, and gets slower exactly as the app succeeds.\n\n" +
			"A view entity is READ-ONLY: no create, change, delete or commit, and no\n" +
			"plain association to or from one (mxbuild: CE6771 — see\n" +
			"domain-model.view-entity.association for the form that works).\n\n" +
			"ALTER ENTITY does not apply. Re-run CREATE OR MODIFY VIEW ENTITY with the\n" +
			"whole definition: the attribute list and the OQL are one unit, and Mendix\n" +
			"rejects a model where they disagree (CE6770 \"View Entity is out of sync\n" +
			"with the OQL Query\").",
		Example: "create view entity Sales.RevenueByRegion (\n" +
			"  Region: String(100),\n" +
			"  OrderCount: Integer,\n" +
			"  Revenue: Decimal\n" +
			") as (\n" +
			"  select c.Region as Region, count(o.ID) as OrderCount, sum(o.Amount) as Revenue\n" +
			"  from Sales.Order as o\n" +
			"  join o/Sales.Order_Customer/Sales.Customer as c\n" +
			"  group by c.Region\n" +
			");",
		SeeAlso: []string{"domain-model.view-entity.oql", "domain-model.view-entity.association", "domain-model.entity", "oql"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.view-entity.oql",
		Summary: "The OQL inside a view entity — clause order, aliases, and the length rule",
		Keywords: []string{
			"oql", "view entity oql", "select", "from", "join", "group by",
			"order by", "limit", "union", "alias", "as alias", "cast",
			"MDL030", "MDL031", "MDL072", "CE0174", "CE6770",
		},
		MinVersion: "10.18.0",
		Syntax: "BOTH CLAUSE ORDERS ARE ACCEPTED — `select … from …` and Mendix's own\n" +
			"from-first `from … join … group by … select …`.\n\n" +
			"An association is walked with SLASHES, never dots:\n" +
			"  join o/Module.Order_Customer/Module.Customer as c\n\n" +
			"FOUR RULES THAT BITE, each caught by `mxcli check` before a build:\n\n" +
			"1. EVERY select column needs an `AS` alias, and the alias is the attribute\n" +
			"   name it fills (MDL030; mxbuild CE0174).\n" +
			"2. ORDER BY requires a LIMIT. Prefer NEITHER, so the page or microflow\n" +
			"   consuming the view sorts and pages as it needs; use `ORDER BY … LIMIT n`\n" +
			"   only for a view that is intrinsically top-N (MDL030; CE0174).\n" +
			"3. A DERIVED string column is String(200), always — a CAST to string, a\n" +
			"   string-returning CASE, any string expression. Declare it `String(200)`\n" +
			"   or mxbuild rejects the view with CE6770. Only a pass-through column\n" +
			"   inherits its source attribute's length (MDL031).\n" +
			"4. A SOURCE may be double-quoted like SQL (`s.\"Month\"`), an ALIAS may not\n" +
			"   (MDL072). So a view attribute can never be called `Month` or `Year` —\n" +
			"   that one is renamed, not quoted.\n\n" +
			"UNION / UNION ALL are supported and round-trip; column count and types must\n" +
			"line up across branches, and an ORDER BY applies to the whole result.",
		Example: "-- A reserved word as a SOURCE: quote it. The alias is renamed instead.\n" +
			"create view entity Sales.SalesByMonth (\n" +
			"  MonthNo: Integer,\n" +
			"  Total: Decimal\n" +
			") as (\n" +
			"  select s.\"Month\" as MonthNo, sum(s.Amount) as Total\n" +
			"  from Sales.Order as s\n" +
			"  group by s.\"Month\"\n" +
			");\n\n" +
			"-- An intrinsically top-N view: ORDER BY paired with LIMIT\n" +
			"create view entity Sales.TopCustomers (\n" +
			"  Name: String(100),\n" +
			"  Revenue: Decimal\n" +
			") as (\n" +
			"  select c.Name as Name, sum(o.Amount) as Revenue\n" +
			"  from Sales.Order as o\n" +
			"  join o/Sales.Order_Customer/Sales.Customer as c\n" +
			"  group by c.Name\n" +
			"  order by Revenue desc\n" +
			"  limit 100\n" +
			");",
		SeeAlso: []string{"domain-model.view-entity", "domain-model.view-entity.association", "oql"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.view-entity.association",
		Summary: "A view entity's associations are DERIVED from its OQL — select an id under an alias",
		Keywords: []string{
			"view entity association", "oql association", "select id as",
			"OqlViewAssociationSource", "CE6771", "CE6770", "MDL080",
		},
		MinVersion: "10.18.0",
		Syntax: "SELECTING A PERSISTENT ENTITY'S ID UNDER AN ALIAS CREATES AN ASSOCIATION,\n" +
			"named after the alias. There is no second statement, and the id column is\n" +
			"NOT one of the view entity's attributes:\n\n" +
			"  select m.ID as MeterRef, sum(r.Kwh) as TotalKwh\n" +
			"    -> association MeterRef, attribute TotalKwh\n\n" +
			"Three things follow, each of which is refused by `mxcli check` rather than\n" +
			"left to the build:\n\n" +
			"- DO NOT declare the alias in the attribute list. `MeterRef: Module.Meter`\n" +
			"  parses (a bare qualified name is how MDL spells an enumeration type) and\n" +
			"  used to be stored as one — mxbuild: CE1613 (MDL080).\n" +
			"- DO NOT write CREATE ASSOCIATION with a view entity at either end. Mendix\n" +
			"  refuses it outright: CE6771 \"It is not possible to create associations\n" +
			"  to/from View Entities.\"\n" +
			"- THE ALIAS IS A MODULE-LEVEL NAME. It cannot collide with an entity or\n" +
			"  enumeration in the same module, case-insensitively — Mendix: \"Duplicate\n" +
			"  name … Entities, associations and enumerations cannot share names.\"\n\n" +
			"The alternative is often better: CAST the id to a string and keep it as a\n" +
			"plain attribute. That is one SQL statement instead of two and materialises\n" +
			"no objects in the client, and the id is still there to look the real object\n" +
			"up with.",
		Example: "-- One attribute declared, TWO select columns: the id column is the association\n" +
			"create view entity Trends.MeterTotals (\n" +
			"  TotalKwh: Decimal\n" +
			") as (\n" +
			"  from Trends.Reading as r\n" +
			"  join r/Trends.Reading_Meter/Trends.Meter as m\n" +
			"  group by m.ID\n" +
			"  select m.ID as MeterRef, sum(r.Kwh) as TotalKwh\n" +
			");\n\n" +
			"-- The flat alternative: the id as a String(200) attribute, no association\n" +
			"create view entity Trends.MeterTotalsFlat (\n" +
			"  MeterId: String(200),\n" +
			"  TotalKwh: Decimal\n" +
			") as (\n" +
			"  from Trends.Reading as r\n" +
			"  join r/Trends.Reading_Meter/Trends.Meter as m\n" +
			"  group by m.ID\n" +
			"  select cast(m.ID as string) as MeterId, sum(r.Kwh) as TotalKwh\n" +
			");",
		SeeAlso: []string{"domain-model.view-entity", "domain-model.view-entity.oql", "domain-model.association"},
	})

	// --- Association ---

	Register(SyntaxFeature{
		Path:    "domain-model.association",
		Summary: "Associations: references between entities (many-to-one, many-to-many)",
		Keywords: []string{
			"association", "reference", "reference set",
			"many-to-one", "many-to-many", "foreign key",
			"owner", "delete behavior",
		},
		Syntax:  "[@anchor(from: (x, y), to: (x, y))]\nCREATE [OR MODIFY] ASSOCIATION Module.Name\n  FROM Module.FromEntity TO Module.ToEntity\n  TYPE Reference|ReferenceSet\n  [OWNER Default|Both]\n  [DELETE_BEHAVIOR behavior]\n  [COMMENT 'text'];\nALTER ASSOCIATION Module.Name SET ANCHOR FROM (x, y) TO (x, y);\nDROP ASSOCIATION Module.Name;\n\nOR MODIFY: updates type/owner/delete behavior in-place, preserves UUID.\n\n@anchor sets the LINE ANCHORS — where the connector attaches to each entity box\nin the domain model editor — as a PERCENTAGE of the box (0..100, whole numbers).\n`from` is the FROM entity's box, `to` the TO entity's: (0, 50) is the middle of\nthe left edge, (100, 50) the right, (50, 100) the bottom centre. Omitting an end\nPRESERVES what is stored, so a CREATE OR MODIFY about something else never\nflattens a hand-tuned line. Cross-module associations have no anchors.\n\nDROP reconciles the entity access rules that named the association, and is\nREFUSED while a message definition still exposes it (that one cannot be\nreconciled -- removing the member would change a published contract). The\nrefusal prints the `alter message definition ... drop member` statement for\neach definition, ready to run.",
		Example: "-- Many-to-one\nCREATE ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  TYPE Reference\n  OWNER Default\n  DELETE_BEHAVIOR DELETE_BUT_KEEP_REFERENCES;\n\n-- Many-to-many\nCREATE ASSOCIATION Shop.Product_Tag\n  FROM Shop.Product TO Shop.Tag\n  TYPE ReferenceSet\n  OWNER Both;\n\n-- Line leaving the bottom of Order and entering the top of Customer\n@anchor(from: (50, 100), to: (50, 0))\nCREATE ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer;\n\n-- Retune the line without restating the association\nALTER ASSOCIATION Shop.Order_Customer SET ANCHOR FROM (0, 54) TO (100, 54);",
		SeeAlso: []string{"domain-model.association.create", "domain-model.association.anchor", "domain-model.association.delete-behavior"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.association.create",
		Summary: "CREATE [OR MODIFY] ASSOCIATION with type, owner, storage, and delete behavior options",
		Keywords: []string{
			"create association", "or modify association", "new association", "reference",
			"reference set", "junction table", "foreign key",
			"owner default", "owner both", "storage column", "storage table",
		},
		Syntax:  "CREATE [OR MODIFY] ASSOCIATION Module.AssociationName\n  FROM Module.FromEntity TO Module.ToEntity\n  TYPE Reference|ReferenceSet\n  [OWNER Default|Both]\n  [STORAGE COLUMN|TABLE]\n  [DELETE_BEHAVIOR behavior]\n  [COMMENT 'text'];\n\nDirection:\n  FROM = entity holding the FK (the \"many\" side)\n  TO   = entity being referenced (the \"one\" side)\n\nTypes:\n  Reference    = Many-to-one (FK column on FROM table)\n  ReferenceSet = Many-to-many (junction table)\n\nOR MODIFY: updates in-place, preserves UUID. Safe to re-run.",
		Example: "-- Many-to-one with delete behavior\nCREATE ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  TYPE Reference\n  OWNER Default\n  DELETE_BEHAVIOR PREVENT;\n\n-- Many-to-many\nCREATE ASSOCIATION Shop.Product_Tag\n  FROM Shop.Product TO Shop.Tag\n  TYPE ReferenceSet\n  OWNER Both;\n\n-- Idempotent update\nCREATE OR MODIFY ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  TYPE Reference\n  OWNER Default\n  DELETE_BEHAVIOR CASCADE;",
		SeeAlso: []string{"domain-model.association.delete-behavior", "domain-model.entity.create"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.association.anchor",
		Summary: "Line anchors: where an association's connector attaches to each entity box",
		Keywords: []string{
			"anchor", "line anchor", "connection point", "connector",
			"diagram layout", "domain model layout", "set anchor",
		},
		Syntax: "@anchor(from: (x, y), to: (x, y))\nCREATE ASSOCIATION Module.Name FROM Module.From TO Module.To;\n\nALTER ASSOCIATION Module.Name SET ANCHOR FROM (x, y) TO (x, y);\n\n" +
			"x and y are a PERCENTAGE of the entity box, 0..100, whole numbers:\n" +
			"  (0, 50)    middle of the LEFT edge\n" +
			"  (100, 50)  middle of the RIGHT edge\n" +
			"  (50, 0)    TOP centre\n" +
			"  (50, 100)  BOTTOM centre\n\n" +
			"`from` is the anchor on the FROM entity's box, `to` on the TO entity's.\n" +
			"Any point on the box is valid — the pair is continuous, not four sides.\n\n" +
			"Omitting an end (or the whole annotation) PRESERVES what is stored, so a\n" +
			"CREATE OR MODIFY about the delete behaviour never flattens a hand-tuned\n" +
			"line. DESCRIBE ASSOCIATION re-emits a non-default pair as the same\n" +
			"@anchor(...), so describe -> edit -> exec round-trips.\n\n" +
			"A fractional coordinate is refused: Mendix stores two integers and its\n" +
			"loader will not OPEN a project whose anchor is fractional.\n" +
			"Cross-module associations have no anchors — Mendix stores none for them.",
		Example: "-- Line leaving the bottom of Order, entering the top of Customer\n@anchor(from: (50, 100), to: (50, 0))\nCREATE ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  TYPE Reference;\n\n-- Retune the line without restating the association\nALTER ASSOCIATION Shop.Order_Customer SET ANCHOR FROM (0, 54) TO (100, 54);\n\n-- Says nothing about anchors: whatever the line was dragged to survives\nCREATE OR MODIFY ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  DELETE_BEHAVIOR CASCADE;",
		SeeAlso: []string{"domain-model.association.create", "domain-model.entity"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.association.delete-behavior",
		Summary: "Delete behavior options for associations",
		Keywords: []string{
			"delete behavior", "cascade", "prevent", "restrict",
			"on delete", "set null", "error message", "referential action",
			"delete and references", "delete but keep references",
			"delete if no references", "referential integrity",
		},
		Syntax: "ON DELETE <action> [ERROR_MESSAGE '<text>']\n" +
			"  ON DELETE SET NULL   Keep the referencing objects, clear the reference (default)\n" +
			"  ON DELETE CASCADE    Delete the referencing objects too\n" +
			"  ON DELETE RESTRICT   Refuse the delete while references exist\n\n" +
			"These are SQL's referential actions, and Mendix's three delete behaviours are\n" +
			"exactly them. FROM/TO already matches a foreign key's direction -- FROM owns the\n" +
			"key, TO is referenced -- so `FROM Order TO Customer ON DELETE RESTRICT` means\n" +
			"deleting a CUSTOMER is refused while Orders reference it, the same way it would\n" +
			"in CREATE TABLE.\n\n" +
			"ERROR_MESSAGE is what the user sees when a RESTRICT delete is refused (Studio\n" +
			"Pro's \"Error message if 'X' object cannot be deleted\"). SQL has no equivalent;\n" +
			"this is a Mendix extension. Omitting it stores an empty message.\n\n" +
			"The older spelling still works and means the same thing:\n" +
			"  DELETE_BEHAVIOR DELETE_BUT_KEEP_REFERENCES | DELETE_AND_REFERENCES\n" +
			"                | DELETE_IF_NO_REFERENCES | CASCADE | PREVENT\n" +
			"                [ERROR_MESSAGE '<text>']\n" +
			"DESCRIBE emits the ON DELETE form, because it says which side is governed.",
		Example: "CREATE ASSOCIATION Shop.Order_Customer\n" +
			"  FROM Shop.Order TO Shop.Customer\n" +
			"  TYPE Reference\n" +
			"  ON DELETE RESTRICT\n" +
			"    ERROR_MESSAGE 'A customer with orders cannot be deleted';\n\n" +
			"CREATE ASSOCIATION Shop.Order_Lines\n" +
			"  FROM Shop.OrderLine TO Shop.Order\n" +
			"  TYPE Reference\n" +
			"  ON DELETE CASCADE;\n\n" +
			"ALTER ASSOCIATION Shop.Order_Customer SET ON DELETE SET NULL;",
		SeeAlso: []string{"domain-model.association.create"},
	})

	// --- Enumeration ---

	Register(SyntaxFeature{
		Path:    "domain-model.enumeration",
		Summary: "Enumerations: named sets of values for entity attributes",
		Keywords: []string{
			"enumeration", "enum", "create enumeration",
			"status", "type", "category", "values",
		},
		Syntax:  "CREATE ENUMERATION Module.Name (\n  Value1 'Caption 1',\n  Value2 'Caption 2',\n  ...\n);",
		Example: "CREATE ENUMERATION MyModule.OrderStatus (\n  Pending 'Pending Approval',\n  Processing 'Being Processed',\n  Shipped 'Shipped to Customer',\n  Delivered 'Delivered',\n  Cancelled 'Order Cancelled'\n);",
		SeeAlso: []string{"domain-model.enumeration.create", "domain-model.entity.attributes"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.enumeration.create",
		Summary: "CREATE ENUMERATION with values and captions, usage in entities",
		Keywords: []string{
			"create enumeration", "new enum", "enum values",
			"caption", "show enumerations", "describe enumeration",
			"drop enumeration",
		},
		Syntax:  "CREATE ENUMERATION Module.Name (\n  ValueName 'Display Caption',\n  ...\n);\n\nALTER ENUMERATION Module.Name ADD VALUE [IF NOT EXISTS] NewValue [CAPTION 'Display Caption'];\nALTER ENUMERATION Module.Name RENAME VALUE OldName TO NewName;\nALTER ENUMERATION Module.Name MODIFY VALUE ValueName CAPTION 'New Caption';\nALTER ENUMERATION Module.Name DROP VALUE [IF EXISTS] ValueName;\n\nIF NOT EXISTS / IF EXISTS make a script RE-RUNNABLE. Without them the second\nrun errors and exec STOPS THERE, so one already-present value leaves every\nlater statement unapplied. A defensive drop-then-add is not a substitute: the\ndrop fails when the value is absent and the add when it is present.\n\nSHOW ENUMERATIONS;\nSHOW ENUMERATIONS IN <module>;\nDESCRIBE ENUMERATION Module.Name;\nDROP ENUMERATION Module.Name;\n\nUsing in entity:\n  AttrName: Enumeration(Module.EnumName)\n\nThe System module's enumerations are platform built-ins with no stored\nunit. SHOW ENUMERATIONS and DESCRIBE ENUMERATION report them anyway, so\ntheir values can be read instead of guessed at until the build rejects one\nwith CE1613. They are READ-ONLY: DESCRIBE prints them as -- comment lines,\nand CREATE / ALTER / DROP / MOVE naming System is refused.\n  mxcli -p app.mpr describe enumeration System.WorkflowActivityType",
		Example: "CREATE ENUMERATION MyModule.OrderStatus (\n  Pending 'Pending Approval',\n  Processing 'Being Processed',\n  Shipped 'Shipped to Customer'\n);\n\n-- Using in an entity\nCREATE PERSISTENT ENTITY MyModule.Order (\n  OrderNumber: String(20) NOT NULL,\n  Status: Enumeration(MyModule.OrderStatus)\n);",
		SeeAlso: []string{"domain-model.enumeration", "domain-model.entity.attributes"},
	})

	// --- Constant ---

	Register(SyntaxFeature{
		Path:    "domain-model.constant",
		Summary: "Constants: named configuration values (String, Integer, Boolean, etc.)",
		Keywords: []string{
			"constant", "configuration", "config value",
			"create constant", "setting",
		},
		Syntax:  "CREATE CONSTANT Module.Name TYPE DataType DEFAULT value [COMMENT 'text'];\nCREATE OR MODIFY CONSTANT Module.Name TYPE DataType DEFAULT value;\n\nSHOW CONSTANTS;\nDESCRIBE CONSTANT Module.Name;\nDROP CONSTANT Module.Name;",
		Example: "CREATE CONSTANT MyModule.ApiBaseUrl\n  TYPE String\n  DEFAULT 'https://api.example.com/v1';\n\nCREATE CONSTANT MyModule.MaxRetries\n  TYPE Integer\n  DEFAULT 3\n  COMMENT 'Maximum API retry attempts';",
		SeeAlso: []string{"domain-model.constant.create"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.constant.create",
		Summary: "CREATE/DROP/DESCRIBE CONSTANT with supported types and configuration values",
		Keywords: []string{
			"create constant", "drop constant", "describe constant",
			"show constants", "constant values", "modify constant",
			"string constant", "integer constant", "boolean constant",
		},
		Syntax: "CREATE CONSTANT Module.Name\n  TYPE String|Integer|Long|Decimal|Boolean|DateTime\n  DEFAULT value\n  [COMMENT 'description'];\n\nCREATE OR MODIFY CONSTANT Module.Name\n  TYPE DataType DEFAULT value [COMMENT 'text'];\n\nSHOW CONSTANTS;\nSHOW CONSTANTS IN <module>;\nSHOW CONSTANT VALUES;\nDESCRIBE CONSTANT Module.Name;\nDROP CONSTANT Module.Name;\n\nRemove override:\n  ALTER SETTINGS DROP CONSTANT 'Module.Name' IN CONFIGURATION 'cfg';\n\n" +
			"Shared vs private values:\n" +
			"  A per-configuration override holds either a SHARED value (stored in the\n" +
			"  model, so in version control — every developer gets it) or a PRIVATE one\n" +
			"  (stored on the developer's own workstation, deliberately out of the repo;\n" +
			"  the usual choice for development API tokens).\n\n" +
			"  MDL preserves that choice but never changes it. ALTER SETTINGS CONSTANT\n" +
			"  applies to shared values only — on a private override it is refused, since\n" +
			"  setting a value would publish a deliberately-local one into version control.\n" +
			"  SHOW CONSTANT VALUES reports it as (private); DESCRIBE SETTINGS reports it\n" +
			"  as a comment, not a re-executable statement. DROP CONSTANT still works.\n" +
			"  Change a constant to a shared value in Studio Pro.",
		Example: "CREATE CONSTANT MyModule.ApiBaseUrl\n  TYPE String\n  DEFAULT 'https://api.example.com/v1';\n\nCREATE CONSTANT MyModule.MaxRetries\n  TYPE Integer DEFAULT 3\n  COMMENT 'Maximum number of API retry attempts';\n\nCREATE CONSTANT MyModule.EnableDebug\n  TYPE Boolean DEFAULT false;\n\nCREATE OR MODIFY CONSTANT MyModule.ApiBaseUrl\n  TYPE String\n  DEFAULT 'https://api.staging.example.com/v2';",
		SeeAlso: []string{"domain-model.constant"},
	})

	// --- Keywords ---

	Register(SyntaxFeature{
		Path:    "domain-model.keywords",
		Summary: "Reserved keywords that require quoting when used as identifiers",
		Keywords: []string{
			"keywords", "reserved words", "identifier",
			"quoted identifier", "escape", "backtick", "double quote",
		},
		Syntax:  "Quoted identifier syntax:\n  \"ModuleName\".EntityName     -- ANSI SQL double quotes\n  `ModuleName`.EntityName     -- MySQL-style backticks\n  \"ModuleName\".\"EntityName\"   -- Both parts quoted\n\nMixed quoting is allowed: \"ComboBox\".CategoryTreeVE",
		Example: "-- Use quotes when module/entity name conflicts with a keyword\nDESCRIBE ENTITY \"ComboBox\".\"CategoryTreeVE\";\nSHOW ENTITIES IN \"ComboBox\";\nSHOW MICROFLOWS IN `Order`;\n\n-- Common conflicts: ComboBox, DataGrid, Gallery, Title, Status, Type, Value",
	})

	// --- Types ---

	Register(SyntaxFeature{
		Path:    "domain-model.types",
		Summary: "Attribute data types reference: String, Integer, Decimal, Boolean, DateTime, etc.",
		Keywords: []string{
			"types", "data types", "attribute types",
			"string", "integer", "long", "decimal", "boolean",
			"datetime", "autonumber", "binary", "hashedstring",
			"enumeration type", "currency", "float",
		},
		Syntax:  "String(n)           Variable-length text up to n characters\nInteger             Whole number (-2B to 2B)\nLong                Large whole number\nDecimal             Precise decimal for currency/calculations\nBoolean             True or false\nDateTime            Date and time combined\nDate                Date only (no time)\nAutoNumber          Auto-incrementing integer\nBinary              Binary data (files, images)\nHashedString        Securely hashed string (passwords)\nEnumeration(Name)   Reference to an enumeration\nAutoOwner           System.owner (auto-set on create)\nAutoChangedBy       System.changedBy (auto-set on commit)\nAutoCreatedDate     DateTime (auto-set on create)\nAutoChangedDate     DateTime (auto-set on commit)",
		Example: "CREATE PERSISTENT ENTITY MyModule.Customer (\n  Name: String(100) NOT NULL,\n  Age: Integer,\n  Balance: Decimal,\n  IsActive: Boolean DEFAULT true,\n  CreatedAt: DateTime,\n  Status: Enumeration(MyModule.Status)\n);",
		SeeAlso: []string{"domain-model.entity.attributes"},
	})
}
