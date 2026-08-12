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
		Path:    "domain-model.entity",
		Summary: "Entity creation: persistent, non-persistent, generalization, event handlers",
		Keywords: []string{
			"entity", "create entity", "persistent", "non-persistent",
			"generalization", "extends", "event handler", "attribute",
		},
		Syntax:  "CREATE PERSISTENT ENTITY Module.Name (\n  Attr: Type [constraints],\n  ...\n) [INDEX (attr1)] [COMMENT 'text'];\n\nCREATE NON_PERSISTENT ENTITY Module.Name (...);\n\nCREATE PERSISTENT ENTITY Module.Name EXTENDS Module.Parent (...);",
		Example: "CREATE PERSISTENT ENTITY MyModule.Customer (\n  Name: String(100) NOT NULL ERROR 'Name is required',\n  Email: String(200) UNIQUE,\n  Balance: Decimal DEFAULT 0,\n  IsActive: Boolean DEFAULT true,\n  Status: Enumeration(MyModule.CustomerType)\n)\nINDEX (Email)\nCOMMENT 'Stores customer information';",
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
		Syntax:  "CREATE PERSISTENT ENTITY Module.Name (\n  Attr: Type [NOT NULL [ERROR 'msg']] [UNIQUE [ERROR 'msg']] [DEFAULT val],\n  ...\n)\n[INDEX (attr1, attr2)]\n[ON BEFORE|AFTER CREATE|COMMIT|DELETE|ROLLBACK CALL Module.MF [RAISE ERROR]]\n[COMMENT 'text'];\n\nCREATE NON_PERSISTENT ENTITY Module.Name (...);\nCREATE PERSISTENT ENTITY Module.Name EXTENDS Module.Parent (...);",
		Example: "-- Persistent with constraints and index\nCREATE PERSISTENT ENTITY Shop.Order (\n  OrderNumber: String(20) NOT NULL,\n  Total: Decimal DEFAULT 0,\n  CreatedAt: DateTime\n)\nINDEX (OrderNumber)\nON BEFORE COMMIT CALL Shop.ValidateOrder($currentObject) RAISE ERROR;\n\n-- With generalization\nCREATE PERSISTENT ENTITY Shop.ProductImage EXTENDS System.Image (\n  Caption: String(200)\n);",
		SeeAlso: []string{"domain-model.entity.alter", "domain-model.entity.attributes"},
	})

	Register(SyntaxFeature{
		Path:    "domain-model.entity.alter",
		Summary: "ALTER ENTITY: add/rename/modify/drop attributes, indexes, documentation, event handlers",
		Keywords: []string{
			"alter entity", "modify entity", "add attribute",
			"drop attribute", "rename attribute", "add index",
			"event handler", "documentation",
			"if not exists", "if exists", "idempotent",
		},
		Syntax:  "ALTER ENTITY Module.Name ADD ATTRIBUTE [IF NOT EXISTS] AttrName: Type [constraints];\nALTER ENTITY Module.Name DROP ATTRIBUTE [IF EXISTS] AttrName;\nALTER ENTITY Module.Name RENAME ATTRIBUTE OldName TO NewName;\nALTER ENTITY Module.Name MODIFY ATTRIBUTE AttrName SET DEFAULT val;\nALTER ENTITY Module.Name ADD INDEX [name] [ON] (attr1, attr2);\nALTER ENTITY Module.Name SET DOCUMENTATION 'text';\nALTER ENTITY Module.Name ADD EVENT HANDLER ON BEFORE COMMIT CALL Module.MF RAISE ERROR;\n\nIF NOT EXISTS / IF EXISTS make the add/drop a no-op (skipped, not an error)\nwhen the attribute is already present / already gone — so a domain script\nre-runs cleanly. For a whole script, 'mxcli exec --continue-on-error' reports\neach failed statement and keeps going instead of halting at the first.",
		Example: "ALTER ENTITY Shop.Customer ADD ATTRIBUTE Phone: String(20);\nALTER ENTITY Shop.Customer ADD ATTRIBUTE IF NOT EXISTS Phone: String(20);  -- re-runnable\nALTER ENTITY Shop.Customer DROP ATTRIBUTE IF EXISTS OldField;              -- re-runnable\nALTER ENTITY Shop.Customer RENAME ATTRIBUTE Email TO EmailAddress;\nALTER ENTITY Shop.Customer ADD INDEX ON (EmailAddress);\nALTER ENTITY Shop.Customer\n  ADD EVENT HANDLER ON BEFORE COMMIT CALL Shop.Validate($currentObject) RAISE ERROR;",
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

	// --- Association ---

	Register(SyntaxFeature{
		Path:    "domain-model.association",
		Summary: "Associations: references between entities (many-to-one, many-to-many)",
		Keywords: []string{
			"association", "reference", "reference set",
			"many-to-one", "many-to-many", "foreign key",
			"owner", "delete behavior",
		},
		Syntax:  "[@anchor(from: (x, y), to: (x, y))]\nCREATE [OR MODIFY] ASSOCIATION Module.Name\n  FROM Module.FromEntity TO Module.ToEntity\n  TYPE Reference|ReferenceSet\n  [OWNER Default|Both]\n  [DELETE_BEHAVIOR behavior]\n  [COMMENT 'text'];\nALTER ASSOCIATION Module.Name SET ANCHOR FROM (x, y) TO (x, y);\nDROP ASSOCIATION Module.Name;\n\nOR MODIFY: updates type/owner/delete behavior in-place, preserves UUID.\n\n@anchor sets the LINE ANCHORS — where the connector attaches to each entity box\nin the domain model editor — as a PERCENTAGE of the box (0..100, whole numbers).\n`from` is the FROM entity's box, `to` the TO entity's: (0, 50) is the middle of\nthe left edge, (100, 50) the right, (50, 100) the bottom centre. Omitting an end\nPRESERVES what is stored, so a CREATE OR MODIFY about something else never\nflattens a hand-tuned line. Cross-module associations have no anchors.",
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
			"delete behavior", "cascade", "prevent",
			"delete and references", "delete but keep references",
			"delete if no references", "referential integrity",
		},
		Syntax:  "DELETE_BEHAVIOR options:\n  DELETE_BUT_KEEP_REFERENCES  Delete object, nullify FK (default)\n  DELETE_AND_REFERENCES       Delete object and cascade to children\n  DELETE_IF_NO_REFERENCES     Prevent deletion if referenced\n  CASCADE                     Alias for DELETE_AND_REFERENCES\n  PREVENT                     Alias for DELETE_IF_NO_REFERENCES",
		Example: "CREATE ASSOCIATION Shop.Order_Customer\n  FROM Shop.Order TO Shop.Customer\n  TYPE Reference\n  DELETE_BEHAVIOR PREVENT;\n\nCREATE ASSOCIATION Shop.Order_Lines\n  FROM Shop.OrderLine TO Shop.Order\n  TYPE Reference\n  DELETE_BEHAVIOR CASCADE;",
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
		Syntax:  "CREATE ENUMERATION Module.Name (\n  ValueName 'Display Caption',\n  ...\n);\n\nALTER ENUMERATION Module.Name ADD VALUE NewValue [CAPTION 'Display Caption'];\nALTER ENUMERATION Module.Name RENAME VALUE OldName TO NewName;\nALTER ENUMERATION Module.Name MODIFY VALUE ValueName CAPTION 'New Caption';\nALTER ENUMERATION Module.Name DROP VALUE ValueName;\n\nSHOW ENUMERATIONS;\nSHOW ENUMERATIONS IN <module>;\nDESCRIBE ENUMERATION Module.Name;\nDROP ENUMERATION Module.Name;\n\nUsing in entity:\n  AttrName: Enumeration(Module.EnumName)",
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
