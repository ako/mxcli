// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// dmDoc builds the shape the #1119 guard exists for: a domain model holding one
// entity holding two attributes, each element carrying both an $ID and the
// separate GUID the runtime keys the database on. Each id/guid is a byte, so the
// same structure can be built twice with chosen identities.
//
// attrPointer is a plain 16-byte binary property that happens to hold an
// element's $ID — the shape a reference takes in a stored document. It is here
// so the guard is measured against a document that contains one.
func dmDoc(t *testing.T, entGUID, a1ID, a1GUID, a2ID, a2GUID byte) []byte {
	t.Helper()
	return marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Entities", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityImpl"},
				{Key: "$ID", Value: bin(2)},
				{Key: "GUID", Value: bin(entGUID)},
				{Key: "Name", Value: "Organization"},
				{Key: "Attributes", Value: bson.A{
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(a1ID)},
						{Key: "GUID", Value: bin(a1GUID)},
						{Key: "Name", Value: "Name"},
					},
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(a2ID)},
						{Key: "GUID", Value: bin(a2GUID)},
						{Key: "Name", Value: "Status"},
					},
				}},
			},
		}},
		{Key: "Associations", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$Association"},
				{Key: "$ID", Value: bin(9)},
				{Key: "GUID", Value: bin(90)},
				{Key: "Name", Value: "Org_Person"},
				// A reference, not a containment edge: the same 16-byte shape
				// under a different key.
				{Key: "ParentPointer", Value: bin(2)},
			},
		}},
	})
}

// TestStorageGUIDChanges_FiresOnTheReportedShape is the guard's positive
// control. The written document is what #1119 produces: every $ID preserved
// (TransplantIDs put them back) and every attribute's GUID replaced by its own
// $ID. If this does not fire, the guard is inert and the rest of the cases prove
// nothing.
func TestStorageGUIDChanges_FiresOnTheReportedShape(t *testing.T) {
	stored := dmDoc(t, 20, 3, 30, 4, 40)
	// GUID = $ID on both attributes, exactly as the codec's EmitGUID default writes it.
	written := dmDoc(t, 20, 3, 3, 4, 4)

	changes := StorageGUIDChanges(written, stored)
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(changes), changes)
	}
	for _, c := range changes {
		if c.Type != "DomainModels$Attribute" {
			t.Errorf("change on %s, want DomainModels$Attribute", c.Type)
		}
		if c.Stored == c.Written {
			t.Errorf("change reported with equal GUIDs: %+v", c)
		}
	}

	err := StorageGUIDError("Sales.DomainModel", written, stored)
	if err == nil {
		t.Fatal("StorageGUIDError returned nil for a document that changes two GUIDs")
	}
	// The message has to name the unit and say what the consequence is — the
	// whole point is that the user would otherwise meet this as missing data
	// after a deploy, with nothing connecting it to an MDL edit.
	for _, want := range []string{"Sales.DomainModel", "mendixsystem$attribute.id", "DomainModels$Attribute"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message does not mention %q: %v", want, err)
		}
	}
}

// TestStorageGUIDChanges_QuietWhenNothingMoved: the guard must be silent on an
// unchanged document, and on one whose CONTENT changed while identities held.
func TestStorageGUIDChanges_QuietWhenNothingMoved(t *testing.T) {
	stored := dmDoc(t, 20, 3, 30, 4, 40)

	if got := StorageGUIDChanges(stored, stored); len(got) != 0 {
		t.Errorf("identical documents reported %d changes: %+v", len(got), got)
	}

	// An entity rename: same identities, different content.
	renamed := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Entities", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityImpl"},
				{Key: "$ID", Value: bin(2)},
				{Key: "GUID", Value: bin(20)},
				{Key: "Name", Value: "Renamed"},
			},
		}},
	})
	if got := StorageGUIDChanges(renamed, stored); len(got) != 0 {
		t.Errorf("a rename reported %d GUID changes: %+v", len(got), got)
	}
}

