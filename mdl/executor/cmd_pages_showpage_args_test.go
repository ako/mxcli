// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mxcli-formula1 §39 (adjacent): a SHOW_PAGE argument naming anything other than
// the enclosing widget's context object was discarded in silence. The page opened
// with the context object instead, `mx check` reported 0 errors, and DESCRIBE
// printed the inferred mapping — so nothing anywhere said the written argument had
// been ignored.
func TestPageArgumentBindsContextObject(t *testing.T) {
	inDataView := func(varName string) pageArgContext {
		return pageArgContext{known: true, present: true, varName: varName}
	}
	cases := []struct {
		name  string
		value string
		ctx   pageArgContext
		want  bool
	}{
		// The context object, under either spelling.
		{"currentObject in a database-backed list", "$currentObject", inDataView(""), true},
		{"currentObject inside a parameter data view", "$currentObject", inDataView("Car"), true},
		{"the context variable by its own name", "$Car", inDataView("Car"), true},
		{"case-insensitive, as MDL identifiers are", "$car", inDataView("Car"), true},

		// The bug: a different variable, silently re-pointed at the context object.
		{"another page parameter", "$Other", inDataView("Car"), false},
		{"any variable in a database-backed list", "$Other", inDataView(""), false},

		// Not a plain variable reference — not checkable against a context object
		// that exists, so left alone.
		{"an association path", "$currentObject/Sales.Order_Customer", inDataView("Car"), true},
		{"a literal", "'Sales'", inDataView("Car"), true},
		{"an expression", "1 + 2", inDataView("Car"), true},

		// ALTER PAGE builds an action without traversing the stored page, so the
		// context object is unknown, not absent. The guard must stay quiet — refusing
		// here would reject `SET Action = SHOW_PAGE P(Car: $Car) ON btnGo`, which is
		// correct code.
		{"context unknown (ALTER PAGE)", "$Car", pageArgContext{}, true},
		{"context unknown, any variable", "$Other", pageArgContext{}, true},

		// mendixlabs/mxcli#1029: no enclosing data widget at all. $currentObject is
		// unbound here, so the empty ParameterMappings mxcli writes is not an
		// inferred mapping but a missing one — CE1571 at build. EVERY form of
		// argument is discarded, including the ones that are left alone above,
		// because there is no context object for them to be checked against.
		{"#1029: a page parameter on a page-level button", "$SomeRef", atDocumentRoot(), false},
		{"#1029: $currentObject outside any data widget", "$currentObject", atDocumentRoot(), false},
		{"#1029: a literal outside any data widget", "'literal'", atDocumentRoot(), false},
		{"#1029: an association path outside any data widget", "$SomeRef/Mod.Assoc", atDocumentRoot(), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ctx.binds(tc.value); got != tc.want {
				t.Errorf("pageArgContext%+v.binds(%q) = %v, want %v", tc.ctx, tc.value, got, tc.want)
			}
		})
	}
}

func showPageButton(arg string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "actionbutton",
		Name: "btnGo",
		Properties: map[string]any{
			"Action": &ast.ActionV3{
				Type:   "showPage",
				Target: "Mod.Detail",
				Args:   []ast.FlowArgV3{{Name: "Car", Value: arg}},
			},
		},
	}
}

// The check-time mirror must see the context variable of the nearest enclosing
// data widget, not of the widget carrying the action — a button has no data
// source of its own.
func TestValidateShowPageArguments_ThroughTheWidgetTree(t *testing.T) {
	dataviewOn := func(ref string, child *ast.WidgetV3) *ast.WidgetV3 {
		return &ast.WidgetV3{
			Type: "dataview",
			Name: "dv1",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: ref},
			},
			Children: []*ast.WidgetV3{child},
		}
	}

	cases := []struct {
		name string
		tree []*ast.WidgetV3
		want int
	}{
		{"argument is the context variable", []*ast.WidgetV3{dataviewOn("$Car", showPageButton("$Car"))}, 0},
		{"argument is $currentObject", []*ast.WidgetV3{dataviewOn("$Car", showPageButton("$currentObject"))}, 0},
		{"argument is another variable", []*ast.WidgetV3{dataviewOn("$Car", showPageButton("$Other"))}, 1},
	}

	// The tree walk resolves every widget against the registry, so a real one is
	// required — the callers all pass one (ValidateWidgetProperties bails when it
	// cannot be built).
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateWidgetTree(tc.tree, registry, "page Mod.P")
			var n int
			for _, v := range got {
				if v.RuleID == "MDL-PAGEARG01" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("MDL-PAGEARG01 violations = %d, want %d (all: %+v)", n, tc.want, got)
			}
		})
	}
}

