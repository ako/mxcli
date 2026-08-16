// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// OData Write Statements
// ============================================================================

// CreateODataClientStmt represents: CREATE ODATA CLIENT Module.Name (...)
type CreateODataClientStmt struct {
	Name              QualifiedName
	Version           string
	ODataVersion      string
	MetadataUrl       string
	TimeoutExpression string
	ProxyType         string
	Description       string
	Documentation     string
	Folder            string // Folder path within module (e.g., "Integration/APIs")
	CreateOrModify    bool   // True if CREATE OR MODIFY was used

	// HTTP configuration
	ServiceUrl        string // Custom service URL (overrides metadata-derived URL)
	UseAuthentication bool
	HttpUsername      string // Mendix expression for username
	HttpPassword      string // Mendix expression for password

	// Whether the credential above was written as a quoted literal rather than
	// a constant reference. The visitor strips a literal's quotes, so by the
	// time it reaches the executor `'f1api'` and `Module.ApiUser` are both bare
	// strings — and only the first is a value mxcli can use for the design-time
	// $metadata fetch. A constant is resolved by the runtime, not by us.
	HttpUsernameIsLiteral bool
	HttpPasswordIsLiteral bool
	ClientCertificate     string

	// Microflow references. `ConfigurationMicroflow` (returns
	// System.ConsumedODataConfiguration) and `HeadersMicroflow` (returns a list
	// of System.HttpHeader) are distinct storage slots on Mendix >= 11.10, so
	// they are tracked separately (writing one into the other's slot triggers
	// CE6808/CE6816).
	ConfigurationMicroflow string // microflow Module.ConfigureMF
	HeadersMicroflow       string // microflow Module.SetHeadersMF
	ErrorHandlingMicroflow string // microflow Module.HandleErrorMF

	// Proxy constant references
	ProxyHost     string
	ProxyPort     string
	ProxyUsername string
	ProxyPassword string

	// Custom HTTP headers
	Headers []HeaderDef

	// UnknownProperties: see CreateODataServiceStmt.UnknownProperties.
	UnknownProperties []string
}

// HeaderDef represents a custom HTTP header entry.
type HeaderDef struct {
	Key   string
	Value string // Mendix expression
	// ValueIsLiteral mirrors HttpUsernameIsLiteral: a quoted literal can be sent
	// on the design-time fetch, a constant reference cannot.
	ValueIsLiteral bool
}

func (s *CreateODataClientStmt) isStatement() {}

// AlterODataClientStmt represents: ALTER ODATA CLIENT Module.Name SET key = value
type AlterODataClientStmt struct {
	Name    QualifiedName
	Changes map[string]any // property name -> new value
}

func (s *AlterODataClientStmt) isStatement() {}

// DropODataClientStmt represents: DROP ODATA CLIENT Module.Name
type DropODataClientStmt struct {
	Name QualifiedName
}

func (s *DropODataClientStmt) isStatement() {}

// CreateODataServiceStmt represents: CREATE ODATA SERVICE Module.Name (...) AUTHENTICATION ... { ... }
type CreateODataServiceStmt struct {
	Name          QualifiedName
	Path          string
	Version       string
	ODataVersion  string
	Namespace     string
	ServiceName   string
	Summary       string
	Description   string
	Documentation string
	Folder        string // Folder path within module (e.g., "Integration/APIs")
	// PublishAssociations selects how associations appear in the metadata:
	// true = as links, false = as an associated object id. The executor
	// defaults an unspecified value to true, so PublishAssociationsSet records
	// whether the author said anything at all — an explicit false is the
	// author's choice and is written as given.
	PublishAssociations    bool
	PublishAssociationsSet bool
	// SupportsGraphQL publishes the same resources over GraphQL too. Unlike
	// PublishAssociations there is no useful default to infer: false is what
	// every service was before, so an unset value is left alone rather than
	// opted in. Set records whether the author said anything, which is what
	// keeps `alter` from turning it off on a service that had it on.
	SupportsGraphQL     bool
	SupportsGraphQLSet  bool
	AuthenticationTypes []string
	// AuthMicroflow is the microflow named by `authentication microflow X`.
	// Custom authentication is the only method that carries a target, and
	// Mendix rejects the service without one (CE0333 "Please select a microflow
	// to use for authentication").
	AuthMicroflow string
	Entities      []*PublishedEntityDef
	// Microflows are OData actions — `publish microflow Module.MF`. Mendix
	// exposes each as an ActionImport in $metadata.
	Microflows     []*PublishedMicroflowDef
	CreateOrModify bool // True if CREATE OR MODIFY was used

	// UnknownProperties holds property names the visitor did not recognise, in
	// source order. The parser accepts any `name: value` pair, so without this
	// a typo is discarded in silence and the model is quietly missing what the
	// author asked for.
	UnknownProperties []string
}

