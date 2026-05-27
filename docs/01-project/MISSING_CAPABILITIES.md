# Missing Capabilities Analysis

Based on investigation of three real-world Mendix 11.6.3 projects:
- **EnquiriesManagement** (28 modules, AI agent app with workflows)
- **Evora-FactoryManagement** (39 modules, industrial IoT app with REST/OData/workflows)
- **LatoProductInventory** (30 modules, product inventory app with AI agents)

## Summary: Unsupported Document Types Across All Projects

| Document Type ($Type) | EM | FM | LPI | Total | Priority |
|----------------------|----|----|-----|-------|----------|
| `workflows$workflow` | 12 | 1 | 0 | **13** | **High** |
| `JsonStructures$JsonStructure` | 23 | 42 | 38 | **103** | Medium |
| `ImportMappings$ImportMapping` | 22 | 35 | 33 | **90** | Medium |
| `ExportMappings$ExportMapping` | 19 | 31 | 24 | **74** | Medium |
| `microflows$rule` | 15 | 28 | 12 | **55** | Medium |
| `MessageDefinitions$MessageDefinitionCollection` | 12 | 11 | 10 | **33** | Low |
| `rest$PublishedRestService` | 2 | 7 | 8 | **17** | **High** |
| `RegularExpressions$RegularExpression` | 8 | 4 | 4 | **16** | Low |
| `CustomIcons$CustomIconCollection` | 5 | 2 | 3 | **10** | Low |
| `Menus$MenuDocument` | 3 | 2 | 2 | **7** | Low |
| `Queues$Queue` | 2 | 2 | 1 | **5** | Low |
| `Texts$SystemTextCollection` | 1 | 1 | 1 | **3** | Low |
| `rest$ConsumedRestService` | 0 | 2 | 0 | **2** | Medium |
| `DatabaseConnector$DatabaseConnection` | 0 | 1 | 0 | **1** | Low |

EM = EnquiriesManagement, FM = FactoryManagement, LPI = LatoProductInventory

## Detailed Analysis by Priority

### High Priority

#### 1. Workflows (`workflows$workflow`) - 13 documents

**Impact**: Core Mendix feature for business process automation. Used heavily in the EnquiriesManagement project for agent-orchestrated enquiry handling.

**What's needed**: Full proposal in [PROPOSAL_workflow_support.md](PROPOSAL_workflow_support.md).

**Summary**: BSON parser, Reader methods, SHOW/DESCRIBE commands, catalog table, cross-references. The workflow domain has 14 concrete activity types and multiple polymorphic hierarchies.

#### 2. Published REST Services (`rest$PublishedRestService`) - 17 documents

**Impact**: Published REST services are a primary integration mechanism in Mendix. FactoryManagement exposes 7 REST services (oauth2, discovery, TCSSO, ViewerService, etc.), LatoProductInventory exposes 8.

**What's needed**:
- BSON parser for `rest$PublishedRestService` documents
- Reader: `ListPublishedRestServices()`, `GetPublishedRestService(id)`
- Commands: `show rest services`, `describe rest service Module.Name`
- Catalog table: `CATALOG.REST_SERVICES` with columns for Name, Path, Version, Authentication, OperationCount
- Cross-references: REST operations referencing microflows, entities

**Complexity**: Medium. Published REST services contain operations with HTTP methods, paths, parameters, microflow handlers, and authentication settings. The structure is well-defined in reflection data.

### Medium Priority

#### 3. JSON Structures (`JsonStructures$JsonStructure`) - 103 documents

**Impact**: JSON structures define schemas used by import/export mappings for REST/JSON data transformations. Very common in integration-heavy apps.

**What's needed**:
- BSON parser for JSON structure documents
- Reader: `ListJsonStructures()`, `GetJsonStructure(id)`
- Commands: `show json structures`, `describe json structure Module.Name`
- Catalog table with Name, ModuleName, documentation

**Complexity**: Low-Medium. JSON structures are relatively simple documents containing a JSON schema definition.

#### 4. Import/Export Mappings (90 + 74 = 164 documents)

**Impact**: Mappings define how JSON/XML data is transformed to/from Mendix objects. Critical for REST integration. Always paired with JSON structures.

**What's needed**:
- BSON parser for `ImportMappings$ImportMapping` and `ExportMappings$ExportMapping`
- Reader methods for both types
- SHOW/DESCRIBE commands for both types
- Catalog tables: `CATALOG.IMPORT_MAPPINGS`, `CATALOG.EXPORT_MAPPINGS`
- Cross-references: mappings reference entities and JSON structures

**Complexity**: Medium. Mappings contain element-to-attribute mappings with entity references, attribute mappings, and value transformations.

#### 5. Rules (`microflows$rule`) - 55 documents

**Impact**: Rules are a special microflow subtype used for entity validation. Currently parsed as regular microflows but not distinguished.

**What's needed**:
- Distinguish rules from microflows in the parser (check `$type == "microflows$rule"`)
- Add `IsRule` field to the Microflow struct or create a separate Rule type
- Add to catalog as `ObjectType = 'rule'` in the objects view
- `show rules [in module]` command

**Complexity**: Low. Rules use the same BSON format as microflows. The main change is distinguishing them in listing/catalog.

#### 6. Consumed REST Services (`rest$ConsumedRestService`) - 2 documents

