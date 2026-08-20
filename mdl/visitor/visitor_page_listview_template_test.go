// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// listViewOf parses a one-statement page script and returns its list view.
func listViewOf(t *testing.T, src string) *ast.WidgetV3 {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("parse error: %v", err)
		}
		t.FailNow()
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("expected CreatePageStmtV3, got %T", prog.Statements[0])
	}
	var find func(ws []*ast.WidgetV3) *ast.WidgetV3
	find = func(ws []*ast.WidgetV3) *ast.WidgetV3 {
		for _, w := range ws {
			if w.Type == "listview" {
				return w
			}
			if got := find(w.Children); got != nil {
				return got
			}
		}
		return nil
	}
	lv := find(stmt.Widgets)
	if lv == nil {
		t.Fatal("no listview in the parsed page")
	}
	return lv
}

// TestListViewSpecializationTemplateParses pins the surface syntax: a template
// is identified by the entity it renders, source order is the stored order, and
// the list view's own widgets stay separate from its templates.
func TestListViewSpecializationTemplateParses(t *testing.T) {
	lv := listViewOf(t, `
create page Pages.Vehicle_Overview (Title: 'Vehicles') {
  listview vehicleListView (DataSource: database from Pages.Vehicle) {
    dynamictext defaultVehicle (Content: 'v')
    template for Pages.Bus   { dynamictext busLabel (Content: 'b') }
    template for Pages.Truck { dynamictext truckLabel (Content: 't') }
  }
};`)

	if len(lv.Children) != 3 {
		t.Fatalf("expected 3 body children (1 default widget + 2 templates), got %d", len(lv.Children))
	}
	if got := lv.Children[0]; got.Specialization != "" || got.Name != "defaultVehicle" {
		t.Errorf("first child = %+v, want the plain default widget with no specialization", got)
	}

	// Order is authored, not derived: TestApp's four templates are Bus, Truck,
	// Car, SUV — neither alphabetical nor domain-model order.
	want := []string{"Pages.Bus", "Pages.Truck"}
	for i, w := range want {
		child := lv.Children[i+1]
		if child.Type != "template" {
			t.Errorf("child %d Type = %q, want template", i+1, child.Type)
		}
		if child.Specialization != w {
			t.Errorf("child %d Specialization = %q, want %q", i+1, child.Specialization, w)
		}
		if child.Name != "" {
			t.Errorf("child %d Name = %q, want empty — a list view template has no name", i+1, child.Name)
		}
		if len(child.Children) != 1 {
			t.Errorf("child %d has %d widget(s), want 1", i+1, len(child.Children))
		}
	}
}

// TestGalleryNamedTemplateStillParses is the collision guard.
//
// `template <name> { }` already existed as a Gallery content slot before
// `template for <Entity> { }` was added, and FOR is in the parser's `keyword`
// rule — so a Gallery template could be swallowed by the new alternative, or
// vice versa. Both forms must keep their own meaning.
func TestGalleryNamedTemplateStillParses(t *testing.T) {
	lv := listViewOf(t, `
create page P.G (Title: 'G') {
  listview outer (DataSource: database from P.E) {
    gallery g (DataSource: database from P.E) {
      template tmpl1 { dynamictext a (Content: 'x') }
    }
  }
};`)
	gallery := lv.Children[0]
	if gallery.Type != "gallery" {
		t.Fatalf("expected a gallery, got %q", gallery.Type)
	}
	tmpl := gallery.Children[0]
	if tmpl.Name != "tmpl1" {
		t.Errorf("gallery template Name = %q, want tmpl1", tmpl.Name)
	}
	if tmpl.Specialization != "" {
		t.Errorf("gallery template Specialization = %q, want empty — a named slot is "+
			"not a specialization template", tmpl.Specialization)
	}
}

// TestAlterPageDropTemplateParses pins the ALTER PAGE half. A template has no
// name, so it cannot be reached through widgetRef like every other DROP target —
// it is addressed by the entity it renders plus the list view holding it, since
// one page can carry two list views with a template for the same entity.
func TestAlterPageDropTemplateParses(t *testing.T) {
	prog, errs := Build(`ALTER PAGE Pages.Vehicle_Overview {
		DROP TEMPLATE FOR Pages.SUV IN vehicleListView
	};`)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("parse error: %v", err)
		}
		t.FailNow()
	}
	stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok {
		t.Fatalf("expected AlterPageStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(stmt.Operations))
	}
	op, ok := stmt.Operations[0].(*ast.DropListViewTemplateOp)
	if !ok {
		t.Fatalf("expected DropListViewTemplateOp, got %T", stmt.Operations[0])
	}
	if op.Specialization != "Pages.SUV" {
		t.Errorf("Specialization = %q, want Pages.SUV", op.Specialization)
	}
	if op.ListView != "vehicleListView" {
		t.Errorf("ListView = %q, want vehicleListView", op.ListView)
	}
}

// TestAlterPageInsertTemplateParses pins that adding a template reuses INSERT
// INTO with the same `template for` block CREATE PAGE uses — one spelling of a
// template everywhere, rather than a second INSERT TEMPLATE form.
func TestAlterPageInsertTemplateParses(t *testing.T) {
	prog, errs := Build(`ALTER PAGE Pages.Vehicle_Overview {
		INSERT INTO vehicleListView {
			template for Pages.Motorcycle { dynamictext mcLabel (Content: 'm') }
		}
	};`)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("parse error: %v", err)
		}
		t.FailNow()
	}
	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	op, ok := stmt.Operations[0].(*ast.InsertWidgetOp)
	if !ok {
		t.Fatalf("expected InsertWidgetOp, got %T", stmt.Operations[0])
	}
	if len(op.Widgets) != 1 {
		t.Fatalf("expected 1 inserted node, got %d", len(op.Widgets))
	}
	if got := op.Widgets[0].Specialization; got != "Pages.Motorcycle" {
		t.Errorf("Specialization = %q, want Pages.Motorcycle", got)
	}
}
