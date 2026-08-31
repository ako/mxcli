// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson"
)

// markerOf returns the leading typed-array marker of the named array field.
func markerOf(t *testing.T, d bson.D, key string) int32 {
	t.Helper()
	for _, e := range d {
		if e.Key != key {
			continue
		}
		a, ok := e.Value.(bson.A)
		if !ok || len(a) == 0 {
			t.Fatalf("%s is not a typed array: %#v", key, e.Value)
		}
		m, ok := a[0].(int32)
		if !ok {
			t.Fatalf("%s has no int32 marker: %#v", key, a[0])
		}
		return m
	}
	t.Fatalf("no %s field", key)
	return 0
}

func childDoc(t *testing.T, d bson.D, key string) bson.D {
	t.Helper()
	for _, e := range d {
		if e.Key == key {
			c, ok := e.Value.(bson.D)
			if !ok {
				t.Fatalf("%s is not a document: %#v", e.Value, e.Value)
			}
			return c
		}
	}
	t.Fatalf("no %s field", key)
	return nil
}

// Every list these writers emit carries the marker Studio Pro writes for that
// field. The values are a census over 19,078 unit files in 54 projects; the
// rationale, counts and the one open case (HomeItems) are on the navMarker*
// constants. These writers previously emitted 1 for all of them, which no
// Studio Pro document uses for these fields -- though 1 is a legitimate marker
// elsewhere in the metamodel, so this is a per-field mismatch, not an invalid
// value.
func TestNavWriters_TypedArrayMarkersMatchStudioPro(t *testing.T) {
	settings := navFormSettingsBson("M.Page")
	if got := markerOf(t, settings, "ParameterMappings"); got != 2 {
		t.Errorf("Forms$FormSettings.ParameterMappings marker = %d, want 2 (1122 documents)", got)
	}

	action := navMenuAction(types.NavMenuItemSpec{Page: "M.Page"})
	if got := markerOf(t, action, "PagesForSpecializations"); got != 2 {
		t.Errorf("Forms$FormAction.PagesForSpecializations marker = %d, want 2 (357 documents)", got)
	}

	item := navMenuItemBson(types.NavMenuItemSpec{Caption: "Home", Page: "M.Page"})
	if got := markerOf(t, item, "Items"); got != 3 {
		t.Errorf("Menus$MenuItem.Items marker = %d, want 3 (459 documents)", got)
	}
	if got := markerOf(t, childDoc(t, item, "Caption"), "Items"); got != 3 {
		t.Errorf("Texts$Text.Items marker = %d, want 3 (169,486 documents)", got)
	}
}

// The profile-level lists come from the patch path, so they are asserted on the
// document the writer produces rather than on a builder.
func TestUpdateNavigationProfile_TypedArrayMarkers(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	nav, err := b.GetNavigation()
	if err != nil || nav == nil || len(nav.Profiles) == 0 {
		t.Skipf("no navigation profiles in fixture: %v", err)
	}
	if err := b.UpdateNavigationProfile(nav.ID, nav.Profiles[0].Name, types.NavigationProfileSpec{
		HomePages: []types.NavHomePageSpec{{IsPage: true, Target: "MyFirstModule.Home"}},
		HasMenu:   true,
		MenuItems: []types.NavMenuItemSpec{{Caption: "Home", Page: "MyFirstModule.Home"}},
	}); err != nil {
		t.Fatalf("UpdateNavigationProfile: %v", err)
	}

	raw, err := b.reader.GetRawUnitBytes(string(nav.ID))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	profiles, _ := doc.Map()["Profiles"].(bson.A)
	var prof bson.D
	for _, p := range profiles {
		if d, ok := p.(bson.D); ok {
			prof = d
			break
		}
	}
	if prof == nil {
		t.Fatal("no profile document")
	}
	if got := markerOf(t, prof, "HomeItems"); got != 2 {
		t.Errorf("Navigation$NavigationProfile.HomeItems marker = %d, want 2 (51 documents)", got)
	}
	if got := markerOf(t, childDoc(t, prof, "Menu"), "Items"); got != 3 {
		t.Errorf("Menus$MenuItemCollection.Items marker = %d, want 3 (153 documents)", got)
	}
}
