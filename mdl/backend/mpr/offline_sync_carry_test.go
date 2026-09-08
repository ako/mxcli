// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// The properties Studio Pro writes on a web profile's offline entity config,
// measured against ako/TestApp's TabletOffline profile: seven configs spanning
// all six sync modes.
//
// CompatibilityMode is why this test exists. The semantic model carried three
// of the four written properties, so it was read and discarded — and a write
// path built on that model would have dropped it the way `create or modify
// entity` dropped access rules. Nothing else would have said so: the document
// stays valid, and `mx check` reports 0 errors either way.
func TestOfflineEntityConfigCarriesEveryStoredProperty(t *testing.T) {
	// One case per observed sync mode, so a mode that stops round-tripping is
	// named rather than hidden in a total.
	cases := []*mpr.NavOfflineEntity{
		{Entity: "Mappings.Customer", SyncMode: "Never"},
		{Entity: "Pages.Bus", SyncMode: "All"},
		{Entity: "Rules.BusinessRule", SyncMode: "Online"},
		{Entity: "Rules.RuleAction", SyncMode: "Constrained",
			Constraint: "[\n  (\n    contains(ActionValue, '''abc''')\n  )\n]"},
		{Entity: "Rules.RuleCategory", SyncMode: "None"},
		{Entity: "Rules.RuleExecutionLog", SyncMode: "NoneAndPreserveData"},
		{Entity: "System.Language", SyncMode: "All"},
		// Not present in any reference config — every one carries false — but
		// carrying only the false case would prove nothing about a bool.
		{Entity: "Mod.Compat", SyncMode: "All", CompatibilityMode: true},
	}

	for _, want := range cases {
		t.Run(want.Entity+"/"+want.SyncMode, func(t *testing.T) {
			in := &mpr.NavigationDocument{
				Name: "Navigation",
				Profiles: []*mpr.NavigationProfile{{
					Name: "TabletOffline", Kind: "TabletOffline",
					OfflineEntities: []*mpr.NavOfflineEntity{want},
				}},
			}
			out := convertNavDoc(in)
			if len(out.Profiles) != 1 || len(out.Profiles[0].OfflineEntities) != 1 {
				t.Fatalf("profile or config lost in conversion: %+v", out)
			}
			if got := *out.Profiles[0].OfflineEntities[0]; got != *want {
				t.Errorf("conversion dropped a property:\n got %+v\nwant %+v", got, *want)
			}
		})
	}
}
