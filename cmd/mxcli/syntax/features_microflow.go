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
			"exposed as", "expose as microflow action", "expose as workflow action",
			"toolbox", "icon", "image",
		},
		Syntax:  "CREATE MICROFLOW Module.Name ($P1: String, $P2: Integer)\n  RETURNS Type AS $Result\n  [FOLDER 'FolderPath']\n  [EXPOSED AS MICROFLOW ACTION 'Caption' IN 'Category'\n     [ICON 'icon.png'] [ICON DARK 'icon-dark.png']\n     [IMAGE 'image.png'] [IMAGE DARK 'image-dark.png']]\n  [EXPOSED AS WORKFLOW ACTION 'Caption' IN 'Category']\n  [NOT EXPOSED AS MICROFLOW|WORKFLOW ACTION]\nBEGIN\n  <statements>\nEND;\n\nEXPOSED AS puts the microflow in Studio Pro's toolbox, so whoever drags it in\ndoes not need to know it is a microflow. There are two toolboxes — the\nmicroflow editor's and the workflow editor's — so the clause names which.\nThe icon is a 64x64 PNG and the image a 256x192 PNG, read from disk relative\nto the .mdl file's own directory. An OMITTED clause preserves what is stored, so\nremoving an entry is NOT EXPOSED and clearing one bitmap is DROP ICON/IMAGE.",
		Example: "CREATE MICROFLOW MyModule.ACT_CreateOrder (\n  $CustomerCode: String,\n  $Quantity: Integer\n)\nRETURNS MyModule.Order AS $NewOrder\nFOLDER 'Orders'\nEXPOSED AS MICROFLOW ACTION 'Create order' IN 'Orders'\n  ICON 'assets/order-64.png'\nBEGIN\n  $NewOrder = CREATE MyModule.Order (\n    OrderNumber = 'ORD-001',\n    Quantity = $Quantity\n  );\n  COMMIT $NewOrder;\n  RETURN $NewOrder;\nEND;",
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
		SeeAlso: []string{"microflow.variables", "microflow.error-handling", "microflow.splits"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.splits",
		Summary: "CASE (enum split) and SPLIT TYPE (object type split)",
		Keywords: []string{
			"case", "when", "then", "end case", "enum split", "enumeration split",
			"split type", "end split", "type split", "inheritance split",
			"specialization", "cast", "empty", "switch", "decision",
		},
		Syntax: "CASE $EnumVarOrAttr                  -- branches on an ENUMERATION\n" +
			"  WHEN Value1, Value2 THEN\n" +
			"    ...\n" +
			"  WHEN (empty) THEN                  -- required (MDL056 / CE0079)\n" +
			"    ...\n" +
			"END CASE;                            -- no ELSE (MDL008)\n\n" +
			"SPLIT TYPE $ObjectVar                -- branches on the RUNTIME TYPE\n" +
			"  WHEN Module.Specialization THEN\n" +
			"    CAST $Specific;\n" +
			"    ...\n" +
			"  WHEN Module.BaseEntity THEN        -- required too (CE0090)\n" +
			"    ...\n" +
			"  WHEN (empty) THEN                  -- the NULL-object branch, not a default\n" +
			"    ...\n" +
			"END SPLIT;\n\n" +
			"Both take `WHEN ... THEN` branches. `SPLIT TYPE` needs one per subtype AND\n" +
			"the base entity; its `(empty)` branch is the null object and covers no type.\n" +
			"Legacy `CASE Module.Entity` / `ELSE` inside SPLIT TYPE still parse (MDL065).",
		Example: "CASE $Order/Status\n  WHEN Draft, Submitted THEN\n    LOG INFO 'Not shipped yet';\n  WHEN Approved THEN\n    LOG INFO 'Ready to ship';\n  WHEN (empty) THEN\n    LOG INFO 'No status';\nEND CASE;\n\nSPLIT TYPE $Animal\n  WHEN Zoo.Dog THEN\n    LOG INFO 'woof';\n  WHEN Zoo.Cat THEN\n    LOG INFO 'meow';\n  WHEN Zoo.Animal THEN\n    LOG INFO 'some other animal';\n  WHEN (empty) THEN\n    LOG INFO 'no animal at all';\nEND SPLIT;",
		SeeAlso: []string{"microflow.control-flow", "microflow.variables"},
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
		Syntax: "$Obj = CREATE Module.Entity (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"CHANGE $Obj (Attr = value) [COMMIT [WITHOUT EVENTS]] [REFRESH];\n" +
			"COMMIT $Obj [WITHOUT EVENTS] [REFRESH];\n" +
			"DELETE $Obj [REFRESH];\n" +
			"ROLLBACK $Obj [REFRESH];\n\n" +
			"-- An omitted modifier always means Mendix's own default, so a bare\n" +
			"-- statement is exactly what Studio Pro gives you for a fresh activity:\n" +
			"--\n" +
			"--   Activity   With events        Refresh in client\n" +
			"--   CREATE     (Commit: No)       No\n" +
			"--   CHANGE     (Commit: No)       No\n" +
			"--   COMMIT     Yes                No\n" +
			"--   DELETE     n/a                No\n" +
			"--   ROLLBACK   n/a                No\n" +
			"--\n" +
			"-- COMMIT is the one whose default is ON, so WITHOUT EVENTS is the form\n" +
			"-- that changes anything; WITH EVENTS parses and means the default. The\n" +
			"-- COMMIT modifier on CREATE/CHANGE is the activity's Commit setting\n" +
			"-- (omitted = No), not the standalone COMMIT $Obj activity.",
		Example: "$NewOrder = CREATE MyModule.Order (\n  OrderNumber = 'ORD-001',\n  Quantity = $Quantity,\n  CreateDate = [%CurrentDateTime%]\n) COMMIT;\n\nCHANGE $NewOrder (MyModule.Order_Customer = $Customer) COMMIT REFRESH;\nCHANGE $Draft (Status = 'Imported') COMMIT WITHOUT EVENTS;\n\nCOMMIT $NewOrder;                  -- runs the commit event handlers\nCOMMIT $Staging WITHOUT EVENTS;    -- bulk import: skip them deliberately\nCOMMIT $NewOrder REFRESH;          -- and repaint it on the open page\n\nDELETE $OldOrder REFRESH;\nROLLBACK $DraftOrder;",
		SeeAlso: []string{"microflow.retrieve", "microflow.variables"},
	})

	Register(SyntaxFeature{
		Path:    "microflow.list-operations",
		Summary: "List manipulation — HEAD, TAIL, FIND, FILTER, SORT, UNION, RANGE, aggregates",
		Keywords: []string{
			"list", "head", "tail", "find", "filter", "sort",
			"union", "intersect", "subtract", "count", "sum",
			"average", "aggregate", "add to list", "remove from list",
			"create list",
			// RANGE was authorable but absent from this topic, so the paging
			// form could not be discovered from the CLI at all (issue #966).
			"range", "paging", "pagination", "offset", "limit", "amount", "page",
		},
		Syntax:  "$List = CREATE LIST OF Module.Entity;\nADD $Item TO $List;\nREMOVE $Item FROM $List;\n$Result = HEAD($List);\n$Result = TAIL($List);\n$Result = FIND($List, condition);\n$Result = FILTER($List, condition);\n$Result = SORT($List, attr ASC);\n$Result = UNION($L1, $L2);\n$Result = INTERSECT($L1, $L2);\n$Result = SUBTRACT($L1, $L2);\n$Result = RANGE($List, offset, amount);\n$Result = RANGE($List, offset);\n$Count = COUNT($List);\n$Sum = SUM($List.Attr);\n$Avg = AVERAGE($List.Attr);\n\n-- RANGE takes OFFSET first, then AMOUNT, and needs at least ONE of them:\n--   RANGE($L, $Offset, $Amount)  page: skip $Offset, take $Amount\n--   RANGE($L, 0, $Amount)        first $Amount\n--   RANGE($L, $Offset)           skip $Offset, take the rest\n-- RANGE($L) with no bound is CE6520 at build time (mxcli check: MDL068).",
		Example: "$AllOrders = CREATE LIST OF MyModule.Order;\nADD $NewOrder TO $AllOrders;\n$First = HEAD($AllOrders);\n$Pending = FILTER($AllOrders, Status = 'Pending');\n$Sorted = SORT($Pending, CreateDate DESC);\n$Page = RANGE($Sorted, $Offset, $PageSize);\n$Total = SUM($AllOrders.Amount);",
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
			"call microflow", "call java action", "invoke", "in queue", "queued call", "background execution",
			"sub-microflow", "java action", "parameter passing",
		},
		Syntax:  "$Result = CALL MICROFLOW Module.Name (Param = value) [IN QUEUE Module.Queue];\n$Result = CALL JAVA ACTION Module.Name (Param = value) [IN QUEUE Module.Queue];",
		Example: "$IsValid = CALL MICROFLOW MyModule.ValidateOrder (\n  Order = $NewOrder\n);\n\n$Token = CALL JAVA ACTION MyModule.GenerateToken (\n  UserId = $User/Id\n);\n\n-- Run the call on a task queue (background execution). The queue must\n-- exist; a queued CALL JAVA ACTION must return Nothing, or the build fails\n-- with CE7038.\nCALL MICROFLOW MyModule.ACT_Refresh () IN QUEUE MyModule.RefreshQueue;\nCALL JAVA ACTION MyModule.RefreshData (Url = $Url) IN QUEUE MyModule.RefreshQueue;",
		SeeAlso: []string{"java-action", "microflow.create", "queue"},
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
		Path:    "microflow.rule",
		Summary: "CREATE RULE — reusable decision logic, callable only from a decision",
		Keywords: []string{
			"rule", "create rule", "list rules", "describe rule", "drop rule",
			"business rule", "decision logic", "reusable condition",
		},
		Syntax: "CREATE [OR MODIFY] RULE Module.Name ($Param: Type)\n" +
			"RETURNS Boolean | enum Module.Enum\n[FOLDER 'path']\nBEGIN\n  <statements>\nEND;\n\n" +
			"LIST RULES [IN Module];\nDESCRIBE RULE Module.Name;\n" +
			"DROP RULE Module.Name;\nMOVE RULE Module.Name TO FOLDER 'path';\n\n" +
			"A rule is called from a decision and nowhere else:\n" +
			"  IF Module.Rule_Name(Param = $Value) THEN ... END IF;\n\n" +
			"The return type is mandatory and must be Boolean or an enumeration\n" +
			"(mxbuild: CE0103/CE0139). A rule may not create, change, delete, commit\n" +
			"or roll back objects, talk to the client, or call a web service\n" +
			"(mxbuild: CE0009) — `mxcli check` refuses these before the build.\n\n" +
			"There is no GRANT EXECUTE ON RULE: a rule is not independently callable,\n" +
			"so its document carries no module-role security.",
		Example: "create or modify rule Sales.Rule_IsSolvent ($pCustomer: Sales.Customer)\n" +
			"returns Boolean\nfolder 'Rules'\nbegin\n  return $pCustomer/Balance >= 0;\nend\n/\n\n" +
			"create or modify microflow Sales.MF_Screen ($pCustomer: Sales.Customer)\nbegin\n" +
			"  if Sales.Rule_IsSolvent(pCustomer = $pCustomer) then\n    return;\n" +
			"  else\n    return;\n  end if;\nend\n/",
		SeeAlso: []string{"microflow.create", "microflow.control-flow"},
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
		Summary: "Canvas layout annotations — @position, @start, @anchor, @curve, @caption, @color",
		Keywords: []string{
			"position", "start", "anchor", "curve", "layout", "canvas",
			"annotation", "caption", "color", "excluded", "bezier",
		},
		Syntax: "@position(x, y)                       -- the activity's centre point\n" +
			"@start(x, y)                          -- the start event, on the FIRST statement\n" +
			"@anchor(from: right, to: left)        -- which SIDE each end of the outgoing flow attaches to\n" +
			"@curve(from: (40, -90), to: (-40, 90))  -- the flow's bezier control vectors\n" +
			"@merge(x, y)                          -- the implicit merge that closes a split\n" +
			"@caption 'text'\n@color Green\n@annotation 'a note'\n@excluded\n\n" +
			"An unrecognised @name is an error (MDL059): it would parse and do nothing,\n" +
			"so a typo of @position would silently discard the layout.\n\n" +
			"Mendix stores no waypoints — a flow's shape is two control vectors, each a\n" +
			"pixel offset from its end of the line. (0, 0) at both ends is straight.\n" +
			"@position on a split belongs to the SPLIT, so its end-if join has its own\n" +
			"annotation. Container Size is still computed, not authorable.\n\n" +
			"@start and @merge position the two nodes that have no statement of their\n" +
			"own, so each is written on the statement it belongs to. Omit @start and the\n" +
			"start is placed one spacing unit left of the first activity, on its centre\n" +
			"line — and a rewrite MOVES it to follow the activities. A start that is not\n" +
			"at that derived spot was put there by hand: it survives a rewrite, and\n" +
			"DESCRIBE emits @start for it so the description round-trips exactly.",
		Example: "create microflow MyModule.ACT_Flow ($In: String)\nreturns String as $Out\nbegin\n" +
			"  @start(145, 100)\n  @position(200, 100)\n  @anchor(from: bottom, to: top)\n" +
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
