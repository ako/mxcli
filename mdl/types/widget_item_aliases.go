// SPDX-License-Identifier: Apache-2.0

package types

// ItemPropertyAliases lists alternative MDL property names that resolve to a
// schema property on an object-list item, keyed by
// (widgetID, objectListPropertyKey, itemPropertyKey).
//
// It lives here rather than beside its first consumer because **two** code paths
// need it and they must not disagree: the create path (the pluggable widget
// engine, which writes a column from MDL) and the ALTER path (the page mutator,
// which edits a column already in the document). Those kept separate hand-written
// tables and drifted — `DynamicCellClass` was aliased on create and absent on
// ALTER, so `ALTER PAGE SET DynamicCellClass ON grid.Column` failed with
// "column property not found" for a property the very same tool had just written
// (mendixlabs/mxcli#919). One table cannot drift from itself.
//
// Only genuine renames belong here. A property whose MDL name differs from the
// schema key by case alone (`Sortable` → `sortable`) needs no entry: both
// consumers match case-insensitively against the schema keys the document
// actually declares, so listing those would be a second thing to keep in sync
// for no gain.
var ItemPropertyAliases = map[string]map[string]map[string][]string{
	DataGridWidgetID: {
		DataGridColumnsKey: {
			"header":      {"Caption"},
			"dynamicText": {"Content"},
			// MDL `ColumnWidth: manual` fills the schema's `width` enum. The
			// keyword path mapped this (`colPropString(..., "ColumnWidth")`);
			// without the alias the engine leaves width at its `autoFill`
			// default, so a `Size:` value becomes invalid (size only applies
			// when width=manual) and Studio Pro flags CE0463.
			"width": {"ColumnWidth"},
			// MDL `DynamicCellClass: '<expr>'` fills the schema's `columnClass`
			// expression (a per-cell dynamic CSS class). Without the alias the
			// engine looks up `columnClass` in the AST property bag, finds
			// nothing, and writes an empty expression — the class is silently
			// dropped. Bug 10a.
			"columnClass": {"DynamicCellClass"},
		},
	},
	"com.mendix.widget.web.heatmap.HeatMap": {
		"scaleColors": {
			// MDL `ColorValue: '#rrggbb'` fills the schema's `colour` primitive
			// (British spelling). Without the alias the engine looks up `colour`,
			// doesn't find `ColorValue`, and the scale colour is silently dropped
			// on write. Same class as `columnClass` (Bug 10a).
			"colour": {"ColorValue"},
		},
	},
}

const (
	// DataGridWidgetID is the pluggable DataGrid 2 widget.
	DataGridWidgetID = "com.mendix.widget.web.datagrid.Datagrid"
	// DataGridColumnsKey is its object-list property holding the columns.
	DataGridColumnsKey = "columns"
)

// ItemPropertyAliasesFor returns the schemaKey → MDL-alias table for one
// object-list slot, or nil when the widget declares none.
func ItemPropertyAliasesFor(widgetID, objectListKey string) map[string][]string {
	byList, ok := ItemPropertyAliases[widgetID]
	if !ok {
		return nil
	}
	return byList[objectListKey]
}
