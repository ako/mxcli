// SPDX-License-Identifier: Apache-2.0

// CE6206, found in the maintenance project's Studio Pro error pane after a
// Tablet web offline profile was added: two pages that had been valid became
// errors, because they bind attributes across two associations.
//
// The rule is pinned by a control inside the same reference page, which carried
// both shapes and had only one flagged:
//
//	MaintenanceRequest_Asset → AssetName               1 step   accepted
//	MaintenanceRequest_Asset → Asset_Site → SiteName   2 steps  CE6206
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func TestAttributePathSteps(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"Name", 0},                          // a plain attribute
		{"Req_Asset/AssetName", 1},           // one hop — allowed offline
		{"Req_Asset/Asset_Site/SiteName", 2}, // two hops — CE6206
		{"A/B/C/D", 3},
		{"", 0},
		{"  Req_Asset/AssetName  ", 1},
	}
	for _, tc := range tests {
		if got := attributePathSteps(tc.path); got != tc.want {
			t.Errorf("attributePathSteps(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}

func TestOfflineProfileNames(t *testing.T) {
	nav := &types.NavigationDocument{Profiles: []*types.NavigationProfile{
		{Name: "Responsive", Kind: "Responsive"},
		{Name: "Phone", Kind: "Phone"},
		{Name: "TabletOffline", Kind: "TabletOffline"},
		{Name: "PhoneOffline", Kind: "PhoneOffline"},
	}}
	got := offlineProfileNames(nav)
	want := []string{"PhoneOffline", "TabletOffline"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("offlineProfileNames = %v, want %v", got, want)
	}
	// The common case: no offline profile at all, so the rule is inert and no
	// project pays for it.
	plain := &types.NavigationDocument{Profiles: []*types.NavigationProfile{
		{Name: "Responsive", Kind: "Responsive"},
	}}
	if got := offlineProfileNames(plain); len(got) != 0 {
		t.Errorf("a project with no offline profile reported %v", got)
	}
	if got := offlineProfileNames(nil); len(got) != 0 {
		t.Errorf("nil navigation reported %v", got)
	}
}

func widget(t string, props map[string]any, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: t, Name: "w", Properties: props, Children: children}
}

func TestOfflinePathViolationsFlagsOnlyTwoOrMoreHops(t *testing.T) {
	profiles := []string{"TabletOffline"}

	// One hop is legal offline and must NOT be reported. This is the control the
	// reference page provided, and without it the rule would flag every
	// association-bound column in an offline app.
	if v := offlinePathViolations("page P", []*ast.WidgetV3{
		widget("column", map[string]any{"Attribute": "Req_Asset/AssetName"}),
	}, profiles); len(v) != 0 {
		t.Errorf("one hop must not be flagged, got %d violations: %+v", len(v), v)
	}

	// Two hops is the reported case.
	v := offlinePathViolations("page P", []*ast.WidgetV3{
		widget("column", map[string]any{"Attribute": "Req_Asset/Asset_Site/SiteName"}),
	}, profiles)
	if len(v) != 1 {
		t.Fatalf("two hops should be flagged, got %d", len(v))
	}
	for _, want := range []string{"MDL-OFFLINE01", "Req_Asset/Asset_Site/SiteName", "TabletOffline", "CE6206"} {
		if !strings.Contains(v[0].RuleID+v[0].Message, want) {
			t.Errorf("diagnostic missing %q: %s", want, v[0].Message)
		}
	}
}

func TestOfflinePathViolationsFindsNestedAndTemplateBindings(t *testing.T) {
	profiles := []string{"TabletOffline"}

	// Nested: the reference failure was a column inside a data grid inside a
	// container, so a flat scan would have missed it.
	nested := widget("container", nil,
		widget("datagrid", nil,
			widget("column", map[string]any{"Attribute": "A/B/C"})))
	if v := offlinePathViolations("page P", []*ast.WidgetV3{nested}, profiles); len(v) != 1 {
		t.Errorf("nested binding not found, got %d violations", len(v))
	}

	// A dynamic text binds through template parameters, not `Attribute:` — which
	// is exactly how the second reported error (Text 'txtAssetSite') was written.
	dyn := widget("dynamictext", map[string]any{
		"Content":       "Site: {1}",
		"ContentParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Req_Asset/Asset_Site/SiteName"}},
	})
	if v := offlinePathViolations("page P", []*ast.WidgetV3{dyn}, profiles); len(v) != 1 {
		t.Errorf("template-parameter binding not found, got %d violations", len(v))
	}
}

func TestOfflinePathViolationsIgnoresXPathConstraints(t *testing.T) {
	// An XPath constraint is full of slashes and is NOT an attribute path, so
	// scanning every path-shaped property would report constraints that offline
	// navigation permits. Only attribute bindings are considered.
	profiles := []string{"TabletOffline"}
	w := widget("datagrid", map[string]any{
		"Constraint": "[MyModule.Order_Customer/MyModule.Customer/Name = 'x']",
	})
	if v := offlinePathViolations("page P", []*ast.WidgetV3{w}, profiles); len(v) != 0 {
		t.Errorf("an XPath constraint must not be flagged, got: %+v", v)
	}
}
