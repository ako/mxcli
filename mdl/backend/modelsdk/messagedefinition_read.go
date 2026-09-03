// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	genMsg "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// ListMessageDefinitionCollections reads every
// MessageDefinitions$MessageDefinitionCollection unit.
//
// Read-only on purpose: a mapping is authorable OVER a definition (#263), the
// definition itself is not. gen already models the whole tree and decodes real
// documents unchanged — verified against Workflow Commons, AgentCore and
// Email_Connector definitions — and none of these types appears in
// modelsdk/gen/keyaudit_test.go's mismatch ledger.
func (b *Backend) ListMessageDefinitionCollections() ([]*model.MessageDefinitionCollection, error) {
	units, err := mprread.ListUnitsWithContainer[*genMsg.MessageDefinitionCollection](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.MessageDefinitionCollection, 0, len(units))
	for _, u := range units {
		out = append(out, messageCollectionFromGen(u.Element, string(u.ContainerID)))
	}
	return out, nil
}

// messageCollectionFromGen converts one stored collection to the semantic model.
//
// Extracted from the loop so the write path's round-trip test can drive the same
// conversion a real read does — a test that rebuilt from a hand-made struct
// would prove nothing about documents Studio Pro actually writes.
func messageCollectionFromGen(g *genMsg.MessageDefinitionCollection, containerID string) *model.MessageDefinitionCollection {
	c := &model.MessageDefinitionCollection{
		ContainerID:   model.ID(containerID),
		Name:          g.Name(),
		Documentation: g.Documentation(),
		Excluded:      g.Excluded(),
		ExportLevel:   g.ExportLevel(),
	}
	c.ID = model.ID(g.ID())
	c.TypeName = "MessageDefinitions$MessageDefinitionCollection"
	for _, md := range g.MessageDefinitionsItems() {
		em, ok := md.(*genMsg.EntityMessageDefinition)
		if !ok {
			// A definition kind this reader does not model. Skipped by name so a
			// mapping referencing it is refused with "not found" rather than
			// silently resolving against nothing.
			continue
		}
		c.Definitions = append(c.Definitions, &model.MessageDefinition{
			Name: em.Name(),
			Root: exposedNodeFromGen(em.ExposedEntity()),
		})
	}
	return c
}

// exposedNodeFromGen converts one node of a definition's exposed tree.
func exposedNodeFromGen(n any) *model.MessageDefinitionElement {
	switch v := n.(type) {
	case nil:
		return nil
	case *genMsg.ExposedEntity:
		if v == nil {
			return nil
		}
		e := &model.MessageDefinitionElement{
			Kind:            "Entity",
			Entity:          v.EntityQualifiedName(),
			ExposedName:     v.ExposedName(),
			ExposedItemName: v.ExposedItemName(),
			OriginalName:    v.OriginalName(),
			Example:         v.Example(),
			Path:            v.Path(),
			MinOccurs:       int(v.MinOccurs()),
			MaxOccurs:       int(v.MaxOccurs()),
		}
		for _, c := range v.ChildrenItems() {
			if child := exposedNodeFromGen(c); child != nil {
				e.Children = append(e.Children, child)
			}
		}
		return e
	case *genMsg.ExposedAssociation:
		if v == nil {
			return nil
		}
		// An association node carries the nested entity's members directly; the
		// association is what reaches them.
		e := &model.MessageDefinitionElement{
			Kind:            "Entity",
			Association:     v.AssociationQualifiedName(),
			Entity:          v.EntityQualifiedName(),
			ExposedName:     v.ExposedName(),
			ExposedItemName: v.ExposedItemName(),
			OriginalName:    v.OriginalName(),
			Example:         v.Example(),
			Path:            v.Path(),
			MinOccurs:       int(v.MinOccurs()),
			MaxOccurs:       int(v.MaxOccurs()),
		}
		for _, c := range v.ChildrenItems() {
			if child := exposedNodeFromGen(c); child != nil {
				e.Children = append(e.Children, child)
			}
		}
		return e
	case *genMsg.ExposedAttribute:
		if v == nil {
			return nil
		}
		return &model.MessageDefinitionElement{
			Kind:          "Attribute",
			Attribute:     v.AttributeQualifiedName(),
			ExposedName:   v.ExposedName(),
			OriginalName:  v.OriginalName(),
			Example:       v.Example(),
			Path:          v.Path(),
			MinOccurs:     int(v.MinOccurs()),
			MaxOccurs:     int(v.MaxOccurs()),
			PrimitiveType: v.PrimitiveType(),
		}
	default:
		return nil
	}
}
