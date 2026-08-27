// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §7: mxcli could not create a navigation profile, so a
// device-specific front end could not be built from MDL at all — Mendix routes a
// phone to a Phone PROFILE on User-Agent, not on viewport width.
//
// The shape below is pinned against a Studio Pro 11.14 document carrying all
// three web profiles, read over PED. It is NOT derived from the Responsive
// sibling, which is how this was first going to be written and would have been
// wrong: only the Phone profile carries ProgressiveWebAppSettings.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func profileKey(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

func TestNewWebProfileMatchesTheStudioProKeySet(t *testing.T) {
	// Every profile in the reference document stores the same fourteen keys,
	// whatever its kind. A key mxcli invents, or one it omits, is the failure
	// that passes mx check and then will not open in Studio Pro.
	want := []string{
		"$ID", "$Type", "AppIcon", "AppTitle", "HomeItems", "HomePage", "Kind",
		"LoginPageSettings", "Menu", "Name", "NotFoundHomepage",
		"OfflineEntityConfigs", "ProgressiveWebAppSettings", "ThrowPartialSyncError",
	}
	for _, kind := range []string{"Responsive", "Phone", "Tablet"} {
		got := newWebProfileBson(kind)
		if len(got) != len(want) {
			t.Errorf("%s: %d keys, want %d", kind, len(got), len(want))
		}
		for _, k := range want {
			if _, ok := profileKey(got, k); !ok {
				t.Errorf("%s: missing key %q", kind, k)
			}
		}
		if v, _ := profileKey(got, "Kind"); v != kind {
			t.Errorf("%s: Kind = %v", kind, v)
		}
		if v, _ := profileKey(got, "Name"); v != kind {
			t.Errorf("%s: Name = %v", kind, v)
		}
	}
}

func TestOnlyPhoneCarriesProgressiveWebAppSettings(t *testing.T) {
	// The asymmetry that makes a profile something to build per kind rather than
	// copy from a sibling. Measured on the reference: Phone has the settings,
	// Responsive and Tablet store null.
	for _, tc := range []struct {
		kind    string
		wantPWA bool
	}{
		{"Phone", true},
		{"Responsive", false},
		{"Tablet", false},
	} {
		v, ok := profileKey(newWebProfileBson(tc.kind), "ProgressiveWebAppSettings")
		if !ok {
			t.Fatalf("%s: no ProgressiveWebAppSettings key at all", tc.kind)
		}
		got, isDoc := v.(bson.D)
		if tc.wantPWA != isDoc {
			t.Errorf("%s: ProgressiveWebAppSettings present = %v, want %v", tc.kind, isDoc, tc.wantPWA)
			continue
		}
		if !tc.wantPWA {
			if v != nil {
				t.Errorf("%s: want an explicit null, got %#v", tc.kind, v)
			}
			continue
		}
		for _, k := range []string{"InstallPrompt", "Precaching"} {
			if b, _ := profileKey(got, k); b != true {
				t.Errorf("Phone: %s = %v, want true", k, b)
			}
		}
	}
}