func (s *CreateODataServiceStmt) isStatement() {}

// PublishedEntityDef represents a PUBLISH ENTITY block within an OData service.
type PublishedEntityDef struct {
	Entity      QualifiedName
	ExposedName string
	ReadMode    string
	InsertMode  string
	UpdateMode  string
	DeleteMode  string
	UsePaging   bool
	PageSize    int
	Members     []*PublishedMemberDef

	// Query options. nil means "not specified" and stores Mendix's own default
	// of true. Countable in particular is not free: it forces the read
	// microflow to take a System.ODataResponse parameter and to compute a count
	// the caller may never ask for.
	Countable     *bool
	SkipSupported *bool
	TopSupported  *bool

	// UnknownProperties: see CreateODataServiceStmt.UnknownProperties.
	UnknownProperties []string
}

// PublishedMicroflowDef represents a PUBLISH MICROFLOW block — an OData action.
//
// Parameter data types and the return type are read off the microflow rather
// than restated here, so the two cannot drift.
type PublishedMicroflowDef struct {
	Microflow   QualifiedName
	ExposedName string
	// Parameters selects and optionally renames the microflow's parameters.
	// Empty means expose them all under their own names.
	Parameters []*PublishedParamDef
	// ExposeAll records an explicit `expose (*)`, which is the same as omitting
	// the clause but says so.
	ExposeAll bool
}

// PublishedParamDef is one exposed microflow parameter.
type PublishedParamDef struct {
	Name        string
	ExposedName string
	CanBeEmpty  bool
}

// PublishedMemberDef represents an EXPOSE member within a PUBLISH ENTITY block.
type PublishedMemberDef struct {
	Name        string
	ExposedName string
	Filterable  bool
	Sortable    bool
	IsPartOfKey bool
}

// AlterODataServiceStmt represents: ALTER ODATA SERVICE Module.Name SET key = value
type AlterODataServiceStmt struct {
	Name    QualifiedName
	Changes map[string]any // property name -> new value
}

func (s *AlterODataServiceStmt) isStatement() {}

// DropODataServiceStmt represents: DROP ODATA SERVICE Module.Name
type DropODataServiceStmt struct {
	Name QualifiedName
}

func (s *DropODataServiceStmt) isStatement() {}

// CreateExternalEntityStmt represents: CREATE [OR MODIFY] EXTERNAL ENTITY Module.Name FROM ODATA CLIENT Module.Service (...) (attrs);
//
// Scalar property fields are pointers so the executor can distinguish
// "omitted by user" (nil → preserve existing value on CREATE OR MODIFY)
// from "explicitly set to zero" (e.g. Countable: false). Treating omitted
// fields as zero on modify silently corrupted entities — see issue #594.
type CreateExternalEntityStmt struct {
	Name                     QualifiedName
	ServiceRef               QualifiedName // FROM ODATA CLIENT ...
	EntitySet                *string
	RemoteName               *string
	Countable                *bool
	Creatable                *bool
	Deletable                *bool
	Updatable                *bool
	AllowCreateChangeLocally *bool       // "Allow creating and changing locally" flag
	Attributes               []Attribute // reuse from ast_entity.go
	Documentation            string
	CreateOrModify           bool

	// UnknownProperties: see CreateODataServiceStmt.UnknownProperties.
	UnknownProperties []string
}

func (s *CreateExternalEntityStmt) isStatement() {}

// CreateExternalEntitiesStmt represents: CREATE [OR MODIFY] EXTERNAL ENTITIES FROM Module.Service [INTO Module] [ENTITIES (Name1, Name2)]
type CreateExternalEntitiesStmt struct {
	ServiceRef     QualifiedName // FROM Module.Service
	TargetModule   string        // INTO Module (optional, defaults to service module)
	EntityNames    []string      // ENTITIES (Name1, Name2) filter (optional, imports all if empty)
	CreateOrModify bool          // True if CREATE OR MODIFY was used
}

func (s *CreateExternalEntitiesStmt) isStatement() {}
