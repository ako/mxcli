// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Re-minting an element's GUID while keeping its $ID makes the Mendix runtime's
// database synchroniser treat the element as deleted and re-added, so it drops
// and recreates the column and loses its data (issue #1119). Nothing else
// notices: the model stays valid, `mx check` passes, and because the new GUID is
// derived from a stable $ID the second identical write is elided.
//
// So the guard has to sit at the write choke point, and these tests go through
// the Writer rather than calling the detector — the wiring is the half that can
// silently come undone, which is what these cover. (Mirrors the duplicate-$ID
// guard's tests next door.)

// guidUnit builds a one-entity domain model whose entity carries an $ID and a
// separate GUID, so the two can be moved independently.
func guidUnit(t *testing.T, entityID, entityGUID, attrID, attrGUID string) []byte {
	t.Helper()
	bin := func(id string) bson.Binary {
		return bson.Binary{Subtype: 0x00, Data: uuidToBlob(id)}
	}
	b, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: bin("11111111-1111-1111-1111-111111111111")},
		{Key: "Entities", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityImpl"},
				{Key: "$ID", Value: bin(entityID)},
				{Key: "GUID", Value: bin(entityGUID)},
				{Key: "Name", Value: "Organization"},
				{Key: "Attributes", Value: bson.A{
					bson.D{
						{Key: "$Type", Value: "DomainModels$Attribute"},
						{Key: "$ID", Value: bin(attrID)},
						{Key: "GUID", Value: bin(attrGUID)},
						{Key: "Name", Value: "Name"},
					},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

const (
	guidEntityID   = "22222222-2222-2222-2222-222222222222"
	guidEntityGUID = "33333333-3333-3333-3333-333333333333"
	guidAttrID     = "44444444-4444-4444-4444-444444444444"
	guidAttrGUID   = "55555555-5555-5555-5555-555555555555"
)

func TestUpdateUnitRefusesAChangedStorageGUID(t *testing.T) {
	const unitID = "66666666-6666-6666-6666-666666666666"
	stored := guidUnit(t, guidEntityID, guidEntityGUID, guidAttrID, guidAttrGUID)
	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

	before, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read seeded unit: %v", err)
	}

	// Exactly what #1119 produced: the $IDs held (TransplantIDs put them back),
	// the attribute's GUID replaced by its own $ID.
	err = w.UpdateRawUnit(unitID, guidUnit(t, guidEntityID, guidEntityGUID, guidAttrID, guidAttrID))
	if err == nil {
		t.Fatal("write accepted a unit that changes an attribute's storage GUID")
	}
	for _, want := range []string{unitID, "DomainModels$Attribute", "mendixsystem$attribute.id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message missing %q: %v", want, err)
		}
	}

	// A guard that reports the problem after the bytes have landed is the
	// failure it exists to prevent.
	after, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit after refusal: %v", err)
	}
	if string(after) != string(before) {
		t.Error("the refused write still changed the stored unit")
	}
}

// The control, and the one that matters most: the guard must not refuse the
// ordinary writes. A guard that fires on a legitimate edit gets switched off,
// and then it protects nothing.
func TestUpdateUnitAcceptsOrdinaryEditsThatKeepGUIDs(t *testing.T) {
	bin := func(id string) bson.Binary {
		return bson.Binary{Subtype: 0x00, Data: uuidToBlob(id)}
	}

	cases := []struct {
		name string
		// contents is the document offered for writing against the same stored unit.
		contents func(t *testing.T) []byte
	}{
		{
			// Content changed, identities held — the shape of every correct ALTER.
			name: "RenameKeepingIdentities",
			contents: func(t *testing.T) []byte {
				b, err := bson.Marshal(bson.D{
					{Key: "$Type", Value: "DomainModels$DomainModel"},
					{Key: "$ID", Value: bin("11111111-1111-1111-1111-111111111111")},
					{Key: "Entities", Value: bson.A{
						bson.D{
							{Key: "$Type", Value: "DomainModels$EntityImpl"},
							{Key: "$ID", Value: bin(guidEntityID)},
							{Key: "GUID", Value: bin(guidEntityGUID)},
							{Key: "Name", Value: "Renamed"},
							{Key: "Attributes", Value: bson.A{
								bson.D{
									{Key: "$Type", Value: "DomainModels$Attribute"},
									{Key: "$ID", Value: bin(guidAttrID)},
									{Key: "GUID", Value: bin(guidAttrGUID)},
									{Key: "Name", Value: "RenamedAttr"},
								},
							}},
						},
					}},
				})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return b
			},
		},
		{
			// A new attribute, with identities of its own. This is ADD ATTRIBUTE,
			// and it must not read as a GUID that moved.
			name: "AddAttribute",
			contents: func(t *testing.T) []byte {
				b, err := bson.Marshal(bson.D{
					{Key: "$Type", Value: "DomainModels$DomainModel"},
					{Key: "$ID", Value: bin("11111111-1111-1111-1111-111111111111")},
					{Key: "Entities", Value: bson.A{
						bson.D{
							{Key: "$Type", Value: "DomainModels$EntityImpl"},
							{Key: "$ID", Value: bin(guidEntityID)},
							{Key: "GUID", Value: bin(guidEntityGUID)},
							{Key: "Name", Value: "Organization"},
							{Key: "Attributes", Value: bson.A{
								bson.D{
									{Key: "$Type", Value: "DomainModels$Attribute"},
									{Key: "$ID", Value: bin(guidAttrID)},
									{Key: "GUID", Value: bin(guidAttrGUID)},
									{Key: "Name", Value: "Name"},
								},
								bson.D{
									{Key: "$Type", Value: "DomainModels$Attribute"},
									{Key: "$ID", Value: bin("77777777-7777-7777-7777-777777777777")},
									{Key: "GUID", Value: bin("88888888-8888-8888-8888-888888888888")},
									{Key: "Name", Value: "Status"},
								},
							}},
						},
					}},
				})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return b
			},
		},
		{
			// DROP ATTRIBUTE: the removed attribute's GUID is absent, not moved.
			name: "DropAttribute",
			contents: func(t *testing.T) []byte {
				b, err := bson.Marshal(bson.D{
					{Key: "$Type", Value: "DomainModels$DomainModel"},
					{Key: "$ID", Value: bin("11111111-1111-1111-1111-111111111111")},
					{Key: "Entities", Value: bson.A{
						bson.D{
							{Key: "$Type", Value: "DomainModels$EntityImpl"},
							{Key: "$ID", Value: bin(guidEntityID)},
							{Key: "GUID", Value: bin(guidEntityGUID)},
							{Key: "Name", Value: "Organization"},
						},
					}},
				})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return b
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A distinct unit id per case so each starts from the same stored bytes.
			unitID := [...]string{
				"a0000000-0000-0000-0000-000000000001",
				"a0000000-0000-0000-0000-000000000002",
				"a0000000-0000-0000-0000-000000000003",
			}[i]
			stored := guidUnit(t, guidEntityID, guidEntityGUID, guidAttrID, guidAttrGUID)
			w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

			if err := w.UpdateRawUnit(unitID, tc.contents(t)); err != nil {
				t.Fatalf("guard refused an ordinary edit: %v", err)
			}
			// And it really wrote: a "pass" that elided the write proves nothing.
			after, err := os.ReadFile(unitPath)
			if err != nil {
				t.Fatalf("read unit: %v", err)
			}
			if string(after) == string(stored) {
				t.Error("the write did not land, so the guard was never exercised")
			}
		})
	}
}