**Impact**: Different from consumed OData services (which are already supported). These are plain REST API integrations.

**What's needed**:
- BSON parser for `rest$ConsumedRestService` documents
- Reader: `ListConsumedRestServices()`, `GetConsumedRestService(id)`
- SHOW/DESCRIBE commands
- Catalog table

**Complexity**: Low-Medium. Similar structure to consumed OData services.

### Low Priority

#### 7. Message Definitions (`MessageDefinitions$MessageDefinitionCollection`) - 33 documents

**Impact**: Message definitions describe the structure of messages used by published services. Related to REST/SOAP operations.

**What's needed**: BSON parser, Reader methods, SHOW command.

**Complexity**: Medium (large documents, avg 132KB in FactoryManagement).

#### 8. Regular Expressions (`RegularExpressions$RegularExpression`) - 16 documents

**Impact**: Named regex patterns referenced by validation rules. Minimal.

**What's needed**: Simple BSON parser (Name, Expression fields), Reader methods, SHOW command.

**Complexity**: Very low. Simple documents with just a name and pattern string.

#### 9. Custom Icons (`CustomIcons$CustomIconCollection`) - 10 documents

**Impact**: Icon collections for use in the UI. Binary content, not useful for code analysis.

**What's needed**: Reader listing only (for completeness). No describe/MDL needed.

**Complexity**: Very low (listing only).

#### 10. Menus (`Menus$MenuDocument`) - 7 documents

**Impact**: Menu configurations (separate from navigation). Rarely used directly.

**What's needed**: Reader listing, basic SHOW command.

**Complexity**: Low.

#### 11. Queues (`Queues$Queue`) - 5 documents

**Impact**: Task queue definitions for asynchronous processing. Lightweight documents.

**What's needed**: BSON parser (Name, Module), Reader methods, SHOW command, catalog table.

**Complexity**: Very low.

#### 12. System Texts (`Texts$SystemTextCollection`) - 3 documents

**Impact**: Translation/localization texts. One per project.

**What's needed**: Reader listing only.

**Complexity**: Very low.

#### 13. Database Connections (`DatabaseConnector$DatabaseConnection`) - 1 document

**Impact**: External database connection configuration. Only in FactoryManagement.

**What's needed**: Reader listing, basic SHOW command.

**Complexity**: Very low.

## Recommended Implementation Order

### Sprint 1: High-Impact Read-Only
1. **Workflows** (Phase 1 from workflow proposal) - highest value, complex
2. **Rules** - low effort, completes microflow domain
3. **Published REST Services** - high value for integration projects

### Sprint 2: Integration Support
4. **JSON Structures** - prerequisite for mappings
5. **Import Mappings** - completes REST integration chain
6. **Export Mappings** - completes REST integration chain
7. **Consumed REST Services** - completes REST support

### Sprint 3: Completeness
8. **Queues** - simple, useful for async patterns
9. **Regular Expressions** - simple
10. **Message Definitions** - completes service descriptions
11. **Menus** - listing only
12. **Custom Icons** - listing only
13. **System Texts** - listing only
14. **Database Connections** - listing only

## Impact on Catalog Coverage

After implementing all items, the catalog would cover:

| Current | After Sprint 1 | After Sprint 2 | After Sprint 3 |
|---------|----------------|----------------|----------------|
| Entities | Entities | Entities | Entities |
| Microflows | Microflows | Microflows | Microflows |
| Nanoflows | Nanoflows | Nanoflows | Nanoflows |
| Pages | Pages | Pages | Pages |
| Snippets | Snippets | Snippets | Snippets |
| Enumerations | Enumerations | Enumerations | Enumerations |
| Java Actions | Java Actions | Java Actions | Java Actions |
| OData Clients | OData Clients | OData Clients | OData Clients |
| OData Services | OData Services | OData Services | OData Services |
| | **Workflows** | Workflows | Workflows |
| | **Rules** | Rules | Rules |
| | **REST Services** | REST Services | REST Services |
| | | **JSON Structures** | JSON Structures |
| | | **Import Mappings** | Import Mappings |
| | | **Export Mappings** | Export Mappings |
| | | **Consumed REST** | Consumed REST |
| | | | **Queues** |
| | | | **Regex** |
| | | | **Msg Defs** |

This would cover **all** document types found in the three test projects.

## Cross-Reference Coverage After Full Implementation

The `CATALOG.REFS` table would gain these additional reference types:

| Source Type | Ref Kind | Target Type | Sprint |
|------------|----------|-------------|--------|
| WORKFLOW | call_microflow | MICROFLOW | 1 |
| WORKFLOW | call_workflow | WORKFLOW | 1 |
| WORKFLOW | show_page | PAGE | 1 |
| WORKFLOW | parameter | ENTITY | 1 |
| WORKFLOW | user_targeting | MICROFLOW | 1 |
| REST_SERVICE | handler | MICROFLOW | 1 |
| IMPORT_MAPPING | entity | ENTITY | 2 |
| IMPORT_MAPPING | json_structure | JSON_STRUCTURE | 2 |
| EXPORT_MAPPING | entity | ENTITY | 2 |
| EXPORT_MAPPING | json_structure | JSON_STRUCTURE | 2 |
| BUSINESS_EVENT | entity | ENTITY | Future |
| BUSINESS_EVENT | handler | MICROFLOW | Future |
