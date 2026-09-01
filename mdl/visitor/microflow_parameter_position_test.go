// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// #993: `@position(x, y)` on a parameter declaration. Before the grammar took
// `annotation*` there, the reporter's script was three parse errors starting at
// "extraneous input '@'".
func TestParameterPositionAnnotationParses(t *testing.T) {
	script := `create nanoflow M.NF (
  @position(300, 100)
  $A: Integer,
  @position(200, 100)
  $B: Integer,
  $C: Integer
)
returns Integer as $R
begin
  @position(300, 200)
  declare $R Integer = $A + $B;
  @position(500, 200)
  return $R;
end;`
	prog, errs := Build(script)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %s", errsText(errs))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateNanoflowStmt)
	if !ok {
		t.Fatalf("statement = %T, want *ast.CreateNanoflowStmt", prog.Statements[0])
	}
	if len(stmt.Parameters) != 3 {
		t.Fatalf("got %d parameters, want 3", len(stmt.Parameters))
	}
	if p := stmt.Parameters[0].Position; p == nil || *p != (ast.Position{X: 300, Y: 100}) {
		t.Errorf("$A position = %v, want 300,100", p)
	}
	if p := stmt.Parameters[1].Position; p == nil || *p != (ast.Position{X: 200, Y: 100}) {
		t.Errorf("$B position = %v, want 200,100", p)
	}
	// Control: an unannotated parameter stays unset, so the layout places it.
	// Without this the test would pass against a visitor that stamped every
	// parameter with the same point.
	if p := stmt.Parameters[2].Position; p != nil {
		t.Errorf("$C position = %v, want nil — no annotation was written", *p)
	}
}

// An annotation a parameter does not take is recorded, not dropped, so MDL059
// can refuse it. `@postion` is the case that matters: silently ignoring it
// discards exactly the placement the author asked for.
func TestUnknownParameterAnnotationIsRecorded(t *testing.T) {
	prog, errs := Build(`create nanoflow M.NF (
  @postion(300, 100)
  $A: Integer
) begin @position(1, 2) return; end;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %s", errsText(errs))
	}
	stmt := prog.Statements[0].(*ast.CreateNanoflowStmt)
	got := stmt.Parameters[0].UnknownAnnotations
	if len(got) != 1 || got[0] != "postion" {
		t.Errorf("UnknownAnnotations = %v, want [postion]", got)
	}
	if stmt.Parameters[0].Position != nil {
		t.Errorf("a typo set a position: %v", *stmt.Parameters[0].Position)
	}
}

// `@position` with too few arguments is not a position — it is recorded as
// unknown rather than silently producing 0;0, which would be a real coordinate
// and would look deliberate.
func TestMalformedParameterPositionIsNotSilentlyZero(t *testing.T) {
	prog, errs := Build(`create nanoflow M.NF (
  @position(300)
  $A: Integer
) begin @position(1, 2) return; end;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %s", errsText(errs))
	}
	p := prog.Statements[0].(*ast.CreateNanoflowStmt).Parameters[0]
	if p.Position != nil {
		t.Errorf("position = %v, want nil", *p.Position)
	}
	if len(p.UnknownAnnotations) != 1 {
		t.Errorf("UnknownAnnotations = %v, want one entry", p.UnknownAnnotations)
	}
}
