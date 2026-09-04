// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// CapTrackV2 FINDINGS §21 — `DATAVIEW dv (…, OnClick: SHOW_PAGE …)` parses,
// passes `mxcli check`, is written by `exec` without a word, and the rendered
// element has no handler and no role="button".
//
// Measured on Mendix 11.13 by writing one page with four widgets and reading it
// back with `describe page`:
//
//	container    OnClick kept   (Pages$DivContainer.OnClickAction)
//	listview     OnClick GONE   (Mendix HAS ListView.ClickAction; mxcli has no writer)
//	dataview     OnClick GONE   (Mendix has no click action on a data view at all)
//	dynamictext  OnClick GONE   (same)
//
// `OnClick:` is an alias for `Action:` — the visitor stores both under
// Properties["Action"] — so the rule keys on the stored property and covers
// both spellings.

func clickPage(widget string) string {
	return `create page W.P ( Title: 'P' )
{
  ` + widget + `
}`
}

// The reported case, plus the two other silent drops measured beside it.
func TestMDLWIDGET23_ReportsTheDroppedAction(t *testing.T) {
	cases := []struct {
		name   string
		widget string
		want   string // a phrase the message must carry
	}{
		{
			name:   "data view (the reported case)",
			widget: `DATAVIEW dv (DataSource: MICROFLOW W.DS, OnClick: SHOW_PAGE W.P) { DYNAMICTEXT t (Content: 'x') }`,
			want:   "no click action on dataview at all",
		},
		{
			name:   "dynamictext",
			widget: `DYNAMICTEXT t (Content: 'x', OnClick: SHOW_PAGE W.P)`,
			want:   "no click action on dynamictext at all",
		},
		{
			// Mendix models this one, so it earns the other sentence: the model
			// could hold it, mxcli just does not write it.
			name:   "listview, which Mendix does model",
			widget: `LISTVIEW lv (DataSource: DATABASE W.Product, OnClick: SHOW_PAGE W.P) { DYNAMICTEXT t (Content: 'x') }`,
			want:   "Mendix does model one on listview",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := widgetViolations(t, clickPage(tc.widget), "MDL-WIDGET23")
			if len(got) != 1 {
				t.Fatalf("got %d MDL-WIDGET23 violations, want 1: %#v", len(got), got)
			}
			if got[0].Severity != linter.SeverityWarning {
				t.Errorf("severity = %v, want Warning (the MDL-WIDGET20/21 family — nothing fails the build)",
					got[0].Severity)
			}
			if !strings.Contains(got[0].Message, tc.want) {
				t.Errorf("message should say %q, got %q", tc.want, got[0].Message)
			}
			if !strings.Contains(got[0].Suggestion, "container") {
				t.Errorf("suggestion should point at a container, the widget whose on-click IS written: %q",
					got[0].Suggestion)
			}
		})
	}
}

// CONTROL 1: the widgets whose action mxcli DOES write must stay silent. A rule
// that fired on these would report the one shape that works, and `container
// (OnClick: …)` is the documented spelling for a clickable container (#603).
func TestMDLWIDGET23_WidgetsThatWriteTheActionAreClean(t *testing.T) {
	for _, w := range []string{
		`CONTAINER c (OnClick: SHOW_PAGE W.P) { DYNAMICTEXT t (Content: 'x') }`,
		`CONTAINER c (Action: SHOW_PAGE W.P) { DYNAMICTEXT t (Content: 'x') }`,
		`ACTIONBUTTON b (Caption: 'Go', Action: SHOW_PAGE W.P)`,
		`LINKBUTTON b (Caption: 'Go', Action: SHOW_PAGE W.P)`,
	} {
		t.Run(strings.Fields(w)[0]+"/"+strings.Fields(w)[3], func(t *testing.T) {
			if got := widgetViolations(t, clickPage(w), "MDL-WIDGET23"); len(got) != 0 {
				t.Errorf("MDL-WIDGET23 fired on a widget whose action is written: %#v", got)
			}
		})
	}
}

