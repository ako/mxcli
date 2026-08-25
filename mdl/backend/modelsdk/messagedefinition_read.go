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
		g := u.Element
		c := &model.MessageDefinitionCollection{
			ContainerID: model.ID(u.ContainerID),
			Name:        g.Name(),
		}
		c.ID = model.ID(g.ID())
		c.TypeName = "MessageDefinitions$MessageDefinitionCollection"
		for _, md := range g.MessageDefinitionsItems() {
			em, ok := md.(*genMsg.EntityMessageDefinition)
			if !ok {
				// A definition kind this reader does not model. Recorded by name
				// so a mapping referencing it is refused with "not found" rather
				// than silently resolving against nothing.
				continue
			}
			c.Definitions = append(c.Definitions, &model.MessageDefinition{
				Name: em.Name(),
				Root: exposedNodeFromGen(em.ExposedEntity()),
			})
		}
		out = append(out, c)
	}
	return out, nil
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
			ExposedName:     v.ExposedName(),
			ExposedItemName: v.ExposedItemName(),
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
			Path:          v.Path(),
			MinOccurs:     int(v.MinOccurs()),
			MaxOccurs:     int(v.MaxOccurs()),
			PrimitiveType: v.PrimitiveType(),
		}
	default:
		return nil
	}
}
