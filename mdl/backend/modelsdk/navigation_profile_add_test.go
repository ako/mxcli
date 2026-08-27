// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §7: mxcli could not create a navigation profile, so a
// device-specific front end could not be built from MDL at all — Mendix routes a
// phone to a Phone PROFILE on User-Agent, not on viewport width.
//
// The shape below is pinned against Studio Pro 11.14 over PED, measured by adding
// a profile through Studio Pro's OWN model API and reading back what it filled
// in — not by copying a profile the reference project already had configured.
// That distinction cost a bug: the reference Phone profile carries PWA settings,
// but a profile Studio Pro has just created does not.
package modelsdkbackend

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
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
	// whatever its kind — an offline profile included. A key mxcli invents, or one
	// it omits, is the failure that passes mx check and then will not open in
	// Studio Pro.
	want := []string{
		"$ID", "$Type", "AppIcon", "AppTitle", "HomeItems", "HomePage", "Kind",
		"LoginPageSettings", "Menu", "Name", "NotFoundHomepage",
		"OfflineEntityConfigs", "ProgressiveWebAppSettings", "ThrowPartialSyncError",
	}
	for _, kind := range types.WebProfileKindNames() {
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

func TestOfflineProfileIsStructurallyIdenticalToItsOnlineTwin(t *testing.T) {
	// Measured over PED: a freshly created PhoneOffline differs from a Phone in
	// Kind and Name and in nothing else. If that ever stops being true the offline
	// kinds need their own builder, and this test is what says so.
	for _, tc := range []struct{ online, offline string }{
		{"Responsive", "ResponsiveOffline"},
		{"Phone", "PhoneOffline"},
		{"Tablet", "TabletOffline"},
	} {
		on, off := newWebProfileBson(tc.online), newWebProfileBson(tc.offline)
		if len(on) != len(off) {
			t.Fatalf("%s/%s: key counts differ (%d vs %d)", tc.online, tc.offline, len(on), len(off))
		}
		for i := range on {
			if on[i].Key != off[i].Key {
				t.Errorf("key %d: %s has %q, %s has %q", i, tc.online, on[i].Key, tc.offline, off[i].Key)
				continue
			}
			switch on[i].Key {
			case "$ID", "Kind", "Name":
				continue // $ID is minted per call; Kind and Name are the difference.
			}
			if fmt.Sprintf("%v", on[i].Value) != fmt.Sprintf("%v", off[i].Value) {
				// $ID appears inside sub-elements too, so compare only the keys we
				// can compare — a sub-document differing solely by minted IDs is
				// expected and not a finding.
				if _, isDoc := on[i].Value.(bson.D); isDoc {
					continue
				}
				t.Errorf("%s differs: %s=%v, %s=%v", on[i].Key, tc.online, on[i].Value, tc.offline, off[i].Value)
			}
		}
	}
}

func TestNoKindCarriesProgressiveWebAppSettings(t *testing.T) {
	// The reference project's Phone and TabletOffline profiles BOTH store
	// {Precaching: true, InstallPrompt: true}, and copying that was the bug. The
	// schema defaults Precaching to false and makes the whole element optional, and
	// a profile added through Studio Pro's own API materialises null — so those
	// values are the user's, not the platform's. Writing them would turn precaching
	// on for every profile mxcli creates.
	for _, kind := range types.WebProfileKindNames() {
		v, ok := profileKey(newWebProfileBson(kind), "ProgressiveWebAppSettings")
		if !ok {
			t.Errorf("%s: no ProgressiveWebAppSettings key at all", kind)
			continue
		}
		if v != nil {
			t.Errorf("%s: ProgressiveWebAppSettings = %#v, want an explicit null", kind, v)
		}
	}
}

func TestOfflineProfilesStartWithNoEntityConfigs(t *testing.T) {
	// Studio Pro DERIVES the offline entity list from the pages the profile
	// reaches; only rows that deviate from the defaults are stored. An empty
	// collection is therefore the correct starting state, not an incomplete one —
	// measured on a freshly created PhoneOffline, whose offlineEntityConfigs is [].
	for _, kind := range []string{"ResponsiveOffline", "PhoneOffline", "TabletOffline"} {
		v, _ := profileKey(newWebProfileBson(kind), "OfflineEntityConfigs")
		arr, ok := v.(bson.A)
		if !ok {
			t.Fatalf("%s: OfflineEntityConfigs is %T, want bson.A", kind, v)
		}
		// Just the typed-array marker, no entries.
		if len(arr) != 1 {
			t.Errorf("%s: OfflineEntityConfigs has %d entries beyond the marker, want 0", kind, len(arr)-1)
		}
	}
}
