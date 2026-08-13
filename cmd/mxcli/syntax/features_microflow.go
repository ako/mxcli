// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "microflow",
		Summary: "Programmatic logic — variables, object operations, control flow, and integrations",
		Keywords: []string{
			"microflow", "nanoflow", "logic", "automation",
			"action", "activity", "flow",
		},
		Syntax:  "CREATE [OR REPLACE | OR MODIFY] MICROFLOW Module.Name ($Param: Type) RETURNS Type AS $Result\nBEGIN\n  <statements>\nEND;",
		Example: "CREATE MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;\n\n-- Re-runnable: replaces the microflow if it already exists\nCREATE OR REPLACE MICROFLOW MyModule.ACT_CreateOrder ($Code: String)\nRETURNS MyModule.Order AS $NewOrder\nBEGIN\n  $NewOrder = CREATE MyModule.Order (OrderNumber = $Code);\n  RETURN $NewOrder;\nEND;",
		SeeAlso: []string{"microflow.create", "microflow.variables", "microflow.control-flow", "create-modifiers"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.synchronize",
		Summary: "SYNCHRONIZE — offline data synchronization (nanoflow only)",
		Keywords: []string{
			"synchronize", "sync", "offline", "unsynchronized", "specific",
			"offline first", "nanoflow",
		},
		Syntax:  "SYNCHRONIZE ALL [ON ERROR ...];\nSYNCHRONIZE UNSYNCHRONIZED [ON ERROR ...];   -- Mendix 9.4+\nSYNCHRONIZE $Var[, $Var...] [ON ERROR ...];\n\nNanoflow only: in a microflow this is MDL057 / CE0009.",
		Example: "CREATE NANOFLOW MyModule.NF_Sync ($Order: MyModule.Order)\nBEGIN\n  SYNCHRONIZE ALL;\n  SYNCHRONIZE UNSYNCHRONIZED;\n  SYNCHRONIZE $Order;\n  SYNCHRONIZE ALL ON ERROR WITHOUT ROLLBACK {\n    LOG ERROR 'sync failed';\n  };\nEND;",
		SeeAlso: []string{"microflow.nanoflow", "microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.create",
		Summary: "Create a microflow with parameters, return type, and body",
		Keywords: []string{
			"create microflow", "new microflow", "define microflow",
			"parameters", "returns", "folder",
		},
		Syntax:  "CREATE MICROFLOW Module.Name ($P1: String, $P2: Integer)\n  RETURNS Type AS $Result\n  [FOLDER 'FolderPath']\nBEGIN\n  <statements>\nEND;",
		Example: "CREATE MICROFLOW MyModule.ACT_CreateOrder (\n  $CustomerCode: String,\n  $Quantity: Integer\n)\nRETURNS MyModule.Order AS $NewOrder\nFOLDER 'Orders'\nBEGIN\n  $NewOrder = CREATE MyModule.Order (\n    OrderNumber = 'ORD-001',\n    Quantity = $Quantity\n  );\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;",
		SeeAlso: []string{"microflow.nanoflow", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.variables",
		Summary: "Declare variables, assign values, and set object attributes",
		Keywords: []string{
			"declare", "variable", "set", "assign", "change",
			"attribute", "expression",
		},
		Syntax: "DECLARE $Var Type;                 -- a variable must be declared before it is assigned\n" +
			"DECLARE $Var Type = expression;\n" +
			"$Var = expression;                -- assign; SET is optional\n" +
			"$Var/Attribute = expression;\n" +
			"SET $Var = expression;            -- same statement, explicit form",
		Example: "DECLARE $Count Integer = 0;\nDECLARE $Name String;\n$Count = $Count + 1;\n$Name = 'Hello';\nSET $Order/Status = 'Pending';",
		SeeAlso: []string{"microflow.object-operations"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.retrieve",
		Summary: "Query the database with WHERE, SORT BY, LIMIT, and OFFSET",
		Keywords: []string{
			"retrieve", "query", "database", "where", "sort",
			"limit", "offset", "find", "fetch",
		},
		// Retrieve-by-association was missing here, so it read as unsupported
		// even though it works and the write-microflows skill documents it.
		Syntax:  "-- From the database\nRETRIEVE $Var FROM Module.Entity\n  [WHERE condition]\n  [SORT BY attr ASC|DESC]\n  [LIMIT n] [OFFSET n];\n\n-- Over an association, from an object you already have\nRETRIEVE $Var FROM $Object/Module.Association;",
		Example: "RETRIEVE $Customer FROM MyModule.Customer\n  WHERE Code = $CustomerCode\n  LIMIT 1;\n\nRETRIEVE $Orders FROM MyModule.Order\n  WHERE Status = 'Pending'\n  SORT BY CreateDate DESC\n  LIMIT 10 OFFSET 0;\n\n-- Follow an association rather than querying the database\nRETRIEVE $Orders FROM $Customer/MyModule.Order_Customer;\nRETRIEVE $Customer FROM $Order/MyModule.Order_Customer;",
		SeeAlso: []string{"microflow.object-operations", "xpath"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.control-flow",
		Summary: "IF/ELSIF/ELSE, LOOP, WHILE, BREAK, CONTINUE, RETURN",
		Keywords: []string{
			"if", "elsif", "else", "then", "end if",
			"loop", "while", "break", "continue", "return",
			"conditional", "branch", "iterate",
		},
		Syntax:  "IF condition THEN\n  ...\nELSIF condition THEN\n  ...\nELSE\n  ...\nEND IF;\n\nLOOP $Item IN $List BEGIN ... END LOOP;\nWHILE condition BEGIN ... END WHILE;\nRETURN $Value;\nRETURN empty;",
		Example: "IF $Customer = empty THEN\n  LOG ERROR NODE 'Svc' 'Not found';\n  RETURN empty;\nELSIF $Customer/Active = false THEN\n  LOG WARNING 'Inactive customer';\nELSE\n  CHANGE $Customer (LastAccess = [%CurrentDateTime%]);\nEND IF;\n\nLOOP $Item IN $OrderLines BEGIN\n  COMMIT $Item;\nEND LOOP;",
		SeeAlso: []string{"microflow.variables", "microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.error-handling",
		Summary: "Error handling with ON ERROR, THROW, CONTINUE, ROLLBACK",
		Keywords: []string{
			"error", "error handling", "on error", "continue",
			"rollback", "throw", "exception", "try", "catch",
		},
		Syntax:  "COMMIT $Obj ON ERROR CONTINUE;\nCOMMIT $Obj ON ERROR ROLLBACK;\nCOMMIT $Obj ON ERROR { <statements> };\nCOMMIT $Obj ON ERROR WITHOUT ROLLBACK { <statements> };",
		Example: "COMMIT $Order ON ERROR {\n  LOG ERROR 'Failed to save order';\n  RETURN empty;\n};\n\nCOMMIT $Batch ON ERROR WITHOUT ROLLBACK {\n  LOG WARNING 'Batch save failed, continuing';\n};",
		SeeAlso: []string{"microflow.control-flow"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.object-operations",
		Summary: "CREATE, CHANGE, COMMIT, ROLLBACK, and DELETE objects",
		Keywords: []string{
			"create object", "change object", "commit", "rollback",
			"delete", "save", "persist", "modify object",
			"with events", "refresh", "commit flag", "without events",
		},
		Syntax:  "$Obj = CREATE Module.Entity (Attr = value) [COMMIT [WITHOUT EVENTS]];\nCHANGE $Obj (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\nCOMMIT $Obj;\nCOMMIT $Obj WITH EVENTS;\nCOMMIT $Obj REFRESH;\nCOMMIT $Obj WITH EVENTS REFRESH;\nDELETE $Obj;\nROLLBACK $Obj;\n\n-- The COMMIT modifier on CREATE/CHANGE is the activity's Commit setting\n-- (omitted = No). The standalone COMMIT $Obj is a separate activity.",
		Example: "$NewOrder = CREATE MyModule.Order (\n  OrderNumber = 'ORD-001',\n  Quantity = $Quantity,\n  CreateDate = [%CurrentDateTime%]\n) COMMIT;\n\nCHANGE $NewOrder (MyModule.Order_Customer = $Customer) COMMIT REFRESH;\nCHANGE $Draft (Status = 'Imported') COMMIT WITHOUT EVENTS;\nDELETE $OldOrder;\nROLLBACK $DraftOrder;",
		SeeAlso: []string{"microflow.retrieve", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.list-operations",
		Summary: "List manipulation — HEAD, TAIL, FIND, FILTER, SORT, UNION, aggregates",
		Keywords: []string{
			"list", "head", "tail", "find", "filter", "sort",
			"union", "intersect", "subtract", "count", "sum",
			"average", "aggregate", "add to list", "remove from list",
			"create list",
		},
		Syntax:  "$List = CREATE LIST OF Module.Entity;\nADD $Item TO $List;\nREMOVE $Item FROM $List;\n$Result = HEAD($List);\n$Result = TAIL($List);\n$Result = FIND($List, condition);\n$Result = FILTER($List, condition);\n$Result = SORT($List, attr ASC);\n$Result = UNION($L1, $L2);\n$Result = INTERSECT($L1, $L2);\n$Result = SUBTRACT($L1, $L2);\n$Count = COUNT($List);\n$Sum = SUM($List.Attr);\n$Avg = AVERAGE($List.Attr);",
		Example: "$AllOrders = CREATE LIST OF MyModule.Order;\nADD $NewOrder TO $AllOrders;\n$First = HEAD($AllOrders);\n$Pending = FILTER($AllOrders, Status = 'Pending');\n$Sorted = SORT($Pending, CreateDate DESC);\n$Total = SUM($AllOrders.Amount);",
		SeeAlso: []string{"microflow.retrieve"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.logging",
		Summary: "LOG statements with level, node, message templates",
		Keywords: []string{
			"log", "logging", "info", "warning", "error", "debug",
			"trace", "critical", "node", "message template",
		},
		Syntax:  "LOG LEVEL [NODE 'Name'] 'message';\nLOG LEVEL 'template {1}' WITH ({1} = $value);\n\n-- Levels: INFO, WARNING, ERROR, DEBUG, TRACE, CRITICAL",
		Example: "LOG INFO NODE 'OrderService' 'Order created successfully';\nLOG WARNING 'Customer not found';\nLOG ERROR 'Failed to process {1}' WITH (\n  {1} = $OrderNumber\n);",
	})

	Register(SyntaxFeature{
		Path:    "microflow.show-page",
		Summary: "Open and close pages from microflows",
		Keywords: []string{
			"show page", "open page", "close page", "display page",
			"navigate", "page action",
		},
		Syntax:  "SHOW PAGE Module.Page;\nSHOW PAGE Module.Page ($Param = $value);\nCLOSE PAGE;",
		Example: "SHOW PAGE MyModule.OrderDetail ($Order = $NewOrder);\nCLOSE PAGE;",
		SeeAlso: []string{"page"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.call",
		Summary: "Call microflows and Java actions with parameters",
		Keywords: []string{
			"call microflow", "call java action", "invoke",
			"sub-microflow", "java action", "parameter passing",
		},
		Syntax:  "$Result = CALL MICROFLOW Module.Name (Param = value);\n$Result = CALL JAVA ACTION Module.Name (Param = value);",
		Example: "$IsValid = CALL MICROFLOW MyModule.ValidateOrder (\n  Order = $NewOrder\n);\n\n$Token = CALL JAVA ACTION MyModule.GenerateToken (\n  UserId = $User/Id\n);",
		SeeAlso: []string{"java-action", "microflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.nanoflow",
		Summary: "CREATE NANOFLOW — client-side logic, same syntax as microflow",
		Keywords: []string{
			"nanoflow", "create nanoflow", "client-side",
			"offline", "client logic",
		},
		Syntax:  "CREATE NANOFLOW Module.Name ($Param: Type) RETURNS Type AS $Result\nBEGIN\n  <statements>\nEND;",
		Example: "CREATE NANOFLOW MyModule.NF_ValidateInput ($Input: String)\nRETURNS Boolean AS $IsValid\nBEGIN\n  IF $Input = empty THEN\n    VALIDATION FEEDBACK $Input MESSAGE 'Required';\n    RETURN false;\n  END IF;\n  RETURN true;\nEND;",
		SeeAlso: []string{"microflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.validation",
		Summary: "Show validation feedback on object attributes",
		Keywords: []string{
			"validation", "feedback", "validation feedback",
			"error message", "field error", "form validation",
		},
		Syntax:  "VALIDATION FEEDBACK $Obj/Attr MESSAGE 'error text';\nVALIDATION FEEDBACK $Obj/Attr MESSAGE '{1} is invalid'\n  OBJECTS [$Value];",
		Example: "VALIDATION FEEDBACK $Order/Quantity MESSAGE 'Quantity must be positive';\nVALIDATION FEEDBACK $Customer/Email MESSAGE '{1} is not valid'\n  OBJECTS [$Customer/Email];",
		SeeAlso: []string{"microflow.error-handling"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.layout",
		Summary: "Canvas layout annotations — @position, @anchor, @curve, @caption, @color",
		Keywords: []string{
			"position", "anchor", "curve", "layout", "canvas",
			"annotation", "caption", "color", "excluded", "bezier",
		},
		Syntax: "@position(x, y)                       -- the activity's centre point\n" +
			"@anchor(from: right, to: left)        -- which SIDE each end of the outgoing flow attaches to\n" +
			"@curve(from: (40, -90), to: (-40, 90))  -- the flow's bezier control vectors\n" +
			"@caption 'text'\n@color Green\n@annotation 'a note'\n@excluded\n\n" +
			"An unrecognised @name is an error (MDL059): it would parse and do nothing,\n" +
			"so a typo of @position would silently discard the layout.\n\n" +
			"Mendix stores no waypoints — a flow's shape is two control vectors, each a\n" +
			"pixel offset from its end of the line. (0, 0) at both ends is straight.\n" +
			"Container Size is still computed, not authorable.",
		Example: "create microflow MyModule.ACT_Flow ($In: String)\nreturns String as $Out\nbegin\n" +
			"  @position(200, 100)\n  @anchor(from: bottom, to: top)\n" +
			"  @curve(from: (40, -90), to: (-40, 90))\n  declare $Tmp String = $In;\n" +
			"  @position(200, 300)\n  declare $Out String = $Tmp;\n  return $Out;\nend;",
		SeeAlso: []string{"microflow", "microflow.create"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.mapping",
		Summary: "IMPORT FROM MAPPING / EXPORT TO MAPPING, and the import Range (All/First/Custom)",
		Keywords: []string{
			"import from mapping", "export to mapping", "import mapping activity",
			"range", "all", "first", "limit", "offset", "single object",
		},
		Syntax: "[$Var =] IMPORT FROM MAPPING Module.IMM ($SourceVar) [<range>];\n" +
			"$Var = EXPORT TO MAPPING Module.EMM ($EntityVar);\n\n" +
			"<range> — Studio Pro's Range on the import activity:\n" +
			"  ALL                          bind the whole result\n" +
			"  FIRST                        bind ONE object (not a one-element list)\n" +
			"  LIMIT <expr> [OFFSET <expr>] a bounded list\n\n" +
			"Omit it and the cardinality is inferred from the mapping's root shape.\n" +
			"The range does not change WHAT the mapping returns: an object-rooted\n" +
			"mapping binds an object under ALL too (Studio Pro's own default).\n" +
			"Mendix rejects OFFSET on a non-list mapping with CE6100.",
		Example: "$Pets  = import from mapping Shop.IMM_Pets($Json) all;\n" +
			"$Pet   = import from mapping Shop.IMM_Pets($Json) first;\n" +
			"$Page  = import from mapping Shop.IMM_Pets($Json) limit 10 offset 5;\n" +
			"$Json2 = export to mapping Shop.EMM_Pet($Pet);",
		SeeAlso: []string{"import-mapping", "export-mapping", "json-structure"},
	})
}
