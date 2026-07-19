// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// TestReconcilePropertyMetadata verifies that augment overwrites an existing
// PropertyType's Category, Caption, and ValueType DefaultValue from the installed
// .mpk. This closes the within-key definition drift behind the marketplace-updated
// widget CE0463 (issue #600 / Gallery@10.24): reconcileEnumValues rebuilds an enum's
// OPTION SET but leaves a stale DefaultValue (e.g. pagingPosition options reconcile to
// {below,above} while the default stays "bottom" — a value the widget no longer
// defines), and Category drifts across widget versions (General::Pagination →
// General::Items). Confirmed empirically: after this reconciliation every Gallery@10.24
// PropertyType matches what `mx update-widgets` produces.
func TestReconcilePropertyMetadata(t *testing.T) {
	// A PropertyType carrying stale metadata from an older template extraction.
	typ := map[string]any{
		"$Type": "CustomWidgets$ObjectType",
		"PropertyTypes": []any{float64(2),
			map[string]any{
				"$Type":       "CustomWidgets$WidgetPropertyType",
				"PropertyKey": "pagingPosition",
				"Category":    "General::Pagination",   // stale
				"Caption":     "Position of pagination", // stale
				"ValueType": map[string]any{
					"$Type":        "CustomWidgets$WidgetValueType",
					"Type":         "Enumeration",
					"DefaultValue": "bottom", // stale — not in the installed option set
				},
			},
		},
	}
	byKey := map[string]mpk.PropertyDef{
		"pagingPosition": {
			Key:          "pagingPosition",
			Category:     "General::Items",
			Caption:      "Position of paging buttons",
			DefaultValue: "below",
		},
	}

	reconcilePropertyMetadata(typ, byKey)

	pt := typ["PropertyTypes"].([]any)[1].(map[string]any)
	if got := pt["Category"]; got != "General::Items" {
		t.Errorf("Category = %v, want General::Items", got)
	}
	if got := pt["Caption"]; got != "Position of paging buttons" {
		t.Errorf("Caption = %v, want 'Position of paging buttons'", got)
	}
	vt := pt["ValueType"].(map[string]any)
	if got := vt["DefaultValue"]; got != "below" {
		t.Errorf("DefaultValue = %v, want below (a value in the installed option set)", got)
	}
}

// TestReconcilePropertyMetadata_LeavesUnknownKeys verifies a PropertyType whose key
// the .mpk does not define is left untouched (no false rewrites), and that an empty
// .mpk value does not clobber a template value.
func TestReconcilePropertyMetadata_LeavesUnknownKeys(t *testing.T) {
	typ := map[string]any{
		"$Type":       "CustomWidgets$WidgetPropertyType",
		"PropertyKey": "somethingElse",
		"Category":    "General::Custom",
		"ValueType":   map[string]any{"$Type": "CustomWidgets$WidgetValueType", "DefaultValue": "keep"},
	}
	// byKey has the key but with empty DefaultValue — must not clobber "keep".
	byKey := map[string]mpk.PropertyDef{
		"somethingElse": {Key: "somethingElse", Category: "General::New", DefaultValue: ""},
	}
	reconcilePropertyMetadata(typ, byKey)
	if got := typ["Category"]; got != "General::New" {
		t.Errorf("Category = %v, want General::New (present in .mpk)", got)
	}
	if got := typ["ValueType"].(map[string]any)["DefaultValue"]; got != "keep" {
		t.Errorf("DefaultValue = %v, want keep (empty .mpk value must not clobber)", got)
	}
}

// TestReorderPropertyTypes verifies the top-level PropertyTypes are reordered to the
// installed .mpk's declaration order (leading array marker preserved, keys absent from
// the .mpk kept after the declared ones), closing the order axis of the object-list
// CE0463 drift (e.g. Gallery 3.x moved pagingPosition ahead of showTotalCount).
func TestReorderPropertyTypes(t *testing.T) {
	pt := func(key string) map[string]any {
		return map[string]any{"$Type": "CustomWidgets$WidgetPropertyType", "PropertyKey": key}
	}
	typ := map[string]any{
		"ObjectType": map[string]any{
			// template order: c, a, sys, b  — marker 2 leads the list
			"PropertyTypes": []any{float64(2), pt("c"), pt("a"), pt("sys"), pt("b")},
		},
	}
	def := &mpk.WidgetDefinition{
		Properties: []mpk.PropertyDef{{Key: "a"}, {Key: "b"}, {Key: "c"}},
	}
	reorderPropertyTypes(typ, def)

	got := typ["ObjectType"].(map[string]any)["PropertyTypes"].([]any)
	if m, ok := got[0].(float64); !ok || m != 2 {
		t.Fatalf("marker not preserved at head: %v", got[0])
	}
	var order []string
	for _, e := range got[1:] {
		order = append(order, e.(map[string]any)["PropertyKey"].(string))
	}
	// declared keys in .mpk order (a,b,c), then undeclared "sys" kept after
	want := []string{"a", "b", "c", "sys"}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order = %v, want %v", order, want)
			break
		}
	}
}
