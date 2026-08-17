// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
	"github.com/mendixlabs/mxcli/model"
)

// xpathModel answers the two questions mdl/xpathrefs asks while following an
// XPath path: is this qualified name an entity, and where does this association
// lead from here.
//
// It is a flat index built once per rename rather than a lookup per step. A
// constraint is walked once per predicate group per document, so a per-step
// backend round trip would turn one rename into thousands of domain-model reads.
type xpathModel struct {
	entities map[string]bool
	// assoc maps an association's qualified name to its two ends, FROM first.
	assoc map[string][2]string
}

var _ xpathrefs.Model = (*xpathModel)(nil)

// buildXPathModel indexes every entity and association in the project by
// qualified name.
func buildXPathModel(ctx *ExecContext) (*xpathModel, error) {
	m := &xpathModel{
		entities: map[string]bool{},
		assoc:    map[string][2]string{},
	}

	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil, err
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, err
	}

	// Entity element ID → qualified name, so an association's ends can be
	// resolved. Associations reference their ends by element ID, and a
	// cross-module association's ends live in a different domain model, so the
	// index has to span the whole project before any end is resolved.
	byID := map[model.ID]string{}
	type pending struct {
		qn                string
		parentID, childID model.ID
	}
	var assocs []pending

	for _, dm := range dms {
		moduleName := h.GetModuleName(h.FindModuleID(dm.ContainerID))
		if moduleName == "" {
			continue
		}
		for _, ent := range dm.Entities {
			qn := moduleName + "." + ent.Name
			m.entities[qn] = true
			byID[ent.ID] = qn
		}
		for _, a := range dm.Associations {
			assocs = append(assocs, pending{moduleName + "." + a.Name, a.ParentID, a.ChildID})
		}
	}

	for _, a := range assocs {
		from, okF := byID[a.parentID]
		to, okT := byID[a.childID]
		if !okF || !okT {
			// An end that cannot be resolved (an external entity, a broken
			// reference) leaves the association out of the index entirely, so a
			// path through it reads as unresolved and blocks the rewrite rather
			// than resolving to half an answer.
			continue
		}
		m.assoc[a.qn] = [2]string{from, to}
	}

	return m, nil
}

func (m *xpathModel) IsEntity(qn string) bool { return m.entities[qn] }

// AssociationTarget resolves a hop in either direction: Mendix XPath traverses an
// association from its FROM end (`[Mod.Order_Person/…]` on an Order) and from its
// TO end (the same step on a Person) alike, and the stored constraint looks the
// same both ways.
func (m *xpathModel) AssociationTarget(qn, from string) (string, bool) {
	ends, ok := m.assoc[qn]
	if !ok || from == "" {
		return "", false
	}
	switch from {
	case ends[0]:
		return ends[1], true
	case ends[1]:
		return ends[0], true
	}
	// A self-association resolves above; anything else means the path does not
	// actually start where the constraint says it does.
	return "", false
}
