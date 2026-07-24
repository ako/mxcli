// SPDX-License-Identifier: Apache-2.0

package catalog

// CatalogSchemaVersion is bumped whenever the schema in createTables changes
// in a way that requires existing on-disk caches to be regenerated.
//
// History:
//
//	2 — split each domain table into <name>_data + <name> view that JOINs
//	    snapshots, removing the denormalized ProjectName / SnapshotDate /
//	    SnapshotSource / SourceId / SourceBranch / SourceRevision columns
//	    from every row (issue #576).
//	1 — initial flat schema with denormalized snapshot columns on every row.
const CatalogSchemaVersion = "8"

// MetaSchemaVersion is the catalog_meta key that records the schema version
// the cache was built against.
const MetaSchemaVersion = "schema_version"

// createTables creates all catalog tables in the SQLite database.
//
// Each domain table is split into two pieces:
//
//   - `<name>_data` is the storage table — the columns that are genuinely
//     owned by this entity, plus `ProjectId` and `SnapshotId` foreign keys.
//   - `<name>` is a view that JOINs `snapshots` to surface
//     `ProjectName`, `SnapshotDate`, `SnapshotSource`, `SourceId`,
//     `SourceBranch`, and `SourceRevision` (whichever the table historically
//     exposed). This preserves the column shape that existing queries
//     — including the `objects` UNION view and ad-hoc user SQL — expect.
//
// Builders INSERT into `<name>_data`. Readers should use `<name>`.
//
// Tables that were already clean (only `ProjectId` + `SnapshotId`, no
// denormalized columns) keep their original shape:
// navigation_menu_items, navigation_role_homes, role_mappings, refs,
// permissions, constant_values.
func (c *Catalog) createTables() error {
	schemas := []string{
		// ----- Metadata + lookup tables (source of truth) -----

		`CREATE TABLE IF NOT EXISTS catalog_meta (
			Key TEXT PRIMARY KEY,
			Value TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS projects (
			ProjectId TEXT PRIMARY KEY,
			ProjectName TEXT,
			MendixVersion TEXT,
			CreatedDate TEXT,
			LastSnapshotDate TEXT,
			SnapshotCount INTEGER DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS snapshots (
			SnapshotId TEXT PRIMARY KEY,
			SnapshotName TEXT,
			ProjectId TEXT,
			ProjectName TEXT,
			SnapshotDate TEXT,
			SnapshotSource TEXT,
			SourceId TEXT,
			SourceBranch TEXT,
			SourceRevision TEXT,
			ObjectCount INTEGER DEFAULT 0,
			IsActive INTEGER DEFAULT 0
		)`,

		// ----- Domain tables: <name>_data + <name> view -----

		// modules
		`CREATE TABLE IF NOT EXISTS modules_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			Source TEXT DEFAULT '',
			AppStoreVersion TEXT,
			AppStoreGuid TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("modules"),

		// entities
		`CREATE TABLE IF NOT EXISTS entities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityType TEXT,
			Description TEXT,
			Generalization TEXT,
			AttributeCount INTEGER DEFAULT 0,
			AssociationCount INTEGER DEFAULT 0,
			AccessRuleCount INTEGER DEFAULT 0,
			ValidationRuleCount INTEGER DEFAULT 0,
			HasEventHandlers INTEGER DEFAULT 0,
			IsExternal INTEGER DEFAULT 0,
			ExternalService TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("entities"),

		// associations
		`CREATE TABLE IF NOT EXISTS associations_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			FromEntity TEXT,
			ToEntity TEXT,
			AssociationType TEXT,
			Owner TEXT,
			StorageFormat TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("associations"),

		// attributes
		`CREATE TABLE IF NOT EXISTS attributes_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			EntityId TEXT,
			EntityQualifiedName TEXT,
			ModuleName TEXT,
			DataType TEXT,
			Length INTEGER,
			IsUnique INTEGER DEFAULT 0,
			IsRequired INTEGER DEFAULT 0,
			DefaultValue TEXT,
			IsCalculated INTEGER DEFAULT 0,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("attributes"),

		// microflows (includes nanoflows; filtered by view below)
		`CREATE TABLE IF NOT EXISTS microflows_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			MicroflowType TEXT,
			Description TEXT,
			ReturnType TEXT,
			ParameterCount INTEGER DEFAULT 0,
			ActivityCount INTEGER DEFAULT 0,
			Complexity INTEGER DEFAULT 1,
			Excluded BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("microflows"),

		// nanoflows view (filtered subset of microflows view)
		`CREATE VIEW IF NOT EXISTS nanoflows AS
			SELECT * FROM microflows WHERE MicroflowType = 'NANOFLOW'`,

		// pages
		`CREATE TABLE IF NOT EXISTS pages_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Title TEXT,
			URL TEXT,
			LayoutRef TEXT,
			Description TEXT,
			ParameterCount INTEGER DEFAULT 0,
			WidgetCount INTEGER DEFAULT 0,
			Excluded BOOLEAN DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("pages"),

		// snippets
		`CREATE TABLE IF NOT EXISTS snippets_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ParameterCount INTEGER DEFAULT 0,
			WidgetCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("snippets"),

		// building blocks (read-only reusable widget compositions)
		`CREATE TABLE IF NOT EXISTS building_blocks_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			DisplayName TEXT,
			Platform TEXT,
			Category TEXT,
			WidgetCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("building_blocks"),

		// layouts
		`CREATE TABLE IF NOT EXISTS layouts_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			LayoutType TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("layouts"),

		// enumerations
		`CREATE TABLE IF NOT EXISTS enumerations_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ValueCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("enumerations"),

		// java_actions
		`CREATE TABLE IF NOT EXISTS java_actions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Documentation TEXT,
			ExportLevel TEXT,
			ReturnType TEXT,
			ParameterCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("java_actions"),

		// javascript_actions
		`CREATE TABLE IF NOT EXISTS javascript_actions_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("javascript_actions"),

		// image_collections
		`CREATE TABLE IF NOT EXISTS image_collections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("image_collections"),

		// data_transformers
		`CREATE TABLE IF NOT EXISTS data_transformers_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("data_transformers"),

		// agent-editor documents (CustomBlobDocuments, discriminated by
		// CustomDocumentType into agent / model / knowledge base / MCP service).
		`CREATE TABLE IF NOT EXISTS agents_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("agents"),
		`CREATE TABLE IF NOT EXISTS ai_models_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("ai_models"),
		`CREATE TABLE IF NOT EXISTS knowledge_bases_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("knowledge_bases"),
		`CREATE TABLE IF NOT EXISTS consumed_mcp_services_data (
			Id TEXT PRIMARY KEY, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, Description TEXT, ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("consumed_mcp_services"),

		// activities
		`CREATE TABLE IF NOT EXISTS activities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			Caption TEXT,
			ActivityType TEXT,
			Sequence INTEGER DEFAULT 0,
			MicroflowId TEXT,
			MicroflowQualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityRef TEXT,
			ActionType TEXT,
			ServiceRef TEXT,
			ActionRef TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("activities"),

		// widget_definitions — widgets AVAILABLE in the project (.mpk / .def.json)
		`CREATE TABLE IF NOT EXISTS widget_definitions_data (
			WidgetId TEXT PRIMARY KEY,
			MdlName TEXT,
			DisplayName TEXT,
			WidgetKind TEXT,
			Version TEXT,
			MpkPath TEXT,
			DefPath TEXT,
			PropertyCount INTEGER,
			ChildSlotCount INTEGER,
			ObjectListCount INTEGER,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widget_definitions"),

		// widget_definition_properties — one row per property / child slot / object list
		`CREATE TABLE IF NOT EXISTS widget_definition_properties_data (
			Id TEXT PRIMARY KEY,
			WidgetId TEXT,
			PropertyKey TEXT,
			Kind TEXT,
			Type TEXT,
			MdlKeyword TEXT,
			Required INTEGER,
			DefaultValue TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widget_definition_properties"),

		// widgets — one row per widget INSTANCE (use site)
		`CREATE TABLE IF NOT EXISTS widgets_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			WidgetType TEXT,
			ContainerId TEXT,
			ContainerQualifiedName TEXT,
			ContainerType TEXT,
			ModuleName TEXT,
			Folder TEXT,
			EntityRef TEXT,
			AttributeRef TEXT,
			MicroflowRef TEXT,
			NanoflowRef TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("widgets"),

		// xpath_expressions
		`CREATE TABLE IF NOT EXISTS xpath_expressions_data (
			Id TEXT PRIMARY KEY,
			DocumentType TEXT,
			DocumentId TEXT,
			DocumentQualifiedName TEXT,
			ComponentType TEXT,
			ComponentId TEXT,
			ComponentName TEXT,
			XPathExpression TEXT,
			XPathAST TEXT,
			TargetEntity TEXT,
			ReferencedEntities TEXT,
			IsParameterized INTEGER DEFAULT 0,
			UsageType TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("xpath_expressions"),

		// odata_clients
		`CREATE TABLE IF NOT EXISTS odata_clients_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Version TEXT,
			ODataVersion TEXT,
			MetadataUrl TEXT,
			Validated INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("odata_clients"),

		// odata_services
		`CREATE TABLE IF NOT EXISTS odata_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Path TEXT,
			Version TEXT,
			ODataVersion TEXT,
			EntitySetCount INTEGER DEFAULT 0,
			AuthenticationTypes TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("odata_services"),

		// workflows
		`CREATE TABLE IF NOT EXISTS workflows_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			ExportLevel TEXT,
			ParameterEntity TEXT,
			ActivityCount INTEGER DEFAULT 0,
			UserTaskCount INTEGER DEFAULT 0,
			MicroflowCallCount INTEGER DEFAULT 0,
			DecisionCount INTEGER DEFAULT 0,
			DueDate TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("workflows"),

		// business_event_services
		`CREATE TABLE IF NOT EXISTS business_event_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Documentation TEXT,
			ServiceName TEXT,
			EventNamePrefix TEXT,
			MessageCount INTEGER DEFAULT 0,
			PublishCount INTEGER DEFAULT 0,
			SubscribeCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("business_event_services"),

		// navigation_profiles
		`CREATE TABLE IF NOT EXISTS navigation_profiles_data (
			ProfileName TEXT PRIMARY KEY,
			Kind TEXT,
			IsNative INTEGER DEFAULT 0,
			HomePage TEXT,
			HomePageType TEXT,
			LoginPage TEXT,
			NotFoundPage TEXT,
			MenuItemCount INTEGER DEFAULT 0,
			RoleBasedHomeCount INTEGER DEFAULT 0,
			OfflineEntityCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("navigation_profiles"),

		// Already-clean tables (no denormalized columns) — kept as plain tables.
		`CREATE TABLE IF NOT EXISTS navigation_menu_items (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ProfileName TEXT NOT NULL,
			ItemPath TEXT NOT NULL,
			Depth INTEGER DEFAULT 0,
			Caption TEXT,
			ActionType TEXT,
			TargetPage TEXT,
			TargetMicroflow TEXT,
			SubItemCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS navigation_role_homes (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ProfileName TEXT NOT NULL,
			UserRole TEXT NOT NULL,
			Page TEXT,
			Microflow TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// rest_clients
		`CREATE TABLE IF NOT EXISTS rest_clients_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			BaseUrl TEXT,
			AuthScheme TEXT,
			OperationCount INTEGER DEFAULT 0,
			Documentation TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("rest_clients"),

		// rest_operations — partial denormalization (ProjectId, SnapshotId, SnapshotDate, SnapshotSource)
		`CREATE TABLE IF NOT EXISTS rest_operations_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			Name TEXT,
			HttpMethod TEXT,
			Path TEXT,
			ParameterCount INTEGER DEFAULT 0,
			HasBody INTEGER DEFAULT 0,
			ResponseType TEXT,
			Timeout INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("rest_operations"),

		// published_rest_services
		`CREATE TABLE IF NOT EXISTS published_rest_services_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Path TEXT,
			Version TEXT,
			ServiceName TEXT,
			ResourceCount INTEGER DEFAULT 0,
			OperationCount INTEGER DEFAULT 0,
			Documentation TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("published_rest_services"),

		// published_rest_operations — partial denormalization
		`CREATE TABLE IF NOT EXISTS published_rest_operations_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ResourceName TEXT,
			HttpMethod TEXT,
			Path TEXT,
			Summary TEXT,
			Microflow TEXT,
			Deprecated INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("published_rest_operations"),

		// external_entities — has ProjectName in addition to SnapshotDate/Source
		`CREATE TABLE IF NOT EXISTS external_entities_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			ServiceName TEXT,
			EntitySet TEXT,
			RemoteName TEXT,
			Countable INTEGER DEFAULT 0,
			Creatable INTEGER DEFAULT 0,
			Deletable INTEGER DEFAULT 0,
			Updatable INTEGER DEFAULT 0,
			AttributeCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("external_entities"),

		// external_actions
		`CREATE TABLE IF NOT EXISTS external_actions_data (
			Id TEXT PRIMARY KEY,
			ServiceName TEXT,
			ActionName TEXT,
			ModuleName TEXT,
			UsageCount INTEGER DEFAULT 0,
			CallerNames TEXT,
			ParameterNames TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("external_actions"),

		// business_events — partial denormalization
		`CREATE TABLE IF NOT EXISTS business_events_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ChannelName TEXT,
			MessageName TEXT,
			CanPublish INTEGER DEFAULT 0,
			CanSubscribe INTEGER DEFAULT 0,
			AttributeCount INTEGER DEFAULT 0,
			Entity TEXT,
			PublishMicroflow TEXT,
			SubscribeMicroflow TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("business_events"),

		// contract_entities — partial denormalization
		`CREATE TABLE IF NOT EXISTS contract_entities_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			EntityName TEXT,
			EntitySetName TEXT,
			KeyProperties TEXT,
			PropertyCount INTEGER DEFAULT 0,
			NavigationCount INTEGER DEFAULT 0,
			Summary TEXT,
			Description TEXT,
			ModuleName TEXT,
			UsedByExternalEntity TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_entities"),

		// contract_actions
		`CREATE TABLE IF NOT EXISTS contract_actions_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ActionName TEXT,
			IsBound INTEGER DEFAULT 0,
			ParameterCount INTEGER DEFAULT 0,
			ReturnType TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_actions"),

		// contract_messages
		`CREATE TABLE IF NOT EXISTS contract_messages_data (
			Id TEXT PRIMARY KEY,
			ServiceId TEXT,
			ServiceQualifiedName TEXT,
			ChannelName TEXT,
			OperationType TEXT,
			MessageName TEXT,
			Title TEXT,
			ContentType TEXT,
			PropertyCount INTEGER DEFAULT 0,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithSnapshotDateSource("contract_messages"),

		// database_connections
		`CREATE TABLE IF NOT EXISTS database_connections_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			DatabaseType TEXT,
			QueryCount INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("database_connections"),

		// jar_dependencies
		`CREATE TABLE IF NOT EXISTS jar_dependencies_data (
			Id TEXT PRIMARY KEY,
			ModuleName TEXT,
			GroupId TEXT,
			ArtifactId TEXT,
			Coordinate TEXT,
			Version TEXT,
			IsIncluded INTEGER DEFAULT 1,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithFullSnapshot("jar_dependencies"),

		// role_mappings, refs, permissions, constant_values — already clean
		`CREATE TABLE IF NOT EXISTS role_mappings (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			UserRoleName TEXT NOT NULL,
			ModuleRoleName TEXT NOT NULL,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS refs (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			SourceType TEXT NOT NULL,
			SourceId TEXT NOT NULL,
			SourceName TEXT NOT NULL,
			TargetType TEXT NOT NULL,
			TargetId TEXT,
			TargetName TEXT NOT NULL,
			RefKind TEXT NOT NULL,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS permissions (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ModuleRoleName TEXT NOT NULL,
			ElementType TEXT NOT NULL,
			ElementName TEXT NOT NULL,
			MemberName TEXT,
			AccessType TEXT NOT NULL,
			XPathConstraint TEXT,
			ModuleName TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// constants — partial denormalization
		`CREATE TABLE IF NOT EXISTS constants_data (
			Id TEXT PRIMARY KEY,
			Name TEXT,
			QualifiedName TEXT,
			ModuleName TEXT,
			Folder TEXT,
			Description TEXT,
			DataType TEXT,
			DefaultValue TEXT,
			ExposedToClient INTEGER DEFAULT 0,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("constants"),

		`CREATE TABLE IF NOT EXISTS constant_values (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			ConstantName TEXT NOT NULL,
			ConfigurationName TEXT NOT NULL,
			Value TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,

		// json_structures — partial denormalization
		`CREATE TABLE IF NOT EXISTS json_structures_data (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			ElementCount INTEGER DEFAULT 0,
			HasSnippet INTEGER DEFAULT 0,
			Documentation TEXT,
			ExportLevel TEXT,
			Folder TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("json_structures"),

		// import_mappings
		`CREATE TABLE IF NOT EXISTS import_mappings_data (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			SchemaSource TEXT,
			ElementCount INTEGER DEFAULT 0,
			Documentation TEXT,
			Folder TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("import_mappings"),

		// export_mappings
		`CREATE TABLE IF NOT EXISTS export_mappings_data (
			Id INTEGER PRIMARY KEY AUTOINCREMENT,
			Name TEXT NOT NULL,
			QualifiedName TEXT NOT NULL,
			ModuleName TEXT NOT NULL,
			SchemaSource TEXT,
			NullValueOption TEXT,
			ElementCount INTEGER DEFAULT 0,
			Documentation TEXT,
			Folder TEXT,
			ProjectId TEXT,
			SnapshotId TEXT
		)`,
		viewWithProjectNameAndSnapshotDateSource("export_mappings"),

		// Objects view - union of all object types.
		// Reads through the per-table views so it picks up ProjectName /
		// SnapshotDate / SnapshotSource from snapshots automatically.
		`CREATE VIEW IF NOT EXISTS objects AS
			SELECT Id, 'MODULE' as ObjectType, Name, QualifiedName, '' as ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM modules
			UNION ALL
			SELECT Id, 'ENTITY' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM entities
			UNION ALL
			SELECT Id, 'ASSOCIATION' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM associations
			UNION ALL
			SELECT Id, 'MICROFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM microflows WHERE MicroflowType = 'MICROFLOW'
			UNION ALL
			SELECT Id, 'NANOFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM microflows WHERE MicroflowType = 'NANOFLOW'
			UNION ALL
			SELECT Id, 'PAGE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM pages
			UNION ALL
			SELECT Id, 'SNIPPET' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM snippets
			UNION ALL
			SELECT Id, 'LAYOUT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM layouts
			UNION ALL
			SELECT Id, 'ENUMERATION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM enumerations
			UNION ALL
			SELECT Id, 'CONSTANT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM constants
			UNION ALL
			SELECT Id, 'JAVA_ACTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM java_actions
			UNION ALL
			SELECT Id, 'JAVASCRIPT_ACTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM javascript_actions
			UNION ALL
			SELECT Id, 'IMAGE_COLLECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM image_collections
			UNION ALL
			SELECT Id, 'DATA_TRANSFORMER' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM data_transformers
			UNION ALL
			SELECT Id, 'AGENT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM agents
			UNION ALL
			SELECT Id, 'AI_MODEL' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM ai_models
			UNION ALL
			SELECT Id, 'KNOWLEDGE_BASE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM knowledge_bases
			UNION ALL
			SELECT Id, 'CONSUMED_MCP_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM consumed_mcp_services
			UNION ALL
			SELECT Id, 'ODATA_CLIENT' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM odata_clients
			UNION ALL
			SELECT Id, 'ODATA_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM odata_services
			UNION ALL
			SELECT Id, 'WORKFLOW' as ObjectType, Name, QualifiedName, ModuleName, Folder, Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM workflows
			UNION ALL
			SELECT Id, 'BUSINESS_EVENT_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM business_event_services
			UNION ALL
			SELECT Id, 'DATABASE_CONNECTION' as ObjectType, Name, QualifiedName, ModuleName, Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM database_connections
			UNION ALL
			SELECT Id, 'REST_CLIENT' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM rest_clients
			UNION ALL
			SELECT Id, 'PUBLISHED_REST_SERVICE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM published_rest_services
			UNION ALL
			SELECT Id, 'EXTERNAL_ENTITY' as ObjectType, Name, QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM external_entities
			UNION ALL
			SELECT Id, 'EXTERNAL_ACTION' as ObjectType, ActionName as Name, ServiceName || '.' || ActionName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM external_actions
			UNION ALL
			SELECT Id, 'BUSINESS_EVENT' as ObjectType, MessageName as Name, ServiceQualifiedName || '.' || MessageName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, SnapshotId || '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM business_events
			UNION ALL
			SELECT Id, 'CONTRACT_ENTITY' as ObjectType, EntityName as Name, ServiceQualifiedName || '.' || EntityName as QualifiedName, ModuleName, '' as Folder, Summary as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_entities
			UNION ALL
			SELECT Id, 'CONTRACT_ACTION' as ObjectType, ActionName as Name, ServiceQualifiedName || '.' || ActionName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_actions
			UNION ALL
			SELECT Id, 'CONTRACT_MESSAGE' as ObjectType, MessageName as Name, ServiceQualifiedName || '.' || MessageName as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, '' as ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM contract_messages
			UNION ALL
			SELECT CAST(Id AS TEXT), 'JSON_STRUCTURE' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM json_structures
			UNION ALL
			SELECT CAST(Id AS TEXT), 'IMPORT_MAPPING' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM import_mappings
			UNION ALL
			SELECT CAST(Id AS TEXT), 'EXPORT_MAPPING' as ObjectType, Name, QualifiedName, ModuleName, Folder, Documentation as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM export_mappings
			UNION ALL
			SELECT Id, 'JAR_DEPENDENCY' as ObjectType, Coordinate as Name, ModuleName || '.' || Coordinate as QualifiedName, ModuleName, '' as Folder, '' as Description,
				ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource
			FROM jar_dependencies`,

		// FTS5 virtual tables for full-text search
		`CREATE VIRTUAL TABLE IF NOT EXISTS strings USING fts5(
			QualifiedName,
			ObjectType,
			StringValue,
			StringContext,
			Language,
			ElementId,
			ModuleName
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS source USING fts5(
			QualifiedName,
			ObjectType,
			SourceText,
			ModuleName
		)`,

		// Indexes for common queries — target the underlying *_data tables.
		`CREATE INDEX IF NOT EXISTS idx_modules_name ON modules_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_name ON entities_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_module ON entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_microflows_name ON microflows_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_microflows_module ON microflows_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_name ON pages_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_pages_module ON pages_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_layouts_name ON layouts_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_layouts_module ON layouts_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_microflow ON activities_data(MicroflowId)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_type ON activities_data(ActivityType)`,
		`CREATE INDEX IF NOT EXISTS idx_widgets_container ON widgets_data(ContainerId)`,
		`CREATE INDEX IF NOT EXISTS idx_widgets_type ON widgets_data(WidgetType)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_defs_kind ON widget_definitions_data(WidgetKind)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_defs_mdlname ON widget_definitions_data(MdlName)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_def_props_widget ON widget_definition_properties_data(WidgetId)`,
		`CREATE INDEX IF NOT EXISTS idx_widget_def_props_kind ON widget_definition_properties_data(Kind)`,
		`CREATE INDEX IF NOT EXISTS idx_xpath_document ON xpath_expressions_data(DocumentId)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_source ON refs(SourceType, SourceName)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_target ON refs(TargetType, TargetName)`,
		`CREATE INDEX IF NOT EXISTS idx_refs_kind ON refs(RefKind)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_entity ON attributes_data(EntityId)`,
		`CREATE INDEX IF NOT EXISTS idx_attributes_entity_qname ON attributes_data(EntityQualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_java_actions_name ON java_actions_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_java_actions_module ON java_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_clients_name ON odata_clients_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_clients_module ON odata_clients_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_services_name ON odata_services_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_odata_services_module ON odata_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_workflows_name ON workflows_data(QualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_workflows_module ON workflows_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_be_services_name ON business_event_services_data(QualifiedName)`,
		`CREATE INDEX IF NOT EXISTS idx_be_services_module ON business_event_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_user_role ON role_mappings(UserRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_module_role ON role_mappings(ModuleRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_role_mappings_module ON role_mappings(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_role ON permissions(ModuleRoleName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_element ON permissions(ElementType, ElementName)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_access ON permissions(AccessType)`,
		`CREATE INDEX IF NOT EXISTS idx_permissions_module ON permissions(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_profile ON navigation_menu_items(ProfileName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_target_page ON navigation_menu_items(TargetPage)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_menu_items_target_mf ON navigation_menu_items(TargetMicroflow)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_role_homes_profile ON navigation_role_homes(ProfileName)`,
		`CREATE INDEX IF NOT EXISTS idx_nav_role_homes_role ON navigation_role_homes(UserRole)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_clients_name ON rest_clients_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_clients_module ON rest_clients_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_operations_service ON rest_operations_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_rest_operations_method ON rest_operations_data(HttpMethod)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_name ON published_rest_services_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_module ON published_rest_services_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_published_rest_ops_service ON published_rest_operations_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_external_entities_service ON external_entities_data(ServiceName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_entities_module ON external_entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_actions_service ON external_actions_data(ServiceName)`,
		`CREATE INDEX IF NOT EXISTS idx_external_actions_module ON external_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_business_events_service ON business_events_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_business_events_module ON business_events_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_entities_service ON contract_entities_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_entities_module ON contract_entities_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_actions_service ON contract_actions_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_actions_module ON contract_actions_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_messages_service ON contract_messages_data(ServiceId)`,
		`CREATE INDEX IF NOT EXISTS idx_contract_messages_module ON contract_messages_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_constants_name ON constants_data(Name)`,
		`CREATE INDEX IF NOT EXISTS idx_constants_module ON constants_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_constant_values_constant ON constant_values(ConstantName)`,
		`CREATE INDEX IF NOT EXISTS idx_constant_values_config ON constant_values(ConfigurationName)`,
		`CREATE INDEX IF NOT EXISTS idx_jar_deps_module ON jar_dependencies_data(ModuleName)`,
		`CREATE INDEX IF NOT EXISTS idx_jar_deps_coord ON jar_dependencies_data(Coordinate)`,

		// --- Graph-analysis views (read the refs graph; populated by REFRESH
		// CATALOG FULL). Module is derived from the qualified-name prefix, NOT by
		// joining entities — that would drop non-entity targets. ---

		// graph_god_nodes — degree centrality (most depended-upon / highest fan-out).
		`CREATE VIEW IF NOT EXISTS graph_god_nodes AS
			WITH deg AS (
				SELECT TargetName AS Asset, COUNT(*) AS InDeg, 0 AS OutDeg
				FROM refs WHERE TargetName != '' GROUP BY TargetName
				UNION ALL
				SELECT SourceName AS Asset, 0 AS InDeg, COUNT(*) AS OutDeg
				FROM refs WHERE SourceName != '' GROUP BY SourceName
			)
			SELECT d.Asset,
				(SELECT ObjectType FROM objects WHERE QualifiedName = d.Asset LIMIT 1) AS ObjectType,
				CASE WHEN instr(d.Asset, '.') > 0 THEN substr(d.Asset, 1, instr(d.Asset, '.') - 1) ELSE d.Asset END AS ModuleName,
				SUM(d.InDeg) AS InDegree, SUM(d.OutDeg) AS OutDegree, SUM(d.InDeg) + SUM(d.OutDeg) AS Degree,
				MAX(gc.PageRank) AS PageRank, MAX(gc.Betweenness) AS Betweenness
			FROM deg d
			LEFT JOIN graph_centrality_data gc ON gc.AssetName = d.Asset
			GROUP BY d.Asset`,

		// graph_module_coupling — cross-module edges ("surprise edges").
		`CREATE VIEW IF NOT EXISTS graph_module_coupling AS
			SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SourceModule,
				substr(TargetName, 1, instr(TargetName, '.') - 1) AS TargetModule,
				COUNT(*) AS Edges,
				group_concat(DISTINCT RefKind) AS RefKinds
			FROM refs
			WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
				AND substr(SourceName, 1, instr(SourceName, '.') - 1) != substr(TargetName, 1, instr(TargetName, '.') - 1)
			GROUP BY SourceModule, TargetModule`,

		// graph_module_cohesion — intra- vs inter-module edge ratio per module.
		`CREATE VIEW IF NOT EXISTS graph_module_cohesion AS
			WITH e AS (
				SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SrcMod,
					CASE WHEN substr(SourceName, 1, instr(SourceName, '.') - 1) = substr(TargetName, 1, instr(TargetName, '.') - 1)
						THEN 1 ELSE 0 END AS Intra
				FROM refs WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
			)
			SELECT SrcMod AS ModuleName,
				SUM(Intra) AS IntraEdges, SUM(1 - Intra) AS InterEdges,
				round(100.0 * SUM(Intra) / COUNT(*), 1) AS CohesionPct
			FROM e GROUP BY SrcMod`,

		// graph_dead_assets — referenceable documents with no inbound edge.
		// Restricted to types that *should* be referenced if used; enums/constants/
		// layouts are excluded because their inbound edges aren't fully captured yet.
		`CREATE VIEW IF NOT EXISTS graph_dead_assets AS
			SELECT o.QualifiedName, o.ObjectType, o.ModuleName
			FROM objects o
			WHERE o.ObjectType IN ('ENTITY', 'MICROFLOW', 'NANOFLOW', 'PAGE', 'SNIPPET')
				AND NOT EXISTS (SELECT 1 FROM refs r WHERE r.TargetName = o.QualifiedName)`,

		// graph_refkind_distribution — the edge vocabulary (calibrates the others).
		`CREATE VIEW IF NOT EXISTS graph_refkind_distribution AS
			SELECT RefKind, SourceType, TargetType, COUNT(*) AS Count,
				round(100.0 * COUNT(*) / (SELECT COUNT(*) FROM refs), 1) AS Pct
			FROM refs GROUP BY RefKind, SourceType, TargetType`,

		// graph_entity_hotspots — entities used by the most flows.
		`CREATE VIEW IF NOT EXISTS graph_entity_hotspots AS
			SELECT TargetName AS Entity,
				COUNT(DISTINCT SourceName) AS UsedByFlows,
				group_concat(DISTINCT ModuleName) AS AcrossModules
			FROM refs
			WHERE TargetType = 'ENTITY' AND SourceType IN ('MICROFLOW', 'NANOFLOW')
			GROUP BY TargetName`,

		// --- Algorithmic graph analysis (populated by REFRESH CATALOG COMMUNITIES,
		// computed by the pure-Go mdl/catalog/graph package). ---

		// communities — Leiden community membership per asset.
		`CREATE TABLE IF NOT EXISTS communities_data (
			AssetName TEXT, ModuleName TEXT, CommunityId INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("communities"),

		// graph_cycles — SCC membership for assets in a dependency cycle.
		`CREATE TABLE IF NOT EXISTS graph_cycles_data (
			AssetName TEXT, ModuleName TEXT, CycleId INTEGER, CycleSize INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_cycles"),

		// graph_layers — topological layer (sequence number) per asset.
		`CREATE TABLE IF NOT EXISTS graph_layers_data (
			AssetName TEXT, ModuleName TEXT, Layer INTEGER,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_layers"),

		// graph_centrality — PageRank / betweenness per asset.
		`CREATE TABLE IF NOT EXISTS graph_centrality_data (
			AssetName TEXT, PageRank REAL, Betweenness REAL,
			ProjectId TEXT, SnapshotId TEXT
		)`,
		viewWithFullSnapshot("graph_centrality"),

		// community_summary — per-community size, type breakdown, dominant-module
		// Label, and members.
		`CREATE VIEW IF NOT EXISTS community_summary AS
			SELECT c.CommunityId,
				(SELECT c2.ModuleName FROM communities_data c2 WHERE c2.CommunityId = c.CommunityId
					GROUP BY c2.ModuleName ORDER BY COUNT(*) DESC, c2.ModuleName LIMIT 1) AS Label,
				COUNT(*) AS Size,
				COUNT(DISTINCT c.ModuleName) AS Modules,
				group_concat(c.AssetName) AS Members
			FROM communities_data c GROUP BY c.CommunityId`,

		// graph_module_dependencies — directed module→module edges (the facts a
		// layering/architecture Starlark rule consumes), with kinds and counts.
		`CREATE VIEW IF NOT EXISTS graph_module_dependencies AS
			SELECT substr(SourceName, 1, instr(SourceName, '.') - 1) AS SourceModule,
				substr(TargetName, 1, instr(TargetName, '.') - 1) AS TargetModule,
				RefKind, COUNT(*) AS Edges
			FROM refs
			WHERE instr(SourceName, '.') > 0 AND instr(TargetName, '.') > 0
				AND substr(SourceName, 1, instr(SourceName, '.') - 1) != substr(TargetName, 1, instr(TargetName, '.') - 1)
			GROUP BY SourceModule, TargetModule, RefKind`,

		// graph_integration_surface — cross-community edges classified into the
		// integration mechanism a split would require (UC2 contract list).
		`CREATE VIEW IF NOT EXISTS graph_integration_surface AS
			SELECT cs.CommunityId AS SourceCommunity, ct.CommunityId AS TargetCommunity,
				r.RefKind, COUNT(*) AS Edges,
				CASE r.RefKind
					WHEN 'associate'  THEN 'OData / shared entity'
					WHEN 'retrieve'   THEN 'OData read'
					WHEN 'create'     THEN 'event / REST write'
					WHEN 'change'     THEN 'event / REST write'
					WHEN 'call'       THEN 'REST (published microflow)'
					WHEN 'generalize' THEN 'BLOCKER: inheritance across boundary'
					ELSE 'review'
				END AS Mechanism
			FROM refs r
			JOIN communities_data cs ON r.SourceName = cs.AssetName
			JOIN communities_data ct ON r.TargetName = ct.AssetName
			WHERE cs.CommunityId != ct.CommunityId
			GROUP BY SourceCommunity, TargetCommunity, r.RefKind`,
	}

	for _, schema := range schemas {
		if _, err := c.db.Exec(schema); err != nil {
			return err
		}
	}

	// Record the schema version so future opens of this cache can decide
	// whether to migrate (drop-and-recreate) on mismatch.
	if _, err := c.db.Exec(
		`INSERT OR REPLACE INTO catalog_meta (Key, Value) VALUES (?, ?)`,
		MetaSchemaVersion, CatalogSchemaVersion,
	); err != nil {
		return err
	}

	return nil
}

// viewWithFullSnapshot builds a "CREATE VIEW <name> AS SELECT t.*, JOINed
// columns FROM <name>_data t LEFT JOIN snapshots s …" statement that exposes
// ProjectName + SnapshotDate + SnapshotSource + SourceId + SourceBranch +
// SourceRevision on top of the underlying _data table. Used for tables whose
// pre-refactor schema had all six denormalized columns.
func viewWithFullSnapshot(name string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.ProjectName,
			s.SnapshotDate,
			s.SnapshotSource,
			s.SourceId,
			s.SourceBranch,
			s.SourceRevision
		FROM ` + name + `_data t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}

// viewWithProjectNameAndSnapshotDateSource exposes ProjectName + SnapshotDate
// + SnapshotSource. Used for tables whose pre-refactor schema had those three
// denormalized columns but not the Source* group.
func viewWithProjectNameAndSnapshotDateSource(name string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.ProjectName,
			s.SnapshotDate,
			s.SnapshotSource
		FROM ` + name + `_data t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}

// viewWithSnapshotDateSource exposes SnapshotDate + SnapshotSource only.
// Used for tables whose pre-refactor schema had only those two denormalized
// columns (no ProjectName, no Source*).
func viewWithSnapshotDateSource(name string) string {
	return `CREATE VIEW IF NOT EXISTS ` + name + ` AS
		SELECT t.*,
			s.SnapshotDate,
			s.SnapshotSource
		FROM ` + name + `_data t
		LEFT JOIN snapshots s ON s.SnapshotId = t.SnapshotId`
}
