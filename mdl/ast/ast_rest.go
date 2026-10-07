// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// REST Client Statements
// ============================================================================

// CreateRestClientStmt represents: CREATE REST CLIENT Module.Name BASE URL '...' AUTHENTICATION ... BEGIN ... END
type CreateRestClientStmt struct {
	CreateGuard      // `create … if not exists` (ako/mxcli#731)
	Name             QualifiedName
	BaseUrl          string
	Authentication   *RestAuthDef // nil = AUTHENTICATION NONE
	Operations       []*RestOperationDef
	Documentation    string
	DocumentationSet bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Folder           string // Folder path within module
	CreateOrModify   bool   // True if CREATE OR MODIFY was used
	OpenApiPath      string // Non-empty = spec-driven; operations come from spec not OPERATION blocks
}

func (s *CreateRestClientStmt) isStatement() {}

// RestAuthDef represents authentication configuration in a CREATE REST CLIENT statement.
type RestAuthDef struct {
	Scheme   string // "BASIC"
	Username string // literal string or $variable name
	Password string // literal string or $variable name
}

// RestOperationDef represents a single operation in a CREATE REST CLIENT statement.
type RestOperationDef struct {
	Name             string
	Documentation    string
	Method           string // "GET", "POST", "PUT", "PATCH", "DELETE"
	Path             string
	Parameters       []RestParamDef  // path parameters
	QueryParameters  []RestParamDef  // query parameters
	Headers          []RestHeaderDef // HTTP headers
	BodyType         string          // "JSON", "FILE", "TEMPLATE", "MAPPING", "" (none)
	BodyVariable     string          // e.g. "$ItemData" or template string
	BodyMapping      *RestMappingDef // for MAPPING body
	ResponseType     string          // "JSON", "STRING", "FILE", "STATUS", "NONE", "MAPPING"
	ResponseVariable string          // e.g. "$CreatedItem"
	ResponseMapping  *RestMappingDef // for MAPPING response
	Timeout          int
}

// RestMappingDef represents an inline import/export mapping in a REST operation.
type RestMappingDef struct {
	Entity  QualifiedName
	Entries []RestMappingEntry
}

// RestMappingEntry is either a value mapping or a nested object mapping.
type RestMappingEntry struct {
	// Value mapping: Left = Right
	Left  string
	Right string

	// Object mapping: [CREATE] Association/Entity = ExposedName { children }
	Create      bool
	Association QualifiedName
	Entity      QualifiedName
	ExposedName string
	Children    []RestMappingEntry
}

// RestParamDef represents a path or query parameter definition.
type RestParamDef struct {
	Name     string // includes $ prefix, e.g. "$userId"
	DataType string // "String", "Integer", "Boolean", "Decimal"
}

// RestHeaderDef represents an HTTP header definition.
//
// Value is the stored value template: `{Name}` in it is replaced by the
// operation parameter Name when the request is sent, as in the path. The old
// spelling `'Bearer ' + $Token` builds the value `Bearer {Token}` (ako/mxcli#707).
type RestHeaderDef struct {
	Name  string // header name, e.g. "Authorization"
	Value string // value template, e.g. "application/json" or "Bearer {Token}"
}

// DropRestClientStmt represents: DROP REST CLIENT Module.Name
type DropRestClientStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropRestClientStmt) isStatement() {}

// DescribeContractFromOpenAPIStmt represents: DESCRIBE CONTRACT OPERATIONS FROM OPENAPI '/path/to/spec.json'
// No project connection required; outputs the MDL that would be generated from the spec.
type DescribeContractFromOpenAPIStmt struct {
	SpecPath string
}

func (s *DescribeContractFromOpenAPIStmt) isStatement() {}

// ============================================================================
// Published REST Service Statements
// ============================================================================

// CreatePublishedRestServiceStmt represents:
//
//	CREATE PUBLISHED REST SERVICE Module.Name (Path: '...', Version: '...') { RESOURCE ... };
type CreatePublishedRestServiceStmt struct {
	CreateGuard // `create … if not exists` (ako/mxcli#731)
	Name        QualifiedName
	Path        string
	Version     string
	ServiceName string
	Folder      string
	// Authentication is nil when the statement does not state it, which keeps
	// the stored setting on create or modify.
	Authentication *PublishedRestAuthentication
	Resources      []*PublishedRestResourceDef
	CreateOrModify bool
}

// PublishedRestAuthentication is `Authentication: none | ( method, … )`.
// Methods holds the stored spellings ("Basic", "Session", "Microflow") in the
// order written, which is the order Studio Pro stores them; empty is `none`.
// Microflow is the custom-authentication microflow, set exactly when Methods
// holds "Microflow".
type PublishedRestAuthentication struct {
	Methods   []string
	Microflow string
}

func (s *CreatePublishedRestServiceStmt) isStatement() {}

type PublishedRestResourceDef struct {
	Name       string
	Operations []*PublishedRestOperationDef
}

type PublishedRestOperationDef struct {
	HTTPMethod    string        // GET, POST, PUT, DELETE, PATCH
	Path          string        // endpoint path (e.g. "/{id}")
	Microflow     QualifiedName // backing microflow
	Deprecated    bool
	ImportMapping string // optional qualified name
	ExportMapping string // optional qualified name
	Commit        string // optional: "Yes", "No"
}

// DropPublishedRestServiceStmt represents: DROP PUBLISHED REST SERVICE Module.Name
type DropPublishedRestServiceStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropPublishedRestServiceStmt) isStatement() {}

// AlterPublishedRestServiceStmt represents:
//
//	ALTER PUBLISHED REST SERVICE Module.Name <action>+
//
// where each action is one of: SET key = 'value' [, ...], ADD RESOURCE
// 'name' { ... }, or DROP RESOURCE 'name'.
type AlterPublishedRestServiceStmt struct {
	Name    QualifiedName
	Actions []PublishedRestAlterAction
}

func (s *AlterPublishedRestServiceStmt) isStatement() {}

// PublishedRestAlterAction is one of the alter operations.
type PublishedRestAlterAction interface {
	isPublishedRestAlterAction()
}

// PublishedRestSetAction represents: SET key = 'value' [, ...] or
// SET ( Key: value, ... ). Authentication is nil unless the list states it.
type PublishedRestSetAction struct {
	Changes        map[string]string
	Authentication *PublishedRestAuthentication
}

func (a *PublishedRestSetAction) isPublishedRestAlterAction() {}

// PublishedRestAddResourceAction represents: ADD RESOURCE 'name' { ops... }
type PublishedRestAddResourceAction struct {
	Resource *PublishedRestResourceDef
}

func (a *PublishedRestAddResourceAction) isPublishedRestAlterAction() {}

// PublishedRestDropResourceAction represents: DROP RESOURCE 'name'
type PublishedRestDropResourceAction struct {
	Name string
}

func (a *PublishedRestDropResourceAction) isPublishedRestAlterAction() {}
