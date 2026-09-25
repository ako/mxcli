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
//
// THE SEMANTIC ID IS NOT ALWAYS THERE, and reading its absence as "a new member"
// was a second instance of #1119 rather than a safe default.
// `CREATE OR MODIFY ENTITY` reaches UpdateEntity through
// mergeDeclaredOntoStoredEntity, which sets `Attributes` and `Indexes` to the lists
// the STATEMENT declares — built from text by the visitor, carrying no ID at all —
// so an ID-only pairing carried nothing and every attribute of a re-declared entity
// was re-minted. The write guard caught it as a refusal on a doctype script that had
// been passing for months; a stored attribute of the same NAME is the same member,
// which is what Studio Pro assumes when a re-declared attribute keeps its column.
//
// So each list is paired in two passes: the exact key first, the weaker one only for
// what it left over. A stored element is claimed at most once, which is what keeps a
// rename-plus-re-add from handing the newcomer the renamed member's data — the ID
// match takes the stored element, and the name fallback then finds it claimed.
func carryChildIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	if ge == nil || orig == nil || entity == nil {
		return
	}
	carryAttributeIdentity(ge, orig, entity)
	carryIndexIdentity(ge, orig, entity)
}

func carryAttributeIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	storedByID := map[string]*genDm.Attribute{}
	storedByName := map[string]*genDm.Attribute{}
	for _, el := range orig.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok && a.Raw() != nil {
			storedByID[string(a.ID())] = a
			storedByName[a.Name()] = a
		}
	}
	if len(storedByID) == 0 {
		return
	}

	// The semantic attribute the rebuild came from, by its (possibly new) name.
	semantic := make(map[string]*domainmodel.Attribute, len(entity.Attributes))
	for _, a := range entity.Attributes {
		semantic[a.Name] = a
	}

	var rebuilt []*genDm.Attribute
	for _, el := range ge.AttributesItems() {
		if ga, ok := el.(*genDm.Attribute); ok {
			rebuilt = append(rebuilt, ga)
		}
	}

	claimed := make(map[string]bool, len(storedByID))
	carry := func(ga, sa *genDm.Attribute) {
		ga.SetID(sa.ID())
		ga.SetRaw(sa.Raw())
		claimed[string(sa.ID())] = true
	}

	// Pass 1 — the exact key. The executor preserves an attribute's ID across a
	// RENAME, so this is what carries the GUID onto the new name.
	paired := make(map[*genDm.Attribute]bool, len(rebuilt))
	for _, ga := range rebuilt {
		sem := semantic[ga.Name()]
		if sem == nil || sem.ID == "" {
			continue
		}
		sa := storedByID[string(sem.ID)]
		if sa == nil {
			continue
		}
		carry(ga, sa)
		paired[ga] = true
	}

	// Pass 2 — by name, for what pass 1 left. This is the statement-declared case,
	// where there is no ID to pair on. A stored attribute pass 1 already claimed is
	// skipped, so an attribute genuinely introduced under a name another member has
	// just vacated still gets a fresh GUID rather than that member's data.
	for _, ga := range rebuilt {
		if paired[ga] {
			continue
		}
		sa := storedByName[ga.Name()]
		if sa == nil || claimed[string(sa.ID())] {
			continue // genuinely new: nothing to carry, a fresh GUID is right
		}
		carry(ga, sa)
	}
}

func carryIndexIdentity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	storedByID := map[string]*genDm.Index{}
	var storedInOrder []*genDm.Index
	for _, el := range orig.IndexesItems() {
		if idx, ok := el.(*genDm.Index); ok && idx.Raw() != nil {
			storedByID[string(idx.ID())] = idx
			storedInOrder = append(storedInOrder, idx)
		}
	}
	if len(storedByID) == 0 {
		return
	}

	rebuilt := ge.IndexesItems()
	claimed := make(map[string]bool, len(storedByID))
	for i, sem := range entity.Indexes {
		if i >= len(rebuilt) {
			break
		}
		gi, ok := rebuilt[i].(*genDm.Index)
		if !ok || sem == nil {
			continue
		}
		var si *genDm.Index
		switch {
		case sem.ID != "":
			si = storedByID[string(sem.ID)]
		case i < len(storedInOrder):
			// The statement-declared case: no ID to pair on, and an index has no name
			// either, so position is what is left. It is the same correspondence the
			// ID-bearing branch already trusts, and it is the weaker one — a reordered
			// or dropped index pairs the wrong way round. That costs an index rebuild
			// and no data, because nothing the platform keys on rides on an index
			// GUID; what a MISSING carry costs is the write, which the guard refuses.
			si = storedInOrder[i]
		}
		if si == nil || claimed[string(si.ID())] {
			continue
		}
		gi.SetID(si.ID())
		gi.SetRaw(si.Raw())
		claimed[string(si.ID())] = true
	}
}
