// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#515, the bulk form. A house style is "every data grid is compact and
// striped", which should be one statement and not one per page.
func parseAlterPagesStyling(t *testing.T, src string) *ast.AlterPagesStylingStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	s, ok := prog.Statements[0].(*ast.AlterPagesStylingStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.AlterPagesStylingStmt", prog.Statements[0])
	}
	return s
}

func TestAlterPagesStyling_Parses(t *testing.T) {
	s := parseAlterPagesStyling(t,
		`alter pages in Sales set 'Compact' = on, 'Striped' = on where widgettype = datagrid dry run;`)

	if s.Module != "Sales" {
		t.Errorf("Module = %q, want Sales", s.Module)
	}
	if s.WidgetType != "datagrid" {
		t.Errorf("WidgetType = %q, want datagrid", s.WidgetType)
	}
	if !s.DryRun {
		t.Error("DryRun = false, want true")
	}
	if len(s.Assignments) != 2 {
		t.Fatalf("got %d assignments, want 2", len(s.Assignments))
	}
	for _, a := range s.Assignments {
		if !a.IsToggle || !a.ToggleOn {
			t.Errorf("assignment %+v: want a toggle set ON", a)
		}
	}
}

// Without IN, the module is empty and the widget type must still land in the
// right field — the rule has two identifierOrKeyword positions and ANTLR returns
// one list, so reading them positionally is where this goes wrong.
func TestAlterPagesStyling_WithoutModule(t *testing.T) {
	s := parseAlterPagesStyling(t,
		`alter pages set 'Striped' = off where widgettype = datagrid;`)

	if s.Module != "" {
		t.Errorf("Module = %q, want empty — no IN clause was given", s.Module)
	}
	if s.WidgetType != "datagrid" {
		t.Errorf("WidgetType = %q, want datagrid", s.WidgetType)
	}
	if s.DryRun {
		t.Error("DryRun = true with no DRY RUN clause")
	}
	if len(s.Assignments) != 1 || !s.Assignments[0].IsToggle || s.Assignments[0].ToggleOn {
		t.Errorf("assignments = %+v, want one toggle set OFF", s.Assignments)
	}
}

// An option value, and a full widget id in place of the keyword.
func TestAlterPagesStyling_OptionValueAndFullWidgetID(t *testing.T) {
	s := parseAlterPagesStyling(t,
		`alter pages set 'Row size' = 'Small' where widgettype = 'com.mendix.widget.web.datagrid.Datagrid';`)

	if s.WidgetType != "com.mendix.widget.web.datagrid.Datagrid" {
		t.Errorf("WidgetType = %q, want the full id", s.WidgetType)
	}
	if len(s.Assignments) != 1 || s.Assignments[0].Value != "Small" || s.Assignments[0].IsToggle {
		t.Errorf("assignments = %+v, want one option value Small", s.Assignments)
	}
}

// The sibling statement must keep parsing as itself. Both start `ALTER PAGES`
// and are told apart by what follows SET, so a grammar change here is exactly
// where the layout form would be swallowed.
func TestAlterPagesStyling_DoesNotShadowTheLayoutForm(t *testing.T) {
	prog, errs := visitor.Build(`alter pages in Sales set layout = Sales.App_Default;`)
	if len(errs) > 0 {
		t.Fatalf("the layout form stopped parsing: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.AlterPagesLayoutStmt); !ok {
		t.Errorf("statement is %T, want *ast.AlterPagesLayoutStmt", prog.Statements[0])
	}
}

// The selector is what makes `datagrid` mean Data grid 2 and nothing else.
// Measured: `WidgetType LIKE '%datagrid%'` matches 20 widgets in 6 containers on
// a blank project, because it also sweeps in the data grid's three FILTER
// widgets — different widgets that do not carry its design properties.
func TestResolveWidgetTypeSelector(t *testing.T) {
	got := resolveWidgetTypeSelector("datagrid")
	if !strings.Contains(got, ".") {
		t.Fatalf("keyword did not resolve to a widget id: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "datagrid") {
		t.Errorf("keyword resolved to %q, which does not look like the data grid", got)
	}
	// Specifically NOT a filter — the thing the LIKE predicate gets wrong.
	if strings.Contains(strings.ToLower(got), "filter") {
		t.Errorf("keyword resolved to a filter widget: %q", got)
	}
	// A full id passes through untouched.
	const id = "com.acme.widget.Thing"
	if resolveWidgetTypeSelector(id) != id {
		t.Errorf("a full widget id was rewritten to %q", resolveWidgetTypeSelector(id))
	}
	// An unknown keyword is left alone rather than guessed at, so the catalog
	// query simply matches nothing and the statement says so.
	if got := resolveWidgetTypeSelector("nosuchwidget"); got != "nosuchwidget" {
		t.Errorf("an unknown keyword was rewritten to %q", got)
	}
}