// CONTROL 2: PLUGGABLE widgets are written by the widget engine from their own
// definition, so they must stay silent — including when no project is given.
//
// This is the regression an earlier draft actually shipped: the rule reported
// everything outside an allow-list, and `mxcli check` without `-p` has no widget
// registry, so `lookupWidgetDef` returns nil for a pluggable widget too and the
// caller's "static widgets only" branch does not hold. Three shipped examples
// were flagged — `datagrid` is DataGrid 2, a pluggable widget whose `onClick`
// the engine writes. The rule now names the types it reports instead.
func TestMDLWIDGET23_PluggableWidgetsAreClean(t *testing.T) {
	for _, w := range []string{
		`DATAGRID dg (DataSource: DATABASE W.Product, onClick: microflow W.ACT) { COLUMN c1 (Attribute: Name) }`,
		`PLUGGABLEWIDGET 'com.mendix.widget.custom.badgebutton.BadgeButton' bb (onClick: microflow W.ACT)`,
	} {
		t.Run(strings.Fields(w)[0], func(t *testing.T) {
			if got := widgetViolations(t, clickPage(w), "MDL-WIDGET23"); len(got) != 0 {
				t.Errorf("MDL-WIDGET23 fired on a pluggable widget, whose action slot IS written: %#v", got)
			}
		})
	}
}

// CONTROL 3: the rule keys on the property being present, not on the widget
// type — a data view without one is an ordinary data view.
func TestMDLWIDGET23_SilentWithoutTheProperty(t *testing.T) {
	src := clickPage(`DATAVIEW dv (DataSource: MICROFLOW W.DS) { DYNAMICTEXT t (Content: 'x') }`)
	if got := widgetViolations(t, src, "MDL-WIDGET23"); len(got) != 0 {
		t.Errorf("MDL-WIDGET23 fired on a data view with no action: %#v", got)
	}
}

// clickCapableInMendix claims three Pages types carry a ClickAction. That claim
// decides which of the two messages an author sees, and it is hand-written, so
// read it back off generated/metamodel — the arbiter per CLAUDE.md.
func TestClickCapableTypesCarryClickActionInMetamodel(t *testing.T) {
	fset := token.NewFileSet()
	path := filepath.Join("..", "..", "generated", "metamodel", "types.go")
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse metamodel: %v", err)
	}

	withClickAction := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasPrefix(ts.Name.Name, "Pages") {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, fld := range st.Fields.List {
			for _, nm := range fld.Names {
				if nm.Name == "ClickAction" || nm.Name == "OnClickAction" {
					withClickAction[ts.Name.Name] = true
				}
			}
		}
		return true
	})

	// Positive control: a parse that found nothing would let every assertion
	// below pass vacuously.
	if len(withClickAction) == 0 {
		t.Fatal("found no Pages type with a click action — the parse is wrong, and a passing run would prove nothing")
	}
	if !withClickAction["PagesDivContainer"] {
		t.Error("PagesDivContainer has no click action in the metamodel, yet mxcli writes one for a container")
	}

	// Every type the rule claims Mendix models must really carry one. Getting
	// this wrong sends the author the wrong remedy — "mxcli has no writer" when
	// in fact the model has no slot.
	for mdlType, metamodelType := range map[string]string{
		"listview":     "PagesListView",
		"staticimage":  "PagesStaticImageViewer",
		"dynamicimage": "PagesDynamicImageViewer",
	} {
		if !clickCapableInMendix[mdlType] {
			t.Errorf("%s dropped out of clickCapableInMendix", mdlType)
		}
		if !withClickAction[metamodelType] {
			t.Errorf("clickCapableInMendix says Mendix models a click action on %s, but %s carries none",
				mdlType, metamodelType)
		}
	}

	// And the other direction, for the types the rule says have no slot at all.
	for mdlType, metamodelType := range map[string]string{
		"dataview":    "PagesDataView",
		"dynamictext": "PagesDynamicText",
		"textbox":     "PagesTextBox",
		"groupbox":    "PagesGroupBox",
	} {
		if clickCapableInMendix[mdlType] {
			t.Errorf("%s is listed as click-capable; the message would name the wrong remedy", mdlType)
		}
		if withClickAction[metamodelType] {
			t.Errorf("%s DOES carry a click action in the metamodel — %s belongs in clickCapableInMendix",
				metamodelType, mdlType)
		}
	}
}
