// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// gridWithWidgetInAColumn is the BSON shape of a pluggable widget whose
// object-list ITEM holds widgets: a DataGrid2 column with custom content, a
// gallery item, a chart series. The item lives in `Value.Objects`, and each
// object carries its own `Properties[].Value.Widgets`.
func gridWithWidgetInAColumn() map[string]any {
	return map[string]any{
		"$ID":   "grid-1",
		"Name":  "grid1",
		"$Type": "CustomWidgets$CustomWidget",
		"Type":  map[string]any{"WidgetId": "com.mendix.widget.web.datagrid.Datagrid"},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t-columns",
					"Value": map[string]any{
						"Objects": []any{
							int32(3),
							map[string]any{
								"Properties": []any{
									int32(3),
									map[string]any{
										"TypePointer": "t-content",
										"Value": map[string]any{
											"Widgets": []any{
												int32(3),
												map[string]any{
													"$ID":   "pb-1",
													"Name":  "pb1",
													"$Type": "CustomWidgets$CustomWidget",
													"Type": map[string]any{
														"WidgetId": "com.mendix.widget.custom.progressbar.ProgressBar",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// A widget inside an object-list item must be indexed.
//
// The catalog walked a pluggable widget's `Object.Properties[].Value.Widgets`
// (a child slot) but not `Value.Objects[]` (an object list), so anything placed
// in a DataGrid2 column, a gallery item or a chart series was invisible.
//
// Measured on a real project by an external test: a page holding 19 chart
// sparklines inside datagrid columns did not appear under "which pages use
// VegaChart?", while the grid around them did. Reproduced here on the fixture —
// a progressbar in a `column c2 { … }` gave one row (the grid), ground truth two.
//
// It matters beyond the widget edge: CATALOG.REFS is a projection of this table,
// so an entity or microflow used ONLY inside a column template reported zero
// references, and anything using reference counts to decide "unused, safe to
// delete" would delete a document in active use. That is issue #940's failure
// mode, which was fixed for List View templates and left open here.
func TestExtractWidgetsRecursive_ObjectListItemWidgets(t *testing.T) {
	got := extractWidgetsRecursive(gridWithWidgetInAColumn())

	var sawGrid, sawNested bool
	for _, w := range got {
		switch w.WidgetType {
		case "com.mendix.widget.web.datagrid.Datagrid":
			sawGrid = true
		case "com.mendix.widget.custom.progressbar.ProgressBar":
			sawNested = true
		}
	}
	if !sawGrid {
		t.Error("the grid itself was not indexed")
	}
	if !sawNested {
		t.Errorf("the widget inside the column was not indexed; got %d widgets: %+v",
			len(got), got)
	}
}

// The control: a child slot (Value.Widgets, no Objects) must keep working. A fix
// that swapped one traversal for the other would pass the test above and lose
// every Gallery content widget.
func TestExtractWidgetsRecursive_ChildSlotStillWalked(t *testing.T) {
	w := map[string]any{
		"$ID": "g-1", "Name": "gal1", "$Type": "CustomWidgets$CustomWidget",
		"Type": map[string]any{"WidgetId": "com.mendix.widget.web.gallery.Gallery"},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t-content",
					"Value": map[string]any{
						"Widgets": []any{
							int32(3),
							map[string]any{"$ID": "t-1", "Name": "txt1", "$Type": "Forms$DynamicText"},
						},
					},
				},
			},
		},
	}
	var sawText bool
	for _, got := range extractWidgetsRecursive(w) {
		if got.WidgetType == "Forms$DynamicText" {
			sawText = true
		}
	}
	if !sawText {
		t.Error("a widget in a child slot stopped being indexed")
	}
}

// The second control: an object list with no widgets in it must not invent rows,
// and must not panic on the marker-only array. getBsonArrayElements strips the
// leading typed-array marker, so an empty list is length 0 here and length 1 raw.
func TestExtractWidgetsRecursive_EmptyObjectList(t *testing.T) {
	w := map[string]any{
		"$ID": "g-1", "Name": "c1", "$Type": "CustomWidgets$CustomWidget",
		"Type": map[string]any{"WidgetId": "com.acme.Thing"},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t",
					"Value":       map[string]any{"Objects": []any{int32(3)}},
				},
			},
		},
	}
	if got := extractWidgetsRecursive(w); len(got) != 1 {
		t.Errorf("got %d widgets, want 1 (the widget itself): %+v", len(got), got)
	}
}
