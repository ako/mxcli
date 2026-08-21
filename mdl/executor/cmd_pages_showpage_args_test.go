// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mxcli-formula1 §39 (adjacent): a SHOW_PAGE argument naming anything other than
// the enclosing widget's context object was discarded in silence. The page opened
// with the context object instead, `mx check` reported 0 errors, and DESCRIBE
// printed the inferred mapping — so nothing anywhere said the written argument had
// been ignored.
func TestPageArgumentBindsContextObject(t *testing.T) {
	cases := []struct {
		name         string
		value        string
		contextVar   string
		contextKnown bool
		want         bool
	}{
		// The context object, under either spelling.
		{"currentObject in a database-backed list", "$currentObject", "", true, true},
		{"currentObject inside a parameter data view", "$currentObject", "Car", true, true},
		{"the context variable by its own name", "$Car", "Car", true, true},
		{"case-insensitive, as MDL identifiers are", "$car", "Car", true, true},

		// The bug: a different variable, silently re-pointed at the context object.
		{"another page parameter", "$Other", "Car", true, false},
		{"any variable in a database-backed list", "$Other", "", true, false},

		// Not a plain variable reference — not checkable this way, so left alone.
		{"an association path", "$currentObject/Sales.Order_Customer", "Car", true, true},
		{"a literal", "'Sales'", "Car", true, true},
		{"an expression", "1 + 2", "Car", true, true},

		// ALTER PAGE builds an action without traversing the stored page, so the
		// context object is unknown, not absent. The guard must stay quiet — refusing
		// here would reject `SET Action = SHOW_PAGE P(Car: $Car) ON btnGo`, which is
		// correct code.
		{"context unknown (ALTER PAGE)", "$Car", "", false, true},
		{"context unknown, any variable", "$Other", "", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageArgumentBindsContextObject(tc.value, tc.contextVar, tc.contextKnown); got != tc.want {
				t.Errorf("pageArgumentBindsContextObject(%q, %q, %v) = %v, want %v",
					tc.value, tc.contextVar, tc.contextKnown, got, tc.want)
			}
		})
	}
}

// The check-time mirror must see the context variable of the nearest enclosing
// data widget, not of the widget carrying the action — a button has no data
// source of its own.
func TestValidateShowPageArguments_ThroughTheWidgetTree(t *testing.T) {
	button := func(arg string) *ast.WidgetV3 {
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
		{"argument is the context variable", []*ast.WidgetV3{dataviewOn("$Car", button("$Car"))}, 0},
		{"argument is $currentObject", []*ast.WidgetV3{dataviewOn("$Car", button("$currentObject"))}, 0},
		{"argument is another variable", []*ast.WidgetV3{dataviewOn("$Car", button("$Other"))}, 1},
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
