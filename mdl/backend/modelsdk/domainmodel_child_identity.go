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
// semantic list staying in lockstep. Indexes have no name, so their fallback is
// position (carryIndexIdentity) — which holds because entityToGen appends one gen
// index per semantic index in the same loop.
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
		carryStoredAttribute(ga, sa)
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

// carryIndexIdentity pairs each rebuilt index with the stored one it came from:
// the stored $ID first, then position for whatever that left.
//
// "No ID means position" was not enough: the executor's CREATE OR MODIFY path mints
// a FRESH ID for every declared index (buildEntityFromStmt), so the ID is present and
// simply misses. Reading that miss as "a new index" re-minted the GUID, the write
// guard refused it (mendixlabs/mxcli#1470) — the same mistake #1119 made for
// attributes, which survive only because their fallback is by name.
//
// An index has no name, and the fallback is position rather than the column list on
// purpose: canon.TransplantIDs reattaches the stored $IDs by aligning the index list
// on $Type, which for nameless indexes is position. A GUID paired by columns onto a
// reordered list would then sit under its neighbour's $ID and be refused as a moved
// GUID. Position agrees with the transplant; a crosswise pairing costs an index
// rebuild and no data — nothing the platform keys on rides on an index GUID.
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
	n := min(len(entity.Indexes), len(rebuilt))
	claimed := make(map[string]bool, len(storedByID))
	paired := make([]bool, n)
	carry := func(i int, si *genDm.Index) {
		gi, ok := rebuilt[i].(*genDm.Index)
		if !ok || si == nil || entity.Indexes[i] == nil || claimed[string(si.ID())] {
			return
		}
		gi.SetID(si.ID())
		gi.SetRaw(si.Raw())
		claimed[string(si.ID())] = true
		paired[i] = true
	}

	// Pass 1 — the stored $ID, which the ALTER paths round-trip.
	for i := 0; i < n; i++ {
		if sem := entity.Indexes[i]; sem != nil && sem.ID != "" {
			carry(i, storedByID[string(sem.ID)])
		}
	}
	// Pass 2 — position, for an index with no ID or one that matched nothing (the
	// statement-declared case, whose ID is freshly minted).
	for i := 0; i < n && i < len(storedInOrder); i++ {
		if !paired[i] {
			carry(i, storedInOrder[i])
		}
	}
}

// carryStoredAttribute makes the rebuilt attribute ga the stored attribute sa as
// far as identity goes: the stored $ID and raw bytes (so the GUID passes through
// instead of being re-minted as $ID — #1119, ako/mxcli#627) and the stored export
// level. Both rebuild sites use it: the entity rewrite (carryAttributeIdentity)
// and the single-attribute rewrite (Backend.UpdateAttribute).
func carryStoredAttribute(ga, sa *genDm.Attribute) {
	ga.SetID(sa.ID())
	if raw := sa.Raw(); raw != nil {
		ga.SetRaw(raw)
	}
	// attributeToGen sets ExportLevel "Hidden", and a property the rebuild
	// sets wins over the carried raw bytes — so without this an API attribute
	// became Hidden on every rewrite of its entity (ako/mxcli#801). The
	// semantic attribute has no export level to take it from.
	if lvl := sa.ExportLevel(); lvl != "" {
		ga.SetExportLevel(lvl)
	}
}
