// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeListViewTemplateCarriesTheSpecialization pins the legacy engine's
// half of #940.
//
// The writer already emitted Forms$ListViewTemplate elements, but with only
// {$ID, $Type, Widgets} — no entity — so every template it wrote matched nothing
// and rendered never. Studio Pro's own documents (ako/TestApp,
// Pages.Vehicle_Overview) carry the entity under the storage name "Entity", not
// the SDK name "Specialization".
func TestSerializeListViewTemplateCarriesTheSpecialization(t *testing.T) {
	lv := &pages.ListView{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: model.ID("lv"), TypeName: "Forms$ListView"},
			Name:        "vehicleListView",
		},
		Templates: []*pages.ListViewTemplate{
			{BaseElement: model.BaseElement{ID: model.ID("t1")}, Specialization: "Pages.Bus"},
			{BaseElement: model.BaseElement{ID: model.ID("t2")}, Specialization: "Pages.Truck"},
		},
	}

	doc := serializeListView(lv)

	var templates bson.A
	for _, e := range doc {
		if e.Key == "Templates" {
			templates, _ = e.Value.(bson.A)
		}
	}
	// The first element is the typed-array marker, not a template.
	if len(templates) != 3 {
		t.Fatalf("Templates has %d element(s) (marker + templates), want 3", len(templates))
	}

	want := []string{"Pages.Bus", "Pages.Truck"}
	for i, wantEntity := range want {
		tpl, ok := templates[i+1].(bson.D)
		if !ok {
			t.Fatalf("template %d is %T, want bson.D", i, templates[i+1])
		}
		var got string
		var keys []string
		for _, e := range tpl {
			keys = append(keys, e.Key)
			if e.Key == "Entity" {
				got, _ = e.Value.(string)
			}
		}
		if got != wantEntity {
			t.Errorf("template %d Entity = %q, want %q (keys present: %v)", i, got, wantEntity, keys)
		}
		// Key order matches Studio Pro's documents.
		if len(keys) != 4 || keys[0] != "$ID" || keys[1] != "$Type" || keys[2] != "Entity" || keys[3] != "Widgets" {
			t.Errorf("template %d keys = %v, want [$ID $Type Entity Widgets]", i, keys)
		}
	}
}
