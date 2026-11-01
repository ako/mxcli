// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1640: a registered list's stored marker is the one Studio Pro
// writes, wherever the list sits; nothing else moves.

func init() {
	RegisterPropertyListMarker("Test1640$Bar", "Items", 2)
}

// unit1640 is a unit holding a registered list (marker given) deep inside an
// unregistered one, beside an unregistered list on the same owner.
func unit1640(items, other int32) bson.D {
	bar := bson.D{
		{Key: "$Type", Value: "Test1640$Bar"},
		{Key: "Items", Value: bson.A{items, bson.D{{Key: "$Type", Value: "Test1640$Item"}}}},
		{Key: "Others", Value: bson.A{other}},
	}
	return bson.D{
		{Key: "$Type", Value: "Test1640$Unit"},
		{Key: "Widgets", Value: bson.A{int32(3), bson.D{
			{Key: "$Type", Value: "Test1640$Widget"},
			{Key: "Value", Value: bson.D{{Key: "$Type", Value: "Test1640$Value"}, {Key: "SortBar", Value: bar}}},
		}}},
	}
}

func TestNormalizeListMarkers_GivesARegisteredListItsMarker(t *testing.T) {
	got := NormalizeListMarkers(marshal(t, unit1640(3, 3)))
	want := marshal(t, unit1640(2, 3))
	if string(got) != string(want) {
		t.Errorf("normalized:\n%v\nwant:\n%v", bson.Raw(got), bson.Raw(want))
	}
}

func TestNormalizeListMarkers_LeavesAMatchingUnitAlone(t *testing.T) {
	in := marshal(t, unit1640(2, 1))
	got := NormalizeListMarkers(in)
	if &got[0] != &in[0] {
		t.Error("a unit with nothing to normalize was re-encoded")
	}
}

func TestNormalizeListMarkers_LeavesAnUnregisteredListAlone(t *testing.T) {
	// Others is not registered, so 1 stays 1 although the encoder would
	// write 3 for a list it built.
	got := NormalizeListMarkers(marshal(t, unit1640(3, 1)))
	if string(got) != string(marshal(t, unit1640(2, 1))) {
		t.Errorf("normalized: %v", bson.Raw(got))
	}
}

func TestNormalizeListMarkers_IsTheEncodersTable(t *testing.T) {
	if m, ok := PropertyListMarker("Test1640$Bar", "Items"); !ok || m != 2 {
		t.Errorf("PropertyListMarker = %d, %v", m, ok)
	}
	if _, ok := PropertyListMarker("Test1640$Bar", "Others"); ok {
		t.Error("an unregistered list reads as registered")
	}
}
