// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

// contextDataView returns a parameter-bound (edit/new-form) DataView BSON node.
func contextDataView(name string) map[string]any {
	return map[string]any{
		"$Type": "Forms$DataView",
		"Name":  name,
		"DataSource": map[string]any{
			"$Type":          "Forms$DataViewSource",
			"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Customer"},
		},
	}
}

func TestIsContextEditDataView(t *testing.T) {
	if !isContextEditDataView(contextDataView("dv")) {
		t.Error("parameter-bound DataView should be a context edit form")
	}
	// Database-source DataView is NOT an edit form (no parameter binding).
	dbDV := map[string]any{
		"$Type":      "Forms$DataView",
		"DataSource": map[string]any{"$Type": "Forms$DataViewSource", "EntityRef": map[string]any{"Entity": "M.E"}},
	}
	if isContextEditDataView(dbDV) {
		t.Error("database-source DataView should not be flagged")
	}
	// A snippet-parameter binding also counts.
	snipDV := map[string]any{
		"$Type": "Forms$DataView",
		"DataSource": map[string]any{
			"$Type":          "Forms$DataViewSource",
			"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "Customer"},
		},
	}
	if !isContextEditDataView(snipDV) {
		t.Error("snippet-parameter DataView should be a context edit form")
	}
	// Non-DataView with a similar source must not match.
	if isContextEditDataView(map[string]any{"$Type": "Forms$TextBox"}) {
		t.Error("non-DataView should not be flagged")
	}
}

// collectReported runs the walk and returns the names of flagged DataViews.
func collectReported(root map[string]any) []string {
	var names []string
	walkForUngridedDataView(root, false, func(dv map[string]any) {
		names = append(names, widgetName(dv))
	})
	return names
}

func TestWalkForUngridedDataView_FlagsBareForm(t *testing.T) {
	// A parameter-bound DataView placed directly under the page (no grid).
	root := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{contextDataView("dvBare")}}
	got := collectReported(root)
	if len(got) != 1 || got[0] != "dvBare" {
		t.Fatalf("expected [dvBare], got %v", got)
	}
}

func TestWalkForUngridedDataView_GridWrappedIsClean(t *testing.T) {
	// layoutgrid → row → column → dataview (the prescribed NewEdit pattern).
	root := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Rows": []any{map[string]any{
			"Columns": []any{map[string]any{
				"Widgets": []any{contextDataView("dvWrapped")},
			}},
		}},
	}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("grid-wrapped DataView should not be flagged, got %v", got)
	}
}

func TestWalkForUngridedDataView_GridAncestorThroughContainer(t *testing.T) {
	// grid → column → container → dataview: an ancestor grid still counts.
	root := map[string]any{
		"$Type": "Forms$LayoutGrid",
		"Rows": []any{map[string]any{
			"Columns": []any{map[string]any{
				"Widgets": []any{map[string]any{
					"$Type":   "Forms$DivContainer",
					"Widgets": []any{contextDataView("dvNested")},
				}},
			}},
		}},
	}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("DataView under a grid ancestor should not be flagged, got %v", got)
	}
}

func TestWalkForUngridedDataView_DatabaseFormIgnored(t *testing.T) {
	// A non-parameter (database) DataView outside a grid is not an edit form.
	dbDV := map[string]any{
		"$Type":      "Forms$DataView",
		"Name":       "dvList",
		"DataSource": map[string]any{"$Type": "Forms$DatabaseSource"},
	}
	root := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{dbDV}}
	if got := collectReported(root); len(got) != 0 {
		t.Fatalf("database DataView should not be flagged, got %v", got)
	}
}
