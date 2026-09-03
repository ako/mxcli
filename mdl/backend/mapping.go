// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// MappingBackend provides import/export mapping and JSON structure operations.
type MappingBackend interface {
	ListImportMappings() ([]*model.ImportMapping, error)
	GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error)
	CreateImportMapping(im *model.ImportMapping) error
	UpdateImportMapping(im *model.ImportMapping) error
	DeleteImportMapping(id model.ID) error
	MoveImportMapping(im *model.ImportMapping) error

	ListExportMappings() ([]*model.ExportMapping, error)
	GetExportMappingByQualifiedName(moduleName, name string) (*model.ExportMapping, error)
	CreateExportMapping(em *model.ExportMapping) error
	UpdateExportMapping(em *model.ExportMapping) error
	DeleteExportMapping(id model.ID) error
	MoveExportMapping(em *model.ExportMapping) error

	// ListMessageDefinitionCollections reads the message-definition documents.
	ListMessageDefinitionCollections() ([]*model.MessageDefinitionCollection, error)
	// CreateMessageDefinitionCollection / Update / Delete author the document
	// (ako/mxcli#272). A message definition is a selection over the domain
	// model — unlike an XML schema or a WSDL it holds nothing external — which
	// is what makes it the one non-JSON mapping source a script can create.
	CreateMessageDefinitionCollection(c *model.MessageDefinitionCollection) error
	UpdateMessageDefinitionCollection(c *model.MessageDefinitionCollection) error
	DeleteMessageDefinitionCollection(id string) error

	// ListXmlSchemas reads the project's XML schema documents. Read-only —
	// there is no CREATE for one — and used to resolve a mapping's
	// `with xml schema` reference before mxbuild reports it as CE1613.
	ListXmlSchemas() ([]*types.XmlSchema, error)

	ListJsonStructures() ([]*types.JsonStructure, error)
	GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error)
	CreateJsonStructure(js *types.JsonStructure) error
	UpdateJsonStructure(js *types.JsonStructure) error
	// DeleteJsonStructure removes a JSON structure by ID.
	// Takes string (not model.ID) to match the SDK writer layer convention.
	DeleteJsonStructure(id string) error
}
