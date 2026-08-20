// SPDX-License-Identifier: Apache-2.0

// Package ast defines the Abstract Syntax Tree nodes for MDL (Mendix Definition Language).
// This package contains types for the domain model subset: entities, attributes,
// associations, enumerations, and view entities.
package ast

import "strings"

// Statement represents any MDL statement that can be executed.
type Statement interface {
	isStatement()
}

// Position represents a location in the domain model canvas.
type Position struct {
	X int
	Y int
}

// QualifiedName represents a module-qualified name like "Module.Entity".
type QualifiedName struct {
	Module string
	Name   string
}

func (q QualifiedName) String() string {
	if q.Module == "" {
		return q.Name
	}
	return q.Module + "." + q.Name
}

// ============================================================================
// Program
// ============================================================================

// Program represents a complete MDL program (sequence of statements).
type Program struct {
	Statements []Statement
}

// ============================================================================
// Move Statement
// ============================================================================

// DocumentType represents the type of document being moved.
type DocumentType string

// The doctypes MOVE accepts. The value is the MDL spelling, which is also what
// the executor prints back — so an unrecognised one cannot be mistaken for a
// recognised one in output.
//
// Only ENTITY is not a top-level document unit: it lives inside a domain model
// and its move converts associations rather than reparenting a row. Everything
// else here reduces to one containment update, which is why doctypes can be
// added to this list without a handler each.
const (
	DocumentTypePage                 DocumentType = "PAGE"
	DocumentTypeMicroflow            DocumentType = "MICROFLOW"
	DocumentTypeSnippet              DocumentType = "SNIPPET"
	DocumentTypeNanoflow             DocumentType = "NANOFLOW"
	DocumentTypeEntity               DocumentType = "ENTITY"
	DocumentTypeEnumeration          DocumentType = "ENUMERATION"
	DocumentTypeConstant             DocumentType = "CONSTANT"
	DocumentTypeDatabaseConnection   DocumentType = "DATABASE CONNECTION"
	DocumentTypeJavaAction           DocumentType = "JAVA ACTION"
	DocumentTypeODataService         DocumentType = "ODATA SERVICE"
	DocumentTypeBuildingBlock        DocumentType = "BUILDING BLOCK"
	DocumentTypeLayout               DocumentType = "LAYOUT"
	DocumentTypeMenu                 DocumentType = "MENU"
	DocumentTypeWorkflow             DocumentType = "WORKFLOW"
	DocumentTypeQueue                DocumentType = "QUEUE"
	DocumentTypeScheduledEvent       DocumentType = "SCHEDULED EVENT"
	DocumentTypeRegularExpression    DocumentType = "REGULAR EXPRESSION"
	DocumentTypeJsonStructure        DocumentType = "JSON STRUCTURE"
	DocumentTypeImportMapping        DocumentType = "IMPORT MAPPING"
	DocumentTypeExportMapping        DocumentType = "EXPORT MAPPING"
	DocumentTypeJavaScriptAction     DocumentType = "JAVASCRIPT ACTION"
	DocumentTypeDataTransformer      DocumentType = "DATA TRANSFORMER"
	DocumentTypeImageCollection      DocumentType = "IMAGE COLLECTION"
	DocumentTypeIconCollection       DocumentType = "ICON COLLECTION"
	DocumentTypeRestClient           DocumentType = "REST CLIENT"
	DocumentTypePublishedRestService DocumentType = "PUBLISHED REST SERVICE"
	DocumentTypeODataClient          DocumentType = "ODATA CLIENT"
	DocumentTypeBusinessEventService DocumentType = "BUSINESS EVENT SERVICE"
	DocumentTypeModel                DocumentType = "MODEL"
	DocumentTypeAgent                DocumentType = "AGENT"
	DocumentTypeKnowledgeBase        DocumentType = "KNOWLEDGE BASE"
	DocumentTypeConsumedMCPService   DocumentType = "CONSUMED MCP SERVICE"
)

// MoveDocumentTypeByKeyword maps a moveDocumentType grammar rule's text to its
// DocumentType. The rule's GetText() concatenates its tokens with no separator,
// so `IMPORT MAPPING` arrives as "IMPORTMAPPING".
//
// The single registry for the doctypes MOVE accepts: the visitor reads it to
// build the statement, and the executor reads its values to decide whether a
// document it found is the kind the statement named. Keeping one list is the
// point — a second copy is how the MOVE FOLDER discriminator went stale before.
var MoveDocumentTypeByKeyword = map[string]DocumentType{
	"PAGE":                 DocumentTypePage,
	"MICROFLOW":            DocumentTypeMicroflow,
	"NANOFLOW":             DocumentTypeNanoflow,
	"SNIPPET":              DocumentTypeSnippet,
	"BUILDINGBLOCK":        DocumentTypeBuildingBlock,
	"LAYOUT":               DocumentTypeLayout,
	"MENU":                 DocumentTypeMenu,
	"ENUMERATION":          DocumentTypeEnumeration,
	"CONSTANT":             DocumentTypeConstant,
	"WORKFLOW":             DocumentTypeWorkflow,
	"QUEUE":                DocumentTypeQueue,
	"SCHEDULEDEVENT":       DocumentTypeScheduledEvent,
	"REGULAREXPRESSION":    DocumentTypeRegularExpression,
	"JSONSTRUCTURE":        DocumentTypeJsonStructure,
	"IMPORTMAPPING":        DocumentTypeImportMapping,
	"EXPORTMAPPING":        DocumentTypeExportMapping,
	"JAVAACTION":           DocumentTypeJavaAction,
	"JAVASCRIPTACTION":     DocumentTypeJavaScriptAction,
	"DATABASECONNECTION":   DocumentTypeDatabaseConnection,
	"DATATRANSFORMER":      DocumentTypeDataTransformer,
	"IMAGECOLLECTION":      DocumentTypeImageCollection,
	"ICONCOLLECTION":       DocumentTypeIconCollection,
	"RESTCLIENT":           DocumentTypeRestClient,
	"PUBLISHEDRESTSERVICE": DocumentTypePublishedRestService,
	"ODATACLIENT":          DocumentTypeODataClient,
	"ODATASERVICE":         DocumentTypeODataService,
	"BUSINESSEVENTSERVICE": DocumentTypeBusinessEventService,
	"MODEL":                DocumentTypeModel,
	"AGENT":                DocumentTypeAgent,
	"KNOWLEDGEBASE":        DocumentTypeKnowledgeBase,
	"CONSUMEDMCPSERVICE":   DocumentTypeConsumedMCPService,
}

// IsMoveDocumentType reports whether spelling (lower-cased, spaced, e.g.
// "json structure") names a doctype MOVE accepts.
func IsMoveDocumentType(spelling string) bool {
	for _, docType := range MoveDocumentTypeByKeyword {
		if strings.EqualFold(string(docType), spelling) {
			return true
		}
	}
	return false
}

// MoveStmt represents: MOVE PAGE/MICROFLOW/SNIPPET/NANOFLOW/ENTITY/ENUMERATION Module.Name TO FOLDER 'path' IN Module
type MoveStmt struct {
	DocumentType DocumentType  // PAGE, MICROFLOW, SNIPPET, NANOFLOW, ENTITY, ENUMERATION
	Name         QualifiedName // Source document qualified name
	Folder       string        // Target folder path (empty = module root)
	TargetModule string        // Target module name (empty = same module)
}

func (s *MoveStmt) isStatement() {}
