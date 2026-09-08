// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson"
)

func offlineCfg(entity, mode, constraint string, compat bool) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Navigation$OfflineEntityConfig"},
		{Key: "CompatibilityMode", Value: compat},
		{Key: "Constraint", Value: constraint},
		{Key: "Entity", Value: entity},
		{Key: "SyncMode", Value: mode},
	}
}

func cfgMap(t *testing.T, v any) map[string]any {
	t.Helper()
	d, ok := v.(bson.D)
	if !ok {
		t.Fatalf("not a document: %T", v)
	}
	return d.Map()
}

// The property MDL cannot spell must survive a rewrite that never mentions it.
//
// Every config in the reference document (ako/TestApp) carries
// CompatibilityMode false, so the true case cannot be observed there — and a
// writer that always emitted false would look correct against all seven. This
// is the test that distinguishes them, and it is the guard-don't-drop rule:
// building the element from the spec alone clears the property silently,
// because the document stays valid and mx check reports 0 errors either way.
func TestOfflineWriteCarriesCompatibilityModeThroughARewrite(t *testing.T) {
	stored := bson.A{
		navMarkerItems,
		offlineCfg("Rules.RuleAction", "All", "", true),
		offlineCfg("Pages.Bus", "All", "", false),
	}
	// The spec rewrites both entities and says nothing about compatibility mode.
	specs := []types.NavOfflineEntitySpec{
		{Entity: "Rules.RuleAction", SyncMode: "Never"},
		{Entity: "Pages.Bus", SyncMode: "Online"},
	}

	out := navOfflineConfigs(stored, specs)
	if len(out) != 3 {
		t.Fatalf("expected marker + 2 configs, got %d entries", len(out))
	}
	got := cfgMap(t, out[1])
	if got["Entity"] != "Rules.RuleAction" {
		t.Fatalf("order not preserved: %v", got["Entity"])
	}
	if got["CompatibilityMode"] != true {
		t.Error("CompatibilityMode was dropped by a rewrite that never mentioned it")
	}
	if got["SyncMode"] != "Never" {
		t.Errorf("SyncMode = %v, want the spec's Never", got["SyncMode"])
	}
	// Control, same shape in the other direction: an entity stored as false
	// stays false, so the carry is reading the stored value rather than
	// defaulting to true.
	if second := cfgMap(t, out[2]); second["CompatibilityMode"] != false {
		t.Errorf("false was not carried either: %v", second["CompatibilityMode"])
	}
}

// An entity the spec ADDS has no stored config to carry from, and takes the
// value every reference config holds.
func TestOfflineWriteDefaultsANewEntityToCompatibilityModeOff(t *testing.T) {
	out := navOfflineConfigs(bson.A{navMarkerItems}, []types.NavOfflineEntitySpec{
		{Entity: "Mod.Fresh", SyncMode: "All"},
	})
	if got := cfgMap(t, out[1]); got["CompatibilityMode"] != false {
		t.Errorf("a new entity must default to false, got %v", got["CompatibilityMode"])
	}
}

// gen declares DownloadMode and ShouldDownload; ako/TestApp writes neither.
// Emitting a property Studio Pro fills in on load is how a document mxbuild
// accepts becomes one Studio Pro cannot open, so the writer must stay silent
// about them — and the typed-array marker must be 3, as the reference has.
func TestOfflineWriteEmitsExactlyThePropertiesStudioProWrites(t *testing.T) {
	out := navOfflineConfigs(bson.A{navMarkerItems},
		[]types.NavOfflineEntitySpec{{Entity: "Mod.E", SyncMode: "Constrained", Constraint: "[X = 1]"}})

	if out[0] != navMarkerItems {
		t.Errorf("typed-array marker = %v, want %v", out[0], navMarkerItems)
	}
	got := cfgMap(t, out[1])
	for _, absent := range []string{"DownloadMode", "ShouldDownload"} {
		if _, present := got[absent]; present {
			t.Errorf("%s must not be written — it occurs zero times in every reference document", absent)
		}
	}
	for _, required := range []string{"$Type", "CompatibilityMode", "Constraint", "Entity", "SyncMode"} {
		if _, present := got[required]; !present {
			t.Errorf("%s missing from the written config", required)
		}
	}
	if len(got) != 6 { // the five above plus $ID
		t.Errorf("wrote %d properties (%v); Studio Pro writes four plus $ID and $Type", len(got), got)
	}
}