// mendixlabs/mxcli#1029: "A page-level `actionbutton` (outside any dataview) with
// `Action: show_page Page(Param: $Var)` silently drops the argument and rebinds
// every target-page parameter to `$currentObject` — CE1571 at build."
//
// `mxcli check` reported "All references valid", `exec` reported success, and
// `DESCRIBE PAGE` then printed `(Item: $currentObject)` on a page where
// $currentObject is unbound. The check-time half of the guard is what a reporter
// meets first, so it is asserted on the same widget tree the report used: a
// button at the ROOT of the page, with nothing data-bound above it.
func TestValidateShowPageArguments_NoEnclosingDataWidget(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}

	// Every variant the report says it bisected, each one still discarded.
	for _, arg := range []string{"$SomeRef", "$currentObject", "'literal'", "$SomeRef/Mod.Assoc"} {
		t.Run(arg, func(t *testing.T) {
			tree := []*ast.WidgetV3{showPageButton(arg)}
			var hits int
			var msg string
			for _, v := range validateWidgetTree(tree, registry, "page Test.List") {
				if v.RuleID == "MDL-PAGEARG01" {
					hits++
					msg = v.Message
				}
			}
			if hits != 1 {
				t.Fatalf("MDL-PAGEARG01 violations = %d, want 1 — a page-level show_page argument is dropped and builds to CE1571", hits)
			}
			// The message has to name the failure the reporter will see from
			// mxbuild, or it sends them looking for a different bug.
			for _, want := range []string{"CE1571", "btnGo", "Mod.Detail"} {
				if !strings.Contains(msg, want) {
					t.Errorf("refusal message does not mention %q: %s", want, msg)
				}
			}
		})
	}

	// A nested container changes nothing: what matters is that no ancestor is
	// data-bound, not how deep the button sits.
	nested := []*ast.WidgetV3{{
		Type:     "container",
		Name:     "c1",
		Children: []*ast.WidgetV3{{Type: "layoutgrid", Name: "lg", Children: []*ast.WidgetV3{showPageButton("$SomeRef")}}},
	}}
	var hits int
	for _, v := range validateWidgetTree(nested, registry, "page Test.List") {
		if v.RuleID == "MDL-PAGEARG01" {
			hits++
		}
	}
	if hits != 1 {
		t.Errorf("MDL-PAGEARG01 violations inside plain containers = %d, want 1", hits)
	}

	// The control for the refusal: a zero-argument show_page at page level is
	// exactly what the report says still works, and must stay silent.
	noArgs := []*ast.WidgetV3{{
		Type:       "actionbutton",
		Name:       "btnGo",
		Properties: map[string]any{"Action": &ast.ActionV3{Type: "showPage", Target: "Mod.Detail"}},
	}}
	for _, v := range validateWidgetTree(noArgs, registry, "page Test.List") {
		if v.RuleID == "MDL-PAGEARG01" {
			t.Errorf("a show_page with no arguments must not be refused: %s", v.Message)
		}
	}
}

// ALTER PAGE grafts widgets into a page this pass never traverses, so it cannot
// say whether a data widget encloses them. The #1029 refusal must not reach it —
// `ALTER PAGE … INSERT actionbutton b (action: show_page P(Car: $Car)) INTO dv`
// is correct code, and refusing it is the false positive the contextKnown flag
// was introduced for in the first place.
func TestValidateShowPageArguments_AlterPageStandsDown(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}
	for _, arg := range []string{"$Car", "$currentObject", "'literal'"} {
		for _, v := range validateWidgetSubtree([]*ast.WidgetV3{showPageButton(arg)}, registry, "alter Mod.P") {
			if v.RuleID == "MDL-PAGEARG01" {
				t.Errorf("ALTER PAGE INSERT of %s was refused: %s", arg, v.Message)
			}
		}
	}
}

