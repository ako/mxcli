// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Import Mapping Statements
// ============================================================================

// CreateImportMappingStmt represents:
//
//	CREATE IMPORT MAPPING Module.Name
//	  WITH JSON STRUCTURE Module.JsonStructure
//	{
//	  CREATE Module.Entity {
//	    PetId = id KEY,
//	    Name = name
//	  }
//	};
type CreateImportMappingStmt struct {
	Name       QualifiedName
	Folder     string        // Folder path within module (empty = leave placement alone)
	SchemaKind string        // "JSON_STRUCTURE" or "XML_SCHEMA" or ""
	SchemaRef  QualifiedName // qualified name of the schema source
	// SchemaRoot selects the element the mapping STARTS at, when that is not the
	// structure's own root (#267). Written in member names, "/"-separated.
	SchemaRoot     string
	RootElement    *ImportMappingElementDef
	CreateOrModify bool // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateImportMappingStmt) isStatement() {}

// DropImportMappingStmt represents: DROP IMPORT MAPPING Module.Name
type DropImportMappingStmt struct {
	Name QualifiedName
}

func (s *DropImportMappingStmt) isStatement() {}

// ImportMappingElementDef represents one element in the mapping tree.
// MappingCustomHandlerDef is the `by Module.Microflow(...)` clause: a microflow
// resolves the object instead of Create/Find (#264).
type MappingCustomHandlerDef struct {
	Microflow  string
	Parameters []*MappingCallParameterDef
}

// MappingCallParameterDef binds one parameter of the called microflow.
// Source is "parent", "parameter", "ancestor" or "path".
type MappingCallParameterDef struct {
	Parameter string
	Source    string
	Level     int
	Path      string
}

type ImportMappingElementDef struct {
	// Object mapping fields
	Entity         string // qualified entity name (e.g. "Module.Customer")
	ObjectHandling string // "Create", "Find", "FindOrCreate"
	Association    string // qualified association name (from Assoc/Entity path)
	CustomHandler  *MappingCustomHandlerDef
	Children       []*ImportMappingElementDef

	// Value mapping fields
	Attribute string // entity attribute name (LHS of =)
	IsKey     bool

	// Shared
	JsonName string // JSON field name (RHS of = for both values and objects)

	// Value transform via microflow
	Converter      string // microflow qualified name (e.g. "Module.ConvertStringToDate")
	ConverterParam string // json field passed to converter
}

// ============================================================================
// Export Mapping Statements
// ============================================================================

// CreateExportMappingStmt represents:
//
//	CREATE EXPORT MAPPING Module.Name
//	  WITH JSON STRUCTURE Module.JsonStructure
//	{
//	  Module.Entity {
//	    jsonField = Attr,
//	    Module.Assoc/Module.Child AS jsonKey { ... }
//	  }
//	};
type CreateExportMappingStmt struct {
	Name       QualifiedName
	Folder     string        // Folder path within module (empty = leave placement alone)
	SchemaKind string        // "JSON_STRUCTURE" or "XML_SCHEMA" or ""
	SchemaRef  QualifiedName // qualified name of the schema source
	// SchemaRoot — see the note on CreateImportMappingStmt (#267).
	SchemaRoot      string
	NullValueOption string // "LeaveOutElement" or "SendAsNil" (default: "LeaveOutElement")
	RootElement     *ExportMappingElementDef
	CreateOrModify  bool // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateExportMappingStmt) isStatement() {}

// DropExportMappingStmt represents: DROP EXPORT MAPPING Module.Name
type DropExportMappingStmt struct {
	Name QualifiedName
}

func (s *DropExportMappingStmt) isStatement() {}

// ExportMappingElementDef represents one element in an export mapping tree.
type ExportMappingElementDef struct {
	// Object mapping fields
	Entity        string // qualified entity name (e.g. "Module.Customer")
	Association   string // qualified association name (from Assoc/Entity path)
	CustomHandler *MappingCustomHandlerDef
	// Group marks an entity-less grouping node: a JSON object with no Mendix
	// object behind it (#262).
	Group    bool
	Children []*ExportMappingElementDef

	// Value mapping fields
	Attribute string // entity attribute name (RHS of =)
	// Converter is the microflow the value passes through on its way out
	// (#266): `jsonKey = Module.MF(Attr)`.
	Converter string

	// Shared
	JsonName string // JSON field name (LHS of = for values, RHS of AS for objects)
}
