// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// DomainModelBackend provides domain model, entity, attribute, and
// association operations.
type DomainModelBackend interface {
	// Domain models
	ListDomainModels() ([]*domainmodel.DomainModel, error)
	GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error)
	GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error)
	UpdateDomainModel(dm *domainmodel.DomainModel) error

	// SetDomainModelAnnotations replaces the canvas notes on a domain model.
	//
	// Separate from UpdateDomainModel because that one deliberately carries
	// annotations through as untouched storage (ADR-0005) — it rebuilds only the
	// entity and association lists, which is what protects notes from every
	// unrelated edit. Folding annotation authoring into it would put the
	// preservation and the mutation on the same code path, which is how they got
	// deleted in the first place.
	SetDomainModelAnnotations(domainModelID model.ID, annotations []*domainmodel.Annotation) error

	// Entities
	CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error
	UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error
	DeleteEntity(domainModelID model.ID, entityID model.ID) error
	MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error)

	// Attributes
	AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	UpdateAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	DeleteAttribute(domainModelID model.ID, entityID model.ID, attrID model.ID) error

	// Associations
	CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error
	CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error
	DeleteAssociation(domainModelID model.ID, assocID model.ID) error
	DeleteCrossAssociation(domainModelID model.ID, assocID model.ID) error

	// View entities
	CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error)
	DeleteViewEntitySourceDocument(id model.ID) error
	DeleteViewEntitySourceDocumentByName(moduleName, docName string) error
	FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error)
	FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error)
	MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error
	UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error)
	UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error
}
