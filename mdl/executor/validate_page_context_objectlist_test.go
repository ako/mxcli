// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// chartRegistry is a registry holding one definition that declares a `series`
// object list — the shape a real chart's .def.json has. Built in memory because
// NO embedded widget definition declares an object list (measured: 0 of them),
// so a test using LoadWidgetRegistry("") would skip everywhere, and the .def.json
// cache a project builds from its .mpk files is gitignored, so a test reading one
// would skip in CI.
func chartRegistry() *WidgetRegistry {
	def := &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.barchart.BarChart",
		MDLName:  "barchart",
		ObjectLists: []ObjectListMapping{
			{PropertyKey: "series", MDLContainer: "SERIES"},
		},
	}
	return &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"BARCHART": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}
}

// dashboardWithThreeSeries mirrors the shape 34-chart-widget-examples.mdl uses:
// three separate charts on one page, each with a series the author called `s`.
func dashboardWithThreeSeries() []*ast.WidgetV3 {
	chart := func(name, container, itemName string) *ast.WidgetV3 {
		return &ast.WidgetV3{
			Type: "pluggablewidget", Name: name,
			Properties: map[string]any{"WidgetType": "com.mendix.widget.web.barchart.BarChart"},
			Children:   []*ast.WidgetV3{{Type: container, Name: itemName}},
		}
	}
	return []*ast.WidgetV3{
		chart("dashBar", "series", "s"),
		chart("dashColumn", "series", "s"),
		chart("dashArea", "series", "s"),
	}
}

// An object-list ITEM's name is mxcli's own, not the model's — the same reason
// widgetKindsWithoutStoredNames already excludes rows and columns.
//
// Proof the model does not hold it: a page authored with `series sRegion (…)`
// comes back from DESCRIBE as `series series1 (…)`. DESCRIBE synthesises the
// name because the stored WidgetObject has none, so no two of them can collide
// under CE0495.
//
// Proof mxbuild agrees: the page above, exec'd into a real 11.6.6 project and
// run through `mx check`, reports 0 errors. The control that the check is not
// inert on that project: a view entity with a deliberately wrong column type in
// the same app fails CE6770, so mxbuild was really validating.
//
// mxcli reported `duplicate widget name 's' (used 3 times)` and, because a
// reference error fails the run, refused to execute the script at all.
func TestCheckDuplicateWidgetNames_ObjectListItemsAreNotWidgets(t *testing.T) {
	got := checkDuplicateWidgetNames(dashboardWithThreeSeries(), chartRegistry())
	for _, e := range got {
		if strings.Contains(e, "'s'") {
			t.Errorf("object-list item names counted as widget names: %q", e)
		}
	}
}

// The control: real duplicate WIDGET names must still be reported. A fix that
// stopped descending into a pluggable widget's children — or that skipped every
// child of one — would pass the test above and turn CE0495 detection off for
// everything inside a chart or a gallery.
func TestCheckDuplicateWidgetNames_RealDuplicatesStillReported(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{Type: "dynamictext", Name: "dup"},
		{Type: "container", Name: "c", Children: []*ast.WidgetV3{
			{Type: "dynamictext", Name: "dup"},
		}},
	}
	got := checkDuplicateWidgetNames(widgets, chartRegistry())
	if len(got) != 1 || !strings.Contains(got[0], "'dup'") {
		t.Errorf("a genuine duplicate widget name was not reported: %v", got)
	}
}

// The second control: a child of a pluggable widget that is NOT one of its
// object-list containers is a real widget in a child slot, and two of those
// sharing a name IS CE0495.
func TestCheckDuplicateWidgetNames_ChildSlotWidgetsStillCount(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{Type: "pluggablewidget", Name: "w1",
			Properties: map[string]any{"WidgetType": "com.mendix.widget.web.barchart.BarChart"},
			Children:   []*ast.WidgetV3{{Type: "dynamictext", Name: "dup"}}},
		{Type: "dynamictext", Name: "dup"},
	}
	got := checkDuplicateWidgetNames(widgets, chartRegistry())
	if len(got) != 1 || !strings.Contains(got[0], "'dup'") {
		t.Errorf("a duplicate inside a pluggable widget's child slot was not reported: %v", got)
	}
}

// The third control: without a registry nothing can be resolved, and the rule
// must fall back to its previous behaviour rather than silently accepting
// everything. `check` runs with no project in CI, so this is the common path.
func TestCheckDuplicateWidgetNames_NoRegistryStillReports(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{Type: "dynamictext", Name: "dup"},
		{Type: "dynamictext", Name: "dup"},
	}
	if got := checkDuplicateWidgetNames(widgets, nil); len(got) != 1 {
		t.Errorf("with no registry, a plain duplicate must still be reported: %v", got)
	}
}
