// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// designPropKeys lists the Key of each Forms$DesignPropertyValue in an encoded
// Forms$Appearance.
func designPropKeys(t *testing.T, appearance any) []string {
	t.Helper()
	a, ok := appearance.(map[string]any)
	if !ok {
		t.Fatalf("Appearance is %T, want a document", appearance)
	}
	list, _ := a["DesignProperties"].(primitive.A)
	var keys []string
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			k, _ := m["Key"].(string)
			keys = append(keys, k)
		}
	}
	return keys
}

// A layout-grid row and column write their appearance and alignment; the
// writer hardcoded an empty appearance and "None" for both, which dropped
// every grid design property on describe → exec.
func TestLayoutGridRowColumnToGen_CarriesAppearanceAndAlignment(t *testing.T) {
	row := &pages.LayoutGridRow{
		Class:                   "row-class",
		VerticalAlignment:       "Center",
		HorizontalAlignment:     "End",
		NoSpacingBetweenColumns: true,
		DesignProperties: []pages.DesignPropertyValue{
			{Key: "Column gap", ValueType: "option", Option: "Large"},
			{Key: "Cards style", ValueType: "toggle"},
		},
		Columns: []*pages.LayoutGridColumn{{
			Weight:            6,
			Style:             "padding: 0",
			VerticalAlignment: "Center",
			DesignProperties: []pages.DesignPropertyValue{
				{Key: "Flex container", ValueType: "option", Option: "Vertical (column)"},
			},
		}},
	}
	g, err := layoutGridRowToGen(row)
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["VerticalAlignment"] != "Center" || doc["HorizontalAlignment"] != "End" || doc["SpacingBetweenColumns"] != false {
		t.Errorf("row alignment: %v / %v / %v, want Center / End / false",
			doc["VerticalAlignment"], doc["HorizontalAlignment"], doc["SpacingBetweenColumns"])
	}
	if got := doc["Appearance"].(map[string]any)["Class"]; got != "row-class" {
		t.Errorf("row Appearance.Class = %v, want row-class", got)
	}
	if keys := designPropKeys(t, doc["Appearance"]); len(keys) != 2 || keys[0] != "Column gap" || keys[1] != "Cards style" {
		t.Errorf("row design properties %v, want [Column gap Cards style]", keys)
	}

	col := doc["Columns"].(primitive.A)[1].(map[string]any) // [0] is the list marker
	if col["VerticalAlignment"] != "Center" {
		t.Errorf("column VerticalAlignment = %v, want Center", col["VerticalAlignment"])
	}
	if got := col["Appearance"].(map[string]any)["Style"]; got != "padding: 0" {
		t.Errorf("column Appearance.Style = %v, want padding: 0", got)
	}
	if keys := designPropKeys(t, col["Appearance"]); len(keys) != 1 || keys[0] != "Flex container" {
		t.Errorf("column design properties %v, want [Flex container]", keys)
	}
}

// Nothing set writes exactly the defaults the writer always wrote.
func TestLayoutGridRowColumnToGen_DefaultsUnchanged(t *testing.T) {
	g, err := layoutGridRowToGen(&pages.LayoutGridRow{Columns: []*pages.LayoutGridColumn{{}}})
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["VerticalAlignment"] != "None" || doc["HorizontalAlignment"] != "None" || doc["SpacingBetweenColumns"] != true {
		t.Errorf("row defaults: %v / %v / %v, want None / None / true",
			doc["VerticalAlignment"], doc["HorizontalAlignment"], doc["SpacingBetweenColumns"])
	}
	if keys := designPropKeys(t, doc["Appearance"]); len(keys) != 0 {
		t.Errorf("row design properties %v, want none", keys)
	}
	col := doc["Columns"].(primitive.A)[1].(map[string]any)
	if col["VerticalAlignment"] != "None" {
		t.Errorf("column VerticalAlignment = %v, want None", col["VerticalAlignment"])
	}
}
