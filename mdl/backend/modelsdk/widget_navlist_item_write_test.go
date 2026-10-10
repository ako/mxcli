// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#950: Studio Pro stores a navigation-list item with no Name key and
// a null ConditionalVisibilitySettings (six of six in ako/TestApp at 11.14.0).
// Writing `Name: ""` and no visibility slot made a describe → exec of
// Rules.Entity_Menu rewrite the snippet. A named item no longer writes its
// Name either: Forms$NavigationListItem declares none, and `mx convert` strips
// it on 10.24 and 11.14 — a key Mendix's merge engine then cannot compare
// across revisions (mendixlabs/mxcli#1373).
//
// The bare encoder here leaves an empty Name out either way, so the Name half
// is proven on the stored unit by the TestApp round trip
// (TestTestAppRoundTrip/snippet_Rules.Entity_Menu fails with "Name: added"
// without the guard); this test pins the visibility slot and the named case.
func TestNavListItemToGen_StudioProShape(t *testing.T) {
	for _, c := range []struct {
		name    string
		hasName bool
	}{{"", false}, {"i1", false}} {
		el, err := navListItemToGen(&pages.NavigationListItem{
			BaseElement: model.BaseElement{ID: "item-1"},
			Name:        c.name,
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&codec.Encoder{}).Encode(el)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.M
		if err := bson.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		if _, ok := doc["Name"]; ok != c.hasName {
			t.Errorf("item named %q: Name key present = %v, want %v", c.name, ok, c.hasName)
		}
		cvs, ok := doc["ConditionalVisibilitySettings"]
		if !ok || cvs != nil {
			t.Errorf("item named %q: ConditionalVisibilitySettings = %v (present %v), want null", c.name, cvs, ok)
		}
	}
}
