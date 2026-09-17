// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// carryChildIdentity copies each stored child's raw bytes onto its rebuilt
// counterpart, so the codec treats the child as an EXISTING element: the
// properties entityToGen set re-encode from the rebuild, while the ones the
// semantic model does not carry — the GUID above all — pass through verbatim.
//
// Only the two children that carry a GUID are covered. Attributes and indexes
// are the entity's GUID-bearing children (`RegisterTypeDefaults` in
// domainmodel_write.go registers EmitGUID for DomainModels$Attribute and
// DomainModels$EntityIndex); access rules, validation rules and event handlers
// have no GUID, so there is no identity to carry and raw-carrying them would
// only risk resurrecting a property a rewrite means to clear.
//
// The correspondence is the stored $ID, which the read path round-trips into the
// semantic model (attributeFromGen / indexFromGen set .ID). That is what makes
// this exact rather than structural: the executor preserves an attribute's ID
// across a RENAME, so a rename carries the GUID forward — Studio Pro renames the
// column and keeps the data — while a genuinely new attribute arrives with an
// empty ID, matches nothing, and correctly gets a fresh GUID.
//
// Pairing by name (not by list position) for attributes is deliberate: names are
// unique within an entity, so the lookup does not depend on entityToGen and the
// semantic list staying in lockstep. Indexes have no name, so they pair by
// position — which holds because entityToGen appends one gen index per semantic
// index in the same loop.
func carryChildIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	if ge == nil || orig == nil || entity == nil {
		return
	}
	carryAttributeIdentity(ge, orig, entity)
	carryIndexIdentity(ge, orig, entity)
}

func carryAttributeIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	stored := map[string]*genDm.Attribute{}
	for _, el := range orig.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok && a.Raw() != nil {
			stored[string(a.ID())] = a
		}
	}
	if len(stored) == 0 {
		return
	}

	// The semantic attribute the rebuild came from, by its (possibly new) name.
	semantic := make(map[string]*domainmodel.Attribute, len(entity.Attributes))
	for _, a := range entity.Attributes {
		semantic[a.Name] = a
	}

	for _, el := range ge.AttributesItems() {
		ga, ok := el.(*genDm.Attribute)
		if !ok {
			continue
		}
		sem := semantic[ga.Name()]
		if sem == nil || sem.ID == "" {
			continue // a newly added attribute: nothing to carry, fresh GUID is right
		}
		sa := stored[string(sem.ID)]
		if sa == nil {
			continue
		}
		ga.SetID(sa.ID())
		ga.SetRaw(sa.Raw())
	}
}

func carryIndexIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	stored := map[string]*genDm.Index{}
	for _, el := range orig.IndexesItems() {
		if idx, ok := el.(*genDm.Index); ok && idx.Raw() != nil {
			stored[string(idx.ID())] = idx
		}
	}
	if len(stored) == 0 {
		return
	}

	rebuilt := ge.IndexesItems()
	for i, sem := range entity.Indexes {
		if i >= len(rebuilt) {
			break
		}
		gi, ok := rebuilt[i].(*genDm.Index)
		if !ok || sem == nil || sem.ID == "" {
			continue
		}
		si := stored[string(sem.ID)]
		if si == nil {
			continue
		}
		gi.SetID(si.ID())
		gi.SetRaw(si.Raw())
	}
}
