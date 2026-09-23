// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The shape `CREATE OR MODIFY PERSISTENT ENTITY` hands to UpdateEntity is not the
// shape every ALTER hands it, and the difference is the whole defect.
//
// `mergeDeclaredOntoStoredEntity` sets `merged.Attributes = declared.Attributes` and
// `merged.Indexes = declared.Indexes` — the members the STATEMENT declares, built from
// text by the visitor, carrying **no element ID at all**. carryChildIdentity keyed
// entirely on that ID and read an empty one as "a genuinely new member, so a fresh
// GUID is right", so re-declaring an existing entity re-minted every attribute GUID
// and dropped every column on the next deploy.
//
// Same data loss as #1119 through a different executor path. The write guard is what
// surfaced it, as a REFUSAL on a doctype script that had been passing for months:
//
//	failed to update entity: refusing to write unit d82b0484-…: 1 element(s) kept
//	their $ID but would be written with a different GUID
//	  dff2ced1-… (DomainModels$Attribute): stored 4b52b36b-…, would write dff2ced1-…
//
// Note WHY the refused element still had the stored `$ID`, because that is the part
// that makes the report look impossible: the fresh element is encoded with
// `GUID = $ID` (EmitGUID), and `canon.TransplantIDs` then substitutes the stored `$ID`
// over *every* 16-byte binary in the document — the GUID field included. The
// corruption therefore arrives wearing the correct `$ID`, which is exactly the
// pairing the guard uses, and is why the guard could see it at all.
//
// The fix is to fall back to the attribute NAME when there is no ID. A name is unique
// within an entity, so a stored attribute of that name IS the same member — which is
// what Studio Pro assumes when a re-declared attribute keeps its column.

// TestCreateOrModifyEntity_PreservesAttributeGUIDsWithoutSemanticIDs is the arm the CI
// failure measured: attributes declared by the statement, so without IDs.
func TestCreateOrModifyEntity_PreservesAttributeGUIDsWithoutSemanticIDs(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	dmID, ent := richestEntity(t, b)
	before := attributeGUIDs(t, b, dmID, ent.ID)
	if len(before) < 3 {
		t.Fatalf("fixture entity %s has %d attributes; need >= 3", ent.Name, len(before))
	}
	// Without GUID != $ID this cannot fail against the broken code: an element mxcli
	// created has them equal from birth, so re-minting reproduces the same value.
	ids := attributeIDs(t, b, dmID, ent.ID)
	for name, guid := range before {
		if guid == "" || guid == ids[name] {
			t.Fatalf("fixture attribute %s has GUID %q and $ID %q; need them to differ", name, guid, ids[name])
		}
	}

	// What mergeDeclaredOntoStoredEntity produces: the stored entity with the
	// statement's member list swapped in, and that list carries no IDs.
	for _, a := range ent.Attributes {
		a.ID = ""
	}

	if err := b.UpdateEntity(dmID, ent); err != nil {
		// A refusal here IS the failure: the guard firing because the carry did not
		// happen. It is not an unrelated error.
		t.Fatalf("UpdateEntity with statement-declared attributes: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Reopen: preservation only counts if it reached disk.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	after := attributeGUIDs(t, b2, dmID, ent.ID)
	moved := 0
	for name, want := range before {
		got, ok := after[name]
		if !ok {
			t.Errorf("attribute %s is gone after the re-declaration", name)
			continue
		}
		if got != want {
			moved++
			if moved <= 3 {
				t.Errorf("attribute %s: storage GUID changed %s -> %s", name, want, got)
			}
		}
	}
	if moved > 3 {
		t.Errorf("... and %d more attributes whose storage GUID changed", moved-3)
	}
}

// TestCreateOrModifyEntity_NewAttributeStillGetsAFreshGUID is the control for the name
// fallback, and the reason the fallback claims each stored attribute at most once: an
// attribute the statement genuinely introduces must NOT inherit an existing or removed
// member's GUID, or the runtime hands it that column's data.
func TestCreateOrModifyEntity_NewAttributeStillGetsAFreshGUID(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	dmID, ent := richestEntity(t, b)
	before := attributeGUIDs(t, b, dmID, ent.ID)

	const added = "CreateOrModifyAdded"
	for _, a := range ent.Attributes {
		a.ID = ""
	}
	ent.Attributes = append(ent.Attributes, &domainmodel.Attribute{
		Name: added,
		Type: &domainmodel.StringAttributeType{Length: 20},
	})

	if err := b.UpdateEntity(dmID, ent); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	after := attributeGUIDs(t, b2, dmID, ent.ID)
	got := after[added]
	if got == "" {
		t.Fatalf("new attribute %s has no storage GUID", added)
	}
	for name, old := range before {
		if old == got {
			t.Errorf("new attribute %s inherited %s's storage GUID %s — the runtime would hand it that column's data",
				added, name, got)
		}
	}
	// And the pre-existing ones still kept theirs, so the fallback did not spend its
	// match on the newcomer.
	for name, want := range before {
		if after[name] != want {
			t.Errorf("attribute %s: storage GUID changed %s -> %s", name, want, after[name])
		}
	}
}

// TestCreateOrModifyEntity_PreservesIndexGUIDsWithoutSemanticIDs is the index arm.
// `merged.Indexes = declared.Indexes` strips index IDs the same way, and an index has
// no name to fall back to — it is addressed by position, which is the correspondence
// the ID-bearing path already trusts (entityToGen appends one gen index per semantic
// index, in order).
//
// No data rides on an index GUID — the platform rebuilds the index — so the cost of a
// wrong pairing here is a rebuild, not a lost column. What a missing carry costs is
// the write itself: the guard refuses it, and `CREATE OR MODIFY` on an indexed entity
// stops working.
func TestCreateOrModifyEntity_PreservesIndexGUIDsWithoutSemanticIDs(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	dmID, _ := richestEntity(t, b)
	const entName = "CreateOrModifyIndexed"
	if err := b.CreateEntity(dmID, &domainmodel.Entity{
		Name:        entName,
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Code", Type: &domainmodel.StringAttributeType{Length: 50}},
			{Name: "Descr", Type: &domainmodel.StringAttributeType{Length: 50}},
		},
	}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	stored := mustFindEntity(t, b, dmID, entName)
	stored.Indexes = []*domainmodel.Index{{
		Attributes: []*domainmodel.IndexAttribute{
			{AttributeID: stored.Attributes[0].ID, Ascending: true},
		},
	}}
	if err := b.UpdateEntity(dmID, stored); err != nil {
		t.Fatalf("UpdateEntity (add index): %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// An index mxcli created has GUID == $ID legitimately and cannot detect the
	// conflation, so seed the divergence the way Studio Pro leaves it, below the
	// writer (which would rightly refuse a write that moves a GUID).
	seeded := patchIndexGUID(t, proj, dmID, entName)

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	target := mustFindEntity(t, b2, dmID, entName)
	for _, a := range target.Attributes {
		a.ID = ""
	}
	for _, i := range target.Indexes {
		i.ID = ""
	}
	if err := b2.UpdateEntity(dmID, target); err != nil {
		t.Fatalf("UpdateEntity with statement-declared members: %v", err)
	}
	if err := b2.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })

	got := indexGUIDs(t, b3, dmID, entName)
	if len(got) != 1 {
		t.Fatalf("expected 1 index after the re-declaration, got %d", len(got))
	}
	if got[0] != seeded {
		t.Errorf("index storage GUID changed %s -> %s", seeded, got[0])
	}
}
