// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// domainModelDocType is the PED document type for a module's domain model.
// It is addressed by module name (documentName = module name only).
const domainModelDocType = "DomainModels$DomainModel"

// ---------------------------------------------------------------------------
// PED wire types
// ---------------------------------------------------------------------------

type pedPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type pedAttribute struct {
	SType           string `json:"$Type"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	EnumerationName string `json:"enumerationName,omitempty"`
}

type pedEntity struct {
	SType          string         `json:"$Type"`
	Name           string         `json:"name"`
	Location       *pedPoint      `json:"location,omitempty"`
	Attributes     []pedAttribute `json:"attributes"`
	Generalization any            `json:"generalization,omitempty"`
}

type pedOperation struct {
	Type  string `json:"type"`            // "set" | "add" | "remove"
	Value any    `json:"value,omitempty"` // for set/add
	Index *int   `json:"index,omitempty"` // for add (optional) / remove (required)
}

type pedOpEntry struct {
	Path      string       `json:"path"`
	Operation pedOperation `json:"operation"`
}

// ---------------------------------------------------------------------------
// Entity operations
// ---------------------------------------------------------------------------

// CreateEntity adds an entity to a module's domain model via PED.
//
// Choreography (verified against Studio Pro 11.x):
//
//	ped_get_schema       (contract: fetch schema before add)
//	ped_update_document  (add the entity constructor at /entities)
//	ped_check_errors     (contract: validate after the final write)
func (b *Backend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	value, err := b.buildEntityValue(entity)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Entity", "DomainModels$Attribute"); err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/entities",
		Operation: pedOperation{Type: "add", Value: value},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// DeleteEntity removes an entity from a module's domain model via PED.
func (b *Backend) DeleteEntity(domainModelID model.ID, entityID model.ID) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.entityNameForID(domainModelID, entityID)
	if err != nil {
		return err
	}
	idx, err := b.entityIndex(moduleName, name)
	if err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/entities",
		Operation: pedOperation{Type: "remove", Index: &idx},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// AddAttribute adds an attribute to an existing entity via PED.
func (b *Backend) AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.entityNameForID(domainModelID, entityID)
	if err != nil {
		return err
	}
	idx, err := b.entityIndex(moduleName, name)
	if err != nil {
		return err
	}
	value, err := b.buildAttributeValue(attr)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Attribute"); err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      fmt.Sprintf("/entities/%d/attributes", idx),
		Operation: pedOperation{Type: "add", Value: value},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// ---------------------------------------------------------------------------
// Association operations
// ---------------------------------------------------------------------------

// pedAssociation is the PED $constructor shape for an association. parentEntity
// and childEntity are entity GUIDs ($ID). For a project Studio Pro has open
// from the same .mpr, those GUIDs equal the local reader's entity IDs — which
// is exactly what the executor passes as assoc.ParentID / assoc.ChildID.
type pedAssociation struct {
	SType        string `json:"$Type"`
	Name         string `json:"name"`
	ParentEntity string `json:"parentEntity"` // FROM / owner (stores the reference)
	ChildEntity  string `json:"childEntity"`  // TO / referenced
	Multiplicity string `json:"multiplicity"`
}

// CreateAssociation adds a within-module association via PED.
//
// Mapping (note Mendix's inverted parent/child naming — see CLAUDE.md):
//   - assoc.ParentID (FROM, FK owner)  -> parentEntity
//   - assoc.ChildID  (TO, referenced)  -> childEntity
//   - Type/Owner                       -> multiplicity
func (b *Backend) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	if assoc.ParentID == "" || assoc.ChildID == "" {
		return fmt.Errorf("association %q: missing parent/child entity id", assoc.Name)
	}
	if err := guardAssociationFeatures(assoc); err != nil {
		return err
	}
	mult, err := associationMultiplicity(assoc)
	if err != nil {
		return err
	}
	if err := b.ensureSchema("DomainModels$Association"); err != nil {
		return err
	}
	value := pedAssociation{
		SType:        "DomainModels$Association",
		Name:         assoc.Name,
		ParentEntity: string(assoc.ParentID),
		ChildEntity:  string(assoc.ChildID),
		Multiplicity: mult,
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/associations",
		Operation: pedOperation{Type: "add", Value: value},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// DeleteAssociation removes a within-module association via PED.
func (b *Backend) DeleteAssociation(domainModelID model.ID, assocID model.ID) error {
	moduleName, err := b.moduleNameForDomainModel(domainModelID)
	if err != nil {
		return err
	}
	name, err := b.associationNameForID(domainModelID, assocID)
	if err != nil {
		return err
	}
	idx, err := b.arrayElementIndex(moduleName, "/associations", "association", name)
	if err != nil {
		return err
	}
	if err := b.pedUpdate(moduleName, pedOpEntry{
		Path:      "/associations",
		Operation: pedOperation{Type: "remove", Index: &idx},
	}); err != nil {
		return err
	}
	return b.pedCheckErrors(moduleName)
}

// associationMultiplicity maps a domain-model association's Type/Owner onto the
// PED multiplicity enum. The "one" side is always the parent (FROM) entity.
func associationMultiplicity(a *domainmodel.Association) (string, error) {
	switch a.Type {
	case domainmodel.AssociationTypeReferenceSet:
		return "many_to_many", nil
	case domainmodel.AssociationTypeReference:
		if a.Owner == domainmodel.AssociationOwnerBoth {
			return "one_to_one", nil
		}
		return "one_to_many", nil
	default:
		return "", fmt.Errorf("association %q: unsupported type %q for the MCP backend", a.Name, a.Type)
	}
}

// guardAssociationFeatures rejects association settings the PED constructor
// cannot express, rather than silently applying PED defaults. The constructor
// covers name/parent/child/multiplicity only; delete behavior and storage
// format fall back to Studio Pro's defaults (keep-references, column), so a
// non-default request must be refused.
func guardAssociationFeatures(a *domainmodel.Association) error {
	defaultDelete := domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	for _, db := range []*domainmodel.DeleteBehavior{a.ChildDeleteBehavior, a.ParentDeleteBehavior} {
		if db != nil && db.Type != "" && db.Type != defaultDelete {
			return fmt.Errorf("association %q: custom delete behavior (%s) is not yet supported by the MCP backend", a.Name, db.Type)
		}
	}
	if a.Source != "" {
		return fmt.Errorf("association %q: external/OData associations are not supported by the MCP backend", a.Name)
	}
	return nil
}

// associationNameForID resolves an association's name from its ID via the local
// reader.
func (b *Backend) associationNameForID(domainModelID, assocID model.ID) (string, error) {
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	for _, a := range dm.Associations {
		if a.ID == assocID {
			return a.Name, nil
		}
	}
	return "", fmt.Errorf("association %s not found in domain model %s", assocID, domainModelID)
}

// ---------------------------------------------------------------------------
// Value builders + feature guards
// ---------------------------------------------------------------------------

// buildEntityValue maps a domain-model Entity onto the PED $constructor shape.
// The constructor is deliberately simple (name, attributes, location,
// generalization). Features it cannot express are rejected with a clear error
// rather than silently dropped — except a Boolean's auto-added `false` default,
// which is Mendix's own default and carries no information.
func (b *Backend) buildEntityValue(entity *domainmodel.Entity) (*pedEntity, error) {
	if !entity.Persistable {
		return nil, unsupportedEntityFeature(entity.Name, "non-persistent entities")
	}
	if len(entity.Indexes) > 0 {
		return nil, unsupportedEntityFeature(entity.Name, "indexes")
	}
	if len(entity.ValidationRules) > 0 {
		return nil, unsupportedEntityFeature(entity.Name, "validation rules (NOT NULL / UNIQUE)")
	}
	if len(entity.EventHandlers) > 0 {
		return nil, unsupportedEntityFeature(entity.Name, "event handlers")
	}
	if entity.HasOwner || entity.HasChangedBy || entity.HasCreatedDate || entity.HasChangedDate {
		return nil, unsupportedEntityFeature(entity.Name, "system members (owner/changedBy/createdDate/changedDate)")
	}

	pe := &pedEntity{
		SType:      "DomainModels$Entity",
		Name:       entity.Name,
		Location:   &pedPoint{X: entity.Location.X, Y: entity.Location.Y},
		Attributes: []pedAttribute{},
	}
	if entity.GeneralizationRef != "" {
		pe.Generalization = entity.GeneralizationRef // by-name reference (e.g. "System.User")
	}
	for _, a := range entity.Attributes {
		pa, err := b.buildAttributeValue(a)
		if err != nil {
			return nil, err
		}
		pe.Attributes = append(pe.Attributes, *pa)
	}
	return pe, nil
}

func (b *Backend) buildAttributeValue(a *domainmodel.Attribute) (*pedAttribute, error) {
	if a.Type == nil {
		return nil, fmt.Errorf("attribute %q: missing type", a.Name)
	}
	typeName := a.Type.GetTypeName()
	pedType, err := pedAttributeType(typeName)
	if err != nil {
		return nil, fmt.Errorf("attribute %q: %w", a.Name, err)
	}
	if err := guardAttributeValue(a, typeName); err != nil {
		return nil, err
	}
	pa := &pedAttribute{SType: "DomainModels$Attribute", Name: a.Name, Type: pedType}
	if et, ok := a.Type.(*domainmodel.EnumerationAttributeType); ok {
		pa.EnumerationName = et.EnumerationRef
	}
	return pa, nil
}

// guardAttributeValue rejects attribute value settings the PED constructor
// cannot carry. A Boolean's auto-added `false` default is allowed through
// (dropped), since it is Mendix's own default.
func guardAttributeValue(a *domainmodel.Attribute, typeName string) error {
	if a.Value == nil {
		return nil
	}
	if a.Value.Type == "CalculatedValue" {
		return fmt.Errorf("attribute %q: calculated attributes are not yet supported by the MCP backend", a.Name)
	}
	if a.Value.DefaultValue != "" {
		if typeName == "Boolean" && strings.EqualFold(a.Value.DefaultValue, "false") {
			return nil // harmless: Boolean's default is false anyway
		}
		return fmt.Errorf("attribute %q: default values are not yet supported by the MCP backend", a.Name)
	}
	return nil
}

// pedAttributeType maps a domain-model attribute type name onto the PED
// constructor's `type` enum.
func pedAttributeType(name string) (string, error) {
	switch name {
	case "String", "Integer", "Long", "Decimal", "Boolean", "DateTime",
		"AutoNumber", "Binary", "HashedString", "Enumeration":
		return name, nil
	case "Date":
		return "DateTime", nil // Mendix stores Date as a DateTime
	default:
		return "", fmt.Errorf("attribute type %q is not supported by the MCP backend", name)
	}
}

func unsupportedEntityFeature(entityName, feature string) error {
	return fmt.Errorf("entity %q: %s are not yet supported by the MCP backend (entity slice); create it against a local .mpr instead", entityName, feature)
}

// ---------------------------------------------------------------------------
// PED helpers
// ---------------------------------------------------------------------------

// moduleNameForDomainModel resolves the PED documentName (the module name) for
// a domain-model ID using the local reader.
func (b *Backend) moduleNameForDomainModel(domainModelID model.ID) (string, error) {
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	mod, err := b.reader.GetModule(dm.ContainerID)
	if err != nil {
		return "", fmt.Errorf("resolve module for domain model %s: %w", domainModelID, err)
	}
	return mod.Name, nil
}

// entityNameForID resolves an entity's name from its ID via the local reader.
func (b *Backend) entityNameForID(domainModelID, entityID model.ID) (string, error) {
	dm, err := b.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return "", fmt.Errorf("resolve domain model %s: %w", domainModelID, err)
	}
	for _, e := range dm.Entities {
		if e.ID == entityID {
			return e.Name, nil
		}
	}
	return "", fmt.Errorf("entity %s not found in domain model %s", entityID, domainModelID)
}

// entityIndex finds the position of an entity within the live /entities array.
func (b *Backend) entityIndex(moduleName, entityName string) (int, error) {
	return b.arrayElementIndex(moduleName, "/entities", "entity", entityName)
}

// arrayElementIndex reads a named-element array (e.g. /entities, /associations)
// over MCP and returns the index of the element with the given name. Reading
// over MCP means the index reflects Studio Pro's in-memory order, which is what
// a subsequent remove-by-index needs.
func (b *Backend) arrayElementIndex(moduleName, jsonPath, kind, name string) (int, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{jsonPath},
	})
	if err != nil {
		return 0, err
	}
	if res.IsError {
		return 0, fmt.Errorf("ped_read_document %s%s: %s", moduleName, jsonPath, res.Text)
	}
	var doc struct {
		Results []struct {
			Result []struct {
				Name string `json:"name"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(res.Text), &doc); err != nil {
		return 0, fmt.Errorf("parse %s of %s: %w", jsonPath, moduleName, err)
	}
	if len(doc.Results) == 0 {
		return 0, fmt.Errorf("%s %q not found in module %q", kind, name, moduleName)
	}
	for i, e := range doc.Results[0].Result {
		if e.Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%s %q not found in module %q", kind, name, moduleName)
}

// ensureSchema fetches schemas for the given element types once per session.
func (b *Backend) ensureSchema(elementTypes ...string) error {
	var needed []string
	for _, t := range elementTypes {
		if !b.schemaFetched[t] {
			needed = append(needed, t)
		}
	}
	if len(needed) == 0 {
		return nil
	}
	res, err := b.client.CallTool("ped_get_schema", map[string]any{"elementTypes": needed})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("ped_get_schema %v: %s", needed, res.Text)
	}
	for _, t := range needed {
		b.schemaFetched[t] = true
	}
	return nil
}

// pedUpdate applies operations to a module's domain model.
func (b *Backend) pedUpdate(moduleName string, ops ...pedOpEntry) error {
	res, err := b.client.CallTool("ped_update_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"operations":   ops,
	})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("ped_update_document %s: %s", moduleName, res.Text)
	}
	return nil
}

// pedCheckErrors validates a module's domain model and surfaces any errors.
func (b *Backend) pedCheckErrors(moduleName string) error {
	res, err := b.client.CallTool("ped_check_errors", map[string]any{
		"documents": []map[string]any{
			{"documentType": domainModelDocType, "documentName": moduleName},
		},
	})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("validation failed for %s: %s", moduleName, res.Text)
	}
	return nil
}
