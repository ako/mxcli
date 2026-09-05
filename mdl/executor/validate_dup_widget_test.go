// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestCheckDuplicateWidgetNames_Unit is the pure-logic guard for FINDINGS #15.
func TestCheckDuplicateWidgetNames_Unit(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{Type: "container", Name: "ruTop", Children: []*ast.WidgetV3{
			{Type: "listview", Name: "ruTop"},
		}},
	}
	errs := checkDuplicateWidgetNames(widgets, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "ruTop") {
		t.Fatalf("expected one duplicate error for ruTop, got %v", errs)
	}
}

// TestCheckDuplicateWidgetNames_Parsed guards the end-to-end path: a page parsed
// from MDL where a container and a listview share a name must be flagged (CE0495).
func TestCheckDuplicateWidgetNames_Parsed(t *testing.T) {
	src := `create or replace page "M"."P" (URL: 'p') {
  container ruTop {
    listview ruTop (DataSource: DATABASE M.Thing) { }
  }
}
/`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	pg, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("statement 0 = %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	dup := checkDuplicateWidgetNames(pg.Widgets, nil)
	if len(dup) != 1 || !strings.Contains(dup[0], "ruTop") {
		t.Fatalf("expected duplicate ruTop error from parsed page, got %v (widget names may not be populated for containers)", dup)
	}
}
