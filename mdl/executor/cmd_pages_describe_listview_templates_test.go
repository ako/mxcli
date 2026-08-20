// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// rawListViewWithTemplates mirrors ako/TestApp's Vehicle_Overview: a list view
// over Pages.Vehicle with its own default body plus four specialization
// templates, in the order Studio Pro stored them.
func rawListViewWithTemplates() map[string]any {
	tmpl := func(entity, name string) map[string]any {
		return map[string]any{
			"$Type":   "Forms$ListViewTemplate",
			"Entity":  entity,
			"Widgets": []any{map[string]any{"$Type": "Forms$TextBox", "Name": name}},
		}
	}
	return map[string]any{
		"$Type": "Forms$ListView",
		"Name":  "vehicleListView",
		"DataSource": map[string]any{
			"$Type": "Forms$ListViewXPathSource",
			"EntityRef": map[string]any{
				"$Type":  "DomainModels$DirectEntityRef",
				"Entity": "Pages.Vehicle",
			},
		},
		"Widgets": []any{map[string]any{"$Type": "Forms$TextBox", "Name": "defaultVehicle"}},
		"Templates": []any{
			tmpl("Pages.Bus", "busLabel"),
			tmpl("Pages.Truck", "truckLabel"),
			tmpl("Pages.Car", "carLabel"),
			tmpl("Pages.SUV", "suvLabel"),
		},
	}
}

// TestDescribeRendersListViewTemplates is the regression test for #940.
//
// parseListViewContent read only the Widgets array, so a list view's
// specialization templates were dropped from DESCRIBE with no warning — and
// SEARCH inherited the gap, because the catalog's source table is built from
// DESCRIBE output. Worse, DESCRIBE emits `create or modify page`, so
// re-executing its output rebuilt the page without them: measured on
// ako/TestApp, a describe → exec round trip took the page from 4 templates to 0,
// with `mx check` reporting 0 errors either way.
func TestDescribeRendersListViewTemplates(t *testing.T) {
	ctx, buf := newMockCtx(t)

	for _, w := range parseRawWidget(ctx, rawListViewWithTemplates(), "Pages.Vehicle") {
		outputWidgetMDLV3(ctx, w, 0)
	}
	out := buf.String()

	// Every template, identified by the entity it renders.
	for _, want := range []string{
		"template for Pages.Bus {",
		"template for Pages.Truck {",
		"template for Pages.Car {",
		"template for Pages.SUV {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q from DESCRIBE output:\n%s", want, out)
		}
	}

	// Contents, which is what SHOW REFERENCES and SEARCH ultimately index.
	for _, want := range []string{"busLabel", "truckLabel", "carLabel", "suvLabel", "defaultVehicle"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing widget %q from DESCRIBE output:\n%s", want, out)
		}
	}

	// Order is authored, not derived — TestApp's is Bus, Truck, Car, SUV, which
	// is neither alphabetical nor domain-model order, so a describe that sorted
	// them would not round-trip to the same document.
	idx := func(s string) int { return strings.Index(out, s) }
	if !(idx("Pages.Bus") < idx("Pages.Truck") && idx("Pages.Truck") < idx("Pages.Car") && idx("Pages.Car") < idx("Pages.SUV")) {
		t.Errorf("templates are not in stored order:\n%s", out)
	}

	// The default body is the list view's own Widgets array, NOT a template, and
	// must not be wrapped in one.
	if strings.Contains(out, "template for Pages.Vehicle") {
		t.Errorf("the list view's own body was rendered as a template:\n%s", out)
	}
}

// TestDescribeListViewTemplateEntityContext pins that a template's body is
// parsed in the specialization's context, not the list view's. An attribute the
// specialization adds does not exist on the list view's entity, so parsing the
// children against the parent entity resolves the wrong thing (or nothing).
func TestDescribeListViewTemplateEntityContext(t *testing.T) {
	ctx, _ := newMockCtx(t)

	widgets := parseRawWidget(ctx, rawListViewWithTemplates(), "Pages.Vehicle")
	if len(widgets) != 1 {
		t.Fatalf("expected 1 list view, got %d", len(widgets))
	}
	var templates []rawWidget
	for _, c := range widgets[0].Children {
		if c.Type == "Forms$ListViewTemplate" {
			templates = append(templates, c)
		}
	}
	if len(templates) != 4 {
		t.Fatalf("got %d template(s), want 4", len(templates))
	}
	for _, tpl := range templates {
		if tpl.EntityContext != tpl.Specialization {
			t.Errorf("template for %s has EntityContext %q, want the specialization itself",
				tpl.Specialization, tpl.EntityContext)
		}
	}
}
