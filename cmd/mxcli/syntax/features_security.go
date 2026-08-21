// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "security",
		Summary: "Application security: roles, access control, demo users",
		Keywords: []string{
			"security", "access control", "roles", "permissions",
			"grant", "revoke", "authentication", "authorization",
		},
		Syntax:  "SHOW PROJECT SECURITY;\nSHOW MODULE ROLES [IN <module>];\nSHOW USER ROLES;\nSHOW SECURITY MATRIX [IN <module>];",
		Example: "SHOW PROJECT SECURITY;\nSHOW SECURITY MATRIX IN Shop;",
		SeeAlso: []string{"security.module-role", "security.entity-access", "security.user-role"},
	})

	Register(SyntaxFeature{
		Path:    "security.module-role",
		Summary: "Create and manage module-level security roles",
		Keywords: []string{
			"module role", "create role", "drop role",
		},
		Syntax:  "CREATE [OR MODIFY] MODULE ROLE <module>.<role> [DESCRIPTION '<text>'];\nDROP MODULE ROLE <module>.<role>;",
		Example: "CREATE MODULE ROLE Shop.Admin DESCRIPTION 'Full access';\n-- OR MODIFY makes a security script re-runnable:\nCREATE OR MODIFY MODULE ROLE Shop.User DESCRIPTION 'Read-only access';",
		SeeAlso: []string{"security.user-role", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.entity-access",
		Summary: "Grant or revoke entity-level access (CRUD, attribute-level, XPath rules)",
		Keywords: []string{
			"entity access", "grant", "revoke", "read", "write",
			"create", "delete", "xpath", "row-level security",
		},
		Syntax: "GRANT <role> ON <module>.<entity> (<rights>) [WHERE '<xpath>'];\n" +
			"REVOKE <role> ON <module>.<entity>;\n" +
			"REVOKE <role> ON <module>.<entity> (<rights>);\n\n" +
			"Rights: CREATE, DELETE, READ *, READ (<attr>,...), WRITE *, WRITE (<attr>,...)\n\n" +
			"Inherited members:\n" +
			"  Mendix inheritance is multi-table — a child adds attributes to its\n" +
			"  parent's, and ALL the parent's members belong to the child. Name them\n" +
			"  in a GRANT exactly like the entity's own; READ */WRITE * covers them\n" +
			"  too. A name that matches no member is an error, not a silent skip.\n\n" +
			"  Exception: entities extending System.User are user entities, whose\n" +
			"  platform members (Name, Password, Blocked, ...) Mendix manages. Do not\n" +
			"  grant those; mxcli leaves them out of the rule automatically.",
		Example: "GRANT Shop.Admin ON Shop.Customer (CREATE, DELETE, READ *, WRITE *);\n" +
			"GRANT Shop.User ON Shop.Customer (READ *) WHERE '[Active = true()]';\n\n" +
			"-- Contract extends DocumentBase: DocName is inherited, ContractNumber is own\n" +
			"GRANT Docs.Viewer ON Docs.Contract (READ (DocName, ContractNumber));\n\n" +
			"-- Attachment extends System.FileDocument: Name and Size are inherited\n" +
			"GRANT Docs.Viewer ON Docs.Attachment (READ (Category, \"Name\", Size));",
		SeeAlso: []string{"security.module-role", "security.microflow-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.microflow-access",
		Summary: "Grant or revoke execution rights on microflows",
		Keywords: []string{
			"microflow access", "execute", "grant microflow",
			"revoke microflow",
		},
		Syntax:  "GRANT EXECUTE ON MICROFLOW <module>.<name> TO <role> [, <role>...];\nREVOKE EXECUTE ON MICROFLOW <module>.<name> FROM <role> [, <role>...];",
		Example: "GRANT EXECUTE ON MICROFLOW Shop.ProcessOrder TO Shop.Admin, Shop.User;\nREVOKE EXECUTE ON MICROFLOW Shop.ProcessOrder FROM Shop.User;",
		SeeAlso: []string{"security.page-access", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.page-access",
		Summary: "Grant or revoke view rights on pages",
		Keywords: []string{
			"page access", "view", "grant page", "revoke page",
		},
		Syntax:  "GRANT VIEW ON PAGE <module>.<name> TO <role> [, <role>...];\nREVOKE VIEW ON PAGE <module>.<name> FROM <role> [, <role>...];",
		Example: "GRANT VIEW ON PAGE Shop.OrderOverview TO Shop.Admin, Shop.User;",
		SeeAlso: []string{"security.microflow-access", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.user-role",
		Summary: "Create and manage application-level user roles that bundle module roles",
		Keywords: []string{
			"user role", "application role", "manage roles",
			"add module roles", "remove module roles",
		},
		Syntax:  "CREATE USER ROLE <name> (<role> [, ...]) [MANAGE ALL ROLES];\nALTER USER ROLE <name> ADD MODULE ROLES (<role> [, ...]);\nALTER USER ROLE <name> REMOVE MODULE ROLES (<role> [, ...]);\nDROP USER ROLE <name>;",
		Example: "CREATE USER ROLE AppAdmin (Shop.Admin, HR.Admin) MANAGE ALL ROLES;\nALTER USER ROLE AppAdmin ADD MODULE ROLES (Reporting.Viewer);",
		SeeAlso: []string{"security.module-role", "security.demo-user"},
	})

	Register(SyntaxFeature{
		Path:    "security.project-security",
		Summary: "Set project security level, demo user and guest access toggles",
		Keywords: []string{
			"project security", "security level", "prototype",
			"production", "off",
		},
		Syntax:  "ALTER PROJECT SECURITY LEVEL OFF|PROTOTYPE|PRODUCTION;\nALTER PROJECT SECURITY DEMO USERS ON|OFF;",
		Example: "ALTER PROJECT SECURITY LEVEL PRODUCTION;\nALTER PROJECT SECURITY DEMO USERS OFF;",
		SeeAlso: []string{"security.demo-user", "security.guest-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.guest-access",
		Summary: "Enable anonymous (guest) access and pick the role anonymous visitors get",
		Keywords: []string{
			"guest access", "anonymous", "anonymous users", "public",
			"unauthenticated", "guest user role", "CE0133",
		},
		Syntax: "ALTER PROJECT SECURITY GUEST ACCESS ON ROLE <UserRole>;\n" +
			"ALTER PROJECT SECURITY GUEST ACCESS ON;   -- only when a role is already configured\n" +
			"ALTER PROJECT SECURITY GUEST ACCESS OFF;  -- keeps the stored role\n" +
			"\n" +
			"-- The role is what anonymous visitors get, so its entity access IS the app's\n" +
			"-- public surface. Mendix requires one: guest access with no role fails the\n" +
			"-- build (CE0133), so ON is refused unless a role is given or already stored.\n" +
			"-- Mendix does not check the role exists, so mxcli does — an unknown role\n" +
			"-- would build cleanly and leave visitors with nothing.",
		Example: "CREATE USER ROLE Anonymous (Shop.Viewer, System.User);\n" +
			"ALTER PROJECT SECURITY GUEST ACCESS ON ROLE Anonymous;\n" +
			"GRANT Anonymous ON Shop.Product (read *);",
		SeeAlso: []string{"security.user-role", "security.project-security"},
	})

	Register(SyntaxFeature{
		Path:    "security.demo-user",
		Summary: "Create and manage demo users for testing",
		Keywords: []string{
			"demo user", "test user", "demo account",
			"password", "login",
		},
		Syntax:  "CREATE DEMO USER '<name>' PASSWORD '<pass>' [ENTITY Module.Entity] (<userrole> [, ...]);\nDROP DEMO USER '<name>';",
		Example: "CREATE DEMO USER 'admin' PASSWORD 'Admin1!' (AppAdmin);\nCREATE DEMO USER 'user' PASSWORD 'User1!' (AppUser);",
		SeeAlso: []string{"security.user-role", "security.project-security"},
	})
}
