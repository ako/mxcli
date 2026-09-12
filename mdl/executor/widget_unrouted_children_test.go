// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// datagridLikeDef is DataGrid2's shape: a `columns` object list, two
// widgets-typed slots, and — the part that matters — NO `template` slot, so
// there is no catch-all for a child that matches nothing.
//
// The container spellings come from mdlContainerForWidgetSlot rather than being
// retyped, so this fixture cannot disagree with the convention table the engine
// and the diagnostic both read.
func datagridLikeDef() *WidgetDefinition {
	const id = "com.mendix.widget.web.datagrid.Datagrid"
	slot := func(key string) ChildSlotMapping {
		return ChildSlotMapping{
			PropertyKey: key,
			// The registry lower-cases this on load; do the same here.
			MDLContainer: strings.ToLower(mdlContainerForWidgetSlot(id, key)),
			Operation:    "widgets",
		}
	}
	return &WidgetDefinition{
		WidgetID:    id,
		MDLName:     "DATAGRID",
		WidgetKind:  "pluggable",
		ObjectLists: []ObjectListMapping{{PropertyKey: "columns", MDLContainer: "column"}},
		ChildSlots:  []ChildSlotMapping{slot("emptyPlaceholder"), slot("filtersPlaceholder")},
	}
}

func wdg(typ, name string, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: typ, Name: name, Children: children}
}

func droppedTypes(def *WidgetDefinition, w *ast.WidgetV3) []string {
	var out []string
	for _, c := range unroutedPluggableChildren(def, w) {
		out = append(out, c.Type)
	}
	return out
}

// TestDataGridFilterBlockIsReportedNotSwallowed is the reported case. The MDL
// that `mxcli syntax page widgets` documented —
//
//	COLUMN c (Attribute: A) FILTER f { TEXTFILTER tf (Attribute: A) }
//
// parses as a COLUMN with no body plus a SIBLING filter widget, because
// widgetV3 is `type name props? body?` and nothing binds the second widget to
// the first. That sibling used to be built and discarded in silence.
func TestDataGridFilterBlockIsReportedNotSwallowed(t *testing.T) {
	grid := wdg("datagrid", "dg",
		wdg("column", "colMeter"),
		wdg("filter", "f", wdg("textfilter", "tf")),
	)
	got := droppedTypes(datagridLikeDef(), grid)
	if len(got) != 1 || got[0] != "filter" {
		t.Fatalf("dropped children = %v, want exactly [filter]", got)
	}
}

// TestDataGridRoutedChildrenAreNotReported is the control. Everything the grid
// really does declare has to stay silent, or the rule just trades a silent drop
// for a wall of false refusals.
func TestDataGridRoutedChildrenAreNotReported(t *testing.T) {
	def := datagridLikeDef()
	for _, c := range []*ast.WidgetV3{
		wdg("column", "colMeter"),                     // the object list
		wdg("controlbar", "cb"),                       // filtersPlaceholder, DataGrid's spelling
		wdg("emptyplaceholder", "ep"),                 // the other widgets slot
		wdg("container", "filtersPlaceholder"),        // `container <slotName>` form
		{Type: "container", Name: "emptyPlaceholder"}, // ditto, by property key
	} {
		if got := droppedTypes(def, wdg("datagrid", "dg", c)); got != nil {
			t.Errorf("child %s/%s reported as dropped: %v", c.Type, c.Name, got)
		}
	}
}

// TestGalleryFilterBlockStaysValid is the control that keeps the fix honest
// about WHY the datagrid case is wrong. `filter { … }` is correct on a gallery
// — the same widget property, under that widget's own keyword — so a rule that
// simply banned `filter` as a child would break the form the skills teach.
// Uses the REAL gallery definition, not a fixture.
func TestGalleryFilterBlockStaysValid(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("GALLERY")
	if !ok {
		t.Skip("gallery definition not embedded in this build")
	}
	gallery := wdg("gallery", "g",
		wdg("filter", "f", wdg("textfilter", "tf")),
		// A gallery has a `template` catch-all, so an arbitrary widget in its
		// body is placed rather than dropped.
		wdg("actionbutton", "btn"),
	)
	if got := droppedTypes(def, gallery); got != nil {
		t.Errorf("gallery children reported as dropped: %v", got)
	}
}