// ako/mxcli#552: a list widget's OWN action is row-scoped, so the row it renders
// IS the context object. The #1029 guard judged every widget's own action in the
// context its PARENT supplies, which is right for a button and wrong for the
// widget that establishes the context — it refused
//
//	datagrid dgRequests (DataSource: DATABASE M.ServiceRequest,
//	  onClick: SHOW_PAGE M.Edit(ServiceRequest: $currentObject))
//
// with "widget `dgRequests` is not inside a data view, list view or grid row",
// while mxbuild accepts the stored page at 0 errors. On a `listview` the message
// contradicted itself outright.
//
// `onClick:` and `Action:` are aliases that both land on Properties["Action"]
// (see validate_widget_action_slot.go), so one shape covers both spellings.
func TestValidateShowPageArguments_ListWidgetOwnRowAction(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}

	rowAction := func(kind string, source any, arg string) *ast.WidgetV3 {
		return &ast.WidgetV3{
			Type: kind,
			Name: "w1",
			Properties: map[string]any{
				"DataSource": source,
				"Action": &ast.ActionV3{
					Type:   "showPage",
					Target: "Mod.Detail",
					Args:   []ast.FlowArgV3{{Name: "Car", Value: arg}},
				},
			},
		}
	}
	database := &ast.DataSourceV3{Type: "database", Reference: "Mod.Car"}

	hits := func(w *ast.WidgetV3) []string {
		var msgs []string
		for _, v := range validateWidgetTree([]*ast.WidgetV3{w}, registry, "page Mod.P") {
			if v.RuleID == "MDL-PAGEARG01" {
				msgs = append(msgs, v.Message)
			}
		}
		return msgs
	}

	// The row object, under either spelling, on each widget kind that renders rows.
	for _, kind := range []string{"datagrid", "listview", "gallery"} {
		t.Run(kind+"/$currentObject", func(t *testing.T) {
			if got := hits(rowAction(kind, database, "$currentObject")); len(got) != 0 {
				t.Errorf("a %s's own row action was refused — mxbuild accepts it at 0 errors: %s", kind, got[0])
			}
		})
	}

	// The bare-entity shorthand leaves a plain string rather than a parsed source,
	// so the entity is unreadable — but the widget plainly binds data, and the
	// guard's own doctrine is that it refuses only what it can PROVE is discarded.
	t.Run("bare-entity shorthand stands the guard down", func(t *testing.T) {
		if got := hits(rowAction("datagrid", "Mod.Car", "$currentObject")); len(got) != 0 {
			t.Errorf("shorthand source was refused: %s", got[0])
		}
	})

	// Controls. Without these the fix is indistinguishable from deleting the rule.
	t.Run("control: a foreign variable on a row action is still refused", func(t *testing.T) {
		if got := hits(rowAction("datagrid", database, "$Other")); len(got) != 1 {
			t.Errorf("MDL-PAGEARG01 violations = %d, want 1 — a database row source names no $Other", len(got))
		}
	})
	t.Run("control: #1029's page-level button is still refused", func(t *testing.T) {
		var n int
		for _, v := range validateWidgetTree([]*ast.WidgetV3{showPageButton("$SomeRef")}, registry, "page Mod.P") {
			if v.RuleID == "MDL-PAGEARG01" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("MDL-PAGEARG01 violations on a contextless button = %d, want 1", n)
		}
	})
	t.Run("control: a button beside the grid, not in it, is still refused", func(t *testing.T) {
		grid := rowAction("datagrid", database, "$currentObject")
		tree := []*ast.WidgetV3{{Type: "container", Name: "c1", Children: []*ast.WidgetV3{grid, showPageButton("$currentObject")}}}
		var n int
		for _, v := range validateWidgetTree(tree, registry, "page Mod.P") {
			if v.RuleID == "MDL-PAGEARG01" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("MDL-PAGEARG01 violations = %d, want 1 — the grid's own action is fine, the sibling button is not", n)
		}
	})
}
