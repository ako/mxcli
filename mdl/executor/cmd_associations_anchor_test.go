// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// upstream #872. `applyAnchors` is the single point where authored anchors reach
// the model, on the CREATE, CREATE OR MODIFY and ALTER paths alike.
//
// The property that matters is asymmetric: naming an end SETS it, and NOT naming
// one PRESERVES what is stored. Without that, a `create or modify association`
// whose subject is the delete behaviour would flatten a hand-tuned line — which
// is the bug the preservation slice of #872 fixed, reintroduced through the new
// syntax.
func TestApplyAnchors(t *testing.T) {
	stored := func() *domainmodel.Association {
		return &domainmodel.Association{
			ParentConnection: &model.Point{X: 11, Y: 99},
			ChildConnection:  &model.Point{X: 9, Y: 0},
		}
	}

	t.Run("neither end named preserves both", func(t *testing.T) {
		a := stored()
		applyAnchors(a, nil, nil)
		if *a.ParentConnection != (model.Point{X: 11, Y: 99}) || *a.ChildConnection != (model.Point{X: 9, Y: 0}) {
			t.Fatalf("a statement that says nothing about anchors must not touch them, got %+v / %+v",
				*a.ParentConnection, *a.ChildConnection)
		}
	})

	t.Run("one end named leaves the other alone", func(t *testing.T) {
		a := stored()
		applyAnchors(a, &ast.Position{X: 50, Y: 100}, nil)
		if *a.ParentConnection != (model.Point{X: 50, Y: 100}) {
			t.Errorf("from anchor = %+v, want {50 100}", *a.ParentConnection)
		}
		if *a.ChildConnection != (model.Point{X: 9, Y: 0}) {
			t.Errorf("to anchor = %+v, want the stored {9 0} — it was not named", *a.ChildConnection)
		}
	})

	t.Run("the zero point is a value, not an absence", func(t *testing.T) {
		a := stored()
		applyAnchors(a, &ast.Position{X: 0, Y: 0}, &ast.Position{X: 0, Y: 0})
		if *a.ParentConnection != (model.Point{}) || *a.ChildConnection != (model.Point{}) {
			t.Fatalf("(0, 0) is the box's top-left and must be written, got %+v / %+v",
				*a.ParentConnection, *a.ChildConnection)
		}
	})

	t.Run("an association with nothing stored takes both", func(t *testing.T) {
		a := &domainmodel.Association{}
		applyAnchors(a, &ast.Position{X: 0, Y: 54}, &ast.Position{X: 100, Y: 54})
		if a.ParentConnection == nil || a.ChildConnection == nil {
			t.Fatal("authored anchors were dropped")
		}
	})
}

// DESCRIBE must emit anchors as the syntax that AUTHORS them, or a
// describe → edit → exec cycle loses the layout — the round-trip requirement the
// issue asked for. Feeding the emitted line straight back through the parser is
// the only check that proves the two sides agree; asserting on a string literal
// would pass against a formatter that emits something nothing can read.
func TestDescribeConnectionPoints_RoundTripsThroughTheParser(t *testing.T) {
	emit := func(t *testing.T, parent, child *model.Point) string {
		t.Helper()
		var buf bytes.Buffer
		describeConnectionPoints(&ExecContext{Output: &buf},
			&domainmodel.Association{ParentConnection: parent, ChildConnection: child})
		return buf.String()
	}

	// An association mxcli created itself carries the defaults and must describe
	// exactly as it did before anchors existed.
	if out := emit(t, &model.Point{X: 0, Y: 50}, &model.Point{X: 100, Y: 50}); out != "" {
		t.Errorf("default anchors must not be printed, got %q", out)
	}
	if out := emit(t, nil, nil); out != "" {
		t.Errorf("absent anchors must not be printed, got %q", out)
	}

	line := emit(t, &model.Point{X: 11, Y: 99}, &model.Point{X: 9, Y: 0})
	if !strings.HasPrefix(line, "@anchor(") {
		t.Fatalf("expected an @anchor annotation, got %q", line)
	}

	prog, errs := visitor.Build(line + "create association M.Child_Parent from M.Child to M.Parent;")
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL the parser rejects (%q): %v", line, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.CreateAssociationStmt", prog.Statements[0])
	}
	if stmt.FromAnchor == nil || *stmt.FromAnchor != (ast.Position{X: 11, Y: 99}) {
		t.Errorf("from anchor did not survive the round trip: %+v", stmt.FromAnchor)
	}
	if stmt.ToAnchor == nil || *stmt.ToAnchor != (ast.Position{X: 9, Y: 0}) {
		t.Errorf("to anchor did not survive the round trip: %+v", stmt.ToAnchor)
	}
}
