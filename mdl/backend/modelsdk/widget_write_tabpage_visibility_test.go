// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1465: a tab page's visibility condition is stored in its own
// Forms$TabPage.ConditionalVisibilitySettings (the type carries one; it has no
// Appearance). The writer used to emit only name, caption and widgets, so
// `Visible:` on a tab page reached the model as nothing. Control: a tab page
// without a condition gets none.
func TestTabPage_WritesConditionalVisibility(t *testing.T) {
	tc := &pages.TabContainer{
		BaseWidget: pages.BaseWidget{Name: "tc"},
		TabPages: []*pages.TabPage{
			{Name: "tpA"},
			{Name: "tpB", ConditionalVisibility: &pages.ConditionalVisibilitySettings{Expression: "$currentObject/ShowTabB"}},
		},
	}
	el, err := widgetToGen(tc)
	if err != nil {
		t.Fatal(err)
	}
	tps := subDocs(t, encodeElement(t, el), "TabPages")
	field := func(doc bsonv1.D, key string) (any, bool) {
		for _, e := range doc {
			if e.Key == key {
				return e.Value, true
			}
		}
		return nil, false
	}
	// Absent here; canon.CompletePropertySets adds Studio Pro's null on write.
	if v, _ := field(tps[0], "ConditionalVisibilitySettings"); v != nil {
		t.Errorf("tpA: ConditionalVisibilitySettings = %v, want none", v)
	}
	v, _ := field(tps[1], "ConditionalVisibilitySettings")
	cvs, ok := v.(bsonv1.D)
	if !ok {
		t.Fatalf("tpB: ConditionalVisibilitySettings = %v, want the settings — the condition was dropped (tab always visible)", v)
	}
	if typ, _ := field(cvs, "$Type"); typ != "Forms$ConditionalVisibilitySettings" {
		t.Errorf("tpB: settings $Type = %v", typ)
	}
	if expr, _ := field(cvs, "Expression"); expr != "$currentObject/ShowTabB" {
		t.Errorf("tpB: Expression = %v, want $currentObject/ShowTabB", expr)
	}
}
