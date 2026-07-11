// SPDX-License-Identifier: Apache-2.0

// Issue #532 — `placeholder <Name> { … }` blocks bind widgets to named layout
// placeholders. Bare widgets still bind to Main.
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestPagePlaceholderBlocksParsed(t *testing.T) {
	input := `create page M.P (title: 'x', layout: M.Master) {
		dynamictext bare (Content: 'bare goes to Main')
		placeholder Main {
			dynamictext m1 (Content: 'in main')
		}
		placeholder Right {
			dynamictext r1 (Content: 'in right')
			dynamictext r2 (Content: 'also right')
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse error: %v", e)
		}
		t.FailNow()
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("expected CreatePageStmtV3, got %T", prog.Statements[0])
	}

	// Bare widget stays on the flat Widgets list (→ Main).
	if len(stmt.Widgets) != 1 || stmt.Widgets[0].Name != "bare" {
		t.Errorf("bare widgets = %d (want 1 named 'bare')", len(stmt.Widgets))
	}
	// Two placeholder blocks captured, in order.
	if len(stmt.Placeholders) != 2 {
		t.Fatalf("placeholders = %d, want 2", len(stmt.Placeholders))
	}
	if stmt.Placeholders[0].Name != "Main" || len(stmt.Placeholders[0].Widgets) != 1 {
		t.Errorf("placeholder[0] = %q with %d widgets, want Main/1", stmt.Placeholders[0].Name, len(stmt.Placeholders[0].Widgets))
	}
	if stmt.Placeholders[1].Name != "Right" || len(stmt.Placeholders[1].Widgets) != 2 {
		t.Errorf("placeholder[1] = %q with %d widgets, want Right/2", stmt.Placeholders[1].Name, len(stmt.Placeholders[1].Widgets))
	}
}

func TestPageWithoutPlaceholdersUnaffected(t *testing.T) {
	prog, errs := Build(`create page M.P (title: 'x', layout: M.L) { dynamictext t (Content: 'hi') };`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if len(stmt.Widgets) != 1 {
		t.Errorf("Widgets = %d, want 1", len(stmt.Widgets))
	}
	if len(stmt.Placeholders) != 0 {
		t.Errorf("Placeholders = %d, want 0 (backward compat)", len(stmt.Placeholders))
	}
}
