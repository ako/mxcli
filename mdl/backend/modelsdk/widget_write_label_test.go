// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A Forms$Label as Studio Pro stores it — measured on the three in a stock
// Administration v4.3.2 + Feedback v4.0.2 project at Mendix 11.13.0 — has
// exactly these top-level keys. gen's Label type ALSO declares top-level Class,
// Style and AccessibilitySettings (older metamodel versions); writing those
// would be keys Studio Pro does not store.
func TestWidgetToGen_Label(t *testing.T) {
	el, err := widgetToGen(&pages.Label{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: "lbl-id", TypeName: "Forms$Label"},
			Name:        "label4",
			Class:       "text-semibold",
		},
		Caption: &model.Text{Translations: map[string]string{"en_US": "Attachment"}},
	})
	if err != nil {
		t.Fatalf("widgetToGen refused a Label: %v", err)
	}
	d := encodeToD(t, el)
	var keys []string
	for _, e := range d {
		if e.Key != "$ID" {
			keys = append(keys, e.Key)
		}
	}
	sort.Strings(keys)
	want := []string{"$Type", "Appearance", "Caption", "ConditionalVisibilitySettings", "Name", "TabIndex"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
	for _, e := range d {
		switch e.Key {
		case "$Type":
			if e.Value != "Forms$Label" {
				t.Errorf("$Type = %v", e.Value)
			}
		case "Name":
			if e.Value != "label4" {
				t.Errorf("Name = %v", e.Value)
			}
		}
	}
}
