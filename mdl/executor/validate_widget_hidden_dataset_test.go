// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// chartSeriesItem builds a chart `series`/`line` item written the documented way:
// one friendly `DataSource:` plus the dataSet mode that selects which schema
// property it lands in.
func chartSeriesItem(dataSet string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Name: "s1",
		Properties: map[string]any{
			"DataSet":    dataSet,
			"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Mod.View"},
		},
	}
}

// chartSeriesMapping mirrors the shape a chart's def.json actually has: BOTH
// datasource sub-properties declare Source "DataSource" and neither declares an
// alias, so a lookup by Source alone cannot tell them apart.
func chartSeriesMapping() *ObjectListMapping {
	return &ObjectListMapping{
		MDLContainer: "SERIES",
		PropertyKey:  "series",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "dataSet", Operation: "primitive", Value: "static"},
			{PropertyKey: "staticDataSource", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "dynamicDataSource", Source: "DataSource", Operation: "datasource"},
		},
	}
}

// The friendly `DataSource:` on a chart series is routed BY dataSet mode when the
// item is built: buildObjectListItem looks the alias up only for the property
// seriesDataSourceMatchesMode selects, so `dataSet: 'static'` writes
// staticDataSource and leaves dynamicDataSource unset.
//
// itemValueMap resolved it by Source instead, which both properties share, so it
// reported dynamicDataSource as explicitly set — and MDL-WIDGET10 then warned
// that a value the script never wrote "will be ignored". Measured: 11 such
// warnings on 34-chart-widget-examples.mdl, one per series in the file, on the
// only syntax the examples and skills document.
func TestItemValueMap_FriendlyDataSourceIsModeRouted(t *testing.T) {
	_, explicit := itemValueMap(chartSeriesItem("static"), chartSeriesMapping())

	if !explicit["staticdatasource"] {
		t.Error("staticDataSource not explicit under dataSet 'static' — the mode-matching " +
			"property is the one the builder writes, so the checker must see it set")
	}
	if explicit["dynamicdatasource"] {
		t.Error("dynamicDataSource reported as explicitly set under dataSet 'static'; " +
			"buildObjectListItem never writes it, so MDL-WIDGET10 warns about a value " +
			"that does not exist")
	}
}

// The control: the routing must follow the mode rather than always preferring
// `static`. A fix that hardcoded "ignore dynamic*" would pass the test above and
// break every dynamic series.
func TestItemValueMap_FriendlyDataSourceFollowsDynamicMode(t *testing.T) {
	_, explicit := itemValueMap(chartSeriesItem("dynamic"), chartSeriesMapping())

	if !explicit["dynamicdatasource"] {
		t.Error("dynamicDataSource not explicit under dataSet 'dynamic' — this is the " +
			"property the builder writes in that mode")
	}
	if explicit["staticdatasource"] {
		t.Error("staticDataSource reported as explicitly set under dataSet 'dynamic'")
	}
}

// The second control: the gate is scoped to chart series. A non-chart object list
// whose sub-properties share a Source must keep resolving by Source, or the
// hidden-property rule goes blind on every other widget.
func TestItemValueMap_NonChartContainerStillResolvesBySource(t *testing.T) {
	mapping := &ObjectListMapping{
		MDLContainer: "COLUMN",
		PropertyKey:  "columns",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "attribute", Source: "Attribute", Operation: "attribute"},
		},
	}
	item := &ast.WidgetV3{Name: "c1", Properties: map[string]any{"Attribute": "Name"}}

	_, explicit := itemValueMap(item, mapping)
	if !explicit["attribute"] {
		t.Error("a non-chart item property stopped resolving through its Source")
	}
}

// The third control: an item that names a datasource by its SCHEMA key keeps
// working regardless of mode. Someone writing `dynamicDataSource:` explicitly
// means it, and the builder honours it (the PropertyKey lookup runs before the
// mode-aware fallback), so the checker must see it set.
func TestItemValueMap_ExplicitSchemaKeyIgnoresMode(t *testing.T) {
	item := &ast.WidgetV3{
		Name: "s1",
		Properties: map[string]any{
			"DataSet":           "static",
			"dynamicDataSource": &ast.DataSourceV3{Type: "database", Reference: "Mod.View"},
		},
	}
	_, explicit := itemValueMap(item, chartSeriesMapping())
	if !explicit["dynamicdatasource"] {
		t.Error("an explicitly named dynamicDataSource was dropped by the mode gate — " +
			"the gate applies to the shared Source lookup, not to the schema key")
	}
}
