// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// The legacy engine must behave identically to modelsdk here. Two writers that
// drift apart is how an engine-specific defect hides: a project written on one
// engine and rewritten on the other would lose the property on exactly one of
// the two paths, and nothing reports it.
func TestLegacyOfflineWriteCarriesCompatibilityMode(t *testing.T) {
	stored := bson.A{
		navMarkerItems,
		bson.D{
			{Key: "$Type", Value: "Navigation$OfflineEntityConfig"},
			{Key: "CompatibilityMode", Value: true},
			{Key: "Entity", Value: "Rules.RuleAction"},
			{Key: "SyncMode", Value: "All"},
		},
	}
	out := buildOfflineConfigsBson(stored, []NavOfflineEntitySpec{
		{Entity: "Rules.RuleAction", SyncMode: "Never"},
	})
	if len(out) != 2 {
		t.Fatalf("expected marker + 1 config, got %d", len(out))
	}
	got := out[1].(bson.D).Map()
	if got["CompatibilityMode"] != true {
		t.Error("legacy dropped CompatibilityMode on a rewrite that never mentioned it")
	}
	if got["SyncMode"] != "Never" {
		t.Errorf("SyncMode = %v, want Never", got["SyncMode"])
	}
}

// The reader hands back map[string]any rather than bson.D on some paths, and a
// carry that only understood one shape would silently default to false for the
// other — which is the shape the legacy parser actually produces.
func TestLegacyOfflineWriteReadsEitherStoredShape(t *testing.T) {
	for name, stored := range map[string]bson.A{
		"bson.D": {navMarkerItems, bson.D{
			{Key: "CompatibilityMode", Value: true}, {Key: "Entity", Value: "Mod.E"}}},
		"map": {navMarkerItems, map[string]any{
			"CompatibilityMode": true, "Entity": "Mod.E"}},
	} {
		t.Run(name, func(t *testing.T) {
			out := buildOfflineConfigsBson(stored, []NavOfflineEntitySpec{{Entity: "Mod.E", SyncMode: "All"}})
			if got := out[1].(bson.D).Map(); got["CompatibilityMode"] != true {
				t.Errorf("carry failed for a stored config shaped as %s", name)
			}
		})
	}
}

func TestLegacyOfflineWriteEmitsTheSamePropertiesAsModelsdk(t *testing.T) {
	out := buildOfflineConfigsBson(bson.A{navMarkerItems},
		[]NavOfflineEntitySpec{{Entity: "Mod.E", SyncMode: "All"}})
	if out[0] != navMarkerItems {
		t.Errorf("marker = %v, want %v", out[0], navMarkerItems)
	}
	got := out[1].(bson.D).Map()
	for _, absent := range []string{"DownloadMode", "ShouldDownload"} {
		if _, present := got[absent]; present {
			t.Errorf("%s must not be written", absent)
		}
	}
	if len(got) != 6 {
		t.Errorf("wrote %d properties (%v), want $ID + $Type + the four Studio Pro writes", len(got), got)
	}
}