// TestStorageGUIDChanges_NewAndDroppedElementsAreNotChanges: an element the
// write adds has an $ID no stored element holds, and one it removes is simply
// absent. Neither is a GUID that moved, and reporting either would make the
// guard fire on every ADD ATTRIBUTE — which is how a guard gets switched off.
func TestStorageGUIDChanges_NewAndDroppedElementsAreNotChanges(t *testing.T) {
	stored := dmDoc(t, 20, 3, 30, 4, 40)

	// Same two attributes plus a third, with identities of its own.
	added := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Entities", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityImpl"},
				{Key: "$ID", Value: bin(2)},
				{Key: "GUID", Value: bin(20)},
				{Key: "Attributes", Value: bson.A{
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(3)},
						{Key: "GUID", Value: bin(30)},
					},
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(4)},
						{Key: "GUID", Value: bin(40)},
					},
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(5)},
						{Key: "GUID", Value: bin(50)},
					},
				}},
			},
		}},
	})
	if got := StorageGUIDChanges(added, stored); len(got) != 0 {
		t.Errorf("adding an attribute reported %d GUID changes: %+v", len(got), got)
	}

	// And dropping one: the survivor keeps its GUID, the removed one is gone.
	dropped := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Entities", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityImpl"},
				{Key: "$ID", Value: bin(2)},
				{Key: "GUID", Value: bin(20)},
				{Key: "Attributes", Value: bson.A{
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(3)},
						{Key: "GUID", Value: bin(30)},
					},
				}},
			},
		}},
	})
	if got := StorageGUIDChanges(dropped, stored); len(got) != 0 {
		t.Errorf("dropping an attribute reported %d GUID changes: %+v", len(got), got)
	}
}

// TestStorageGUIDChanges_OneSidedGUIDIsNotAChange: a GUID only one side carries
// is left alone in both directions. Inventing an optional property Studio Pro
// fills in on load is how a document becomes unopenable (CLAUDE.md on overlay
// writes), and the guard must not demand one be written.
func TestStorageGUIDChanges_OneSidedGUIDIsNotAChange(t *testing.T) {
	withGUID := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$Attribute"},
		{Key: "$ID", Value: bin(3)},
		{Key: "GUID", Value: bin(30)},
	})
	withoutGUID := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$Attribute"},
		{Key: "$ID", Value: bin(3)},
	})

	if got := StorageGUIDChanges(withoutGUID, withGUID); len(got) != 0 {
		t.Errorf("dropping the GUID key reported %d changes: %+v", len(got), got)
	}
	if got := StorageGUIDChanges(withGUID, withoutGUID); len(got) != 0 {
		t.Errorf("adding the GUID key reported %d changes: %+v", len(got), got)
	}
}

// TestStorageGUIDChanges_PointerIsNotAnElement: a 16-byte binary under some
// other key is a reference, and the document in these tests holds one that
// points at the entity. Treating it as an element would make the guard report
// on references rather than identities.
func TestStorageGUIDChanges_PointerIsNotAnElement(t *testing.T) {
	stored := dmDoc(t, 20, 3, 30, 4, 40)
	// Repoint the association at a different element. Only ParentPointer moves;
	// no GUID does.
	written := marshal(t, bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Associations", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$Association"},
				{Key: "$ID", Value: bin(9)},
				{Key: "GUID", Value: bin(90)},
				{Key: "Name", Value: "Org_Person"},
				{Key: "ParentPointer", Value: bin(7)},
			},
		}},
	})
	if got := StorageGUIDChanges(written, stored); len(got) != 0 {
		t.Errorf("repointing a reference reported %d GUID changes: %+v", len(got), got)
	}
}

// TestStorageGUIDChanges_UnreadableBytesDoNotBlockAWrite: the guard sits on the
// write path, so failing a write because it could not parse the bytes would be a
// worse failure than the one it prevents. Mirrors DuplicateElementIDs.
func TestStorageGUIDChanges_UnreadableBytesDoNotBlockAWrite(t *testing.T) {
	good := dmDoc(t, 20, 3, 30, 4, 40)
	garbage := []byte{0x01, 0x02, 0x03}

	if got := StorageGUIDChanges(garbage, good); got != nil {
		t.Errorf("unreadable contents reported changes: %+v", got)
	}
	if got := StorageGUIDChanges(good, garbage); got != nil {
		t.Errorf("unreadable stored bytes reported changes: %+v", got)
	}
	if err := StorageGUIDError("u", garbage, good); err != nil {
		t.Errorf("unreadable contents refused the write: %v", err)
	}
}

// TestStorageGUIDChanges_ReportsAreStable: the order must not depend on map
// iteration, or the error message reorders between runs.
func TestStorageGUIDChanges_ReportsAreStable(t *testing.T) {
	stored := dmDoc(t, 20, 3, 30, 4, 40)
	written := dmDoc(t, 21, 3, 3, 4, 4)

	first := StorageGUIDError("u", written, stored).Error()
	for i := 0; i < 20; i++ {
		if got := StorageGUIDError("u", written, stored).Error(); got != first {
			t.Fatalf("message not stable across runs:\n%s\n%s", first, got)
		}
	}
	// All three GUIDs moved, so all three are reported.
	if n := len(StorageGUIDChanges(written, stored)); n != 3 {
		t.Errorf("got %d changes, want 3 (entity + two attributes)", n)
	}
}