// TestUnroutedRuleStaysQuietWhenItCannotKnow pins the conservative bail-outs.
// The check-time rule must never claim a drop that exec would not make, so each
// case where routing depends on something the definition does not show has to
// produce silence.
func TestUnroutedRuleStaysQuietWhenItCannotKnow(t *testing.T) {
	child := wdg("filter", "f")
	cases := []struct {
		name string
		def  *WidgetDefinition
	}{
		{"no definition at all", nil},
		{"skeleton definition", &WidgetDefinition{WidgetID: "x", MDLName: "X"}},
		{
			// With no child slots, applyChildSlots returns early and the auto
			// pass hands leftovers to the first widgets-typed property — which
			// this function cannot see.
			name: "object lists but no child slots",
			def: &WidgetDefinition{WidgetID: "x", MDLName: "X",
				ObjectLists: []ObjectListMapping{{PropertyKey: "columns", MDLContainer: "column"}}},
		},
		{
			// `template` IS the catch-all (defaultSlotContainer).
			name: "has a template slot",
			def: &WidgetDefinition{WidgetID: "x", MDLName: "X",
				ChildSlots: []ChildSlotMapping{{PropertyKey: "content", MDLContainer: "template"}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := droppedTypes(c.def, wdg("x", "w", child)); got != nil {
				t.Errorf("reported %v, want silence", got)
			}
		})
	}
}

// TestUnroutedMessageNamesThisWidgetsSpelling: the useful half of the
// diagnostic. An author who copied a gallery example needs the word DataGrid
// uses for the same property, not a list to search.
func TestUnroutedMessageNamesThisWidgetsSpelling(t *testing.T) {
	def := datagridLikeDef()
	msg := unroutedChildMessage(def, wdg("filter", "f"))
	if !strings.Contains(msg, "`controlbar`") {
		t.Errorf("message does not name DataGrid's spelling of the slot: %s", msg)
	}
	if !strings.Contains(msg, "dropped on write") {
		t.Errorf("message does not say what would happen: %s", msg)
	}
	// Control: a child with no counterpart elsewhere gets no invented advice.
	if m := unroutedChildMessage(def, wdg("actionbutton", "btn")); strings.Contains(m, "spelled") {
		t.Errorf("invented an alternative spelling for a widget that has none: %s", m)
	}
}

// TestCheckAndExecAgreeOnUnroutedChildren is the anti-drift guard. The two
// halves are separate entry points — a linter rule and a writer refusal — and
// the failure they exist to prevent is precisely the two disagreeing: a check
// that passes and a write that drops, which is where this started.
func TestCheckAndExecAgreeOnUnroutedChildren(t *testing.T) {
	def := datagridLikeDef()
	for _, w := range []*ast.WidgetV3{
		wdg("datagrid", "dg", wdg("filter", "f")),
		wdg("datagrid", "dg", wdg("column", "c")),
		wdg("datagrid", "dg", wdg("controlbar", "cb")),
		wdg("datagrid", "dg", wdg("actionbutton", "btn"), wdg("column", "c")),
		wdg("datagrid", "dg"),
	} {
		checkFired := len(validateUnroutedChildren(w, def, "page X")) > 0
		execRefused := refuseUnroutedChildren(def, w) != nil
		if checkFired != execRefused {
			t.Errorf("check=%v exec=%v for children %v — the two must agree",
				checkFired, execRefused, droppedTypes(def, w))
		}
	}
}

func TestUnroutedViolationIsAnError(t *testing.T) {
	v := validateUnroutedChildren(wdg("datagrid", "dg", wdg("filter", "f")), datagridLikeDef(), "page X")
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1", len(v))
	}
	if v[0].RuleID != "MDL-WIDGET29" {
		t.Errorf("RuleID = %s, want MDL-WIDGET29", v[0].RuleID)
	}
	if v[0].Severity != linter.SeverityError {
		t.Errorf("Severity = %s, want error — a dropped widget is not a style note", v[0].Severity)
	}
}
